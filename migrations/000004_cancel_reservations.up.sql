BEGIN;

ALTER TABLE reservations DROP CONSTRAINT reservations_status_check;
ALTER TABLE reservations ADD CONSTRAINT reservations_status_check
    CHECK (status IN ('SCHEDULED', 'WAITING', 'ACTIVE', 'COMPLETED', 'EXPIRED', 'CANCELLED'));

ALTER TABLE reservations DROP CONSTRAINT reservations_no_overlap;
ALTER TABLE reservations ADD CONSTRAINT reservations_no_overlap EXCLUDE USING gist (
    charger_id WITH =,
    tstzrange(start_time, end_time, '[)') WITH &&
) WHERE (status <> 'CANCELLED');

COMMIT;
