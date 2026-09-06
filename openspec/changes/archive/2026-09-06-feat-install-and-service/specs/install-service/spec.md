# install-service Specification

> **Change**: feat-install-and-service · Fase 5
> **Domain**: install-service (NEW — no prior canonical spec in `openspec/specs/`)
> **Reference**: PRD § Fase 5, §3.5 (XDG layout); ADR-0002 (no root), ADR-0005 (backup manual), ADR-0007 (bind/port), ADR-0008 (TUI no involucrado), ADR-0014 (release assets); `docs/deployment.md` (spec de facto)
> **DoD coverage**: items 1–9 (ver scenarios)

## Purpose

`scripts/install.sh` debe completarse — extend-only, sin reescribir Fase 4 — en un pipeline de despliegue `curl | bash` que va de un VPS limpio a un servicio MCP activo que sobrevive logout y reboot: detección de OS/arch, descarga del release con verificación SHA256, registro del servicio user-level, y un log final accionable. El flujo interactivo de Fase 4 (prompts, validadores, `finalize()`, checkpoint) queda intacto; el pipeline deploy se agrega después. ADR-0008: la instalación no involucra la TUI Bubble Tea; es bash puro.

## Requirements

### REQ-INS-001 — Superficie CLI: `--setup-only` intacto, `--version` nuevo

`install.sh` MUST aceptar `--version vX.Y.Z` (tag de release pinned, formato `vMAJOR.MINOR.PATCH`, sin `latest`) como disparador del pipeline deploy. El comportamiento de `--setup-only` (default en Fase 4) y `--help` MUST permanecer idéntico a Fase 4; un argumento desconocido MUST producir error y exit no-cero, igual que hoy. `install.sh --version` (flag del instalador) y `mcp-server --version` (flag del binario, capability `binary-version`) son interfaces distintas: ambas existirán tras esta fase.

#### Scenario: Flag --version dispara el deploy (DoD 1)

- GIVEN un VPS Ubuntu 22.04+ limpio con los setup JSONs presentes
- WHEN se ejecuta `curl -fsSL <url> | bash -s -- --version v0.3.0` sin TTY
- THEN el pipeline deploy se ejecuta, exit 0, y `systemctl --user is-active mcp-appointments-crm` reporta `active` en menos de 5 minutos totales

#### Scenario: --setup-only sin cambios de Fase 4

- GIVEN `install.sh` ejecutado con `--setup-only` en una terminal (TTY)
- WHEN el operador completa los prompts
- THEN el flujo interactivo Fase 4 (captura, validación, checkpoint, `finalize()`) produce los tres JSONs exactamente como antes de esta fase

#### Scenario: Argumento desconocido sigue fallando

- GIVEN cualquier entorno
- WHEN se ejecuta `install.sh --nonsense`
- THEN el script MUST imprimir uso y salir con código no-cero

### REQ-INS-002 — Detección OS/arch mapea a exactamente los 4 assets GoReleaser

El pipeline MUST derivar OS y arch vía `uname -s` / `uname -m` y mapearlos a exactamente los 4 assets soportados (`mcp-appointments-crm_{Linux,Darwin}_{x86_64,arm64}.tar.gz`). Una combinación no mapeada (p.ej. `Windows`/`i386`) MUST fallar con un error claro en español/inglés nombrando la combinación detectada, sin descargar nada. Windows no se automatiza en esta fase (non-goal declarado; ver capability `service-units` REQ-SU-004).

#### Scenario: Matriz 4x mapeada correctamente (DoD 3)

- GIVEN `uname -s` = `Linux` y `uname -m` = `arm64`
- WHEN el pipeline compone el nombre del asset
- THEN el asset es `mcp-appointments-crm_Linux_arm64.tar.gz` descargado de `https://github.com/egkike/mcp-appointments-crm/releases/download/{tag}/{asset}`

#### Scenario: OS/arch no soportado falla con error claro (DoD 3)

- GIVEN `uname -s` = `Windows_NT` (vía subsistema) o `uname -m` = `i686`
- WHEN el pipeline intenta resolver el asset
- THEN el script MUST salir no-cero con un mensaje que nombre la combinación OS/arch detectada y NO MUST dejar archivos parciales en `BIN_DIR`

