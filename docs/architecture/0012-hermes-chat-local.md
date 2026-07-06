# ADR-0012: Segundo canal de comunicación — Chat nativo de Hermes (operación local)

- **Status**: accepted
- **Date**: 2026-06-29
- **Authors**: Kike
- **Related**: ADR-0009 (Authorization model), ADR-0010 (Admin TUI), ADR-0011 (Owner as Client), `docs/PRD.md` §3.8.9, `openspec/changes/feat-authorization/tasks.md` (Future work)

## Context

El sistema MCP recibe tool calls de un LLM (Hermes) que actúa como intermediario entre los clientes del negocio y el bot de WhatsApp Business / Telegram. El modelo de autorización (ADR-0009 + PRD §3.8) define el `X-Caller-Id` como el phone/handle del usuario, inyectado por el **bot de messenger** desde el contexto del chat.

**Pero hay un segundo canal de comunicación que no estaba modelado:**

- El **Chat nativo de Hermes** (la interfaz del LLM agent) — el owner/admin puede hablarle a Hermes directamente, sin pasar por el bot de messenger. Esto es típico cuando el developer corre Hermes localmente en su máquina o en la VPS y abre una sesión de chat (terminal, IDE, OpenCode Chat, etc.).

**La pregunta:** ¿cómo se identifica el caller cuando el owner le habla a Hermes desde el Chat nativo (no desde WhatsApp/Telegram)?

## Decision

**El Chat nativo de Hermes es un sub-comando del binario principal** (`mcp-appointments-crm hermes chat`). Corre en la VPS, se conecta al MCP server en `127.0.0.1:3000` (loopback), e inyecta el `X-Caller-Id` en cada tool call.

**Default:** el `X-Caller-Id` del owner se guarda durante `install.sh` en `~/.config/mcp-appointments-crm/caller-id` (o en el `.env` que `install.sh` ya genera per §3.5). El Chat lo lee al iniciar.

**Override:** el owner puede exportar `MCP_CALLER_ID=+5491100001111 mcp-appointments-crm hermes chat` para simular ser un cliente (debug, testing, o simular la perspectiva de un cliente).

**Multi-user via override:** el staff puede hacer SSH a la VPS y correr `MCP_CALLER_ID=+5491100002222 mcp-appointments-crm hermes chat` con su propio caller_id.

## Componentes

1. **`install.sh` (extensión de RF9)**: además de capturar el `X-Caller-Id` del owner para crear la fila en `accounts`, también lo guarda en `~/.config/mcp-appointments-crm/caller-id` (o en el `.env` existente). Output adicional:
   ```
   [mcp-appointments-crm] Setup completado.
   Tu caller_id (admin del sistema): +5491100000000
   Para usar el Chat de Hermes, ejecuta:
     mcp-appointments-crm hermes chat
   Override con otro caller_id (debug):
     MCP_CALLER_ID=+5491100001111 mcp-appointments-crm hermes chat
   ```

2. **`mcp-appointments-crm hermes chat`** (sub-comando del binario, Fase 2+):
   - Lee `$MCP_CALLER_ID` env var; si está vacía, lee `~/.config/mcp-appointments-crm/caller-id`.
   - Si no hay caller_id en ningún lado: error "Configura MCP_CALLER_ID antes de usar el chat" y exit 1.
   - Se conecta al MCP server en `127.0.0.1:3000` (loopback, sin exponer el MCP al exterior).
   - Inicia el loop de chat con el LLM (mismo Hermes que consume el bot, pero con caller_id del owner en lugar de phone del chat context).
   - Cada tool call del LLM lleva el header `X-Caller-Id: <caller_id>`.

3. **`CallerResolver`** no cambia: ya resuelve el caller_id a `Caller{Role, ...}`. Lo que cambia es **quién inyecta** el header: el bot (vía messenger) o el Chat (vía sub-comando). El resolver es agnóstico al canal.

## Consecuencias

