# install-docs Specification

> **Change**: feat-install-and-service · Fase 5
> **Domain**: install-docs (NEW — no prior canonical spec in `openspec/specs/`)
> **Reference**: PRD § Fase 5, §3.5; ADR-0005 (backup manual), ADR-0007 (bind loopback), ADR-0008 (TUI no involucrado en la instalación), D1/D5 (`reservas.db`, caveat de ejecución manual); `docs/deployment.md` (referencia detallada, se mantiene)
> **DoD coverage**: items 12, 13

## Purpose

Entregar la historia de soporte completa en español: `docs/installation.md` (manual de instalación paso a paso, el camino del cliente) y `docs/maintenance.md` (manual de operación anual: backups, upgrades, logs, control del servicio, troubleshooting). `docs/deployment.md` permanece como referencia técnica detallada; `installation.md` es la ruta customer-facing. Ambos deben ser verificables de punta a punta contra el mismo flujo de DoD 1. La instalación documentada es bash puro vía `install.sh` — la TUI Bubble Tea (ADR-0008) no participa en este flujo y los docs no deben referenciarla como paso de instalación.

## Requirements

### REQ-IDOC-001 — `docs/installation.md`: instalación verificable de punta a punta (DoD 12)

`docs/installation.md` MUST existir, estar escrita en español, y documentar el flujo completo desde VPS limpio hasta servicio activo: prerrequisitos (Ubuntu 22.04+ o macOS, bash/sqlite3/gzip), el comando de instalación pinned (`curl ... | bash -s -- --version vX.Y.Z`), la verificación post-install (servicio activo, endpoint MCP, DB), y el paso de setup interactivo previo (ejecutar `bash install.sh` en terminal para completar los prompts cuando aplique). Un revisor MUST poder seguir el documento de arriba a abajo contra el flujo de DoD 1 sin adivinar ningún paso intermedio. El documento MUST declarar explícitamente el caveat de D1: una ejecución manual de `./mcp-server` (sin el servicio) usa `./data/appointments.db` por default — el camino soportado es el servicio.

#### Scenario: Un revisor sigue la instalación sin adivinar (DoD 12)

- GIVEN un VPS Ubuntu 22.04+ limpio y `docs/installation.md` abierto
- WHEN el revisor ejecuta cada comando del documento en orden
- THEN llega al mismo estado final de DoD 1 (servicio activo, endpoint respondiendo, DB en el layout XDG) sin pasos faltantes ni comandos que fallen por omisión

#### Scenario: El caveat de ejecución manual está declarado (D1)

- GIVEN `docs/installation.md`
- WHEN se lee la sección de verificación/notas
- THEN declara que `./mcp-server` directo usa `./data/appointments.db` y que el camino soportado es el servicio con `MCP_DB_PATH` al layout XDG

#### Scenario: La TUI no aparece como paso de instalación (ADR-0008)

- GIVEN `docs/installation.md`
- WHEN se revisan los pasos
- THEN ningún paso referencia la TUI Bubble Tea ni un binario TUI; la instalación es `install.sh` (bash) y el servicio

### REQ-IDOC-002 — `docs/maintenance.md`: manual de operación anual (DoD 13)

`docs/maintenance.md` MUST existir, estar escrita en español, y cubrir como mínimo: (a) ejecución y restauración de backups (`backup.sh`, verificación de integridad), (b) upgrade mediante re-ejecución de `install.sh --version vX.Y.Z` más nuevo (comportamiento REQ-INS-013), (c) inspección de logs (journalctl en Linux y `LOG_DIR`), (d) control del servicio por OS (start/stop/status/restart para systemd user y launchd), y (e) el caveat de DB de ejecución manual (D1/D5): nunca ejecutar el binario a mano junto al servicio, o se bifurca la DB a `./data/appointments.db`. El manual DEBERÍA (SHOULD) incluir troubleshooting de los casos conocidos de `systemctl --user` sin bus de usuario sobre ssh plano (riesgo 4 del proposal).

#### Scenario: Cobertura de las cinco secciones (DoD 13)

- GIVEN `docs/maintenance.md`
- WHEN se revisa su estructura
- THEN contiene secciones para backups/restore, upgrade, logs, control de servicio por OS, y el caveat de ejecución manual, en español

#### Scenario: El flujo de upgrade documentado coincide con el comportamiento real

- GIVEN una instalación activa y `docs/maintenance.md`
- WHEN el operador sigue la sección de upgrade con un tag más nuevo
- THEN el resultado es el del scenario REQ-INS-013: binario reemplazado, estado preservado, servicio activo con la versión nueva

### REQ-IDOC-003 — Unificación del nombre de DB en la documentación (D5)

Toda la documentación de producción (`installation.md`, `maintenance.md`, y las menciones existentes en `deployment.md`/`PRD.md` donde `appointments.db` aparece como DB de producción) MUST converger en `reservas.db` como nombre canónico de la DB de producción, consistente con PRD §3.5 y `backups/reservas-YYYYMMDD.db.gz`. La mención de `appointments.db` solo es aceptable como default interno de desarrollo para ejecución directa del binario (D1), explicado como caveat, nunca como ruta de producción.

#### Scenario: Ninguna doc de producción promete appointments.db

- GIVEN `docs/installation.md`, `docs/maintenance.md` y `docs/deployment.md`
- WHEN se buscan referencias a `appointments.db`
- THEN solo aparecen, si aparecen, como caveat de ejecución manual/desarrollo; toda ruta de producción usa `reservas.db`

### REQ-IDOC-004 — Scheduling de backups: guía opcional del cliente (ADR-0005)

`maintenance.md` MAY incluir una guía opcional para que el cliente automatice la ejecución periódica de `backup.sh` (cron/systemd timer), presentada claramente como tarea opcional del cliente. El sistema MUST NOT incluir scheduling automático de backups (ADR-0005, REQ-BKP-004); si la guía existe, MUST declarar que es una decisión del cliente y no una función soportada del producto.

#### Scenario: Guía opcional claramente marcada

- GIVEN `docs/maintenance.md` con una sección de scheduling
- WHEN se lee esa sección
- THEN está presentada como tarea opcional del cliente y no como función del producto, sin contradecir REQ-BKP-004