### REQ-INS-003 — Descarga verificada por SHA256 con escritura atómica

El pipeline MUST descargar el asset y `checksums.txt` del release indicado, verificar SHA256 del archive antes de extraer, y escribir el binario en `BIN_DIR` mediante escritura atómica (temp + rename, patrón existente `atomic_write`). Una verificación fallida (checksum ausente del archivo, hash distinto, o binario alterado en un byte) MUST producir exit no-cero, un error explícito con la URL para descarga manual, y NO MUST dejar binario en `BIN_DIR` (cleanup del temporal vía trap existente).

#### Scenario: Binario alterado en un byte falla verificación (DoD 4)

- GIVEN el asset descargado con un byte modificado respecto al checksum oficial
- WHEN el pipeline verifica SHA256
- THEN el script MUST salir no-cero con mensaje de verificación fallida y `BIN_DIR/mcp-server` NO MUST existir tras la ejecución

#### Scenario: checksums.txt no incluye el asset

- GIVEN el release existe pero `checksums.txt` no lista el asset resuelto
- WHEN el pipeline busca el hash esperado
- THEN el script MUST salir no-cero con mensaje explícito (no silencioso) indicando el asset faltante

### REQ-INS-004 — Setup JSONs son prerrequisito del deploy

El pipeline deploy MUST requerir los tres setup JSONs (`setup_business.json`, `setup_staff.json`, `setup_services.json`) ya finalizados en el directorio de configuración. Si faltan uno o más, el script MUST salir no-cero con un mensaje específico nombrando cada archivo faltante. Este comportamiento MUST estar cubierto por shunit2 (nueva suite deploy, estilo existente).

#### Scenario: JSONs faltantes nombrados uno a uno (DoD 2)

- GIVEN un host sin ninguno de los tres setup JSONs
- WHEN se ejecuta `install.sh --version v0.3.0`
- THEN el script MUST salir no-cero y el mensaje MUST nombrar `setup_business.json`, `setup_staff.json` y `setup_services.json`

#### Scenario: JSONs presentes habilitan el deploy

- GIVEN los tres setup JSONs finalizados (p.ej. copiados desde una sesión interactiva previa)
- WHEN se ejecuta el pipeline en el mismo host
- THEN el deploy procede sin prompts interactivos

### REQ-INS-005 — Degradación explícita sin TTY (D6)

`install.sh` MUST verificar `[ -t 0 ]`: el flujo interactivo de setup solo corre con TTY. Sin TTY (`curl | bash`), el script MUST saltar los prompts interactivos, requerir los setup JSONs (o checkpoint completado) ya presentes, y proceder directo al pipeline deploy. Si sin TTY no hay setup completo, el script MUST detenerse con un mensaje claro (español/inglés) indicando ejecutar `bash install.sh` en una terminal primero — nunca bloquear en silencio, nunca asumir respuestas de prompts.

#### Scenario: curl|bash sin TTY y sin setup se detiene con mensaje (DoD 1/D6)

- GIVEN un VPS limpio piped sin TTY y sin setup JSONs
- WHEN se ejecuta `curl ... | bash -s -- --version v0.3.0`
- THEN el script MUST salir no-cero con mensaje que indique completar el setup en una terminal; NO MUST presentar prompts ni colgar

#### Scenario: curl|bash sin TTY con setup completo despliega (DoD 1)

- GIVEN un VPS con los tres JSONs presentes (sin TTY)
- WHEN se ejecuta el pipeline deploy
- THEN el flujo salta todos los prompts y completa el despliegue hasta servicio activo

### REQ-INS-006 — `resolve_paths()` extendida: DATA_DIR, BIN_DIR, LOG_DIR por OS (D4)

