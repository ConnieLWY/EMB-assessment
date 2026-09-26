BEGIN;
DROP TABLE charging_sessions;
DROP TABLE reservations;
DROP TABLE chargers;
DROP TABLE auth_sessions;
DROP TABLE users;
DROP EXTENSION btree_gist;
COMMIT;
