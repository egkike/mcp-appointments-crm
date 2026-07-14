# Spec: accounts-repo

> Reference: `docs/PRD.md` §3.8.2 (schema de `accounts`); `docs/architecture/0009-authorization-model.md` Decisión; `openspec/changes/feat-db-layer/specs/data-access/spec.md` (meta-spec de la capa `internal/repository/`: prepared statements, sentinels, `go-sqlmock`, ≥80% cobertura)
> Change: feat-authorization
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe persistir la whitelist de cuentas con permisos elevados (`owner`, `admin` y `staff`) en la tabla `accounts` y exponer un repositorio que centralice el CRUD con prepared statements, sentinels de error y cobertura de tests ≥ 80% con `go-sqlmock`. Sin este repo no hay forma de gestionar la whitelist desde un futuro script de admin, TUI o handler MCP.

## Requirements

### Requirement: Estructura del modelo `Account`

`internal/model` MUST exportar el struct `Account` mapeando 1:1 a las columnas de `accounts`. `ProfessionalID *string` distingue "ausente" (admin) de "string vacío". `IsActive bool` se serializa a `0`/`1` en SQLite. `CreatedAt` y `UpdatedAt` son strings ISO 8601 UTC con milisegundos.

```go
type Account struct {
    ID             string
    Role           string  // "owner" | "admin" | "staff"
    DisplayName    string
    ProfessionalID *string // no-nil solo si Role == "staff"
    IsActive       bool
    CreatedAt      string
    UpdatedAt      string
}
```

#### Scenario: Admin o owner con ProfessionalID nil

- GIVEN un admin (o owner) sin professional_id
- WHEN se construye `Account{ID: "+5491100000000", Role: "admin", IsActive: true}` o `Role: "owner"`
- THEN `ProfessionalID == nil`

#### Scenario: Staff con ProfessionalID seteado

- GIVEN un staff con `professional_id = "p-001"`
- WHEN se construye `Account{Role: "staff", ProfessionalID: &"p-001"}`
- THEN `*ProfessionalID == "p-001"`

### Requirement: Constructor del repo recibe `*sql.DB` y `*slog.Logger` ya configurados

`AccountsRepo` MUST ser un struct con `db *sql.DB` y `logger *slog.Logger` privados, y constructor `NewAccountsRepo(db *sql.DB, logger *slog.Logger) *AccountsRepo`. El constructor MUST NO abrir la conexión ni ejecutar migrations (per convención `data-access`). El logger se usa para audit log en `Create`, `Update` y `Deactivate` (ver Requirement "Logging de auditoría MUST").

#### Scenario: Constructor no abre conexión

- GIVEN un `*sql.DB` ya abierto y un `*slog.Logger` configurado
- WHEN se llama `NewAccountsRepo(db, logger)`
- THEN MUST retornar `*AccountsRepo` con `db` y `logger` asignados
- AND no se ejecuta ningún `sql.Open`, `Ping` ni `Exec`

### Requirement: `Create` inserta con validación de role y single-owner check

`Create(ctx, *Account) error` MUST ejecutar `INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`. Validaciones previas (rechazo con `ErrInvalidInput` sin tocar la DB):
- `ID` MUST no ser vacío.
- `Role` MUST ser exactamente `"owner"`, `"admin"` o `"staff"`.
- Si `Role == "staff"`, `ProfessionalID` MUST ser no-nil y apuntar a un string no vacío.

Single-owner check (defense-in-depth, además del trigger SQLite):
- Si `Role == "owner"`, antes del INSERT, ejecutar `SELECT COUNT(*) FROM accounts WHERE role='owner' AND is_active=1`. Si > 0, retornar `ErrConflict` envuelto con mensaje en español (`"ya existe un owner activo; desactívalo antes de crear otro"`).
- Si `Role != "owner"`, no aplicar el check.

UNIQUE-constraint violation en `id` MUST mapearse a `ErrConflict` envuelto.

#### Scenario: Insert de admin exitoso

- GIVEN una cuenta con `Role = "admin"`, `IsActive = true`
- WHEN se llama `Create(ctx, &account)`
- THEN el repo MUST ejecutar el INSERT con los 5 placeholders y retornar `nil`

#### Scenario: Insert de owner exitoso (primer owner)

- GIVEN la tabla `accounts` sin filas con `role='owner'`
- WHEN se llama `Create(ctx, &account)` con `Role = "owner"`
- THEN el repo MUST ejecutar el INSERT y retornar `nil`
- AND MUST emitir audit log con `actor_id` (del ctx, si hay) y `target_id=newAccountID, target_role="owner"`

