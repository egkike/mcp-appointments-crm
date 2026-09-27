# ADR-0018: Modelo de canales de comunicación

- **Status**: accepted
- **Date**: 2026-09-27
- **Authors**: Kike
- **Related**: ADR-0009 (modelo de autorización), ADR-0012 (Chat de Hermes — multi-user retirado), ADR-0015 (mantenimiento por Hermes), ADR-0017 (config de Hermes), PRD §3.8.9 / §3.8.10 / §7

## Context

El smoke from-zero de v0.6.1 (2026-09-26) validó wizard, deploy pineado, seed de la TUI,
opción 7 y el sweep de tools, pero dejó implícito el modelo de canales: quién escribe, desde
qué número, con qué identidad llega cada mensaje al MCP server y qué caminos evitan el número
del negocio. La entrevista de producto del 2026-09-27, contrastada contra código
(`internal/auth/resolver.go`, el `CHECK` de `business_profile`), congeló las decisiones de
abajo. Este ADR las registra y retira la sección multi-user de ADR-0012.

## Decision

### Decisión 1 — Un solo canal del negocio (WhatsApp primario, Telegram alternativa)

El negocio tiene **exactamente un** teléfono/cuenta y opera con WhatsApp **o** Telegram, nunca
ambos. La restricción ya existe en el schema:
`CHECK (messenger_platform IS NULL OR messenger_platform IN ('whatsapp','telegram'))`
(`internal/db/schema.go:33`), así que no hay cambio de schema. WhatsApp es la plataforma
primaria; Telegram es la alternativa soportada.

### Decisión 2 — Identidad por emisor sobre el canal del negocio

Clientes, profesionales y owner escriben **al número del negocio desde su propio teléfono**, y
todos leen esa misma conversación. El gateway/agent de Hermes (lado plataforma de mensajería)
lee el `from` de cada mensaje e inyecta el header `X-Caller-Id`. El MCP server **no cambia**:
`CallerResolver` resuelve el identificador contra la base y aplica el rol que corresponda.

```
emisor (cliente | profesional | owner) —su propio número→ número del negocio (WhatsApp | Telegram)
        │
        ▼
gateway/agent Hermes: lee `from` → inyecta header `X-Caller-Id`
        │
        ▼
MCP server (127.0.0.1:3000) → auth.CallerResolver(id)
        ├── accounts(id, is_active=1) ──► Caller{Role, ProfessionalID}   (owner | admin | staff)
        │        └── clients(id) ──────► + ClientID   (doble rol, ADR-0011)
        ├── accounts(id, is_active=0) ─► 401 cuenta deshabilitada
        ├── clients(id) ───────────────► Role = client
        └── sin fila ──────────────────► 401 desconocido
```

El canal no otorga rol: el rol sale de `accounts`/`clients`. El gateway solo transporta el
identificador del emisor.

### Decisión 3 — Canales owner-only que evitan el número del negocio

Solo el owner tiene dos caminos que **no** pasan por el canal del negocio:

**3a) SSH + `mcp-server hermes chat`** — el gatekeeper es el login del OS (SSH). El
`X-Caller-Id` se prefill-ea desde el TUI (opción 7, ADR-0017); `MCP_CALLER_ID` queda como
override de debug local. Su semántica de override **cambió**: ver Retirement.

**3b) Bot privado de Telegram (owner-only)** — bot privado cuya allowlist admite únicamente el
chat del owner. Acá la identidad se inyecta **estáticamente desde configuración**: Telegram
**no** expone el teléfono del emisor a un bot (solo un user id anónimo), así que este canal
**no puede** hacer resolución per-sender. El archivo de configuración del MCP server guarda el
teléfono real del owner y es la fuente de verdad de esa inyección. Cualquier emisor fuera de la
allowlist se rechaza **en el gateway**, antes de que una tool call llegue al server.

## Retirement — ADR-0012 (§3.8.9), sección multi-user

Se retira la sección multi-user de [ADR-0012](./0012-hermes-chat-local.md) y su espejo en PRD
§3.8.9:

- **El staff ya no puede usar SSH/`hermes chat` ni el override `MCP_CALLER_ID`.** La regla
  "el staff puede hacer SSH con `MCP_CALLER_ID` para impersonar a un caller" queda
  **eliminada**: era impersonación sin control de rol.
- **`MCP_CALLER_ID` sigue existiendo como override local owner-only** para debug (simular un
  cliente contra el server).
- **El canal Chat queda single-user (owner).**

## Consecuencias

- **Autoridad por roles intacta**: la autoridad siempre sale de las tablas `accounts`/`clients`.
  El archivo de configuración **transporta el identificador, nunca el rol**; un config
  manipulado no puede escalar privilegios, solo cambiar a otra identidad ya autorizada.
- **Reglas de producto pendientes para el spec del bot futuro** (hoy no implementadas):
  - **Auto-registro de desconocido**: un teléfono que escribe por primera vez al canal del
    negocio se auto-registra como cliente vía `get_or_create_client` (rol client,
    self-service). Hoy el server responde **401**; el gap es requisito explícito del change
    del bot.
  - **Fricción de `request_contact` en Telegram**: sin teléfono expuesto, resolver un emisor
    en Telegram exige pedir el contacto en el primer mensaje. Es la razón por la que WhatsApp
    es el canal primario.
- **Loopback enforcement sin cambios**: el canal del negocio sigue siendo la única interfaz
  expuesta al exterior, y **todos** los canales hablan con el MCP server estrictamente por
  `127.0.0.1:3000`. El bot privado de Telegram mantiene el mismo requisito.

## References

- [ADR-0009](./0009-authorization-model.md) — modelo de autorización (`Caller`, roles, single-owner).
- [ADR-0011](./0011-owner-as-client.md) — owner como cliente (doble rol).
- [ADR-0012](./0012-hermes-chat-local.md) — Chat de Hermes local; su multi-user queda retirado acá.
- [ADR-0015](./0015-hermes-operational-maintenance.md) — tools de mantenimiento por Hermes.
- [ADR-0017](./0017-hermes-config-tui.md) — TUI "configurar Hermes" (opción 7, `X-Caller-Id`).
- `internal/auth/resolver.go` — cadena `accounts → clients → desconocido`.
- `internal/db/schema.go:33` — `CHECK` de `business_profile.messenger_platform`.
- `docs/PRD.md` §3.8.9 / §3.8.10 / §7 (Fase N).
