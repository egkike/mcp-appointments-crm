#!/bin/bash
# scripts/uninstall.sh — desinstalación de mcp-appointments-crm.
#
# Contrato D2/D3/D4 de feat-install-scope-uninstall (spec install-scope-uninstall):
#   * Sin flags = dry-run: imprime el plan completo por artefacto y NO escribe nada;
#     la ejecución exige --apply y cada paso reporta lo que hizo.
#   * Los datos del negocio se preservan por default; --purge-data es el único
#     camino destructivo y exige doble gate (--yes + dos confirmaciones en TTY).
#   * audit-hash-key sólo se borra con --purge-config, con advertencia explícita
#     sobre la auditoría histórica del HMAC (ADR-0021).
#   * Hermes: --purge-hermes-entry quita mcp_servers.mcp-appointments del
#     config.yaml (byte-clean, atomic replace, sin invocar el binario Go); la
#     skill recepcionista-negocio se borra por default (OQ-3) y --keep-skill
#     la preserva. El desacople corre ANTES de borrar el binario.
#   * Verificación final por artefacto; fail-closed si algo removido reaparece.
#
# Bash 3.2 floor — misma disciplina que scripts/install.sh:
# sin arrays asociativos, sin mapfile/readarray, sin ${var,,}/${var^^},
# sin pipefail, sin set -e. Corre aunque el binario no exista (OQ-2).

umask 077
set -u

# Modo y flags. Valores resueltos en parse_args.
APPLY=0
PURGE_DATA=0
PURGE_CONFIG=0
PURGE_HERMES_ENTRY=0
KEEP_SKILL=0
ASSUME_YES=0

# Layout resuelto por resolve_paths (espejo inmutable de install.sh).
PLATFORM=""
CONFIG_DIR=""
DATA_DIR=""
LOG_DIR=""
BIN_DIR=""
ENV_FILE=""
DATA_SCRIPTS_DIR=""
SERVICE_KIND=""
SERVICE_PATH=""

# Hermes (espejo de internal/admin/hermes.go + install.sh).
HERMES_HOME_DIR=""
HERMES_CONFIG=""
HERMES_SKILL_DIR=""

# Constantes del layout instalado.
SERVICE_UNIT_NAME="mcp-appointments-crm"
LAUNCHD_LABEL="com.mcp.appointments.server"
SKILL_SLUG="recepcionista-negocio"
AUDIT_KEY_FILE="audit-hash-key"
CALLER_ID_FILE="caller-id"
DB_BASENAME="reservas.db"

# Tabla de artefactos considerados. Índices paralelos: label, path, verbo,
# tipo de verificación final (exists | hermes_entry | none).
ROW_LABEL=()
ROW_PATH=()
ROW_VERB=()
ROW_CHECK=()
LAST_ROW_INDEX=""

# Índices de los artefactos que ejecutan pasos (asignados en collect_rows).
IDX_SERVICE=""
IDX_HERMES_ENTRY=""
IDX_HERMES_SKILL=""
IDX_BINARY=""
IDX_BINARY_PREV=""
IDX_LOGS=""
IDX_CONFIG_ENV=""
IDX_CONFIG_CALLER=""
IDX_CONFIG_AUDIT_KEY=""
IDX_CONFIG_SETUP=""
IDX_CONFIG_BACKUPS=""
IDX_CONFIG_TEMPLATES=""
IDX_CONFIG_SCRIPTS=""
IDX_CONFIG_DIR=""
IDX_DATA_DB=""
IDX_DATA_WAL=""
IDX_DATA_SHM=""
IDX_DATA_SCRIPTS=""
IDX_DATA_BACKUPS=""

# APPLY_ERRORS cuenta los pasos que fallaron sin abortar el resto del plan;
# el exit final es != 0 si queda alguno.
APPLY_ERRORS=0

# CURRENT_TMP es el temporal en vuelo (atomic replace del config de Hermes).
# El trap EXIT/INT/TERM/HUP lo limpia si el script aborta a mitad de camino.
CURRENT_TMP=""
cleanup_tmp() {
  if [ -n "$CURRENT_TMP" ]; then
    rm -f "$CURRENT_TMP"
    CURRENT_TMP=""
  fi
}
trap cleanup_tmp EXIT INT TERM HUP

# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

usage() {
  cat <<'EOF'
Uso: ./scripts/uninstall.sh [--apply] [--dry-run] [--purge-data]
                            [--purge-config] [--purge-hermes-entry]
                            [--keep-skill] [--yes] [--help]

Sin flags el script corre en dry-run: imprime el plan completo por artefacto
y NO modifica nada. La ejecución exige --apply.

  --apply               Ejecuta la desinstalación (sin este flag no se escribe nada).
  --dry-run             Fuerza el modo plan (default).
  --purge-data          Borra la DB del negocio (reservas.db + -wal/-shm) y la copia
                        de scripts/ del data dir. Doble gate: --yes más dos
                        confirmaciones interactivas cuando stdin es un TTY, y
                        chequeo fail-closed de runtime vivo sobre la DB.
                        El directorio backups/ se preserva SIEMPRE.
  --purge-config        Borra el directorio de configuración completo, incluido
                        audit-hash-key (invalida la auditoría histórica del HMAC).
  --purge-hermes-entry  Quita mcp_servers.mcp-appointments de ~/.hermes/config.yaml
                        (o de $HERMES_HOME/config.yaml) sin tocar claves ajenas.
  --keep-skill          Conserva la skill recepcionista-negocio de Hermes
                        (por default se borra: OQ-3).
  --yes                 Afirmación explícita no interactiva (requerida por --purge-data).
  --help                Muestra esta ayuda.
EOF
}

parse_args() {
  while [ "$#" -gt 0 ]; do
    case "$1" in
      --apply) APPLY=1 ;;
      --dry-run) APPLY=0 ;;
      --purge-data) PURGE_DATA=1 ;;
      --purge-config) PURGE_CONFIG=1 ;;
      --purge-hermes-entry) PURGE_HERMES_ENTRY=1 ;;
      --keep-skill) KEEP_SKILL=1 ;;
      --yes) ASSUME_YES=1 ;;
      --help|-h)
        usage
        exit 0
        ;;
      *)
        echo "Error: argumento desconocido: $1" >&2
        usage >&2
        exit 1
        ;;
    esac
    shift
  done
}

# ---------------------------------------------------------------------------
# Layout — espejo inmutable de scripts/install.sh resolve_paths()
# ---------------------------------------------------------------------------

# _check_not_symlink: defensa en profundidad. Un directorio raíz del layout que
# sea symlink no es una instalación de este installer; fail-closed antes de
# borrar nada.
_check_not_symlink() {
  local path="$1" label="$2"
  if [ -L "$path" ]; then
    echo "Error: $label es un enlace simbólico: $path" >&2
    return 1
  fi
  return 0
}

