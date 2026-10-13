-- Run only against the offline deployment copy, never against the active local database.
PRAGMA foreign_keys = ON;
BEGIN IMMEDIATE;

DELETE FROM auth_sessions;
DELETE FROM o_auth_states;
DELETE FROM email_verification_codes;

COMMIT;
