#!/bin/bash
# mcp-appointments-crm setup installer (Fase 4 PR1)
#
# Bash 3.2 floor — macOS stock /bin/bash must run this file.
# Forbidden constructs: associative arrays, mapfile/readarray, declare -n,
# ${var,,}, ${var^^}, pipefail, set -e.
# Regex: [[ $x =~ ERE ]] only (no \d/\w).
# Numeric parsing: $((10#$n)) to avoid octal pitfalls.

umask 077
set -u

CURRENT_TMP=""
cleanup_tmp() { rm -f "$CURRENT_TMP"; }
trap cleanup_tmp EXIT

# ---------------------------------------------------------------------------
# String helpers
# ---------------------------------------------------------------------------

trim_value() {
  local s="$1" c
  while [ -n "$s" ]; do
    c=${s%"${s#?}"}
    case $c in [$' \t\n\r']) s=${s#?} ;; *) break ;; esac
  done
  while [ -n "$s" ]; do
    c=${s#"${s%?}"}
    case $c in [$' \t\n\r']) s=${s%?} ;; *) break ;; esac
  done
  printf '%s' "$s"
}

is_blank() {
  local t
  t=$(trim_value "$1")
  [ -z "$t" ]
}

char_code() {
  local LC_ALL=C
  printf '%d' "'$1'"
}