resolve_paths() {
  if [ -z "${HOME:-}" ]; then
    echo 'Error: la variable HOME no está definida.' >&2
    return 1
  fi
  if [ "$(uname -s)" = "Darwin" ]; then
    PLATFORM="darwin"
    CONFIG_DIR="$HOME/Library/Application Support/MCP Appointments CRM"
    DATA_DIR="$HOME/Library/Application Support/MCP Appointments CRM"
    LOG_DIR="$HOME/Library/Logs/MCP Appointments CRM"
    BIN_DIR="$HOME/.local/bin"
    SERVICE_KIND="launchd"
    SERVICE_PATH="$HOME/Library/LaunchAgents/$LAUNCHD_LABEL.plist"
  else
    PLATFORM="linux"
    CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/mcp-appointments-crm"
    DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/mcp-appointments-crm"
    LOG_DIR="${XDG_STATE_HOME:-$HOME/.local/state}/mcp-appointments-crm"
    BIN_DIR="${XDG_BIN_HOME:-$HOME/.local/bin}"
    SERVICE_KIND="systemd"
    SERVICE_PATH="$HOME/.config/systemd/user/$SERVICE_UNIT_NAME.service"
  fi
  # Quirk replicado del installer: ENV_FILE es SIEMPRE ~/.config en ambos OS,
  # aunque CONFIG_DIR apunte a Library/Application Support en macOS.
  ENV_FILE="$HOME/.config/mcp-appointments-crm/.env"
  DATA_SCRIPTS_DIR="$DATA_DIR/scripts"

  _check_not_symlink "$CONFIG_DIR" 'el directorio de configuración' || return 1
  _check_not_symlink "$DATA_DIR" 'DATA_DIR' || return 1
  _check_not_symlink "$BIN_DIR" 'BIN_DIR' || return 1
  _check_not_symlink "$LOG_DIR" 'LOG_DIR' || return 1
  return 0
}

# resolve_hermes_paths: misma resolución que internal/admin.HermesDir() y
# install.sh install_receptionist_skill() — HERMES_HOME es el directorio
# mismo, nunca el padre de .hermes.
resolve_hermes_paths() {
  if [ -n "${HERMES_HOME:-}" ]; then
    HERMES_HOME_DIR="$HERMES_HOME"
  else
    HERMES_HOME_DIR="$HOME/.hermes"
  fi
  HERMES_CONFIG="$HERMES_HOME_DIR/config.yaml"
  HERMES_SKILL_DIR="$HERMES_HOME_DIR/skills/$SKILL_SLUG"
}

platform_label() {
  if [ "$PLATFORM" = "darwin" ]; then
    printf 'macOS (launchd)'
  else
    printf 'Linux (systemd --user)'
  fi
}

# ---------------------------------------------------------------------------
# Tabla de artefactos
# ---------------------------------------------------------------------------

