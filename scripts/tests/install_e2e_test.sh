#!/bin/bash
# E2E and integration tests for install.sh interactive flow (PR2a: tasks 2.1-2.6).
#
# Scope: flat store, checkpoint, field registry, R/S/Q + reconfigure gate,
# prompt_field engine, business profile section. Hours/staff/services/resume
# flows belong to PR2b (tasks 2.7-2.13).

. "$(dirname "$0")/../install.sh" || exit 1

setUp() {
  TEST_HOME=$(mktemp -d)
  export HOME="$TEST_HOME"
  export XDG_CONFIG_HOME="$TEST_HOME/.config"
  resolve_paths
  store_clear
}

tearDown() {
  rm -rf "$TEST_HOME"
}

_run_install() {
  bash "$(dirname "$0")/../install.sh"
}

_assert_json_parseable() {
  python3 -m json.tool "$1" >/dev/null 2>&1
  assertTrue "parseable $1" "[ $? -eq 0 ]"
}

# Input builder: 18 business answers (PR2a flow stops after business section).

_business_input() {
  printf '%s\n' \
    "D'Átelier \"La Casa\"" \
    "Peluquería" \
    "ar" \
    "Calle 123" \
    "-34.5" \
    "-58.3" \
    "https://example.com/cover.jpg" \
    "+5491122334455" \
    "whatsapp" \
    "+5491111111111" \
    "hola@latel.com" \
    "" \
    "Descripción general" \
    "efectivo, tarjeta , transferencia" \
    "" \
    "" \
    "America/Argentina/Buenos_Aires" \
    "30"
}

# Task 2.1 / 2.2: flat store + checkpoint ------------------------------------

test_store_helpers() {
  store_set "business.name" "X"
  assertEquals 'get' 'X' "$(store_get business.name)"
  assertTrue 'has' 'store_has business.name'
  store_set "business.name" "Y"
  assertEquals 'overwrite' 'Y' "$(store_get business.name)"
  store_unset "business.name"
  assertFalse 'unset' 'store_has business.name'
  store_set "a.b" 'with "quotes'
  assertEquals 'quotes' 'with "quotes' "$(store_get a.b)"
  store_clear
  assertEquals 'clear' '' "$STORE"
}

test_checkpoint_render_parses() {
  store_set "business.name" "D'Átelier \"La Casa\""
  store_set "business.industry" "null"
  store_set "business.slot_interval_minutes" "30"
  local json rc
  json=$(checkpoint_render)
  printf '%s' "$json" | python3 -m json.tool >/dev/null 2>&1
  rc=$?
  assertTrue 'json parseable' "[ $rc -eq 0 ]"
}

test_checkpoint_save_load_roundtrip() {
  mkdir -p "$CONFIG_DIR"
  store_set "business.name" "X"
  store_set "business.country" "AR"
  checkpoint_save
  assertTrue 'checkpoint file' "[ -f \"$CHECKPOINT_PATH\" ]"
  _assert_json_parseable "$CHECKPOINT_PATH"
  store_clear
  checkpoint_load
  assertEquals 'load name' 'X' "$(store_get business.name)"
  assertEquals 'load country' 'AR' "$(store_get business.country)"
}

# Task 2.4: R/S/Q menu and REQ-IS-9 gate -------------------------------------

test_rsq_start_over_deletes_checkpoint() {
  mkdir -p "$CONFIG_DIR"
  printf '{\n  "version": 1,\n  "business.name": "X"\n}\n' > "$CHECKPOINT_PATH"
  printf 'S\n' | _run_install >/dev/null 2>&1
  assertFalse 'checkpoint deleted' "[ -f \"$CHECKPOINT_PATH\" ]"
}

test_rsq_quit_preserves_checkpoint() {
  mkdir -p "$CONFIG_DIR"
  printf '{\n  "version": 1,\n  "business.name": "X"\n}\n' > "$CHECKPOINT_PATH"
  printf 'Q\n' | _run_install >/dev/null 2>&1
  assertEquals 'quit exit' 0 "$?"
  assertTrue 'checkpoint preserved' "[ -f \"$CHECKPOINT_PATH\" ]"
}

test_reconfigure_decline_leaves_files() {
  mkdir -p "$SETUP_DIR"
  printf '{}' > "$SETUP_DIR/setup_business.json"
  printf '{}' > "$SETUP_DIR/setup_staff.json"
  printf '{}' > "$SETUP_DIR/setup_services.json"
  printf 'N\n' | _run_install >/dev/null 2>&1
  assertEquals 'decline exit' 0 "$?"
  assertTrue 'business untouched' "[ -f \"$SETUP_DIR/setup_business.json\" ]"
  assertFalse 'no checkpoint' "[ -f \"$CHECKPOINT_PATH\" ]"
}

# Task 2.5 / 2.6: prompt engine + business section (PR2a happy path) ---------

test_fresh_happy_path() {
  local out rc field
  out=$(_business_input | _run_install 2>&1)
  rc=$?
  assertEquals 'business-only exit' 0 "$rc"
  assertTrue 'stub message' "printf '%s\n' \"$out\" | grep -q 'PR2b'"
  assertTrue 'checkpoint kept' "[ -f \"$CHECKPOINT_PATH\" ]"
  _assert_json_parseable "$CHECKPOINT_PATH"
  for field in "${BP_KEYS[@]}"; do
    assertTrue "business.$field captured" "grep -q \"business.$field\" \"$CHECKPOINT_PATH\""
  done
  assertFalse 'no hours keys' "grep -q 'hours\.' \"$CHECKPOINT_PATH\""
  assertFalse 'no staff keys' "grep -q 'staff\.' \"$CHECKPOINT_PATH\""
  assertFalse 'no services keys' "grep -q 'services\.' \"$CHECKPOINT_PATH\""
}

test_invalid_email_reasks() {
  local out rc
  out=$( (_business_input | head -n 10; printf 'no-arroba\nhola@latel.com\n') | _run_install 2>&1)
  rc=$?
  assertNotEquals 'email retry non-zero' 0 "$rc"
  assertTrue 'spanish error' "printf '%s' \"$out\" | grep -q 'correo electrónico'"
  assertTrue 'email restored' "grep -q 'hola@latel.com' \"$CHECKPOINT_PATH\""
  assertFalse 'website not set as email' "grep -q 'website_url.*hola@latel.com' \"$CHECKPOINT_PATH\""
}

test_blank_optional_and_defaults() {
  _business_input | _run_install >/dev/null 2>&1
  assertTrue 'website null' "grep -q '\"business.website_url\": null' \"$CHECKPOINT_PATH\""
  assertTrue 'currency default ARS' "grep -q '\"business.currency_code\": \"ARS\"' \"$CHECKPOINT_PATH\""
  assertTrue 'country uppercase' "grep -q '\"business.country\": \"AR\"' \"$CHECKPOINT_PATH\""
}

# TODO PR2b: re-add the 2.7-2.13 suites from /tmp/e2e-pr2-full.sh once
# run_hours_section, run_staff_section, run_services_section, render_setup_*,
# finalize, run_summary_confirm and resume land in PR2b:
#   - test_hours_validation
#   - test_resume_mid_staff
#   - test_resume_revalidates_tampered_email
#   - full-flow happy path assertions over setup_business/staff/services.json

. "$(dirname "$0")/lib/shunit2" || exit 1
