# Spec: accounts-repo

> Reference: `docs/PRD.md` §3.8.2 (schema de `accounts`); `docs/architecture/0009-authorization-model.md` Decisión; `openspec/changes/feat-db-layer/specs/data-access/spec.md` (meta-spec de la capa `internal/repository/`: prepared statements, sentinels, `go-sqlmock`, ≥80% cobertura)
> Change: feat-authorization
> Status: NEW (no prior spec existed)

## Purpose

El sistema debe persistir la whitelist de cuentas con permisos elevados (`admin` y `staff`) en la tabla `accounts` y exponer un repositorio que centralice el CRUD con prepared statements, sentinels de error y cobertura de tests ≥ 80% con `go-sqlmock`. Sin este repo no hay forma de gestionar la whitelist desde un futuro script de admin, TUI o handler MCP.

## Requirements

### Requirement: Estructura del modelo `Account`

`internal/model` MUST exportar el struct `Account` mapeando 1:1 a las columnas de `accounts`. `ProfessionalID *string` distingue "ausente" (admin) de "string vacío". `IsActive bool` se serializa a `0`/`1` en SQLite. `CreatedAt` y `UpdatedAt` son strings ISO 8601 UTC con milisegundos.

```go
type Account struct {
    ID             string
    Role           string  // "admin" o "staff"
    DisplayName    string
    ProfessionalID *string // no-nil solo si Role == "staff"
    IsActive       bool
    CreatedAt      string
    UpdatedAt      string
}
```

#### Scenario: Admin con ProfessionalID nil

- GIVEN un admin sin professional_id
- WHEN se construye `Account{ID: "+5491100000000", Role: "admin", IsActive: true}`
- THEN `ProfessionalID == nil`

#### Scenario: Staff con ProfessionalID seteado

- GIVEN un staff con `professional_id = "p-001"`
- WHEN se construye `Account{Role: "staff", ProfessionalID: &"p-001"}`
- THEN `*ProfessionalID == "p-001"`

### Requirement: Constructor del repo recibe `*sql.DB` ya abierto

`AccountsRepo` MUST ser un struct con `db *sql.DB` privado y constructor `NewAccountsRepo(db *sql.DB) *AccountsRepo`. El constructor MUST NO abrir la conexión ni ejecutar migrations (per convención `data-access`).

#### Scenario: Constructor no abre conexión

- GIVEN un `*sql.DB` ya abierto
- WHEN se llama `NewAccountsRepo(db)`
- THEN MUST retornar `*AccountsRepo` con `db` asignado
- AND no se ejecuta ningún `sql.Open`, `Ping` ni `Exec`

### Requirement: `Create` inserta con validación de role

`Create(ctx, *Account) error` MUST ejecutar `INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`. Validaciones previas (rechazo con `ErrInvalidInput` sin tocar la DB):
- `ID` MUST no ser vacío.
- `Role` MUST ser exactamente `"admin"` o `"staff"`.
- Si `Role == "staff"`, `ProfessionalID` MUST ser no-nil y apuntar a un string no vacío.

UNIQUE-constraint violation en `id` MUST mapearse a `ErrConflict` envuelto.

#### Scenario: Insert de admin exitoso

- GIVEN una cuenta con `Role = "admin"`, `IsActive = true`
- WHEN se llama `Create(ctx, &account)`
- THEN el repo MUST ejecutar el INSERT con los 5 placeholders y retornar `nil`

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

`GetByRole(ctx, role) ([]*Account, error)` MUST ejecutar `SELECT ... FROM accounts WHERE role = ? ORDER BY created_at ASC`. Si no hay filas, MUST retornar slice vacío (no nil) y `nil`. Si `role` no es `"admin"` o `"staff"`, MUST retornar `ErrInvalidInput` sin tocar la DB.

#### Scenario: GetByRole con múltiples admins

- GIVEN tres filas con `role = "admin"` y dos con `role = "staff"`
- WHEN se llama `GetByRole(ctx, "admin")`
- THEN MUST retornar un slice con 3 elementos ordenados por `created_at` ASC

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

### Requirement: `Delete` elimina una cuenta

`Delete(ctx, id) error` MUST ejecutar `DELETE FROM accounts WHERE id = ?`. Si 0 rows affected, MUST retornar `ErrNotFound`. Si `id` es vacío, MUST retornar `ErrInvalidInput`.

#### Scenario: Delete exitoso

- GIVEN una fila con el id consultado
- WHEN se llama `Delete(ctx, "+5491100000000")`
- THEN `RowsAffected()` MUST ser 1 y el error MUST ser `nil`

#### Scenario: Delete de id inexistente

- GIVEN ninguna fila con el id consultado
- WHEN se llama `Delete(ctx, "missing")`
- THEN MUST retornar `ErrNotFound` envuelto

#### Scenario: Delete con id vacío

- WHEN se llama `Delete(ctx, "")`
- THEN MUST retornar `ErrInvalidInput` sin ejecutar el DELETE

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

## Notes

- Este spec es ortogonal a `Caller` (`auth-identity`) y a la validación semántica de roles (`auth-roles`); el repo sólo conoce strings, la conversión a `Caller` la hace el middleware.
- Coverage target ≥ 80% consistente con la meta-spec `data-access` de `feat-db-layer` y con la propuesta `feat-authorization` §Success Criteria.
- `is_active` se almacena como `INTEGER` en SQLite; el modelo lo expone como `bool`. La conversión `0` → `false`, `1` → `true` es responsabilidad del repo.
- Las convenciones `ORDER BY created_at ASC` / `ORDER BY display_name ASC` son determinísticas: el caller no reordena, y las expectations de go-sqlmock son posicionalmente estables.
