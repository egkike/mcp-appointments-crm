# Delta: auth-middleware (feat-whatsapp-bot)

> Reference: `openspec/changes/feat-whatsapp-bot/proposal.md` D2, `design.md` §1.1, `exploration.md` §3, ADR-0018 Decision 2.
> Change: feat-whatsapp-bot
> Type: one MODIFIED requirement ("Lectura del header `X-Caller-Id`", which pins the precedence order empty-header → allowlist → resolution) plus ADDED requirements. The remaining existing requirements ("Resolución del caller en 1-2 queries", "Caller inyectado en `context.Context`", "Rechazo por permisos insuficientes", "Logging de auditoría", "Sin dependencias externas") keep their current text and scenarios; the additions below pin the new seam and leave the resolution chain itself untouched (read-only, still 1–2 queries, still no writes).

## MODIFIED Requirements

### Requirement: Lectura del header `X-Caller-Id`

El middleware MUST leer el header `X-Caller-Id` de cada request HTTP entrante. La búsqueda del header MUST ser case-insensitive (HTTP headers son case-insensitive por RFC 7230 §3.2; `X-Caller-Id`, `x-caller-id` y `X-CALLER-ID` son equivalentes). El valor leído es el phone o handle del messenger.

Si el header está ausente, o si su valor es la string vacía después de trim, el middleware MUST rechazar el request con HTTP `401 Unauthorized` y un cuerpo que contenga el mensaje en español `"no se proporcionó X-Caller-Id"` (per PRD §3.8.6).

**Precedencia (feat-whatsapp-bot, D2).** Este chequeo universal MUST ejecutarse PRIMERO y sin excepciones. El orden de evaluación MUST ser: (1) header ausente o vacío → `401` con `"no se proporcionó X-Caller-Id"`; (2) allowlist estática de paths anónimos (requirement "Anonymous allowlist evaluated before caller resolution"); (3) resolución del caller. Ningún path, incluido el de la allowlist, MAY saltarse el paso (1); un path allowlisted con header presente y no vacío MUST llegar al handler sin resolución (la validación del formato del teléfono ocurre en el use case, no en el seam).

#### Scenario: Header presente con valor no vacío

- GIVEN un request HTTP con header `X-Caller-Id: +5491155554444`
- WHEN el middleware procesa el request
- THEN el valor leído MUST ser `+5491155554444` (sin espacios al inicio/final)
- AND el middleware MUST continuar con la resolución del caller (no retornar 401)

#### Scenario: Header ausente retorna 401

- GIVEN un request HTTP sin el header `X-Caller-Id`
- WHEN el middleware procesa el request
- THEN el middleware MUST retornar HTTP `401 Unauthorized`
- AND el cuerpo MUST contener `"no se proporcionó X-Caller-Id"`
- AND el handler downstream MUST NO ejecutarse

#### Scenario: Header con valor vacío retorna 401

- GIVEN un request HTTP con header `X-Caller-Id:   ` (whitespace)
- WHEN el middleware procesa el request
- THEN el middleware MUST retornar HTTP `401 Unauthorized`
- AND el cuerpo MUST contener `"no se proporcionó X-Caller-Id"`
- AND el handler downstream MUST NO ejecutarse

#### Scenario: Header case-insensitive

- GIVEN un request HTTP con header `x-caller-id: +5491155554444` (lowercase)
- WHEN el middleware procesa el request
- THEN el valor leído MUST ser `+5491155554444` y el middleware MUST continuar con la resolución

#### Scenario: Header ausente en un path allowlisted retorna 401 (precedencia)

- GIVEN un request HTTP al path del tool `register_client` SIN el header `X-Caller-Id`
- WHEN el middleware procesa el request
- THEN el middleware MUST retornar HTTP `401 Unauthorized` con el cuerpo `"no se proporcionó X-Caller-Id"` (traducido a JSON-RPC `-32000` igual que en cualquier otro path)
- AND la allowlist anónima MUST NO evaluarse y ningún caller anónimo MUST ser inventado
- AND el handler downstream MUST NO ejecutarse

## ADDED Requirements

### Requirement: Anonymous allowlist evaluated before caller resolution

The middleware MUST evaluate a static, code-owned allowlist of MCP paths **before** attempting caller resolution, and only after the universal empty-header check of the requirement "Lectura del header `X-Caller-Id`" has passed. For an allowlisted path with a non-empty header the middleware MUST skip resolution entirely, MUST NOT query `accounts` or `clients`, MUST NOT fabricate a caller with a role, and MUST mark the context so downstream handlers can distinguish the anonymous case. For every other path the existing resolution requirement applies unchanged, including the HTTP `401` rejection for unknown phones. The allowlist MUST be a compile-time constant in the source: it MUST NOT be readable from configuration, environment variables, request headers or the database, and it MUST contain exactly the `register_client` path (see `client-registration`).

#### Scenario: Allowlisted path skips resolution

- GIVEN an MCP request to the `register_client` path carrying `X-Caller-Id: '+5491100999999'`
- AND no row for that phone in `accounts` or `clients`
- WHEN the middleware processes the request
- THEN it MUST NOT emit any query against `accounts` or `clients`
- AND the downstream handler MUST execute

#### Scenario: Non-allowlisted path keeps the existing 401 behavior

- GIVEN an MCP request to any tool path other than `register_client` carrying an unknown `X-Caller-Id`
- WHEN the middleware processes the request
- THEN the middleware MUST resolve the caller as before and MUST answer HTTP `401 Unauthorized`
- AND the downstream handler MUST NOT execute

#### Scenario: The allowlist cannot be extended from outside

- GIVEN the middleware source after this change
- WHEN the allowlist is inspected
- THEN it MUST be a static map with exactly one entry and no code path MAY add entries from configuration, headers or the database

#### Scenario: Anonymous context does not carry a role

- GIVEN an allowlisted request that reaches the handler
- WHEN the handler inspects the context
- THEN the context MUST NOT contain a caller with a role
- AND the handler MUST obtain the phone only from the `X-Caller-Id` header value

#### Scenario: Missing header on the allowlisted path still fails with the universal 401

- GIVEN an MCP request to `register_client` WITHOUT an `X-Caller-Id` header
- WHEN the middleware processes the request
- THEN the universal empty-header check MUST run first and answer HTTP `401` with `"no se proporcionó X-Caller-Id"` (JSON-RPC `-32000`, REQ-AM-WIRED-002)
- AND the allowlist seam MUST NOT be evaluated, the handler MUST NOT execute and no anonymous caller MUST be invented to work around the missing header

#### Scenario: No other tool becomes anonymous by accident

- GIVEN the full MCP tool path surface after this change
- WHEN each non-`register_client` path is called with an unknown `X-Caller-Id`
- THEN every one of them MUST answer `401` and MUST NOT reach its handler

## Notes

- The seam is transport-edge only (`internal/auth/middleware.go`); it adds no dependency and no database access (the existing "Sin dependencias externas" requirement still holds).
- Precedence is the whole point: the universal empty-header check runs first, then the seam, then resolution — so that a missing header can never be traded for an anonymous registration, the attacker-controlled body can never influence identity, and no resolver write path is introduced (the resolver stays read-only).
- The audit requirement for privileged actions is unchanged; registration audit events are specified in `client-registration` and mask the phone value.
