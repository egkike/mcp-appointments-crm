# ADR-0011: Owner/admin/staff pueden ser clientes del negocio (mismo phone, doble rol)

- **Status**: accepted
- **Date**: 2026-06-29
- **Authors**: Kike
- **Related**: ADR-0009 (Authorization model), ADR-0010 (Admin TUI), `openspec/changes/feat-authorization/specs/auth-roles/spec.md` §"Determinación del role del caller"

## Context

El sistema MCP recibe tool calls de un LLM (Hermes) que actúa como intermediario entre los clientes del negocio y el bot de WhatsApp Business / Telegram. El LLM (o un atacante que lo comprometa) podría solicitar datos sensibles sin restricción. ADR-0009 introduce una capa de autorización con la tabla `accounts` como whitelist para owner/admin/staff y un mecanismo de header `X-Caller-Id` para identificar al caller.

**Pero hay un caso operacional importante que no estaba cubierto:**

- El **dueño del negocio** (owner en `accounts`) es **una persona real** que probablemente también consume los servicios del negocio. Ej: el dueño de una peluquería quiere cortarse el pelo ahí. El dueño de una veterinaria quiere llevar a su propia mascota. El dueño de un café quiere almorzar en su propio café.
- Similar: el **staff** (peluquero, veterinario, mesero) también es cliente del negocio en su tiempo libre. La peluquera del staff se corta el pelo en su propio salón. La veterinaria del staff trae a su perro a la clínica.

**La pregunta es: ¿cómo reserva el owner/staff para sí mismo?**

Con el spec actual de ADR-0009:
- El owner es identificado por `X-Caller-Id: +5491100000000`
- El sistema lo resuelve como `Caller{ID: "+5491100000000", Role: "owner", ProfessionalID: nil, ClientID: nil}`
- El owner no tiene `ClientID`, porque no hay fila en `clients` con su phone
- Si el owner dice "quiero un turno para mí el viernes a las 15", el LLM invoca `create_booking` con `client_id=?` — el owner no tiene `client_id`, así que el sistema no sabe para quién es la reserva
- El sistema rechaza: "no te reconozco como cliente" — frustrante para el dueño

## Decision

**El owner/admin/staff crean su propio `client` row** durante el setup, usando el mismo `phone` que su `account`. El sistema los trata como **dos identidades coexistentes con el mismo `id`**:
- Una en `accounts` (rol: `owner`/`admin`/`staff`) para gestionar el negocio
- Una en `clients` (rol implícito: `client`) para reservar como cliente

El `CallerResolver` ahora ejecuta **2 queries siempre que la cuenta existe en `accounts`**:
1. `SELECT * FROM accounts WHERE id = ?`
2. Si la cuenta existe y está activa: `SELECT id FROM clients WHERE id = ?` para popular el `ClientID`

**Resultado del resolver:**

| Caso | Queries | Caller retornado |
|---|---|---|
| Phone solo en `clients` | 2 (accounts vacío + clients) | `Role: "client", ClientID: &id` |
| Phone solo en `accounts` (admin) | 2 (accounts + clients vacío) | `Role: "admin", ClientID: nil` |
| Phone en **ambos** (owner/client) | 2 (accounts + clients) | `Role: "owner", ClientID: &id` |
| Phone desconocido | 2 (ambas vacías) | `ErrUnauthenticated` |
| Phone inactivo en `accounts` | 1 (accounts) | `ErrUnauthenticated` (no consulta clients) |

## Consecuencias

**Positive**:
- **El dueño puede reservar para sí mismo** sin fricción: usa el mismo `X-Caller-Id` que usa para gestionar, y el sistema lo reconoce como `client` automáticamente.
- **El staff también puede reservar para sí mismo**: la peluquera del staff se corta el pelo en su propio salón con un solo chat.
- **El modelo refleja la realidad**: una persona del negocio es, naturalmente, un cliente del negocio.
- **Defense-in-depth intacta**: el LLM NO puede falsificar `X-Caller-Id`, y el resolver distingue correctamente entre los dos roles. El `Role` del `Caller` determina qué tools puede usar (admin: full access; client: solo sus propios datos).
- **El sistema unifica el resolver**: ya no hay un caso especial para "admin que también es cliente" — siempre se hacen las queries y se combinan.

**Negative**:
- **El owner/staff tiene 2 entries** (una en `accounts`, una en `clients`) con el mismo `id` (phone). Esto puede confundir a quien mire la DB sin contexto. **Mitigación**: documentar en el comentario de la tabla `accounts` y `clients` que la combinación `accounts.id = clients.id` es válida y representa "persona del negocio que también es cliente".
- **El TUI tiene un paso extra**: el owner debe crear su `client` row durante el setup (opción "Add Yourself as Client" en el TUI). **Mitigación**: el TUI lo ofrece como caso especial, con confirmación simple.
- **El resolver hace 2 queries siempre que la cuenta existe en `accounts`**: en lugar de 1-2 según el caso. **Mitigación**: el costo es 1 query extra en el path "admin/staff sin client row" (caso raro). Aceptable.

**Rejected alternatives**:
- (a) **Self-service client creation**: cuando un admin hace su primera reserva, el sistema crea automáticamente un `client` row. **Rechazado** porque complica el modelo (un mismo phone con 2 identidades implícitas), tiene race conditions si dos requests concurrentes, y la primera reserva falla si la creación implícita falla.
- (b) **Admin/staff NO pueden reservar para sí mismos**: tienen que hacerlo como cliente separado (otro phone). **Rechazado** porque es fricción operacional absurda (el dueño tiene que mantener 2 phones, uno personal para reservar, otro para gestionar). El sistema debe reflejar la realidad: una persona puede ser ambos.

## Reversibility

- Si en el futuro se quiere separar más las identidades, se puede agregar una columna `kind` en `clients` (`'person' | 'business_contact'`), pero el spec actual no lo necesita.
- Si se quiere deshabilitar el comportamiento "admin también es cliente" (raro), se puede agregar un flag `--no-self-as-client` en el TUI, pero por ahora es el default.

## Implementation order

1. **`feat-authorization` PR 2** (auth primitives): incluye el `Caller` struct con `ClientID *string`. El spec de `auth-roles` ahora documenta el algoritmo de 2 queries para el caso combinado.
2. **`feat-authorization` PR 2** (auth primitives, en curso): el `CallerResolver` se implementa con la lógica de 2 queries cuando la cuenta existe en `accounts`.
3. **Fase 2+**: el TUI menú operacional (ADR-0010) tiene una opción "Add Yourself as Client" para que el owner cree su `client` row durante el setup.
4. **Fase 2+**: si el owner quiere reservar y NO tiene `client` row aún, el bot le dice: "Para reservar como cliente, primero regístrate como cliente del negocio" — o el TUI lo crea automáticamente con un `INSERT OR IGNORE` en el `clients` table.

## References

- `docs/PRD.md` §3.8 (Modelo de Autorización)
- `docs/PRD.md` §3.8.8 (TUI menú operacional)
- `docs/architecture/0009-authorization-model.md` (ADR-0009)
- `docs/architecture/0010-admin-tui.md` (ADR-0010)
- `openspec/changes/feat-authorization/specs/auth-roles/spec.md` §"Determinación del role del caller"
- `openspec/changes/feat-authorization/tasks.md` (Future work: TUI "Add Yourself as Client")
