#!/usr/bin/env bash

set -Eeuo pipefail

fail() {
    printf '备份失败：%s\n' "$1" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "缺少命令：$1"
}

database_driver="${CANVAS_DATABASE_DRIVER:-}"
data_dir="${CANVAS_DATA_PATH:-}"
backup_root="${CANVAS_BACKUP_DIR:-}"
release_revision="${CANVAS_RELEASE_REVISION:-}"

[[ "$database_driver" == "sqlite" || "$database_driver" == "postgres" || "$database_driver" == "postgresql" ]] || fail "CANVAS_DATABASE_DRIVER 必须是 sqlite 或 postgres"
[[ -n "$data_dir" && -d "$data_dir" ]] || fail "CANVAS_DATA_PATH 必须指向现有应用数据目录"
[[ -n "$backup_root" ]] || fail "必须配置独立的 CANVAS_BACKUP_DIR"
[[ "$release_revision" =~ ^[0-9a-f]{40}$ ]] || fail "CANVAS_RELEASE_REVISION 必须是当前部署的 40 位小写提交哈希"
[[ "${CANVAS_BACKUP_QUIESCED:-}" == "yes" ]] || fail "先停止后端写入，再显式设置 CANVAS_BACKUP_QUIESCED=yes"

require_command realpath
require_command python3
require_command tar

data_dir="$(realpath "$data_dir")"
mkdir -p "$backup_root"
backup_root="$(realpath "$backup_root")"
case "${backup_root}/" in
    "${data_dir}/"*) fail "备份目录不能位于应用数据目录内" ;;
esac

settings_key="${data_dir}/.settings-key"
[[ -f "$settings_key" ]] || fail "数据目录缺少 .settings-key"
[[ "$(wc -c < "$settings_key" | tr -d '[:space:]')" == "32" ]] || fail ".settings-key 必须恰好为 32 字节"

# 备份恢复只接受目录和普通文件。源目录中的链接或设备文件会让归档语义不再
# 等同于当前数据目录，因此必须在 tar 读取它们之前拒绝。
if find "$data_dir" \( -type l -o \( ! -type f -a ! -type d \) \) -print -quit | grep -q .; then
    fail "应用数据包含符号链接或特殊文件，拒绝生成可能越过恢复目录的备份"
fi

umask 077
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
short_revision="${release_revision:0:12}"
staging="$(mktemp -d "${backup_root}/.kai-backup.XXXXXX")"
archive_tmp="$(mktemp "${backup_root}/.kai-archive.XXXXXX")"
archive_token="${archive_tmp##*.kai-archive.}"
archive_name="beidou-kai-${database_driver}-${timestamp}-${short_revision}-${archive_token}.tar.gz"
cleanup() {
    rm -rf -- "$staging"
    rm -f -- "$archive_tmp"
}
trap cleanup EXIT

payload="${staging}/payload"
mkdir -p "${payload}/data"

# 后端已经静默后再复制文件，保证媒体、加密密钥与数据库属于同一个业务时点。
tar -C "$data_dir" \
    --exclude='./backups' \
    --exclude='./open_ai_canvas.db' \
    --exclude='./open_ai_canvas.db-wal' \
    --exclude='./open_ai_canvas.db-shm' \
    -cf - . | tar -C "${payload}/data" -xf -

case "$database_driver" in
    sqlite)
        require_command sqlite3
        sqlite_source="${data_dir}/open_ai_canvas.db"
        sqlite_backup="${payload}/data/open_ai_canvas.db"
        [[ -f "$sqlite_source" ]] || fail "SQLite 数据库不存在：$sqlite_source"
        [[ "$sqlite_backup" != *"'"* ]] || fail "备份路径不能包含单引号"
        sqlite_backup_target="$sqlite_backup"
        if command -v cygpath >/dev/null 2>&1; then
            # Windows 合成测试使用原生 sqlite3；点命令中的路径不会经过 MSYS 参数转换。
            sqlite_backup_target="$(cygpath -w "$sqlite_backup")"
        fi
        sqlite3 "$sqlite_source" ".backup '${sqlite_backup_target}'"
        [[ "$(sqlite3 -readonly "$sqlite_backup" 'PRAGMA quick_check;')" == "ok" ]] || fail "SQLite 备份 quick_check 未通过"
        [[ -z "$(sqlite3 -readonly "$sqlite_backup" 'PRAGMA foreign_key_check;')" ]] || fail "SQLite 备份存在外键违规"
        ;;
    postgres|postgresql)
        if [[ -n "${CANVAS_POSTGRES_DUMP_PATH:-}" ]]; then
            require_command pg_restore
            [[ -f "$CANVAS_POSTGRES_DUMP_PATH" ]] || fail "CANVAS_POSTGRES_DUMP_PATH 指向的文件不存在"
            pg_restore --list "$CANVAS_POSTGRES_DUMP_PATH" >/dev/null
            cp -- "$CANVAS_POSTGRES_DUMP_PATH" "${payload}/postgres.dump"
        else
            require_command pg_dump
            [[ -n "${DATABASE_URL:-}" ]] || fail "PostgreSQL 备份必须配置 DATABASE_URL，或先提供 CANVAS_POSTGRES_DUMP_PATH"
            pg_dump --format=custom --no-owner --no-acl --file="${payload}/postgres.dump" "$DATABASE_URL"
        fi
        ;;
esac

cat > "${payload}/backup.env" <<EOF
FORMAT_VERSION=1
DATABASE_DRIVER=${database_driver}
RELEASE_REVISION=${release_revision}
CREATED_AT=${timestamp}
DATA_LAYOUT=data
EOF

(
    python3 - "$payload" <<'PY'
import hashlib
import pathlib
import sys

root = pathlib.Path(sys.argv[1]).resolve()
entries = []
for path in root.rglob("*"):
    if path.is_symlink():
        raise SystemExit(f"refusing symlink: {path}")
    if not path.is_file() or path.name == "SHA256SUMS":
        continue
    relative = path.relative_to(root).as_posix()
    if "\n" in relative or "\r" in relative:
        raise SystemExit(f"refusing control character in path: {relative!r}")
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    entries.append((relative, digest.hexdigest()))

manifest = root / "SHA256SUMS"
with manifest.open("x", encoding="utf-8", newline="\n") as stream:
    for relative, digest in sorted(entries):
        stream.write(f"{digest}  {relative}\n")
PY
)

tar -C "$staging" -czf "$archive_tmp" payload
mv "$archive_tmp" "${backup_root}/${archive_name}"
printf '备份完成：%s\n' "${backup_root}/${archive_name}"
