#!/bin/bash
# Test runner for install.sh unit and E2E tests.
# Discovers every *_test.sh under scripts/tests/ (excluding lib/).

set -u

ROOT_DIR="$(cd "$(dirname "$0")" && pwd)"
FAIL=0
COUNT=0

for suite in $(find "$ROOT_DIR" -maxdepth 1 -name '*_test.sh' -type f | sort); do
  COUNT=$((COUNT + 1))
  name="$(basename "$suite")"
  printf '\n== %s ==\n' "$name"
  if bash "$suite"; then
    printf 'PASS: %s\n' "$name"
  else
    printf 'FAIL: %s\n' "$name"
    FAIL=$((FAIL + 1))
  fi
done

printf '\n== Resumen ==\n'
printf 'Suites ejecutados: %d\n' "$COUNT"
printf 'Suites fallidos:   %d\n' "$FAIL"

if [ "$FAIL" -ne 0 ]; then
  exit 1
fi
exit 0
