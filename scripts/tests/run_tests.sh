#!/bin/bash
# Test runner for install.sh unit and E2E tests.
# Discovers every *_test.sh under scripts/tests/ (excluding lib/).
#
# Nota: scripts/tests/lib/shunit2 es vendored unmodified (shunit2 2.1.x)
# y queda excluido de los conteos de líneas del installer. No modificarlo.

set -u

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
FAIL=0
COUNT=0

for suite in $(find "$ROOT_DIR" -maxdepth 1 -name '*_test.sh' -type f | sort); do
  COUNT=$((COUNT + 1))
  name="$(basename "$suite")"
  printf '\n== %s ==\n' "$name"
  if bash "$suite"; then
    # PASS explícito por suite para que un run limpio no quede silencioso.
    printf 'PASS: %s\n' "$name"
  else
    printf 'FAIL: %s\n' "$name"
    FAIL=$((FAIL + 1))
  fi
done

printf '\n== Resumen ==\n'
printf 'Suites ejecutados: %d\n' "$COUNT"
printf 'Suites fallidos:   %d\n' "$FAIL"

# Línea final de estado: OK cuando no hubo fallos, ERROR en caso contrario.
if [ "$FAIL" -eq 0 ]; then
  printf 'OK: %d/%d suites pasaron.\n' "$COUNT" "$COUNT"
  exit 0
fi
printf 'ERROR: %d/%d suites fallaron.\n' "$FAIL" "$COUNT"
exit 1