`resolve_paths()` MUST resolver, además del `CONFIG_DIR` existente: `DATA_DIR`, `BIN_DIR` y `LOG_DIR` por OS — Linux: `DATA_DIR=${XDG_DATA_HOME:-$HOME/.local/share}/mcp-appointments-crm`, `BIN_DIR=${XDG_BIN_HOME:-$HOME/.local/bin}`, `LOG_DIR=${XDG_STATE_HOME:-$HOME/.local/state}/mcp-appointments-crm`; macOS: `DATA_DIR=$HOME/Library/Application Support/MCP Appointments CRM`, `LOG_DIR=$HOME/Library/Logs/MCP Appointments CRM`, `BIN_DIR=$HOME/.local/bin` (guía de PATH en docs). El rechazo existente de symlink en `CONFIG_DIR` MUST extenderse a los nuevos directorios. La DB de producción MUST ser `{DATA_DIR}/reservas.db` (D5). Si el directorio de la DB no existe, el instalador MUST crearlo con permisos owner-only (el binario ya crea directorios vía `db.NewDatabase`; el instalador no MUST fallar por un `DATA_DIR` inexistente).

#### Scenario: Layout XDG en Linux (D4)

- GIVEN Linux con `XDG_DATA_HOME` sin definir
- WHEN `resolve_paths()` corre
- THEN `DATA_DIR=$HOME/.local/share/mcp-appointments-crm` y la DB de producción es `$HOME/.local/share/mcp-appointments-crm/reservas.db`

#### Scenario: Symlink en DATA_DIR rechazado

- GIVEN `DATA_DIR` resuelto apunta a un symlink
- WHEN `resolve_paths()` valida el directorio
- THEN el script MUST rechazar con error, igual que hace hoy con `CONFIG_DIR`

#### Scenario: Directorio de DB inexistente se crea

- GIVEN un host sin `DATA_DIR` creado
- WHEN el deploy instala y arranca el servicio
- THEN el directorio existe (creado por instalador o binario) con permisos owner-only y la DB se crea allí

### REQ-INS-007 — `.env` creado si no existe, nunca sobrescrito

El instalador MUST escribir `{CONFIG_DIR}/.env` con `MCP_BIND=127.0.0.1` y `MCP_PORT=3000` **solo si el archivo no existe**. Un `.env` existente MUST quedar byte-idéntico tras cualquier re-run o upgrade (las personalizaciones del usuario sobreviven). El instalador MUST NOT escribir jamás un bind no-loopback (`127.0.0.0/8` o `::1` solamente, ADR-0007). La precedencia `env vars > .env > defaults` del binario queda intacta.

#### Scenario: .env existente no se sobrescribe (upgrade)

- GIVEN un `.env` existente con `MCP_PORT=3100` personalizado por el cliente
- WHEN se re-ejecuta `install.sh --version v0.3.1` (upgrade)
- THEN el archivo `.env` MUST quedar idéntico (puerto 3100 preservado) y el deploy usa ese valor

#### Scenario: Primer install crea .env loopback

- GIVEN un host sin `.env`
- WHEN el deploy se completa
- THEN `.env` existe con `MCP_BIND=127.0.0.1` y `MCP_PORT=3000`, permisos owner-only

### REQ-INS-008 — Registro de servicio user-level (Linux: systemd + linger)

En Linux, el instalador MUST instalar la unit systemd **user** en `~/.config/systemd/user/mcp-appointments-crm.service` (renderizada desde `setup/service/`, ver capability `service-units`), ejecutar `daemon-reload`, `enable` y `start`, y ejecutar `loginctl enable-linger $USER` para que el servicio sobreviva logout y reboot. El instalador MUST operar a nivel usuario: MUST NOT requerir ni ejecutar comandos como root (ADR-0002); cualquier paso que exigiría root MUST fallar con mensaje claro en lugar de escalar.

#### Scenario: Unidad enabled + active tras install (DoD 5)

- GIVEN el deploy completado en Linux
- WHEN se consulta `systemctl --user` para `mcp-appointments-crm`
- THEN la unit reporta `enabled` y `active`

#### Scenario: Linger habilitado (DoD 6)

- GIVEN el deploy completado en Linux
- WHEN se ejecuta `loginctl show-user $USER`
- THEN reporta `Linger=yes`

#### Scenario: Sobrevive logout y reboot simulados (DoD 5)

- GIVEN el servicio activo con linger
- WHEN se simula logout (sesión cerrada) y reinicio del sistema
- THEN el servicio se reinicia automáticamente y vuelve a `active` sin intervención

### REQ-INS-009 — Registro de servicio user-level (macOS: launchd)

