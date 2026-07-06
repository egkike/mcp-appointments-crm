# Spec: auth-middleware

> Reference: `docs/PRD.md` §3.8.3 (Flujo de identificación del caller), §3.8.5 (defensa contra LLM comprometido), §3.8.6 (mensajes de error al LLM); `docs/architecture/0009-authorization-model.md` Componente 3 (middleware)
> Change: feat-authorization
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe traducir cada MCP request entrante en un `Caller` autorizado, leyendo el header `X-Caller-Id` inyectado por el cliente MCP (Hermes), resolviéndolo contra las tablas `accounts` y `clients`, y adjuntándolo al `context.Context` antes de invocar el handler del tool. El middleware es la primera línea de defensa (capa "coarse-grained" del enforcement en 3 capas de PRD §3.8.4). Esta spec describe la CONTRATO del middleware en aislamiento: el wiring del HTTP server es una tarea separada (Fase 2).

> **Nota sobre el TUI menú (Fase 2+)**: el sub-comando `mcp-appointments-crm admin tui` corre en la VPS como **otro proceso** (no es un MCP tool). El TUI no usa este middleware HTTP — el admin opera directamente contra `AccountsRepo` (bypasseando la capa HTTP). El TUI se cubre por `accounts-repo` y por la nueva sección §3.8.8 "TUI menú operacional" del PRD (Fase 2). El gatekeeper de seguridad del TUI es el **admin del OS** (SSH a la VPS); el spec del middleware HTTP no aplica al TUI.

## Requirements

### Requirement: Lectura del header `X-Caller-Id`

El middleware MUST leer el header `X-Caller-Id` de cada request HTTP entrante. La búsqueda del header MUST ser case-insensitive (HTTP headers son case-insensitive por RFC 7230 §3.2; `X-Caller-Id`, `x-caller-id` y `X-CALLER-ID` son equivalentes). El valor leído es el phone o handle del messenger.

Si el header está ausente, o si su valor es la string vacía después de trim, el middleware MUST rechazar el request con HTTP `401 Unauthorized` y un cuerpo que contenga el mensaje en español `"no se proporcionó X-Caller-Id"` (per PRD §3.8.6).

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

### Requirement: Resolución del caller en 1-2 queries

Una vez leído el `X-Caller-Id`, el middleware MUST resolver el caller siguiendo la cadena de búsqueda definida en `auth-roles` (Requirement "Determinación del role del caller"):

1. Query 1: `SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?`.
2. Si la fila existe y `is_active = 1`, continuar con Query 2 (`SELECT id FROM clients WHERE id = ?`) para popular el `ClientID` (per ADR-0011, owner/admin/staff pueden ser clientes). Si `clients` no tiene fila, `ClientID` queda `nil`.
3. Si la fila existe pero `is_active = 0`, retornar 401 con mensaje de cuenta deshabilitada (NO consulta `clients`).
4. Si no hay fila en `accounts`, Query 2: `SELECT id FROM clients WHERE id = ?`.
5. Si hay fila en `clients`, construir `Caller{Role: "client", ClientID: &id}` y terminar.
6. Si no hay fila en ninguna tabla, retornar 401 con mensaje "no te reconozco".

**Resumen de queries por caso (1-2 queries):**
- Caller solo en `clients`: 1 query.
- Caller solo en `accounts` (admin sin client row): 2 queries (accounts + clients vacío).
- Caller en ambos (admin+client, owner+client): 2 queries, `ClientID` poblado.
- Caller desconocido: 2 queries, ambas vacías.
- Caller inactivo: 1 query (retorna error).

Las queries MUST usar placeholders `?` (nunca concatenación). MUST usar el `context.Context` del request (con su timeout / cancelación). MUST emitir a lo sumo 2 queries por request. MUST NO usar cache en memoria en esta versión (la latencia adicional de 1-2 queries es aceptable per ADR-0009; cache diferida a Fase 2+).

#### Scenario: Caller admin encontrado en accounts (1 query)

