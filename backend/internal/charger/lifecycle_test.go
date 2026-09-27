package charger_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"ev-charger-assessment/backend/internal/charger"
	"ev-charger-assessment/backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newLifecycleService(t *testing.T) (*pgxpool.Pool, *charger.Service, *[]charger.StatusEvent) {
	t.Helper()
	pool, _ := testutil.NewDatabase(t, true)
	events := make([]charger.StatusEvent, 0)
	service := charger.NewService(charger.NewStore(pool), nil, func(e charger.StatusEvent) { events = append(events, e) })
	return pool, service, &events
}

func insertReservation(t *testing.T, pool *pgxpool.Pool, chargerID string, start, end time.Time) string {
	t.Helper()
	var id string
	err := pool.QueryRow(context.Background(), `INSERT INTO reservations(user_id,charger_id,start_time,end_time)
		SELECT id,$1,$2,$3 FROM users WHERE username='demo' RETURNING id`, chargerID, start, end).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func reservationStatus(t *testing.T, pool *pgxpool.Pool, id string) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(context.Background(), "SELECT status FROM reservations WHERE id=$1", id).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func chargerStatus(t *testing.T, pool *pgxpool.Pool, id string) (string, time.Time) {
	t.Helper()
	var status string
	var updatedAt time.Time
	if err := pool.QueryRow(context.Background(), "SELECT status,updated_at FROM chargers WHERE id=$1", id).Scan(&status, &updatedAt); err != nil {
		t.Fatal(err)
	}
	return status, updatedAt
}

func TestChargerStatusSimulation(t *testing.T) {
	t.Run("persisted update and event", func(t *testing.T) {
		pool, service, events := newLifecycleService(t)
		at := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
		if err := service.Simulate(context.Background(), at, func(int) int { return 0 }); err != nil {
			t.Fatal(err)
		}
		status, updatedAt := chargerStatus(t, pool, "charger-1")
		if status != "CHARGING" || len(*events) != 1 || (*events)[0].ChargerID != "charger-1" || (*events)[0].Status != status || !(*events)[0].UpdatedAt.Equal(updatedAt) {
			t.Fatalf("status=%s events=%+v", status, *events)
		}
		var end time.Time
		if err := pool.QueryRow(context.Background(), "SELECT planned_end_at FROM charging_sessions WHERE charger_id='charger-1' AND ended_at IS NULL").Scan(&end); err != nil {
			t.Fatal(err)
		}
		if !end.Equal(at.Add(12 * time.Second)) {
			t.Fatalf("planned end=%s", end)
		}
	})
	t.Run("no eligible charger", func(t *testing.T) {
		pool, service, events := newLifecycleService(t)
		if _, err := pool.Exec(context.Background(), "UPDATE chargers SET status='CHARGING'"); err != nil {
			t.Fatal(err)
		}
		if err := service.Simulate(context.Background(), time.Now().Add(time.Hour), func(int) int { t.Fatal("no selection expected"); return 0 }); err != nil {
			t.Fatal(err)
		}
		if len(*events) != 0 {
			t.Fatalf("events=%+v", *events)
		}
	})
	t.Run("rollback emits nothing", func(t *testing.T) {
		pool, service, events := newLifecycleService(t)
		_, err := pool.Exec(context.Background(), `CREATE FUNCTION reject_status_change() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'reject'; END $$;
			CREATE TRIGGER reject_status BEFORE UPDATE OF status ON chargers FOR EACH ROW EXECUTE FUNCTION reject_status_change()`)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.Simulate(context.Background(), time.Now().Add(time.Hour), func(int) int { return 0 }); err == nil {
			t.Fatal("expected transaction failure")
		}
		status, _ := chargerStatus(t, pool, "charger-1")
		if status != "AVAILABLE" || len(*events) != 0 {
			t.Fatalf("status=%s events=%+v", status, *events)
		}
		var count int
		if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM charging_sessions WHERE charger_id='charger-1'").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rolled-back sessions=%d", count)
		}
	})
	t.Run("monotonic event timestamps", func(t *testing.T) {
		pool, service, events := newLifecycleService(t)
		at := time.Now().UTC().Add(time.Hour).Truncate(time.Microsecond)
		if err := service.Simulate(context.Background(), at, func(int) int { return 2 }); err != nil {
			t.Fatal(err)
		}
		if err := service.Simulate(context.Background(), at, func(int) int { return 2 }); err != nil {
			t.Fatal(err)
		}
		if len(*events) != 2 || !(*events)[1].UpdatedAt.After((*events)[0].UpdatedAt) {
			t.Fatalf("events=%+v", *events)
		}
		status, updatedAt := chargerStatus(t, pool, "charger-3")
		if status != "CHARGING" || !updatedAt.Equal((*events)[1].UpdatedAt) {
			t.Fatalf("status=%s updated=%s events=%+v", status, updatedAt, *events)
		}
	})
	t.Run("session ends at next reservation", func(t *testing.T) {
		pool, service, events := newLifecycleService(t)
		at := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
		id := insertReservation(t, pool, "charger-1", at.Add(5*time.Second), at.Add(time.Minute))
		if err := service.Simulate(context.Background(), at, func(int) int { return 0 }); err != nil {
			t.Fatal(err)
		}
		var end time.Time
		if err := pool.QueryRow(context.Background(), "SELECT planned_end_at FROM charging_sessions WHERE charger_id='charger-1' AND reservation_id IS NULL").Scan(&end); err != nil {
			t.Fatal(err)
		}
		if !end.Equal(at.Add(5 * time.Second)) {
			t.Fatalf("planned end=%s", end)
		}
		if err := service.Reconcile(context.Background(), end); err != nil {
			t.Fatal(err)
		}
		if reservationStatus(t, pool, id) != "ACTIVE" || len(*events) != 1 {
			t.Fatalf("status=%s events=%+v", reservationStatus(t, pool, id), *events)
		}
	})
	t.Run("simulated cycle and pending maintenance", func(t *testing.T) {
		pool, service, events := newLifecycleService(t)
		at := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
		if err := service.Simulate(context.Background(), at, func(int) int { return 0 }); err != nil {
			t.Fatal(err)
		}
		if err := service.Reconcile(context.Background(), at.Add(12*time.Second)); err != nil {
			t.Fatal(err)
		}
		if status, _ := chargerStatus(t, pool, "charger-1"); status != "MAINTENANCE" || len(*events) != 2 {
			t.Fatalf("status=%s events=%+v", status, *events)
		}
		if err := service.Simulate(context.Background(), at.Add(13*time.Second), func(int) int { return 0 }); err != nil {
			t.Fatal(err)
		}
		if status, _ := chargerStatus(t, pool, "charger-1"); status != "AVAILABLE" {
			t.Fatalf("status=%s", status)
		}
		insertReservation(t, pool, "charger-3", at.Add(time.Hour), at.Add(2*time.Hour))
		if _, err := pool.Exec(context.Background(), "UPDATE chargers SET status='CHARGING' WHERE id IN ('charger-1','charger-2')"); err != nil {
			t.Fatal(err)
		}
		before := len(*events)
		if err := service.Simulate(context.Background(), at.Add(14*time.Second), func(int) int { t.Fatal("no eligible charger expected"); return 0 }); err != nil {
			t.Fatal(err)
		}
		if status, _ := chargerStatus(t, pool, "charger-3"); status != "MAINTENANCE" || len(*events) != before {
			t.Fatalf("status=%s events=%+v", status, *events)
		}
	})
}

