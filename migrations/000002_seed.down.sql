BEGIN;
-- Foreign keys prevent removal of seed records that have acquired application data.
DELETE FROM chargers WHERE id IN ('charger-1', 'charger-2', 'charger-3');
DELETE FROM users WHERE id IN (
    '11111111-1111-4111-8111-111111111111',
    '22222222-2222-4222-8222-222222222222'
);
COMMIT;
