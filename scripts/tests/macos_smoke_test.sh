#!/bin/bash
# scripts/tests/macos_smoke_test.sh — REAL-binary end-to-end smoke for the
# macOS CI job (native darwin runner, macos-15):
#
#   1. builds cmd/mcp-server with the runner's Go, injecting the version via
#      -ldflags exactly like GoReleaser does (internal/buildinfo.Version)
#   2. stages a checksummed fake release tar.gz carrying that binary plus the
#      setup/ tree (service templates + skill template) and the backup script,
#      so download_and_verify reads it over file:// without touching the network
#   3. runs the full `install.sh --version` in a sandboxed HOME: setup JSON
#      stubs (existence-gated by require_setup_files), non-colliding MCP_PORT,
#      real curl + SHA256 verification + tar extraction
#   4. asserts the native binary answers --version with the tag, the launchd
#      agent plist was rendered with the right directories, and the service is
#      actually loaded (launchctl print) with a running PID
#   5. verifies the real-binary bootout teardown: the agent disappears, and a
#      second bootout rerun is harmless (nothing left behind, no crash)
#
# The harness never touches the network (file:// release base) and keeps every
# filesystem interaction inside its own mktemp prefix via a sandboxed HOME.
# Requires bash 3.2 (macOS stock) — anything newer is not used on purpose.
# ENFORCED, not claimed (R3-001, reliability advisory on the PR #174 review):
# the preflight guard aborts with a clear message under a non-bash interpreter
# or a bash older than 3.x, instead of failing later with a cryptic
# version-quirk symptom (set -u substitution behavior differs on old bash).

set -u
umask 077

SCRIPT_DIR=$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)
REPO_ROOT=$(CDPATH='' cd -- "$SCRIPT_DIR/../.." && pwd)
INSTALLER="$REPO_ROOT/scripts/install.sh"
# validate_tag requires vMAJOR.MINOR.PATCH; this tag must never collide with a
# real release because MCP_RELEASE_BASE points at the staged fake asset.
SMOKE_TAG="v0.0.999"
# Uncommon port: preflight_port_available fails the deploy if a listener
# already holds the port, so pick one that should be free on the runner.
SMOKE_PORT="30187"

PASS_COUNT=0
FAIL_COUNT=0
Fx=""
Fx=$(mktemp -d "/tmp/mcp-appointments-macos-smoke.XXXXXX")
trap 'rm -rf "$Fx"' EXIT

say() { printf 'smoke: %s\n' "$1"; }
pass() { PASS_COUNT=$((PASS_COUNT + 1)); say "PASS: $1"; }
fail() { FAIL_COUNT=$((FAIL_COUNT + 1)); say "FAIL: $1"; }

die() {
	say "ABORT: $1"
	exit 1
}

# --- Interpreter guard (enforces the bash 3.2 promise above) ----------------
# Deliberately BEFORE the darwin skip: mis-invocation (sh/dash) must fail
# loudly everywhere, not silently take the SKIP path.
if [ -z "${BASH_VERSION:-}" ]; then
	die "this harness must run under bash (BASH_VERSION unset — for example, invoked via sh/dash); re-run with: bash $0"
fi
BASH_MAJOR=$(printf "%s" "$BASH_VERSION" | cut -d. -f1)
case "$BASH_MAJOR" in
	''|*[!0-9]*)
		die "cannot parse bash major version from '$BASH_VERSION'"
		;;
esac
if [ "$BASH_MAJOR" -lt 3 ]; then
	die "bash $BASH_VERSION is too old: the harness targets stock macOS bash 3.2, so major >= 3 is required"
fi
pass "interpreter guard: bash $BASH_VERSION (major $BASH_MAJOR) satisfies the bash 3.2 promise"

# ---------------------------------------------------------------------------
# Preflight: this harness is darwin-only on purpose (it exercises launchctl,
# shasum and LaunchAgents paths that exist only on a native runner).
# ---------------------------------------------------------------------------
OS_NAME=$(uname -s)
if [ "$OS_NAME" = "Darwin" ]; then
	pass "running on darwin (uname: $(uname -srm))"
else
	say "SKIP: non-darwin host ($OS_NAME) — native smoke requires macOS"
	exit 0
fi
command -v go >/dev/null 2>&1 || die "go not on PATH"
command -v curl >/dev/null 2>&1 || die "curl not on PATH"
command -v shasum >/dev/null 2>&1 || die "shasum not on PATH"

cd "$REPO_ROOT" || die "cannot cd to repo root"

# ---------------------------------------------------------------------------
# 1. Build the real binary, injecting the tag the installer will verify.
# ---------------------------------------------------------------------------
mkdir -p "$Fx/build" || die "mktemp build dir"
if ! ( go build -o "$Fx/build/mcp-server" \
	-ldflags "-X github.com/egkike/mcp-appointments-crm/internal/buildinfo.Version=$SMOKE_TAG" \
	./cmd/mcp-server ) >"$Fx/build.log" 2>&1; then
	tail -5 "$Fx/build.log" >&2
	die "go build cmd/mcp-server failed"
