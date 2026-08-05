# ADR-0013: Layered architecture — Clean Architecture + Ports & Adapters (Hexagonal)

- **Status**: accepted
- **Date**: 2026-08-05
- **Authors**: Kike
- **Supersedes**: none
- **Related**: PRD.md §2.4 (Architectural goals), refactor-clean-architecture chain (PRs #42, #44, #45)

## Context

El proyecto empezó el 2026-06-24 como una `internal/`-only library (sin binario, sin `main.go`). A medida que creció (use cases, repos, DTOs, validadores, auth helpers, FTS5, schema bootstrap), la separación entre **dominio** e **infraestructura** se fue volviendo difusa:

- El paquete `internal/model/` contenía tipos de dominio (Client, Booking, Service, etc.) que se usaban indistintamente desde la capa de aplicación, de infraestructura y de los repositorios.
- Los repositorios mezclaban lógica de negocio (validación de specialty IDs, normalización de phone) con queries SQL crudas.
- No había un límite arquitectónico explícito: cualquier package podía importar cualquier otro, y la convención "domain no depende de nadie" no estaba formalizada.
- El primer binario del proyecto (`cmd/mcp-server/main.go`, PR #45) no existía — el wiring DI estaba deferido como TASK-FU.3 desde `feat-booking-validator-service` (PRs #38, #39).

El refactor `refactor-clean-architecture` (PRs #42, #44, #45, commits `3bad038` → `8c90861` → `c38fbc6` → `988baeb`) fue la respuesta explícita a esta deuda arquitectónica. Esta ADR documenta la **decisión de diseño resultante** — el patrón que el proyecto adoptó y que toda la base de código respeta hoy.

## Decision

El proyecto adopta **Clean Architecture + Ports & Adapters (Hexagonal)** como su modelo de capas. La consecuencia más visible: existen **dos carpetas `repository/`** en `internal/`, con funciones complementarias y opuestas.

### Las dos carpetas `repository/`

#### `internal/domain/repository/` — **Los puertos (contratos)**

Define **QUÉ** necesita el dominio para persistir datos, sin saber CÓMO se implementa.

- 9 archivos, ~9 KB totales (uno por aggregate root: `clients.go`, `bookings.go`, `services.go`, `professionals.go`, `schedules.go`, `accounts.go`, `business_profile.go`, `business_hours_exception.go`, `pending_alerts.go`).
- Cada archivo declara una sola interface (p. ej. `type ClientsRepo interface { FindByID, FindByPhone, Save, ... }`).
- **Solo importa `context` y `internal/domain/entity`** (los structs de dominio y los errores de dominio).
- **NO** sabe nada de SQLite, `database/sql`, prepared statements, ni SQL.
- **NO** sabe nada de cómo se carga la conexión a la base.
- Ejemplo canónico (`internal/domain/repository/clients.go`):

```go
package repository

// ClientsRepo defines the persistence contract for Client aggregates.
// Implementations must return domain.ErrNotFound when a lookup misses.
type ClientsRepo interface {
    FindByID(ctx context.Context, id string) (*entity.Client, error)
    FindByPhone(ctx context.Context, phone string) (*entity.Client, error)
    Save(ctx context.Context, c *entity.Client) error
}
```

#### `internal/repository/` — **Los adaptadores (implementaciones)**

Implementa **CÓMO** se persiste el dominio, usando SQLite (`modernc.org/sqlite`).

- 9 archivos de implementación (uno por aggregate) + 9 archivos `_test.go` con tests usando `go-sqlmock` + helpers (`auth_filter.go`, `datetime.go`, `sqlite_errors.go`, `doc.go`).
- Importa la interface del dominio con **alias explícito**: `domainrepo "github.com/.../internal/domain/repository"`. El alias es deliberado para que el lector vea siempre, en cada línea, si se está usando el puerto o el adaptador.
- **Conoce `*sql.DB`, queries SQL, prepared statements, FTS5 virtual tables, triggers, busy_timeout, WAL mode** — toda la complejidad de SQLite vive aquí.
- **Línea mágica de compile-time check** (aparece en los 9 archivos):

  ```go
  var _ domainrepo.ClientsRepo = (*ClientsRepo)(nil)
  ```

  Esta línea es la red de seguridad más fuerte del proyecto: garantiza en **compile time** que cada adaptador satisface exactamente la interface del dominio. Si el dominio agrega un método a la interface, el adaptador **no compila** hasta que lo implemente.

- Ejemplo de la firma de un adaptador (`internal/repository/clients.go:17`):

  ```go
  // Compile-time interface conformance check.
  var _ domainrepo.ClientsRepo = (*ClientsRepo)(nil)

  // ClientsRepo provides CRUD, FTS5 search, and phone-based lookup for the
  // clients table. Phone is UNIQUE (serves as the chat ID for WhatsApp/Telegram).
  type ClientsRepo struct {
      db *sql.DB
  }
  ```

### El diagrama de dependencias

```
┌──────────────────────────────────────────────────┐
│ internal/domain/                                 │  ← La cúspide
│   ├─ entity/        (structs de dominio)         │
│   └─ repository/    (interfaces puras)           │  ← PUERTOS
│        ↑                                          │
│        │ depende solo de entity + context        │
│        │                                          │
│  ┌─────┴──────────────────────────────────────┐  │
│  │ internal/application/usecase/                │  ← Orquestación
│  │   (importa domain/repository — interfaces)   │  │  ← USA LOS PUERTOS
│  └──────────────────────────────────────────────┘  │
│        ↑                                          │
│        │                                          │
│  ┌─────┴──────────────────────────────────────┐  │
│  │ internal/repository/                         │  │  ← ADAPTADORES
│  │   (implementa domain/repository.*)           │  │     (los cables)
│  │   (importa domain/repository con alias)      │  │
│  │                                              │  │
│  │   La ÚNICA capa que conoce SQLite.           │  │
│  └──────────────────────────────────────────────┘  │
│        ↑                                          │
│        │ (solo cmd/mcp-server/main.go importa     │
│        │  AMBAS y las conecta)                     │
│  ┌─────┴──────────────────────────────────────┐  │
│  │ cmd/mcp-server/main.go                       │  │  ← COMPOSITION ROOT
│  │   (el ÚNICO lugar que importa ambos)         │  │     (el enchufe)
│  └──────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────┘
```

### Las tres reglas arquitectónicas (hard rules)

1. **Domain zero-dep rule** — `internal/domain/` no importa nada de `internal/` fuera de `internal/domain/*`. Por eso el `bookingValidator` interface vive en `internal/application/usecase/validator.go` y no en `internal/domain/service/` (el domain no puede importar `entity` desde ahí, y `service.ValidateBookingInput` referencia tipos de `entity`). Promotion a `internal/domain/service/` queda deferida hasta que aparezca un tercer consumer (D4 en `cmd/mcp-server/main.go`).

2. **Use cases solo importan interfaces, nunca implementaciones** — `internal/application/usecase/*.go` solo importa `internal/domain/repository` (con `import "github.com/.../internal/domain/repository"` sin alias), nunca `internal/repository`. Esto se verifica con grep como parte de la pre-flight:

   ```bash
   grep -L 'internal/repository"' internal/application/usecase/*.go
   # → todos los use cases aparecen
   ```

3. **Composition root único** — Solo `cmd/mcp-server/main.go` importa ambos packages y los cablea. Este es el ÚNICO lugar del proyecto donde una impl concreta (p. ej. `repository.NewClientsRepo(db)`) se conecta a una interface del dominio (p. ej. `domainrepo.ClientsRepo`). Las 9 líneas de compile-time check garantizan que esa conexión es válida.

### El analogía (instalación eléctrica)

- **`internal/domain/repository/`** es el **diagrama de tomacorrientes**: dice "necesito un enchufe de 220V con 3 patas". No le importa si la energía viene de un generador a gas, solar, o atómica.
- **`internal/repository/`** es el **generador concreto**: efectivamente entrega 220V con 3 patas usando una turbina específica (SQLite + modernc.org/sqlite).
- **`cmd/mcp-server/main.go`** es el **electricista**: el ÚNICO que sabe dónde está el generador y enchufa el cable al tomacorriente.

## Consequences

### Positive

- **Testabilidad sin tocar infra**: los use cases se testean con mocks que satisfacen las interfaces del dominio (ver `internal/application/usecase/mocks_test.go`). No hace falta levantar SQLite para validar la lógica de negocio. La cobertura de use cases corre en milisegundos con `go test -race`.
- **Swap de infraestructura sin tocar el dominio**: cambiar de SQLite a PostgreSQL, o agregar un in-memory cache, es trabajo de `internal/repository/` (nueva impl) + `cmd/mcp-server/main.go` (cambiar qué impl se inyecta). El dominio y los use cases siguen exactamente iguales.
- **Dominio puro y testeable por sí mismo**: `internal/domain/` no tiene dependencias de `database/sql`, ni de logs, ni de HTTP, ni de JSON. Los structs de `internal/domain/entity/` se pueden instanciar y validar en tests sin ningún setup. Las reglas de negocio viven en `internal/domain/service/` (AvailabilityService, BookingValidator) y son puras funciones, sin I/O.
- **Compile-time safety**: el `var _ domainrepo.XxxRepo = (*XxxRepo)(nil)` en los 9 adaptadores garantiza que cualquier drift entre el contrato del dominio y la impl se detecta al compilar, no al ejecutar. Si refactorizamos un método del dominio, los 9 adaptadores fallan al compilar y el programador sabe inmediatamente qué líneas tocar.
- **Onboarding legible**: la regla "domain no depende de nadie" se enseña en una línea. Una vez entendida, leer el código es leer capas. Un nuevo dev puede decir "este cambio toca la capa de aplicación, no toca infra" con solo ver los imports.

### Negative

- **Más archivos por dominio**: en lugar de 1 archivo por aggregate (lo que era `internal/model/`), ahora hay 2 (interface en `domain/repository/`, impl en `repository/`). Esto dobla la cantidad de archivos de persistencia.
- **Indirección para el lector**: para entender cómo se persiste un `Client`, hay que saltar entre `internal/domain/repository/clients.go` (la interface) y `internal/repository/clients.go` (la impl). En un proyecto de 1000 líneas esto es ruido; en uno de 50K como este, es claridad.
- **Onboarding más lento al principio**: el patrón Ports & Adapters no es intuitivo para devs que vienen de capas más planas (controllers/services/repositories con `internal/data/`). Requiere entender la diferencia entre "puerto" (contrato) y "adaptador" (impl) antes de poder contribuir.
- **Riesgo de "interface explosion"**: tentación de crear interfaces para todo. La regla práctica: solo se abstrae cuando hay 2+ implementaciones reales o cuando el testeo lo requiere. No se crean interfaces especulativas.

### Trade-offs aceptados

- **No hay DI container**: la wiring es manual en `cmd/mcp-server/main.go` (D9 del design original del refactor). Esto es ~150 LOC de código explícito y aburrido, pero es 100% trazable, no requiere reflexión, y no tiene overhead de runtime. Se prefirió explícito > mágico.
- **Compile-time check vs runtime assertion**: se eligió compile-time (`var _ Interface = (*Impl)(nil)`) sobre runtime assertions (`assert.Implements(...)` en tests). El compile-time es 0 LOC, 0 nanosegundos, y bloquea el build antes de que el código salga de la máquina del dev.
- **`bookingValidator` interface en usecase/ no en domain/**: el domain no puede importar entity, así que el narrow contract (que solo necesitan los use cases) vive en `internal/application/usecase/validator.go`. Promotion a `internal/domain/service/` queda deferida hasta que aparezca un 3er consumer (D4 documentado en `cmd/mcp-server/main.go`).

## References

### Implementación (PRs mergeados en main)

- **PR #42** — `feat(repository, domain): P3.3 entity enrichment (PR 1 of 4) (#22, #23)` — commit `f2f5969`. Cierra #22 (RescheduleBookingUseCase datetime validation) y #23 (datetime validation scattered). Introduce los 8 métodos nuevos de `internal/domain/entity/` (CanTransitionTo, ValidDuration, etc.) y mueve las validaciones cross-entity de los repos al dominio.
- **PR #44** — `refactor(repository): P3.4 infra cleanup (PR 2 of 4) — delete internal/model/, split errors.go, add idgen.NewUUID() wrapper` — commit `8c90861`. Borra `internal/model/` (11 archivos, 203 LOC), migra TODOS los `model.*` → `entity.*` en el repository layer, splitea `errors.go` → `doc.go` + `sqlite_errors.go`, agrega el wrapper `idgen.NewUUID()`.
- **PR #45** — `feat(cmd): P4.1+P4.2 composition root + phase 4 verify (PR 3/3, FINAL)` — commit `c38fbc6` (squash) + commit `93d4ca3` (GGA SUGGESTION follow-up). Crea `cmd/mcp-server/main.go` (142 líneas) como composition root. Resuelve TASK-FU.3 (BookingValidator singleton shared entre CreateBooking/RescheduleBooking use cases).

### Decisiones SDD intermedias

- **Commit `3bad038`** — `docs(sdd): record TASK-FU.3 wiring reminder in refactor-clean-architecture P4.1`. Captura el contexto del wiring de TASK-FU.3 (deferred desde `feat-booking-validator-service` PRs #38, #39) en el `tasks.md` del refactor-clean-architecture, antes de que P4.1 lo resolviera. Patrón a repetir: cuando un deferred follow-up cruza SDD changes, hacer un capture-commit explícito en el receiving change.

### Tareas deferidas resueltas por este cambio

- **TASK-FU.3** (de `feat-booking-validator-service`): "Full DI wiring cleanup in `cmd/mcp-server/main.go` (P4 of refactor-clean-architecture)". Resuelto por PR #45 con `service.NewBookingValidator()` construido una sola vez en el composition root y compartido entre los dos use cases de 7 args (CreateBooking, RescheduleBooking).

### Espec del proyecto (source of truth)

- `openspec/specs/architecture/spec.md` — la especificación contractual de las capas. Define 5 contratos (C1–C5) que cada fase del refactor debe satisfacer. Creada como parte del SDD `refactor-clean-architecture` y archivada en `openspec/changes/archive/2026-08-05-refactor-clean-architecture/`.

### Skills y patterns

- **Pattern #575** — P3.3 Entity Enrichment — Apply Progress (PR 1 of 4). Documenta el primer PR del chain y la mecánica del work-unit commit pattern.
- **Pattern #579** — Judgment Day round 1+2 pattern (CRITICAL fix + scoped re-judgment) for refactor-clean-architecture. Documenta el flujo de 2 jueces ciegos en paralelo que usamos para validar los 3 PRs.
- **AGENTS.md** §"Project Stack" y §"Coding Standards" — el nivel de la red de seguridad: prepared statements con `?`, GGA pre-commit, `go test -race`, no `any` types, etc.