# add_row <label> <path> <verbo> <check>
add_row() {
  ROW_LABEL[${#ROW_LABEL[@]}]="$1"
  ROW_PATH[${#ROW_PATH[@]}]="$2"
  ROW_VERB[${#ROW_VERB[@]}]="$3"
  ROW_CHECK[${#ROW_CHECK[@]}]="$4"
  LAST_ROW_INDEX=$((${#ROW_LABEL[@]} - 1))
}

set_verb() {
  ROW_VERB[$1]="$2"
}

# _path_exists: -e no alcanza para un symlink colgado.
_path_exists() {
  [ -e "$1" ] || [ -L "$1" ]
}

# _config_owned_rows_verb resuelve el verbo de las filas de configuración: con
# --purge-config se purgan, si no se preservan por decisión.
_config_verb() {
  if [ "$PURGE_CONFIG" -eq 1 ]; then
    printf 'purge'
  else
    printf 'keep'
  fi
}

# _data_verb resuelve el verbo de las filas de datos: sólo --purge-data las toca.
_data_verb() {
  if [ "$PURGE_DATA" -eq 1 ]; then
    printf 'purge'
  else
    printf 'flag'
  fi
}

collect_rows() {
  local verb

  # 1. Servicio (stop + unidad/plist).
  if _path_exists "$SERVICE_PATH"; then verb="remove"; else verb="absent"; fi
  add_row "servicio $SERVICE_KIND: $SERVICE_PATH" "$SERVICE_PATH" "$verb" "exists"
  IDX_SERVICE="$LAST_ROW_INDEX"

  # Nota: linger de systemd --user nunca se toca (lo puede usar otro service
  # del operador). Se imprime como nota, fuera de la tabla de artefactos.

  # 3. Entrada Hermes mcp_servers.mcp-appointments.
  if [ "$PURGE_HERMES_ENTRY" -eq 1 ]; then
    if _hermes_entry_present; then verb="remove"; else verb="absent"; fi
  else
    if [ -f "$HERMES_CONFIG" ]; then verb="intact"; else verb="absent"; fi
  fi
  add_row "entrada Hermes mcp_servers.mcp-appointments: $HERMES_CONFIG" \
    "$HERMES_CONFIG" "$verb" "hermes_entry"
  IDX_HERMES_ENTRY="$LAST_ROW_INDEX"

  # 4. Skill de Hermes: default PURGA (OQ-3), --keep-skill la preserva.
  if [ -d "$HERMES_SKILL_DIR" ]; then
    if [ "$KEEP_SKILL" -eq 1 ]; then verb="keep"; else verb="remove"; fi
  else
    verb="absent"
  fi
  add_row "skill Hermes $SKILL_SLUG: $HERMES_SKILL_DIR" \
    "$HERMES_SKILL_DIR" "$verb" "exists"
  IDX_HERMES_SKILL="$LAST_ROW_INDEX"

  # 5. Binario y binario previo (después del desacople de Hermes).
  if _path_exists "$BIN_DIR/mcp-server"; then verb="remove"; else verb="absent"; fi
  add_row "binario: $BIN_DIR/mcp-server" "$BIN_DIR/mcp-server" "$verb" "exists"
  IDX_BINARY="$LAST_ROW_INDEX"

  if _path_exists "$BIN_DIR/mcp-server.prev"; then verb="remove"; else verb="absent"; fi
  add_row "binario previo: $BIN_DIR/mcp-server.prev" \
    "$BIN_DIR/mcp-server.prev" "$verb" "exists"
  IDX_BINARY_PREV="$LAST_ROW_INDEX"

  # 6. Logs.
  if _path_exists "$LOG_DIR"; then verb="remove"; else verb="absent"; fi
  add_row "logs: $LOG_DIR" "$LOG_DIR" "$verb" "exists"
  IDX_LOGS="$LAST_ROW_INDEX"

  # 7. Config: preservados por default, purgados sólo con --purge-config.
  verb=$(_config_verb)
  if _path_exists "$ENV_FILE"; then
    add_row "config .env: $ENV_FILE" "$ENV_FILE" "$verb" "exists"
  else
    add_row "config .env: $ENV_FILE" "$ENV_FILE" "absent" "exists"
  fi
  IDX_CONFIG_ENV="$LAST_ROW_INDEX"

  verb=$(_config_verb)
  if _path_exists "$CONFIG_DIR/$CALLER_ID_FILE"; then
    add_row "config $CALLER_ID_FILE: $CONFIG_DIR/$CALLER_ID_FILE" \
      "$CONFIG_DIR/$CALLER_ID_FILE" "$verb" "exists"
  else
    add_row "config $CALLER_ID_FILE: $CONFIG_DIR/$CALLER_ID_FILE" \
      "$CONFIG_DIR/$CALLER_ID_FILE" "absent" "exists"
  fi
  IDX_CONFIG_CALLER="$LAST_ROW_INDEX"

  verb=$(_config_verb)
  if _path_exists "$CONFIG_DIR/$AUDIT_KEY_FILE"; then
    add_row "config $AUDIT_KEY_FILE: $CONFIG_DIR/$AUDIT_KEY_FILE" \
      "$CONFIG_DIR/$AUDIT_KEY_FILE" "$verb" "exists"
  else
    add_row "config $AUDIT_KEY_FILE: $CONFIG_DIR/$AUDIT_KEY_FILE" \
      "$CONFIG_DIR/$AUDIT_KEY_FILE" "absent" "exists"
  fi
  IDX_CONFIG_AUDIT_KEY="$LAST_ROW_INDEX"

  verb=$(_config_verb)
  if _path_exists "$CONFIG_DIR/setup"; then
    add_row "config setup/: $CONFIG_DIR/setup" "$CONFIG_DIR/setup" "$verb" "exists"
  else
    add_row "config setup/: $CONFIG_DIR/setup" "$CONFIG_DIR/setup" "absent" "exists"
  fi
  IDX_CONFIG_SETUP="$LAST_ROW_INDEX"

  verb=$(_config_verb)
  if [ "$CONFIG_DIR" = "$DATA_DIR" ]; then
    verb="keep"   # macOS: este backups/ es el del operador; nada lo borra.
  fi
  if _path_exists "$CONFIG_DIR/backups"; then
    add_row "config backups/: $CONFIG_DIR/backups" \
      "$CONFIG_DIR/backups" "$verb" "exists"
  else
    add_row "config backups/: $CONFIG_DIR/backups" \
      "$CONFIG_DIR/backups" "absent" "exists"
  fi
  IDX_CONFIG_BACKUPS="$LAST_ROW_INDEX"

  # templates/ y la copia de scripts/ son artefactos del installer: se van
  # siempre con --apply (no son datos del operador).
  if _path_exists "$CONFIG_DIR/templates"; then verb="remove"; else verb="absent"; fi
  add_row "config templates/: $CONFIG_DIR/templates" \
    "$CONFIG_DIR/templates" "$verb" "exists"
  IDX_CONFIG_TEMPLATES="$LAST_ROW_INDEX"

  if _path_exists "$CONFIG_DIR/scripts"; then verb="remove"; else verb="absent"; fi
  add_row "config scripts/ (copia): $CONFIG_DIR/scripts" \
    "$CONFIG_DIR/scripts" "$verb" "exists"
  IDX_CONFIG_SCRIPTS="$LAST_ROW_INDEX"

  if [ "$PURGE_CONFIG" -eq 1 ]; then
    # macOS: CONFIG_DIR == DATA_DIR. Purgar el directorio entero borraría la DB
    # del negocio sin el doble gate; en ese caso se purga sólo la superficie de
    # configuración y la fila del directorio queda en "preservado".
    if [ "$CONFIG_DIR" = "$DATA_DIR" ]; then
      add_row "config dir completo: $CONFIG_DIR (comparte path con el data dir: se purga sólo la superficie de config)" \
        "$CONFIG_DIR" "keep" "exists"
    else
      add_row "config dir completo: $CONFIG_DIR" "$CONFIG_DIR" "purge" "exists"
    fi
  else
    add_row "config dir completo: $CONFIG_DIR (se conserva; --purge-config lo borra entero)" \
      "$CONFIG_DIR" "keep" "exists"
  fi
  IDX_CONFIG_DIR="$LAST_ROW_INDEX"

  # 8. Datos del negocio: intactos salvo --purge-data.
  verb=$(_data_verb)
  if _path_exists "$DATA_DIR/$DB_BASENAME"; then
    add_row "data DB: $DATA_DIR/$DB_BASENAME" "$DATA_DIR/$DB_BASENAME" "$verb" "exists"
  else
    add_row "data DB: $DATA_DIR/$DB_BASENAME" "$DATA_DIR/$DB_BASENAME" "absent" "exists"
  fi
  IDX_DATA_DB="$LAST_ROW_INDEX"

  verb=$(_data_verb)
  if _path_exists "$DATA_DIR/$DB_BASENAME-wal"; then
    add_row "data WAL: $DATA_DIR/$DB_BASENAME-wal" \
      "$DATA_DIR/$DB_BASENAME-wal" "$verb" "exists"
  else
    add_row "data WAL: $DATA_DIR/$DB_BASENAME-wal" \
      "$DATA_DIR/$DB_BASENAME-wal" "absent" "exists"
  fi
  IDX_DATA_WAL="$LAST_ROW_INDEX"

  verb=$(_data_verb)
  if _path_exists "$DATA_DIR/$DB_BASENAME-shm"; then
    add_row "data SHM: $DATA_DIR/$DB_BASENAME-shm" \
      "$DATA_DIR/$DB_BASENAME-shm" "$verb" "exists"
  else
    add_row "data SHM: $DATA_DIR/$DB_BASENAME-shm" \
      "$DATA_DIR/$DB_BASENAME-shm" "absent" "exists"
  fi
  IDX_DATA_SHM="$LAST_ROW_INDEX"

  verb=$(_data_verb)
  if _path_exists "$DATA_SCRIPTS_DIR"; then
    add_row "data scripts/ (copia): $DATA_SCRIPTS_DIR" \
      "$DATA_SCRIPTS_DIR" "$verb" "exists"
  else
    add_row "data scripts/ (copia): $DATA_SCRIPTS_DIR" \
      "$DATA_SCRIPTS_DIR" "absent" "exists"
  fi
  IDX_DATA_SCRIPTS="$LAST_ROW_INDEX"

  # backups/ del data dir: preservado SIEMPRE, incluso con --purge-data.
  if [ -d "$DATA_DIR/backups" ]; then verb="keep"; else verb="absent"; fi
  add_row "data backups/ (preservado por decisión, nunca se borra): $DATA_DIR/backups" \
    "$DATA_DIR/backups" "$verb" "none"
  IDX_DATA_BACKUPS="$LAST_ROW_INDEX"
}

# ---------------------------------------------------------------------------
# Render de la tabla
# ---------------------------------------------------------------------------

_render_verb() {
  local verb="$1"
  if [ "$APPLY" -eq 1 ]; then
    case "$verb" in
      remove) printf 'removido' ;;
      purge) printf 'PURGADO' ;;
      keep|flag) printf 'preservado por decisión' ;;
      absent) printf 'no existía' ;;
      intact) printf 'intacto' ;;
      *) printf '%s' "$verb" ;;
    esac
    return 0
  fi
  case "$verb" in
    remove) printf 'BORRARÁ' ;;
    purge) printf 'PURGARÁ' ;;
    keep) printf 'PRESERVARÁ' ;;
    flag) printf 'TOCARÁ' ;;
    absent) printf 'NO EXISTE' ;;
    intact) printf 'INTACTO' ;;
    *) printf '%s' "$verb" ;;
  esac
}