fi
[ -x "$Fx/build/mcp-server" ] || die "built binary missing"
pass "built real darwin binary with ldflags version $SMOKE_TAG"

# ---------------------------------------------------------------------------
# 2. Stage the fake release: GoReleaser-shaped archive + checksums.txt.
#    install_binary expects the binary at the archive root (EXTRACT_DIR/
#    mcp-server); resolve_service_template favors EXTRACT_DIR/setup/service;
#    seed_skill_pristine favors EXTRACT_DIR/setup/templates.
# ---------------------------------------------------------------------------
STAGE="$Fx/release-stage"
mkdir -p "$STAGE/setup/service" "$STAGE/setup/templates" "$STAGE/scripts" || die "staging mkdir"
cp "$Fx/build/mcp-server" "$STAGE/mcp-server" || die "stage binary"
cp "$REPO_ROOT/setup/service/com.mcp.appointments.server.plist" "$STAGE/setup/service/" || die "stage plist template"
if [ -d "$REPO_ROOT/setup/templates" ]; then
	cp -R "$REPO_ROOT/setup/templates/." "$STAGE/setup/templates/" || die "stage skill templates"
fi
if [ -f "$REPO_ROOT/scripts/backup.sh" ]; then
	cp "$REPO_ROOT/scripts/backup.sh" "$STAGE/scripts/backup.sh" || die "stage backup script"
fi

ARCH=$(uname -m)
case "$ARCH" in
	arm64) ASSET_PATH="mcp-appointments-crm_Darwin_arm64.tar.gz" ;;
	x86_64) ASSET_PATH="mcp-appointments-crm_Darwin_x86_64.tar.gz" ;;
	*) die "unsupported runner arch: $ARCH" ;;
esac
# download_and_verify composes the URL as <base>/<tag>/<asset> (GitHub
# Releases shape), so the fake release must carry the tag subdirectory too.
mkdir -p "$Fx/release/$SMOKE_TAG" || die "release dir"
ASSET_FILE="$Fx/release/$SMOKE_TAG/$ASSET_PATH"
if ! ( cd "$STAGE" && tar -czf "$ASSET_FILE" mcp-server setup scripts ); then
	die "tar staging failed"
fi
rm -rf "$STAGE" || die "cannot remove staging"
RAW_HASH=$(shasum -a 256 "$ASSET_FILE" | awk '{print $1}')
printf '%s  %s\n' "$RAW_HASH" "$ASSET_PATH" > "$Fx/release/$SMOKE_TAG/checksums.txt" || die "checksums.txt write"
pass "staged checksummed fake release: $ASSET_PATH"

# curl file:// URLs need an absolute path with no spaces; mktemp chose this
# dir, but keep the guard cheap and explicit because the contract matters.
case "$Fx" in
	*[[:space:]]*) die "temp dir contains spaces; file:// release base would break" ;;
esac
RELEASE_BASE="file://$Fx/release"

# ---------------------------------------------------------------------------
# 3. Sandbox HOME + setup stubs, then run the real installer end-to-end.
# ---------------------------------------------------------------------------
# Threads HOME through every subprocess: resolve_paths, the LaunchAgents dir,
# the .env file — the whole install tree lives inside the temp prefix, so the
# harness never mutates the runner user's real HOME.
SMOKE_HOME="$Fx/home"
BIN_DIR="$SMOKE_HOME/.local/bin"
LOG_DIR="$SMOKE_HOME/Library/Logs/MCP Appointments CRM"
PLIST_PATH="$SMOKE_HOME/Library/LaunchAgents/com.mcp.appointments.server.plist"
ENV_PATH="$SMOKE_HOME/.config/mcp-appointments-crm/.env"

mkdir -p "$SMOKE_HOME/Library/Application Support/MCP Appointments CRM/setup" || die "HOME setup dir"
for f in setup_business.json setup_staff.json setup_services.json; do
	printf '{}\n' > "$SMOKE_HOME/Library/Application Support/MCP Appointments CRM/setup/$f" || die "setup stub $f"
done
pass "staged setup JSON stubs in sandboxed HOME"

HOME="$SMOKE_HOME"
HERMES_HOME="$SMOKE_HOME/.hermes"
MCP_RELEASE_BASE="$RELEASE_BASE"
MCP_PORT="$SMOKE_PORT"
export HOME HERMES_HOME MCP_RELEASE_BASE MCP_PORT

INSTALL_RC=0
bash "$INSTALLER" --version "$SMOKE_TAG" > "$Fx/install.log" 2>&1 || INSTALL_RC=$?
if [ "$INSTALL_RC" -eq 0 ]; then
	pass "install.sh --version $SMOKE_TAG exited 0 (real curl + checksum + tar + launchctl path)"
