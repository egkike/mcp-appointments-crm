# Propuesta: feat/db-layer — Extensión de esquema + capa de repositorio

## Intent

La base actual (`internal/db/database.go`, 126 líneas) no cumple el modelo de datos
canónico del PRD §3.7: hay **4 tablas** (faltan 6), los **FTS5 no tienen triggers de
sincronización** (`clients_fts`/`services_fts` devuelven cero resultados siempre), y
`appointments` **no tiene `professional_id`**, lo que hace imposible la cadena
`check_availability` de 5 pasos (PRD §3.7.13). Fase 1 del roadmap.

## Scope

### In Scope
- Reescribir `internal/db/database.go` al esquema completo de **10 tablas** (PRD §3.7).
- Renombrar `appointments`→`bookings`, `duration_mins`→`duration_minutes`,
  `start_time`/`end_time`→`start_datetime`/`end_datetime` (ADR-0004).
- Mover `messenger_*` de `clients` a `business_profile`; IDs `INTEGER`→`TEXT` UUID v4
  (salvo `business_profile.id='singleton'`); timestamps `DATETIME`→`TEXT` ISO 8601.
- Añadir **6 triggers FTS5** (`*_ai`, `*_ad`, `*_au` por tabla) — ADR-0006 Decisión 4.
- Desnormalizar `bookings.end_datetime` y 3 índices secundarios.
- Crear `internal/model/` (8 archivos) y `internal/repository/` (9 archivos) con
  prepared-statement CRUD + `CheckAvailability` (5 pasos).
- Promover `go-sqlmock` de `// indirect` a directa. TDD estricto, ≥80% cobertura repos.

### Out of Scope
- `internal/config/dotenv.go` y parser `.env` (ADR-0005/0007) → **Fase 2**.
- Runner de migraciones con diffs de esquema (sólo agregamos la **tabla** `schema_version` para tracking; las migraciones incrementales vienen en Fase 2+).
- Handlers MCP, TUI, `install.sh`, CI workflows (Fases 2-5).

## Capabilities

> `openspec/specs/` vacío: todas son **NEW**. Cada una → `openspec/specs/<name>/spec.md`.

### New Capabilities
- `business-profile`: perfil singleton, `business_hours` JSON, `messenger_*`, métodos de pago.
- `business-hours-exception`: excepciones fechadas con `UNIQUE(exception_date)`.
- `professionals`: profesionales activos con horario laboral.
- `schedules`: horario semanal por profesional, `UNIQUE(professional_id, day_of_week)`.
- `services`: catálogo con `duration_minutes`, `description`, `is_active`, FTS5.
- `clients`: clientes con `preferences`, sin `messenger_*`, FTS5.
- `bookings`: reservas con FK `professional_id`, `end_datetime` denormalizado, estado FSM,
  y la cadena `check_availability` de 5 pasos (§3.7.13).
- `pending-alerts`: alertas programadas con índice `(scheduled_datetime, status)`.
- `data-access`: estructura y convenciones de la capa `internal/repository/` (interfaces,
  prepared statements, sentinels, ≥80% cobertura).

### Modified Capabilities
- Ninguna (no existen specs previas).

## Approach

Reemplazo destructivo del esquema (seguro en estado pre-release: cero clientes). TDD
estricto: primero `*_test.go` con `go-sqlmock` (CRUD) y SQLite en memoria real (triggers
FTS — `go-sqlmock` no simula efectos de trigger). Repositorios vía interfaces pequeñas con
`context.Context`, `fmt.Errorf("...: %w", err)`, sentinels en `errors.go`. Las renombraciones
siguen ADR-0004. Repartido en **3 PRs encadenados** (budget elevado a 600 — obs 456).

### Cadena de 3 PRs