#### Scenario: Rechazo si role es inválido

- GIVEN una cuenta con `Role = "client"` (o `"manager"`, o `""`)
- WHEN se llama `Create(ctx, &account)`
- THEN el repo MUST retornar `ErrInvalidInput` envuelto
- AND MUST NO ejecutar el INSERT

#### Scenario: Rechazo si staff sin professional_id

- GIVEN una cuenta con `Role = "staff"` y `ProfessionalID = nil`
- WHEN se llama `Create(ctx, &account)`
- THEN el repo MUST retornar `ErrInvalidInput`
- AND MUST NO ejecutar el INSERT

#### Scenario: Rechazo de segundo owner activo (single-owner invariant)

- GIVEN una fila existente con `id='+5491100000000'`, `role='owner'`, `is_active=1`
- WHEN se llama `Create(ctx, &account)` con `Role = "owner"` y un id diferente
- THEN el repo MUST ejecutar el `SELECT COUNT(*)` y retornar un error que envuelve `ErrConflict` con mensaje en español
- AND MUST NO ejecutar el INSERT
- AND MUST emitir audit log de intento de creación de segundo owner (defense-in-depth: el log permite detectar patrones de ataque)

#### Scenario: Transfer ownership: desactivar owner anterior permite crear uno nuevo

- GIVEN una fila existente con `id='+5491100000000'`, `role='owner'`, `is_active=0` (desactivado vía `Deactivate`)
- WHEN se llama `Create(ctx, &account)` con `Role = "owner"` y un id diferente
- THEN el repo MUST ejecutar el INSERT y retornar `nil` (el check cuenta solo owners **activos**)

#### Scenario: Rechazo si ID duplicado

- GIVEN una fila existente con `id = "+5491100000000"`
- WHEN se llama `Create(ctx, &account)` con la misma `id`
- THEN el repo MUST retornar un error que envuelve `ErrConflict` (verificable con `errors.Is(err, repository.ErrConflict)`)

### Requirement: `Get` retorna una cuenta por id

`Get(ctx, id) (*Account, error)` MUST ejecutar `SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE id = ?`. Si no hay fila, MUST retornar `ErrNotFound` envuelto.

#### Scenario: Get de cuenta existente

- GIVEN una fila con `id = "+5491100000000"`, `role = "admin"`
- WHEN se llama `Get(ctx, "+5491100000000")`
- THEN MUST retornar `*Account` con `ID == "+5491100000000"`, `Role == "admin"`, `error == nil`

#### Scenario: Get de staff popula ProfessionalID

- GIVEN una fila con `role = "staff"`, `professional_id = "p-001"`
- WHEN se llama `Get(ctx, "+5491100002222")`
- THEN el `*Account` retornado MUST tener `ProfessionalID != nil` con `*ProfessionalID == "p-001"`

#### Scenario: Get de id inexistente

- GIVEN ninguna fila con el id consultado
- WHEN se llama `Get(ctx, "missing")`
- THEN MUST retornar `nil` y un error que envuelve `ErrNotFound` (verificable con `errors.Is`)

### Requirement: `GetByRole` filtra por rol

`GetByRole(ctx, role) ([]*Account, error)` MUST ejecutar `SELECT ... FROM accounts WHERE role = ? ORDER BY created_at ASC`. Si no hay filas, MUST retornar slice vacío (no nil) y `nil`. Si `role` no es `"owner"`, `"admin"` o `"staff"`, MUST retornar `ErrInvalidInput` sin tocar la DB.

#### Scenario: GetByRole con múltiples owners+admins

- GIVEN dos filas con `role = "owner"`, tres con `role = "admin"` y dos con `role = "staff"` (en cualquier orden histórico)
- WHEN se llama `GetByRole(ctx, "owner")`
- THEN MUST retornar un slice con 2 elementos (los owners) ordenados por `created_at` ASC

#### Scenario: GetByRole sin resultados

- GIVEN ninguna fila con `role = "admin"`
- WHEN se llama `GetByRole(ctx, "admin")`
- THEN MUST retornar un slice vacío (length 0, NOT nil) y `nil`

#### Scenario: GetByRole con role inválido

- WHEN se llama `GetByRole(ctx, "client")` o `GetByRole(ctx, "manager")`
- THEN MUST retornar `nil, ErrInvalidInput` y MUST NO ejecutar la query