# print_backup_guidance: guía explícita de backup junto a las filas de datos, en
# dry-run y en --apply. Refleja el estado ACTUAL del data dir: en --apply se
# imprime después de los pasos, así que con --purge-data ya muestra el fallback
# (la copia residente de scripts/ se purgó). Es texto aditivo: no agrega estado
# ni filas a la tabla.
print_backup_guidance() {
  echo '== Guía de backup (los datos del negocio no se van solos a otro lado) =='
  if [ -f "$DATA_SCRIPTS_DIR/backup.sh" ]; then
    echo "Comando de backup sugerido: bash \"$DATA_SCRIPTS_DIR/backup.sh\" \"$DATA_DIR/$DB_BASENAME\""
  else
    echo "Rollback manual sin ese script: copiá \"$DATA_DIR/$DB_BASENAME\" junto con \"$DB_BASENAME-wal\" y \"$DB_BASENAME-shm\" si existen (los 3 archivos juntos; el par wal/shm sin el .db es inservible)."
  fi
  echo "Los exports .gz que ya existan en \"$DATA_DIR/backups/\" no se tocan con ningún flag: backups/ se preserva siempre."
}

print_table() {
  local i=0 count=${#ROW_LABEL[@]}
  if [ "$APPLY" -eq 1 ]; then
    echo '== Verificación final por artefacto =='
  else
    echo '== Plan de desinstalación por artefacto (dry-run) =='
  fi
  while [ "$i" -lt "$count" ]; do
    printf '  [%s] %s\n' "$(_render_verb "${ROW_VERB[$i]}")" "${ROW_LABEL[$i]}"
    i=$((i + 1))
  done
}

# verify_absence: fail-closed. Todo artefacto marcado remove/purge debe seguir
# ausente; si reapareció (race con un runtime, un service que se relanza, un
# reinstall concurrente) el script falla en vez de ocultar el resto.
verify_absence() {
  local i=0 count=${#ROW_VERB[@]} verb path check failed=0
  while [ "$i" -lt "$count" ]; do
    verb="${ROW_VERB[$i]}"
    if [ "$verb" = "remove" ] || [ "$verb" = "purge" ]; then
      path="${ROW_PATH[$i]}"
      check="${ROW_CHECK[$i]}"
      case "$check" in
        hermes_entry)
          if _hermes_entry_present; then
            echo "Error: la entrada mcp_servers.mcp-appointments reapareció en $path." >&2
            failed=1
          fi
          ;;
        exists)
          if [ -n "$path" ] && _path_exists "$path"; then
            echo "Error: $path reapareció después de su remoción (estado inconsistente)." >&2
            failed=1
          fi
          ;;
        *) ;;
      esac
    fi
    i=$((i + 1))
  done
  return "$failed"
}

# ---------------------------------------------------------------------------
# Helpers de borrado y de inspección
# ---------------------------------------------------------------------------

