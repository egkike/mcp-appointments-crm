# Spec: auth-roles

> Reference: `docs/PRD.md` §3.8 (Modelo de Autorización), §3.8.1 (por qué `accounts` solo contiene admin y staff), §3.8.2 (schema de `accounts`); `docs/architecture/0009-authorization-model.md` Decisión (schema), Consecuencias, Rejected alternatives (a)-(d)
> Change: feat-authorization
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe definir los cuatro roles (`owner`, `admin`, `staff`, `client`) que un caller puede tener, junto con la tabla `accounts` que actúa como whitelist para los roles elevados. La tabla almacena las cuentas con permisos elevados (`owner`, `admin`, `staff`), enforza vía CHECK constraints que todo `staff` tiene un `professional_id` válido, y distingue explícitamente a los `client` por su presencia en `clients` (no en `accounts`). Esta spec NO cubre el parsing de headers ni la inyección en `context.Context` (eso es `auth-middleware`).

## Requirements

### Requirement: Cuatro roles canónicos como constantes

El paquete `internal/auth` MUST exportar cuatro constantes de tipo `string` con los valores exactos:

- `RoleOwner  = "owner"`
- `RoleAdmin  = "admin"`
- `RoleStaff  = "staff"`
- `RoleClient = "client"` (esta constante existe pero NO se usa en `accounts.role`; los `client` se identifican por su presencia en `clients`)

Estas constantes MUST ser los únicos valores válidos para `Caller.Role` en todo el sistema. `RoleOwner` y `RoleAdmin` son los únicos valores válidos para `accounts.role` (junto con `RoleStaff`); `RoleClient` es el valor implícito para callers identificados via `clients`. `owner` y `admin` tienen los mismos permisos operacionales; `owner` además tiene la capacidad exclusiva de crear/eliminar otros admins (single-owner invariant, ver auth-middleware RBAC).

#### Scenario: Constantes exportadas con los valores correctos

- GIVEN el código bajo `internal/auth/`
- WHEN se enumeran los símbolos exportados
- THEN `RoleOwner`, `RoleAdmin`, `RoleStaff` y `RoleClient` MUST estar entre ellos
- AND sus valores MUST ser exactamente `"owner"`, `"admin"`, `"staff"` y `"client"` respectivamente

#### Scenario: Validación rechaza roles desconocidos

- GIVEN un `caller` con `Role = "manager"` (valor que no está en las cuatro constantes)
- WHEN cualquier repositorio o middleware lo inspecciona
- THEN el sistema MUST tratarlo como error y retornar un semantic error en español (no debe pasarlo como si fuera owner/admin/staff/client)

### Requirement: Schema canónico de la tabla `accounts`

La tabla `accounts` MUST crearse durante `initSchema` con el siguiente esquema (verbatim de `docs/PRD.md` §3.8.2 y ADR-0009):

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

Notas:
- `id` es el phone o handle del messenger. Es la PK.
- `role` SOLO puede ser `'owner'`, `'admin'` o `'staff'` (los `client` NO tienen fila en `accounts`).
- `is_active` es `0` o `1`; por defecto `1`.
- `created_at` y `updated_at` siguen el formato ISO 8601 UTC con milisegundos (regex `^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}\.\d{3}Z$`), consistentes con el resto del proyecto.

#### Scenario: Tabla accounts existe con todas las columnas

- GIVEN un `initSchema` ejecutado contra una base de datos fresca
- WHEN se hace `PRAGMA table_info(accounts)`
- THEN el resultado MUST incluir `id`, `role`, `display_name`, `professional_id`, `is_active`, `created_at`, `updated_at` con los tipos `TEXT` o `INTEGER` correspondientes

#### Scenario: Default de is_active es 1

- GIVEN una tabla `accounts` recién creada (sin filas)
- WHEN se inserta una fila con sólo `(id, role)` y el resto por defecto
- THEN el `is_active` almacenado MUST ser `1`
- AND `created_at` y `updated_at` MUST tener el formato ISO 8601 UTC con milisegundos

### Requirement: CHECK constraint de role en DB

La tabla `accounts` MUST enforzar a nivel SQLite que `role` solo puede ser `'owner'`, `'admin'` o `'staff'`. Un INSERT con `role = 'client'` (u otro valor) MUST ser rechazado por la base de datos con un CHECK-constraint violation.

#### Scenario: Insert con role inválido falla