### Requirement: `List` retorna todas las cuentas

`List(ctx) ([]*Account, error)` MUST ejecutar `SELECT ... FROM accounts ORDER BY created_at ASC`. Si no hay filas, MUST retornar slice vacío (no nil).

#### Scenario: List con múltiples filas

- GIVEN 5 filas en la tabla
- WHEN se llama `List(ctx)`
- THEN MUST retornar un slice con 5 elementos ordenados por `created_at` ASC

#### Scenario: List en tabla vacía

- GIVEN ninguna fila
- WHEN se llama `List(ctx)`
- THEN MUST retornar un slice vacío (NOT nil) y `nil`

### Requirement: `Update` modifica una cuenta existente

`Update(ctx, *Account) error` MUST ejecutar `UPDATE accounts SET role = ?, display_name = ?, professional_id = ?, is_active = ?, updated_at = ? WHERE id = ?`. `updated_at` se regenera al momento del UPDATE. Si 0 rows affected, MUST retornar `ErrNotFound`. Validaciones análogas a `Create`.

#### Scenario: Update exitoso

- GIVEN una fila con `id = "+5491100000000"`, `display_name = "Old"`
- WHEN se llama `Update(ctx, &Account{ID: "+5491100000000", Role: "admin", DisplayName: "New", IsActive: true})`
- THEN `RowsAffected()` MUST ser 1 y el error MUST ser `nil`

#### Scenario: Update de fila inexistente

- GIVEN ninguna fila con el id consultado
- WHEN se llama `Update(ctx, &Account{ID: "missing", Role: "admin"})`
- THEN MUST retornar `ErrNotFound` envuelto

#### Scenario: Update con role inválido

- WHEN se llama `Update(ctx, &Account{Role: "manager"})`
- THEN MUST retornar `ErrInvalidInput` sin ejecutar el UPDATE

### Requirement: `Deactivate` (soft delete) desactiva una cuenta

`Deactivate(ctx, id) error` MUST ejecutar `UPDATE accounts SET is_active = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`. Si 0 rows affected, MUST retornar `ErrNotFound`. Si `id` es vacío, MUST retornar `ErrInvalidInput`. Si la fila ya tiene `is_active=0`, el UPDATE es no-op (no error, no audit log duplicado).

`Delete` (hard delete) se deprecaba: las cuentas deben desactivarse vía `Deactivate`, no eliminarse. Hard delete solo se permite via un sub-comando administrativo explícito (e.g., `purge-inactive` en el TUI, con confirmación extra) — fuera del scope de este change.

#### Scenario: Deactivate exitoso

- GIVEN una fila con el id consultado y `is_active = 1`
- WHEN se llama `Deactivate(ctx, "+5491100000000")`
- THEN `RowsAffected()` MUST ser 1, la fila en DB tiene `is_active = 0`, y el error MUST ser `nil`
- AND MUST emitir audit log con `action="deactivate_account"`, `target_id="+5491100000000"`, `target_role=<role actual>`

#### Scenario: Deactivate de id inexistente

- GIVEN ninguna fila con el id consultado
- WHEN se llama `Deactivate(ctx, "missing")`
- THEN MUST retornar `ErrNotFound` envuelto

#### Scenario: Deactivate con id vacío

- WHEN se llama `Deactivate(ctx, "")`
- THEN MUST retornar `ErrInvalidInput` sin ejecutar el UPDATE

#### Scenario: Deactivate idempotente (segunda llamada es no-op)

- GIVEN una fila con `is_active = 0` (ya desactivada)
- WHEN se llama `Deactivate(ctx, "+5491100000000")` de nuevo
- THEN `RowsAffected()` MUST ser 0 y el error MUST ser `nil` (no es error desactivar algo ya desactivado)
- AND MUST NO emitir audit log duplicado

### Requirement: `IsActive` chequea si la cuenta está activa

`IsActive(ctx, id) (bool, error)` MUST ejecutar `SELECT is_active FROM accounts WHERE id = ?`. Si la fila NO existe, MUST retornar `(false, nil)` — NO `ErrNotFound` (es una pregunta booleana, no un lookup).

#### Scenario: IsActive para cuenta activa

- GIVEN una fila con `is_active = 1`
- WHEN se llama `IsActive(ctx, "+5491100000000")`
- THEN MUST retornar `(true, nil)`

#### Scenario: IsActive para cuenta inactiva

- GIVEN una fila con `is_active = 0`
- WHEN se llama `IsActive(ctx, "+5491100000000")`
- THEN MUST retornar `(false, nil)` (no es error)

