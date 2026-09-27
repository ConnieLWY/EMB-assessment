# Database Design

User primary keys and all foreign keys referencing users use UUIDs. The API represents `user_id` as a UUID string. Charger IDs remain text to preserve the assessment's required `charger-1` fixture.

```mermaid
erDiagram
    USERS ||--o{ AUTH_SESSIONS : "authenticates through"
    USERS ||--o{ RESERVATIONS : "creates"
    CHARGERS ||--o{ RESERVATIONS : "accepts"
    CHARGERS ||--o{ CHARGING_SESSIONS : "hosts"
    RESERVATIONS o|--o| CHARGING_SESSIONS : "is fulfilled by"

    USERS {
        uuid id PK
        text username UK
        text password_hash
        timestamptz created_at
    }
    AUTH_SESSIONS {
        uuid id PK
        uuid user_id FK
        text token_hash UK
        timestamptz expires_at
        timestamptz created_at
    }
    CHARGERS {
        text id PK
        text name
        text location
        text status "AVAILABLE / CHARGING / MAINTENANCE"
        timestamptz updated_at
        timestamptz created_at
    }
    RESERVATIONS {
        uuid id PK
        uuid user_id FK
        text charger_id FK
        timestamptz start_time
        timestamptz end_time
        text status "SCHEDULED / WAITING / ACTIVE / COMPLETED / EXPIRED / CANCELLED"
        timestamptz created_at
        timestamptz updated_at
    }
    CHARGING_SESSIONS {
        uuid id PK
        text charger_id FK
        uuid reservation_id FK,UK "Null for a random simulated session"
        timestamptz started_at
        timestamptz planned_end_at
        timestamptz ended_at "Null while the session is unfinished"
    }
```

## Constraints and Relationships

- Non-cancelled reservation intervals `[start_time, end_time)` must not overlap for the same charger. Cancelled rows stay in history without holding their former time slots. `end_time` must be later than `start_time`.
- Each charger may have at most one unfinished charging session.
- Each reservation may have at most one charging session. A null `reservation_id` indicates random simulated use; otherwise, the user is identified through the linked reservation.
- A session linked to a reservation must belong to the same charger as that reservation.
- A session's planned end time must be later than its start time. Its actual end time, when present, must not precede its start time.
- Charger state changes and session start or end updates must occur in the same transaction.
- Both reservation creation and simulated session creation lock the corresponding charger before checking occupancy and writing changes.