# _safe_rm_rf: ningún borrado recursivo sin path absoluto y no trivial. Defensa
# en profundidad contra una variable vacía o mal resuelta.
_safe_rm_rf() {
  local target="${1:-}"
  case "$target" in
    ""|"/"|"."|"..")
      echo "Error: ruta insegura para borrar: '$target'" >&2
      return 1
      ;;
  esac
  case "$target" in
    /*) ;;
    *)
      echo "Error: ruta no absoluta para borrar: '$target'" >&2
      return 1
      ;;
  esac
  if [ -L "$target" ]; then
    rm -f "$target"
    return $?
  fi
  rm -rf "$target"
}

_count_files() {
  local dir="$1"
  if [ ! -d "$dir" ]; then
    printf '0'
    return 0
  fi
  find "$dir" -maxdepth 1 -type f 2>/dev/null | wc -l | tr -d '[:space:]'
}

# ---------------------------------------------------------------------------
# Hermes — desacople byte-clean (awk línea a línea, nunca re-marshal)
# ---------------------------------------------------------------------------

# _hermes_entry_present: true sólo si `mcp_servers:` (top-level, indent 0)
# tiene como hijo directo la entrada. Una key homónima bajo otro padre o más
# profunda no cuenta: nunca se reporta ni se toca.
_hermes_entry_present() {
  [ -f "$HERMES_CONFIG" ] || return 1
  awk '
    /^mcp_servers:$/ { in_ms = 1; next }
    in_ms && /^[^ #]/ { exit }
    in_ms && /^  mcp-appointments:/ { found = 1; exit }
    END { exit(found ? 0 : 1) }
  ' "$HERMES_CONFIG"
}

# _hermes_remove_entry: quita el bloque mcp_servers.mcp-appointments respetando
# bytes de todo lo demás. Reglas (contrato D3):
#   * inicio: la línea exacta "  mcp-appointments:" (indent 2);
#   * fin: la próxima línea con indent <= 2 que abre una key hermana (las
#     líneas en blanco y los comentarios dentro del bloque se consumen);
#   * si mcp_servers: queda sin hijos, se elimina la key completa;
#   * replace atómico (tmp + mv); si la entrada ya no existe, no se reescribe.
_hermes_remove_entry() {
  local tmp="${HERMES_CONFIG}.uninstall.$$"
  CURRENT_TMP="$tmp"
  if ! awk '
    function _indent(s,   r) { match(s, /^ */); r = RLENGTH; return r }
    function _blank(s) { return s ~ /^[ \t\r]*$/ }
    function _comment(s) { return s ~ /^[ \t]*#/ }
    function _key(s) { return !_blank(s) && !_comment(s) }
    {
      n += 1
      lines[n] = $0
    }
    END {
      ms = 0
      ent = 0
      for (i = 1; i <= n; i++) {
        if (lines[i] == "mcp_servers:") { ms = i; break }
      }
      if (ms > 0) {
        for (i = ms + 1; i <= n; i++) {
          if (lines[i] ~ /^[^ #]/) { break }
          if (lines[i] ~ /^  mcp-appointments:/) { ent = i; break }
        }
      }
      if (ent == 0) {
        for (i = 1; i <= n; i++) { print lines[i] }
        exit 0
      }
      endline = n + 1
      for (i = ent + 1; i <= n; i++) {
        if (_key(lines[i]) && _indent(lines[i]) <= 2) { endline = i; break }
      }
      for (i = 1; i <= n; i++) {
        if (i >= ent && i < endline) { continue }
        keep[i] = 1
      }
      if (ms > 0 && keep[ms] == 1) {
        child = 0
        for (i = ms + 1; i <= n; i++) {
          if (keep[i] != 1) { continue }
          if (!_key(lines[i])) { continue }
          if (_indent(lines[i]) == 0) { break }
          child = 1
          break
        }
        if (child == 0) { keep[ms] = 0 }
      }
      for (i = 1; i <= n; i++) {
        if (keep[i] == 1) { print lines[i] }
      }
    }
  ' "$HERMES_CONFIG" > "$tmp"; then
    echo "Error: no se pudo reescribir $HERMES_CONFIG." >&2
    CURRENT_TMP=""
    rm -f "$tmp"
    return 1
  fi
  chmod 0600 "$tmp" 2>/dev/null || true
  if ! mv -f "$tmp" "$HERMES_CONFIG"; then
    echo "Error: no se pudo reemplazar $HERMES_CONFIG (temporal: $tmp)." >&2
    CURRENT_TMP=""
    rm -f "$tmp"
    return 1
  fi
  CURRENT_TMP=""
  return 0
}

# ---------------------------------------------------------------------------
# Gate destructivo de --purge-data (D4)
# ---------------------------------------------------------------------------

_print_purge_gate_info() {
  local db="$DATA_DIR/$DB_BASENAME" size backups
  echo 'Gate destructivo de --purge-data:'
  echo "  DB del negocio: $db"
  if [ -f "$db" ]; then
    size=$(du -h "$db" 2>/dev/null | awk '{print $1}')
    echo "  Tamaño: ${size:-desconocido}"
  else
    echo '  Tamaño: la DB no existe (no hay datos que borrar)'
  fi
  backups=$(_count_files "$DATA_DIR/backups")
  if [ "$backups" -gt 0 ]; then
    echo "  Backups: $backups archivo(s) en $DATA_DIR/backups (se PRESERVAN siempre)"
  else
    echo "  Backups: 0 archivos en $DATA_DIR/backups"
    echo "  Antes de purgar hacé un backup: docs/installation.md Paso 2 (bash \"$DATA_DIR/scripts/backup.sh\" \"$db\")"
  fi
}

_prompt_first_confirmation() {
  local raw
  printf '%s' 'Confirma borrar permanentemente la DB del negocio (sin vuelta atrás)? (y/N): '
  if ! read -r raw; then
    printf '\n'
    echo 'Error: no se pudo leer la confirmación; no se borra nada.' >&2
    return 1
  fi
  raw=$(printf '%s' "$raw" | tr -d '[:space:]')
  case "$raw" in
    y|Y|s|S|si|sí|SI|SÍ) return 0 ;;
  esac
  echo 'Purga de datos cancelada: no se borró nada.' >&2
  return 1
}

_prompt_final_confirmation() {
  local raw
  printf '%s' 'ÚLTIMA CONFIRMACIÓN: este borrado es permanente y sin vuelta atrás (escribí BORRAR en mayúsculas): '
  if ! read -r raw; then
    printf '\n'
    echo 'Error: no se pudo leer la segunda confirmación; no se borra nada.' >&2
    return 1
  fi
  if [ "$raw" = "BORRAR" ]; then
    return 0
  fi
  echo 'Purga de datos cancelada: la segunda confirmación no fue BORRAR; no se borró nada.' >&2
  return 1
}

purge_data_gate() {
  _print_purge_gate_info
  if [ "$ASSUME_YES" -ne 1 ]; then
    echo 'Error: --purge-data exige el gate destructivo completo:' >&2
    echo '  1) --yes explícito (afirmación no interactiva).' >&2
    echo '  2) primera confirmación: "Confirma borrar permanentemente la DB del negocio (sin vuelta atrás)? (y/N): " (y/s).' >&2
    echo '  3) segunda confirmación "ÚLTIMA CONFIRMACIÓN": escribir BORRAR en mayúsculas.' >&2
    if [ ! -t 0 ]; then
      echo '  stdin no es un TTY: agregá --yes para satisfacer 2) y 3) de forma explícita.' >&2
    else
      echo '  Volvé a correr con --yes para habilitar las confirmaciones interactivas.' >&2
    fi
    return 1
  fi
  if [ -t 0 ]; then
    if ! _prompt_first_confirmation; then
      return 1
    fi
    if ! _prompt_final_confirmation; then
      return 1
    fi
    return 0
  fi
  echo 'Confirmaciones: --yes en stdin no interactivo (los dos gates quedan afirmados explícitamente).'
  return 0
}

# purge_data_live_check: se corre ANTES de cualquier borrado y es fail-closed.
# Un MainPID vivo, un writer que el manager no ve, un pgrep ausente (liveness
# no demostrable) o un -wal/-shm no vacío abortan la purga entera.
purge_data_live_check() {
  local pid wal
  if [ "$PLATFORM" = "linux" ] && command -v systemctl >/dev/null 2>&1; then
    pid=$(systemctl --user show "$SERVICE_UNIT_NAME" -p MainPID 2>/dev/null \
      | sed -n 's/^MainPID=//p' | head -n 1 | tr -d '[:space:]')
    case "$pid" in
      ''|*[!0-9]*) pid="" ;;
    esac
    if [ -n "$pid" ] && [ "$pid" != "0" ]; then
      if kill -0 "$pid" 2>/dev/null; then
        echo "Error: el servicio $SERVICE_UNIT_NAME sigue vivo (MainPID=$pid); no se borra la DB por debajo de un runtime activo. Pará el servicio y volvé a correr." >&2
        return 1
      fi
    fi
  fi
  # Canal directo: el manager no ve un `./mcp-server` lanzado a mano (ni existe
  # en WSL sin systemd), así que la ausencia de MainPID no prueba liveness.
  if ! command -v pgrep >/dev/null 2>&1; then
    echo 'Error: no se puede probar que no haya un proceso mcp-server vivo (pgrep no está disponible); detenélo a mano y volvé a correr el uninstall.' >&2
    return 1
  fi
  # R3-001 (validator): exit 0 = writer vivo, exit 1 = sin writer (única salida
  # que permite el purge), >=2 = pgrep falló (no se puede probar liveness)
  # — fail-closed también ahí, nunca se borra sin prueba de ausencia.
  pgrep -f 'mcp-server' >/dev/null 2>&1
  local pgrep_rc=$?
  case $pgrep_rc in
    0)
      echo 'Error: hay un proceso mcp-server corriendo (no gestionado por el service manager); detenélo a mano y volvé a correr el uninstall.' >&2
      return 1
      ;;
    1)
      : # sin writer: sigue al check de WAL/SHM
      ;;
    *)
      echo 'Error: no se puede probar que no haya un proceso mcp-server vivo (pgrep falló); detenélo a mano y volvé a correr el uninstall.' >&2
      return 1
      ;;
  esac
  for wal in "$DATA_DIR/$DB_BASENAME-wal" "$DATA_DIR/$DB_BASENAME-shm"; do
    if [ -f "$wal" ] && [ -s "$wal" ]; then
      echo "Error: $wal no está vacío: hay (o quedó) un handle abierto sobre la DB. Cerrá el server y volvé a correr." >&2
      return 1
    fi
  done
  return 0
}

