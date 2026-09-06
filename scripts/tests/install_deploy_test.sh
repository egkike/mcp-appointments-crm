#!/bin/bash
# Deploy pipeline tests for scripts/install.sh (Fase 5 PR3).
# Covers REQ-INS-001..013 and REQ-SU-001..005.

set -u

. "$(dirname "$0")/../install.sh" || exit 1

INSTALL_SH_PATH="$(cd "$(dirname "$0")/.." && pwd)/install.sh"

# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------

setUp() {
  TEST_HOME=$(mktemp -d)
  export HOME="$TEST_HOME"
  unset XDG_CONFIG_HOME XDG_DATA_HOME XDG_BIN_HOME XDG_STATE_HOME
  _SAVED_SCRIPT_DIR="$SCRIPT_DIR"
  _SAVED_EXTRACT_DIR="${EXTRACT_DIR:-}"

  # Provide minimal fake systemctl/loginctl so verify_installation and
  # enable_linger do not fail on hosts without a user service manager.
  mkdir -p "$TEST_HOME/fakebin"
  printf '#!/bin/bash\nexit 0\n' > "$TEST_HOME/fakebin/systemctl"
  printf '#!/bin/bash\nexit 0\n' > "$TEST_HOME/fakebin/loginctl"
  chmod +x "$TEST_HOME/fakebin/systemctl" "$TEST_HOME/fakebin/loginctl"
  PATH="$TEST_HOME/fakebin:$PATH"

  resolve_paths >/dev/null 2>&1 || true
  rc=0
}

tearDown() {
  rm -rf "$TEST_HOME"
  SCRIPT_DIR="$_SAVED_SCRIPT_DIR"
  EXTRACT_DIR="$_SAVED_EXTRACT_DIR"
  rc=0
}

# Create a minimal fixture release directory with an asset + checksums.txt.
# Files are placed under a {version}/ subdirectory, matching the release URL
# layout: {RELEASE_BASE_URL}/{tag}/{asset}.
_make_release_fixture() {
  local version="${1:-v0.3.0}" fixture_dir asset_dir asset_path
  fixture_dir=$(mktemp -d)
  asset_dir="$fixture_dir/$version"
  mkdir -p "$asset_dir"
  asset_path="$asset_dir/mcp-appointments-crm_Linux_x86_64.tar.gz"

  local bin_dir
  bin_dir=$(mktemp -d)
  printf '#!/bin/bash\necho "%s"\n' "$version" > "$bin_dir/mcp-server"
  chmod +x "$bin_dir/mcp-server"
  tar -czf "$asset_path" -C "$bin_dir" mcp-server
  rm -rf "$bin_dir"

  (cd "$asset_dir" && sha256sum "$(basename "$asset_path")" > checksums.txt)
  printf '%s' "$fixture_dir"
}

# ---------------------------------------------------------------------------
# T4.1 / T4.2 — Core deploy functions
# ---------------------------------------------------------------------------

test_compose_asset_name_matrix() {
  assertEquals 'Linux x86_64' \
    'mcp-appointments-crm_Linux_x86_64.tar.gz' \
    "$(compose_asset_name Linux x86_64)"
  assertEquals 'Linux aarch64 -> arm64' \
    'mcp-appointments-crm_Linux_arm64.tar.gz' \
    "$(compose_asset_name Linux aarch64)"
  assertEquals 'Darwin x86_64' \
    'mcp-appointments-crm_Darwin_x86_64.tar.gz' \
    "$(compose_asset_name Darwin x86_64)"
  assertEquals 'Darwin arm64' \
    'mcp-appointments-crm_Darwin_arm64.tar.gz' \
    "$(compose_asset_name Darwin arm64)"

  local out rc=0
  out=$(compose_asset_name Windows_NT i686 2>&1) || rc=$?
  assertTrue 'unmapped combo fails' "[ \${rc:-0} -ne 0 ]"
  assertTrue 'error names the combo' "printf '%s' \"$out\" | grep -q 'Windows_NT'"
  assertTrue 'error names arch' "printf '%s' \"$out\" | grep -q 'i686'"
}

test_validate_tag() {
  validate_tag v0.3.0 >/dev/null 2>&1
  assertEquals 'v0.3.0 ok' 0 $?

  validate_tag latest >/dev/null 2>&1
  assertNotEquals 'latest rejected' 0 $?

  validate_tag 0.3.0 >/dev/null 2>&1
  assertNotEquals 'missing v prefix rejected' 0 $?

  validate_tag vX >/dev/null 2>&1
  assertNotEquals 'non-numeric rejected' 0 $?

  validate_tag '' >/dev/null 2>&1
  assertNotEquals 'empty rejected' 0 $?
}