| PR | Alcance | LOC | Cambio |
|----|---------|-----|--------|
| **1** | `database.go` (10 tablas + 6 triggers + 3 índices + tabla `schema_version`) + 8 modelos + `errors.go` + `internal/db/database_test.go` (integración FTS en memoria, ~100 LOC) | ~420 | Fundación |
| **2** | Repos simples: `business_profile` (lazy-init), `services`, `clients`, `business_hours_exception` + `*_test.go` (`go-sqlmock`) | ~500 | CRUD |
| **3** | Repos complejos: `bookings` con `CheckAvailability` (5 pasos), `professionals`, `schedules`, `pending_alerts` + `*_test.go` | ~600 | Lógica |
| Total | ~1500-1900 LOC | — | 3 PRs ≤600 |

Orden estricto: PR 2 y PR 3 dependen del esquema de PR 1.

## Consecuencias

**Positivas**
- FTS5 funciona (triggers insertan/borran/actualizan automáticamente).
- `check_availability` viable (FK `professional_id` + horarios + excepciones + solapamientos).
- `end_datetime` denormalizado simplifica las consultas de solapamiento.
- Capa de repositorio unit-testable; ≥80% cobertura con `go-sqlmock`.
- 2 PRs de repos son revertibles de forma independiente (capa aislada).

**Negativas**
- Cambio de esquema destructivo (aceptable pre-release; sin datos de usuario).
- `go-sqlmock` no prueba triggers → se requiere SQLite en memoria real para ese subconjunto
  (única desviación del patrón "100% go-sqlmock").
- `repository/bookings.go` es el archivo individual más grande (≈200 LOC: 5 pasos + CRUD).

**Alternativas rechazadas**
- (a) Mantener el estado actual y solo documentar los huecos → **No**: FTS roto es bug #1.
- (b) Implementar la sincronización FTS en Go en vez de triggers → **Rechazado** por ADR-0006
  Decisión 4 (los triggers son la solución).
- (c) Introducir un runner de migraciones con `schema_version` en Fase 1 → **Rechazado**:
  sobre-ingeniería para una base sin datos.

## Affected Areas

| Área | Impacto | Descripción |
|------|---------|-------------|
| `internal/db/database.go` | Modificado | Reescritura: 4→10 tablas, +6 triggers, +3 índices |
| `internal/db/database_test.go` | Nuevo | Integración FTS con SQLite en memoria |
| `internal/model/*.go` (8) | Nuevo | Modelos por tabla, singular |
| `internal/repository/*.go` (9) | Nuevo | CRUD + `CheckAvailability` + `errors.go` |
| `go.mod` | Modificado | `go-sqlmock` y `google/uuid` de indirect a directas |

## Risks

| Riesgo | Prob. | Mitigación |
|--------|-------|------------|
| Trigger FTS roto por incompatibilidad UUID/rowid | Media | FTS usa `content_rowid='rowid'`; integración en memoria cubre el caso |
| Cobertura <80% por complejidad de `bookings` | Media | TDD tabla-driven; cubrir cada paso de los 5 aisladamente |
| Renombrado deja referencias colgando | Baja | `go build ./...` + `go vet` en pre-flight de cada PR |
| Student/sync de `business_profile` singleton | Baja | Lazy-init en `GetBusinessProfile` (`INSERT OR IGNORE` + `SELECT`) |

## Rollback Plan

- **PR 1 (esquema)**: destructivo en primer arranque. Como no hay datos de usuario, revertir
  es `git revert <pr1-merge-commit>`. **Bloquea PR 2 y PR 3** hasta que aterrice un fix,
  porque dependen del nuevo esquema.
- **PR 2 (repos simples)**: `git revert <merge>` → elimina 4 archivos de repos; el esquema y
  los modelos siguen funcionando. Independiente.
- **PR 3 (repos complejos)**: `git revert <merge>` → elimina `bookings`/`professionals`/
  `schedules`/`pending_alerts`. Independiente de PR 2.
- Cada PR se revierte de forma aislada salvo PR 1, que gatea a los demás.

## Dependencies

- `modernc.org/sqlite` v1.53.0 (ya presente) — driver SQLite en memoria para tests de triggers.
- `github.com/DATA-DOG/go-sqlmock` v1.5.2 (presente, indirect → direct).
- `github.com/google/uuid` v1.6.0 (presente, indirect → direct).
- Sin nuevas dependencias externas (cumple ADR-0005).

