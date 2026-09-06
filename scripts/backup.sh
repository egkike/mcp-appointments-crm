#!/bin/bash
# scripts/backup.sh — backup de la base de datos de reservas.
# Uso: backup.sh <ruta-de-la-DB>
#
# Produce {dir-db}/backups/reservas-YYYYMMDD.db.gz usando sqlite3 .backup y gzip.
# Sin scheduling, sin dependencias extra (bash 3.2+, sqlite3, gzip).

umask 077
set -u

TMP=""
TMP_GZ=""
cleanup() { rm -f "${TMP:-}" "${TMP_GZ:-}"; }
trap cleanup EXIT INT TERM HUP

usage() {
  echo "Uso: backup.sh <ruta-de-la-DB>" >&2
}

main() {
  local db_path="${1:-}"
  if [ -z "$db_path" ]; then
    usage
    exit 1
  fi

  local tool
  for tool in sqlite3 gzip; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      echo "Error: falta '$tool' en PATH" >&2
      exit 1
    fi
  done

  if [ ! -f "$db_path" ]; then
    echo "Error: no existe la DB: $db_path" >&2
    exit 1
  fi

  local backup_dir stamp final
  backup_dir="$(dirname "$db_path")/backups"
  mkdir -p "$backup_dir" || exit 1

  stamp=$(date +%Y%m%d)
  final="$backup_dir/reservas-$stamp.db.gz"

  TMP=$(mktemp "$backup_dir/.backup.XXXXXX") || exit 1
  TMP_GZ="$TMP.gz"

  sqlite3 "$db_path" ".backup '$TMP'" || { rm -f "$TMP" "$TMP_GZ"; exit 1; }
  gzip -c "$TMP" > "$TMP_GZ" || { rm -f "$TMP" "$TMP_GZ"; exit 1; }
  rm -f "$TMP"
  mv "$TMP_GZ" "$final" || exit 1

  echo "Backup: $final ($(wc -c < "$final" | tr -d '[:space:]') bytes)"
}

main "$@"
