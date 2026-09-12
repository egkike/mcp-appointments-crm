# ADR-0016: Alcance del SDD admin-tui (identidad y cuentas)

- **Status**: accepted
- **Date**: 2026-09-12
- **Authors**: Kike
- **Related**: ADR-0009 (authorization model), ADR-0010 (admin TUI), ADR-0015 (maintenance tools)

## Context

[ADR-0010](./0010-admin-tui.md) decidió **qué** es la TUI (sub-comando del binario
principal, no invocable por el LLM, gatekeeper = admin del OS por SSH) y qué capacidades
tiene. Quedó pendiente acotar **el alcance del SDD que la implementa**, porque la TUI
arrastra el problema de identidad de todo el sistema y varias restricciones técnicas
verificadas contra código el 2026-09-12:

1. **La TUI es greenfield.** No existe `admin_tui.go`, no existe sub-comando `admin` y
   Bubble Tea no está cableado. El entry point planificado es un sub-comando del binario
   principal (`cmd/<binario>/admin_tui.go`), que opera **directo sobre `AccountsRepo`** y
   **no** es un tool MCP.

2. **Sin identidad no hay sistema.** Toda mutación de `AccountsRepo` exige
   `auth.RequireRole` desde el `ctx`. La TUI no pasa por el middleware HTTP, así que el
   **primer owner es un huevo-y-gallina**: el seed necesita un caller con rol `owner` que
   todavía no existe. Resolverlo es una restricción de diseño del SDD, no una feature
   opcional.

3. **`accounts.professional_id` no tiene FK a `professionals`.** El riesgo de
   dangling-reference es real y el operador no conoce los UUID generados por el seeder, así
   que la TUI necesita un selector de profesional, no un campo de texto libre.

4. **Asimetría de claves de día.** `business_profile`/`business_hours` usa claves `"1"`..`"7"`
   con lunes = 1, mientras `schedules.day_of_week` usa `0`..`6` con domingo = 0. Cruza el
   borde de la TUI y de los tools de mantenimiento.

5. **Nombre del entry-point sin fijar.** El binario instalado se llama `mcp-server`, mientras
   que la documentación dice `mcp-appointments-crm admin tui`. El SDD debe fijar uno solo.

6. **Presupuesto de review.** El change toca identidad, cuentas y auditoría: es sensible. El
   presupuesto canónico es 400 líneas de diff.

## Decision

**El SDD `admin-tui` cubre identidad y cuentas. Todo lo demás queda explícitamente fuera.**

### Decision 1: MVP = owner seed gateway + gestión de cuentas

| Capacidad | Alcance |
|---|---|
| **Owner seed gateway** | Primer arranque: crea el owner inicial y escribe el archivo `caller-id`. Es el desbloqueante del sistema. |
| **Add Staff** | Alta de cuenta `staff` con `phone`, `display_name` y `professional_id` elegido por selector. |
| **Deactivate** | Soft delete (`is_active=0`). Nunca hard delete (preserva historia). |
| **Transfer Ownership** | Flujo de dos pasos respetando el single-owner invariant de ADR-0009. |
| **List views** | Listados read-only (todas las cuentas, por rol). |
| **Audit log view** | Vista de los eventos de auditoría de cuentas. |
| **Add Yourself as Client** | Alta del owner/admin/staff como cliente del negocio (mismo phone, doble rol — ADR-0011). |

### Decision 2: non-goals explícitos

- **Edición de perfil, servicios, profesionales y horarios/agendas**: es el change de
  mantenimiento por Hermes ([ADR-0015](./0015-hermes-operational-maintenance.md)). No entra
  en la TUI aunque el operador esté sentado frente a ella.
- **`purge-inactive`**: follow-up (Fase 2+), con confirmación extra. No es MVP.
- **Superficie MCP nueva**: la TUI no expone ni consume tools nuevos.
- **Remplazo del wizard de `install.sh`**: ADR-0008 sigue vigente para setup.

### Decision 3: restricciones de diseño a cerrar en el SDD

1. **Bootstrap del auth-context**: cómo obtiene el primer owner su `auth.Caller` válido sin
   pasar por el middleware HTTP. Debe quedar resuelto en design; sin eso el seed no puede
   escribir.
2. **Sin FK `professional_id`**: picker de profesional obligatorio en `Add Staff` +
   prefill del teléfono desde `professionals.phone`, y validación explícita contra el repo.
3. **Nombre del entry-point**: fijar en spec un único nombre de sub-comando y un solo layout
   de archivo, coherente con el nombre real del binario instalado.
4. **Day-keys**: normalizar la asimetría `"1"`..`"7"` (lunes=1) vs `0`..`6` (domingo=0) en un
   único punto de traducción, con tests.
5. **Aislamiento**: la TUI comparte `*sql.DB`, `*slog.Logger` y los repos con el MCP server,
   pero no depende del transporte HTTP.

### Decision 4: criterio de corte por presupuesto de review

Si el MVP excede el presupuesto canónico de ~400 líneas de diff, **se divide en PRs
encadenados**, con el mismo criterio que `feat-setup-import` (3 PRs, sin exception). El
`size:exception` **no se infiere ni se aplica en silencio**: requiere aceptación explícita
del owner. Si durante el SDD aparece necesidad de editar datos operativos, no se amplía el
alcance: se referencia ADR-0015.

## Consequences

### Positive

- Alcance chico y verificable: identidad + cuentas, con un único desbloqueante claro (owner
  seed) que habilita todo lo demás.
- Defense-in-depth preservado: las cuentas siguen fuera del MCP (ADR-0010) y la TUI sigue
  siendo herramienta local del operador.
- Los non-goals evitan el diff inflado y el riesgo de mezclar identidad con datos de negocio.

### Negative

- El operador necesita **dos superficies**: la TUI para cuentas y Hermes para datos
  operativos. Es deliberado, pero hay que documentarlo para no confundir al cliente.
- El bootstrap del auth-context puede forzar una excepción acotada al diseño de autorización
  actual (nuevo camino de construcción de `auth.Caller` sin HTTP). Debe quedar explícita en
  design para que el gate la vea.
- `purge-inactive` queda diferido: cuentas desactivadas acumulan filas hasta Fase 2+.

### Rejected alternatives

| Alternativa | Por qué se rechaza |
|---|---|
| **Incluir edición de perfil/servicios/horarios en la TUI** | Duplica la superficie de escritura con ADR-0015 y duplica el trabajo de validación donde el cliente no opera. **Rechazado**. |
| **Exponer la gestión de cuentas como tools MCP** | Ya rechazado en ADR-0010: un LLM comprometido podría escalar privilegios. **Rechazado**. |
| **Pedir `size:exception` preventivo para el MVP completo** | La exception no se infiere; el default es dividir en PRs encadenados. **Rechazado**. |
| **Convertir el seed del owner en un flag de `install.sh`** | Reintroduce prompts de identidad en el wizard y contradice ADR-0008/ADR-0010 (seed movido a la TUI). **Rechazado**. |

## References

- [ADR-0008](./0008-install-prompts.md) — prompts inline en `install.sh` (setup one-time).
- [ADR-0009](./0009-authorization-model.md) — `accounts`, roles, single-owner invariant, soft delete.
- [ADR-0010](./0010-admin-tui.md) — decisión original de la TUI (capacidades, stack, enforcement).
- [ADR-0011](./0011-owner-as-client.md) — owner/admin/staff como clientes del negocio.
- [ADR-0015](./0015-hermes-operational-maintenance.md) — mantenimiento de datos operativos por Hermes.
- `docs/PRD.md` Fase N — backlog pendiente declarado (`TUI identidad + seed del owner`).