## Success Criteria

- [ ] `internal/db/database.go` crea las 10 tablas de PRD §3.7 con los 6 triggers FTS5.
- [ ] `internal/db/database_test.go` demuestra que insertar/borrar/actualizar en `clients` y
      `services` mantiene `clients_fts`/`services_fts` sincronizados.
- [ ] `CheckAvailability` implementa los 5 pasos de PRD §3.7.13 con mensajes semánticos.
- [ ] `go test -v -race ./...` verde; cobertura ≥80% en `internal/repository/`.
- [ ] `go build -o /dev/null ./...`, `go vet`, `golangci-lint` limpios.
- [ ] Las renombraciones de ADR-0004 aplicadas y sin referencias colgantes.

## Referencias

- **PRD**: `docs/PRD.md` §3.7 (esquema 10 tablas), §3.7.9 (6 triggers FTS), §3.7.13 (cadena 5 pasos).
- **ADRs**: `docs/architecture/0004-naming-conventions.md`, `0005-optional-external-tools.md`,
  `0006-data-model-and-reservations.md` (5 decisiones: `business_hours` JSON, tabla
  `business_hours_exception`, `end_datetime` denormalizado, FTS vía triggers, validación 5 pasos),
  `0007-server-config.md`.
- **Commits**: `943f697` (budget → 600), `99eca18` (tabla `business_hours_exception`).
- **Engram**: obs 452 (contexto), 453 (testing-capabilities), 456 (preflight), 464 (estado proyecto).

## Schema version + estrategia de migración

Para tracking del estado del esquema, Fase 1 introduce la tabla `schema_version`
(1 sola fila, columna `version` como `INTEGER PRIMARY KEY`). No incluye un runner
de migraciones incrementales (eso es Fase 2+); sólo permite saber en qué versión
está una base dada.

```sql
CREATE TABLE schema_version (
    version         INTEGER PRIMARY KEY,
    applied_at      TEXT NOT NULL DEFAULT (datetime('now')),
    description     TEXT
);

-- En el primer arranque, initSchema inserta la versión inicial:
INSERT INTO schema_version (version, description) VALUES
    (1, 'initial 10-table schema per PRD §3.7 + 6 FTS sync triggers + 3 secondary indexes');
```

`initSchema` usa la presencia de la fila `(version=1)` como señal de "ya está
inicializado". Si no existe, ejecuta el `CREATE TABLE IF NOT EXISTS` batch + el INSERT.
Si ya existe, no-op (idempotente). **Destructivo en primer arranque** (foundation-only
state, sin datos de usuario); los arranques subsecuentes son no-op.

## Decisiones confirmadas en esta propuesta

1. **Nomenclatura de triggers FTS** (confirmado 2026-06-25 por Kike): se usa
   `clients_fts_ai`/`clients_fts_ad`/`clients_fts_au` y el análogo para `services_fts`.
   Naming con infix `_fts_` para consistencia con el nombre de la tabla. PRD §3.7.10
   actualizado para reflejarlo (ver commit de este ajuste).
2. **Tabla `schema_version`** (confirmado 2026-06-25 por Kike): incluida en Fase 1 PR 1
   (trackeo de versión, sin runner de migraciones). Ver sección "Schema version + estrategia
   de migración" arriba para el SQL y la semántica.
3. **Seeder first-boot de `business_profile`** (confirmado 2026-06-25 por Kike): lazy-init
   en el repositorio. `GetBusinessProfile(ctx)` hace `INSERT OR IGNORE INTO business_profile
   (id, name) VALUES ('singleton', '')` y luego `SELECT * FROM business_profile WHERE
   id = 'singleton'`. Es idempotente (múltiples llamadas son safe) y self-healing. Caveat:
   sólo se dispara cuando se llama a través del repo — un `SELECT` directo vía SQL
   devolvería 0 filas en un fresh install. La convención "todo acceso vía repo" se enforce
   por code review.