- GIVEN la tabla `accounts` vacía
- WHEN se ejecuta `INSERT INTO accounts (id, role) VALUES ('+5491100001111', 'manager')`
- THEN SQLite MUST rechazar la sentencia con un CHECK-constraint error
- AND el repositorio MUST surface eso como un `ErrInvalidInput` o `ErrConflict` con mensaje en español

#### Scenario: Insert con role client es rechazado por accounts

- GIVEN la tabla `accounts` vacía
- WHEN se ejecuta `INSERT INTO accounts (id, role) VALUES ('+5491100001111', 'client')`
- THEN SQLite MUST rechazar la sentencia (los clients NO van en `accounts`)
- AND el repositorio MUST surface eso como un `ErrInvalidInput` semántico

### Requirement: CHECK constraint staff-implica-professional_id

La tabla `accounts` MUST enforzar a nivel DB que toda fila con `role = 'staff'` tenga un `professional_id` NO-NULO. Esto se logra con la segunda CHECK: `((role = 'staff' AND professional_id IS NOT NULL) OR (role IN ('admin', 'owner')))`.

#### Scenario: Staff sin professional_id es rechazado

- GIVEN la tabla `accounts` vacía
- WHEN se ejecuta `INSERT INTO accounts (id, role) VALUES ('+5491100001111', 'staff')` (sin `professional_id`)
- THEN SQLite MUST rechazar la sentencia con CHECK-constraint violation
- AND el repositorio MUST surface eso como `ErrInvalidInput`

#### Scenario: Admin o owner con professional_id es aceptado (campo ignorado)

- GIVEN la tabla `accounts` vacía
- WHEN se ejecuta `INSERT INTO accounts (id, role) VALUES ('+5491100000000', 'admin')` o `('owner')` con `professional_id = 'p-001'` o NULL
- THEN SQLite MUST aceptar la sentencia (el CHECK permite admin/owner con o sin `professional_id`)

#### Scenario: Staff con professional_id es aceptado

- GIVEN la tabla `accounts` vacía y un profesional `p-001` existente
- WHEN se ejecuta `INSERT INTO accounts (id, role, professional_id) VALUES ('+5491100002222', 'staff', 'p-001')`
- THEN SQLite MUST aceptar la sentencia

### Requirement: Single-owner invariant (un solo owner activo a la vez)

La tabla `accounts` MUST enforzar a nivel DB y a nivel de repo que existe a lo sumo una fila activa con `role='owner'` en cualquier momento. Esto se logra con dos capas:

1. **Trigger SQLite** `accounts_single_owner` que fires on INSERT y UPDATE: rechaza la operación si resultaría en más de un `role='owner'` activo.
2. **Repo check** en `AccountsRepo.Create`: antes de insertar un `role='owner'`, ejecuta `SELECT COUNT(*) FROM accounts WHERE role='owner' AND is_active=1`; si > 0, retorna `ErrConflict` con mensaje en español.

Defense-in-depth: el trigger es la fuente de verdad (enforce a nivel DB); el repo check da un mensaje de error claro y evita depender solo del trigger.

#### Scenario: Primer owner se inserta exitosamente

- GIVEN la tabla `accounts` vacía
- WHEN se ejecuta `INSERT INTO accounts (id, role, display_name) VALUES ('+5491100000000', 'owner', 'Dueño')`
- THEN el trigger MUST aceptar (no hay otro owner activo)
- AND la fila se inserta

#### Scenario: Segundo owner activo es rechazado por el trigger

- GIVEN una fila existente con `id='+5491100000000'`, `role='owner'`, `is_active=1`
- WHEN se ejecuta `INSERT INTO accounts (id, role, display_name) VALUES ('+5491100001111', 'owner', 'Otro Owner')`
- THEN el trigger MUST rechazar la sentencia con un error (e.g., "RAISE function ... single-owner invariant")
- AND el repo MUST surface eso como `ErrConflict` con mensaje en español

#### Scenario: Owner desactivado permite crear un nuevo owner (transfer ownership)

- GIVEN una fila existente con `id='+5491100000000'`, `role='owner'`, `is_active=0` (desactivado)
- WHEN se ejecuta `INSERT INTO accounts (id, role, display_name) VALUES ('+5491100001111', 'owner', 'Nuevo Dueño')`
- THEN el trigger MUST aceptar la sentencia (no hay otro owner **activo**)
- AND la fila se inserta

#### Scenario: Reactivar un owner desactivado es rechazado

