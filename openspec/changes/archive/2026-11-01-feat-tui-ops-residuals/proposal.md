# Proposal: feat-tui-ops-residuals — cierre de residuales operativos de la TUI (audit persistido, day-keys, purge-inactive)

> **Change:** feat-tui-ops-residuals
> **Status:** proposed (spec phase) — doc-first; el apply de código va en un change posterior
> **Inputs:** `odd/tasks/feat-tui-ops-residuals.md` (especificación autoritativa, S1–S4 congeladas), `docs/PRD.md` §3.8.8 / §7 Fase N, ADR-0010, ADR-0015, ADR-0016, ADR-0013
> **Other artifacts:** `design.md`, `specs/tui-operational-residuals/spec.md`, `tasks.md`
> **Evidence base:** cada afirmación cita `path:line` (inventario completo en "Evidence Inventory")

---

## Executive Summary

La TUI operacional de cuentas (`mcp-server admin tui`) entregó su MVP el 2026-09-16 con cinco capacidades (seed del owner, Add Staff, Deactivate, listados, Transfer Ownership, Add Yourself as Client — `docs/PRD.md:1343`, ADR-0016 Decision 1). Al cerrar ese MVP quedaron **tres residuales explícitos** y una **línea stale** en el PRD:

1. **S1 — audit log persistido + vista read-only.** Hoy toda la auditoría de cuentas vive solo en `slog` estructurado (`internal/repository/accounts.go:81,110,260,301,378`; `internal/application/usecase/maintenance_audit.go:59`; `internal/application/usecase/registration_audit.go:52`). ADR-0016 Decision 1 lista "Audit log view" como capacidad entregable, pero la tabla `accounts` no tiene tabla hermana de auditoría (`internal/db/schema.go:123-133`) y el menú TUI no expone la vista (`internal/tui/view.go:82-91`).
2. **S2 — normalización de day-keys.** Conviven dos codificaciones del día de la semana: `time.Weekday` / `schedules.day_of_week` en `0..6` (domingo=0) y `business_hours` en `"1".."7"` (lunes=1), con `entity.ProfileDayKey` (`internal/domain/entity/business_profile.go:84-92`) como único traductor. Los sitios crudos `int(...Weekday())` (`internal/domain/service/slot_context.go:89`, `internal/domain/service/availability.go:108`, `internal/domain/service/booking_time_validator.go:116`) alimentan `schedules` (correcto por casualidad) pero no hay un pin que impida pasarlos a `GetOpenClose`. ADR-0016 Decision 3.4 pide "normalizar la asimetría … en un único punto de traducción, con tests".
3. **S3 — `purge-inactive`.** El borrado duro de cuentas está prohibido en el MVP (ADR-0016 Decision 1: "Nunca hard delete") y diferido al follow-up Fase 2+ "con confirmación extra" (`docs/PRD.md:904`, `docs/PRD.md:1364`, ADR-0016 Decision 2). El repo expone `Deactivate` pero **no** `Delete` (`internal/domain/repository/accounts.go:11-40`; `internal/admin/accounts.go:30-34` documenta que el hard delete está ausente a propósito).
4. **S4 — línea stale del PRD.** `docs/PRD.md:864` marca "⏳ Fase 2+ — TUI menú operacional con seed del owner — pendiente", contradiciendo `docs/PRD.md:1343` (MVP ENTREGADO 2026-09-16, ADR-0016).

Este change **cierra la fase doc-first**: fija spec (GIVEN/WHEN/THEN), decisiones de diseño con evidencia contra código, plan de apply en slices ≤ ~400 líneas, y la corrección de una sola línea del PRD. **No toca Go, no toca schema en esta fase** (el apply es un change posterior, T3/T4 de `odd/tasks/feat-tui-ops-residuals.md`).

## Business Problem