# ---------------------------------------------------------------------------
# Pasos de la reversión (contrato D2, orden inverso al install)
# ---------------------------------------------------------------------------

# _abort_service_running <motivo> <salida-del-stop>: fail-closed.
_abort_service_running() {
  echo "Error: no se pudo detener el servicio $SERVICE_UNIT_NAME ($1); el uninstall abortó sin borrar nada. Detené el servicio a mano y volvé a correr el script." >&2
  if [ -n "${2:-}" ]; then
    echo "Salida del stop: $2" >&2
  fi
}

# _stop_service: stop reteniendo la salida (nada de `|| true`). Stop OK → rc 0;
# stop fallido con el servicio todavía activo → aborta (rc 1, fail-closed);
# stop fallido pero ya inactivo → SKIP-safe (rc 0). Sin gestor (WSL sin
# systemd) sigue siendo SKIP-safe: no haber systemctl no es un stop fallido.
_stop_service() {
  local output="" rc=0
  if [ "$PLATFORM" = "darwin" ]; then
    if ! command -v launchctl >/dev/null 2>&1; then
      echo 'Advertencia: launchctl no disponible; se omite el stop del servicio (SKIP).'
      return 0
    fi
    output=$(launchctl bootout "gui/$UID/$LAUNCHD_LABEL" 2>&1)
    rc=$?
    if [ "$rc" -eq 0 ]; then
      echo "Servicio detenido (launchctl bootout gui/$UID/$LAUNCHD_LABEL)."
      return 0
    fi
    if launchctl print "gui/$UID/$LAUNCHD_LABEL" >/dev/null 2>&1; then
      _abort_service_running "launchctl print gui/$UID/$LAUNCHD_LABEL sigue cargado" "$output"
      return 1
    fi
    echo "Advertencia: launchctl bootout falló (rc=$rc) pero el servicio ya no está activo; se continúa."
    return 0
  fi
  if ! command -v systemctl >/dev/null 2>&1; then
    echo 'Advertencia: systemctl no disponible; se omite el stop del servicio (SKIP: WSL sin systemd).'
    return 0
  fi
  output=$(systemctl --user stop "$SERVICE_UNIT_NAME" 2>&1)
  rc=$?
  if [ "$rc" -eq 0 ]; then
    echo "Servicio detenido (systemctl --user stop $SERVICE_UNIT_NAME)."
    return 0
  fi
  if systemctl --user is-active "$SERVICE_UNIT_NAME" >/dev/null 2>&1; then
    _abort_service_running "systemctl --user is-active $SERVICE_UNIT_NAME lo reporta activo" "$output"
    return 1
  fi
  echo "Advertencia: el stop falló (rc=$rc) pero el servicio ya no está activo; se continúa."
  return 0
}

step_service() {
  local idx="$IDX_SERVICE"
  if [ ! -e "$SERVICE_PATH" ] && [ ! -L "$SERVICE_PATH" ]; then
    echo "Servicio: la unidad no existía ($SERVICE_PATH)."
    set_verb "$idx" absent
  else
    if ! _stop_service; then
      return 1
    fi
    if rm -f "$SERVICE_PATH"; then
      echo "Unidad de servicio removida: $SERVICE_PATH"
      set_verb "$idx" remove
    else
      echo "Error: no se pudo remover $SERVICE_PATH" >&2
      APPLY_ERRORS=$((APPLY_ERRORS + 1))
      set_verb "$idx" keep
    fi
  fi
  if [ "$PLATFORM" = "linux" ]; then
    if command -v systemctl >/dev/null 2>&1; then
      if systemctl --user daemon-reload >/dev/null 2>&1; then
        echo 'daemon-reload: OK.'
      else
        echo 'Advertencia: systemctl --user daemon-reload falló (best-effort, se continúa).'
      fi
    else
      echo 'Advertencia: systemctl no disponible; daemon-reload omitido (SKIP).'
    fi
    echo 'Nota: linger de systemd --user queda intacto (podría usarlo otro service del operador).'
  fi
  return 0
}

step_hermes_entry() {
  local idx="$IDX_HERMES_ENTRY"
  if [ "$PURGE_HERMES_ENTRY" -ne 1 ]; then
    if [ -f "$HERMES_CONFIG" ]; then
      echo "Entrada Hermes: se conserva ($HERMES_CONFIG); usá --purge-hermes-entry para quitarla."
      set_verb "$idx" intact
    else
      echo "Entrada Hermes: $HERMES_CONFIG no existe (nada que hacer)."
      set_verb "$idx" absent
    fi
    return 0
  fi
  if [ ! -f "$HERMES_CONFIG" ]; then
    echo "Entrada Hermes: $HERMES_CONFIG no existe (nada que hacer, no se crea)."
    set_verb "$idx" absent
    return 0
  fi
  if ! _hermes_entry_present; then
    echo "Entrada Hermes: mcp_servers.mcp-appointments no existía en $HERMES_CONFIG (archivo sin cambios)."
    set_verb "$idx" absent
    return 0
  fi
  if ! _hermes_remove_entry; then
    APPLY_ERRORS=$((APPLY_ERRORS + 1))
    return 0
  fi
  if _hermes_entry_present; then
    echo "Error: $HERMES_CONFIG todavía contiene mcp_servers.mcp-appointments." >&2
    APPLY_ERRORS=$((APPLY_ERRORS + 1))
  else
    echo "Entrada Hermes removida de $HERMES_CONFIG (verificación: sin mcp_servers.mcp-appointments)."
    set_verb "$idx" remove
  fi
}

