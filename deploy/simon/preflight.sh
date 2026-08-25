#!/usr/bin/env sh
set -eu

db_path=${1:-}
if [ -z "$db_path" ]; then
    echo "usage: $0 /absolute/path/to/open_ai_canvas.db" >&2
    exit 2
fi
if [ ! -f "$db_path" ]; then
    echo "database does not exist: $db_path" >&2
    exit 2
fi
settings_key=$(dirname "$db_path")/.settings-key
if [ ! -s "$settings_key" ]; then
    echo "settings key does not exist beside the database: $settings_key" >&2
    exit 1
fi
settings_key_bytes=$(wc -c < "$settings_key" | tr -d '[:space:]')
if [ "$settings_key_bytes" != "32" ]; then
    echo "settings key must be exactly 32 bytes, found: $settings_key_bytes" >&2
    exit 1
fi
if ! command -v sqlite3 >/dev/null 2>&1; then
    echo "sqlite3 is required for the read-only deployment preflight" >&2
    exit 2
fi

quick_check=$(sqlite3 -readonly "$db_path" "PRAGMA quick_check;")
if [ "$quick_check" != "ok" ]; then
    echo "SQLite quick_check failed: $quick_check" >&2
    exit 1
fi
foreign_key_violations=$(sqlite3 -readonly "$db_path" "PRAGMA foreign_key_check;" | wc -l | tr -d '[:space:]')
if [ "$foreign_key_violations" != "0" ]; then
    echo "SQLite foreign_key_check found $foreign_key_violations violation(s)" >&2
    exit 1
fi

single_admin_index=$(sqlite3 -readonly "$db_path" "SELECT COUNT(*) FROM sqlite_master WHERE type = 'index' AND name = 'idx_users_single_admin' AND sql IS NOT NULL;")
if [ "$single_admin_index" != "1" ]; then
    echo "database is missing the single-admin unique index" >&2
    exit 1
fi

admin_count=$(sqlite3 -readonly "$db_path" "SELECT COUNT(*) FROM users WHERE role = 'admin';")
if [ "$admin_count" != "1" ]; then
    echo "deployment requires exactly one admin, found: $admin_count" >&2
    exit 1
fi
active_admin_count=$(sqlite3 -readonly "$db_path" "SELECT COUNT(*) FROM users WHERE role = 'admin' AND status = 'active';")
if [ "$active_admin_count" != "1" ]; then
    echo "the only admin must be active" >&2
    exit 1
fi
local_admin_count=$(sqlite3 -readonly "$db_path" "SELECT COUNT(*) FROM users WHERE role = 'admin' AND status = 'active' AND length(password_hash) > 0;")
if [ "$local_admin_count" != "1" ]; then
    echo "the only admin must retain a local password for deployment login" >&2
    exit 1
fi

registration_enabled=$(sqlite3 -readonly "$db_path" "SELECT COALESCE((SELECT json_extract(value_json, '$.enabled') FROM system_settings WHERE key = 'registration'), 0);")
if [ "$registration_enabled" != "0" ]; then
    echo "public registration is enabled in the database override" >&2
    exit 1
fi

session_count=$(sqlite3 -readonly "$db_path" "SELECT COUNT(*) FROM auth_sessions;")
if [ "$session_count" != "0" ]; then
    echo "deployment snapshot still contains $session_count auth session(s); invalidate them before upload" >&2
    exit 1
fi

oauth_state_count=$(sqlite3 -readonly "$db_path" "SELECT COUNT(*) FROM o_auth_states;")
if [ "$oauth_state_count" != "0" ]; then
    echo "deployment snapshot still contains $oauth_state_count OAuth state(s); clear them before upload" >&2
    exit 1
fi

verification_code_count=$(sqlite3 -readonly "$db_path" "SELECT COUNT(*) FROM email_verification_codes;")
if [ "$verification_code_count" != "0" ]; then
    echo "deployment snapshot still contains $verification_code_count email verification code(s); clear them before upload" >&2
    exit 1
fi

echo "preflight ok: SQLite integrity and the 32-byte settings key are valid, registration is closed, temporary auth state is empty, and exactly one active admin is enforced"