test_require_setup_files_missing() {
  mkdir -p "$CONFIG_DIR"

  local out rc=0
  out=$(require_setup_files 2>&1) || rc=$?
  assertTrue '0 files fails' "[ \${rc:-0} -ne 0 ]"
  assertTrue 'names business' "printf '%s' \"$out\" | grep -q 'setup_business.json'"
  assertTrue 'names staff' "printf '%s' \"$out\" | grep -q 'setup_staff.json'"
  assertTrue 'names services' "printf '%s' \"$out\" | grep -q 'setup_services.json'"

  touch "$CONFIG_DIR/setup_business.json"
  out=$(require_setup_files 2>&1) || rc=$?
  assertTrue '1 file fails' "[ \${rc:-0} -ne 0 ]"
  assertFalse 'business no longer missing' "printf '%s' \"$out\" | grep -q 'setup_business.json'"
  assertTrue 'staff still missing' "printf '%s' \"$out\" | grep -q 'setup_staff.json'"
  assertTrue 'services still missing' "printf '%s' \"$out\" | grep -q 'setup_services.json'"

  touch "$CONFIG_DIR/setup_staff.json" "$CONFIG_DIR/setup_services.json"
  require_setup_files >/dev/null 2>&1
  assertEquals 'all present ok' 0 $?
}

test_resolve_paths_linux_default() {
  unset XDG_CONFIG_HOME XDG_DATA_HOME XDG_BIN_HOME XDG_STATE_HOME
  resolve_paths >/dev/null 2>&1
  assertEquals 'CONFIG_DIR' "$HOME/.config/mcp-appointments-crm" "$CONFIG_DIR"
  assertEquals 'DATA_DIR' "$HOME/.local/share/mcp-appointments-crm" "$DATA_DIR"
  assertEquals 'BIN_DIR' "$HOME/.local/bin" "$BIN_DIR"
  assertEquals 'LOG_DIR' "$HOME/.local/state/mcp-appointments-crm" "$LOG_DIR"
  assertEquals 'ENV_FILE' "$HOME/.config/mcp-appointments-crm/.env" "$ENV_FILE"
}

test_resolve_paths_linux_xdg() {
  export XDG_DATA_HOME="$TEST_HOME/xdg_data"
  export XDG_BIN_HOME="$TEST_HOME/xdg_bin"
  export XDG_STATE_HOME="$TEST_HOME/xdg_state"
  resolve_paths >/dev/null 2>&1
  assertEquals 'DATA_DIR xdg' "$TEST_HOME/xdg_data/mcp-appointments-crm" "$DATA_DIR"
  assertEquals 'BIN_DIR xdg' "$TEST_HOME/xdg_bin" "$BIN_DIR"
  assertEquals 'LOG_DIR xdg' "$TEST_HOME/xdg_state/mcp-appointments-crm" "$LOG_DIR"
}

test_resolve_paths_darwin() {
  local fake_uname tmp
  tmp=$(mktemp -d)
  fake_uname="$tmp/uname"
  printf '#!/bin/bash\necho Darwin\n' > "$fake_uname"
  chmod +x "$fake_uname"

  unset XDG_CONFIG_HOME XDG_DATA_HOME XDG_BIN_HOME XDG_STATE_HOME
  HOME="$TEST_HOME" PATH="$tmp:$PATH" resolve_paths >/dev/null 2>&1
  assertEquals 'Darwin DATA_DIR' \
    "$TEST_HOME/Library/Application Support/MCP Appointments CRM" "$DATA_DIR"
  assertEquals 'Darwin LOG_DIR' \
    "$TEST_HOME/Library/Logs/MCP Appointments CRM" "$LOG_DIR"
  assertEquals 'Darwin BIN_DIR' "$TEST_HOME/.local/bin" "$BIN_DIR"
  rm -rf "$tmp"
}

test_resolve_paths_symlink_rejected() {
  mkdir -p "$TEST_HOME/real" "$TEST_HOME/.local/share"
  ln -s "$TEST_HOME/real" "$TEST_HOME/.local/share/mcp-appointments-crm"

  local out rc=0
  out=$(resolve_paths 2>&1) || rc=$?
  assertTrue 'symlink in DATA_DIR rejected' "[ \${rc:-0} -ne 0 ]"
  assertTrue 'error mentions symlink' "printf '%s' \"$out\" | grep -qi 'enlace simbólico\|symlink'"
}