step_hermes_skill() {
  local idx="$IDX_HERMES_SKILL"
  if [ ! -d "$HERMES_SKILL_DIR" ]; then
    echo "Skill Hermes: $HERMES_SKILL_DIR no existía."
    set_verb "$idx" absent
    return 0
  fi
  if [ "$KEEP_SKILL" -eq 1 ]; then
    echo "Skill Hermes preservada por decisión (--keep-skill): $HERMES_SKILL_DIR"
    set_verb "$idx" keep
    return 0
  fi
  if _safe_rm_rf "$HERMES_SKILL_DIR"; then
    echo "Skill Hermes removida: $HERMES_SKILL_DIR"
    set_verb "$idx" remove
  else
    echo "Error: no se pudo remover la skill de Hermes: $HERMES_SKILL_DIR" >&2
    APPLY_ERRORS=$((APPLY_ERRORS + 1))
    set_verb "$idx" keep
  fi
}

# _remove_artifact <path> <row index> <etiqueta>
_remove_artifact() {
  local path="$1" idx="$2" label="$3"
  if ! _path_exists "$path"; then
    echo "$label — no existía: $path"
    set_verb "$idx" absent
    return 0
  fi
  if _safe_rm_rf "$path"; then
    echo "$label — removido: $path"
    set_verb "$idx" remove
  else
    echo "Error: no se pudo remover $path" >&2
    APPLY_ERRORS=$((APPLY_ERRORS + 1))
    set_verb "$idx" keep
  fi
}

step_binary() {
  _remove_artifact "$BIN_DIR/mcp-server" "$IDX_BINARY" 'Binario'
  _remove_artifact "$BIN_DIR/mcp-server.prev" "$IDX_BINARY_PREV" 'Binario previo'
}

step_logs() {
  _remove_artifact "$LOG_DIR" "$IDX_LOGS" 'Logs'
}

# _keep_artifact <path> <row index> <etiqueta> — artefacto preservado por decisión.
_keep_artifact() {
  local path="$1" idx="$2" label="$3"
  if _path_exists "$path"; then
    echo "$label — preservado por decisión: $path"
    set_verb "$idx" keep
  else
    echo "$label — no existía: $path"
    set_verb "$idx" absent
  fi
}

# _config_verbs_all <verbo> — fija el verbo de todas las filas de configuración.
_config_verbs_all() {
  set_verb "$IDX_CONFIG_ENV" "$1"
  set_verb "$IDX_CONFIG_CALLER" "$1"
  set_verb "$IDX_CONFIG_AUDIT_KEY" "$1"
  set_verb "$IDX_CONFIG_SETUP" "$1"
  set_verb "$IDX_CONFIG_BACKUPS" "$1"
  set_verb "$IDX_CONFIG_TEMPLATES" "$1"
  set_verb "$IDX_CONFIG_SCRIPTS" "$1"
}

step_config() {
  if [ "$PURGE_CONFIG" -eq 1 ]; then
    _step_config_purge
    return 0
  fi
  # Default: .env, caller-id, audit-hash-key, setup/ y backups/ se preservan.
  _keep_artifact "$ENV_FILE" "$IDX_CONFIG_ENV" 'Config .env'
  _keep_artifact "$CONFIG_DIR/$CALLER_ID_FILE" "$IDX_CONFIG_CALLER" "Config $CALLER_ID_FILE"
  _keep_artifact "$CONFIG_DIR/$AUDIT_KEY_FILE" "$IDX_CONFIG_AUDIT_KEY" "Config $AUDIT_KEY_FILE"
  _keep_artifact "$CONFIG_DIR/setup" "$IDX_CONFIG_SETUP" 'Config setup/'
  _keep_artifact "$CONFIG_DIR/backups" "$IDX_CONFIG_BACKUPS" 'Config backups/'
  _keep_artifact "$CONFIG_DIR" "$IDX_CONFIG_DIR" 'Config dir'
  # Artefactos del installer: se van siempre con --apply.
  _remove_artifact "$CONFIG_DIR/templates" "$IDX_CONFIG_TEMPLATES" 'Config templates/'
  _remove_artifact "$CONFIG_DIR/scripts" "$IDX_CONFIG_SCRIPTS" 'Config scripts/ (copia)'
}

_step_config_purge() {
  if _path_exists "$CONFIG_DIR/$AUDIT_KEY_FILE"; then
    echo 'Advertencia: borrar audit-hash-key invalida la auditoría histórica del HMAC (ADR-0021): los registros antiguos pierden su identidad verificable.'
  fi
  if [ "$CONFIG_DIR" = "$DATA_DIR" ]; then
    # macOS: config y data comparten directorio. Se purga sólo la superficie
    # de configuración para no tocar la DB del negocio sin --purge-data.
    _purge_config_surface
    echo "Nota: en macOS el config dir coincide con el data dir ($CONFIG_DIR); la DB y backups/ se tocan sólo con --purge-data."
    if _path_exists "$CONFIG_DIR"; then
      set_verb "$IDX_CONFIG_DIR" keep
    else
      set_verb "$IDX_CONFIG_DIR" absent
    fi
    return 0
  fi
  if ! _path_exists "$CONFIG_DIR"; then
    echo "Config dir — no existía: $CONFIG_DIR"
    _config_verbs_all absent
    set_verb "$IDX_CONFIG_DIR" absent
  elif _safe_rm_rf "$CONFIG_DIR"; then
    echo "Config dir purgado: $CONFIG_DIR"
    _config_verbs_all purge
    set_verb "$IDX_CONFIG_DIR" purge
  else
    echo "Error: no se pudo purgar el config dir: $CONFIG_DIR" >&2
    APPLY_ERRORS=$((APPLY_ERRORS + 1))
    set_verb "$IDX_CONFIG_DIR" keep
  fi
  # Quirk del installer: ENV_FILE vive siempre en ~/.config/... aunque
  # CONFIG_DIR esté overrideado por XDG_CONFIG_HOME. Es superficie de purge de
  # primera clase: no puede quedar un .env colgado fuera del config dir.
  if _path_exists "$ENV_FILE"; then
    if _safe_rm_rf "$ENV_FILE"; then
      echo "Config .env (ruta del quir) purgado: $ENV_FILE"
      set_verb "$IDX_CONFIG_ENV" purge
    else
      echo "Error: no se pudo purgar $ENV_FILE" >&2
      APPLY_ERRORS=$((APPLY_ERRORS + 1))
      set_verb "$IDX_CONFIG_ENV" keep
    fi
  fi
}

