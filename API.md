# API Contract

## Conventions

- REST base path: `/api`. Direct local backend URL: `http://localhost:8080`.
- JSON request bodies use `Content-Type: application/json`.
- Responses use JSON except for `204 No Content` and WebSocket upgrades.
- User, reservation, and session identifiers use UUIDs. Charger identifiers use text, including `charger-1`.
- Input timestamps use RFC 3339 with an explicit offset. Response timestamps use UTC with a `Z` suffix.
- Reservation intervals are half-open: `[start_time, end_time)`.
- Example dates are illustrative; clients and tests must submit future reservation times.
- Private responses use `Cache-Control: no-store`. Password hashes, session tokens, and other users' reservation details are never returned.

## Endpoints

| Method | Path | Authentication | Success |
| --- | --- | --- | --- |
| POST | `/api/auth/login` | Public | `200` |
| POST | `/api/auth/logout` | Optional; invalidates the current session if present | `204` |
| GET | `/api/auth/me` | Required | `200` |
| GET | `/api/chargers` | Public | `200` |
| POST | `/api/chargers/{id}/reserve` | Required | `201` |
| GET | `/api/reservations` | Required; current user's reservations only | `200` |
| GET | `/api/ws` | Public; device status only | `101` |

## Authentication

Login uses a username and password from a seeded account. Successful login issues an opaque random session token in an `ev_session` cookie; only its hash is stored in `auth_sessions`.

The cookie uses `HttpOnly`, `SameSite=Lax`, and `Path=/`, with no `Domain` attribute. Its lifetime is 24 hours, matching the server-side session expiration. HTTPS deployments also use `Secure`; local HTTP development omits that attribute. A successful login creates a fresh token and invalidates any previous session supplied by that browser.

Browser mutations, including login and logout, require an `Origin` matching the configured frontend origin. Missing or untrusted origins return `403 ORIGIN_NOT_ALLOWED`. CLI clients and integration tests send the configured origin explicitly. The application uses the same host consistently in local development; `localhost` and `127.0.0.1` are not interchangeable for session cookies.

For direct cross-origin development requests, the frontend includes credentials and the backend allows only the configured frontend origin, with credentialed CORS and preflight support. Same-origin proxy deployments use relative `/api` URLs.

### POST /api/auth/login

Request:

```json
{
  "username": "alice",
  "password": "example-password"
}
```

Response: `200 OK`, with the session cookie set.

```json
{
  "user": {
    "id": "11111111-1111-4111-8111-111111111111",
    "username": "alice"
  }
}
```

Missing fields return `400 VALIDATION_ERROR`. Unknown usernames and incorrect passwords both return `401 INVALID_CREDENTIALS` with the same generic message. Login attempts are rate-limited; exceeded limits return `429 RATE_LIMITED` with `Retry-After`.

### POST /api/auth/logout

No request body. Invalidate the current server-side session and clear the cookie. Return `204 No Content`, including when the cookie is missing, expired, or already invalidated. Origin validation still applies.

### GET /api/auth/me

Return `200 OK` with the same user response shape as login. Missing, invalid, or expired sessions return `401 UNAUTHENTICATED`.

## Chargers

### GET /api/chargers

Return all seed chargers, ordered by ID. An empty collection returns an empty array.

```json
{
  "chargers": [
    {
      "id": "charger-1",
      "name": "Charger 1",
      "location": "Level 1, Bay A",
      "status": "AVAILABLE",
      "updated_at": "2026-10-01T10:00:00Z"
    }
  ]
}
```

Status is one of `AVAILABLE`, `CHARGING`, or `MAINTENANCE`. It describes current device state, not availability for every future time slot. No occupant identity is exposed.

## Reservations

### POST /api/chargers/{id}/reserve

Request:

```json
{
  "user_id": "11111111-1111-4111-8111-111111111111",
  "start_time": "2026-10-01T14:00:00Z",
  "end_time": "2026-10-01T15:00:00Z"
}
```

Validation and behavior:

1. Require an authenticated session and a valid request origin.
2. Require a UUID `user_id` matching the authenticated user. The field remains part of the payload to preserve the assessment contract.
3. Require valid timestamps, a start time not earlier than server validation time, and an end time later than the start.
4. Within a transaction, lock the charger and check its existence, maintenance state, unfinished sessions, and reservation conflicts.
5. Enforce non-overlapping reservations in the database and commit the new reservation before returning success.

Response: `201 Created`.

```json
{
  "reservation": {
    "id": "22222222-2222-4222-8222-222222222222",
    "user_id": "11111111-1111-4111-8111-111111111111",
    "charger_id": "charger-1",
    "start_time": "2026-10-01T14:00:00Z",
    "end_time": "2026-10-01T15:00:00Z",
    "status": "SCHEDULED",
    "created_at": "2026-10-01T10:00:00Z",
    "updated_at": "2026-10-01T10:00:00Z"
  }
}
```

