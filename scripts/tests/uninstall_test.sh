#!/bin/bash
# Tests para scripts/uninstall.sh (feat-install-scope-uninstall, T2 — issue #165).
#
# Sandbox duro: cada test corre con HOME=$(mktemp -d) y los XDG_* sin setear,
# así el layout resuelto es el default del installer y NUNCA se toca el HOME
# real del operador. tearDown borra el sandbox completo.
#
# Golden: scripts/tests/golden/uninstall_dry_run_plan.txt. La comparación
# post-procesa la salida con sed (TEST_HOME -> __HOME__) para que el fixture
# sea determinista. Regeneración explícita:
#   UNINSTALL_GOLDEN_REGEN=1 bash scripts/tests/uninstall_test.sh
# El fixture pina el layout Linux; en macOS el test se saltea (paths launchd).
#
# shunit2 es el vendored de scripts/tests/lib/ (se carga al final).

set -u

SCRIPTS_DIR="$(cd "$(dirname "$0")/.." && pwd)"
UNINSTALL_SH="$SCRIPTS_DIR/uninstall.sh"
GOLDEN_DIR="$(cd "$(dirname "$0")" && pwd)/golden"
GOLDEN_PLAN="$GOLDEN_DIR/uninstall_dry_run_plan.txt"
SKILL_DIR_NAME="recepcionista-negocio"
# PATH_BASE se captura en el primer setUp: ya incluye el directorio de shims de
# shunit2 (oneTimeSetUp/oneTimeTearDown se resuelven por PATH), así que los
# tests no pueden perderlo al reescribir PATH para el sandbox.
PATH_BASE=""

# ---------------------------------------------------------------------------
# Sandbox
# ---------------------------------------------------------------------------

setUp() {
  TEST_HOME=$(mktemp -d)
  # Artefactos del harness (out.txt) fuera de TEST_HOME: el snapshot
  # byte-level del dry-run sólo debe ver lo que escribe el script.
  TEST_TMP=$(mktemp -d)
  OUT_FILE="$TEST_TMP/out.txt"
  export HOME="$TEST_HOME"
  unset XDG_CONFIG_HOME XDG_DATA_HOME XDG_STATE_HOME XDG_BIN_HOME HERMES_HOME
  export FAKE_MAIN_PID=0

  # fakebin: systemctl/launchctl deterministas. El fake nunca toca el service
  # real; `show -p MainPID` reporta FAKE_MAIN_PID (0 = servicio inexistente).
  FAKEBIN="$TEST_HOME/fakebin"
  mkdir -p "$FAKEBIN"
  _write_fake_service_manager
  if [ -z "$PATH_BASE" ]; then
    PATH_BASE="$PATH"
  fi
  PATH="$FAKEBIN:$PATH_BASE"

  _paths
  RUN_RC=0
  return 0
}

tearDown() {
  PATH="$PATH_BASE"
  rm -rf "$TEST_HOME" "$TEST_TMP"
  return 0
}

# _write_fake_service_manager_variant <stop_rc> <is_active_rc>: simula stop
# FALLIDO (rc != 0) y el estado que reporta el re-chequeo posterior.
_write_fake_service_manager_variant() {
  local stop_rc="$1" active_rc="$2"
  cat > "$FAKEBIN/systemctl" <<EOS
#!/bin/bash
case "\${1:-}" in
  --user)
    case "\${2:-}" in
      show) printf 'MainPID=%s\n' "\${FAKE_MAIN_PID:-0}" ;;
      stop) printf 'fallo simulado del stop\n' >&2 ; exit ${stop_rc} ;;
      is-active) exit ${active_rc} ;;
    esac
    ;;
esac
exit 0
EOS
  chmod +x "$FAKEBIN/systemctl"
}

# _write_fake_systemctl_inconclusive: manager PRESENTE pero indeciso
# (`show -p MainPID` no imprime nada), fuerza el fallback de proceso directo.
_write_fake_systemctl_inconclusive() {
  cat > "$FAKEBIN/systemctl" <<'EOS'
#!/bin/bash
case "${1:-}" in
  --user) case "${2:-}" in show) exit 0 ;; esac ;;
esac
exit 0
EOS
  chmod +x "$FAKEBIN/systemctl"
}

# _spawn_fake_writer: proceso vivo llamado "mcp-server" (pgrep -f lo matchea):
# simula un `./mcp-server` lanzado a mano, fuera del service manager.
_spawn_fake_writer() {
  printf '#!/bin/bash\nsleep 60\n' > "$FAKEBIN/mcp-server"
  chmod +x "$FAKEBIN/mcp-server"
  "$FAKEBIN/mcp-server" >/dev/null 2>&1 &
  FAKE_WRITER_PID=$!
  sleep 1
}

