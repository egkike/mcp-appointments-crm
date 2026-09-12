# ADR-0015: Hermes como mantenedor de datos operativos (MCP maintenance tools)

- **Status**: accepted
- **Date**: 2026-09-12
- **Authors**: Kike
- **Related**: ADR-0009 (authorization model), ADR-0010 (admin TUI), ADR-0016 (admin-tui scope)

## Context

El sistema quedó en un estado donde los datos operativos son cargables una sola vez y no
editables por el canal operativo diario. Contexto verificado contra código el 2026-09-12:

1. **El install siembra una vez y no vuelve a preguntar.** `scripts/install.sh` (wizard
   inline) escribe 3 JSONs en `<config>/setup/` y el primer arranque ejecuta
   `config.SeedOnBoot`, que siembra **solo datos de negocio**: `business_profile` (18 campos
   + las claves de horarios `"1"`..`"7"`), `professionals` + `schedules`, y `services`.
   El seeder **nunca toca `accounts`** y **nunca pide un owner**.

2. **Hermes no puede mutar esos datos.** El servidor expone 11 tools MCP. Todos son
   read-only salvo el ciclo de vida de reservas y `mark_alert_as_sent`.
   `get_business_profile` es read-only y `update_business_profile` **no existe en ningún
   lado**. Tampoco hay tools de gestión de servicios, profesionales ni agendas.

3. **Consecuencia operativa.** Hoy, cualquier cambio posterior al install —corregir un
   horario, agregar un servicio, desactivar un profesional— es **SQL manual** contra
   `reservas.db`. No hay segundo wizard ni tools de update.

4. **La capa de persistencia ya tiene las mutaciones.** `BusinessProfileRepo.Update`,
   `ServicesRepo.Save/Update/Delete` y el resto de los repos de escritura existen y están
   cableados con `auth.Caller`. Exponerlos como tools MCP es **wiring + RBAC**, no trabajo
   de base de datos.

La pregunta es: **¿quién mantiene los datos operativos después del install?**

## Decision

**Hermes mantiene los datos operativos vía nuevos tools MCP con RBAC owner.**

### Decision 1: nuevos tools MCP de mantenimiento

- Se agregan tools de escritura para perfil de negocio, servicios, profesionales y
  horarios/agendas, apoyados en los métodos de repositorio que ya existen.
- El conjunto exacto de tools, sus schemas, validaciones y códigos de error se definen en el
  SDD change correspondiente (ver Decision 3). Este ADR fija la decisión, no la interfaz.

### Decision 2: RBAC owner, errores semánticos en español

- Las mutaciones exigen rol `owner` (per [ADR-0009](./0009-authorization-model.md)). El
  alcance admin parcial se evalúa en el SDD, no acá: el default del MVP es owner-only.
- Los fallos de negocio devuelven mensajes semánticos en español, no dumps del sistema
  (ej. *"Error: el profesional seleccionado no trabaja los domingos"*), consistente con el
  estándar de errores del proyecto.
- Sin gestión de cuentas: crear, desactivar o transferir cuentas **no** es un tool MCP.
  Esa superficie vive en la TUI ([ADR-0016](./0016-admin-tui-scope.md)) por
  defense-in-depth: el LLM nunca debe poder escalar privilegios.

### Decision 3: SDD change separado, fuera del presupuesto de admin-tui

- El trabajo se ejecuta como un change SDD **separado y posterior** al de la TUI de
  identidad. Mezclar ambos infla el diff, mezcla dos modelos de riesgo (identidad/auth vs
  datos de negocio) y rompe el presupuesto de review.
- La TUI no es prerequisito técnico de estos tools, pero sí lo es operativamente: sin una
  cuenta `owner` activa, el RBAC de los nuevos tools responde 401.

## Consequences

### Positive

- Cierra el gap post-install: corregir perfil, servicios, profesionales u horarios deja de
  requerir SQL manual.
- Reutiliza la capa de repositorio existente; el costo real es exponer handlers y aplicar
  RBAC, no escribir persistencia nueva.
- El canal operativo diario (Hermes) queda alineado con cómo el cliente realmente opera el
  negocio: por conversación, no por SSH.

### Negative

- **Superficie de escritura nueva expuesta al LLM.** Cada tool de mantenimiento amplía el
  blast radius de un LLM comprometido o mal dirigido. Mitigación: RBAC owner obligatorio,
  validación de input por schema, errores semánticos sin detalles internos, y audit log
  estructurado en las mutaciones críticas.
- **Riesgo de drift semántico con la TUI.** Dos caminos de escritura sobre las mismas tablas
  (tools MCP y TUI) pueden divergir en validaciones. Mitigación: validación en el use
  case/repo, no en el transporte.
- Mantenimiento adicional: cada tool nuevo suma tests y documentación para Hermes.

### Rejected alternatives

| Alternativa | Por qué se rechaza |
|---|---|
| **Editar todo desde la TUI** (una sola superficie de escritura, sin tools nuevos) | La TUI requiere que un humano entre por SSH a la VPS. Hermes **es** el canal operativo diario del cliente; obligarlo a la TUI para cambiar un horario reintroduce el trabajo manual que este ADR busca eliminar. La separación TUI=identidad / Hermes=datos operativos también preserva defense-in-depth. |
| **Seguir con SQL manual** | No es operación de negocio: exige acceso a la VPS, conocimiento del schema y no deja validación ni audit log. |
| **Segundo wizard de instalación** (re-ejecutar prompts sobre los 3 JSONs) | Reejecutar el install es destructivo respecto del estado actual y no cubre cambios incrementales (un servicio, un horario). Un wizard es un evento de setup, no de mantenimiento. |
| **Gestión de cuentas vía MCP** | Rechazado explícitamente: un LLM comprometido que conozca un phone whitelisted podría escalar privilegios. Las cuentas quedan en la TUI (ADR-0010, ADR-0016). |

## Non-goals

- Gestión de cuentas de cualquier tipo (alta, baja, transferencia de ownership, listados).
- Segundo wizard de instalación.
- Edición de datos de clientes y de reservas más allá de los tools ya existentes.
- Cualquier forma de export/import masivo de datos de negocio.

## References

- `docs/PRD.md` Fase N — backlog pendiente declarado (`Tools Hermes de mantenimiento de datos operativos`).
- [ADR-0009](./0009-authorization-model.md) — modelo de autorización (`accounts`, roles, single-owner invariant).
- [ADR-0010](./0010-admin-tui.md) — TUI menú operacional.
- [ADR-0016](./0016-admin-tui-scope.md) — alcance del SDD `admin-tui` (identidad y cuentas).
- [ADR-0013](./0013-layered-architecture.md) — capas donde viven los repos de escritura.