else
	fail "install.sh --version $SMOKE_TAG exited $INSTALL_RC"
	say "----- install.log ($(wc -c < "$Fx/install.log") bytes, invisible chars shown) -----"
	cat -ev "$Fx/install.log" >&2
	say "----- end install.log -----"
	# Diagnostic rerun under bash -x: same env, so the trace reproduces the
	# failure line-for-line and gives the CI log the exact failing command.
	bash -x "$INSTALLER" --version "$SMOKE_TAG" > "$Fx/install-x.log" 2>&1
	say "----- bash -x trace tail -----"
	tail -80 "$Fx/install-x.log" >&2
	say "----- end bash -x trace tail -----"
fi

# Restore the runner's HOME so later asserts and any failure hygiene step the
# CI prints keep the real user identity.
unset HOME HERMES_HOME MCP_RELEASE_BASE MCP_PORT

# ---------------------------------------------------------------------------
# 4. Post-install assertions against the real filesystem + launchd.
# ---------------------------------------------------------------------------
if [ -x "$BIN_DIR/mcp-server" ]; then
	pass "binary installed at BIN_DIR"
else
	fail "binary missing at $BIN_DIR/mcp-server"
fi

if [ -x "$BIN_DIR/mcp-server" ]; then
	if ( HOME="$SMOKE_HOME" "$BIN_DIR/mcp-server" --version 2>&1 ) | grep -qF "$SMOKE_TAG"; then
		pass "binary --version reports $SMOKE_TAG"
	else
		fail "binary --version output mismatch"
	fi
fi

if [ -f "$PLIST_PATH" ]; then
	pass "launchd agent plist present"
	if grep -qF "$BIN_DIR/mcp-server" "$PLIST_PATH" \
		&& grep -qF "$SMOKE_HOME/Library/Application Support/MCP Appointments CRM/reservas.db" "$PLIST_PATH" \
		&& grep -qF "$LOG_DIR" "$PLIST_PATH"; then
		pass "plist rendered with sandboxed BIN_DIR/DATA_DIR/LOG_DIR"
	else
		fail "plist missing expected rendered paths (BIN_DIR/DATA_DIR/LOG_DIR)"
	fi
else
	fail "launchd agent plist missing at $PLIST_PATH"
fi

LAUNCHCTL_UID="$(id -u)"
if launchctl print "gui/$LAUNCHCTL_UID/com.mcp.appointments.server" > "$Fx/launchctl-print.txt" 2>&1; then
	pass "launchd service actually loaded (launchctl print ok)"
	PID_LINE=$(grep -E 'pid = [0-9]+' "$Fx/launchctl-print.txt" | head -n 1 | grep -Eo '[0-9]+')
	if [ -n "$PID_LINE" ] && [ "$PID_LINE" != "0" ]; then
		pass "agent holds a running PID ($PID_LINE) — RunAtLoad + KeepAlive honored"
	else
		fail "agent loaded but no running PID visible in launchctl print"
	fi
else
	fail "launchd service not loaded (launchctl print failed)"
	say "----- launchctl print stderr -----"
	tail -5 "$Fx/launchctl-print.txt" >&2
	say "----- end launchctl print -----"
fi

if [ -f "$ENV_PATH" ] && grep -qF "MCP_PORT=$SMOKE_PORT" "$ENV_PATH"; then
	pass ".env persisted with the exported MCP_PORT"
else
	fail "$ENV_PATH missing or without MCP_PORT=$SMOKE_PORT"
fi

# ---------------------------------------------------------------------------
# 5. Teardown + idempotent bootout (the repo has no uninstall subcommand; the
# real-world cleanup is launchctl bootout, so prove both passes here).
# ---------------------------------------------------------------------------
launchctl bootout "gui/$LAUNCHCTL_UID/com.mcp.appointments.server" > "$Fx/bootout1.log" 2>&1
# rc is checked only through the state: launchctl print must now fail.
if ! launchctl print "gui/$LAUNCHCTL_UID/com.mcp.appointments.server" > /dev/null 2>&1; then
	pass "agent gone after bootout (launchctl print fails)"
else
	fail "agent still loaded after bootout"
fi

# The second bootout must be harmless (agent already gone — verify no crash and
# that the label remains absent).
launchctl bootout "gui/$LAUNCHCTL_UID/com.mcp.appointments.server" > "$Fx/bootout2.log" 2>&1
if ! launchctl print "gui/$LAUNCHCTL_UID/com.mcp.appointments.server" > /dev/null 2>&1; then
	pass "second bootout is harmless (agent still absent, no crash)"
else
	fail "second bootout left the agent in an unexpected state"
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
say "PASS: $PASS_COUNT, FAIL: $FAIL_COUNT"
if [ "$FAIL_COUNT" -eq 0 ]; then
	say "smoke OK"
	exit 0
fi
say "smoke FAILED"
exit 1
