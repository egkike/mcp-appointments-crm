# Propuesta: feat/authorization — Modelo de autorización para el servidor MCP

## Intent

El MCP server recibe tool calls de un LLM (Hermes) intermediario entre clientes y el bot del
negocio. Sin capa de autorización (PRD §3.8, ADR-0009, ya en `main`): el LLM (o un atacante que
lo comprometa) puede solicitar datos sensibles sin restricción; no se distingue un `admin`
configurando el sistema de un `client` reservando; cualquier teléfono que escribe al bot
tendría los permisos del dueño.

## Scope

### In Scope
- Tabla `accounts` (whitelist `admin`/`staff`) — schema PRD §3.8.2 / ADR-0009.
- `internal/auth/`: `Caller`, `WithCaller(ctx, caller)`, `FromContext(ctx)`.
- Middleware que resuelve `X-Caller-Id` → `Caller` (accounts → clients → `ErrUnauthenticated`).
- `internal/repository/accounts.go` CRUD + `internal/model/account.go` + tests `go-sqlmock`.
- Enforcement 3 capas: middleware (coarse) + repos (medium, role filtering) + SQL (`WHERE professional_id=?` / `client_id=?`, fine).
- Mensajes semánticos al LLM en español (PRD §3.8.6).

### Out of Scope
- Handler/wiring del MCP server (Fase 2).
- Cambios al cliente Hermes (inyecta `X-Caller-Id`; no se toca).
- `feat-db-layer` PR 3 (BookingsRepo + check_availability) — integrará auth al implementarse.
- Seed del admin vía `install.sh` y cache en memoria de accounts (Fase 2+).

## Capabilities

> `openspec/specs/` vacío: todas son **NEW** → `openspec/specs/<name>/spec.md`.

### New Capabilities
- `auth-identity`: `Caller` struct + propagación vía `context.Context` (`WithCaller`/`FromContext`).
- `auth-roles`: roles `admin`/`staff`/`client` + tabla `accounts` (schema, CHECKs).
- `auth-middleware`: middleware que resuelve `X-Caller-Id` a `Caller`.
- `accounts-repo`: CRUD de `accounts` con prepared statements + tests `go-sqlmock`.

### Modified Capabilities
- Ninguna (no hay specs archivadas). La integración con `internal/repository` existente es por código, no delta spec.

## Approach

TDD estricto (`go-sqlmock` antes que producción). `internal/auth/` aísla primitivas de context y
el middleware; `accounts.go` añade el repo. El middleware hace 1-2 queries por tool call
(`accounts` + `clients`). Sin nuevas dependencias (stdlib `context`/`net/http`/`database/sql`).
**Dos PRs encadenados** (force-chained, el split es mandatory bajo el budget 400-LOC):
PR 1 (data layer: schema + model + repo + integration test, ~460 LOC) → PR 2 (auth primitives:
Caller + Resolver + Middleware, ~520 LOC). Ver `tasks.md` Forecast table para el breakdown.

## Affected Areas

| Área | Impacto | Descripción |
|------|---------|-------------|
| `docs/PRD.md` §3.8 | Referencia | Modelo canónico (ya en `main`) |
| `docs/architecture/0009-authorization-model.md` | Referencia | ADR canónico (ya en `main`) |
| `internal/auth/*.go` | Nuevo | `Caller`, context helpers, middleware |
| `internal/repository/accounts.go` | Nuevo | CRUD accounts + `*_test.go` |
| `internal/model/account.go` | Nuevo | Modelo `Account` |
| `internal/db/schema.go` | Modificado | `CREATE TABLE accounts` + CHECKs |

## Risks

| Riesgo | Prob. | Mitigación |
|--------|-------|------------|
| Repos existentes no chequean ctx → enforcement salteado | Media | Auditar en tasks; TDD cubre cada repo caller-aware |
| Middleware se registra después de handlers | Baja | Wiring en Fase 2; contrato del middleware documentado ahora |
| `accounts` CHECK inconsistente con PRD §3.8.2 | Baja | Schema copiado literal del PRD; test de integración |
| Latencia 1-2 queries/tool call | Baja | Aceptable; cache diferida a futuro |

## Rollback Plan

`git revert <merge>` — cambio pre-release sin datos de usuario. `internal/auth/` y `accounts.go`
son archivos nuevos (removibles). El `CREATE TABLE accounts` se revierte con el mismo commit;
no hay migración de datos.

## Dependencies

- Stdlib: `context`, `net/http`, `database/sql`. Sin nuevas deps externas (cumple AGENTS.md).
- Requiere el merge previo de `main` (trae PRD §3.8 + ADR-0009) al tracker del change.

## Success Criteria

- [ ] `accounts` creada con los CHECKs de PRD §3.8.2.
- [ ] `Caller` propaga vía `context.Context`; `FromContext` retorna ok/false.
- [ ] Middleware resuelve `admin`/`staff`/`client`/`unauthenticated` con mensajes §3.8.6.
- [ ] `AccountsRepo` CRUD con `go-sqlmock`; cobertura ≥80% en `internal/auth/` y `accounts.go`.
- [ ] `go test -v -race ./...`, `go build -o /dev/null ./...`, `go vet`, `golangci-lint` limpios.
- [ ] TDD estricto; GGA limpio en cada commit.

## Referencias

- `docs/PRD.md` §3.8 (roles, schema, flujo `X-Caller-Id`, enforcement 3 capas, mensajes).
- `docs/architecture/0009-authorization-model.md` (contexto, consecuencias, alternativas rechazadas, reversibilidad, orden de implementación).
- `openspec/changes/feat-db-layer/proposal.md` (referencia de formato y estructura).