test_ensure_env_file_creates() {
  ensure_env_file
  assertTrue 'env file created' "[ -f \"$ENV_FILE\" ]"
  assertEquals 'env perms' '600' "$(stat -c %a "$ENV_FILE")"
  assertTrue 'MCP_BIND' "grep -q '^MCP_BIND=127.0.0.1$' \"$ENV_FILE\""
  assertTrue 'MCP_PORT' "grep -q '^MCP_PORT=3000$' \"$ENV_FILE\""
}

test_ensure_env_file_preserves() {
  mkdir -p "$(dirname "$ENV_FILE")"
  printf 'MCP_BIND=127.0.0.1\nMCP_PORT=3100\n' > "$ENV_FILE"
  local before
  before=$(cat "$ENV_FILE")
  ensure_env_file
  assertEquals 'env preserved byte-identical' "$before" "$(cat "$ENV_FILE")"
}

test_sha256_file_valid() {
  local tmp expected actual
  tmp=$(mktemp -d)
  printf 'fixture content' > "$tmp/asset.tar.gz"
  expected=$(sha256sum "$tmp/asset.tar.gz" | awk '{print $1}')
  actual=$(sha256_file "$tmp/asset.tar.gz")
  assertEquals 'sha256 matches sha256sum' "$expected" "$actual"
  rm -rf "$tmp"
}

test_download_and_verify_valid() {
  local fixture_dir
  fixture_dir=$(_make_release_fixture v0.3.0)
  INSTALL_TAG="v0.3.0"
  ASSET_NAME="mcp-appointments-crm_Linux_x86_64.tar.gz"
  MCP_RELEASE_BASE="file://$fixture_dir"
  export MCP_RELEASE_BASE

  download_and_verify >/dev/null 2>&1
  assertEquals 'valid release passes' 0 $?
  assertTrue 'asset file retained' "[ -f \"$ASSET_FILE\" ]"

  rm -rf "$fixture_dir"
}

