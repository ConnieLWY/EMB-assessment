# EV Charger Live Status & Reservation

A technical assessment project for a Go backend, React TypeScript frontend, PostgreSQL database, and Docker Compose setup.

## Design Documents

- [Database ERD](ERD.md)
- [API Contract](API.md)

## Run the Backend

Prerequisites: Docker with Docker Compose. Local Go development also requires Go 1.27 or later and the [golang-migrate CLI](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate) v4.20.1.

From the repository root:

```sh
docker compose up --build -d backend
curl http://localhost:8080/api/chargers
```

Compose waits for PostgreSQL to be healthy, applies SQL migrations through a one-shot `migrate` service, and starts the backend only after migration success. Database state persists in the `postgres_data` volume. `docker compose down` stops the services without removing that volume.

The default local ports are `8080` for the backend, `55432` for the development database, and `55433` for the separate test database. `.env.example` lists the configuration values; copy it to `.env` to override Compose defaults. The included passwords are public local demonstration values, not production credentials.

To run Go directly instead of the backend container:

```sh
docker compose up -d --wait db
docker compose run --rm migrate
cd backend
DATABASE_URL='postgres://ev_app:local-dev-only@localhost:55432/ev_charger?sslmode=disable' go run ./cmd/server
```

Use one backend at a time to avoid port conflicts. Set `HTTP_ADDR` to override its default `:8080` listener.

## Swagger UI