- GIVEN una fila en `accounts` con `id = '+5491100000000'`, `role = 'admin'`, `is_active = 1`
- AND un request con `X-Caller-Id: +5491100000000`
- WHEN el middleware procesa el request
- THEN el middleware MUST emitir exactamente 1 query (`SELECT ... FROM accounts WHERE id = ?`)
- AND MUST emitir 0 queries contra `clients`
- AND el `Caller` resultante MUST tener `Role = "admin"`, `ProfessionalID == nil`, `ClientID == nil`

#### Scenario: Caller staff encontrado en accounts (1 query)

- GIVEN una fila en `accounts` con `id = '+5491100002222'`, `role = 'staff'`, `professional_id = 'p-001'`, `is_active = 1`
- AND un request con `X-Caller-Id: +5491100002222`
- WHEN el middleware procesa el request
- THEN el middleware MUST emitir exactamente 1 query contra `accounts`
- AND el `Caller` resultante MUST tener `Role = "staff"`, `ProfessionalID` apuntando a `"p-001"`, `ClientID == nil`

#### Scenario: Cuenta desactivada retorna 401 con mensaje específico

- GIVEN una fila en `accounts` con `id = '+5491100000000'`, `is_active = 0`
- AND un request con `X-Caller-Id: +5491100000000`
- WHEN el middleware procesa el request
- THEN el middleware MUST retornar HTTP `401 Unauthorized`
- AND el cuerpo MUST contener `"tu cuenta está deshabilitada. Contacta al administrador."`
- AND el handler downstream MUST NO ejecutarse
- AND el middleware MUST NO emitir la segunda query contra `clients`

#### Scenario: Caller client encontrado en clients (2 queries)

- GIVEN ninguna fila en `accounts` con `id = '+5491100003333'`
- AND una fila en `clients` con `id = '+5491100003333'`
- AND un request con `X-Caller-Id: +5491100003333`
- WHEN el middleware procesa el request
- THEN el middleware MUST emitir 1 query contra `accounts` (sin resultado) y 1 query contra `clients` (con resultado) — total 2 queries
- AND el `Caller` resultante MUST tener `Role = "client"`, `ProfessionalID == nil`, `ClientID` apuntando al id

#### Scenario: Caller desconocido retorna 401 con mensaje específico

- GIVEN ninguna fila en `accounts` ni en `clients` con `id = '+5491100099999'`
- AND un request con `X-Caller-Id: +5491100099999`
- WHEN el middleware procesa el request
- THEN el middleware MUST retornar HTTP `401 Unauthorized`
- AND el cuerpo MUST contener `"no te reconozco. Por favor registrate primero."`
- AND el handler downstream MUST NO ejecutarse

### Requirement: Caller inyectado en `context.Context`

Una vez resuelto el `Caller` exitosamente, el middleware MUST inyectarlo en el `context.Context` del request usando `auth.WithCaller(ctx, caller)`. El `ctx` enriquecido es el que se pasa al handler downstream.

El middleware MUST NO retornar el `Caller` por otro canal (no por un struct field, no por una variable global). El contrato de propagación es exclusivamente vía `context.Context` (ver `auth-identity`).

#### Scenario: Handler downstream recibe caller en el ctx

- GIVEN un middleware que resolvió exitosamente un `Caller` (cualquier rol)
- AND un handler downstream que llama `auth.FromContext(ctx)` al inicio
- WHEN el handler se ejecuta
- THEN `FromContext(ctx)` MUST devolver `(caller, true)` con todos los campos poblados

### Requirement: Rechazo por permisos insuficientes (coarse-grained RBAC)

El middleware MUST aceptar una configuración de "rol(es) requerido(s)" por tool/ruta. Si el rol del caller resuelto NO está en el conjunto permitido para el endpoint, el middleware MUST retornar HTTP `403 Forbidden` con un cuerpo que contenga el mensaje en español `"no tienes permiso para realizar esta acción"` (per PRD §3.8.6).

