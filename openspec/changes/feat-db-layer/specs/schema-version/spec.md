# Spec: Schema Version

> Reference: `docs/PRD.md §3.7` (Data Model), `docs/architecture/0006-data-model-and-reservations.md` (ADR-0006), `openspec/changes/feat-db-layer/proposal.md` (sección "Schema version + estrategia de migración")
> Change: feat-db-layer
> Status: NEW (no prior spec existed)
> PR: 1 (foundation)

## Purpose

Tracking del estado del esquema de la base de datos. Permite que fases futuras (Fase 2+) hagan migraciones incrementales sin perder los datos del usuario, y permite a `initSchema` ejecutarse de forma idempotente en arranques subsecuentes.

## Requirements

### Requirement: Schema Version History Table

El sistema MUST crear la tabla `schema_version` durante `initSchema`, con el siguiente esquema. La tabla actúa como historial de versiones: `version=1` se inserta en el primer arranque, y futuras migraciones (Fase 2+) insertan `version=2`, `version=3`, etc. (sin UPDATE, solo INSERT de nuevas filas).

```sql
CREATE TABLE schema_version (
    version         INTEGER PRIMARY KEY,
    applied_at      TEXT NOT NULL DEFAULT (datetime('now')),
    description     TEXT
);
```

#### Scenario: First-Run Creation

- GIVEN una base de datos recién creada (sin la tabla `schema_version`)
- WHEN `initSchema(ctx, db)` se ejecuta
- THEN la tabla `schema_version` existe
- AND la tabla `schema_version` está vacía hasta que se inserte la fila de versión inicial

#### Scenario: Subsequent Run Is Idempotent

- GIVEN una base de datos donde `schema_version` ya contiene la fila `(version=1, ...)`
- WHEN `initSchema(ctx, db)` se ejecuta por segunda vez
- THEN `initSchema` no falla (idempotente)
- AND `initSchema` no re-crea las otras tablas
- AND la fila de versión 1 sigue existiendo sin duplicarse

### Requirement: Initial Version 1 Inserted on First Run

En el primer arranque (cuando la tabla `schema_version` no existe), `initSchema` MUST insertar una fila con `version=1` y la descripción del esquema inicial.

#### Scenario: Version 1 Row Inserted

- GIVEN una base de datos recién creada
- WHEN `initSchema(ctx, db)` completa exitosamente
- THEN existe exactamente UNA fila en `schema_version` con `version=1`
- AND `applied_at` es la fecha/hora actual (formato ISO 8601 UTC)
- AND `description` es exactamente `"initial schema: 10 domain tables per PRD §3.7 + schema_version + 6 FTS sync triggers + 3 secondary indexes"`

#### Scenario: Applied At Is Automatic

- GIVEN el INSERT de la versión inicial
- WHEN se ejecuta
- THEN el sistema NO necesita setear `applied_at` explícitamente
- AND el valor de `applied_at` es el `datetime('now')` de SQLite en formato ISO 8601

### Requirement: Schema Initialization Idempotency

`initSchema` MUST ser idempotente: ejecutarlo N veces con el mismo `*sql.DB` produce el mismo estado final que ejecutarlo 1 vez.

#### Scenario: Multiple InitSchema Calls

- GIVEN una base de datos recién creada
- WHEN `initSchema(ctx, db)` se llama 3 veces seguidas
- THEN la operación es exitosa las 3 veces
- AND la base de datos tiene el mismo estado que después de 1 llamada
- AND la tabla `schema_version` tiene exactamente 1 fila (no se duplicó)

#### Scenario: Idempotent Retry After Partial Failure

- GIVEN un `initSchema` que falla a mitad de camino (ej. error de SQLite en el 5to `CREATE TABLE`)
- WHEN el operador re-ejecuta `initSchema` en un segundo intento
- THEN los `CREATE TABLE` que ya habían exitoso se preservan (gracias a `IF NOT EXISTS`)
- AND los faltantes se crean
- AND `schema_version` se crea al final del batch exitoso
- AND la base de datos queda eventualmente consistente: todas las tablas presentes

### Requirement: Version Tracking Reserved for Future Migrations

La tabla `schema_version` está diseñada para que fases futuras (Fase 2+) agreguen migraciones incrementales (v2, v3, etc.). Esta especificación documenta la intención y la estructura; la implementación de las migraciones en sí NO está en scope de Fase 1.

#### Scenario: Future Migration Mechanism (Documented, Not Implemented)

- GIVEN un esquema en versión 1 (Fase 1)
- WHEN Fase 2 necesite agregar una columna o tabla
- THEN Fase 2 leerá `MAX(version) FROM schema_version` (o equivalente)
- AND ejecutará las migraciones incrementales hasta la versión target
- AND actualizará la tabla con la nueva versión (insertando una nueva fila o actualizando la existente según el diseño del runner)
- AND el resultado es un esquema actualizado sin pérdida de datos del usuario

#### Scenario: Schema Version Scope Boundary

- GIVEN esta spec es la única fuente de verdad para `schema_version` en Fase 1
- WHEN algún sub-agente o developer considere agregar un campo a la tabla
- THEN la modificación se discute en una ADR nueva (no se cambia este spec sin proceso de decisión)
- AND el cambio se documenta como una migration v2 explícita cuando se introduzca

## Notes

- **Fase 1 NO incluye un runner de migraciones**: la tabla se crea y la fila v1 se inserta, pero `initSchema` no compara versiones ni ejecuta migraciones. El primer uso real del versionado será en Fase 2+ cuando se agregue la primera tabla o columna.
- **Costo en Fase 1**: ~5 LOC (1 `CREATE TABLE` + 1 `INSERT` en `initSchema`). Sin riesgo, sin dependencias externas, sin impacto en performance.
- **Beneficio futuro (estructural)**: cuando Fase 2-5 necesiten cambiar el esquema, el patrón versionado ya está en la base; no hay que agregar `schema_version` en medio de una migración. Coherente con la filosofía "preparar el terreno" del proyecto (ver ADR-0001, ADR-0002, ADR-0006).
- **Destructive replace en Fase 1**: mientras la base esté en estado pre-release (sin datos de usuario), `initSchema` puede hacer `DROP TABLE` + recreate si el `schema_version` no se encuentra. Una vez que el release público exista y haya datos, esta lógica debe cambiar a migración incremental (decisión Fase 2+).
- **Relación con la propuesta**: la sección "Schema version + estrategia de migración" del `proposal.md` (commit `7d0dc77`) describe la decisión de incluirla en Fase 1; este spec formaliza los requirements testeables.
- **Relación con los PRs**: este spec corresponde al PR 1 del chain (3 PRs encadenados, budget 600). El archivo `internal/db/database.go` contiene la creación de la tabla + INSERT; el test en `internal/db/database_test.go` cubre los scenarios de esta spec.
- **SQL canónico**: el bloque exacto de `CREATE TABLE schema_version ...` está en el `proposal.md` (sección "Schema version + estrategia de migración"). Esta spec NO incluye bloques SQL ejecutables; los scenarios describen el comportamiento esperado, no la implementación.