Open [Swagger UI](http://localhost:8080/swagger/) after starting the backend. Expand `GET /api/chargers`, select **Try it out**, then **Execute** to query the running database.

The [OpenAPI document](backend/internal/apidocs/openapi.json) is served at `/swagger/openapi.json`. Swagger UI assets are bundled with the Go backend, so browser access requires no external CDN or separate documentation container. API requests use the same origin as the documentation page.

Update the OpenAPI document alongside each new endpoint. [API.md](API.md) describes the complete application contract; Swagger UI exposes the available endpoints.

## Migrations and Seed Data

SQL migrations live in `migrations/` and use paired `.up.sql` and `.down.sql` files. `golang-migrate` records the applied version in `schema_migrations`; rerunning `up` does not duplicate seed data. The backend checks that migrations have completed and never changes the schema itself.

- `000001_schema`: creates users, login sessions, chargers, reservations, charging sessions, and their constraints.
- `000002_seed`: adds three chargers and two demo users. `charger-1` and `charger-2` start available; `charger-3` starts in maintenance.

| Username | User ID | Demo password |
| --- | --- | --- |
| `alice` | `11111111-1111-4111-8111-111111111111` | `DemoPass123!` |
| `bob` | `22222222-2222-4222-8222-222222222222` | `DemoPass123!` |

Passwords are stored as bcrypt hashes. Rolling back seed data is permitted only when it has no dependent application records; foreign keys prevent silently deleting reservations or sessions. Schema rollback removes application tables and is intended for disposable databases.

## Run Backend Tests

Install the prerequisites above, then run from the repository root:

```sh
docker compose --profile test up -d --wait test-db
cd backend
TEST_DATABASE_URL='postgres://ev_test:local-test-only@localhost:55433/ev_charger_test?sslmode=disable' go test -race ./... -count=1
```

The test fixture requires a database URL whose database name ends in `_test`. Each test creates its own randomly named database on that test server, runs the real migration CLI, and drops only that temporary database afterward. The test account therefore needs database creation permission. A missing test database configuration fails the test instead of silently skipping it.

Tests cover migration application and rollback, seed password verification, non-overlapping reservation intervals, adjacent slots, one unfinished charging session per charger, matching charger references, configuration validation, and HTTP charger listing with empty and database-error responses.

## Assessment Requirements

The required functionality includes database migrations and seed chargers, charger listing and reservation REST endpoints, a background status simulator running every 10–15 seconds, WebSocket status broadcasts, and a live dashboard with a reservation form.

The Go tests must include:

- `TestConcurrentReservations`: 10 concurrent HTTP requests for the same slot on `charger-1`, with exactly one `201 Created` and nine `409 Conflict` responses.
- `TestChargerStatusSimulation`: verify persisted status changes and emitted update events.

The deliverable will retain `backend/`, `frontend/`, `migrations/`, `docker-compose.yml`, and this README at the repository root.

## Assumptions & Trade-offs

The following assumptions and trade-offs define behavior left unspecified by the assessment.

### Charger and device simulation

- **One charger represents one independently reservable charging point.** Multi-connector stations are outside this assessment's scope. This keeps reservation ownership unambiguous.
- **Device states remain `AVAILABLE`, `CHARGING`, and `MAINTENANCE`.** Reservation information is tracked separately. A charger can be available now and reserved for a future period, so a fourth `RESERVED` device state would obscure the distinction.
- **No OCPP integration is included.** Database-backed simulated charging sessions represent device occupancy. This satisfies the mock-device scope but does not establish the physical state of real hardware.

### Identity — an agreed extension

- **Simple login and logout use seeded test accounts.** Registration, password recovery, and third-party authentication are excluded.
- **User IDs are UUIDs**, including the user foreign keys in login sessions and reservations. API requests send them as UUID strings. The assessment's `user-123` is treated as an illustrative value rather than a required literal. Charger IDs remain text so the required `charger-1` test fixture is preserved.
- **Users may reserve only for themselves.** The reservation request retains the assessment's `user_id`, `start_time`, and `end_time` fields. The backend checks that `user_id` matches the authenticated session rather than trusting the submitted identity.
- **The form retains a user ID input, prefilled with the signed-in user's ID.** This preserves the requested interface while preventing reservations on behalf of another account.
- Login adds setup to the concurrency test: all 10 requests must be authenticated and otherwise valid, so the nine failures represent reservation conflicts rather than authentication errors.

### Reservation times and availability

- **Only future reservations are accepted.** Start time must not precede server validation time, and end time must be strictly later than start time.
- **Times include an explicit time zone.** They are stored and returned in UTC and displayed in the user's local time zone, avoiding reliance on the server's local zone.
- **Intervals are half-open: `[start_time, end_time)`.** Reservations for 14:00–15:00 and 15:00–16:00 do not overlap. There is no early holding period or turnaround buffer.
- **A successful reservation guarantees exclusive scheduled use, not immediate charging.** Booking a future slot does not change the current device state.
- **Existing simulated occupancy also matters.** A new reservation that overlaps an unfinished session's planned occupancy is rejected. A currently charging device may accept a later, non-overlapping reservation. An overdue session blocks new reservations until reconciled because its release time is uncertain.
- **A charger currently in maintenance rejects all new reservations, including future ones.** This is conservative because the mock system has no maintenance completion estimate.

### Reservation lifecycle

- **Charging starts automatically at the reservation's start time when the charger is free.** No arrival, plug-in, or manual start step is modeled. This is a demonstration assumption, not a statement that booking a real charger starts power delivery.
- **The associated simulated session ends at the reservation's end time.** Back-to-back reservations transfer use without an intermediate available state; otherwise the charger returns to `AVAILABLE`.
- **Unexpected occupancy is never overwritten or forcibly stopped.** A reservation waits if another session remains active or the device is unexpectedly unavailable. If it becomes available before the end time, only the remaining booked time is used. A reservation that never starts expires at its original end time; it is not extended.
- **Lifecycle transitions are checked periodically.** They may occur shortly after a boundary rather than at its exact instant. Restart recovery reconciles persisted reservations and sessions instead of replaying charging periods that have already elapsed.

### Simulator coordination

- **The simulator respects reservations.** It does not overwrite active reserved sessions or start a simulated session that extends into the next reservation. Simulated sessions are released when due independently of random charger selection.
- **Chargers with pending reservations do not randomly enter maintenance.** This simplifies the demo and avoids deliberately invalidating accepted reservations; real equipment failures would require a broader disruption policy.
- **If no charger is eligible for a random update, that tick is skipped.** Reservation correctness takes priority over producing a status event on every tick.

### Responses and live updates

- **Successful reservations return `201`; overlap or maintenance conflicts return `409`.** Distinct error codes and messages explain the conflict. Malformed inputs return `400`, missing authentication `401`, identity mismatches `403`, and missing chargers `404`.
- **Device status events are sent only after a real state change has been committed.** Future booking alone does not emit a fictitious status change; its success is communicated by the reservation response. Reservation activation and completion broadcast when they change the device state.
- **The WebSocket event contract remains exactly the assessment's device-status format:**

```json
{
  "event": "CHARGER_STATUS_UPDATED",
  "data": {
    "charger_id": "charger-1",
    "status": "CHARGING",
    "updated_at": "2026-09-25T11:34:49Z"
  }
}
```

- **WebSocket delivery is live rather than a durable event history.** Clients re-fetch current state after reconnecting to recover missed updates. Reservation lifecycle information is separate from the device-status payload.

## Correctness Criteria for Implementation

- Database-enforced non-overlapping reservations per charger.
- At most one unfinished charging session per charger.
- Reservation creation, simulator updates, and lifecycle transitions coordinate through short database transactions; no database lock is held for the duration of a reservation.
- Occupancy checks and session creation are atomic, so two concurrent operations cannot both claim a free charger.
- Repeated worker execution does not create duplicate charging sessions.
- Tests cover the two required assessment scenarios as well as identity enforcement, adjacent reservations, session conflicts, lifecycle transitions, and restart reconciliation.