# _purge_config_surface: purga la superficie de configuración cuando CONFIG_DIR
# y DATA_DIR comparten path (macOS), sin rozar los datos del negocio.
_purge_config_surface() {
  _purge_artifact "$ENV_FILE" "$IDX_CONFIG_ENV" 'Config .env'
  _purge_artifact "$CONFIG_DIR/$CALLER_ID_FILE" "$IDX_CONFIG_CALLER" "Config $CALLER_ID_FILE"
  _purge_artifact "$CONFIG_DIR/$AUDIT_KEY_FILE" "$IDX_CONFIG_AUDIT_KEY" "Config $AUDIT_KEY_FILE"
  _purge_artifact "$CONFIG_DIR/setup" "$IDX_CONFIG_SETUP" 'Config setup/'
  # backups/ NUNCA se borra: en macOS este dir ES el $DATA_DIR/backups (las
  # únicas copias de recuperación del operador). --purge-config no lo toca.
  _keep_artifact "$CONFIG_DIR/backups" "$IDX_CONFIG_BACKUPS" 'Config backups/ (es el data backups/: se preserva)'
  _purge_artifact "$CONFIG_DIR/templates" "$IDX_CONFIG_TEMPLATES" 'Config templates/'
  _purge_artifact "$CONFIG_DIR/scripts" "$IDX_CONFIG_SCRIPTS" 'Config scripts/ (copia)'
}

step_data() {
  if [ "$PURGE_DATA" -ne 1 ]; then
    _keep_artifact "$DATA_DIR/$DB_BASENAME" "$IDX_DATA_DB" 'Data DB'
    _keep_artifact "$DATA_DIR/$DB_BASENAME-wal" "$IDX_DATA_WAL" 'Data WAL'
    _keep_artifact "$DATA_DIR/$DB_BASENAME-shm" "$IDX_DATA_SHM" 'Data SHM'
    _keep_artifact "$DATA_SCRIPTS_DIR" "$IDX_DATA_SCRIPTS" 'Data scripts/ (copia)'
  else
    _purge_artifact "$DATA_DIR/$DB_BASENAME" "$IDX_DATA_DB" 'Data DB'
    _purge_artifact "$DATA_DIR/$DB_BASENAME-wal" "$IDX_DATA_WAL" 'Data WAL'
    _purge_artifact "$DATA_DIR/$DB_BASENAME-shm" "$IDX_DATA_SHM" 'Data SHM'
    _purge_artifact "$DATA_SCRIPTS_DIR" "$IDX_DATA_SCRIPTS" 'Data scripts/ (copia)'
  fi
  if [ -d "$DATA_DIR/backups" ]; then
    echo "Data backups/ — preservado por decisión (nunca se borra): $DATA_DIR/backups"
    set_verb "$IDX_DATA_BACKUPS" keep
  else
    echo "Data backups/ — no existía: $DATA_DIR/backups"
    set_verb "$IDX_DATA_BACKUPS" absent
  fi
}

# _purge_artifact <path> <row index> <etiqueta> — borrado destructivo de datos.
_purge_artifact() {
  local path="$1" idx="$2" label="$3"
  if ! _path_exists "$path"; then
    echo "$label — no existía: $path"
    set_verb "$idx" absent
    return 0
  fi
  if _safe_rm_rf "$path"; then
    echo "$label — PURGADO: $path"
    set_verb "$idx" purge
  else
    echo "Error: no se pudo purgar $path" >&2
    APPLY_ERRORS=$((APPLY_ERRORS + 1))
    set_verb "$idx" keep
  fi
}

run_apply() {
  # El stop es el primer paso y el único que puede abortar: si sigue vivo el
  # runtime, no se toca nada más (fail-closed, cero borrados).
  if ! step_service; then
    return 1
  fi
  step_hermes_entry
  step_hermes_skill
  step_binary
  step_logs
  step_config
  step_data
}

# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

main() {
  parse_args "$@"
  resolve_paths || exit 1
  resolve_hermes_paths
  collect_rows

  if [ "$APPLY" -eq 1 ]; then
    echo 'mcp-appointments-crm — desinstalación (apply)'
    printf 'Plataforma: %s\n\n' "$(platform_label)"
    if [ "$PURGE_DATA" -eq 1 ]; then
      if ! purge_data_gate; then
        exit 1
      fi
      if ! purge_data_live_check; then
        echo 'Error: --purge-data abortado fail-closed: no se borró nada.' >&2
        exit 1
      fi
      echo ''
    fi
    run_apply || exit 1
    echo ''
    print_table
    print_backup_guidance
    if [ "$PLATFORM" = "linux" ]; then
      echo 'Nota: linger de systemd --user queda intacto (no se toca en la desinstalación).'
    fi
    echo ''
    if ! verify_absence; then
      echo 'Estado final: ERROR — un artefacto removido volvió a aparecer; no se oculta el resto.' >&2
      exit 1
    fi
    if [ "$APPLY_ERRORS" -ne 0 ]; then
      echo "Estado final: ERROR — $APPLY_ERRORS paso(s) fallaron." >&2
      exit 1
    fi
    echo 'Estado final: OK — desinstalación consistente.'
    exit 0
  fi

  echo 'mcp-appointments-crm — plan de desinstalación (dry-run: no se escribe nada)'
  printf 'Plataforma: %s\n\n' "$(platform_label)"
  if [ "$PURGE_DATA" -eq 1 ]; then
    if ! purge_data_live_check; then
      echo 'Advertencia: el purge fallará cerrado con el estado actual (ver el error de arriba).' >&2
    fi
  fi
  if [ "$PURGE_CONFIG" -eq 1 ] && _path_exists "$CONFIG_DIR/$AUDIT_KEY_FILE"; then
    echo 'Advertencia: borrar audit-hash-key invalida la auditoría histórica del HMAC (ADR-0021): los registros antiguos pierden su identidad verificable.'
    echo ''
  fi
  print_table
  print_backup_guidance
  if [ "$PLATFORM" = "linux" ]; then
    echo 'Nota: linger de systemd --user queda intacto (no se toca en la desinstalación).'
  fi
  echo ''
  echo 'Pasá --apply para ejecutar la desinstalación.'
  exit 0
}

if [ "${BASH_SOURCE[0]:-$0}" = "$0" ]; then
  main "$@"
fi