func TestReservationLifecycle(t *testing.T) {
	pool, service, events := newLifecycleService(t)
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	first := insertReservation(t, pool, "charger-1", start, start.Add(30*time.Minute))
	second := insertReservation(t, pool, "charger-1", start.Add(30*time.Minute), start.Add(time.Hour))
	if err := service.Reconcile(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	if status := reservationStatus(t, pool, first); status != "ACTIVE" {
		t.Fatalf("first=%s", status)
	}
	if status, _ := chargerStatus(t, pool, "charger-1"); status != "CHARGING" {
		t.Fatalf("charger=%s", status)
	}
	if len(*events) != 1 {
		t.Fatalf("events=%+v", *events)
	}
	// A second service represents a process restart over the same database.
	restarted := charger.NewService(charger.NewStore(pool), nil, func(e charger.StatusEvent) { *events = append(*events, e) })
	if err := restarted.Reconcile(context.Background(), start.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM charging_sessions WHERE reservation_id=$1", first).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("duplicate sessions=%d", count)
	}
	if err := restarted.Reconcile(context.Background(), start.Add(30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if reservationStatus(t, pool, first) != "COMPLETED" || reservationStatus(t, pool, second) != "ACTIVE" {
		t.Fatal("back-to-back transition failed")
	}
	if status, _ := chargerStatus(t, pool, "charger-1"); status != "CHARGING" || len(*events) != 1 {
		t.Fatalf("status=%s events=%+v", status, *events)
	}
	if err := restarted.Reconcile(context.Background(), start.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if reservationStatus(t, pool, second) != "COMPLETED" {
		t.Fatal("second reservation not completed")
	}
	if status, _ := chargerStatus(t, pool, "charger-1"); status != "AVAILABLE" || len(*events) != 2 {
		t.Fatalf("status=%s events=%+v", status, *events)
	}
}

func TestWaitingExpirationAndRestart(t *testing.T) {
	pool, service, _ := newLifecycleService(t)
	start := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	if _, err := pool.Exec(context.Background(), "UPDATE chargers SET status='CHARGING' WHERE id='charger-2'"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), "INSERT INTO charging_sessions(charger_id,started_at,planned_end_at) VALUES('charger-2',$1,$2)", start.Add(-time.Minute), start.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	waiting := insertReservation(t, pool, "charger-2", start, start.Add(30*time.Minute))
	if err := service.Reconcile(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	if reservationStatus(t, pool, waiting) != "WAITING" {
		t.Fatal("reservation did not wait")
	}
	if err := service.Reconcile(context.Background(), start.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if reservationStatus(t, pool, waiting) != "ACTIVE" {
		t.Fatal("reservation did not start when occupancy ended")
	}
	var sessionStart time.Time
	if err := pool.QueryRow(context.Background(), "SELECT started_at FROM charging_sessions WHERE reservation_id=$1", waiting).Scan(&sessionStart); err != nil {
		t.Fatal(err)
	}
	if !sessionStart.Equal(start.Add(5 * time.Minute)) {
		t.Fatalf("late start=%s", sessionStart)
	}
	expired := insertReservation(t, pool, "charger-3", start, start.Add(10*time.Minute))
	if err := service.Reconcile(context.Background(), start); err != nil {
		t.Fatal(err)
	}
	if reservationStatus(t, pool, expired) != "WAITING" {
		t.Fatal("maintenance reservation did not wait")
	}
	restarted := charger.NewService(charger.NewStore(pool), nil, nil)
	if err := restarted.Reconcile(context.Background(), start.Add(11*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if reservationStatus(t, pool, expired) != "EXPIRED" {
		t.Fatal("elapsed reservation was replayed")
	}
	var count int
	if err := pool.QueryRow(context.Background(), "SELECT count(*) FROM charging_sessions WHERE reservation_id=$1", expired).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expired reservation sessions=%d", count)
	}
}

func TestConcurrentSimulationAndReservation(t *testing.T) {
	pool, service, _ := newLifecycleService(t)
	now := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	var userID string
	if err := pool.QueryRow(context.Background(), "SELECT id FROM users WHERE username='demo'").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	gate := make(chan struct{})
	var wg sync.WaitGroup
	var reserveErr, simulateErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-gate
		_, reserveErr = service.Reserve(context.Background(), userID, "charger-1", charger.ReserveInput{UserID: userID, StartTime: now.Add(time.Second), EndTime: now.Add(10 * time.Second)})
	}()
	go func() {
		defer wg.Done()
		<-gate
		simulateErr = service.Simulate(context.Background(), now, func(int) int { return 0 })
	}()
	close(gate)
	wg.Wait()
	if simulateErr != nil {
		t.Fatal(simulateErr)
	}
	if reserveErr != nil && reserveErr != charger.ErrOccupied {
		t.Fatalf("reserve error=%v", reserveErr)
	}
	var overlaps int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM reservations r JOIN charging_sessions s ON s.charger_id=r.charger_id
		WHERE s.reservation_id IS NULL AND s.ended_at IS NULL AND tstzrange(r.start_time,r.end_time,'[)') && tstzrange(s.started_at,s.planned_end_at,'[)')`).Scan(&overlaps); err != nil {
		t.Fatal(err)
	}
	if overlaps != 0 {
		t.Fatalf("simulator occupied a reserved interval: %d", overlaps)
	}
}
