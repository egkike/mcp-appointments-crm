#!/bin/bash
# Tests for the receptionist skill install step in scripts/install.sh
# (TASK-4.2 / ADR-0020 Decision 1): rendering of {business_name}, atomic 0600
# write, modified-file protection, non-fatal failure and HERMES_HOME override.

set -u

. "$(dirname "$0")/../install.sh" || exit 1

TEMPLATE="$SCRIPT_DIR/../setup/templates/recepcionista-negocio/SKILL.md"

setUp() {
  TEST_HOME=$(mktemp -d)
  export HOME="$TEST_HOME"
  export XDG_CONFIG_HOME="$TEST_HOME/.config"
  export HERMES_HOME="$TEST_HOME/hermes-home"
  unset EXTRACT_DIR
  resolve_paths >/dev/null 2>&1 || true
  store_clear
  SKILL_INSTALL_OUTCOME=""
}

tearDown() {
  rm -rf "$TEST_HOME"
}

_skill_dest() { printf '%s' "$HERMES_HOME/skills/recepcionista-negocio/SKILL.md"; }

# Fresh install: renders the template and writes it 0600 into HERMES_HOME.
test_fresh_install_renders_placeholder() {
  local dest expected
  store_set "business.name" "D'Átelier"
  install_receptionist_skill >/dev/null 2>&1
  dest=$(_skill_dest)
  expected=$(render_skill_template "$TEMPLATE" "D'Átelier")
  assertTrue 'skill file exists' "[ -f \"$dest\" ]"
  assertEquals 'rendered content' "$expected" "$(cat "$dest")"
  assertFalse 'no {business_name} left' "grep -q '{business_name}' \"$dest\""
  assertTrue 'business name present' "grep -q \"D'Átelier\" \"$dest\""
  assertTrue '{nombre} stays runtime var' "grep -q '{nombre}' \"$dest\""
  assertEquals 'file mode 600' '600' "$(stat -c %a "$dest")"
  assertEquals 'outcome installed' 'instalada' "$SKILL_INSTALL_OUTCOME"
}

# The renderer replaces every occurrence of {business_name}, not just the first.
test_render_replaces_every_occurrence() {
  local t out
  t=$(mktemp)
  printf '%s\n' 'A {business_name} B {business_name} C' > "$t"
  out=$(render_skill_template "$t" "Mi Negocio")
  rm -f "$t"
  assertEquals 'all occurrences replaced' 'A Mi Negocio B Mi Negocio C' "$out"
}

# An existing file equal to the rendered template is not rewritten (same inode)
# and is reported as already installed.
test_existing_identical_file_is_not_rewritten() {
  local dest before after
  store_set "business.name" "Negocio"
  install_receptionist_skill >/dev/null 2>&1
  dest=$(_skill_dest)
  before=$(stat -c %i "$dest")
  install_receptionist_skill >/dev/null 2>&1
  after=$(stat -c %i "$dest")
  assertEquals 'outcome already installed' 'ya instalada sin cambios' "$SKILL_INSTALL_OUTCOME"
  assertEquals 'same inode (no rewrite)' "$before" "$after"
}

# A user-modified file is preserved and the Spanish notice points at TUI option 7.
test_existing_modified_file_is_not_overwritten() {
  local dest out_file out
  store_set "business.name" "Negocio"
  dest=$(_skill_dest)
  mkdir -p "${dest%/*}"
  printf 'contenido modificado a mano\n' > "$dest"
  out_file=$(mktemp)
  install_receptionist_skill >"$out_file" 2>&1
  out=$(cat "$out_file")
  rm -f "$out_file"
  assertEquals 'content preserved' 'contenido modificado a mano' "$(cat "$dest")"
  assertEquals 'outcome modified' \
    'modificada por el usuario, no se sobrescribió' "$SKILL_INSTALL_OUTCOME"
  assertTrue 'notice mentions TUI option 7' \
    "printf '%s' \"$out\" | grep -q 'opción 7'"
}

# HERMES_HOME is overridable and the default lives under $HOME/.hermes.
test_hermes_home_override() {
  local dest
  store_set "business.name" "Negocio"
  HERMES_HOME="$TEST_HOME/custom-hermes"
  export HERMES_HOME
  install_receptionist_skill >/dev/null 2>&1
  dest="$HERMES_HOME/skills/recepcionista-negocio/SKILL.md"
  assertTrue 'written under override' "[ -f \"$dest\" ]"
  assertFalse 'not under default home' \
    "[ -f \"$TEST_HOME/.hermes/skills/recepcionista-negocio/SKILL.md\" ]"
}

# A failed write does not abort: outcome is the generic Spanish error.
test_skill_write_failure_is_non_fatal() {
  local rc=0
  store_set "business.name" "Negocio"
  mkdir -p "$HERMES_HOME/skills"
  chmod 500 "$HERMES_HOME/skills"
  install_receptionist_skill >/dev/null 2>&1 || rc=$?
  chmod 700 "$HERMES_HOME/skills"
  assertEquals 'non-fatal return 0' 0 "${rc:-1}"
  assertEquals 'error outcome' 'error: no se pudo escribir' "$SKILL_INSTALL_OUTCOME"
}

# atomic_write must leave a pre-existing destination intact when it cannot write.
test_atomic_write_failure_leaves_original_intact() {
  local tmp dest rc=0
  tmp=$(mktemp -d)
  dest="$tmp/skill.md"
  printf 'original' > "$dest"
  chmod 500 "$tmp"
  printf 'nuevo' | atomic_write "$dest" 2>/dev/null || rc=$?
  chmod 700 "$tmp"
  assertTrue 'write fails on read-only dir' "[ \${rc:-0} -ne 0 ]"
  assertEquals 'original intact' 'original' "$(cat "$dest")"
  rm -rf "$tmp"
}

# The final summary uses the exact "Skill recepcionista: <outcome>" format.
test_summary_line_format() {
  store_set "business.name" "Negocio"
  install_receptionist_skill >/dev/null 2>&1
  assertEquals 'summary line' 'Skill recepcionista: instalada' "$(print_skill_summary)"
}

. "$(dirname "$0")/lib/shunit2" || exit 1
