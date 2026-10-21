# Proposal: fix-v081-skill-seed-oq5

## Resumen

Dos defectos de seed, un mismo PR, antes de v0.8.1:

1. **B11** — `run_deploy` (instalación/upgrade vía `install.sh --version`) no siembra el template pristino de la skill en `<CONFIG_DIR>/templates/recepcionista-negocio/SKILL.md`. El wizard sí lo hace (vía `install_receptionist_skill`), pero un upgrade deja el directorio de configuración sin template → el cascade de la TUI falla fail-secure ("no se encontró el template"). Descubierto por el smoke B2 en v0.8.0 (backlog L18).
2. **OQ-5** — `render_setup_staff` hardcodea `"specialties": []` (install.sh ~:930) y `seed()` (Go) inserta profesionales **antes** que servicios, así que toda instalación nueva nace con la relación profesional↔servicio vacía. Con S7a (`search_professionals_advanced` filtrando por servicio) esto dejaría a cualquier instalación nueva con la tool devolviendo `[]` para todo. El alternativa (carga manual con `update_professional` en cada instalación) queda para el host demo (S8.1 del change `feat-owner-chat`), pero el origen hay que corregirlo (decisión owner 2026-10-19, Log L8/L25).

## Outcome

- Un upgrade `--version` siempre deja el template pristino de la skill en el directorio de configuración (permiso 0600/0700) previo al summary, sin cambiar el comportamiento del wizard.
- Una instalación nueva, después del wizard, nace con specialties resueltas a ids de servicios reales (o vacías si el operador eligió no asignar ninguna), sin datos huérfanos y con error semántico español si un nombre no corresponde a ningún servicio ingresado.

## Scope

- `scripts/install.sh`: seed del pristine en `run_deploy`; inversión de secciones del wizard (services → staff); prompt specialties multi-select en `run_staff_section`; `render_setup_staff` emite los nombres elegidos.
- `internal/config/setup_seeder.go`: `seed()` inserta servicios primero, construye mapa nombre→id y resuelve las specialties de cada profesional dentro de la transacción (fail-closed, mensaje semántico en español con los nombres válidos).
- Tests: `scripts/tests/install_deploy_test.sh` (seed en deploy), `scripts/tests/install_skill_test.sh`, `scripts/tests/install_e2e_test.sh` (orden nuevo del wizard), `internal/config/setup_loader_test.go` / nuevo test de integración de seed con specialties reales.
- Docs: PRD (contrato del wizard y del setup JSON), changelog 1.24 (v0.8.1).

## Non-goals

- No se instala ni re-renderiza la skill **instalada** en deploy: el render con nombre de negocio queda en el cascade de la TUI (opción 7), como hoy.
- No se toca `internal/db/**` (sin DDL; `specialties` sigue siendo JSON de ids en `professionals`).
- No se toca `internal/tui/**` / `internal/admin/**` (el cascade y el guard existen y funcionan si queda el template; B8/f7 etc. no aplican).
- OQ-4 no aplica acá (valores del host demo salen por `update_professional`, feat-owner-chat).
- No se valida la intersección en el write de bookings (S9, change propio del paquete B12).

## Constraints

- Todo SQL permite placeholders; el seed corre dentro de la transacción existente (rollback total ante cualquier fallo).
- El wizard vuelve a ser compilable y estable: `bash -n`, `shellcheck`, tests bash sintéticos (`scripts/tests/run_tests.sh`), pipeline Go (`go build ./...`, `go test -race ./...`).
- `validateForSeed` sigue pasando nombres sin romperse: `prof.Validate()` no inspecciona el contenido de specialties (evidencia E54/l37 feat-owner-chat).