str_toupper() {
  local s="$1" c out="" i=0 len code
  len=${#s}
  while [ $i -lt "$len" ]; do
    c=${s:$i:1}
    code=$(char_code "$c")
    if [ "$code" -ge 97 ] && [ "$code" -le 122 ]; then
      code=$((code - 32))
      c=$(printf '%b' "\\x$(printf '%02x' "$code")")
    fi
    out="${out}${c}"
    i=$((i + 1))
  done
  printf '%s' "$out"
}

# ---------------------------------------------------------------------------
# Validators (pure: exit 0 valid / 1 invalid, Spanish error to stderr)
# ---------------------------------------------------------------------------

v_nonempty() { is_blank "$1" && { echo 'Error: este campo no puede estar vacío.' >&2; return 1; }; return 0; }
v_country() { [[ $1 =~ ^[A-Za-z]{2}$ ]] && return 0; echo 'Error: el país debe tener exactamente dos letras (ej. AR).' >&2; return 1; }
v_email() { [[ $1 =~ ^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$ ]] && return 0; echo 'Error: el correo electrónico no tiene un formato válido.' >&2; return 1; }
v_phone() { [[ $1 =~ ^\+[0-9]{8,15}$ ]] && return 0; echo 'Error: el teléfono debe comenzar con + y tener entre 8 y 15 dígitos.' >&2; return 1; }
v_url() { [[ $1 =~ ^https?://[^[:space:]]+$ ]] && return 0; echo 'Error: la URL debe comenzar con http:// o https://.' >&2; return 1; }
v_messenger_platform() { case $1 in whatsapp|telegram) return 0 ;; esac; echo 'Error: la plataforma debe ser whatsapp o telegram.' >&2; return 1; }
v_symbol() { is_blank "$1" && { echo 'Error: el símbolo no puede estar vacío.' >&2; return 1; }; return 0; }

_decimal_range() {
  local val="$1" bound="$2" num int frac
  [[ $val =~ ^-?[0-9]+(\.[0-9]+)?$ ]] || return 1
  num=${val#-}
  if [[ $num == *.* ]]; then int=${num%%.*}; frac=${num#*.}; else int=$num; frac=""; fi
  int=$((10#$int))
  [ "$int" -lt "$bound" ] && return 0
  [ "$int" -gt "$bound" ] && return 1
  [ -z "$frac" ] && return 0
  case $frac in *[!0]*) return 1 ;; *) return 0 ;; esac
}

v_latitude() { _decimal_range "$1" 90 && return 0; echo 'Error: la latitud debe estar entre -90 y 90.' >&2; return 1; }
v_longitude() { _decimal_range "$1" 180 && return 0; echo 'Error: la longitud debe estar entre -180 y 180.' >&2; return 1; }
v_currency() { [[ $1 =~ ^[A-Z]{3}$ ]] && return 0; echo 'Error: la moneda debe tener tres letras mayúsculas (ej. ARS).' >&2; return 1; }
v_timezone() { [[ $1 =~ ^[A-Z][A-Za-z_]*(/[A-Z][A-Za-z0-9_+-]*){0,2}$ ]] && return 0; echo 'Error: la zona horaria no tiene forma de ruta IANA (ej. America/Argentina/Buenos_Aires).' >&2; return 1; }
v_positive_int() { [[ $1 =~ ^[0-9]+$ ]] && [ $((10#$1)) -gt 0 ] && return 0; echo 'Error: debe ser un número entero mayor a cero.' >&2; return 1; }
v_price() { [[ $1 =~ ^[0-9]+(\.[0-9]+)?$ ]] && return 0; echo 'Error: el precio debe ser un número mayor o igual a cero.' >&2; return 1; }

v_payment_list() {
  local list="$1" item count=0
  local IFS=,
  for item in $list; do
    item=$(trim_value "$item")
    [ -n "$item" ] && count=$((count + 1))
  done
  [ $count -ge 1 ] && return 0
  echo 'Error: ingrese al menos un método de pago separado por comas.' >&2
  return 1
}

v_hhmm() { [[ $1 =~ ^([01][0-9]|2[0-3]):[0-5][0-9]$ ]] && return 0; echo 'Error: el horario debe tener formato HH:MM de 24 horas.' >&2; return 1; }

v_time_pair() {
  local start="$1" end="$2" s_m e_m
  v_hhmm "$start" || return 1
  v_hhmm "$end" || return 1
  s_m=$((10#${start%%:*} * 60 + 10#${start##*:}))
  e_m=$((10#${end%%:*} * 60 + 10#${end##*:}))
  [ $s_m -lt $e_m ] && return 0
  echo 'Error: la hora de inicio debe ser anterior a la de cierre.' >&2
  return 1
}

# ---------------------------------------------------------------------------
# Transforms
# ---------------------------------------------------------------------------

t_trim() { trim_value "$1"; }
t_upper() { str_toupper "$1"; }

# ---------------------------------------------------------------------------
# JSON helpers
# ---------------------------------------------------------------------------

json_escape() {
  local s="$1" c out="" i=0 len code
  local LC_ALL=C
  len=${#s}
  while [ $i -lt "$len" ]; do
    c=${s:$i:1}
    case $c in
      '"') out="${out}\\\"" ;;
      \\) out="${out}\\\\" ;;
      $'\n') out="${out}\\n" ;;
      $'\t') out="${out}\\t" ;;
      $'\r') out="${out}\\r" ;;
      *)
        code=$(char_code "$c")
        if [ "$code" -lt 32 ] || [ "$code" -eq 127 ]; then
          out="${out}$(printf '\\u00%02X' "$code")"
        else
          out="${out}${c}"
        fi
        ;;
    esac
    i=$((i + 1))
  done
  printf '%s' "$out"
}

json_unescape() {
  local s="$1" c out="" i=0 len hex byte
  local LC_ALL=C
  len=${#s}
  while [ $i -lt "$len" ]; do
    c=${s:$i:1}
    if [ "$c" = "\\" ]; then
      local next=${s:$((i + 1)):1}
      case $next in
        \\) out="${out}\\"; i=$((i + 2)) ;;
        '"') out="${out}\""; i=$((i + 2)) ;;
        n) out="${out}"$'\n'; i=$((i + 2)) ;;
        t) out="${out}"$'\t'; i=$((i + 2)) ;;
        r) out="${out}"$'\r'; i=$((i + 2)) ;;
        u)
          hex=${s:$((i + 2)):4}
          byte=$(printf '%b' "\\x${hex#??}")
          out="${out}${byte}"
          i=$((i + 6))
          ;;
        *) out="${out}${c}${next}"; i=$((i + 2)) ;;
      esac
    else
      out="${out}${c}"
      i=$((i + 1))
    fi
  done
  printf '%s' "$out"
}

# ---------------------------------------------------------------------------
# Atomic write + path resolution
# ---------------------------------------------------------------------------

atomic_write() {
  local dest="$1"
  CURRENT_TMP="${dest}.new.$$"
  cat > "$CURRENT_TMP" && mv -f "$CURRENT_TMP" "$dest"
  local rc=$?
  CURRENT_TMP=""
  return $rc
}

resolve_paths() {
  if [ -z "${HOME:-}" ]; then
    echo 'Error: la variable HOME no está definida.' >&2
    return 1
  fi
  local os
  os=$(uname -s)
  if [ "$os" = "Darwin" ]; then
    CONFIG_DIR="$HOME/Library/Application Support/MCP Appointments CRM"
  else
    CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/mcp-appointments-crm"
  fi
  if [ -L "$CONFIG_DIR" ]; then
    echo "Error: el directorio de configuración es un enlace simbólico: $CONFIG_DIR" >&2
    return 1
  fi
  # shellcheck disable=SC2034
  SETUP_DIR="$CONFIG_DIR/setup"
  # shellcheck disable=SC2034
  CHECKPOINT_PATH="$CONFIG_DIR/setup.json.tmp"
}

# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

usage() {
  cat <<'EOF'
Uso: ./scripts/install.sh [--setup-only|--help]

  --setup-only   Ejecuta solo el flujo de configuración inicial (default).
  --help         Muestra esta ayuda.
EOF
}

main() {
  case "${1:-}" in
    --help) usage; exit 0 ;;
    --setup-only|"") echo 'Setup flow: disponible en el siguiente PR'; exit 0 ;;
    *) echo "Error: argumento desconocido: $1" >&2; exit 1 ;;
  esac
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
