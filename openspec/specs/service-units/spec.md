# service-units Specification

> **Change**: feat-install-and-service · Fase 5
> **Domain**: service-units (NEW — no prior canonical spec in `openspec/specs/`)
> **Reference**: PRD §3.5 (layout XDG, `.env`, unit user-level), ADR-0002 (user-level, no root), ADR-0007 (bind/port), D1 (`MCP_DB_PATH` en el unit, sin cambio en Go), D5 (`reservas.db`); `docs/deployment.md`
> **DoD coverage**: item 14

## Purpose

Definir los templates de registro de servicio user-level en `setup/service/` (nuevo directorio) que el instalador renderiza e instala: unit systemd user (Linux), LaunchAgent launchd (macOS) y placeholder NSSM (Windows, template + docs solamente — la automatización de Windows es non-goal declarado de Fase 5). Los templates cargan la configuración desde `.env` y fijan `MCP_DB_PATH` al layout XDG documentado, eliminando el DB split-brain para despliegues manejados por el servicio (D1).

## Requirements

### REQ-SU-001 — El directorio `setup/service/` contiene exactamente los 3 templates

`setup/service/` MUST contener exactamente tres archivos: `mcp-appointments-crm.service` (systemd user), `com.mcp.appointments.server.plist` (launchd) y `nssm-install.md` (placeholder Windows). Los templates son parte del repositorio y versionados con el código; el instalador los renderiza/instala al host destino (comportamiento de instalación cubierto en capability `install-service`).

#### Scenario: Contenido del directorio (DoD 14)

- GIVEN el repositorio tras esta fase
- WHEN se lista `setup/service/`
- THEN contiene exactamente `mcp-appointments-crm.service`, `com.mcp.appointments.server.plist` y `nssm-install.md`

### REQ-SU-002 — Contract de la unit systemd user

El template `mcp-appointments-crm.service` MUST declarar una unit **user-level** (instalada en `~/.config/systemd/user/`, nunca `/etc/systemd/system/` — ADR-0002) y MUST contener, como mínimo: `EnvironmentFile=%h/.config/mcp-appointments-crm/.env`, `Environment=MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db`, `ExecStart` apuntando al binario `mcp-server` en el `BIN_DIR` del usuario, y `WantedBy=default.target` para el arranque post-login/boot. El unit MUST NOT contener valores de bind/port hardcodeados que contradigan el `.env` (la fuente de `MCP_BIND`/`MCP_PORT` es el `.env`, ADR-0007). El unit MUST NOT ejecutarse como root ni referenciar rutas de sistema.

#### Scenario: Unit lleva EnvironmentFile y MCP_DB_PATH (DoD 14)

- GIVEN el template `mcp-appointments-crm.service`
- WHEN se inspecciona su contenido
- THEN contiene `EnvironmentFile=%h/.config/mcp-appointments-crm/.env` y `Environment=MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db`

#### Scenario: La unit instalada apunta al layout XDG (D1/D4)

- GIVEN el deploy completado en Linux (unit renderizada e instalada)
- WHEN el servicio arranca
- THEN el proceso `mcp-server` usa la DB `{DATA_DIR}/reservas.db` (no `./data/appointments.db`) y lee bind/port del `.env`

#### Scenario: Habilitada por defecto tras install

- GIVEN la unit instalada por el pipeline
- WHEN el host se reinicia
- THEN la unit arranca automáticamente (WantedBy=default.target + linger, ver REQ-INS-008)

### REQ-SU-003 — Contract del LaunchAgent launchd (macOS)

El template `com.mcp.appointments.server.plist` MUST declarar un LaunchAgent user-level (instalado en `~/Library/LaunchAgents/`) con Label `com.mcp.appointments.server`, arrancando el binario `mcp-server` del `BIN_DIR` del usuario con `MCP_DB_PATH` apuntando al `DATA_DIR` de macOS (`~/Library/Application Support/MCP Appointments CRM/reservas.db`, D4) y cargando bind/port equivalentes al `.env` (EnvironmentVariables o argumentos equivalentes). El plist MUST persistir entre sesiones (launchd lo hace por defecto; RunAtLoad/KeepAlive según el template).

#### Scenario: Plist user-level con DB del layout macOS

- GIVEN el template `com.mcp.appointments.server.plist`
- WHEN el instalador lo renderiza e instala en `~/Library/LaunchAgents/`
- THEN el agente arranca `mcp-server` con `MCP_DB_PATH` = `~/Library/Application Support/MCP Appointments CRM/reservas.db` y el proceso escucha en loopback con el puerto del `.env`

### REQ-SU-004 — NSSM placeholder para Windows: docs, no automatización

`setup/service/nssm-install.md` MUST ser un documento (español) que describa el registro manual del servicio en Windows con NSSM: layout `%APPDATA%\MCP Appointments CRM\` (D4), `MCP_DB_PATH` correspondiente, y la línea de comando de NSSM equivalente. El instalador MUST NOT automatizar el registro en Windows en esta fase (non-goal declarado); el documento fija el límite explícitamente para el cliente Windows.

#### Scenario: Placeholder documenta sin automatizar

- GIVEN `setup/service/nssm-install.md` y el instalador
- WHEN un cliente Windows sigue el documento
- THEN puede registrar el servicio manualmente con NSSM siguiendo los pasos; el instalador `install.sh` no ejecuta ni intenta ningún paso de registro en Windows

### REQ-SU-005 — Los templates no hardcodean rutas absolutas del host

Los templates MUST usar especifiers/variables del gestor de servicios (`%h` en systemd, `~`/expansión en launchd) o marcadores que el instalador sustituye al renderizar, de forma que el mismo template versionado sirva para cualquier usuario/host sin edición manual. Un template con una ruta absoluta hardcodeada de un host concreto MUST considerarse defecto.

#### Scenario: La misma unit sirve para dos usuarios distintos

- GIVEN el template systemd renderizado para el usuario `alice` en su VPS
- WHEN el mismo template se renderiza para el usuario `bob` en otro host
- THEN ambas instalaciones funcionan sin editar el template manualmente (`%h` resuelve al home de cada usuario)
