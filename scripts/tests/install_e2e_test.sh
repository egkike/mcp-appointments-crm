#!/bin/bash
# E2E and integration tests for install.sh interactive flow (PR2b: tasks 2.7-2.13).

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

_hours_input() {
  printf '%s\n' \
    "09:00-18:00" \
    "09:00-18:00" \
    "09:00-18:00" \
    "09:00-18:00" \
    "09:00-18:00" \
    "09:00-13:00" \
    "cerrado"
}

_staff_input() {
  printf '%s\n' \
    "Ana" \
    "Estilista" \
    "+5491122334455" \
    "ana@example.com" \
    "09:00-18:00" \
    "09:00-18:00" \
    "09:00-18:00" \
    "09:00-18:00" \
    "09:00-18:00" \
    "no" \
    "no" \
    ""
}

_services_input() {
  printf '%s\n' \
    "Corte ✂️ clásico" \
    "" \
    "30" \
    "1500" \
    ""
}

_full_input() {
  _business_input
  _hours_input
  _staff_input
  _services_input
  printf 's\n'
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

# Task 2.7 / 2.13: business hours + full happy path --------------------------

test_full_happy_path() {
  local out rc
  out=$(_full_input | _run_install 2>&1)
  rc=$?
  echo "DEBUG rc=$rc SETUP_DIR=[$SETUP_DIR]" >&2
  ls -la "$SETUP_DIR" >&2 || true
  echo "DEBUG out tail" >&2
  echo "$out" | tail -10 >&2
  assertEquals 'full flow exit' 0 "$rc"
  assertFalse 'checkpoint removed' "[ -f \"$CHECKPOINT_PATH\" ]"
  _assert_json_parseable "$SETUP_DIR/setup_business.json"
  _assert_json_parseable "$SETUP_DIR/setup_staff.json"
  _assert_json_parseable "$SETUP_DIR/setup_services.json"
  assertFalse 'no id in business' "grep -q '\\\"id\\\"' \"$SETUP_DIR/setup_business.json\""
  assertFalse 'no id in staff' "grep -q '\\\"id\\\"' \"$SETUP_DIR/setup_staff.json\""
  assertFalse 'no id in services' "grep -q '\\\"id\\\"' \"$SETUP_DIR/setup_services.json\""
  assertTrue 'business_hours present' "grep -q 'business_hours' \"$SETUP_DIR/setup_business.json\""
  assertTrue 'saturday open' "grep -q 'saturday' \"$SETUP_DIR/setup_business.json\""
  assertTrue 'sunday null' "grep -q '\"sunday\": null' \"$SETUP_DIR/setup_business.json\""
  assertTrue 'staff schedule day 1' "grep -q '\"day_of_week\": 1' \"$SETUP_DIR/setup_staff.json\""
  assertTrue 'staff status active' "grep -q '\"status\": \"active\"' \"$SETUP_DIR/setup_staff.json\""
  assertTrue 'service is_active 1' "grep -q '\"is_active\": 1' \"$SETUP_DIR/setup_services.json\""
  assertTrue 'service unicode' "grep -q 'Corte' \"$SETUP_DIR/setup_services.json\""
  assertEquals 'setup dir mode' '700' "$(stat -c %a "$SETUP_DIR")"
  assertEquals 'business file mode' '600' "$(stat -c %a "$SETUP_DIR/setup_business.json")"
}

test_hours_malformed_reasks() {
  local out rc
  out=$({ _business_input; printf '9:00 AM\n09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-13:00\ncerrado\n'; _staff_input; _services_input; printf 's\n'; } | _run_install 2>&1)
  rc=$?
  assertEquals 'exit after full flow' 0 "$rc"
  assertTrue 'spanish hhmm error' "printf '%s' \"$out\" | grep -q 'HH:MM'"
  assertTrue 'monday open captured' "grep -q '\"monday\": {\"open\": \"09:00\"' \"$SETUP_DIR/setup_business.json\""
}

test_hours_inverted_range_reasks() {
  local out rc
  out=$({ _business_input; printf '18:00-09:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-13:00\ncerrado\n'; _staff_input; _services_input; printf 's\n'; } | _run_install 2>&1)
  rc=$?
  assertEquals 'exit after full flow' 0 "$rc"
  assertTrue 'spanish range error' "printf '%s' \"$out\" | grep -q 'anterior'"
}

# Task 2.5 / 2.6: prompt engine + business profile ---------------------------

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

# Task 2.8: staff section -----------------------------------------------------

test_staff_one_professional() {
  local out rc count
  out=$({ _business_input; _hours_input; _staff_input; _services_input; printf 's\n'; } | _run_install 2>&1)
  rc=$?
  assertEquals 'exit' 0 "$rc"
  count=$(grep -c '"day_of_week"' "$SETUP_DIR/setup_staff.json")
  assertTrue 'staff name' 'grep -q "\\\"name\\\": \\\"Ana\\\"" "$SETUP_DIR/setup_staff.json"'
  assertEquals 'staff 5 schedule entries' 5 "$count"
  assertFalse 'no sunday schedule' 'grep -q "\\\"day_of_week\\\": 0" "$SETUP_DIR/setup_staff.json"'
  assertFalse 'no saturday schedule' 'grep -q "\\\"day_of_week\\\": 6" "$SETUP_DIR/setup_staff.json"'
}

test_staff_zero_refused() {
  local out rc
  out=$({ _business_input; _hours_input; printf '\nAna\n\n\n\n09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\nno\nno\n\n'; _services_input; printf 's\n'; } | _run_install 2>&1)
  rc=$?
  assertEquals 'exit' 0 "$rc"
  assertTrue 'refusal message' "printf '%s' \"$out\" | grep -q 'al menos un profesional'"
}

test_staff_invalid_schedule_reasks() {
  local out rc
  out=$({ _business_input; _hours_input; printf 'Ana\n\n\n\n18:00-09:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\nno\nno\n\n'; _services_input; printf 's\n'; } | _run_install 2>&1)
  rc=$?
  assertEquals 'exit' 0 "$rc"
  assertTrue 'schedule error' "printf '%s' \"$out\" | grep -q 'anterior'"
}

# Task 2.9: services section --------------------------------------------------

test_services_one_service() {
  local out rc
  out=$({ _business_input; _hours_input; _staff_input; _services_input; printf 's\n'; } | _run_install 2>&1)
  rc=$?
  assertEquals 'exit' 0 "$rc"
  assertTrue 'service name' "grep -q 'Corte' \"$SETUP_DIR/setup_services.json\""
  assertTrue 'price' "grep -q '\"price\": 1500' \"$SETUP_DIR/setup_services.json\""
}

test_services_zero_refused() {
  local out rc
  out=$({ _business_input; _hours_input; _staff_input; printf '\nCorte\n\n30\n1500\n\n'; printf 's\n'; } | _run_install 2>&1)
  rc=$?
  assertEquals 'exit' 0 "$rc"
  assertTrue 'refusal message' "printf '%s' \"$out\" | grep -q 'al menos un servicio'"
}

test_services_negative_price_reasks() {
  local out rc
  out=$({ _business_input; _hours_input; _staff_input; printf 'Corte\n\n30\n-100\n1500\n\n'; printf 's\n'; } | _run_install 2>&1)
  rc=$?
  assertEquals 'exit' 0 "$rc"
  assertTrue 'price error' "printf '%s' \"$out\" | grep -q 'precio'"
}

test_services_zero_duration_reasks() {
  local out rc
  out=$({ _business_input; _hours_input; _staff_input; printf 'Corte\n\n0\n30\n1500\n\n'; printf 's\n'; } | _run_install 2>&1)
  rc=$?
  assertEquals 'exit' 0 "$rc"
  assertTrue 'duration error' "printf '%s' \"$out\" | grep -q 'mayor a cero'"
}

# Task 2.10 / 2.11: summary + finalize ---------------------------------------

test_summary_decline_preserves_checkpoint() {
  local out rc
  out=$({ _business_input; _hours_input; _staff_input; _services_input; printf 'n\n'; } | _run_install 2>&1)
  rc=$?
  assertNotEquals 'decline exit non-zero' 0 "$rc"
  assertTrue 'checkpoint preserved' "[ -f \"$CHECKPOINT_PATH\" ]"
  assertFalse 'business json not written' "[ -f \"$SETUP_DIR/setup_business.json\" ]"
}

# Task 2.12: resume -----------------------------------------------------------

test_resume_mid_staff() {
  local out rc
  # First run: complete business + hours + partial staff (name + basic fields + Monday schedule).
  out=$({
    _business_input; _hours_input
    printf 'Ana\nEstilista\n+5491122334455\nana@example.com\n09:00-18:00\n'
  } | _run_install 2>&1)
  rc=$?
  assertNotEquals 'first run cancel' 0 "$rc"
  assertTrue 'checkpoint exists' "[ -f \"$CHECKPOINT_PATH\" ]"
  # Second run: resume; no business/hours answers, only remaining staff days + finish + services + summary.
  out=$({
    printf 'R\n'
    printf '09:00-18:00\n09:00-18:00\n09:00-18:00\n09:00-18:00\nno\nno\n\n'
    _services_input
    printf 's\n'
  } | _run_install 2>&1)
  rc=$?
  assertEquals 'resume exit' 0 "$rc"
  assertFalse 'checkpoint removed' "[ -f \"$CHECKPOINT_PATH\" ]"
  assertTrue 'staff completed' "grep -q '\"name\": \"Ana\"' \"$SETUP_DIR/setup_staff.json\""
}

test_resume_revalidates_tampered_email() {
  local out rc
  mkdir -p "$CONFIG_DIR"
  cat > "$CHECKPOINT_PATH" <<'CP'
{
  "version": 1,
  "business.name": "X",
  "business.industry": "Peluquería",
  "business.country": "AR",
  "business.address": "Calle 123",
  "business.latitude": -34.6,
  "business.longitude": -58.4,
  "business.cover_photo_url": "https://example.com/cover.jpg",
  "business.public_phone": "+5491122334455",
  "business.messenger_platform": "whatsapp",
  "business.messenger_id": "+5491111111111",
  "business.contact_email": "no-arroba"
}
CP
  out=$({
    printf 'R\n'
    printf 'hola@latel.com\n'
    printf '\n\n\n\n\n\n\n\n\n\n\n\n\n\n\n'
    _hours_input
    _staff_input
    _services_input
    printf 's\n'
  } | _run_install 2>&1)
  rc=$?
  assertEquals 'resume exit' 0 "$rc"
  assertTrue 'valid email in json' "grep -q 'hola@latel.com' \"$SETUP_DIR/setup_business.json\""
  assertFalse 'invalid email not in json' "grep -q 'no-arroba' \"$SETUP_DIR/setup_business.json\""
}

test_mid_finalization_resume() {
  local out rc
  mkdir -p "$CONFIG_DIR"
  cat > "$CHECKPOINT_PATH" <<'CP'
{
  "version": 1,
  "business.name": "X",
  "business.industry": "Peluquería",
  "business.country": "AR",
  "business.address": "Calle 123",
  "business.latitude": -34.6,
  "business.longitude": -58.4,
  "business.cover_photo_url": "https://example.com/cover.jpg",
  "business.public_phone": "+5491122334455",
  "business.messenger_platform": "whatsapp",
  "business.messenger_id": "+5491111111111",
  "business.contact_email": "hola@latel.com",
  "business.website_url": null,
  "business.general_description": null,
  "business.accepted_payment_methods": null,
  "business.currency_code": "ARS",
  "business.currency_symbol": "$",
  "business.timezone": "UTC",
  "business.slot_interval_minutes": 30,
  "hours.monday.open": "09:00",
  "hours.monday.close": "18:00",
  "hours.tuesday.open": null,
  "hours.wednesday.open": null,
  "hours.thursday.open": null,
  "hours.friday.open": null,
  "hours.saturday.open": null,
  "hours.sunday.open": null,
  "staff.0.name": "Ana",
  "staff.0.role_specialty": null,
  "staff.0.phone": null,
  "staff.0.email": null,
  "staff.0.sched.0.open": null,
  "staff.0.sched.1.open": null,
  "staff.0.sched.2.open": null,
  "staff.0.sched.3.open": null,
  "staff.0.sched.4.open": null,
  "staff.0.sched.5.open": null,
  "staff.0.sched.6.open": null,
  "services.0.name": "Corte",
  "services.0.description": null,
  "services.0.duration_minutes": 30,
  "services.0.price": 1500
}
CP
  mkdir -p "$SETUP_DIR"
  printf '{}' > "$SETUP_DIR/setup_business.json"
  out=$({
    printf 'R\n\n\ns\n'
  } | _run_install 2>&1)
  rc=$?
  assertEquals 'resume finalize exit' 0 "$rc"
  assertFalse 'checkpoint removed' "[ -f \"$CHECKPOINT_PATH\" ]"
  _assert_json_parseable "$SETUP_DIR/setup_business.json"
  _assert_json_parseable "$SETUP_DIR/setup_staff.json"
  _assert_json_parseable "$SETUP_DIR/setup_services.json"
}

. "$(dirname "$0")/lib/shunit2" || exit 1