test_download_and_verify_tampered() {
  local fixture_dir asset_path version_dir
  fixture_dir=$(_make_release_fixture v0.3.0)
  version_dir="$fixture_dir/v0.3.0"
  asset_path="$version_dir/mcp-appointments-crm_Linux_x86_64.tar.gz"
  # Flip one byte in the middle of the gzipped archive.
  python3 - <<PY || { echo 'SKIP: python3 not available' >&2; rm -rf "$fixture_dir"; return; }
import sys
p='$asset_path'
with open(p,'rb') as f: d=bytearray(f.read())
d[len(d)//2] ^= 1
with open(p,'wb') as f: f.write(d)
PY

  INSTALL_TAG="v0.3.0"
  ASSET_NAME="mcp-appointments-crm_Linux_x86_64.tar.gz"
  MCP_RELEASE_BASE="file://$fixture_dir"
  export MCP_RELEASE_BASE
  BIN_DIR="$TEST_HOME/bin"
  mkdir -p "$BIN_DIR"

  local rc=0
  download_and_verify >/dev/null 2>&1 || rc=$?
  assertTrue 'tampered asset fails' "[ \${rc:-0} -ne 0 ]"
  assertFalse 'no binary installed after tampered download' "[ -f \"$BIN_DIR/mcp-server\" ]"

  rm -rf "$fixture_dir"
}

test_download_and_verify_missing_checksum() {
  local fixture_dir
  fixture_dir=$(_make_release_fixture v0.3.0)
  # Remove the asset line from checksums.txt.
  printf '' > "$fixture_dir/v0.3.0/checksums.txt"

  INSTALL_TAG="v0.3.0"
  ASSET_NAME="mcp-appointments-crm_Linux_x86_64.tar.gz"
  MCP_RELEASE_BASE="file://$fixture_dir"
  export MCP_RELEASE_BASE

  local out rc=0
  out=$(download_and_verify 2>&1) || rc=$?
  assertTrue 'missing checksum fails' "[ \${rc:-0} -ne 0 ]"
  assertTrue 'error names the asset' "printf '%s' \"$out\" | grep -q '$ASSET_NAME'"

  rm -rf "$fixture_dir"
}

# ---------------------------------------------------------------------------
# T5.1 / T5.2 — Service rendering, verification, summary
# ---------------------------------------------------------------------------

test_render_systemd_unit_default() {
  local template rendered
  template=$(resolve_service_template mcp-appointments-crm.service)
  rendered=$(render_systemd_unit "$template")
  assertTrue 'BIN_DIR substituted' "printf '%s' \"$rendered\" | grep -q \"$BIN_DIR/mcp-server\""
  assertTrue 'literal %h preserved' "printf '%s' \"$rendered\" | grep -q '%h/.local/share/mcp-appointments-crm/reservas.db'"
}

test_render_systemd_unit_xdg_data() {
  export XDG_DATA_HOME="$TEST_HOME/custom-data"
  resolve_paths >/dev/null 2>&1
  local template rendered
  template=$(resolve_service_template mcp-appointments-crm.service)
  rendered=$(render_systemd_unit "$template")
  assertTrue 'custom MCP_DB_PATH rendered' \
    "printf '%s' \"$rendered\" | grep -q \"Environment=MCP_DB_PATH=$DATA_DIR/reservas.db\""
  assertFalse 'literal %h not used when custom' \
    "printf '%s' \"$rendered\" | grep -q '%h/.local/share/mcp-appointments-crm/reservas.db'"
}

test_render_systemd_unit_source_archive() {
  local fixture_dir template_path template
  fixture_dir=$(mktemp -d)
  mkdir -p "$fixture_dir/setup/service"
  template_path="$fixture_dir/setup/service/mcp-appointments-crm.service"
  cp "$(resolve_service_template mcp-appointments-crm.service)" "$template_path"
  printf '\n# from archive\n' >> "$template_path"

  EXTRACT_DIR="$fixture_dir"
  template=$(resolve_service_template mcp-appointments-crm.service)
  assertEquals 'archive template chosen' "$template_path" "$template"
  rm -rf "$fixture_dir"
}

test_render_systemd_unit_source_repo_fallback() {
  local fixture_dir template
  fixture_dir=$(mktemp -d)
  mkdir -p "$fixture_dir/setup/service"
  EXTRACT_DIR="$fixture_dir"
  template=$(resolve_service_template mcp-appointments-crm.service)
  assertTrue 'repo fallback used' "printf '%s' \"$template\" | grep -q '/setup/service/mcp-appointments-crm.service$'"
  rm -rf "$fixture_dir"
}

test_render_launchd_plist() {
  local template rendered
  template=$(resolve_service_template com.mcp.appointments.server.plist)
  rendered=$(render_launchd_plist "$template")
  assertTrue 'BIN_DIR substituted' "printf '%s' \"$rendered\" | grep -q \"$BIN_DIR/mcp-server\""
  assertTrue 'DATA_DIR substituted' "printf '%s' \"$rendered\" | grep -q \"$DATA_DIR/reservas.db\""
  assertTrue 'LOG_DIR substituted' "printf '%s' \"$rendered\" | grep -q \"$LOG_DIR/mcp-server\""
}

test_enable_linger() {
  if [ "$(uname -s)" != "Linux" ]; then
    echo 'SKIP: enable_linger solo aplica en Linux'
    return
  fi
  local tmp
  tmp=$(mktemp -d)
  cat > "$tmp/loginctl" <<EOF
#!/bin/bash
printf '%s\\n' "\$*" >> "$tmp/called"
EOF
  chmod +x "$tmp/loginctl"

  PATH="$tmp:$PATH" enable_linger >/dev/null 2>&1
  assertEquals 'enable_linger returns 0' 0 $?
  assertTrue 'loginctl invoked' "[ -f \"$tmp/called\" ]"
  assertTrue 'enable-linger arg present' "grep -q 'enable-linger' \"$tmp/called\""
  rm -rf "$tmp"
}

test_enable_linger_unset_user() {
  # R3-001: with USER unset under set -u, linger must fall back to
  # id -un instead of aborting on unbound variable.
  if [ "$(uname -s)" != "Linux" ]; then
    echo 'SKIP: enable_linger solo aplica en Linux'
    return
  fi
  local tmp out rc=0 expected
  tmp=$(mktemp -d)
  cat > "$tmp/loginctl" <<EOF
#!/bin/bash
printf '%s\\n' "\$*" >> "$tmp/called"
EOF
  chmod +x "$tmp/loginctl"
  expected=$(id -un)
  # NOTE: path goes in $1, not $0 — otherwise the BASH_SOURCE guard
  # at the bottom of install.sh would run main() on source.
  out=$(env -u USER PATH="$tmp:/usr/bin:/bin" bash -c '. "$1"; enable_linger' _ "$INSTALL_SH_PATH" 2>&1) || rc=$?
  assertEquals 'enable_linger returns 0 without USER' 0 ${rc:-0}
  assertTrue 'loginctl invoked without USER' "[ -f \"$tmp/called\" ]"
  assertTrue 'falls back to id -un' "grep -q \"enable-linger $expected\" \"$tmp/called\""
  rm -rf "$tmp"
}

test_verify_install_version_match() {
  INSTALL_TAG="v0.3.0"
  mkdir -p "$BIN_DIR"
  printf '#!/bin/bash\necho "%s"\n' "$INSTALL_TAG" > "$BIN_DIR/mcp-server"
  chmod +x "$BIN_DIR/mcp-server"

  verify_installation >/dev/null 2>&1
  assertEquals 'version match ok' 0 $?
}

test_verify_install_version_mismatch() {
  INSTALL_TAG="v0.3.0"
  mkdir -p "$BIN_DIR"
  printf '#!/bin/bash\necho "v0.2.0"\n' > "$BIN_DIR/mcp-server"
  chmod +x "$BIN_DIR/mcp-server"

  verify_installation >/dev/null 2>&1
  assertNotEquals 'version mismatch fails' 0 $?
}

test_print_post_install_summary() {
  INSTALL_TAG="v0.3.0"
  DATA_DIR="$TEST_HOME/data"
  mkdir -p "$DATA_DIR"
  local out
  out=$(print_post_install_summary 2>&1)
  assertTrue 'backup.sh line with DB path' \
    "printf '%s' \"$out\" | grep -q 'backup.sh.*$DATA_DIR/reservas.db'"
  assertTrue 'recommended tools block' \
    "printf '%s' \"$out\" | grep -qi 'herramientas recomendadas\|recommended additional tools'"
  assertTrue 'endpoint URL' "printf '%s' \"$out\" | grep -q '127.0.0.1:3000/mcp'"
  assertTrue 'MCP_DB_PATH' "printf '%s' \"$out\" | grep -q \"$DATA_DIR/reservas.db\""
}

test_run_setup_guard_tty_no_tty() {
  # Simulate the piped invocation that has $0 == bash and no TTY.
  local out rc=0
  out=$(bash -c ". $INSTALL_SH_PATH; run_setup_guard_tty" bash </dev/null 2>&1) || rc=$?
  assertTrue 'no tty fails' "[ \${rc:-0} -ne 0 ]"
  assertTrue 'mentions terminal' "printf '%s' \"$out\" | grep -qi 'terminal'"
}

# ---------------------------------------------------------------------------
# Backup script installation (CRITICAL-1 fix)
# ---------------------------------------------------------------------------

test_install_backup_script_from_extract() {
  local fixture_dir expected
  fixture_dir=$(mktemp -d)
  mkdir -p "$fixture_dir/scripts"
  printf '#!/bin/bash\necho "backup from archive"\n' > "$fixture_dir/scripts/backup.sh"
  chmod 0644 "$fixture_dir/scripts/backup.sh"
  expected=$(cat "$fixture_dir/scripts/backup.sh")

  EXTRACT_DIR="$fixture_dir"
  SCRIPT_DIR=$(mktemp -d)
  install_backup_script >/dev/null 2>&1
  assertEquals 'persistent backup exists' 0 $?
  assertTrue 'backup file installed' "[ -f \"$DATA_DIR/scripts/backup.sh\" ]"
  assertTrue 'backup is executable' "[ -x \"$DATA_DIR/scripts/backup.sh\" ]"
  assertEquals 'backup perms' '755' "$(stat -c %a "$DATA_DIR/scripts/backup.sh")"
  assertEquals 'backup dir perms' '700' "$(stat -c %a "$DATA_DIR/scripts")"
  assertEquals 'content matches archive' "$expected" "$(cat "$DATA_DIR/scripts/backup.sh")"

  rm -rf "$fixture_dir" "$SCRIPT_DIR"
}

test_install_backup_script_fallback_script_dir() {
  local fake_script_dir expected
  fake_script_dir=$(mktemp -d)
  mkdir -p "$fake_script_dir/scripts"
  printf '#!/bin/bash\necho "backup from script dir"\n' > "$fake_script_dir/scripts/backup.sh"
  expected=$(cat "$fake_script_dir/scripts/backup.sh")

  EXTRACT_DIR=""
  SCRIPT_DIR="$fake_script_dir/scripts"
  install_backup_script >/dev/null 2>&1
  assertEquals 'fallback install ok' 0 $?
  assertTrue 'backup file installed from fallback' "[ -f \"$DATA_DIR/scripts/backup.sh\" ]"
  assertTrue 'backup is executable' "[ -x \"$DATA_DIR/scripts/backup.sh\" ]"
  assertEquals 'fallback content matches' "$expected" "$(cat "$DATA_DIR/scripts/backup.sh")"

  rm -rf "$fake_script_dir"
}

test_install_backup_script_no_source_fails() {
  local out rc=0
  SCRIPT_DIR=$(mktemp -d)
  EXTRACT_DIR=""

  out=$(install_backup_script 2>&1) || rc=$?
  assertTrue 'no source fails' "[ \${rc:-0} -ne 0 ]"
  assertTrue 'error mentions backup.sh' "printf '%s' \"$out\" | grep -q 'backup.sh'"

  rm -rf "$SCRIPT_DIR"
}

test_post_install_backup_cmd_prefers_persistent() {
  local persistent out
  persistent="$DATA_DIR/scripts/backup.sh"
  mkdir -p "$DATA_DIR/scripts"
  printf '#!/bin/bash\necho persistent\n' > "$persistent"
  chmod 0755 "$persistent"

  EXTRACT_DIR=""
  out=$(_post_install_backup_cmd)
  assertEquals 'prefers persistent path' "$persistent" "$out"
}

# ---------------------------------------------------------------------------
# RDD correction_required (review-214a49fd9beea8c5) — regression tests
# ---------------------------------------------------------------------------

test_ensure_dirs_failure_propagates() {
  # R3-ENSUREDIRS-PARTIAL / R4-ENSURE-DIRS-MASK: an early mkdir failure
  # must fail the function even when later dirs succeed.
  mkdir() {
    case "$*" in
      *"$DATA_DIR"*) return 1 ;;
    esac
    command mkdir "$@"
  }
  local rc=0
  ensure_dirs >/dev/null 2>&1 || rc=$?
  unset -f mkdir
  assertTrue 'ensure_dirs fails when DATA_DIR cannot be created' "[ \${rc:-0} -ne 0 ]"
}