- **La auditoría no es consultable por el operador.** ADR-0010 y ADR-0016 venden la "Audit log view" como capacidad del TUI, pero el único consumidor del audit es el journal del servicio (`slog`). Un operador que quiere responder "¿quién desactivó esta cuenta?" tiene que `journalctl` o `grep` sobre logs rotados — el mismo trabajo manual que ADR-0015/0016 vinieron a eliminar (`docs/architecture/0010-admin-tui.md` Decision "Audit log view"; `docs/architecture/0016-admin-tui-scope.md` Decision 1).
- **El audit es efímero y no sobrevive a la rotación.** Las mutaciones críticas de `accounts` emiten a `slog` (`internal/repository/accounts.go:110` y ss.), sin durabilidad en la DB. Si el journal rota o el operador reinstala, la historia de quién creó/desactivó/transfirió una cuenta se pierde.
- **La asimetría de day-keys es un footgun latente.** `ProfileDayKey` existe y está documentado como el único traductor (`internal/domain/entity/business_profile.go:84-92`, `:108-109`: "never from a raw int(time.Weekday())"), pero no hay un tipo ni un test que ancle la convención. Un refactor futuro que pase el resultado de `int(Weekday())` (0..6) a `GetOpenClose` (espera 1..7) leería **horarios corridos un día** sin fallar en compilación. El residual lo registró el owner el 2026-09-16 (`docs/PRD.md:1343`).
- **No existe camino de limpieza física.** Las cuentas desactivadas se acumulan indefinidamente (ADR-0016 Consequences: "`purge-inactive` queda diferido: cuentas desactivadas acumulan filas"). El PRD promete un sub-comando con confirmación extra que hoy no existe.
- **El PRD miente en su changelog de fases.** `docs/PRD.md:864` declara pendiente un MVP entregado hace más de un mes; cualquier lector nuevo (o el propio owner) planifica contra un estado falso.

## Product Outcome

- **Auditoría persistida y consultable:** cada mutación de cuenta deja una fila en una tabla de auditoría dedicada (sin PII de terceros; teléfono enmascarado a sus últimos 4 dígitos cuando aparezca), y el operador puede verla **read-only** desde el menú `mcp-server admin tui` (Bubble Tea + flujo de consola sin TTY).
- **Una sola convención de day-key, anclada por tests:** se fija por spec si el proyecto adopta un tipo `entity.DayKey` o normaliza a `"1".."7"` (lunes=1) con un único punto de traducción; el blast radius queda inventariado y el pin por test impide la regresión silenciosa.
- **`purge-inactive` existe, es FK-safe y exige doble confirmación:** borra físicamente **solo** cuentas `is_active=0`, nunca un owner activo, con un paso extra de confirmación en **ambos** flujos (TTY y consola), reusando el núcleo revisado (`internal/admin`).
- **El PRD refleja el estado real:** la línea stale se reescribe para declarar el MVP entregado y los tres residuales trackeados en este change.

## Scope

### S1 — Audit log persistido + vista read-only

- **Deliverable:** tabla de auditoría (nueva DDL en `internal/db/schema.go`, expansión aditiva e idempotente vía `CREATE TABLE IF NOT EXISTS`), escritura de una fila por mutación de cuenta (creación, update, deactivate, transfer, rechazos de invariantes relevantes), y una **vista read-only** en el menú TUI (`internal/tui/view.go:82-91` + dispatch `internal/tui/model.go:605-628`).
- **Paridad de consola:** el flujo sin TTY (`cmd/mcp-server/admin_tui.go:411-433` `adminMenuOptions`) expone la misma vista, igual que las 7 capacidades actuales.
- **Evidencia actual:** el audit hoy es `slog` puro (`internal/repository/accounts.go:48` `auditAttrs`, sitios `:81,:110,:260,:301,:378`; `internal/application/usecase/maintenance_audit.go:59`; `internal/application/usecase/registration_audit.go:52`; `internal/auth/middleware.go:160` "privileged access"). La tabla `accounts` (`internal/db/schema.go:123-133`) no tiene tabla de auditoría hermana.

### S2 — Normalización de day-keys

