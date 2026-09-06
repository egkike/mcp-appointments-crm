#!/bin/bash
# Unit tests for setup/service/ templates (shunit2).
# Covers REQ-SU-001..005 + regression for R3-ENVFILE-BRITTLE
# (EnvironmentFile MUST stay optional so a missing .env never
# fails the unit start).

set -u

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
SERVICE_DIR="$SCRIPT_DIR/../../setup/service"
UNIT="$SERVICE_DIR/mcp-appointments-crm.service"
PLIST="$SERVICE_DIR/com.mcp.appointments.server.plist"
NSSM_DOC="$SERVICE_DIR/nssm-install.md"

test_systemd_envfile_is_optional() {
  assertTrue '.env must be optional (dash prefix, R3-ENVFILE-BRITTLE)' \
    "grep -q '^EnvironmentFile=-' \"$UNIT\""
}

test_systemd_stays_user_level() {
  assertFalse 'no User= directive (user-level, REQ-SU-002)' \
    "grep -q '^User=' \"$UNIT\""
  assertTrue 'WantedBy=default.target present' \
    "grep -q '^WantedBy=default.target' \"$UNIT\""
  assertTrue 'MCP_DB_PATH fallback present' \
    "grep -q '^Environment=MCP_DB_PATH=' \"$UNIT\""
}

test_systemd_no_system_paths() {
  # Comment lines may mention /etc to document its absence; only
  # effective directives count.
  assertFalse 'no /etc paths in directives' \
    "grep -v '^[[:space:]]*#' \"$UNIT\" | grep -q '/etc'"
}

test_launchd_contract() {
  assertTrue 'Label present' \
    "grep -q '<string>com.mcp.appointments.server</string>' \"$PLIST\""
  assertTrue 'RunAtLoad true' "grep -q '<key>RunAtLoad</key>' \"$PLIST\""
  assertTrue 'KeepAlive true' "grep -q '<key>KeepAlive</key>' \"$PLIST\""
}

test_templates_have_no_hardcoded_home() {
  assertFalse 'unit has no /home/ literal (%h portable, REQ-SU-005)' \
    "grep -q '/home/' \"$UNIT\""
  assertFalse 'plist has no /home/ literal' \
    "grep -q '/home/' \"$PLIST\""
}

test_nssm_placeholder_exists() {
  assertTrue 'nssm-install.md exists (REQ-SU-004)' "[ -f \"$NSSM_DOC\" ]"
}

# shellcheck disable=SC1091
. "$SCRIPT_DIR/lib/shunit2"