- GIVEN una fila con `role='owner'`, `is_active=0` y otra fila con `role='owner'`, `is_active=1` (transfer en progreso)
- WHEN se ejecuta `UPDATE accounts SET is_active=1 WHERE id=<owner desactivado>`
- THEN el trigger MUST rechazar la sentencia (resultaría en 2 owners activos)
- AND el repo MUST surface eso como `ErrConflict`

### Requirement: Determinación del role del caller (resolución)

La función de resolución del caller (referenciada por `auth-middleware` pero especificada aquí por contrato) MUST seguir el siguiente orden de búsqueda, en dos queries máximo:

1. `SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?` (sin filtro de `is_active` en SQL) — si devuelve una fila:
   - Si `is_active = 1` → el caller es `{ID, Role: row.role, ProfessionalID: row.professional_id, ClientID: nil}`. NO consulta `clients`.
   - Si `is_active = 0` → el caller MUST ser rechazado con un semantic error en español (`"tu cuenta está deshabilitada. Contacta al administrador."`). NO consulta `clients`.
2. Si `accounts` no tiene fila para ese `id` → `SELECT id FROM clients WHERE id = ?` — si devuelve una fila, el caller es `{ID, Role: "client", ProfessionalID: nil, ClientID: &id}`.
3. Si no hay fila en ninguna de las dos tablas, MUST retornar `ErrUnauthenticated` con mensaje en español (`"no te reconozco. Por favor registrate primero."`).

Esta función MUST ejecutarse dentro de un `*sql.DB` y MUST usar el `context.Context` recibido para cancelación. MUST NO ser un singleton global: vive en el middleware o en un helper inyectable.

#### Scenario: Caller en accounts como admin

- GIVEN una fila en `accounts` con `id = '+5491100000000'`, `role = 'admin'`, `is_active = 1`
- WHEN el resolver consulta con `'+5491100000000'`
- THEN MUST retornar un `Caller{ID: '+5491100000000', Role: "admin", ProfessionalID: nil, ClientID: nil}`
- AND MUST NO consultar `clients`

#### Scenario: Caller en accounts como staff con professional_id

- GIVEN una fila en `accounts` con `id = '+5491100002222'`, `role = 'staff'`, `professional_id = 'p-001'`, `is_active = 1`
- WHEN el resolver consulta con `'+5491100002222'`
- THEN MUST retornar un `Caller{ID: '+5491100002222', Role: "staff", ProfessionalID: &"p-001", ClientID: nil}`

#### Scenario: Caller en accounts con is_active=0 es rechazado

- GIVEN una fila en `accounts` con `id = '+5491100000000'`, `is_active = 0`
- WHEN el resolver consulta con `'+5491100000000'`
- THEN MUST retornar `ErrUnauthenticated` con mensaje en español que indica cuenta deshabilitada
- AND MUST NO consultar `clients` (un account desactivado no es un client)

#### Scenario: Caller solo en clients

- GIVEN ninguna fila en `accounts` con `id = '+5491100003333'`, y una fila en `clients` con `id = '+5491100003333'`
- WHEN el resolver consulta con `'+5491100003333'`
- THEN MUST retornar un `Caller{ID: '+5491100003333', Role: "client", ProfessionalID: nil, ClientID: &"+5491100003333"}`

#### Scenario: Caller en ninguna tabla es rechazado

- GIVEN ninguna fila en `accounts` ni en `clients` con `id = '+5491100099999'`
- WHEN el resolver consulta con `'+5491100099999'`
- THEN MUST retornar `ErrUnauthenticated` con mensaje en español `"no te reconozco. Por favor registrate primero."`

## Notes

- El cliente (`client`) NO tiene fila en `accounts` por diseño (PRD §3.8.1, ADR-0009 Rejected alternative c). Su rol se infiere por presencia en `clients.id`.
- El campo `business_profile.messenger_id` identifica la cuenta del BOT del negocio, NO un caller — es ortogonal a `accounts`/`clients`.
- El segundo CHECK de la tabla es la materialización del "todo staff tiene FK a professional" (PRD §3.8.2). Esto enforza a nivel DB, no en Go, para que un INSERT directo vía SQL tampoco pueda saltarse la invariante.
- El coverage target para `internal/auth/` es ≥ 80% (per propuesta §Success Criteria). El test de la función de resolución del caller (este spec, Requirement "Determinación del role del caller") puede usar `go-sqlmock` para validar las queries y los roles devueltos; el middleware que la invoca se testea en `auth-middleware`.