El contrato del middleware en aislamiento es: dado un endpoint con `RequiredRoles: []string{"admin"}` y un caller con `Role = "client"`, el middleware rechaza con 403 ANTES de invocar el handler. La lista de roles por tool vive en una tabla/mapa en el wiring (Fase 2); esta spec describe el comportamiento genérico.

#### Scenario: Caller admin accede a tool que requiere admin

- GIVEN un endpoint configurado con `RequiredRoles = ["admin"]`
- AND un caller con `Role = "admin"`
- WHEN el middleware procesa el request
- THEN el middleware MUST invocar el handler downstream (no retornar 403)

#### Scenario: Caller client es rechazado en tool de admin

- GIVEN un endpoint configurado con `RequiredRoles = ["admin"]`
- AND un caller con `Role = "client"`
- WHEN el middleware procesa el request
- THEN el middleware MUST retornar HTTP `403 Forbidden`
- AND el cuerpo MUST contener `"no tienes permiso para realizar esta acción"`
- AND el handler downstream MUST NO ejecutarse

#### Scenario: Caller staff accede a tool que permite staff

- GIVEN un endpoint configurado con `RequiredRoles = ["staff", "admin"]`
- AND un caller con `Role = "staff"`
- WHEN el middleware procesa el request
- THEN el middleware MUST invocar el handler downstream

#### Scenario: Endpoint sin RequiredRoles acepta cualquier caller autenticado

- GIVEN un endpoint sin `RequiredRoles` configurado (o `RequiredRoles = nil`)
- AND un caller autenticado de cualquier rol
- WHEN el middleware procesa el request
- THEN el middleware MUST invocar el handler downstream (el coarse-grained RBAC es opcional por endpoint)

### Requirement: Logging de auditoría para acciones privilegiadas

Cuando el caller resuelto es `admin`, el middleware MAY loguear un registro de auditoría con al menos: timestamp ISO 8601 UTC, `caller_id`, y nombre del tool/ruta. Esto es defense-in-depth: si el LLM escala a admin (porque conoce un phone whitelisted), queda el rastro forense.

Para callers `staff` y `client`, el audit log es opcional y puede diferirse a Fase 2+. MUST NO loguear passwords, tokens, ni PII sensible más allá del `caller_id` (que ya es el phone/handle).

#### Scenario: Admin accede a un tool → se emite audit log

- GIVEN un caller con `Role = "admin"` que accede a un tool
- WHEN el middleware autoriza el acceso
- THEN el sistema MUST emitir (asynchronously o sincrónicamente) un log que incluya el `caller_id` y el nombre del tool
- AND el log MUST tener timestamp ISO 8601 UTC

### Requirement: Sin dependencias externas

El middleware MUST implementar usando únicamente la stdlib (`net/http`, `context`, posiblemente `log/slog` para audit). MUST NO agregar dependencias a `go.mod` para esta capacidad. Esto cumple ADR-0005.

#### Scenario: Importaciones mínimas

- GIVEN el código del middleware bajo `internal/auth/`
- WHEN se enumeran los imports de producción (no `_test.go`)
- THEN el único paquete externo permitido es la stdlib

## Notes

- El middleware hace 1-2 queries por tool call. Sin cache en esta versión. Latencia aceptable per ADR-0009.
- El HTTP server wiring (crear el `*http.ServeMux`, registrar el middleware, los handlers por tool) es out-of-scope per la propuesta §Out of Scope. Esta spec describe el comportamiento del middleware en aislamiento, con un contrato tipo `func Wrap(next http.Handler, resolver CallerResolver) http.Handler` o equivalente.
- El test de unidad del middleware puede usar `httptest.NewRecorder` + `httptest.NewRequest` para simular requests, y `go-sqlmock` para los `accounts`/`clients` SELECTs. El `auth-roles` spec cubre los tests del resolver en sí; este spec cubre el contrato HTTP del middleware.
- El `CallerResolver` (función helper que encapsula la query chain de `auth-roles`) puede vivir en `internal/auth/` o en un sub-paquete — la spec no fuerza la ubicación exacta, sólo el contrato.
- Coverage target ≥ 80% en `internal/auth/` (per propuesta §Success Criteria).
