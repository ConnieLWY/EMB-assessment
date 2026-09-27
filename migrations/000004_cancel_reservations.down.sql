BEGIN;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM reservations WHERE status = 'CANCELLED') THEN
        RAISE EXCEPTION 'Cannot roll back cancellation support while cancelled reservations exist';
    END IF;
END;
$$;

ALTER TABLE reservations DROP CONSTRAINT reservations_no_overlap;
ALTER TABLE reservations ADD CONSTRAINT reservations_no_overlap EXCLUDE USING gist (
    charger_id WITH =,
    tstzrange(start_time, end_time, '[)') WITH &&
);

ALTER TABLE reservations DROP CONSTRAINT reservations_status_check;
ALTER TABLE reservations ADD CONSTRAINT reservations_status_check
    CHECK (status IN ('SCHEDULED', 'WAITING', 'ACTIVE', 'COMPLETED', 'EXPIRED'));

COMMIT;