# _paths_macos: layout Darwin (CONFIG_DIR == DATA_DIR, plist de launchd).
_paths_macos() {
  P_CONFIG="$HOME/Library/Application Support/MCP Appointments CRM"
  P_DATA="$HOME/Library/Application Support/MCP Appointments CRM"
  P_LOGS="$HOME/Library/Logs/MCP Appointments CRM"
  P_BIN="$HOME/.local/bin"
  P_SERVICE="$HOME/Library/LaunchAgents/com.mcp.appointments.server.plist"
  P_ENV="$HOME/.config/mcp-appointments-crm/.env"
  P_HERMES="$HOME/.hermes"
  P_HERMES_CONFIG="$P_HERMES/config.yaml"
  P_SKILL="$P_HERMES/skills/$SKILL_DIR_NAME"
}

# _force_darwin: shim de `uname` en fakebin para ejercitar la rama macOS desde
# un host Linux (hook de test mínimo; producción no tiene override).
_force_darwin() {
  printf '#!/bin/bash\necho Darwin\n' > "$FAKEBIN/uname"
  chmod +x "$FAKEBIN/uname"
  _paths_macos
}

_write_fake_service_manager() {
  local target
  for target in systemctl launchctl; do
    cat > "$FAKEBIN/$target" <<'EOS'
#!/bin/bash
# fake service manager — determinista. `--user show -p MainPID` reporta
# FAKE_MAIN_PID (default 0); cualquier otro verbo no hace nada y sale 0.
case "${1:-}" in
  --user)
    case "${2:-}" in
      show) printf 'MainPID=%s\n' "${FAKE_MAIN_PID:-0}" ;;
    esac
    ;;
esac
exit 0
EOS
    chmod +x "$FAKEBIN/$target"
  done
}

# _paths: espejo del layout de install.sh resolve_paths()/uninstall.sh. Se
# duplica a propósito: el test verifica el contrato desde afuera, sin sourcear
# el script bajo prueba.
_paths() {
  if [ "$(uname -s)" = "Darwin" ]; then
    P_CONFIG="$HOME/Library/Application Support/MCP Appointments CRM"
    P_DATA="$HOME/Library/Application Support/MCP Appointments CRM"
    P_LOGS="$HOME/Library/Logs/MCP Appointments CRM"
    P_BIN="$HOME/.local/bin"
    P_SERVICE="$HOME/Library/LaunchAgents/com.mcp.appointments.server.plist"
  else
    P_CONFIG="$HOME/.config/mcp-appointments-crm"
    P_DATA="$HOME/.local/share/mcp-appointments-crm"
    P_LOGS="$HOME/.local/state/mcp-appointments-crm"
    P_BIN="$HOME/.local/bin"
    P_SERVICE="$HOME/.config/systemd/user/mcp-appointments-crm.service"
  fi
  # Quirk del installer: el .env vive SIEMPRE en ~/.config en ambos OS.
  P_ENV="$HOME/.config/mcp-appointments-crm/.env"
  P_HERMES="$HOME/.hermes"
  P_HERMES_CONFIG="$P_HERMES/config.yaml"
  P_SKILL="$P_HERMES/skills/$SKILL_DIR_NAME"
}

# ---------------------------------------------------------------------------
# Fixtures
# ---------------------------------------------------------------------------

# _seed_installation: instalación completa. Los -wal/-shm se siembran VACÍOS
# (huella de un cierre limpio) para que --purge-data no dispare el gate de
# handle vivo; el caso "wal no vacío" es un test propio.
_seed_installation() {
  mkdir -p "$(dirname "$P_SERVICE")" "$P_BIN" "$P_LOGS" "$P_DATA/scripts" \
    "$P_DATA/backups" "$P_CONFIG/setup" "$P_CONFIG/templates/$SKILL_DIR_NAME" \
    "$P_CONFIG/backups" "$P_SKILL"

  printf '[Unit]\nDescription=fake unit\n' > "$P_SERVICE"

  printf '#!/bin/bash\necho bin\n' > "$P_BIN/mcp-server"
  chmod +x "$P_BIN/mcp-server"
  printf '#!/bin/bash\necho prev\n' > "$P_BIN/mcp-server.prev"

  printf 'linea de log\n' > "$P_LOGS/mcp-server.log"

  printf 'db-bytes\n' > "$P_DATA/reservas.db"
  : > "$P_DATA/reservas.db-wal"
  : > "$P_DATA/reservas.db-shm"
  printf '#!/bin/bash\necho backup\n' > "$P_DATA/scripts/backup.sh"
  printf 'backup-gz\n' > "$P_DATA/backups/reservas-20260101.db.gz"

  printf 'MCP_BIND=127.0.0.1\nMCP_PORT=3000\n' > "$P_ENV"
  printf 'caller-abc\n' > "$P_CONFIG/caller-id"
  printf 'hmac-key-bytes\n' > "$P_CONFIG/audit-hash-key"
  printf '{"business":"x"}\n' > "$P_CONFIG/setup/setup_business.json"
  printf 'template\n' > "$P_CONFIG/templates/$SKILL_DIR_NAME/SKILL.md"
  printf 'config-backup\n' > "$P_CONFIG/backups/config-20260101.tar.gz"

  printf 'skill-render\n' > "$P_SKILL/SKILL.md"
  _write_hermes_config "$P_HERMES"
}