- **Deliverable:** una única convención documentada y ejecutable. Análisis de diseño (no decidido acá): (a) introducir un tipo `entity.DayKey` con constructores/validadores, o (b) normalizar `schedules.day_of_week` mentalmente a `"1".."7"` reutilizando `ProfileDayKey` como frontera. En ambos casos: **un solo entry point** de traducción y **pin por test**.
- **Blast radius inventariado:** `internal/domain/service/slot_context.go:89`, `internal/domain/service/availability.go:108`, `internal/domain/service/booking_time_validator.go:116` (convención `schedules` 0..6), `internal/domain/service/booking_time_validator.go:119` (`entity.ProfileDayKey`), `internal/domain/entity/business_profile.go:84-135` (definición + parseo tolerante `"01"`/`"007"`), `internal/db/schema.go:60-70` (`schedules.day_of_week CHECK BETWEEN 0 AND 6`).

### S3 — `purge-inactive` sub-command

- **Deliverable:** un camino de borrado físico de cuentas `is_active=0` con confirmación extra, shape exacto a decidir por el owner (ver Open Questions): (a) una capacidad nueva del menú TUI `mcp-server admin tui` con paridad de consola, o (b) un sub-comando dedicado `mcp-server admin purge-inactive` que comparte el mismo núcleo `internal/admin`. El PRD §3.8.8 dice literalmente "sub-comando `purge-inactive`" (`docs/PRD.md:904`); ADR-0016/0010 gobiernan que la gestión de cuentas vive en la TUI, no en MCP.
- **Garantías:** FK-safe (inventario en design §3), refusa owner activo, idempotente, y nunca alcanzable por el LLM (sigue fuera del MCP — ADR-0010, ADR-0015 Decision 2).
- **Evidencia actual:** `internal/admin/accounts.go:30-34` documenta la ausencia deliberada de hard delete; `internal/domain/repository/accounts.go` y `internal/repository/accounts.go` no exponen `Delete`.

### S4 — Corrección de la línea stale del PRD

- **Deliverable:** reescritura de **exactamente** `docs/PRD.md:864` (una línea) para reflejar el MVP entregado (`docs/PRD.md:1343`, ADR-0016, 2026-09-16) y los tres residuales trackeados en este change.
- **Restricción de superficie:** no se toca ninguna otra línea de `docs/PRD.md` (ver `tasks.md` §Phase 2 y el reporte de entrega).

## Non-goals

- **La edición de perfil, servicios, profesionales y horarios/agendas queda fuera.** Es mantenimiento por Hermes (ADR-0015 Decision 2; ADR-0016 Decision 2) y no se duplica en la TUI.
- **Sin cambios en la superficie MCP ni en el RBAC de los tools.** Este change no agrega, quita ni re-gatea ningún tool MCP; `purge-inactive` y la vista de audit **no** son tools (ADR-0010, ADR-0015 Decision 2: un LLM comprometido no debe escalar privilegios).
- **Sin cambios en el modelo de identidad/roles.** `owner/admin/staff` y el single-owner invariant (ADR-0009) quedan intactos.
- **Sin migración destructiva de datos de negocio.** La única DDL nueva es aditiva (tabla de auditoría). El enfoque de S2 no migra `schedules` ni `business_hours`.
- **Sin cambios de transporte HTTP/SSE** ni del binding loopback `127.0.0.1:3000`.
- **Sin nuevas dependencias de terceros.**
- **Sin runtime de Hermes.** No se toca el canal conversacional ni la skill de recepción (ADR-0020).

## Evidence Inventory

