package charger

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

func nextStatusTime(now, previous time.Time) time.Time {
	now = now.UTC().Truncate(time.Microsecond)
	if !now.After(previous) {
		return previous.UTC().Add(time.Microsecond)
	}
	return now
}

// Reconcile advances reservations and ends due sessions after a restart or time boundary.
func (s *Service) Reconcile(ctx context.Context, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC().Truncate(time.Microsecond)
	rows, err := s.store.pool.Query(ctx, "SELECT id FROM chargers ORDER BY id")
	if err != nil {
		return err
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		event, err := s.reconcileCharger(ctx, id, now)
		if err != nil {
			return err
		}
		if event != nil {
			s.publish(*event)
		}
	}
	return nil
}

func (s *Service) reconcileCharger(ctx context.Context, id string, now time.Time) (*StatusEvent, error) {
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var oldStatus string
	var updatedAt time.Time
	if err := tx.QueryRow(ctx, "SELECT status,updated_at FROM chargers WHERE id=$1 FOR UPDATE", id).Scan(&oldStatus, &updatedAt); err != nil {
		return nil, err
	}
	status := oldStatus
	var sessionID string
	var reservationID *string
	var startedAt, plannedEnd time.Time
	err = tx.QueryRow(ctx, `SELECT id,reservation_id,started_at,planned_end_at FROM charging_sessions
		WHERE charger_id=$1 AND ended_at IS NULL`, id).Scan(&sessionID, &reservationID, &startedAt, &plannedEnd)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	unfinished := err == nil
	dueOrdinary := unfinished && reservationID == nil && !plannedEnd.After(now)
	if unfinished && !plannedEnd.After(now) {
		if _, err := tx.Exec(ctx, "UPDATE charging_sessions SET ended_at=$2 WHERE id=$1", sessionID, now); err != nil {
			return nil, err
		}
		if reservationID != nil {
			if _, err := tx.Exec(ctx, "UPDATE reservations SET status='COMPLETED',updated_at=$2 WHERE id=$1 AND status='ACTIVE'", *reservationID, now); err != nil {
				return nil, err
			}
		}
		unfinished = false
		if status == "CHARGING" {
			status = "AVAILABLE"
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE reservations SET status='EXPIRED',updated_at=$2
		WHERE charger_id=$1 AND status IN ('SCHEDULED','WAITING') AND end_time <= $2`, id, now); err != nil {
		return nil, err
	}
	if dueOrdinary {
		var pending bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reservations WHERE charger_id=$1
			AND status IN ('SCHEDULED','WAITING','ACTIVE') AND end_time>$2)`, id, now).Scan(&pending); err != nil {
			return nil, err
		}
		if !pending {
			status = "MAINTENANCE"
		}
	}
	var eligibleID string
	var reservationEnd time.Time
	err = tx.QueryRow(ctx, `SELECT id,end_time FROM reservations WHERE charger_id=$1 AND status IN ('SCHEDULED','WAITING')
		AND start_time<=$2 AND end_time>$2 ORDER BY start_time,id LIMIT 1`, id, now).Scan(&eligibleID, &reservationEnd)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if err == nil {
		if status == "AVAILABLE" && !unfinished {
			if _, err := tx.Exec(ctx, `INSERT INTO charging_sessions(charger_id,reservation_id,started_at,planned_end_at)
				VALUES($1,$2,$3,$4)`, id, eligibleID, now, reservationEnd); err != nil {
				return nil, err
			}
			if _, err := tx.Exec(ctx, "UPDATE reservations SET status='ACTIVE',updated_at=$2 WHERE id=$1", eligibleID, now); err != nil {
				return nil, err
			}
			status = "CHARGING"
		} else {
			if _, err := tx.Exec(ctx, "UPDATE reservations SET status='WAITING',updated_at=$2 WHERE id=$1 AND status='SCHEDULED'", eligibleID, now); err != nil {
				return nil, err
			}
		}
	}
	var event *StatusEvent
	if status != oldStatus {
		at := nextStatusTime(now, updatedAt)
		if _, err := tx.Exec(ctx, "UPDATE chargers SET status=$2,updated_at=$3 WHERE id=$1", id, status, at); err != nil {
			return nil, err
		}
		event = &StatusEvent{ChargerID: id, Status: status, UpdatedAt: at}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return event, nil
}

// Simulate changes one eligible charger while respecting reserved intervals.
func (s *Service) Simulate(ctx context.Context, now time.Time, pick func(int) int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	now = now.UTC().Truncate(time.Microsecond)
	rows, err := s.store.pool.Query(ctx, `SELECT c.id FROM chargers c WHERE c.status IN ('AVAILABLE','MAINTENANCE')
		AND NOT EXISTS(SELECT 1 FROM charging_sessions cs WHERE cs.charger_id=c.id AND cs.ended_at IS NULL)
		AND NOT EXISTS(SELECT 1 FROM reservations r WHERE r.charger_id=c.id AND r.status IN ('SCHEDULED','WAITING','ACTIVE')
			AND r.start_time<=$1 AND r.end_time>$1)
		AND (c.status='AVAILABLE' OR NOT EXISTS(SELECT 1 FROM reservations r WHERE r.charger_id=c.id
			AND r.status IN ('SCHEDULED','WAITING','ACTIVE') AND r.end_time>$1)) ORDER BY c.id`, now)
	if err != nil {
		return err
	}
	ids := make([]string, 0)
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		return nil
	}
	if pick == nil {
		pick = func(int) int { return 0 }
	}
	chosen := pick(len(ids))
	if chosen < 0 || chosen >= len(ids) {
		return errors.New("simulator selected an invalid charger index")
	}
	return s.simulateCharger(ctx, ids[chosen], now)
}

func (s *Service) simulateCharger(ctx context.Context, id string, now time.Time) error {
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var status string
	var updatedAt time.Time
	if err := tx.QueryRow(ctx, "SELECT status,updated_at FROM chargers WHERE id=$1 FOR UPDATE", id).Scan(&status, &updatedAt); err != nil {
		return err
	}
	if status != "AVAILABLE" && status != "MAINTENANCE" {
		return nil
	}
	var unfinished bool
	if err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM charging_sessions WHERE charger_id=$1 AND ended_at IS NULL)", id).Scan(&unfinished); err != nil {
		return err
	}
	if unfinished {
		return nil
	}
	var nextStart *time.Time
	if err := tx.QueryRow(ctx, `SELECT min(start_time) FROM reservations WHERE charger_id=$1
		AND status IN ('SCHEDULED','WAITING','ACTIVE') AND end_time>$2`, id, now).Scan(&nextStart); err != nil {
		return err
	}
	if status == "MAINTENANCE" && nextStart != nil {
		return nil
	}
	if status == "AVAILABLE" {
		end := now.Add(12 * time.Second)
		if nextStart != nil && nextStart.Before(end) {
			end = *nextStart
		}
		if !end.After(now) {
			return nil
		}
		if _, err := tx.Exec(ctx, "INSERT INTO charging_sessions(charger_id,started_at,planned_end_at) VALUES($1,$2,$3)", id, now, end); err != nil {
			return err
		}
		status = "CHARGING"
	} else {
		status = "AVAILABLE"
	}
	at := nextStatusTime(now, updatedAt)
	if _, err := tx.Exec(ctx, "UPDATE chargers SET status=$2,updated_at=$3 WHERE id=$1", id, status, at); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.publish(StatusEvent{ChargerID: id, Status: status, UpdatedAt: at})
	return nil
}