# _write_hermes_config <hermes_home>: config.yaml con la entrada nuestra, un
# servidor hermano y claves ajenas (top-level y comentarios) alrededor.
_write_hermes_config() {
  local dir="$1"
  mkdir -p "$dir"
  cat > "$dir/config.yaml" <<'YAML'
# Hermes gateway config (operador)
gateway:
  port: 8080

mcp_servers:
  mcp-appointments:
    command: /home/operador/.local/bin/mcp-server
    args:
      - --transport
      - sse
    env:
      MCP_PORT: "3000"
  other-server:
    command: /opt/other-server
    args:
      - --verbose

logging:
  level: info
# cola: comentario del operador al final del archivo
YAML
}

# _write_expected_hermes_config <file>: salida esperada tras quitar la entrada.
# Contrato D3: el bloque se consume desde "  mcp-appointments:" hasta la próxima
# key hermana (indent <= 2); la línea en blanco previa a "  other-server:" está
# dentro del bloque, por eso desaparece. Todo lo demás queda byte-exacto.
_write_expected_hermes_config() {
  cat > "$1" <<'YAML'
# Hermes gateway config (operador)
gateway:
  port: 8080

mcp_servers:
  other-server:
    command: /opt/other-server
    args:
      - --verbose

logging:
  level: info
# cola: comentario del operador al final del archivo
YAML
}

# ---------------------------------------------------------------------------
# Helpers de ejecución / aserción
# ---------------------------------------------------------------------------

# _run [flags...]: corre el uninstall con stdin NO-TTY (< /dev/null) y guarda
# stdout+stderr en $OUT_FILE. Exit code en RUN_RC.
_run() {
  : > "$OUT_FILE"
  RUN_RC=0
  bash "$UNINSTALL_SH" "$@" > "$OUT_FILE" 2>&1 < /dev/null || RUN_RC=$?
}

# _run_env <VAR> <valor> [flags...]: idem _run con una variable exportada
# (se usa para HERMES_HOME).
_run_env() {
  local key="$1" value="$2"
  shift 2
  : > "$OUT_FILE"
  RUN_RC=0
  env "$key=$value" bash "$UNINSTALL_SH" "$@" > "$OUT_FILE" 2>&1 < /dev/null || RUN_RC=$?
}

# _out_grep_fixed: búsqueda literal en la salida (sin regex: los patrones
# llevan corchetes y guiones).
_out_grep_fixed() { grep -q -F -e "$1" "$OUT_FILE"; }

# _rows: sólo las filas de la tabla de artefactos.
_rows() { grep -F -e '  [' "$OUT_FILE"; }

# _non_absent_rows: filas que NO dicen "no existía".
_non_absent_rows() { _rows | grep -v -F -e 'no existía' || true; }

# _row_says_absent <etiqueta>: la fila de ese artefacto reporta "no existía".
_row_says_absent() {
  _rows | grep -F -e "$1" | grep -q -F -e 'no existía'
}

_sha() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# _snapshot: inventario + huella de todo el sandbox (nombres ordenados y hash
# por archivo). Es la evidencia byte-level de que el dry-run no escribió nada.
_snapshot() {
  local f
  (
    cd "$TEST_HOME" || return 1
    find . -mindepth 1 | sort
    find . -type f | sort | while IFS= read -r f; do
      printf '%s  %s\n' "$(_sha "$f")" "$f"
    done
  )
}

# ---------------------------------------------------------------------------
# T2 — dry-run default (D2)
# ---------------------------------------------------------------------------

test_dry_run_default_no_op_byte_evidence() {
  _seed_installation
  local before after
  before=$(_snapshot)

  _run

  after=$(_snapshot)
  assertEquals 'dry-run sin flags termina 0' 0 "$RUN_RC"
  assertEquals 'sandbox byte-identical (nombres + sha256 de cada archivo)' \
    "$before" "$after"
  assertTrue 'el plan imprime filas BORRARÁ' "_out_grep_fixed '[BORRARÁ]'"
  assertTrue 'el plan imprime los preservados' "_out_grep_fixed 'PRESERVARÁ'"
  assertTrue 'el plan imprime a Hermes como INTACTO sin el flag' \
    "_out_grep_fixed 'INTACTO'"
  assertTrue 'el plan invita a --apply' "_out_grep_fixed '--apply'"
  assertTrue 'el plan nombra el modo dry-run' "_out_grep_fixed 'dry-run'"
  # Guía de backup (contrato aditivo): con la copia residente de scripts/ el
  # plan sugiere el comando exacto.
  assertTrue 'el plan imprime el comando de backup sugerido' \
    "_out_grep_fixed 'Comando de backup sugerido: bash'"
  assertTrue 'el comando sugerido apunta al backup.sh residente' \
    "_out_grep_fixed '$P_DATA/scripts/backup.sh'"
  assertTrue 'el plan aclara que backups/ se preserva siempre' \
    "_out_grep_fixed 'se preserva siempre'"
}

