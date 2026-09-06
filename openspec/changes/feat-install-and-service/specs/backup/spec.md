# backup Specification

> **Change**: feat-install-and-service · Fase 5
> **Domain**: backup (NEW — no prior canonical spec in `openspec/specs/`)
> **Reference**: PRD §3.5 (backups bajo el data dir), ADR-0005 (backup manual, sin scheduler), ADR-0001 (no Docker); `docs/deployment.md`
> **DoD coverage**: items 10, 11

## Purpose

Proveer un script de backup portable (`scripts/backup.sh`) que cualquier operador puede ejecutar en Linux o macOS con las herramientas más básicas posibles, produciendo un respaldo consistente de la base de datos de reservas (`reservas.db`, D5) y un procedimiento de restauración verificable. El backup es manual: no hay scheduler ni automatización (ADR-0005); la guía de scheduling opcional es tarea del cliente documentada en `maintenance.md`.

## Requirements

### REQ-BKP-001 — Prerrequisitos mínimos y portables: bash, sqlite3, gzip

`backup.sh` MUST ejecutarse correctamente con solo `bash`, `sqlite3` y `gzip` en el `PATH`, en Linux y macOS. Si falta alguno de los tres, el script MUST fallar con un mensaje claro nombrando la herramienta faltante, antes de tocar la base de datos. El script MUST NOT depender de herramientas adicionales (jq, python, rsync, etc.) ni de Bash 4+ (mismo piso 3.2 que `install.sh`).

#### Scenario: Corre con solo bash, sqlite3, gzip (DoD 11)

- GIVEN un contenedor/matriz mínima con solo bash, sqlite3 y gzip disponibles
- WHEN se ejecuta `bash scripts/backup.sh <db-path>`
- THEN el backup se produce sin errores

#### Scenario: Falta sqlite3 y el error lo nombra

- GIVEN un entorno sin `sqlite3` en el PATH
- WHEN se ejecuta `backup.sh`
- THEN el script sale no-cero con un mensaje que nombra `sqlite3` como prerrequisito faltante y no modifica la DB ni crea archivos

### REQ-BKP-002 — Snapshot consistente vía `sqlite3 .backup` + gzip

`backup.sh` MUST producir el respaldo usando `sqlite3 {db} ".backup '{tmp}'"` (copia consistente, segura con la DB en uso en modo WAL) seguido de `gzip`, generando `backups/reservas-YYYYMMDD.db.gz` dentro del directorio del data dir (directorio padre de la DB), donde `YYYYMMDD` es la fecha local del día de ejecución. El script MUST aceptar la ruta de la DB como argumento (la línea de comando sugerida en el log post-install de `install-service` REQ-INS-010 la referencia). Si la ruta de la DB no existe (p.ej. el directorio o el archivo no están), el script MUST fallar con mensaje claro indicando la ruta buscada; el directorio `backups/` MUST crearse si no existe. Re-ejecutar el mismo día produce el mismo nombre de archivo y el script MUST completar con exit 0 (el snapshot de ese día se reemplaza).

#### Scenario: Backup diario en el data dir (DoD 10)

- GIVEN la DB activa del servicio en `{DATA_DIR}/reservas.db` (modo WAL, servicio corriendo)
- WHEN se ejecuta `bash scripts/backup.sh {DATA_DIR}/reservas.db`
- THEN se crea `{DATA_DIR}/backups/reservas-YYYYMMDD.db.gz` con la fecha local y la DB activa no se corrompe ni se bloquea

#### Scenario: Ruta de DB inexistente falla con mensaje claro

- GIVEN una ruta de DB que no existe (directorio o archivo ausente)
- WHEN se ejecuta `backup.sh <ruta>`
- THEN el script sale no-cero con un mensaje que muestra la ruta buscada y no crea `backups/`

#### Scenario: Re-run el mismo día es exitoso

- GIVEN ya existe `backups/reservas-20260906.db.gz` de una ejecución anterior hoy
- WHEN se ejecuta `backup.sh` de nuevo con la misma DB
- THEN el script termina exit 0 y el archivo del día queda actualizado con el snapshot más reciente

### REQ-BKP-003 — Respaldo restaurable y verificable

Un respaldo producido por `backup.sh` MUST ser restaurable: al descomprimirlo (`gunzip`) el archivo resultante MUST pasar un integrity check de SQLite (`PRAGMA integrity_check` devuelva `ok`, o `.recover` equivalente) tanto en Linux como en macOS. Los datos restaurados MUST ser consultables (la DB restaurada abre y responde a SELECTs).

#### Scenario: Restauración con integrity check pasa en ambos OS (DoD 10)

- GIVEN `reservas-YYYYMMDD.db.gz` producido en Linux
- WHEN se descomprime con `gunzip` y se ejecuta `sqlite3 <restaurada> "PRAGMA integrity_check;"` en Linux y en macOS
- THEN el resultado es `ok` en ambos sistemas

#### Scenario: La DB restaurada contiene los datos respaldados

- GIVEN un backup tomado con la DB conteniendo al menos una reserva
- WHEN se restaura a una ruta nueva y se consulta
- THEN los datos preexistentes (reservas/clientes) están presentes

### REQ-BKP-004 — Backup manual, sin scheduler (ADR-0005)

`backup.sh` MUST NOT instalar, configurar ni ejecutar ningún mecanismo de scheduling (cron, launchd timer, systemd timer). El script es una ejecución única por invocación. La automatización opcional es una tarea del cliente documentada como guía en `docs/maintenance.md` (capability `install-docs`), no una función del script.

#### Scenario: El script no registra scheduling

- GIVEN un host limpio donde se ejecuta `backup.sh`
- WHEN se revisan cron/timers del usuario antes y después de la ejecución
- THEN no hay entradas nuevas de scheduling creadas por el script
