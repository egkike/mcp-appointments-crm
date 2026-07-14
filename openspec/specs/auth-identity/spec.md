# Spec: auth-identity

> Reference: `docs/PRD.md` §3.8.4 (propagación de `caller` vía `context.Context`); `docs/architecture/0009-authorization-model.md` Componentes 2 y 3
> Change: feat-authorization
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe modelar la identidad del `caller` (owner / admin / staff / client) como un value type inmutable y propagarlo a través de las capas de la aplicación usando `context.Context`. Esta capacidad es la primitiva de bajo nivel sobre la que se construyen el middleware (`auth-middleware`) y el enforcement en repositorios; aísla la mecánica de propagación del modelo de roles y del header HTTP.

## Requirements

### Requirement: `Caller` es un value type con campos explícitos

El paquete `internal/auth` MUST exportar el struct `Caller` con los siguientes campos públicos (todos con tipos concretos, sin `any` ni `interface{}`):

- `ID string` — phone o handle del messenger (PK en `accounts` o `clients`).
- `Role string` — uno de `"owner"`, `"admin"`, `"staff"`, `"client"`. La validación del valor vive en `auth-roles`; este spec NO enforza el conjunto permitido.
- `ProfessionalID *string` — FK a `professionals.id`; NO-nil solo si `Role == "staff"`.
- `ClientID *string` — FK a `clients.id`; no-nil si el caller también existe en `clients` (owner/admin/staff pueden ser clientes per ADR-0011).

`Caller` MUST ser comparable por valor (todos los campos tienen tipos comparables) y MUST ser seguro de copiar entre goroutines. La zero value (`Caller{}`) es válida y representa "caller ausente"; la presencia se distingue vía `FromContext`, no por inspección del struct.

#### Scenario: Crear un caller de staff con ProfessionalID

- GIVEN un código que quiere modelar un staff member
- WHEN se construye `Caller{ID: "+5491155554444", Role: "staff", ProfessionalID: &"p-001"}` con `ProfessionalID` apuntando a `"p-001"` (Go syntax: `&"p-001"` es un puntero a un literal de string; en código real, usar una variable `pID := "p-001"; c := Caller{... ProfessionalID: &pID}`)
- THEN el struct contiene los cuatro campos con los valores asignados, y `ClientID == nil`

#### Scenario: Crear un caller de client con ClientID

- GIVEN un código que quiere modelar un client
- WHEN se construye `Caller{ID: "+5491100001111", Role: "client", ClientID: &"+5491100001111"}` con `ClientID` igual al ID del cliente (Go syntax: `&"+5491100001111"` es un puntero a un literal de string; en código real, usar una variable `id := "+5491100001111"; c := Caller{... ClientID: &id}`)
- THEN el struct contiene los cuatro campos, `ProfessionalID == nil`, y `Role == "client"`

### Requirement: `WithCaller` inyecta un Caller en el contexto

`WithCaller(ctx context.Context, caller Caller) context.Context` MUST retornar un nuevo `context.Context` que lleva el `caller` asociado. La función MUST usar una clave de contexto privada (no exportada) para evitar colisiones con otros paquetes. La firma MUST ser exactamente:

```go
func WithCaller(ctx context.Context, caller Caller) context.Context
```

`WithCaller` MUST NO mutar el `ctx` recibido (los contextos son inmutables por convención del stdlib). MUST NO hacer panic si `caller` es la zero value (un caller vacío sigue siendo un valor válido para inyectar; la decisión de "ausente" la toma `FromContext`).

#### Scenario: WithCaller retorna un contexto nuevo

- GIVEN un `ctx` base (no-cancelable, sin valores)
- WHEN se llama `WithCaller(ctx, someCaller)` con un caller no-cero
- THEN el valor retornado MUST ser distinto del `ctx` original (comparación de puntero o `!=`) y `FromContext(returnedCtx)` MUST retornar el caller inyectado con `ok == true`
- AND `FromContext(returnedCtx)` MUST devolver `(someCaller, true)`

#### Scenario: WithCaller con zero value no hace panic

- GIVEN un `ctx` base
- WHEN se llama `WithCaller(ctx, Caller{})` con la zero value
- THEN la función MUST NO panic
- AND `FromContext(returnedCtx)` MUST devolver `(Caller{}, true)` (presente, pero con campos vacíos)

### Requirement: `FromContext` recupera el Caller

