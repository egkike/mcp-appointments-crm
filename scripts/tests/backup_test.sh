#!/bin/bash
# Unit tests for scripts/backup.sh (shunit2).
# Covers REQ-BKP-001..004.

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BACKUP_SCRIPT="$SCRIPT_DIR/../backup.sh"

# Crea un PATH mínimo con bash y gzip pero sin sqlite3.
_minimal_path() {
  local d="$1"
  ln -s "$(command -v bash)" "$d/bash"
  ln -s "$(command -v gzip)" "$d/gzip"
  printf '%s' "$d"
}

test_prereqs_missing_fails() {
  local tmp db out rc
  tmp=$(mktemp -d)
  db="$tmp/reservas.db"
  sqlite3 "$db" "CREATE TABLE t(id INTEGER); INSERT INTO t VALUES (1);"
  out=$(PATH="$(_minimal_path "$tmp")" bash "$BACKUP_SCRIPT" "$db" 2>&1) || rc=$?
  assertTrue 'exit non-zero when sqlite3 is missing' "[ \${rc:-0} -ne 0 ]"
  assertTrue 'error names sqlite3' "printf '%s' \"$out\" | grep -q 'sqlite3'"
  assertFalse 'backups dir is not created' "[ -d \"$tmp/backups\" ]"
  assertTrue 'db is untouched' "[ -f \"$db\" ]"
  rm -rf "$tmp"
}

test_happy_path() {
  local tmp db backup out restored restored_dir
  tmp=$(mktemp -d)
  db="$tmp/reservas.db"
  sqlite3 "$db" "CREATE TABLE t(id INTEGER PRIMARY KEY, name TEXT); INSERT INTO t VALUES (1,'reserva-1');"
  out=$(bash "$BACKUP_SCRIPT" "$db" 2>&1)
  assertEquals 'happy path exit 0' 0 $?
  backup="$tmp/backups/reservas-$(date +%Y%m%d).db.gz"
  assertTrue 'backup file exists' "[ -f \"$backup\" ]"
  assertTrue 'output mentions Backup:' "printf '%s' \"$out\" | grep -q 'Backup:'"
  restored_dir=$(mktemp -d)
  restored="$restored_dir/restored.db"
  gunzip -c "$backup" > "$restored"
  assertEquals 'integrity check is ok' 'ok' "$(sqlite3 "$restored" 'PRAGMA integrity_check;')"
  assertEquals 'data is preserved' 'reserva-1' "$(sqlite3 "$restored" 'SELECT name FROM t WHERE id=1;')"
  rm -rf "$tmp" "$restored_dir"
}

test_db_not_found() {
  local tmp out rc
  tmp=$(mktemp -d)
  out=$(bash "$BACKUP_SCRIPT" "$tmp/missing.db" 2>&1) || rc=$?
  assertTrue 'exit non-zero when db is missing' "[ \${rc:-0} -ne 0 ]"
  assertTrue 'error names the missing path' "printf '%s' \"$out\" | grep -q 'missing.db'"
  assertFalse 'backups dir is not created' "[ -d \"$tmp/backups\" ]"
  rm -rf "$tmp"
}

test_rerun_same_day() {
  local tmp db backup out restored restored_dir
  tmp=$(mktemp -d)
  db="$tmp/reservas.db"
  sqlite3 "$db" "CREATE TABLE t(id INTEGER PRIMARY KEY, name TEXT); INSERT INTO t VALUES (1,'primero');"
  bash "$BACKUP_SCRIPT" "$db" >/dev/null 2>&1
  assertEquals 'first backup exit 0' 0 $?
  backup="$tmp/backups/reservas-$(date +%Y%m%d).db.gz"
  assertTrue 'backup file exists after first run' "[ -f \"$backup\" ]"
  sqlite3 "$db" "INSERT INTO t VALUES (2,'segundo');"
  out=$(bash "$BACKUP_SCRIPT" "$db" 2>&1)
  assertEquals 'rerun same day exit 0' 0 $?
  assertTrue 'output mentions Backup:' "printf '%s' \"$out\" | grep -q 'Backup:'"
  restored_dir=$(mktemp -d)
  restored="$restored_dir/restored.db"
  gunzip -c "$backup" > "$restored"
  assertEquals 'second row is present after rerun' 'segundo' "$(sqlite3 "$restored" 'SELECT name FROM t WHERE id=2;')"
  rm -rf "$tmp" "$restored_dir"
}

test_no_scheduling() {
  local tmp db before_crontab after_crontab before_timers after_timers
  tmp=$(mktemp -d)
  db="$tmp/reservas.db"
  sqlite3 "$db" "CREATE TABLE t(id INTEGER); INSERT INTO t VALUES (1);"
  before_crontab=$(crontab -l 2>/dev/null || true)
  before_timers=$(systemctl --user list-timers 2>/dev/null || true)
  bash "$BACKUP_SCRIPT" "$db" >/dev/null 2>&1
  assertEquals 'no-scheduling run exit 0' 0 $?
  after_crontab=$(crontab -l 2>/dev/null || true)
  after_timers=$(systemctl --user list-timers 2>/dev/null || true)
  assertEquals 'crontab is unchanged' "$before_crontab" "$after_crontab"
  assertEquals 'systemd timers are unchanged' "$before_timers" "$after_timers"
  rm -rf "$tmp"
}

. "$(dirname "$0")/lib/shunit2" || exit 1