test_golden_dry_run_plan() {
  if [ "$(uname -s)" = "Darwin" ]; then
    # El fixture pina el layout Linux (systemd --user, XDG). En macOS el plan
    # es otro por contrato; se saltea explícito en vez de fingir verde.
    startSkipping
  fi

  _seed_installation
  _run
  assertEquals 'dry-run termina 0' 0 "$RUN_RC"

  sed "s|$TEST_HOME|__HOME__|g" "$OUT_FILE" > "$TEST_TMP/plan.norm"

  if [ "${UNINSTALL_GOLDEN_REGEN:-0}" = "1" ]; then
    mkdir -p "$GOLDEN_DIR"
    cp "$TEST_TMP/plan.norm" "$GOLDEN_PLAN"
  fi

  if ! cmp -s "$TEST_TMP/plan.norm" "$GOLDEN_PLAN"; then
    diff -u "$GOLDEN_PLAN" "$TEST_TMP/plan.norm" >&2 || true
  fi
  assertTrue 'el plan (post-procesado) es byte-idéntico al golden' \
    "cmp -s '$TEST_TMP/plan.norm' '$GOLDEN_PLAN'"

  if [ "$(uname -s)" = "Darwin" ]; then
    endSkipping
  fi
}

# ---------------------------------------------------------------------------
# T2 — --apply (D2)
# ---------------------------------------------------------------------------

test_apply_removes_service_binary_logs_keep_data() {
  _seed_installation
  local env_sha caller_sha key_sha setup_sha db_sha backups_sha
  env_sha=$(_sha "$P_ENV")
  caller_sha=$(_sha "$P_CONFIG/caller-id")
  key_sha=$(_sha "$P_CONFIG/audit-hash-key")
  setup_sha=$(_sha "$P_CONFIG/setup/setup_business.json")
  db_sha=$(_sha "$P_DATA/reservas.db")
  backups_sha=$(_sha "$P_DATA/backups/reservas-20260101.db.gz")

  # Este sandbox modela un host SIN la copia residente del backup
  # ($DATA_DIR/scripts/backup.sh borrado por el operador): la guía de backup
  # debe caer al fallback manual, que es el camino que queda cuando la copia ya
  # no está.
  rm -f "$P_DATA/scripts/backup.sh"

  _run --apply

  assertEquals 'apply termina 0' 0 "$RUN_RC"
  assertTrue 'service unit removida' "[ ! -e '$P_SERVICE' ]"
  assertTrue 'binario removido' "[ ! -e '$P_BIN/mcp-server' ]"
  assertTrue 'binario previo removido' "[ ! -e '$P_BIN/mcp-server.prev' ]"
  assertTrue 'logs removidos' "[ ! -e '$P_LOGS' ]"
  assertTrue 'templates/ del config removido' "[ ! -e '$P_CONFIG/templates' ]"

  assertEquals '.env intacto (hash)' "$env_sha" "$(_sha "$P_ENV")"
  assertEquals 'caller-id intacto (hash)' "$caller_sha" "$(_sha "$P_CONFIG/caller-id")"
  assertEquals 'audit-hash-key intacto (hash)' "$key_sha" "$(_sha "$P_CONFIG/audit-hash-key")"
  assertEquals 'setup/*.json intacto (hash)' "$setup_sha" "$(_sha "$P_CONFIG/setup/setup_business.json")"
  assertEquals 'DB intacta (hash)' "$db_sha" "$(_sha "$P_DATA/reservas.db")"
  assertEquals 'backups/ intacto (hash)' "$backups_sha" "$(_sha "$P_DATA/backups/reservas-20260101.db.gz")"

  assertTrue 'la tabla reporta removido' "_out_grep_fixed '[removido]'"
  assertTrue 'la tabla reporta preservado por decisión' \
    "_out_grep_fixed 'preservado por decisión'"
  assertTrue 'cierra con estado OK' "_out_grep_fixed 'Estado final: OK'"
  # Guía de backup, rama fallback: sin la copia residente no hay comando que
  # sugerir, así que lista los tres archivos a copiar a mano.
  assertTrue 'sin copia residente imprime el fallback manual' \
    "_out_grep_fixed 'Rollback manual sin ese script:'"
  assertTrue 'no sugiere un script que no existe' \
    "! _out_grep_fixed 'Comando de backup sugerido: bash'"
  assertTrue 'aclara que los .gz de backups/ no se tocan' \
    "_out_grep_fixed 'no se tocan con ningún flag'"
}