#### Scenario: IsActive para id inexistente

- GIVEN ninguna fila con el id consultado
- WHEN se llama `IsActive(ctx, "missing")`
- THEN MUST retornar `(false, nil)` sin error

### Requirement: `ListByProfessional` filtra staff por profesional

`ListByProfessional(ctx, professionalID) ([]*Account, error)` MUST ejecutar `SELECT ... FROM accounts WHERE role = 'staff' AND professional_id = ? ORDER BY display_name ASC`. Si no hay filas, MUST retornar slice vacío (no nil). Si `professionalID` es vacío, MUST retornar `ErrInvalidInput`.

#### Scenario: ListByProfessional con múltiples staff

- GIVEN dos filas con `role = "staff"`, `professional_id = "p-001"`, y una con `professional_id = "p-002"`
- WHEN se llama `ListByProfessional(ctx, "p-001")`
- THEN MUST retornar un slice con 2 elementos, todos con `ProfessionalID == &"p-001"`, ordenados por `DisplayName` ASC

#### Scenario: ListByProfessional excluye admins

- GIVEN una fila con `role = "admin"` y `professional_id = "p-001"` (caso edge hipotético)
- WHEN se llama `ListByProfessional(ctx, "p-001")`
- THEN MUST retornar un slice vacío (la query filtra por `role = 'staff'`)

#### Scenario: ListByProfessional con id vacío

- WHEN se llama `ListByProfessional(ctx, "")`
- THEN MUST retornar `nil, ErrInvalidInput` sin ejecutar la query

### Requirement: Errores envueltos con sentinels

Todos los errores MUST estar envueltos con `fmt.Errorf("...: %w", err)`. Mensajes MUST ser frases en español (per coding standards); MUST NO contener stack traces, `goroutine`, `.go:`, ni file paths. Los sentinels `ErrNotFound`, `ErrConflict`, `ErrInvalidInput` están en `internal/repository/errors.go` (ver `data-access` spec).

#### Scenario: Sentinel se preserva a través del wrap

- GIVEN un método que retorna `ErrNotFound` o `ErrConflict`
- WHEN el caller hace `errors.Is(err, repository.ErrNotFound)` (o `ErrConflict`)
- THEN MUST ser `true`

#### Scenario: Mensaje en español sin stack trace

- GIVEN cualquier error retornado por el repo
- WHEN se inspecciona `err.Error()`
- THEN la cadena MUST ser una frase en español (ej. `"cuenta con id '+5491100099999' no encontrada"`)
- AND MUST NO contener `goroutine`, `.go:`, ni file paths

### Requirement: Prepared statements exclusivamente

Todas las queries MUST usar `?` placeholders. Ningún método MUST concatenar valores de `account` o `id` en el string SQL. Los nombres de tabla/columna son constantes del paquete, NO derivados de input del usuario.

#### Scenario: Las queries usan placeholders

- GIVEN el código fuente de `internal/repository/accounts.go`
- WHEN se enumeran los strings SQL
- THEN cada query MUST contener al menos un `?` placeholder para los valores variables
- AND no debe haber `fmt.Sprintf` ni concatenación construyendo SQL a partir de campos de `*Account` o `id`

### Requirement: Tests con `go-sqlmock`

Cada método público MUST tener tests en `internal/repository/accounts_test.go` que cubran al menos: happy path, error de DB genérico, not-found (cuando aplique), conflict (cuando aplique), invalid input (cuando aplique). Los tests MUST usar `go-sqlmock` (in-memory) y correr bajo `go test -v -race ./...` sin data races.

#### Scenario: Test de Create cubre happy path y UNIQUE violation

- GIVEN un mock que espera el INSERT con los placeholders correctos
- WHEN se ejecuta el test del happy path
- THEN el mock MUST satisfacerse y el error MUST ser `nil`
- AND un segundo sub-test con mock retornando UNIQUE violation MUST producir un error que envuelve `ErrConflict`

#### Scenario: Tests pasan con race detector

- GIVEN la suite de tests
- WHEN se ejecuta `go test -v -race ./internal/repository/...`
- THEN el comando MUST exit 0 y MUST NO reportar data races

### Requirement: Cobertura ≥ 80% en el archivo del repo

La suite MUST alcanzar cobertura de líneas ≥ 80% medida con `go test -cover`. El PR description MUST incluir el output (per convención `data-access` spec).

#### Scenario: Threshold de cobertura cumplido