test_install_service_write_failure() {
  # R3-ATOMICWRITE-UNCHECKED: a failed unit write must stop the deploy
  # before daemon-reload. Fake systemctl exits 0, so only the write
  # failure can fail this call.
  atomic_write() { return 1; }
  local rc=0
  install_service_linux >/dev/null 2>&1 || rc=$?
  unset -f atomic_write
  assertTrue 'install_service_linux fails when unit write fails' "[ \${rc:-0} -ne 0 ]"
}

test_install_binary_backs_up_prev() {
  # R4-BINARY-NO-ROLLBACK: the previous binary must survive the replace.
  local extract_dir bin_dir
  extract_dir=$(mktemp -d)
  bin_dir=$(mktemp -d)
  printf 'new-binary' > "$extract_dir/mcp-server"
  printf 'old-binary' > "$bin_dir/mcp-server"
  EXTRACT_DIR="$extract_dir"
  BIN_DIR="$bin_dir"
  install_binary >/dev/null 2>&1
  assertEquals 'dest has new content' 'new-binary' "$(cat "$bin_dir/mcp-server")"
  assertEquals 'prev keeps old content' 'old-binary' "$(cat "$bin_dir/mcp-server.prev")"
  rm -rf "$extract_dir" "$bin_dir"
}

test_verify_install_hints_rollback() {
  # R4-BINARY-NO-ROLLBACK: on verify failure the operator is told
  # where the preserved previous binary lives.
  local bin_dir out rc=0
  bin_dir=$(mktemp -d)
  printf '#!/bin/bash\necho wrong-version\n' > "$bin_dir/mcp-server"
  chmod +x "$bin_dir/mcp-server"
  printf 'old-binary' > "$bin_dir/mcp-server.prev"
  BIN_DIR="$bin_dir"
  INSTALL_TAG="v9.9.9"
  out=$(verify_installation 2>&1) || rc=$?
  assertTrue 'verify fails on version mismatch' "[ \${rc:-0} -ne 0 ]"
  assertTrue 'error points at the preserved binary' "printf '%s' \"$out\" | grep -q 'mcp-server.prev'"
  rm -rf "$bin_dir"
}

. "$(dirname "$0")/lib/shunit2" || exit 1