test_double_run_idempotent() {
  _seed_installation
  _run --apply
  assertEquals 'primer apply termina 0' 0 "$RUN_RC"

  _run --apply
  assertEquals 'segundo apply termina 0' 0 "$RUN_RC"
  assertTrue 'service unit no reaparece' "[ ! -e '$P_SERVICE' ]"
  assertTrue 'binario no reaparece' "[ ! -e '$P_BIN/mcp-server' ]"
  assertTrue 'logs no recreados' "[ ! -e '$P_LOGS' ]"
  assertTrue 'no hay filas removido' "! _out_grep_fixed '[removido]'"
  assertTrue 'no hay filas PURGADO' "! _out_grep_fixed '[PURGADO]'"
  assertTrue 'fila servicio: no existía' "_row_says_absent 'servicio'"
  assertTrue 'fila binario: no existía' "_row_says_absent 'binario:'"
  assertTrue 'fila binario previo: no existía' "_row_says_absent 'binario previo'"
  assertTrue 'fila logs: no existía' "_row_says_absent 'logs:'"
  assertTrue 'fila skill Hermes: no existía' "_row_says_absent 'skill Hermes'"
  assertTrue 'fila config templates/: no existía' \
    "_row_says_absent 'config templates/'"
}

# ---------------------------------------------------------------------------
# T2/S4 — --purge-data y su doble gate (D4)
# ---------------------------------------------------------------------------

test_purge_data_requires_double_confirmation() {
  _seed_installation
  local db_sha backups_sha
  db_sha=$(_sha "$P_DATA/reservas.db")
  backups_sha=$(_sha "$P_DATA/backups/reservas-20260101.db.gz")

  # (a) stdin no-TTY sin --yes: aborta nombrando los dos gates y --yes.
  _run --apply --purge-data
  assertNotEquals 'aborta sin --yes' 0 "$RUN_RC"
  assertTrue 'nombra la primera confirmación' \
    "_out_grep_fixed 'primera confirmación'"
  assertTrue 'nombra la ÚLTIMA CONFIRMACIÓN' \
    "_out_grep_fixed 'ÚLTIMA CONFIRMACIÓN'"
  assertTrue 'nombra el --yes requerido' "_out_grep_fixed '--yes'"
  assertEquals 'la DB queda intacta (hash)' "$db_sha" "$(_sha "$P_DATA/reservas.db")"
  assertTrue 'la unit queda intacta' "[ -f '$P_SERVICE' ]"

  # (b) --yes en stdin no interactivo satisface ambos gates.
  _run --apply --purge-data --yes
  assertEquals 'apply --purge-data --yes termina 0' 0 "$RUN_RC"
  assertTrue 'DB purgada' "[ ! -e '$P_DATA/reservas.db' ]"
  assertTrue 'WAL purgado' "[ ! -e '$P_DATA/reservas.db-wal' ]"
  assertTrue 'SHM purgado' "[ ! -e '$P_DATA/reservas.db-shm' ]"
  assertTrue 'copia de scripts/ del data dir purgada' "[ ! -e '$P_DATA/scripts' ]"
  assertEquals 'backups/ del data dir intacto (hash)' \
    "$backups_sha" "$(_sha "$P_DATA/backups/reservas-20260101.db.gz")"
  assertTrue 'la tabla reporta PURGADO' "_out_grep_fixed '[PURGADO]'"
  assertTrue 'backups/ impreso como preservado por decisión' \
    "_out_grep_fixed 'backups/'"
}

test_purge_data_live_db_fail_closed() {
  _seed_installation
  sleep 60 &
  local live_pid=$!
  export FAKE_MAIN_PID="$live_pid"

  _run --apply --purge-data --yes

  export FAKE_MAIN_PID=0
  kill "$live_pid" 2>/dev/null || true

  assertNotEquals 'aborta fail-closed' 0 "$RUN_RC"
  assertTrue 'el mensaje nombra el MainPID vivo' \
    "_out_grep_fixed 'MainPID=$live_pid'"
  assertTrue 'cero borrados: unit intacta' "[ -f '$P_SERVICE' ]"
  assertTrue 'cero borrados: binario intacto' "[ -f '$P_BIN/mcp-server' ]"
  assertTrue 'cero borrados: DB intacta' "[ -f '$P_DATA/reservas.db' ]"
  assertTrue 'cero borrados: logs intactos' "[ -d '$P_LOGS' ]"
}

test_purge_data_nonempty_wal_fail_closed() {
  _seed_installation
  printf 'wal-con-handle-abierto\n' > "$P_DATA/reservas.db-wal"

  _run --apply --purge-data --yes

  assertNotEquals 'aborta fail-closed por WAL no vacío' 0 "$RUN_RC"
  assertTrue 'nombra el archivo -wal' "_out_grep_fixed 'reservas.db-wal'"
  assertTrue 'la DB sigue ahí' "[ -f '$P_DATA/reservas.db' ]"
}