| # | Afirmación | Evidencia (`path:line`) |
|---|---|---|
| E1 | Las mutaciones de `accounts` emiten solo `slog` estructurado | `internal/repository/accounts.go:81` (Warn second owner), `:110` (created), `:260` (updated), `:301` (deactivated), `:378` (transferred) |
| E2 | El shape del audit de cuentas es `{actor_id, target_id, target_role, ts}` | `internal/repository/accounts.go:48-58` (`auditAttrs`), `internal/repository/accounts.go:40-45` (`actorFromContext`) |
| E3 | Una mutación de mantenimiento = un registro `slog`; sin caller id (PII) | `internal/application/usecase/maintenance_audit.go:48-72` |
| E4 | El registro de clientes audita con teléfono enmascarado | `internal/application/usecase/registration_audit.go:38-56` (`maskPhone`) |
| E5 | El middleware ya emite "privileged access" con caller hasheado | `internal/auth/middleware.go:160` |
| E6 | `accounts` no tiene tabla de auditoría hermana | `internal/db/schema.go:123-133` |
| E7 | El logger de audit se cablea en el composition root | `cmd/mcp-server/main.go:369-374` (register), `:392-400` (maintenance) — el `odd` file cita `main.go:365,389`, comentarios del mismo wiring |
| E8 | El menú TUI es declarativo (una entrada por capacidad) | `internal/tui/view.go:62-71` (enum), `:74-91` (`menuItems`), `internal/tui/model.go:605-628` (dispatch) |
| E9 | El flujo de consola espeja el menú Bubble Tea | `cmd/mcp-server/admin_tui.go:411-433` (`adminMenuOptions`), `:436-470` (`runAdminMenu`), `:798-820` (`promptConfirm`) |
| E10 | Test de paridad de consola como patrón | `internal/tui/skill_console_test.go:45` (`TestRegenerateSkillForConsoleOutcomes`) |
| E11 | ADR-0016 lista "Audit log view" como capacidad y difiere hard delete | `docs/architecture/0016-admin-tui-scope.md` Decision 1 (tabla), Decision 2 |
| E12 | ADR-0016 pide normalizar la asimetría de day-keys en un único punto | `docs/architecture/0016-admin-tui-scope.md` Decision 3.4 |
| E13 | `entity.ProfileDayKey` es el traductor único documentado | `internal/domain/entity/business_profile.go:84-92`; contrato "never raw `int(time.Weekday())`" en `:100-119` |
| E14 | Parseo tolerante de day-keys legados (`"01"`, `"007"`) sin migración | `internal/domain/entity/business_profile.go:122-135` |
| E15 | Sitios crudos `int(...Weekday())` en producción | `internal/domain/service/slot_context.go:89`, `internal/domain/service/availability.go:108`, `internal/domain/service/booking_time_validator.go:116` |
| E16 | `booking_time_validator` traduce explícitamente a `profileDayKey` | `internal/domain/service/booking_time_validator.go:116-119`, `:142` |
| E17 | `schedules.day_of_week` es `CHECK BETWEEN 0 AND 6` | `internal/db/schema.go:60-70` |
| E18 | El PRD manda `purge-inactive` con confirmación extra | `docs/PRD.md:904`; backlog `docs/PRD.md:1364` |
| E19 | No hay `Delete` en el puerto/repo de cuentas | `internal/domain/repository/accounts.go:11-40`; `internal/admin/accounts.go:30-34` |
| E20 | `accounts.professional_id` no tiene FK; ninguna tabla referencia `accounts` | `internal/db/schema.go:123-129` (sin `REFERENCES`); `grep "REFERENCES accounts"` vacío |
| E21 | FKs de `bookings` a `professionals`/`services` son RESTRICT | `internal/db/schema.go:96-98` |
| E22 | La DDL es aditiva e idempotente por construcción | `internal/db/schema.go:7-249` (`CREATE TABLE ... IF NOT EXISTS` en cada builder, p. ej. `:9`, `:123`), `internal/db/database.go:178-223` (`initSchema` compone los builders) |
| E23 | El MVP de TUI+seed fue ENTREGADO 2026-09-16 | `docs/PRD.md:1343`; ADR-0016 (`docs/architecture/0016-admin-tui-scope.md`) |
| E24 | La línea `docs/PRD.md:864` está stale | `docs/PRD.md:864` vs `docs/PRD.md:1343` |
