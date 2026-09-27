package database_test

import (
	"context"
	"errors"
	"testing"

	"ev-charger-assessment/backend/internal/testutil"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

const userID = "11111111-1111-4111-8111-111111111111"

func TestMigrations(t *testing.T) {
	pool, dsn := testutil.NewDatabase(t, false)
	testutil.Migrate(t, dsn, "up")
	testutil.Migrate(t, dsn, "up")
	ctx := context.Background()
	var chargers, users, version int
	var dirty bool
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM chargers").Scan(&chargers); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&users); err != nil {
		t.Fatal(err)
	}
	if chargers != 3 || users != 2 {
		t.Fatalf("seed counts: chargers=%d users=%d", chargers, users)
	}
	if err := pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations").Scan(&version, &dirty); err != nil {
		t.Fatal(err)
	}
	if version != 3 || dirty {
		t.Fatalf("migration version=%d dirty=%v", version, dirty)
	}
	for _, username := range []string{"demo", "demo2"} {
		var hash string
		if err := pool.QueryRow(ctx, "SELECT password_hash FROM users WHERE username=$1", username).Scan(&hash); err != nil {
			t.Fatal(err)
		}
		if err := bcrypt.CompareHashAndPassword([]byte(hash), []byte("DemoPass123!")); err != nil {
			t.Fatalf("invalid demo password for %s: %v", username, err)
		}
	}
	testutil.Migrate(t, dsn, "down", "2")
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM chargers").Scan(&chargers); err != nil {
		t.Fatal(err)
	}
	if chargers != 0 {
		t.Fatalf("seed rollback left %d chargers", chargers)
	}
	testutil.Migrate(t, dsn, "down", "1")
	var table *string
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.chargers')::text").Scan(&table); err != nil {
		t.Fatal(err)
	}
	if table != nil {
		t.Fatal("schema rollback left chargers table")
	}
	testutil.Migrate(t, dsn, "up")
}

func TestSchemaConstraints(t *testing.T) {
	pool, _ := testutil.NewDatabase(t, true)
	ctx := context.Background()
	reservation := func(charger, start, end string) (string, error) {
		var id string
		err := pool.QueryRow(ctx, `INSERT INTO reservations(user_id, charger_id, start_time, end_time)
			VALUES ($1, $2, $3::timestamptz, $4::timestamptz) RETURNING id::text`, userID, charger, start, end).Scan(&id)
		return id, err
	}
	id, err := reservation("charger-1", "2030-01-01T14:00:00Z", "2030-01-01T15:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	_, err = reservation("charger-1", "2030-01-01T14:30:00Z", "2030-01-01T15:30:00Z")
	assertSQLState(t, err, "23P01")
	if _, err := reservation("charger-1", "2030-01-01T15:00:00Z", "2030-01-01T16:00:00Z"); err != nil {
		t.Fatalf("adjacent reservation: %v", err)
	}
	if _, err := reservation("charger-2", "2030-01-01T14:00:00Z", "2030-01-01T15:00:00Z"); err != nil {
		t.Fatalf("different charger: %v", err)
	}
	_, err = reservation("charger-3", "2030-01-01T14:00:00Z", "2030-01-01T14:00:00Z")
	assertSQLState(t, err, "23514")
	_, err = pool.Exec(ctx, `INSERT INTO charging_sessions(charger_id, reservation_id, started_at, planned_end_at)
		VALUES ('charger-2', $1, '2030-01-01T14:00:00Z', '2030-01-01T15:00:00Z')`, id)
	assertSQLState(t, err, "23503")
	execSQL(t, pool, `INSERT INTO charging_sessions(charger_id, reservation_id, started_at, planned_end_at)
		VALUES ('charger-1', $1, '2030-01-01T14:00:00Z', '2030-01-01T15:00:00Z')`, id)
	_, err = pool.Exec(ctx, `INSERT INTO charging_sessions(charger_id, started_at, planned_end_at)
		VALUES ('charger-1', '2030-01-01T14:30:00Z', '2030-01-01T15:00:00Z')`)
	assertSQLState(t, err, "23505")
	execSQL(t, pool, "UPDATE charging_sessions SET ended_at='2030-01-01T15:00:00Z' WHERE charger_id='charger-1'")
	_, err = pool.Exec(ctx, `INSERT INTO charging_sessions(charger_id, reservation_id, started_at, planned_end_at)
		VALUES ('charger-1', $1, '2030-01-01T15:00:00Z', '2030-01-01T16:00:00Z')`, id)
	assertSQLState(t, err, "23505")
	execSQL(t, pool, `INSERT INTO charging_sessions(charger_id, started_at, planned_end_at)
		VALUES ('charger-1', '2030-01-01T15:00:00Z', '2030-01-01T16:00:00Z')`)
	_, err = pool.Exec(ctx, "UPDATE chargers SET status='RESERVED' WHERE id='charger-1'")
	assertSQLState(t, err, "23514")
	_, err = pool.Exec(ctx, "UPDATE charging_sessions SET ended_at=started_at - interval '1 second'")
	assertSQLState(t, err, "23514")
}

func assertSQLState(t *testing.T, err error, want string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != want {
		t.Fatalf("want SQLSTATE %s, got %v", want, err)
	}
}

func execSQL(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatal(err)
	}
}
