BEGIN;

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE TABLE users (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    username text NOT NULL UNIQUE CHECK (length(btrim(username)) > 0),
    password_hash text NOT NULL CHECK (length(password_hash) > 0),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE auth_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    token_hash text NOT NULL UNIQUE,
    expires_at timestamptz NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT auth_session_time_valid CHECK (expires_at > created_at AND isfinite(expires_at))
);
CREATE INDEX auth_sessions_user_idx ON auth_sessions(user_id);
CREATE INDEX auth_sessions_expiry_idx ON auth_sessions(expires_at);

CREATE TABLE chargers (
    id text PRIMARY KEY CHECK (length(btrim(id)) > 0),
    name text NOT NULL CHECK (length(btrim(name)) > 0),
    location text NOT NULL CHECK (length(btrim(location)) > 0),
    status text NOT NULL DEFAULT 'AVAILABLE'
        CHECK (status IN ('AVAILABLE', 'CHARGING', 'MAINTENANCE')),
    updated_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE reservations (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id uuid NOT NULL REFERENCES users(id),
    charger_id text NOT NULL REFERENCES chargers(id),
    start_time timestamptz NOT NULL,
    end_time timestamptz NOT NULL,
    status text NOT NULL DEFAULT 'SCHEDULED'
        CHECK (status IN ('SCHEDULED', 'WAITING', 'ACTIVE', 'COMPLETED', 'EXPIRED')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT reservation_time_valid CHECK (
        end_time > start_time AND isfinite(start_time) AND isfinite(end_time)
    ),
    CONSTRAINT reservation_charger_key UNIQUE (id, charger_id),
    CONSTRAINT reservations_no_overlap EXCLUDE USING gist (
        charger_id WITH =,
        tstzrange(start_time, end_time, '[)') WITH &&
    )
);
CREATE INDEX reservations_user_time_idx ON reservations(user_id, start_time DESC, id);
CREATE INDEX reservations_pending_idx ON reservations(start_time)
    WHERE status IN ('SCHEDULED', 'WAITING', 'ACTIVE');

CREATE TABLE charging_sessions (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    charger_id text NOT NULL REFERENCES chargers(id),
    reservation_id uuid UNIQUE,
    started_at timestamptz NOT NULL,
    planned_end_at timestamptz NOT NULL,
    ended_at timestamptz,
    CONSTRAINT session_reservation_charger_fk FOREIGN KEY (reservation_id, charger_id)
        REFERENCES reservations(id, charger_id),
    CONSTRAINT charging_session_time_valid CHECK (
        planned_end_at > started_at AND isfinite(started_at) AND isfinite(planned_end_at)
        AND (ended_at IS NULL OR (ended_at >= started_at AND isfinite(ended_at)))
    )
);
CREATE UNIQUE INDEX charging_sessions_one_unfinished_per_charger
    ON charging_sessions(charger_id) WHERE ended_at IS NULL;
CREATE INDEX charging_sessions_due_idx ON charging_sessions(planned_end_at)
    WHERE ended_at IS NULL;

COMMIT;
