# Spec: install-scope-uninstall

## Purpose

Fijar el producto soportado de plataformas del ciclo de vida (install/uninstall)
y el contrato de `scripts/uninstall.sh`: Linux + macOS nativo, Windows por WSL,
y una desinstalación por defecto **no destructiva** y verificable.

## Requirements

### Requirement: La guía de instalación declara las plataformas soportadas

**GIVEN** un operador que consulta la instalación
**WHEN** lee `docs/installation.md` y `scripts/install.sh`
**THEN** las plataformas soportadas para instalar son Linux (systemd --user) y
macOS (launchd); Windows aparece **solo como WSL2** (con systemd habilitado y
el mismo one-liner del path Linux), y el camino binario nativo Windows queda
explicitado como no-soportado para el installer (el asset `.exe` es para
pruebas manuales del binario, ADR-0014).

#### Scenario: usuario de Windows con WSL2

WHEN el operador ejecuta el installer dentro de WSL2 con systemd habilitado
THEN el `uname` del host entrega `Linux` y el installer corre el path Linux
estándar (systemd --user), igual que en un host Linux.

#### Scenario: usuario de Windows sin WSL

WHEN el operador pide instalación sin WSL
THEN ninguna pieza del repo ofrece un path nativo de instalación; la única
opción documentada es el zip `.exe` para correr el binario a mano (sin
service/config/data sembrados).

### Requirement: `uninstall.sh` es dry-run por defecto y explícito para ejecutar

**GIVEN** una instalación existente del server
**WHEN** se corre `scripts/uninstall.sh` sin flags
**THEN** imprime el plan completo por plataforma (service, binario, dirs,
config, artefactos de Hermes si los flags lo alcancen) **sin modificar
nada**; la ejecución exige `--apply` y cada paso reporta lo que hizo.

#### Scenario: doble run es idempotente

WHEN el uninstall se corre por segunda vez (post `--apply` exitoso)
THEN reporta "nada que hacer" por artefacto ya ausente y termina 0, sin
errores ni creaciones.

### Requirement: Los datos del negocio se preservan por defecto

**GIVEN** el data dir con `reservas.db` (más `-wal/-shm`), config dir con
`.env`, `caller-id`, `audit-hash-key` y backups del operador
**WHEN** la desinstalación corre con los defaults
**THEN** ningún archivo de datos/config se borra: solo service unit, binario,
logs y artefactos copiados; el reporte final lista los **preservados con su
path** (no son "restos" sino guardados).

#### Scenario: purga explícita de datos

WHEN se corre `--apply --purge-data --yes`
THEN borra el DB y sus wal/shm SOLO si el service ya está detenido (doble gate
con la segunda confirmación "permanente y sin vuelta atrás"); con un runtime
vivo sobre la DB, falla cerrado sin borrar.

### Requirement: La desinstalación desacopla el gateway Hermes solo con flags

**GIVEN** una instalación que ya corrió la opción 7 de la TUI
**WHEN** el operador pasa `--purge-hermes-entry` (la skill, en cambio, se
borra POR DEFAULT — OQ-3: un Hermes vivo no debe quedar con instrucciones
huérfanas apuntando a un server inexistente — y `--keep-skill` la preserva)
**THEN** y solo entonces el script quita `mcp_servers.mcp-appointments` del
config del gateway con garantía byte-clean (claves ajenas y configuración
de otros servidores `mcp_servers` sobreviven; re-marshale idempotente si ya
no existe — molde `RemoveHermesTelegramOwnerBot`), y borra
`~/.hermes/skills/recepcionista-negocio/` por default (salvo `--keep-skill`).

#### Scenario: uninstall que corre ANTES de la opción 7 y con flags de purge

WHEN `--apply --purge-hermes-entry` (sin `--keep-skill`) corre en un host sin
entrada Hermes ni skill instalada
THEN ambas remociones son no-ops idempotentes (sin error): la entrada ya no
existía y la skill ya no existía.

### Requirement: El uninstall respeta PRD y verifica el estado al finalizar

**GIVEN** PRD `:1362` (dry-run default, preservar backups, verificación de
ausencia)
**WHEN** el `--apply` termina
**THEN** imprime una tabla final de verificación por artefacto que distingue
**removido** / **preservado por decisión** / **no existía**, y devuelve 0
solo si ningún "removido" volvió a existir.
