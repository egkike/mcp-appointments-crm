#!/bin/bash
# OQ-5: specialties pass at the end of the setup wizard + render in
# setup_staff.json (change fix-v081-skill-seed-oq5).

set -u

. "$(dirname "$0")/../install.sh" || exit 1

setUp() {
  TEST_HOME=$(mktemp -d)
  export HOME="$TEST_HOME"
  export XDG_CONFIG_HOME="$TEST_HOME/.config"
  resolve_paths >/dev/null 2>&1
  mkdir -p "$CONFIG_DIR"
  store_clear
  SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
}

tearDown() {
  rm -rf "$TEST_HOME"
}

_seed_services() {
  store_set "services.0.name" "Lavado de Cabello"
  store_set "services.1.name" "Corte de Cabello"
  store_set "services.2.name" "Corte de barba"
}

_seed_staff() {
  store_set "staff.0.name" "Romina"
  store_set "staff.1.name" "Santiago"
}

# --- prompt_staff_specialties -------------------------------------------------

test_specialties_by_numbers() {
  _seed_services; _seed_staff
  prompt_staff_specialties 0 <  <(printf '2 1\n')
  assertEquals 0 $?
  assertEquals 'Corte de Cabello' "$(store_get "staff.0.spec.0")"
  assertEquals 'Lavado de Cabello' "$(store_get "staff.0.spec.1")"
  assertEquals 'specset marker' '1' "$(store_get "staff.0.specset")"
}

test_specialties_by_names_with_commas() {
  _seed_services; _seed_staff
  prompt_staff_specialties 0 <  <(printf 'Corte de barba, Corte de Cabello\n')
  assertEquals 0 $?
  assertEquals 'Corte de barba' "$(store_get "staff.0.spec.0")"
  assertEquals 'Corte de Cabello' "$(store_get "staff.0.spec.1")"
}

test_specialties_dedup_preserves_order() {
  _seed_services; _seed_staff
  prompt_staff_specialties 0 <  <(printf '1 1 2\n')
  assertEquals 0 $?
  assertEquals 'first kept' 'Lavado de Cabello' "$(store_get "staff.0.spec.0")"
  assertEquals 'second different pick' 'Corte de Cabello' "$(store_get "staff.0.spec.1")"
  assertFalse 'no third pick stored' "store_has 'staff.0.spec.2'"
}

test_specialties_invalid_reprompts() {
  _seed_services; _seed_staff
  prompt_staff_specialties 0 <  <(printf '9\nzzz brave\n2\n')
  assertEquals 0 $?
  assertEquals 'final pick after invalid entries' 'Corte de Cabello' "$(store_get "staff.0.spec.0")"
  assertFalse 'single resolved pick' "store_has 'staff.0.spec.1'"
}

test_specialties_empty_is_allowed() {
  _seed_services; _seed_staff
  local rc=0
  prompt_staff_specialties 0 <  <(printf '\n') > "$TEST_HOME/out.txt" 2>&1 || rc=$?
  assertEquals 0 "$rc"
  assertTrue 'explicit aviso' "grep -q 'update_professional' \"$TEST_HOME/out.txt\""
  assertEquals 'specset marker set' '1' "$(store_get 'staff.0.specset')"
  assertFalse 'no spec stored' "store_has 'staff.0.spec.0'"
}

test_specialties_no_services_skips_prompt() {
  _seed_staff
  local rc=0
  prompt_staff_specialties 0 <  <(printf 'no-deberia-leer-esto\n') > "$TEST_HOME/out.txt" 2>&1 || rc=$?
  assertEquals 0 "$rc"
  # Without services the prompt is skipped entirely.
  assertFalse 'no prompt (no services)' "grep -q 'Servicios del negocio' \"$TEST_HOME/out.txt\""
  assertEquals 'marker set silently' '1' "$(store_get 'staff.0.specset')"
  assertFalse 'leftover marker is not a spec' "store_has 'staff.0.spec.0'"
}

test_specialties_marker_resume_is_idempotent() {
  _seed_services; _seed_staff
  store_set "staff.0.specset" "1"
  local out="$TEST_HOME/out.txt"
  prompt_staff_specialties 0 <  <(printf 'este-input-no-se-debe-leer\n') > "$out" 2>&1
  assertFalse 'already answered: no prompt' "grep -q 'Servicios del negocio' \"$out\""
}

# --- run_staff_specialties_pass ----------------------------------------------

test_pass_iterates_all_staff() {
  _seed_services; _seed_staff
  run_staff_specialties_pass <  <(printf '1\n2\n')
  assertEquals 0 $?
  assertEquals 'Lavado de Cabello' "$(store_get 'staff.0.spec.0')"
  assertEquals 'Corte de Cabello' "$(store_get 'staff.1.spec.0')"
}

test_pass_without_staff_is_noop() {
  _seed_services
  local rc=0
  run_staff_specialties_pass <  <(printf 'X\n') > "$TEST_HOME/out.txt" 2>&1 || rc=$?
  assertEquals 0 "$rc"
  assertFalse 'no prompts at all' "grep -q 'Servicios del negocio' \"$TEST_HOME/out.txt\""
  assertFalse 'nothing stored' "store_has 'staff.0.specset'"
}

# --- render_setup_staff ------------------------------------------------------

test_render_emits_specialties_names() {
  _seed_services; _seed_staff
  store_set "staff.0.spec.0" "Lavado de Cabello"
  store_set "staff.0.spec.1" 'Corte "con" comillas'
  store_set "staff.1.spec.0" "Corte de Cabello"
  local out ok
  out=$(render_setup_staff)
  ok=0; printf '%s' "$out" | grep -qF '"specialties": ["Lavado de Cabello"' || ok=1
  assertTrue 'romina has picked names' "$ok"
  ok=0; printf '%s' "$out" | grep -qF 'Corte \"con\" comillas' || ok=1
  assertTrue 'json-escaped quotes kept' "$ok"
  ok=0; printf '%s' "$out" | grep -qF '"specialties": ["Corte de Cabello"]' || ok=1
  assertTrue 'santiago keeps one name' "$ok"
  ok=0; printf '%s' "$out" | grep -qF '"specialties": []' && ok=1
  assertTrue 'no empty array left' "$ok"
}

test_render_specialties_empty_by_default() {
  _seed_staff
  local out ok
  out=$(render_setup_staff)
  ok=0; printf '%s' "$out" | grep -qF '"specialties": []' || ok=1
  assertTrue 'legacy empty array preserved' "$ok"
}

. "$(dirname "$0")/lib/shunit2" || exit 1