test_purge_data_manager_inconclusive_pgrep_writer_fail_closed() {
  # R3-001: manager indeciso + writer vivo fuera de systemd => fail-closed.
  _seed_installation
  _write_fake_systemctl_inconclusive
  _spawn_fake_writer

  _run --apply --purge-data --yes

  kill "$FAKE_WRITER_PID" 2>/dev/null || true

  assertNotEquals 'aborta fail-closed' 0 "$RUN_RC"
  assertTrue 'nombra el writer no gestionado' \
    "_out_grep_fixed 'no gestionado por el service manager'"
  assertTrue 'cero borrados: DB intacta' "[ -f '$P_DATA/reservas.db' ]"
  assertTrue 'cero borrados: unit intacta' "[ -f '$P_SERVICE' ]"
}

test_purge_data_manager_missing_pgrep_no_match_allows() {
  # WSL sin systemd: manager ausente + pgrep sin match => la purga procede.
  _seed_installation
  local nobin="$TEST_HOME/nobin"
  local tool
  mkdir -p "$nobin"
  for tool in awk sed grep cat rm mv cp mkdir du find head tail tr uname id \
    dirname basename ls chmod touch wc sort cut pgrep; do
    ln -sf "$(command -v "$tool")" "$nobin/$tool"
  done

  : > "$OUT_FILE"
  RUN_RC=0
  PATH="$nobin" /bin/bash "$UNINSTALL_SH" --apply --purge-data --yes \
    > "$OUT_FILE" 2>&1 < /dev/null || RUN_RC=$?
  PATH="$PATH_BASE"

  assertEquals 'sin systemctl y sin writer termina 0' 0 "$RUN_RC"
  assertTrue 'DB purgada' "[ ! -e '$P_DATA/reservas.db' ]"
  assertTrue 'backups/ preservado' "[ -d '$P_DATA/backups' ]"
}

# ---------------------------------------------------------------------------
# T2/S4 — --purge-config y audit-hash-key (OQ-4)
# ---------------------------------------------------------------------------

test_purge_config_removes_keyfile_with_hmac_warning() {
  _seed_installation
  local backups_sha
  backups_sha=$(_sha "$P_DATA/backups/reservas-20260101.db.gz")

  _run --apply --purge-config --purge-data --yes

  assertEquals 'apply --purge-config --purge-data --yes termina 0' 0 "$RUN_RC"
  assertTrue 'config dir eliminado entero' "[ ! -e '$P_CONFIG' ]"
  assertTrue 'audit-hash-key eliminado' "[ ! -e '$P_CONFIG/audit-hash-key' ]"
  assertTrue '.env eliminado' "[ ! -e '$P_ENV' ]"
  assertTrue 'caller-id eliminado' "[ ! -e '$P_CONFIG/caller-id' ]"
  assertTrue 'setup/ eliminado' "[ ! -e '$P_CONFIG/setup' ]"
  assertTrue 'advertencia HMAC impresa' \
    "_out_grep_fixed 'invalida la auditoría histórica del HMAC'"
  assertTrue 'DB purgada' "[ ! -e '$P_DATA/reservas.db' ]"
  assertEquals 'backups/ del data dir intacto (hash)' \
    "$backups_sha" "$(_sha "$P_DATA/backups/reservas-20260101.db.gz")"
}

# ---------------------------------------------------------------------------
# T2/S3 — --purge-hermes-entry (byte-clean, idempotente)
# ---------------------------------------------------------------------------

test_purge_hermes_entry_byte_clean_siblings_survive() {
  _seed_installation
  local custom="$TEST_HOME/hermes-custom"
  local home_hermes_sha before
  _write_hermes_config "$custom"
  _write_expected_hermes_config "$TEST_TMP/expected.yaml"
  home_hermes_sha=$(_sha "$P_HERMES_CONFIG")
  before=$(_sha "$custom/config.yaml")

  _run_env HERMES_HOME "$custom" --apply --purge-hermes-entry

  assertEquals 'apply --purge-hermes-entry termina 0' 0 "$RUN_RC"
  assertNotEquals 'el config fue reescrito' "$before" "$(_sha "$custom/config.yaml")"
  if ! cmp -s "$custom/config.yaml" "$TEST_TMP/expected.yaml"; then
    diff -u "$TEST_TMP/expected.yaml" "$custom/config.yaml" >&2 || true
  fi
  assertTrue 'sibling + claves ajenas byte-exactos vs esperado' \
    "cmp -s '$custom/config.yaml' '$TEST_TMP/expected.yaml'"
  assertTrue 'el servidor hermano sobrevive' \
    "grep -q -F -e '  other-server:' '$custom/config.yaml'"
  assertTrue 'la key mcp_servers: sigue existiendo (tenía otro hijo)' \
    "grep -q -F -e 'mcp_servers:' '$custom/config.yaml'"
  assertTrue 'la entrada Hermes ya no está' \
    "! grep -q -F -e 'mcp-appointments' '$custom/config.yaml'"
  assertEquals 'el ~/.hermes del HOME queda intacto (HERMES_HOME respetado)' \
    "$home_hermes_sha" "$(_sha "$P_HERMES_CONFIG")"
  assertTrue 'la tabla reporta la entrada Hermes como removida' \
    "_out_grep_fixed 'entrada Hermes mcp_servers.mcp-appointments'"
}