**Positive**:
- **Dos canales soportados** con la misma capa de auth: bot de messenger (multi-user) y Chat de Hermes (single-user por default). El sistema trata al caller por su `X-Caller-Id`, no por el canal.
- **Defense-in-depth intacta**:
  - El LLM NO puede falsificar el `MCP_CALLER_ID` — la env var se lee del shell del owner, no del LLM.
  - El admin del OS (SSH a la VPS) sigue siendo el gatekeeper. El Chat corre en la VPS, no expone nada al exterior.
  - Loopback enforcement: el MCP server sigue en `127.0.0.1:3000`. El Chat también es loopback.
- **Override para debug/testing**: el owner puede ejecutar `MCP_CALLER_ID=+5491100001111 mcp-appointments-crm hermes chat` para simular ser un cliente (e.g., para verificar que el filtrado de `client_id` funciona).
- **Consistente con el TUI menú (ADR-0010)**: ambos son sub-comandos del binario, corren en la VPS, gatekeeper SSH.

**Negative**:
- **Asume single-user en la VPS** por default: el Chat de Hermes corre como el owner. Si múltiples personas (owner + staff) quieren usar el Chat simultáneamente, cada una tiene su propia sesión SSH y puede usar `MCP_CALLER_ID` para impersonar.
- **Dependencia del path `caller-id`**: si el archivo `~/.config/mcp-appointments-crm/caller-id` se borra, el Chat no funciona hasta que se restaure. **Mitigación**: el install.sh es idempotente — re-ejecutarlo regenera el caller-id.
- **El Chat local no es un caso operacional primario** (el owner opera vía WhatsApp/Telegram como cualquier cliente, y el LLM identifica que es el owner por el X-Caller-Id). El Chat local es más para debug y para setups donde el owner quiere hablarle a Hermes sin el bot.

**Rejected alternatives**:
- (a) **Hermes local en la laptop del owner** (sin SSH a la VPS): requeriría exponer el MCP al exterior (vía SSH tunnel o port forward) o reescribir la regla de loopback. **Rechazado** por violar loopback enforcement (PRD §3.5).
- (b) **Single-user mode con caller_id hardcodeado** (sin override): inflexible, no soporta debug/testing. **Rechazado**.
- (c) **El Chat pregunta el caller_id al iniciar** (sin config): fricción cada vez que abre el Chat. **Rechazado** (la config se hace una vez en `install.sh`).

## Reversibility

- Si se quiere exponer el MCP al exterior (raro, no recomendado), se puede agregar un flag `--expose-mcp` al binario que bind a `0.0.0.0` en lugar de `127.0.0.1`. **No implementado** — defense-in-depth gana.
- Si se quiere cambiar la config de `caller-id` (e.g., a `~/.config/mcp-appointments-crm/auth.json` con más campos), es backward-compatible: el código lee del nuevo path primero, fallback al viejo.
- Si se quiere permitir `MCP_CALLER_ID` desde múltiples archivos (e.g., `.env` de un proyecto), el código puede iterar sobre paths en orden de prioridad.

## Implementation order

1. **`feat-authorization` PR 1** (data layer, en curso): ya incluye el `Caller` struct. No cambios para este ADR.
2. **`feat-authorization` PR 2** (auth primitives, en curso): el `CallerResolver` se implementa con la lógica de 2 queries cuando la cuenta existe en `accounts`. No cambios para este ADR.
3. **Fase 2+**: implementar `mcp-appointments-crm hermes chat` como sub-comando. Extender `install.sh` para guardar el `caller-id` en `~/.config/mcp-appointments-crm/caller-id`.

## References

- `docs/PRD.md` §3.8 (Modelo de Autorización) + §3.8.9 (Chat de Hermes)
- `docs/PRD.md` §3.5 (Loopback enforcement)
- `docs/PRD.md` §5.1 RF9 (install.sh con seed del owner)
- `docs/architecture/0009-authorization-model.md` (ADR-0009)
- `docs/architecture/0010-admin-tui.md` (ADR-0010)
- `docs/architecture/0011-owner-as-client.md` (ADR-0011)
- `openspec/changes/feat-authorization/tasks.md` (Future work)
