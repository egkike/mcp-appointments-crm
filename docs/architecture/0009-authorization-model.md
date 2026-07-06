# ADR-0009: Authorization model — `accounts` whitelist for admin/staff

- **Status**: accepted
- **Date**: 2026-06-29
- **Authors**: Kike

## Context

El sistema MCP recibe tool calls de un LLM (Hermes) que actúa como intermediario entre los clientes del negocio y el bot de WhatsApp Business / Telegram. Sin una capa de autorización:

- El LLM (o un atacante que lo comprometa) podría solicitar datos sensibles (ej: "dame todos los clientes con reservas de los últimos 30 días") sin restricción.
- No hay forma de distinguir entre un admin que está configurando el sistema y un cliente que está intentando reservar.
- Cualquier número de teléfono que escriba al bot tendría los mismos permisos que el dueño.

Adicionalmente, la tabla `business_profile` ya tiene `messenger_id`, que identifica la cuenta del **bot del negocio** (no la cuenta del admin). El bot es un canal que recibe mensajes de TODAS las cuentas.

## Decision

**Introducir una nueva tabla `accounts` como whitelist de permisos elevados, e identificar al caller en cada MCP request vía header HTTP inyectado por el cliente MCP (Hermes).**

### Componentes

1. **Nueva tabla `accounts`** con `role IN ('admin', 'staff')` y FK a `professionals` para staff. Los clientes NO tienen entry en esta tabla; se identifican por su presencia en `clients`.

2. **Header `X-Caller-Id`** en cada MCP request, inyectado por el cliente MCP desde el contexto del chat. El LLM no manipula este header.

3. **Middleware de autenticación** que:
   - Lee `X-Caller-Id`.
   - Busca en `accounts`; si está, `caller = {role: admin|staff, ...}`.
   - Si no, busca en `clients`; si está, `caller = {role: client, client_id: id}`.
   - Si no está en ninguno, retorna `ErrUnauthenticated`.
   - Carga el `caller` en `context.Context`.

4. **Repositorios con enforcement**:
   - Cada método chequea `caller.Role` desde el ctx.
   - Staff filtra por `professional_id`; client filtra por `client_id`; admin tiene full access.
   - Las queries SQL incluyen `WHERE professional_id = ?` o `WHERE client_id = ?` para staff/client.

5. **Defense-in-depth**: aunque el LLM se comprometa, no puede falsificar `X-Caller-Id` (viene del chat context, no del LLM). Para escalar a admin necesita conocer un phone whitelisted.

### Schema

```sql
CREATE TABLE accounts (
    id              TEXT PRIMARY KEY,
    role            TEXT NOT NULL CHECK (role IN ('owner', 'admin', 'staff')),
    display_name    TEXT,
    professional_id TEXT,
    is_active       INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK ((role = 'staff' AND professional_id IS NOT NULL) OR (role IN ('admin', 'owner')))
);
```

> **Status note (2026-06-29)**: el schema de `accounts` fue extendido por [ADR-0010](../architecture/0010-admin-tui.md) con el rol `owner` (single-owner invariant, soft delete via `Deactivate`, audit log MUST). El CHECK de role se actualizó a `IN ('owner', 'admin', 'staff')` y el segundo CHECK a `OR (role IN ('admin', 'owner'))`. Ver ADR-0010 para el refinamiento operacional completo.

## Consequences

**Positive**:
- Defense-in-depth real: el LLM no puede escalar a admin sin conocer un phone whitelisted.
- Separación clara de identidades: bot (en `business_profile.messenger_id`), admin/staff (en `accounts`), clients (en `clients`).
- Mensajes semánticos al LLM (español), sin stack traces.
- El middleware coarse-grained filtra el 80% de los requests no autorizados antes de llegar al repo.
- Queries SQL con `WHERE professional_id = ?` / `WHERE client_id = ?` enforza row-level access.
- CHECK constraints a nivel de DB enforzan la invariante "staff tiene professional_id, admin no".

**Negative**:
- Más complejidad: 3 capas de enforcement (middleware + repo + SQL) que deben mantenerse consistentes.
- El `caller` debe propagarse vía `context.Context` consistentemente. Si un repo no chequea el ctx, se saltea el enforcement.
- Requiere que el cliente MCP (Hermes) inyecte `X-Caller-Id` correctamente. Si el cliente no lo hace, todos los requests fallan con `ErrUnauthenticated`.
- Tabla `accounts` adicional a mantener: inserción inicial del admin via `install.sh`, gestión via repo, desactivación (`is_active = 0`) cuando un staff deja el negocio.
- Latencia adicional: 1-2 queries (accounts + clients) por cada tool call. Mitigable con cache en memoria.

**Rejected alternatives**:
- **(a) Una sola cuenta admin fija, LLM declara `acting_as: admin|client`**: el LLM es responsable de declarar su rol. Menos seguro (un LLM comprometido puede escalar). Además, requiere que el LLM nunca se equivoque. **Rechazado**.
- **(b) Roles en una sola tabla `users`**: complica el modelo y mezcla admin/staff con clients. Las cuentas con permisos elevados son un conjunto cerrado (whitelist); los clients son un conjunto abierto. Conceptos distintos que no se mezclan bien. **Rechazado**.
- **(c) `accounts` con role 'client' explícito**: duplica datos de clients (id, display_name) en `accounts`. Innecesario. La presencia en `clients` ya implica role=client. **Rechazado**.
- **(d) Roles en JWT firmados por el bot**: agrega una capa de criptografía que no aporta defense-in-depth real (el LLM aún puede falsificar tokens si está comprometido). **Rechazado** por complejidad innecesaria.

## Reversibility

Si el modelo de autorización necesita evolucionar (ej: agregar "manager" entre admin y staff, agregar role 'auditor' con read-only, etc.):

- Agregar un nuevo valor en el CHECK constraint de `accounts` (migration ligera).
- Agregar la lógica correspondiente en el middleware y los repos.

El `X-Caller-Id` header y la propagación via `context.Context` son estables; no necesitan cambios.

Si en el futuro se quisiera migrar a un sistema de tokens firmados (JWT), sería ortogonal: el middleware podría aceptar tokens además de headers, sin cambiar los repos.

## Implementation order

La capa de autorización se implementa como un **change SDD separado** (`feat-authorization`) **antes** de los PRs complejos de `feat-db-layer` (sobre todo antes de PR 3, que expone datos de staff y clients via `check_availability`).

Orden:
1. **`feat-authorization`** (este change, Fase 0) — schema, repo, middleware, integración con el flujo MCP
2. **`feat-db-layer` PR 1a + PR 1b + PR 2** (ya mergeados en el tracker)
3. **`feat-db-layer` PR 3** (Bookings + CheckAvailability) — ahora con la capa de auth integrada
4. **Fase 2+** (handlers MCP, install.sh con seed del admin)

## References

- `docs/PRD.md` §3.8 (nueva sección, agregada con este ADR)
- `docs/architecture/0006-data-model-and-reservations.md` (decisiones relacionadas al schema)
- Próximo: `openspec/changes/feat-authorization/` (proposal + specs + design + tasks)
