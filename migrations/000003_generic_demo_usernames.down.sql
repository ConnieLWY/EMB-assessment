BEGIN;
UPDATE users SET username = 'alice'
WHERE id = '11111111-1111-4111-8111-111111111111' AND username = 'demo';
UPDATE users SET username = 'bob'
WHERE id = '22222222-2222-4222-8222-222222222222' AND username = 'demo2';
COMMIT;
