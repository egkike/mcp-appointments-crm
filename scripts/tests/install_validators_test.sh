#!/bin/bash
# Unit tests for install.sh pure helpers and validators (PR1).

. "$(dirname "$0")/../install.sh" || exit 1

# _run_validator_silently: invoca un validador descartando stdout/stderr.
# Útil cuando solo nos importa el código de retorno.
_run_validator_silently() { "$1" "$2" >/dev/null 2>&1; }

# _assert_validator_exit: corre el validador y compara su exit code con
# el esperado. El mensaje "$3" aparece en el assertEquals de shunit2.
_assert_validator_exit() {
      local fn=$1 exp=$2 input=$3 msg=$4 rc
      _run_validator_silently "$fn" "$input"; rc=$?
      assertEquals "$msg" "$exp" "$rc"
}

# Aliases cortos para que las suites se lean compactas.
_run() { _run_validator_silently "$@"; }
_chk() { _assert_validator_exit "$@"; }

# String helpers -----------------------------------------------------------

test_trim_value() {
      assertEquals 'simple trim' 'x' "$(trim_value '  x  ')"
      assertEquals 'trailing CR' 'x' "$(trim_value "$(printf 'x\r')")"
      assertEquals 'accented pass-through' 'DÁtelier' "$(trim_value ' DÁtelier ')"
      assertEquals 'empty' '' "$(trim_value '')"
}

test_str_toupper() {
      assertEquals 'lower' 'AR' "$(str_toupper 'ar')"
      assertEquals 'upper stays' 'ARS' "$(str_toupper 'ARS')"
      assertEquals 'mixed' 'HOLA' "$(str_toupper 'HoLa')"
      assertEquals 'accented pass-through' 'áé' "$(str_toupper 'áé')"
}

test_is_blank() {
      assertTrue  'empty is blank' 'is_blank ""'
      assertTrue  'spaces are blank' 'is_blank "   "'
      assertEquals 'tab is blank' 0 "$(is_blank $'\t'; echo $?)"
      assertFalse 'text is not blank' 'is_blank "x"'
}

# BP_KEYS regression guard (REQ-IS-2 order, 18 fields) — ensures resume order matches spec
# BP_KEYS is the source of truth for resume position derivation (design §6.3).
test_bp_keys_order() {
  local expected="name industry country address latitude longitude cover_photo_url public_phone messenger_platform messenger_id contact_email website_url general_description accepted_payment_methods currency_code currency_symbol timezone slot_interval_minutes"
  assertEquals 'BP_KEYS order equals spec REQ-IS-2 (18 fields)' "$expected" "${BP_KEYS[*]}"
}

# Validators batch 1 -------------------------------------------------------

test_validators_batch1() {
      _chk v_nonempty 1 '' 'nonempty empty fails'
      _chk v_nonempty 1 '   ' 'nonempty spaces fail'
      _chk v_nonempty 0 'x' 'nonempty ok'
      assertNotEquals 'spanish error' '' "$(_run_err v_nonempty '')"

      _chk v_country 0 ar 'country ar ok'
      _chk v_country 0 AR 'country AR ok'
      _chk v_country 1 ARG 'country ARG fails'

      _chk v_email 1 'no-arroba' 'email no @ fails'
      _chk v_email 0 'a@b.c' 'email ok'

      _chk v_phone 0 '+5491122334455' 'phone ok'
      _chk v_phone 1 '12345678' 'phone no plus fails'
      _chk v_phone 1 '+123' 'phone too short'

      _chk v_url 0 'https://x.com' 'url https ok'
      _chk v_url 0 'http://x.com' 'url http ok'
      _chk v_url 1 'ftp://x' 'url ftp fails'

      _chk v_messenger_platform 0 whatsapp 'messenger whatsapp ok'
      _chk v_messenger_platform 0 telegram 'messenger telegram ok'
      _chk v_messenger_platform 1 sms 'messenger sms fails'

      _chk v_symbol 1 '' 'symbol empty fails'
      _chk v_symbol 0 '$' 'symbol dollar ok'
}

# _capture_validator_stderr: ejecuta el validador y captura solo su stderr
# (donde viven los mensajes en español). El orden "2>&1 >/dev/null" es
# intencional: redirige stderr al stdout original del subshell, luego
# cierra stdout, dejando stderr visible para la captura $().
# shellcheck disable=SC2069
_run_err() { "$1" "$2" 2>&1 >/dev/null; }

# Validators batch 2 -------------------------------------------------------

