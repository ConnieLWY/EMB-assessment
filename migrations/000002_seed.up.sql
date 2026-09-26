BEGIN;

-- Local demonstration accounts only. Both use the password documented in README.md.
INSERT INTO users (id, username, password_hash) VALUES
    ('11111111-1111-4111-8111-111111111111', 'alice', '$2a$12$rulRn4vyeGnqXoQApQ1ByerTX/ffr/ggu2oylSavGUA3slAo72.rW'),
    ('22222222-2222-4222-8222-222222222222', 'bob', '$2a$12$rulRn4vyeGnqXoQApQ1ByerTX/ffr/ggu2oylSavGUA3slAo72.rW');

INSERT INTO chargers (id, name, location, status) VALUES
    ('charger-1', 'Charger 1', 'Level 1, Bay A', 'AVAILABLE'),
    ('charger-2', 'Charger 2', 'Level 1, Bay B', 'AVAILABLE'),
    ('charger-3', 'Charger 3', 'Level 2, Bay A', 'MAINTENANCE');

COMMIT;