En macOS, el instalador MUST instalar el LaunchAgent en `~/Library/LaunchAgents/com.mcp.appointments.server.plist` (renderizado desde `setup/service/`) y cargarlo vía `launchctl` de forma user-level. launchd persiste por defecto; no se requiere equivalente de linger.

#### Scenario: LaunchAgent cargado tras install

- GIVEN el deploy completado en macOS
- WHEN se consulta el estado del LaunchAgent `com.mcp.appointments.server`
- THEN está cargado y el proceso `mcp-server` responde en el endpoint MCP

### REQ-INS-010 — Log final de post-instalación accionable

Al completar el deploy, el script MUST imprimir un log final que incluya, como mínimo: (a) una línea de comando `backup.sh` copy-pasteable que referencie el `MCP_DB_PATH` real en uso, (b) el bloque "Recommended additional tools" no vacío en español, (c) la URL del endpoint MCP (`http://127.0.0.1:3000/mcp`) y la ruta resuelta de la DB. Declarar el `MCP_DB_PATH` exacto en el log mitiga el riesgo de DB split-brain (riesgo 5 del proposal): una ejecución manual de `./mcp-server` usaría `./data/appointments.db` en su lugar.

#### Scenario: Log final contiene los cuatro elementos (DoD 7, 8, 9)

- GIVEN el deploy completado exitosamente
- WHEN se lee la salida final del script
- THEN contiene la línea `backup.sh` con la ruta real de la DB, un bloque "Recommended additional tools" no vacío en español, la URL `http://127.0.0.1:3000/mcp` y la ruta resuelta de la DB

### REQ-INS-011 — Verificación post-install del binario instalado

El pipeline MUST verificar tras la instalación que `$BIN_DIR/mcp-server --version` imprime la versión correspondiente al tag solicitado y exit 0 (interface definida en capability `binary-version`). Una discrepancia o falla MUST producir exit no-cero del instalador con mensaje explícito.

#### Scenario: Versión instalada coincide con el tag

- GIVEN el deploy con `--version v0.3.0` completado
- WHEN el instalador ejecuta `$BIN_DIR/mcp-server --version`
- THEN la salida contiene la versión del tag y exit 0

### REQ-INS-012 — Compatibilidad Fase 4: extend-only, Bash 3.2, suites shunit2 intactas (D3)

Todo código nuevo en `install.sh` MUST respetar el piso Bash 3.2 establecido (sin arrays asociativos, `mapfile`, `declare -n`, `${var,,}`, `pipefail`, `set -e`; se mantiene `set -u`, `umask 077`, trap/cleanup existente). Las dos suites shunit2 existentes (`scripts/tests/install_validators_test.sh`, `scripts/tests/install_e2e_test.sh`) MUST pasar **sin modificación**; los tests nuevos del deploy se agregan al lado, en el mismo estilo shunit2, sin reemplazar los existentes. El motor de prompts, validadores y `finalize()` de Fase 4 MUST permanecer sin cambios funcionales.

#### Scenario: Suites Fase 4 verdes y sin modificar

- GIVEN el repositorio tras esta fase
- WHEN se ejecutan `install_validators_test.sh` e `install_e2e_test.sh`
- THEN ambas pasan y `git diff` sobre esos dos archivos de test está vacío

#### Scenario: Piso Bash 3.2 no violado

- GIVEN el `install.sh` extendido
- WHEN se revisa el código nuevo (o se ejecuta en bash 3.2 real)
- THEN no aparece ninguna construcción Bash 4+ y el script corre en macOS con /bin/bash nativo

### REQ-INS-013 — Upgrade: re-run con tag más nuevo

Re-ejecutar `install.sh --version <nuevo>` sobre una instalación existente MUST reemplazar el binario atómicamente, preservar `.env`, setup JSONs y DB, y reiniciar el servicio. El upgrade documentado en `maintenance.md` (capability `install-docs`) depende de este comportamiento.

#### Scenario: Upgrade preserva estado y reinicia servicio

- GIVEN una instalación activa con DB con datos y `.env` personalizado
- WHEN se ejecuta `install.sh --version v0.3.1`
- THEN el binario nuevo está en `BIN_DIR`, la DB y `.env` intactos, y el servicio reinicia a `active` con la versión nueva