test_purge_hermes_entry_childless_drops_servers_key() {
  local custom="$TEST_HOME/hermes-solo"
  mkdir -p "$custom"
  printf 'gateway:\n  port: 8080\n\nmcp_servers:\n  mcp-appointments:\n    command: /x\n    env:\n      A: "1"\n\nlogging:\n  level: info\n' \
    > "$custom/config.yaml"
  printf 'gateway:\n  port: 8080\n\nlogging:\n  level: info\n' \
    > "$TEST_TMP/expected.yaml"

  _run_env HERMES_HOME "$custom" --apply --purge-hermes-entry

  assertEquals 'termina 0' 0 "$RUN_RC"
  if ! cmp -s "$custom/config.yaml" "$TEST_TMP/expected.yaml"; then
    diff -u "$TEST_TMP/expected.yaml" "$custom/config.yaml" >&2 || true
  fi
  assertTrue 'mcp_servers: sin hijos se elimina junto con la entrada' \
    "cmp -s '$custom/config.yaml' '$TEST_TMP/expected.yaml'"
}

test_purge_hermes_entry_idempotent_noop() {
  _seed_installation
  _run --apply --purge-hermes-entry
  assertEquals 'primer apply termina 0' 0 "$RUN_RC"

  local cfg_sha
  cfg_sha=$(_sha "$P_HERMES_CONFIG")

  _run --apply --purge-hermes-entry
  assertEquals 'segundo apply termina 0' 0 "$RUN_RC"
  assertEquals 'config.yaml byte-idéntico (no se reescribe)' \
    "$cfg_sha" "$(_sha "$P_HERMES_CONFIG")"
  assertTrue 'la fila reporta no existía' "_out_grep_fixed 'no existía'"
  assertTrue 'no hay filas removido en el segundo run' \
    "! _out_grep_fixed '[removido]'"
}

test_purge_hermes_entry_config_absent() {
  _seed_installation
  rm -f "$P_HERMES_CONFIG"

  _run --apply --purge-hermes-entry

  assertEquals 'sin config.yaml termina 0 (no-op)' 0 "$RUN_RC"
  assertTrue 'no se crea el archivo' "[ ! -e '$P_HERMES_CONFIG' ]"
  assertTrue 'la fila reporta no existía' "_out_grep_fixed 'no existía'"
}

# ---------------------------------------------------------------------------
# T2/S3 — skill de Hermes: default PURGA (OQ-3), --keep-skill la conserva
# ---------------------------------------------------------------------------

test_keep_skill_preserves_skill() {
  _seed_installation
  local skill_sha
  skill_sha=$(_sha "$P_SKILL/SKILL.md")

  _run --apply --keep-skill
  assertEquals 'apply --keep-skill termina 0' 0 "$RUN_RC"
  assertTrue 'la skill sigue presente' "[ -d '$P_SKILL' ]"
  assertEquals 'la skill queda byte-idéntica' "$skill_sha" "$(_sha "$P_SKILL/SKILL.md")"
  assertTrue 'la fila la marca preservada por decisión' \
    "_out_grep_fixed 'preservado por decisión'"

  _run --apply
  assertEquals 'apply default termina 0' 0 "$RUN_RC"
  assertTrue 'sin --keep-skill la skill se borra (OQ-3)' "[ ! -e '$P_SKILL' ]"
}

# ---------------------------------------------------------------------------
# Corrección nativa (review-c8f016c62464c5ea) — R1/R3: backups en macOS y
# stop fallido fail-closed
# ---------------------------------------------------------------------------

test_purge_config_macos_preserves_data_backups() {
  # macOS: config dir == data dir, así que --purge-config podía borrar el
  # backups/ del operador (R1/R3-macos-purge-deletes-data). No debe.
  _force_darwin
  _seed_installation
  local backups_sha db_sha
  backups_sha=$(_sha "$P_DATA/backups/reservas-20260101.db.gz")
  db_sha=$(_sha "$P_DATA/reservas.db")

  _run --apply --purge-config

  assertEquals 'apply --purge-config termina 0' 0 "$RUN_RC"
  assertTrue 'backups/ del data dir sobrevive al purge de config' \
    "[ -d '$P_DATA/backups' ]"
  assertEquals 'el .gz del operador queda byte-idéntico' \
    "$backups_sha" "$(_sha "$P_DATA/backups/reservas-20260101.db.gz")"
  assertEquals 'la DB no se toca sin --purge-data' \
    "$db_sha" "$(_sha "$P_DATA/reservas.db")"
  # El resto de la semántica de --purge-config no cambia.
  assertTrue 'audit-hash-key eliminado' "[ ! -e '$P_CONFIG/audit-hash-key' ]"
  assertTrue '.env eliminado' "[ ! -e '$P_ENV' ]"
  assertTrue 'caller-id eliminado' "[ ! -e '$P_CONFIG/caller-id' ]"
  assertTrue 'advertencia HMAC impresa' \
    "_out_grep_fixed 'invalida la auditoría histórica del HMAC'"
  # Fila de verificación final para backups/ como preservado por decisión.
  assertTrue 'fila de backups/ preservado por decisión' \
    "_rows | grep -F -e 'backups/' | grep -q -F -e 'preservado por decisión'"
}

