package charger

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func NewStore(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

func (s *Store) ListChargers(ctx context.Context) ([]Charger, error) {
	rows, err := s.pool.Query(ctx, "SELECT id, name, location, status, updated_at FROM chargers ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	chargers := make([]Charger, 0)
	for rows.Next() {
		var c Charger
		if err := rows.Scan(&c.ID, &c.Name, &c.Location, &c.Status, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.UpdatedAt = c.UpdatedAt.UTC()
		chargers = append(chargers, c)
	}
	return chargers, rows.Err()
}

func (s *Store) ListReservations(ctx context.Context, userID string) ([]Reservation, error) {
	rows, err := s.pool.Query(ctx, `SELECT id, user_id, charger_id, start_time, end_time, status, created_at, updated_at
		FROM reservations WHERE user_id=$1 ORDER BY start_time DESC, id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Reservation, 0)
	for rows.Next() {
		var r Reservation
		if err := rows.Scan(&r.ID, &r.UserID, &r.ChargerID, &r.StartTime, &r.EndTime, &r.Status, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.UTC()
		result = append(result, r)
	}
	return result, rows.Err()
}

func (r *Reservation) UTC() {
	r.StartTime, r.EndTime = r.StartTime.UTC(), r.EndTime.UTC()
	r.CreatedAt, r.UpdatedAt = r.CreatedAt.UTC(), r.UpdatedAt.UTC()
}

func (s *Store) reserve(ctx context.Context, actorID, chargerID string, start, end, now time.Time) (Reservation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Reservation{}, err
	}
	defer tx.Rollback(ctx)
	var status string
	err = tx.QueryRow(ctx, `SELECT status FROM chargers WHERE id=$1 FOR UPDATE`, chargerID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrChargerNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	if status == "MAINTENANCE" {
		return Reservation{}, ErrMaintenance
	}
	var unfinished, occupied bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM charging_sessions WHERE charger_id=$1 AND ended_at IS NULL),
		EXISTS(SELECT 1 FROM charging_sessions WHERE charger_id=$1 AND ended_at IS NULL
		AND (planned_end_at <= $4 OR tstzrange(started_at, planned_end_at, '[)') && tstzrange($2, $3, '[)')))`, chargerID, start, end, now).Scan(&unfinished, &occupied)
	if err != nil {
		return Reservation{}, err
	}
	if occupied || (status == "CHARGING" && !unfinished) {
		return Reservation{}, ErrOccupied
	}
	var conflict bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM reservations WHERE charger_id=$1
		AND status <> 'CANCELLED' AND tstzrange(start_time, end_time, '[)') && tstzrange($2, $3, '[)'))`, chargerID, start, end).Scan(&conflict)
	if err != nil {
		return Reservation{}, err
	}
	if conflict {
		return Reservation{}, ErrReservationConflict
	}
	var r Reservation
	err = tx.QueryRow(ctx, `INSERT INTO reservations(user_id,charger_id,start_time,end_time)
		VALUES($1,$2,$3,$4) RETURNING id,user_id,charger_id,start_time,end_time,status,created_at,updated_at`, actorID, chargerID, start, end).
		Scan(&r.ID, &r.UserID, &r.ChargerID, &r.StartTime, &r.EndTime, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23P01" {
			return Reservation{}, ErrReservationConflict
		}
		return Reservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Reservation{}, err
	}
	r.UTC()
	return r, nil
}

func (s *Store) cancelReservation(ctx context.Context, actorID, reservationID string, now time.Time) (Reservation, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return Reservation{}, err
	}
	defer tx.Rollback(ctx)
	var chargerID string
	err = tx.QueryRow(ctx, "SELECT charger_id FROM reservations WHERE id=$1 AND user_id=$2", reservationID, actorID).Scan(&chargerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, ErrReservationNotFound
	}
	if err != nil {
		return Reservation{}, err
	}
	var lockedID string
	if err := tx.QueryRow(ctx, "SELECT id FROM chargers WHERE id=$1 FOR UPDATE", chargerID).Scan(&lockedID); err != nil {
		return Reservation{}, err
	}
	var status string
	if err := tx.QueryRow(ctx, "SELECT status FROM reservations WHERE id=$1 AND user_id=$2 FOR UPDATE", reservationID, actorID).Scan(&status); err != nil {
		return Reservation{}, err
	}
	if status != "SCHEDULED" && status != "WAITING" {
		return Reservation{}, ErrReservationNotCancellable
	}
	var r Reservation
	err = tx.QueryRow(ctx, `UPDATE reservations SET status='CANCELLED', updated_at=$2 WHERE id=$1
		RETURNING id,user_id,charger_id,start_time,end_time,status,created_at,updated_at`, reservationID, now).
		Scan(&r.ID, &r.UserID, &r.ChargerID, &r.StartTime, &r.EndTime, &r.Status, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return Reservation{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Reservation{}, err
	}
	r.UTC()
	return r, nil
}
