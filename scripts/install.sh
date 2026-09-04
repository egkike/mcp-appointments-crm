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

# CURRENT_TMP holds the in-flight temp file path used by atomic_write.
# Single active file at a time (single-threaded script); the EXIT trap
# cleans it up on normal exit, INT/TERM/HUP on abnormal termination.
# CONFIG_DIR, SETUP_DIR, CHECKPOINT_PATH are populated by resolve_paths
# and are intentionally global so the installer flow can reuse them.
CURRENT_TMP=""
cleanup_tmp() { [ -n "$CURRENT_TMP" ] && rm -f "$CURRENT_TMP"; }
# EXIT for normal exit; INT/TERM/HUP for shell signals (best-effort cleanup).
# Single-threaded: no race between concurrent atomic_write calls.
trap cleanup_tmp EXIT INT TERM HUP

# ---------------------------------------------------------------------------
# String helpers
# ---------------------------------------------------------------------------
# Bash 3.2 floor: no ${var^^}, no `tr` con LC_ALL controlado, ni arrays
# asociativos. Los helpers de abajo hacen trabajo byte a byte a mano
# para mantener la portabilidad en el /bin/bash stock de macOS.

trim_value() {
  local s="$1" c
  # Adelante: si el primer byte es espacio/tab/NL/CR, lo recorta.
  while [ -n "$s" ]; do
    c=${s%"${s#?}"}            # primer byte (binds tighter que %)
    case $c in [$' \t\n\r']) s=${s#?} ;; *) break ;; esac
  done
  # Atrás: si el último byte es espacio/tab/NL/CR, lo recorta.
  while [ -n "$s" ]; do
    c=${s#"${s%?}"}            # último byte
    case $c in [$' \t\n\r']) s=${s%?} ;; *) break ;; esac
  done
  printf '%s' "$s"
}

is_blank() {
  local t
  t=$(trim_value "$1")
  [ -z "$t" ]
}

# char_code: devuelve el valor numérico del primer byte de $1.
# LC_ALL=C garantiza que printf '%d' "'$c'" sea estable byte a byte
# en cualquier locale (sino, locales multibyte lo rompen).
char_code() {
  local LC_ALL=C
  printf '%d' "'$1'"
}