test_stop_failure_fail_closed() {
  # Stop fallido + servicio activo: prohibido borrar debajo de un runtime vivo.
  _seed_installation
  _write_fake_service_manager_variant 1 0

  _run --apply

  assertNotEquals 'aborta fail-closed' 0 "$RUN_RC"
  assertTrue 'el error nombra el stop fallido' \
    "_out_grep_fixed 'no se pudo detener el servicio'"
  assertTrue 'el error dice que abortó sin borrar nada' \
    "_out_grep_fixed 'abortó sin borrar nada'"
  assertTrue 'la salida del stop fallido se retiene y reporta' \
    "_out_grep_fixed 'fallo simulado del stop'"
  assertTrue 'cero borrados: unit intacta' "[ -f '$P_SERVICE' ]"
  assertTrue 'cero borrados: binario intacto' "[ -f '$P_BIN/mcp-server' ]"
  assertTrue 'cero borrados: logs intactos' "[ -d '$P_LOGS' ]"
  assertTrue 'cero borrados: DB intacta' "[ -f '$P_DATA/reservas.db' ]"
}

test_stop_failure_inactive_continues() {
  # Stop fallido pero ya inactivo: SKIP-safe, advierte y continúa.
  _seed_installation
  _write_fake_service_manager_variant 1 3

  _run --apply

  assertEquals 'continúa y termina 0' 0 "$RUN_RC"
  assertTrue 'advierte el stop fallido sin abortar' \
    "_out_grep_fixed 'Advertencia'"
  assertTrue 'advierte que ya no estaba activo' \
    "_out_grep_fixed 'ya no está activo'"
  assertTrue 'la unit se remueve' "[ ! -e '$P_SERVICE' ]"
}

# ---------------------------------------------------------------------------
# T2 — CLI + robustez
# ---------------------------------------------------------------------------

test_unknown_flag_and_help() {
  _run --flag-inexistente
  assertNotEquals 'flag desconocido sale != 0' 0 "$RUN_RC"
  assertTrue 'imprime el uso' "_out_grep_fixed 'Uso:'"
  assertTrue 'nombra el flag rechazado' "_out_grep_fixed '--flag-inexistente'"

  _run --help
  assertEquals '--help sale 0' 0 "$RUN_RC"
  assertTrue '--help imprime el uso' "_out_grep_fixed 'Uso:'"
}

test_all_absent_noop_exits_zero() {
  # Nada instalado: el uninstall corre igual (OQ-2) y es un no-op idempotente.
  _run --apply

  assertEquals 'sin instalación termina 0' 0 "$RUN_RC"
  assertEquals 'todas las filas dicen no existía' '' "$(_non_absent_rows)"
  assertTrue 'no crea el config dir' "[ ! -e '$P_CONFIG' ]"
  assertTrue 'no crea el binario' "[ ! -e '$P_BIN/mcp-server' ]"
  assertTrue 'no crea logs' "[ ! -e '$P_LOGS' ]"
  assertTrue 'cierra con estado OK' "_out_grep_fixed 'Estado final: OK'"
}

test_no_systemctl_warns_and_continues() {
  # WSL sin systemd: el stop se saltea con advertencia y la desinstalación sigue.
  _seed_installation
  local nobin="$TEST_HOME/nobin"
  local tool
  mkdir -p "$nobin"
  for tool in awk sed grep cat rm mv cp mkdir du find head tail tr uname id \
    dirname basename ls chmod touch wc sort cut; do
    ln -sf "$(command -v "$tool")" "$nobin/$tool"
  done

  : > "$OUT_FILE"
  RUN_RC=0
  PATH="$nobin" /bin/bash "$UNINSTALL_SH" --apply \
    > "$OUT_FILE" 2>&1 < /dev/null || RUN_RC=$?
  # shunit2 resuelve sus shims (oneTimeTearDown, etc.) por PATH: se restaura
  # antes de cualquier assert para no romper el runner.
  PATH="$PATH_BASE"

  assertEquals 'sin systemctl igual termina 0' 0 "$RUN_RC"
  assertTrue 'advierte el SKIP del stop' "_out_grep_fixed 'SKIP'"
  assertTrue 'la unit igual se remueve' "[ ! -e '$P_SERVICE' ]"
  assertTrue 'nota de linger intacto' "_out_grep_fixed 'linger'"
}

# shellcheck disable=SC1091  # loader vendored de shunit2 (no se sigue: es POSIX sh)
. "$(dirname "$0")/lib/shunit2" || exit 1