Future booking does not immediately change charger status. No device-status event is emitted unless the device state actually changes.

| Condition | HTTP status | Error code |
| --- | --- | --- |
| Invalid fields, UUID, or time interval | `400` | `VALIDATION_ERROR` |
| Missing or expired login | `401` | `UNAUTHENTICATED` |
| User ID differs from the authenticated user | `403` | `USER_ID_MISMATCH` |
| Charger does not exist | `404` | `CHARGER_NOT_FOUND` |
| Overlapping reservation | `409` | `RESERVATION_CONFLICT` |
| Overlapping unfinished simulated session, or overdue occupancy | `409` | `CHARGER_OCCUPIED` |
| Charger is currently in maintenance | `409` | `CHARGER_IN_MAINTENANCE` |

A repeated successful request is not treated as a second success: the duplicate slot conflicts and returns `409`. If a response is lost, the client refreshes its reservation list before deciding whether to retry.

### GET /api/reservations

Return only the authenticated user's reservations, ordered by `start_time` descending and then `id`. No user selector is accepted. The assessment uses an unpaginated collection because its seeded demonstration dataset is small.

```json
{
  "reservations": [
    {
      "id": "22222222-2222-4222-8222-222222222222",
      "user_id": "11111111-1111-4111-8111-111111111111",
      "charger_id": "charger-1",
      "start_time": "2026-10-01T14:00:00Z",
      "end_time": "2026-10-01T15:00:00Z",
      "status": "SCHEDULED",
      "created_at": "2026-10-01T10:00:00Z",
      "updated_at": "2026-10-01T10:00:00Z"
    }
  ]
}
```

| Reservation status | Meaning |
| --- | --- |
| `SCHEDULED` | Accepted and awaiting activation |
| `WAITING` | The booked period has started, but the device remains occupied or unavailable |
| `ACTIVE` | The reservation's simulated charging session has started |
| `COMPLETED` | The reservation's charging session has ended |
| `EXPIRED` | The booked period ended without the reservation starting |

Lifecycle updates are periodic, so status may briefly lag a time boundary. The frontend refreshes this collection after booking, on login, on reconnect, and every five seconds while the authenticated page is visible. Polling is stopped on logout; a `401` clears private client state. Device events alone cannot communicate all reservation changes, including back-to-back reservations that keep a charger in `CHARGING`.

## WebSocket

### GET /api/ws

Direct local URL: `ws://localhost:8080/api/ws`. HTTPS deployments use `wss`. Browser handshakes must use the configured frontend origin. Non-browser test clients supply that origin as well.

The connection carries public device-status updates only. The server broadcasts to all connected clients after a status change is committed:

```json
{
  "event": "CHARGER_STATUS_UPDATED",
  "data": {
    "charger_id": "charger-1",
    "status": "CHARGING",
    "updated_at": "2026-10-01T14:00:00Z"
  }
}
```

The client does not send reservation commands over this connection. Updates originate from the simulator or reservation lifecycle processing. Writes are serialized per connection, and slow or disconnected clients must not block other clients or database operations.

There is no event replay or delivery acknowledgement. On initial connection and reconnection, the frontend buffers arriving events while fetching the charger snapshot, then applies only events newer than each charger's `updated_at`. Updates for each charger must carry strictly increasing timestamps and be published in commit order. The client ignores older or duplicate events and reconnects with capped exponential backoff after a disconnect. A failed snapshot request is retried rather than treated as an empty dashboard.

## Errors

All REST errors use this envelope:

```json
{
  "error": {
    "code": "RESERVATION_CONFLICT",
    "message": "This charger is already reserved for the selected time slot."
  }
}
```

Messages are English and suitable for display. Codes are stable identifiers for frontend behavior. Internal database errors, credentials, and stack traces are never exposed. Unexpected failures return `500 INTERNAL_ERROR` with a generic message. JSON endpoints reject unsupported content types with `415 UNSUPPORTED_MEDIA_TYPE` and oversized request bodies with `413 PAYLOAD_TOO_LARGE`.

## Assessment Verification

- `TestConcurrentReservations` uses 10 authenticated requests with the same future slot on `charger-1`. The charger starts available with no conflicting simulated session. Simulator activity is disabled for this isolated test. The assertion is exactly one `201` and nine `409 RESERVATION_CONFLICT` responses.
- `TestChargerStatusSimulation` invokes a controllable simulation step and checks the persisted charger state and emitted event without waiting for a real 10–15 second interval.
- Integration checks cover broadcast delivery, identity enforcement, adjacent time slots, competing simulation and reservation writes, and the reservation lifecycle.
