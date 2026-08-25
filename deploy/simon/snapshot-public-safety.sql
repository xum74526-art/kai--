-- Run only against the offline deployment copy, never against the active local database.
PRAGMA foreign_keys = ON;
BEGIN IMMEDIATE;

DELETE FROM auth_sessions;
DELETE FROM o_auth_states;
DELETE FROM email_verification_codes;

INSERT INTO system_settings (key, value_json, updated_by, created_at, updated_at)
SELECT 'registration', '{"enabled":false}', id, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
FROM users
WHERE role = 'admin'
LIMIT 1
ON CONFLICT(key) DO UPDATE SET
    value_json = excluded.value_json,
    updated_by = excluded.updated_by,
    updated_at = CURRENT_TIMESTAMP;

COMMIT;