`FromContext(ctx context.Context) (Caller, bool)` MUST retornar el `caller` asociado al contexto y `true` si está presente. Si no hay ningún caller asociado, MUST retornar la zero value de `Caller` y `false` (NUNCA panic, NUNCA retornar un error).

La firma MUST ser exactamente:

```go
func FromContext(ctx context.Context) (Caller, bool)
```

#### Scenario: FromContext con caller presente

- GIVEN un `ctx` producido por `WithCaller(ctx, caller)` con un caller no-cero
- WHEN se llama `FromContext(ctx)`
- THEN el resultado MUST ser `(caller, true)` con todos los campos idénticos (incluyendo `ProfessionalID` y `ClientID` como punteros que apuntan a los mismos valores)

#### Scenario: FromContext en contexto sin caller

- GIVEN un `ctx` base (sin ningún caller inyectado)
- WHEN se llama `FromContext(ctx)`
- THEN el resultado MUST ser `(Caller{}, false)`
- AND la función MUST NO panic, MUST NO retornar un error, y MUST NO ejecutar queries

#### Scenario: FromContext con contexto cancelado

- GIVEN un `ctx` que ya fue cancelado (`ctx.Done()` está cerrado) y que NO tiene caller inyectado
- WHEN se llama `FromContext(ctx)`
- THEN el resultado MUST ser `(Caller{}, false)` — la cancelación no afecta la lectura del valor; sólo la propagación del caller

### Requirement: El Caller sobrevive wraps de contexto

El `Caller` MUST propagarse a través de wraps estándar de `context.Context`: `context.WithCancel`, `context.WithTimeout`, `context.WithDeadline` y `context.WithValue`. Esto valida que el paquete usa la primitiva correcta del stdlib y no un mecanismo custom que se rompa al envolver el contexto.

#### Scenario: Propagación a través de WithCancel

- GIVEN un `ctx` con un caller inyectado
- WHEN se hace `cancelCtx, cancel := context.WithCancel(ctx)` y luego `WithCaller(cancelCtx, caller)`
- THEN `FromContext(cancelCtx)` MUST devolver `(caller, true)`
- AND aún después de llamar `cancel()`, `FromContext(cancelCtx)` MUST seguir devolviendo `(caller, true)` (la cancelación no borra valores)

#### Scenario: Propagación a través de WithTimeout

- GIVEN un `ctx` con un caller inyectado
- WHEN se hace `timeoutCtx, cancel := context.WithTimeout(ctx, 1*time.Second)` y luego `WithCaller(timeoutCtx, caller)`
- THEN `FromContext(timeoutCtx)` MUST devolver `(caller, true)` antes de que expire el timeout

### Requirement: Sin dependencias externas

El paquete `internal/auth` MUST importar únicamente la stdlib de Go (`context`, posiblemente `errors` para sentinels en archivos adyacentes). MUST NO agregar dependencias a `go.mod` para esta capacidad. Esto cumple ADR-0005 (no nuevas deps externas).

#### Scenario: Importaciones mínimas

- GIVEN el código bajo `internal/auth/` (excluyendo `*_test.go`)
- WHEN se enumeran los imports
- THEN el único paquete externo permitido es la stdlib; NO debe haber `github.com/...` ni `golang.org/x/...` en archivos de producción de este paquete

## Notes

- Esta spec describe SOLO las primitivas de contexto. NO define los valores permitidos de `Role` (eso es `auth-roles`), ni cómo se obtiene el caller del request HTTP (eso es `auth-middleware`).
- El uso de punteros (`*string`) para `ProfessionalID` y `ClientID` permite distinguir "ausente" de "string vacío" sin sentinels. El cero value de `*string` es `nil` y representa correctamente la ausencia.
- `WithCaller` y `FromContext` se usan en pares. **En Fase 2+**, los repositorios business (BookingsRepo, ClientsRepo, etc.) deberían llamar `FromContext` al inicio de cada método público y rechazar (`ErrUnauthenticated`) si `ok == false` (ver `auth-middleware` para el caso típico). **AccountsRepo importa `internal/auth` únicamente para `auth.FromContext(ctx)`** (lectura del caller desde ctx para audit log); no usa la API de `auth` para enforcement. El enforcement de admin-only se hace en el middleware Fase 2 (ver `design.md` Decisión 5).
- El coverage target para `internal/auth/` es ≥ 80% (per `data-access` meta-spec de `feat-db-layer` y la propuesta `feat-authorization` §Success Criteria).