# str_toupper: upper-case ASCII por aritmética de código por byte.
# Bytes 0x61-0x7A (a-z) se convierten a A-Z restando 32. Bytes no-ASCII
# (acentos, emoji, UTF-8 multibyte) pasan tal cual.
str_toupper() {
  local s="$1" c out="" i=0 len code
  len=${#s}
  while [ $i -lt "$len" ]; do
    c=${s:$i:1}
    code=$(char_code "$c")
    if [ "$code" -ge 97 ] && [ "$code" -le 122 ]; then
      code=$((code - 32))
      # Emitir el byte convertido: printf %b interpreta \xHH literalmente.
      c=$(printf '%b' "\\x$(printf '%02x' "$code")")
    fi
    out="${out}${c}"
    i=$((i + 1))
  done
  printf '%s' "$out"
}

# ---------------------------------------------------------------------------
# Field registry
# ---------------------------------------------------------------------------
# Arrays paralelos (bash 3.2-safe) con el orden canónico de prompts,
# validadores, defaults y textos. BP_KEYS es la fuente de verdad para el
# orden de reanudación.

BP_KEYS=(
  name industry country address latitude longitude cover_photo_url
  public_phone messenger_platform messenger_id contact_email website_url
  general_description accepted_payment_methods currency_code currency_symbol
  timezone slot_interval_minutes
)

BP_PROMPTS=(
  "Nombre del negocio"
  "Industria"
  "País (dos letras, ej. AR)"
  "Dirección"
  "Latitud"
  "Longitud"
  "URL de foto de portada"
  "Teléfono público"
  "Plataforma de mensajería (whatsapp/telegram)"
  "ID de mensajería"
  "Correo electrónico de contacto"
  "Sitio web"
  "Descripción general"
  "Métodos de pago aceptados (separados por coma)"
  "Código de moneda (ej. ARS)"
  "Símbolo de moneda"
  "Zona horaria (ej. America/Argentina/Buenos_Aires)"
  "Intervalo de turnos (minutos)"
)

BP_REQUIRED=(1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0)

BP_VALIDATORS=(
  v_nonempty "" v_country "" v_latitude v_longitude v_url
  v_phone v_messenger_platform "" v_email v_url "" v_payment_list
  v_currency v_symbol v_timezone v_positive_int
)

BP_DEFAULTS=("" "" "" "" "" "" "" "" "" "" "" "" "" "" "ARS" "$" "UTC" "30")
# shellcheck disable=SC2034  # usado por render_setup_business en PR2b
BP_TYPES=(s s s s n n s s s s s s s l s s s i)

# Day mapping for business hours (monday..sunday) and staff schedule
# day-of-week integer (0=Sunday..6=Saturday, canonical schedules capability).
DAY_NAMES=(monday tuesday wednesday thursday friday saturday sunday)
DAY_LABELS=(Lunes Martes Miércoles Jueves Viernes Sábado Domingo)

STAFF_DAY_ORDER=(1 2 3 4 5 6 0)
STAFF_DAY_LABELS=(Lunes Martes Miércoles Jueves Viernes Sábado Domingo)

# Staff subfield registry (name is handled separately because it doubles as
# the loop terminator once at least one professional is captured).
STAFF_FIELD_KEYS=(role_specialty phone email)
STAFF_FIELD_PROMPTS=("Especialidad / rol" "Teléfono" "Correo electrónico")
STAFF_FIELD_REQUIRED=(0 0 0)
STAFF_FIELD_VALIDATORS=("" v_phone v_email)

# Service subfield registry (name handled separately for the same reason).
SERVICE_FIELD_KEYS=(description duration_minutes price)
SERVICE_FIELD_PROMPTS=("Descripción" "Duración (minutos)" "Precio")
SERVICE_FIELD_REQUIRED=(0 1 1)
SERVICE_FIELD_VALIDATORS=("" v_positive_int v_price)


# ---------------------------------------------------------------------------
# Validators (pure: exit 0 valid / 1 invalid, Spanish error to stderr)
# ---------------------------------------------------------------------------
# Regex cheat-sheet (todos están anclados con ^ ... $):
#   v_nonempty          -> rechaza cadenas en blanco (usa is_blank).
#   v_country           -> exactamente 2 letras ASCII (may/min).
#   v_email             -> local@dominio.tld sin espacios ni @ adicional.
#   v_phone             -> E.164: '+' seguido de 8 a 15 dígitos.
#   v_url               -> http:// o https:// sin espacios.
#   v_messenger_platform-> enum: whatsapp | telegram.
#   v_symbol            -> rechaza vacío.
#   v_latitude/longitude-> rango decimal vía _decimal_range.
#   v_currency          -> exactamente 3 letras MAYÚSCULAS (ISO 4217).
#   v_timezone          -> ruta IANA: Area/Location[/Sub] con segmentos
#                          que arrancan en mayúscula. Acepta "UTC" y
#                          hasta 2 segmentos "/" (ej. America/Argentina/Buenos_Aires).
#                          NO acepta "GMT-3" (eso es POSIX TZ, no IANA).
#   v_positive_int      -> entero > 0; $((10#$1)) evita parsing octal.
#   v_price             -> decimal >= 0 (acepta 0).
#   v_payment_list      -> al menos 1 ítem no vacío separado por coma.
#   v_hhmm              -> 24h HH:MM estricto (01-09 OK, "9:00" NO).
#   v_time_pair         -> inicio < cierre, ambos v_hhmm válidos.

v_nonempty() { is_blank "$1" && { echo 'Error: este campo no puede estar vacío.' >&2; return 1; }; return 0; }
v_country() { [[ $1 =~ ^[A-Za-z]{2}$ ]] && return 0; echo 'Error: el país debe tener exactamente dos letras (ej. AR).' >&2; return 1; }
v_email() { [[ $1 =~ ^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$ ]] && return 0; echo 'Error: el correo electrónico no tiene un formato válido.' >&2; return 1; }
v_phone() { [[ $1 =~ ^\+[0-9]{8,15}$ ]] && return 0; echo 'Error: el teléfono debe comenzar con + y tener entre 8 y 15 dígitos.' >&2; return 1; }
v_url() { [[ $1 =~ ^https?://[^[:space:]]+$ ]] && return 0; echo 'Error: la URL debe comenzar con http:// o https://.' >&2; return 1; }
v_messenger_platform() { case $1 in whatsapp|telegram) return 0 ;; esac; echo 'Error: la plataforma debe ser whatsapp o telegram.' >&2; return 1; }
v_symbol() { is_blank "$1" && { echo 'Error: el símbolo no puede estar vacío.' >&2; return 1; }; return 0; }

# _decimal_range val bound: chequea que |val| <= bound con parte entera y
# fraccionar explícitas. Acepta signo opcional, separa int y frac, y exige
# que frac == "0...0" cuando |int| == bound (no es un half-open abierto).
_decimal_range() {
  local val="$1" bound="$2" num int frac
  [[ $val =~ ^-?[0-9]+(\.[0-9]+)?$ ]] || return 1
  num=${val#-}
  if [[ $num == *.* ]]; then int=${num%%.*}; frac=${num#*.}; else int=$num; frac=""; fi
  int=$((10#$int))
  [ "$int" -lt "$bound" ] && return 0
  [ "$int" -gt "$bound" ] && return 1
  [ -z "$frac" ] && return 0
  # Si la parte entera iguala el bound, cualquier dígito != 0 en la fracción
  # excede el rango (ej. 90.1 falla para v_latitude con bound=90).
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
# json_escape/json_unescape son inversos para los caracteres que este
# instalador produce/consume. Reglas:
#   - LC_ALL=C fija el locale para que el cálculo por byte sea estable.
#   - Iteración por bytes con ${s:i:1} (soporte bash 3.2; mapfile no existe).
#   - Caracteres de control (<0x20, 0x7F) salen como \u00XX; el resto de
#     bytes >=0x80 (UTF-8 multibyte) pasa verbatim, así no corrompemos
#     cadenas con acentos o emoji en el JSON final.
#   - El path \u en unescape solo necesita reconstruir el byte bajo (\u00XX)
#     porque json_escape nunca emite \uXXXX de cuatro dígitos hex
#     "altos". Para el instalador no hace falta decodificar Unicode
#     completo (los strings que guardamos son ASCII + UTF-8 literal).

json_escape() {
  local s="$1"
  s=${s//\\/\\\\}
  s=${s//\"/\\\"}
  s=${s//$'\n'/\\n}
  s=${s//$'\r'/\\r}
  s=${s//$'\t'/\\t}
  if [[ "$s" =~ [[:cntrl:]] ]]; then
    local c out="" i=0 len code
    local LC_ALL=C
    len=${#s}
    while [ $i -lt "$len" ]; do
      c=${s:$i:1}
      code=$(char_code "$c")
      if [ "$code" -lt 32 ] || [ "$code" -eq 127 ]; then
        out="${out}$(printf '\\u00%02X' "$code")"
      else
        out="${out}${c}"
      fi
      i=$((i + 1))
    done
    printf '%s' "$out"
  else
    printf '%s' "$s"
  fi
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
          # hex = "\u00XX" (siempre 4 hex según nuestro escape).
          # ${hex#??} descarta "00" y deja los 2 dígitos bajos del byte.
          # Decodificar el byte alto no es necesario porque el instalador
          # nunca produce \uXXXX con plano Unicode >0x00FF.
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
# JSON value helpers

# Render a comma-separated list as a JSON array of trimmed, escaped strings.
_json_array() {
  local list="$1" item first=1
  local IFS=,
  for item in $list; do
    item=$(trim_value "$item")
    [ -z "$item" ] && continue
    [ "$first" -eq 0 ] && printf ', '
    first=0
    printf '"%s"' "$(json_escape "$item")"
  done
}

# Render a store value according to its type: s=string, n/i=number, l=list.
json_value() {
  local value="$1" type="$2"
  if [ "$value" = "null" ]; then
    printf 'null'
  elif [ "$type" = "s" ]; then
    printf '"%s"' "$(json_escape "$value")"
  elif [ "$type" = "n" ] || [ "$type" = "i" ]; then
    printf '%s' "$value"
  elif [ "$type" = "l" ]; then
    printf '['
    _json_array "$value"
    printf ']'
  fi
}
# ---------------------------------------------------------------------------

# Flat dotted-key store

# STORE contiene líneas "clave=valor" separadas por newline. La clave literal
# "null" marca un campo opcional respondido en blanco. El registro (arriba)
# controla los nombres de clave; nunca se usa input del operador como clave.

STORE=""

store_clear() { STORE=""; }

store_has() {
  local key="$1" line
  [ -z "$STORE" ] && return 1
  while IFS= read -r line || [ -n "$line" ]; do
    [ "${line%%=*}" = "$key" ] && return 0
  done <<EOF
$STORE
EOF
  return 1
}

store_get() {
  local key="$1" line
  while IFS= read -r line || [ -n "$line" ]; do
    if [ "${line%%=*}" = "$key" ]; then
      printf '%s' "${line#*=}"
      return 0
    fi
  done <<EOF
$STORE
EOF
}

store_set() {
  local key="$1" value="$2" line new_store=""
  while IFS= read -r line || [ -n "$line" ]; do
    if [ "${line%%=*}" = "$key" ]; then
      new_store="${new_store}${key}=${value}"$'\n'
    else
      new_store="${new_store}${line}"$'\n'
    fi
  done <<EOF
$STORE
EOF
  if ! store_has "$key"; then
    new_store="${new_store}${key}=${value}"$'\n'
  fi
  STORE="$new_store"
}

store_unset() {
  local key="$1" line new_store=""
  while IFS= read -r line || [ -n "$line" ]; do
    [ "${line%%=*}" = "$key" ] && continue
    new_store="${new_store}${line}"$'\n'
  done <<EOF
$STORE
EOF
  STORE="$new_store"
}

checkpoint_render() {
  local line key value out
  out="{"
  out="${out}"$'\n'"  \"version\": 1"
  while IFS= read -r line || [ -n "$line" ]; do
    [ -z "$line" ] && continue
    key=${line%%=*}
    value=${line#*=}
        out="${out},"$'\n'"  \"$(json_escape "$key")\": "
    if [ "$value" = "null" ]; then
      out="${out}null"
    elif [[ $value =~ ^-?[0-9]+(\.[0-9]+)?$ ]]; then
      out="${out}${value}"
    else
      out="${out}\"$(json_escape "$value")\""
    fi
  done <<EOF
$STORE
EOF
  out="${out}"$'\n'"}"
  printf '%s' "$out"
}

# ---------------------------------------------------------------------------
# Checkpoint I/O
# ---------------------------------------------------------------------------

checkpoint_save() {
  checkpoint_render | atomic_write "$CHECKPOINT_PATH"
}

checkpoint_load() {
  [ -f "$CHECKPOINT_PATH" ] || return 0
  local line key value version_ok=0
  while IFS= read -r line; do
    if [[ $line =~ ^[[:space:]]*\"version\"[[:space:]]*:[[:space:]]*1 ]]; then
      version_ok=1
      continue
    fi
    if [[ $line =~ ^[[:space:]]*\"([^\"]+)\"[[:space:]]*:[[:space:]]*(.*)$ ]]; then
      key="${BASH_REMATCH[1]}"
      value="${BASH_REMATCH[2]}"
      value=${value%,}
      [ "$key" = "version" ] && continue
      if [ "$value" = "null" ]; then
        store_set "$key" "null"
      elif [[ $value =~ ^\"(.*)\"$ ]]; then
        store_set "$key" "$(json_unescape "${BASH_REMATCH[1]}")"
      elif [[ $value =~ ^-?[0-9]+(\.[0-9]+)?$ ]]; then
        store_set "$key" "$value"
      fi
    fi
  done < "$CHECKPOINT_PATH"
  [ "$version_ok" -eq 1 ] && return 0
  return 1
}

validator_for_key() {
  local key="$1"
  case "$key" in
    business.name) echo v_nonempty ;;
    business.country) echo v_country ;;
    business.latitude) echo v_latitude ;;
    business.longitude) echo v_longitude ;;
    business.cover_photo_url|business.website_url) echo v_url ;;
    business.public_phone) echo v_phone ;;
    business.messenger_platform) echo v_messenger_platform ;;
    business.contact_email) echo v_email ;;
    business.currency_code) echo v_currency ;;
    business.currency_symbol) echo v_symbol ;;
    business.timezone) echo v_timezone ;;
    business.slot_interval_minutes) echo v_positive_int ;;
    business.accepted_payment_methods) echo v_payment_list ;;
    staff.*.phone) echo v_phone ;;
    staff.*.email) echo v_email ;;
    services.*.duration_minutes) echo v_positive_int ;;
    services.*.price) echo v_price ;;
    hours.*.open|hours.*.close|staff.*.sched.*.open|staff.*.sched.*.close) echo v_hhmm ;;
  esac
}

revalidate_all() {
  local line key value validator
  while IFS= read -r line || [ -n "$line" ]; do
    [ -z "$line" ] && continue
    key=${line%%=*}
    value=${line#*=}
    [ "$value" = "null" ] && continue
    validator=$(validator_for_key "$key")
    if [ -n "$validator" ]; then
      if ! $validator "$value" >/dev/null 2>&1; then
        store_unset "$key"
      fi
    fi
  done <<EOF
$STORE
EOF
}

# ---------------------------------------------------------------------------
# Atomic write + path resolution
# ---------------------------------------------------------------------------

atomic_write() {
  local dest="$1"
  # Tmp junto al destino garantiza misma partición -> mv atómico POSIX.
  # El trap EXIT/INT/TERM/HUP limpia el tmp si el script aborta entre
  # la apertura y el rename. CURRENT_TMP se borra en ambos paths
  # (éxito y fallo) para que el trap posterior no borre el archivo
  # final ya movido. Script single-threaded: no hay race entre llamadas.
  # Durabilidad: si el sistema cae justo después del mv, queda el
  # archivo viejo (parseable) o el nuevo (parseable); no hay estado
  # intermedio corrupto. No hacemos fsync del directorio porque el
  # loader Go de Fase 5 tolera checkpoint ausente con un mensaje claro.
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
# Prompt engine + helpers
# ---------------------------------------------------------------------------

prompt_yes_no() {
  local prompt="$1" raw
  while true; do
    printf '%s' "$prompt"
    if ! read -r raw; then
      echo '' >&2
      echo 'Instalación cancelada. El checkpoint se conserva.' >&2
      exit 1
    fi
    raw=$(trim_value "$raw")
    case "$raw" in
      [sS]|si|sí) return 0 ;;
      [nN]|no|"") return 1 ;;
    esac
    echo 'Ingrese s para sí o n para no.' >&2
  done
}

prompt_field() {
  local var="$1" prompt="$2" validator="$3" mode="$4" default="${5:-}" transform="${6:-}" raw value
  while true; do
    printf '%s' "$prompt"
    [ "$mode" = "default" ] && [ -n "$default" ] && printf ' [%s]' "$default"
    printf ': '
    if ! read -r raw; then
      echo '' >&2
      echo 'Instalación cancelada. El checkpoint se conserva.' >&2
      exit 1
    fi
    value=$(trim_value "$raw")
    if is_blank "$value"; then
      case "$mode" in
        required)
          echo 'Error: este campo es obligatorio.' >&2
          continue
          ;;
        default)
          value="$default"
          ;;
        optional)
          store_set "$var" "null"
          checkpoint_save
          return 0
          ;;
      esac
    fi
    if [ -n "$validator" ]; then
      if ! $validator "$value"; then
        continue
      fi
    fi
    if [ -n "$transform" ]; then
      value=$($transform "$value")
    fi
    store_set "$var" "$value"
    checkpoint_save
    return 0
  done
}

prompt_day_hours() {
  local name="$1" key_open="$2" raw start end
  local key_close="${key_open%.open}.close"
  while true; do
    printf 'Horario para %s (cerrado/no trabaja o HH:MM-HH:MM): ' "$name"
    if ! read -r raw; then
      echo '' >&2
      echo 'Instalación cancelada. El checkpoint se conserva.' >&2
      exit 1
    fi
    raw=$(trim_value "$raw")
    if [ "$raw" = "cerrado" ] || [ "$raw" = "c" ] || [ "$raw" = "no" ]; then
      store_set "$key_open" "null"
      checkpoint_save
      return 0
    fi
    start=${raw%%-*}
    end=${raw##*-}
    if v_hhmm "$start" && v_hhmm "$end" && v_time_pair "$start" "$end"; then
      store_set "$key_open" "$start"
      store_set "$key_close" "$end"
      checkpoint_save
      return 0
    fi
  done
}

# ---------------------------------------------------------------------------
# ---------------------------------------------------------------------------
# Section runners
# ---------------------------------------------------------------------------

run_business_section() {
  local i=0 n=${#BP_KEYS[@]} field key validator mode default transform prompt
  while [ $i -lt "$n" ]; do
    field=${BP_KEYS[i]}
    key="business.$field"
    store_has "$key" && { i=$((i + 1)); continue; }
    validator=${BP_VALIDATORS[i]}
    if [ "${BP_REQUIRED[i]}" -eq 1 ]; then mode=required; else mode=optional; fi
    default=${BP_DEFAULTS[i]}
    [ -n "$default" ] && mode=default
    prompt=${BP_PROMPTS[i]}
    transform=""
    case "$field" in country|currency_code) transform=t_upper ;; esac
    prompt_field "$key" "$prompt" "$validator" "$mode" "$default" "$transform"
    i=$((i + 1))
  done
}

staff_entry_complete() {
  local i="$1" idx day
  store_has "staff.$i.name" || return 1
  for idx in 0 1 2; do
    store_has "staff.$i.${STAFF_FIELD_KEYS[idx]}" || return 1
  done
  for day in "${STAFF_DAY_ORDER[@]}"; do
    store_has "staff.$i.sched.$day.open" || return 1
  done
  return 0
}

service_entry_complete() {
  local i="$1" idx
  store_has "services.$i.name" || return 1
  for idx in 0 1 2; do
    store_has "services.$i.${SERVICE_FIELD_KEYS[idx]}" || return 1
  done
  return 0
}

run_hours_section() {
  local i=0 name key_open
  while [ $i -lt 7 ]; do
    name=${DAY_LABELS[i]}
    key_open="hours.${DAY_NAMES[i]}.open"
    store_has "$key_open" && { i=$((i + 1)); continue; }
    prompt_day_hours "$name" "$key_open"
    i=$((i + 1))
  done
}

run_staff_section() {
  local i=0 count=0 idx day name_key key key_open raw value finished=0
  while [ $finished -eq 0 ]; do
    name_key="staff.$i.name"
    if store_has "$name_key"; then
      if staff_entry_complete "$i"; then
        count=$((count + 1))
        i=$((i + 1))
        continue
      fi
    else
      if [ "$count" -gt 0 ] || [ "$i" -gt 0 ]; then
        printf 'Nombre del profesional (dejar en blanco para terminar): '
        if ! read -r raw; then
          echo '' >&2
          echo 'Instalación cancelada. El checkpoint se conserva.' >&2
          exit 1
        fi
        raw=$(trim_value "$raw")
        if is_blank "$raw"; then
          finished=1
          continue
        fi
        if ! v_nonempty "$raw"; then
          continue
        fi
        store_set "$name_key" "$raw"
        checkpoint_save
      else
        while true; do
          printf 'Nombre del profesional: '
          if ! read -r raw; then
            echo '' >&2
            echo 'Instalación cancelada. El checkpoint se conserva.' >&2
            exit 1
          fi
          raw=$(trim_value "$raw")
          if is_blank "$raw"; then
            echo 'Debe ingresar al menos un profesional.' >&2
            continue
          fi
          store_set "$name_key" "$raw"
          checkpoint_save
          break
        done
      fi
    fi
    for idx in 0 1 2; do
      key="staff.$i.${STAFF_FIELD_KEYS[idx]}"
      store_has "$key" && continue
      mode=optional
      [ "${STAFF_FIELD_REQUIRED[idx]}" -eq 1 ] && mode=required
      prompt_field "$key" "${STAFF_FIELD_PROMPTS[idx]}" "${STAFF_FIELD_VALIDATORS[idx]}" "$mode"
    done
    for idx in 0 1 2 3 4 5 6; do
      day=${STAFF_DAY_ORDER[idx]}
      key_open="staff.$i.sched.$day.open"
      store_has "$key_open" && continue
      prompt_day_hours "${STAFF_DAY_LABELS[idx]}" "$key_open"
    done
    count=$((count + 1))
    i=$((i + 1))
  done
}

run_services_section() {
  local i=0 count=0 idx name_key key raw value finished=0
  while [ $finished -eq 0 ]; do
    name_key="services.$i.name"
    if store_has "$name_key"; then
      if service_entry_complete "$i"; then
        count=$((count + 1))
        i=$((i + 1))
        continue
      fi
    else
      if [ "$count" -gt 0 ] || [ "$i" -gt 0 ]; then
        printf 'Nombre del servicio (dejar en blanco para terminar): '
        if ! read -r raw; then
          echo '' >&2
          echo 'Instalación cancelada. El checkpoint se conserva.' >&2
          exit 1
        fi
        raw=$(trim_value "$raw")
        if is_blank "$raw"; then
          finished=1
          continue
        fi
        if ! v_nonempty "$raw"; then
          continue
        fi
        store_set "$name_key" "$raw"
        checkpoint_save
      else
        while true; do
          printf 'Nombre del servicio: '
          if ! read -r raw; then
            echo '' >&2
            echo 'Instalación cancelada. El checkpoint se conserva.' >&2
            exit 1
          fi
          raw=$(trim_value "$raw")
          if is_blank "$raw"; then
            echo 'Debe ingresar al menos un servicio.' >&2
            continue
          fi
          store_set "$name_key" "$raw"
          checkpoint_save
          break
        done
      fi
    fi
    for idx in 0 1 2; do
      key="services.$i.${SERVICE_FIELD_KEYS[idx]}"
      store_has "$key" && continue
      mode=optional
      [ "${SERVICE_FIELD_REQUIRED[idx]}" -eq 1 ] && mode=required
      prompt_field "$key" "${SERVICE_FIELD_PROMPTS[idx]}" "${SERVICE_FIELD_VALIDATORS[idx]}" "$mode"
    done
    count=$((count + 1))
    i=$((i + 1))
  done
}
# ---------------------------------------------------------------------------
# JSON assembly, summary and finalization
# ---------------------------------------------------------------------------

count_staff() {
  local i=0
  while store_has "staff.$i.name"; do
    i=$((i + 1))
  done
  printf '%s' "$i"
}

count_services() {
  local i=0
  while store_has "services.$i.name"; do
    i=$((i + 1))
  done
  printf '%s' "$i"
}

render_setup_business() {
  local i=0 field key value
  printf '{\n'
  while [ $i -lt ${#BP_KEYS[@]} ]; do
    field=${BP_KEYS[i]}
    key="business.$field"
    value=$(store_get "$key")
    [ $i -gt 0 ] && printf ',\n'
    printf '  "%s": %s' "$(json_escape "$field")" "$(json_value "$value" "${BP_TYPES[i]}")"
    i=$((i + 1))
  done
  printf ',\n  "business_hours": {\n'
  i=0
  while [ $i -lt 7 ]; do
    field=${DAY_NAMES[i]}
    key="hours.$field.open"
    value=$(store_get "$key")
    [ $i -gt 0 ] && printf ',\n'
    if [ "$value" = "null" ] || [ -z "$value" ] || ! store_has "$key"; then
      printf '    "%s": null' "$(json_escape "$field")"
    else
      printf '    "%s": {"open": "%s", "close": "%s"}'         "$(json_escape "$field")"         "$(json_escape "$value")"         "$(json_escape "$(store_get "hours.$field.close")")"
    fi
    i=$((i + 1))
  done
  printf '\n  }\n}\n'
}

render_setup_staff() {
  local i=0 first=1 idx day value open close day_first
  printf '[\n'
  while store_has "staff.$i.name"; do
    [ "$first" -eq 0 ] && printf ',\n'
    first=0
    printf '  {\n'
    printf '    "name": %s,\n' "$(json_value "$(store_get "staff.$i.name")" s)"
    value=$(store_get "staff.$i.role_specialty")
    printf '    "role_specialty": %s,\n' "$(json_value "$value" s)"
    printf '    "status": "active",\n'
    value=$(store_get "staff.$i.email")
    printf '    "email": %s,\n' "$(json_value "$value" s)"
    value=$(store_get "staff.$i.phone")
    printf '    "phone": %s,\n' "$(json_value "$value" s)"
    printf '    "specialties": [],\n'
    printf '    "schedule": ['
    day_first=1
    for day in 0 1 2 3 4 5 6; do
      if store_has "staff.$i.sched.$day.open"; then
        open=$(store_get "staff.$i.sched.$day.open")
        if [ "$open" != "null" ]; then
          close=$(store_get "staff.$i.sched.$day.close")
          [ "$day_first" -eq 0 ] && printf ','
          day_first=0
          printf '\n      {"day_of_week": %s, "start_time": "%s", "end_time": "%s"}'             "$day" "$(json_escape "$open")" "$(json_escape "$close")"
        fi
      fi
    done
    printf '\n    ]\n'
    printf '  }'
    i=$((i + 1))
  done
  printf '\n]\n'
}

render_setup_services() {
  local i=0 first=1 idx value
  printf '[\n'
  while store_has "services.$i.name"; do
    [ "$first" -eq 0 ] && printf ',\n'
    first=0
    printf '  {\n'
    printf '    "name": %s,\n' "$(json_value "$(store_get "services.$i.name")" s)"
    value=$(store_get "services.$i.description")
    printf '    "description": %s,\n' "$(json_value "$value" s)"
    value=$(store_get "services.$i.duration_minutes")
    printf '    "duration_minutes": %s,\n' "$(json_value "$value" i)"
    value=$(store_get "services.$i.price")
    printf '    "price": %s,\n' "$(json_value "$value" n)"
    printf '    "is_active": 1\n'
    printf '  }'
    i=$((i + 1))
  done
  printf '\n]\n'
}

run_summary_confirm() {
  local name phone email
  name=$(store_get "business.name")
  phone=$(store_get "business.public_phone")
  email=$(store_get "business.contact_email")
  [ "$phone" = "null" ] && phone=""
  [ "$email" = "null" ] && email=""
  printf '\n'
  echo "Resumen de la configuración:"
  printf '  Negocio: %s\n' "$name"
  [ -n "$phone" ] && printf '  Teléfono: %s\n' "$phone"
  [ -n "$email" ] && printf '  Correo: %s\n' "$email"
  printf '  Profesionales: %s\n' "$(count_staff)"
  printf '  Servicios: %s\n' "$(count_services)"
  printf '\n'
  if ! prompt_yes_no "¿Confirmar y guardar? [S]í/[n]o"; then
    echo 'Configuración descartada. El checkpoint se conserva.' >&2
    exit 1
  fi
}

finalize() {
  if ! mkdir -p "$SETUP_DIR"; then
    echo 'Error: no se pudo crear el directorio de configuración. El checkpoint se conserva.' >&2
    return 1
  fi
  if ! render_setup_business | atomic_write "$SETUP_DIR/setup_business.json"; then
    echo 'Error: no se pudo guardar setup_business.json. El checkpoint se conserva.' >&2
    return 1
  fi
  if ! render_setup_staff | atomic_write "$SETUP_DIR/setup_staff.json"; then
    echo 'Error: no se pudo guardar setup_staff.json. El checkpoint se conserva.' >&2
    return 1
  fi
  if ! render_setup_services | atomic_write "$SETUP_DIR/setup_services.json"; then
    echo 'Error: no se pudo guardar setup_services.json. El checkpoint se conserva.' >&2
    return 1
  fi
  # Validación ANTES de eliminar el checkpoint: si el JSON no valida,
  # el checkpoint se preserva para poder reanudar sin perder datos.
  if command -v jq >/dev/null 2>&1; then
    if ! jq empty "$SETUP_DIR/setup_business.json" >/dev/null 2>&1 || \
       ! jq empty "$SETUP_DIR/setup_staff.json" >/dev/null 2>&1 || \
       ! jq empty "$SETUP_DIR/setup_services.json" >/dev/null 2>&1; then
      echo 'Error: los archivos generados no son JSON válido. El checkpoint se conserva.' >&2
      return 1
    fi
  elif command -v python3 >/dev/null 2>&1; then
    if ! python3 -m json.tool "$SETUP_DIR/setup_business.json" >/dev/null 2>&1 || \
       ! python3 -m json.tool "$SETUP_DIR/setup_staff.json" >/dev/null 2>&1 || \
       ! python3 -m json.tool "$SETUP_DIR/setup_services.json" >/dev/null 2>&1; then
      echo 'Error: los archivos generados no son JSON válido. El checkpoint se conserva.' >&2
      return 1
    fi
  fi
  rm -f "$CHECKPOINT_PATH"
  echo "Configuración guardada en:"
  echo "  $SETUP_DIR/setup_business.json"
  echo "  $SETUP_DIR/setup_staff.json"
  echo "  $SETUP_DIR/setup_services.json"
}

# Entry points
# ---------------------------------------------------------------------------

setup_files_exist() {
  [ -f "$SETUP_DIR/setup_business.json" ] && \
  [ -f "$SETUP_DIR/setup_staff.json" ] && \
  [ -f "$SETUP_DIR/setup_services.json" ]
}

prompt_rsq() {
  local raw
  while true; do
    printf '[R]eanudar / [S]tart over / [Q]uit: '
    if ! read -r raw; then
      echo '' >&2
      echo 'Instalación cancelada. El checkpoint se conserva.' >&2
      exit 1
    fi
    raw=$(trim_value "$raw")
    case "$raw" in
      [rR])
        if checkpoint_load; then
          revalidate_all
          echo 'Reanudando configuración...' >&2
          return 0
        fi
        echo 'Error: checkpoint no reconocido. Elija S para empezar de nuevo o Q para salir.' >&2
        ;;
      [sS])
        rm -f "$CHECKPOINT_PATH"
        store_clear
        return 0
        ;;
      [qQ])
        exit 0
        ;;
      *)
        echo 'Opción inválida. Ingrese R, S o Q.' >&2
        ;;
    esac
  done
}

prompt_reconfigure() {
  local raw
  while true; do
    printf 'Ya existe una configuración. ¿Sobrescribir? (s/N): '
    if ! read -r raw; then
      echo '' >&2
      exit 1
    fi
    raw=$(trim_value "$raw")
    case "$raw" in
      [sS]|si|sí) return 0 ;;
      [nN]|no|"") return 1 ;;
    esac
    echo 'Ingrese s para sí o n para no.' >&2
  done
}

run_setup() {
  resolve_paths || exit 1
  mkdir -p "$SETUP_DIR"
  if [ -f "$CHECKPOINT_PATH" ]; then
    prompt_rsq
  elif setup_files_exist; then
    if ! prompt_reconfigure; then
      exit 0
    fi
  fi
  run_business_section
  run_hours_section
  run_staff_section
  run_services_section
  run_summary_confirm
  finalize
}

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
    --setup-only|"") run_setup "$@" ;;
    *) echo "Error: argumento desconocido: $1" >&2; exit 1 ;;
  esac
}

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
  main "$@"
fi