- GIVEN la suite de tests
- WHEN se ejecuta `go test -v -race -cover ./internal/repository/...`
- THEN la cobertura reportada MUST ser ≥ 80%

### Requirement: Logging de auditoría MUST en operaciones críticas

Las operaciones que mutan la tabla `accounts` MUST emitir un audit log estructurado via `*slog.Logger` (inyectado por el constructor del repo, ver Requirement "Constructor del repo"). El log es **MUST**, no MAY. Operaciones cubiertas:

- `Create(ctx, account)`: emite `slog.Info("account created", "actor_id", <ctx caller>, "target_id", account.ID, "target_role", account.Role, "ts", <ISO 8601 UTC>)`. **Excepción**: si el `Create` es del primer owner (durante TUI menú o seed inicial), el `actor_id` puede ser `"tui"` o `"system"` (no hay caller de ctx).
- `Deactivate(ctx, id)`: emite `slog.Info("account deactivated", "actor_id", <ctx caller>, "target_id", id, "target_role", <role of deactivated account>, "ts", <ISO 8601 UTC>)`. **No emite** si la cuenta ya estaba desactivada (idempotencia).
- `Update(ctx, account)`: emite `slog.Info("account updated", "actor_id", <ctx caller>, "target_id", account.ID, "target_role", account.Role, "ts", <ISO 8601 UTC>)`.

`Get`, `GetByRole`, `List`, `ListByProfessional`, `IsActive`, `Create` que retorna error de UNIQUE conflict — NO emiten audit log (son read-only o errores esperados, no operaciones exitosas que valen auditar).

`Create` que retorna error de single-owner violation — **SÍ emite audit log** de intento (defense-in-depth: el log permite detectar patrones de ataque, ej: varios intentos de crear owner por un caller no-owner). El `actor_id` es el caller del ctx; el `target_role` es `"owner"`. El log es `slog.Warn` (no `slog.Info`) porque es un evento de seguridad.

Si el `actor_id` no se puede obtener del `ctx` (no hay caller), el log se omite el campo (no es error).

#### Scenario: Create emite audit log

- GIVEN un `ctx` con `auth.Caller{ID: "+5491100000000", Role: "admin"}`
- WHEN se llama `Create(ctx, &Account{ID: "+5491100001111", Role: "staff", ProfessionalID: &"p-001", IsActive: true})` exitosamente
- THEN el logger MUST emitir un log con `msg="account created"`, `actor_id="+5491100000000"`, `target_id="+5491100001111"`, `target_role="staff"`, `ts=<ISO 8601 UTC>`

#### Scenario: Deactivate emite audit log

- GIVEN un `ctx` con `auth.Caller{ID: "+5491100000000", Role: "admin"}`
- AND una cuenta activa con `id="+5491100002222"`, `role="staff"`
- WHEN se llama `Deactivate(ctx, "+5491100002222")` exitosamente
- THEN el logger MUST emitir un log con `msg="account deactivated"`, `actor_id="+5491100000000"`, `target_id="+5491100002222"`, `target_role="staff"`, `ts=<ISO 8601 UTC>`

#### Scenario: Deactivate idempotente no emite audit log duplicado

- GIVEN una cuenta ya desactivada
- WHEN se llama `Deactivate(ctx, <id>)` (segunda llamada)
- THEN MUST NO emitirse audit log (RowsAffected=0 indica no-op)

#### Scenario: Create sin caller en ctx omite actor_id

- GIVEN un `ctx` sin `auth.Caller` (e.g., durante TUI menú o seed inicial)
- WHEN se llama `Create(ctx, &Account{...})` exitosamente
- THEN el logger MUST emitir un log con `msg="account created"`, **sin** `actor_id`, pero con `target_id` y `target_role`

## Notes

- Este spec es ortogonal a `Caller` (`auth-identity`) y a la validación semántica de roles (`auth-roles`); el repo sólo conoce strings, la conversión a `Caller` la hace el middleware.
- Coverage target ≥ 80% consistente con la meta-spec `data-access` de `feat-db-layer` y con la propuesta `feat-authorization` §Success Criteria.
- `is_active` se almacena como `INTEGER` en SQLite; el modelo lo expone como `bool`. La conversión `0` → `false`, `1` → `true` es responsabilidad del repo.
- Las convenciones `ORDER BY created_at ASC` / `ORDER BY display_name ASC` son determinísticas: el caller no reordena, y las expectations de go-sqlmock son posicionalmente estables.
