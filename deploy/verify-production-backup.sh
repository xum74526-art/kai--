#!/usr/bin/env bash

set -Eeuo pipefail

fail() {
    printf '恢复校验失败：%s\n' "$1" >&2
    exit 1
}

require_command() {
    command -v "$1" >/dev/null 2>&1 || fail "缺少命令：$1"
}

archive="${1:-}"
[[ -n "$archive" && -f "$archive" ]] || fail "用法：$0 /absolute/path/to/kaiying-backup.tar.gz"
require_command python3

umask 077
restore_root="$(mktemp -d "${TMPDIR:-/tmp}/kaiying-restore-verify.XXXXXX")"
cleanup() {
    if [[ "${KEEP_RESTORE_VERIFY_DIR:-}" == "yes" ]]; then
        printf '已保留隔离校验目录：%s\n' "$restore_root"
    else
        rm -rf -- "$restore_root"
    fi
}
trap cleanup EXIT

# 先检查全部成员，再逐个复制普通文件；不让 tar 在链接或越界路径已生效后才报错。
python3 - "$archive" "$restore_root" <<'PY'
import pathlib
import shutil
import sys
import tarfile

archive = pathlib.Path(sys.argv[1])
root = pathlib.Path(sys.argv[2]).resolve()
seen = set()
with tarfile.open(archive, "r:gz") as bundle:
    members = bundle.getmembers()
    if not members:
        raise SystemExit("backup archive is empty")
    for member in members:
        pure = pathlib.PurePosixPath(member.name)
        if pure.is_absolute() or ".." in pure.parts or not pure.parts or pure.parts[0] != "payload":
            raise SystemExit(f"unsafe archive path: {member.name!r}")
        if not (member.isdir() or member.isfile()):
            raise SystemExit(f"links and special files are forbidden: {member.name!r}")
        normalized = pure.as_posix()
        if normalized in seen:
            raise SystemExit(f"duplicate archive path: {member.name!r}")
        seen.add(normalized)

    for member in members:
        pure = pathlib.PurePosixPath(member.name)
        target = root.joinpath(*pure.parts)
        if member.isdir():
            target.mkdir(parents=True, exist_ok=True)
            continue
        target.parent.mkdir(parents=True, exist_ok=True)
        source = bundle.extractfile(member)
        if source is None:
            raise SystemExit(f"cannot read archive member: {member.name!r}")
        with source, target.open("xb") as output:
            shutil.copyfileobj(source, output)
PY
[[ -d "${restore_root}/payload" ]] || fail "备份包缺少 payload 目录"

payload="${restore_root}/payload"
metadata="${payload}/backup.env"
[[ -f "$metadata" && -f "${payload}/SHA256SUMS" ]] || fail "备份包缺少元数据或校验和"
python3 - "$payload" <<'PY'
import hashlib
import pathlib
import re
import sys

root = pathlib.Path(sys.argv[1]).resolve()
manifest = root / "SHA256SUMS"
expected = {}
for number, line in enumerate(manifest.read_text(encoding="utf-8").splitlines(), 1):
    match = re.fullmatch(r"([0-9a-f]{64})  (.+)", line)
    if not match:
        raise SystemExit(f"invalid checksum line {number}")
    digest, raw_path = match.groups()
    pure = pathlib.PurePosixPath(raw_path)
    if pure.is_absolute() or ".." in pure.parts or pure.as_posix() != raw_path or raw_path == "SHA256SUMS":
        raise SystemExit(f"unsafe checksum path on line {number}: {raw_path!r}")
    if raw_path in expected:
        raise SystemExit(f"duplicate checksum path: {raw_path!r}")
    expected[raw_path] = digest

actual = {}
for path in root.rglob("*"):
    if path.is_symlink():
        raise SystemExit(f"unexpected symlink: {path}")
    if path.is_file() and path != manifest:
        actual[path.relative_to(root).as_posix()] = path

if set(expected) != set(actual):
    missing = sorted(set(expected) - set(actual))
    extra = sorted(set(actual) - set(expected))
    raise SystemExit(f"checksum coverage mismatch: missing={missing}, extra={extra}")

for relative, path in actual.items():
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(chunk)
    if digest.hexdigest() != expected[relative]:
        raise SystemExit(f"checksum mismatch: {relative}")
    print(f"{relative}: OK")
PY

metadata_value() {
    local key="$1"
    sed -n "s/^${key}=//p" "$metadata" | tail -n 1
}

[[ "$(metadata_value FORMAT_VERSION)" == "1" ]] || fail "不支持的备份格式版本"
driver="$(metadata_value DATABASE_DRIVER)"
revision="$(metadata_value RELEASE_REVISION)"
[[ "$revision" =~ ^[0-9a-f]{40}$ ]] || fail "备份包缺少有效的发布 revision"
settings_key="${payload}/data/.settings-key"
[[ -f "$settings_key" ]] || fail "备份包缺少 .settings-key"
[[ "$(wc -c < "$settings_key" | tr -d '[:space:]')" == "32" ]] || fail ".settings-key 不是 32 字节"

case "$driver" in
    sqlite)
        require_command sqlite3
        database="${payload}/data/open_ai_canvas.db"
        [[ -f "$database" ]] || fail "备份包缺少 SQLite 数据库"
        [[ "$(sqlite3 -readonly "$database" 'PRAGMA quick_check;')" == "ok" ]] || fail "SQLite quick_check 未通过"
        [[ -z "$(sqlite3 -readonly "$database" 'PRAGMA foreign_key_check;')" ]] || fail "SQLite 存在外键违规"
        ;;
    postgres|postgresql)
        require_command pg_restore
        dump="${payload}/postgres.dump"
        [[ -f "$dump" ]] || fail "备份包缺少 PostgreSQL dump"
        pg_restore --list "$dump" >/dev/null
        if [[ -n "${POSTGRES_VERIFY_URL:-}" ]]; then
            require_command psql
            [[ "${POSTGRES_RESTORE_VERIFY_CONFIRM:-}" == "restore-isolated-database" ]] || fail "完整 PostgreSQL 恢复只允许写入新建的空校验库；请设置 POSTGRES_RESTORE_VERIFY_CONFIRM=restore-isolated-database"
            [[ "${POSTGRES_VERIFY_URL}" != "${DATABASE_URL:-}" ]] || fail "隔离校验库不能与生产 DATABASE_URL 相同"
            verify_database="$(psql "$POSTGRES_VERIFY_URL" -XAtqc 'SELECT current_database()')"
            [[ "$verify_database" == *_restore_verify ]] || fail "校验数据库名必须以 _restore_verify 结尾"
            existing_tables="$(psql "$POSTGRES_VERIFY_URL" -XAtqc "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'")"
            [[ "$existing_tables" == "0" ]] || fail "隔离 PostgreSQL 校验库必须是新建的空库；脚本不会清空已有数据库"
            pg_restore --exit-on-error --no-owner --no-acl --dbname="$POSTGRES_VERIFY_URL" "$dump"
            restored_tables="$(psql "$POSTGRES_VERIFY_URL" -XAtqc "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='public'")"
            [[ "$restored_tables" =~ ^[1-9][0-9]*$ ]] || fail "隔离 PostgreSQL 恢复后没有业务表"
        fi
        ;;
    *) fail "未知数据库类型：$driver" ;;
esac

printf '恢复校验通过：driver=%s revision=%s\n' "$driver" "$revision"