test_validators_batch2() {
      _chk v_latitude 0 0 'lat 0 ok'
      _chk v_latitude 0 90 'lat 90 ok'
      _chk v_latitude 0 -90 'lat -90 ok'
      _chk v_latitude 0 90.0 'lat 90.0 ok'
      _chk v_latitude 1 95 'lat 95 fails'
      _chk v_latitude 1 -90.1 'lat -90.1 fails'

      _chk v_longitude 0 180 'lon 180 ok'
      _chk v_longitude 0 -180 'lon -180 ok'
      _chk v_longitude 1 180.1 'lon 180.1 fails'
      _chk v_longitude 1 -180.5 'lon -180.5 fails'

      _chk v_currency 0 ARS 'currency ARS ok'
      _chk v_currency 1 ars 'currency lowercase fails'

      _chk v_timezone 0 UTC 'tz UTC ok'
      _chk v_timezone 0 'America/Argentina/Buenos_Aires' 'tz full ok'
      _chk v_timezone 1 'GMT-3' 'tz GMT-3 fails'

      _chk v_positive_int 0 30 'positive int ok'
      _chk v_positive_int 0 09 'positive int leading zero ok'
      _chk v_positive_int 1 0 'positive int zero fails'

      _chk v_price 0 0 'price zero ok'
      _chk v_price 0 1500.50 'price decimal ok'
      _chk v_price 1 -100 'price negative fails'

      _chk v_payment_list 0 'efectivo, tarjeta' 'payment ok'
      _chk v_payment_list 1 '' 'payment empty fails'
      _chk v_payment_list 1 ',,' 'payment only commas fails'
      # Edge cases: un solo método y separadores solo con whitespace.
      _chk v_payment_list 0 'efectivo' 'payment single item ok'
      _chk v_payment_list 1 '  ,  ' 'payment blank items fails'

      _chk v_hhmm 0 09:00 'hhmm ok'
      _chk v_hhmm 1 9:00 'hhmm no leading zero fails'
      _chk v_hhmm 1 '9:00 AM' 'hhmm am pm fails'
      _chk v_hhmm 1 25:00 'hhmm bad hour fails'
      # Edge cases v_timezone: profundidad y separadores mal formados.
      _chk v_timezone 0 'Europe/London' 'tz Europe/London ok'
      _chk v_timezone 1 'invalid//path' 'tz double slash fails'
      _chk v_timezone 1 'a/b/c/d' 'tz too deep fails'
}

test_v_time_pair() {
      _chk2() { v_time_pair "$1" "$2" >/dev/null 2>&1; echo $?; }
      assertEquals 'valid pair' 0 "$(_chk2 09:00 18:00)"
      assertEquals 'inverted' 1 "$(_chk2 18:00 09:00)"
      assertEquals 'same' 1 "$(_chk2 09:00 09:00)"
      assertEquals 'bad format' 1 "$(_chk2 9:00 18:00)"
      # Cross-midnight se considera inválido: el validador exige inicio
      # < cierre dentro del mismo día (22:00 -> 06:00 falla).
      assertEquals 'cross midnight' 1 "$(_chk2 22:00 06:00)"
}

# Transforms ---------------------------------------------------------------

test_transforms() {
      assertEquals 't_upper ar' 'AR' "$(t_upper 'ar')"
      assertEquals 't_upper ars' 'ARS' "$(t_upper 'ars')"
      assertEquals 't_trim' 'x' "$(t_trim '  x  ')"
}

# JSON escape/unescape -----------------------------------------------------

test_json_escape_roundtrip() {
      local s got
      for s in 'D'"'"'Átelier "La Casa"' 'a\b' $'line1\nline2' $'col1\tcol2' \
               'Corte ✂️ clásico' 'ñÁé' $'\x01' $'\x1f' ''; do
        got=$(json_unescape "$(json_escape "$s")")
        assertEquals "round-trip ${#s}" "$s" "$got"
      done
}

# Atomic write -------------------------------------------------------------

test_atomic_write() {
      local tmp dest
      tmp=$(mktemp -d)
      dest="$tmp/out.txt"
      printf 'hello world' | atomic_write "$dest"
      assertEquals 'content' 'hello world' "$(cat "$dest")"
      assertEquals 'no leftover tmp' '' "$(find "$tmp" -name '*.new.*' -print)"
      rm -rf "$tmp"
}

# Path resolution ----------------------------------------------------------

test_resolve_paths_linux() {
      local got
      got=$(HOME=/home/x XDG_CONFIG_HOME=/custom/config resolve_paths; printf '%s' "$CONFIG_DIR")
      assertEquals 'linux xdg' '/custom/config/mcp-appointments-crm' "$got"
}

test_resolve_paths_macos() {
      local got tmp
      got=$(
        tmp=$(mktemp -d)
        printf '%s\n%s' '#!/bin/bash' 'echo Darwin' > "$tmp/uname"
        chmod +x "$tmp/uname"
        HOME=/Users/x PATH="$tmp:$PATH" resolve_paths
        printf '%s' "$CONFIG_DIR"
      )
      assertEquals 'macos path' '/Users/x/Library/Application Support/MCP Appointments CRM' "$got"
}

test_resolve_paths_symlink() {
      local tmp rc
      tmp=$(mktemp -d)
      mkdir -p "$tmp/real"
      ln -s "$tmp/real" "$tmp/mcp-appointments-crm"
      rc=0
      (HOME=/home/x XDG_CONFIG_HOME="$tmp" resolve_paths) >/dev/null 2>&1 || rc=$?
      assertEquals 'symlink fatal' 1 "$rc"
      rm -rf "$tmp"
}

. "$(dirname "$0")/lib/shunit2" || exit 1