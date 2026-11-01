# Design: feat-tui-ops-residuals

**Change:** feat-tui-ops-residuals
**Inputs:** `proposal.md` (S1–S4, Evidence Inventory E1–E24), ADR-0013 (capas), ADR-0015 (mantenimiento por Hermes), ADR-0016 (alcance TUI), ADR-0010 (TUI de cuentas)
**Constraint:** este documento es doc-first. Fija diseño y decisiones; el código se aplica en un change posterior (T3/T4 de `odd/tasks/feat-tui-ops-residuals.md`). No hay cambios de código en esta fase.

---

## 0. Contrato de capas (ADR-0013)

Este change toca la capa de persistencia (tabla de auditoría), la capa de aplicación/admin (núcleo `internal/admin`), la capa de UI (TUI Bubble Tea + consola) y documentación. **No toca `internal/mcp` ni el RBAC de tools.**

| Capa | Ubicación | Este change agrega (apply) |
|---|---|---|
| Persistencia | `internal/db/schema.go`, `internal/repository/` (adaptador de auditoría) | DDL aditiva de la tabla `accounts_audit`; un adaptador que escribe con prepared statements |
| Dominio (puertos) | `internal/domain/repository/` | Puerto de auditoría (lectura/escritura) si la escritura vive fuera del repo de cuentas |
| Aplicación / núcleo de operador | `internal/admin/` | `PurgeInactiveAccounts`, `ListAuditEvents` (funciones puras de orquestación) |
| UI (transporte) | `internal/tui/`, `cmd/mcp-server/admin_tui.go` | Pantalla `capabilityAuditLog`, capacidad `purge-inactive` + espejo de consola |
| Dominio (day-keys) | `internal/domain/entity/`, `internal/domain/service/` | Tipo/entry point único + call sites normalizados (S2) |
| Docs | `docs/PRD.md:864` | Reescritura de una línea |
| MCP | `internal/mcp/` | **Nada.** Sin tools nuevos, sin cambios de RBAC |

Regla de dependencias: la TUI depende de `internal/admin`; `internal/admin` depende de puertos de dominio; el SQL vive solo en `internal/repository`. La tabla de auditoría se escribe por un adaptador de persistencia, nunca desde la UI. Ciclos prohibidos.

### 0.1 Decisión de gobernanza (ADR-0010 / ADR-0015 / ADR-0016)

Las tres superficies nuevas (vista de audit, `purge-inactive`, day-keys) **NO son tools MCP**. ADR-0010 rechaza explícitamente "LLM hace admin CRUD via MCP tools" (defense-in-depth); ADR-0015 Decision 2 reserva la gestión de cuentas a la TUI; ADR-0016 Decision 2 lista `purge-inactive` como "follow-up (Fase 2+), con confirmación extra. No es MVP". Este change implementa exactamente ese follow-up, sin ampliar la superficie expuesta al LLM.

---

## 1. S1 — Auditoría persistida + vista read-only

### 1.1 Problema de diseño

Hoy el audit de cuentas es `slog` puro (E1–E7). Necesitamos durabilidad consultable **sin** introducir PII ni romper el patrón de emisión "un registro por mutación en la capa que conoce la mutación" (`internal/application/usecase/maintenance_audit.go:1-45`, `internal/repository/accounts.go:48-58`).

### 1.2 Decisión S1-a: tabla de auditoría dedicada, aditiva e independiente de `accounts`

Se agrega una tabla nueva al DDL canónico (`internal/db/schema.go`), compuesta por `initSchema` como los demás builders (E22). El DDL es **aditivo**: `CREATE TABLE IF NOT EXISTS` no altera filas existentes ni exige migración de datos. `schema_version` ya usa `INSERT OR IGNORE` (`internal/db/schema.go:244-248`), así que una versión nueva se agrega como fila, sin `ALTER`.

DDL propuesto (forma, no final — el apply la materializa en `internal/db/schema.go`):

```sql
CREATE TABLE IF NOT EXISTS accounts_audit (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    ts             TEXT NOT NULL,        -- RFC3339Nano UTC, mismo shape que el audit actual
    event          TEXT NOT NULL,        -- ver catálogo §1.3
    actor_ref      TEXT,                 -- referencia hasheada/opaca del actor; nunca el id crudo
    target_id      TEXT,                 -- id de la cuenta afectada (teléfono whitelisted)
    target_role    TEXT,                 -- owner | admin | staff
    action         TEXT NOT NULL,        -- create | update | deactivate | transfer | reject | purge
    phone_masked   TEXT,                 -- últimos 4 dígitos enmascarados; NULL cuando no aplica
    detail         TEXT                  -- JSON mínimo, sin PII; NULL cuando no aporta
);
CREATE INDEX IF NOT EXISTS idx_accounts_audit_ts ON accounts_audit(ts DESC);
CREATE INDEX IF NOT EXISTS idx_accounts_audit_target ON accounts_audit(target_id, ts DESC);
```

**Campos "responsable" (README del diseño):** `ts`, `event`, `action`, `actor_ref`, `target_id`, `target_role`. `actor_ref` replica la decisión ya tomada en el audit de middleware: el caller id crudo es PII (teléfono/email) y el middleware usa una referencia hasheada (E5, `internal/application/usecase/maintenance_audit.go:26-29`: "No caller ID is logged. Raw caller IDs are PII"). La tabla de auditoría **no inventa** un criterio de PII distinto: reutiliza `actor_ref`.

**Sin PII:** `phone_masked` es obligatorio cuando el evento toca un identificador-telefono y **solo** puede contener el resultado de la regla `maskPhone` ya existente (`internal/application/usecase/registration_audit.go:38-46`: conserva los últimos 4 dígitos, o `"***"` si el valor es demasiado corto). **Nunca** se persiste el teléfono completo, nombre de cliente, ni contenido de conversación. `detail` no puede llevar `display_name` (PII indirecta).

### 1.3 Catálogo de eventos a persistir (una fila por mutación)

| Evento | Origen (evidencia) | `action` | `target_*` | `phone_masked` |
|---|---|---|---|---|
| `account_created` | `internal/repository/accounts.go:110` | `create` | id + role | si el id es teléfono |
| `account_updated` | `internal/repository/accounts.go:260` | `update` | id + role | igual regla |
| `account_deactivated` | `internal/repository/accounts.go:301` | `deactivate` | id + role | igual regla |
| `ownership_transferred` | `internal/repository/accounts.go:378` | `transfer` | `target_id` = sucesor; `previous_owner_id` en `detail` | igual regla |
| `second_owner_rejected` | `internal/repository/accounts.go:81` (Warn) | `reject` | id + role | igual regla |
| `inactive_purged` | nuevo, S3 | `purge` | id + role | igual regla |

Se mantiene la regla "exactamente un registro por resultado" (`internal/application/usecase/registration_audit.go:1-12`). La **lectura** (`List`) no genera auditoría (`internal/application/usecase/maintenance_audit.go` audita solo mutaciones) — la vista de audit es read-only y no se auto-audita.

**¿Se mantiene también el `slog`?** Sí. La fila en DB es durable; el `slog` sigue siendo el canal operativo del servicio (`journalctl`). La decisión de mantener ambos evita romper consumidores de logs y es coherente con "defense in depth". El costo es doble escritura; se documenta como trade-off aceptado (ver §7 OQ sobre retención).

### 1.4 Dónde se escribe (capa)

El audit de cuentas se emite hoy en el **repositorio** (`internal/repository/accounts.go`) y en el **usecase** (`maintenance_audit.go`). Para no invertir dependencias, la escritura de la fila de auditoría ocurre **en el mismo punto de emisión** que hoy hace `logger.Info`:

- Opción elegida: un adaptador `internal/repository/accounts_audit.go` que expone `Append(ctx, event)` con prepared statement, inyectado en el punto que ya emite. `*Account` rows y `ctx` ya están disponibles ahí.
- Alternativa rechazada: emitir en `internal/mcp` — viola ADR-0013 (transporte no escribe persistencia) y no capturaría mutaciones del TUI, que no pasan por el mux.

**Regla de prepared statements:** `INSERT INTO accounts_audit (...) VALUES (?, ?, ?)`; nunca concatenación (AGENTS.md). La vista de audit lee con `SELECT ... ORDER BY ts DESC LIMIT ?` (`?` = límite validado).

### 1.5 Vista read-only en la TUI (Bubble Tea + consola)

- Nueva entrada en `menuItems()` (`internal/tui/view.go:82-91`) y nuevo `capabilityAuditLog` en el enum (`internal/tui/view.go:62-71`), con su caso en el dispatch (`internal/tui/model.go:605-628`). La pantalla usa el patrón de tabla existente (`internal/tui/table.go`, `internal/tui/view.go` `viewTable`).
- **Read-only por construcción:** no hay acción de escritura en la pantalla; el operador solo pagina/regresa. Sin promoción de evento a mutación.
- **Paridad de consola:** una entrada nueva en `adminMenuOptions` (`cmd/mcp-server/admin_tui.go:411-433`) que renderiza la misma tabla. El núcleo (`internal/admin.ListAuditEvents`) es compartido, igual que las 7 capacidades actuales (E9).
- **Superficie tipada, no `any`:** la proyección es un struct concreto (`admin.AuditEventView`) con campos explícitos; la UI no ve `map[string]any` (AGENTS.md: prohibido `any`).

### 1.6 Inmutabilidad del audit

- La tabla es **append-only a nivel de código**: no existe ningún método de `UPDATE`/`DELETE` sobre `accounts_audit` en el adaptador. No se agrega trigger SQLite en esta versión (un trigger `BEFORE UPDATE/DELETE ... RAISE(ABORT)` es una defensa opcional a evaluar en el apply; ver OQ-2).
- El `sqlite` no impide `DELETE` directo por CLI; la inmutabilidad es contractual + revisada por tests (no hay API que borre). Se documenta el límite honesto: un operador con shell puede alterar la DB — el mismo límite que hoy tiene con `slog` y el journal.

---

## 2. S2 — Normalización de day-keys

### 2.1 Estado actual (E12–E17)

| Convención | Codificación | Sitios |
|---|---|---|
| `time.Weekday` / `schedules.day_of_week` | `0..6`, domingo=0 | `internal/domain/service/slot_context.go:89`, `internal/domain/service/availability.go:108`, `internal/domain/service/booking_time_validator.go:116`, DDL `internal/db/schema.go:60-70` |
| `business_hours` JSON | `"1".."7"`, lunes=1 | `entity.ProfileDayKey` (`internal/domain/entity/business_profile.go:84-92`), `GetOpenClose` (`:100-119`), parseo tolerante (`:122-135`) |

`entity.ProfileDayKey` es el traductor **documentado** (`:108-109`: "never from a raw `int(time.Weekday())`") pero es una función sin tipo: nada impide pasar un `int` de la convención equivocada a `GetOpenClose(dayOfWeek int)`.

### 2.2 Decisión S2: tipo nominal `entity.DayKey` (recomendada)

Se propone introducir un **tipo nominal** que haga imposible la mezcla en compilación:

```go
// internal/domain/entity/daykey.go (nuevo)
type DayKey int // 1..7, lunes=1 (business_hours)

// ScheduleWeekday es el encoding 0..6 (domingo=0) de schedules.day_of_week.
type ScheduleWeekday int

func ProfileDayKey(w time.Weekday) DayKey { /* lunes=1 .. domingo=7 */ }
func ScheduleDay(w time.Weekday) ScheduleWeekday { /* domingo=0 .. sábado=6 */ }
```

- `GetOpenClose(day DayKey)` y `FindByProfessionalAndDay(... day ScheduleWeekday)` cambian de `int` a los tipos nominales.
- Los sitios crudos `int(...Weekday())` se reemplazan por `entity.ScheduleDay(...)` (mismos valores, ahora explícitos).
- **Blast radius:** 3 call sites de servicio (E15), 1 de traducción ya explícita (E16), la firma de `GetOpenClose` y del puerto `SchedulesRepo`, y sus mocks/tests. La DDL no cambia.
- **Estrategia de migración/compat:** **cero migración de datos.** Los valores persistidos son idénticos (`schedules` sigue `0..6`; `business_hours` sigue `"1".."7"`). El tipo es solo de frontera en Go. El parseo tolerante de claves legadas (`"01"`, `"007"`, E14) se conserva tal cual: es la compat de lectura ya decidida en su día.
- **Alternativa evaluada y descartada:** migrar `schedules.day_of_week` a `"1".."7"` para unificar en la DB. Requiere reescribir filas + `CHECK`, multiplica el riesgo (una migración destructiva sobre datos vivos) sin ganancia funcional, y contradice "no migración destructiva" (proposal Non-goals). **Rechazada.**
- **Entry point único:** ambos constructores viven en `internal/domain/entity` y son las únicas funciones que traducen `time.Weekday`; los call sites de servicio no vuelven a llamar `int(Weekday())`.

### 2.3 Pin por test

- Test de la traducción: `time.Sunday → DayKey(7)` y `→ ScheduleWeekday(0)`; `time.Monday → 1` y `1`; tabla completa `0..6`.
- Test de frontera: un `GetOpenClose` con clave fuera de `1..7`/día cerrado devuelve `ok=false, err=nil` (día legítimamente cerrado), y una clave `"1".."7"` corrupta devuelve `err != nil` (fail-closed, `:100-119`).
- Guard de regresión: test que recorre los call sites de servicio y verifica que el valor entregado a `FindByProfessionalAndDay` es `ScheduleWeekday` y el entregado a `GetOpenClose` es `DayKey` — garantizado por el compilador una vez que los tipos son nominales; el test cubre los valores.

---

## 3. S3 — `purge-inactive`

### 3.1 Experiencia de confirmación (doble confirmación, ambos flujos)

**TTY (Bubble Tea):**
1. Listar las cuentas `is_active=0` con su rol y etiqueta (`admin.AccountLabel`).
2. Selección explícita de la cuenta (o "todas las inactivas", según OQ-1).
3. **Paso extra de confirmación** (por encima del `[S]í/No` estándar): el operador debe escribir una confirmación fuerte (por ejemplo, el último sufijo del teléfono o la palabra `PURGAR`), no solo `s`. Esto satisface "confirmación extra" (`docs/PRD.md:904`) y la convención de guardas destructivas del repo.
4. Éxito ⇒ mensaje en español con el conteo de filas borradas; volver al menú.

**Consola (sin TTY):** espeja 1:1 los pasos, reusando `promptConfirm` (`cmd/mcp-server/admin_tui.go:798-820`) **más** un segundo `promptValidated` para el token de confirmación fuerte. La paridad es el contrato vigente (E9, E10).

**Reglas duras:**
- Nunca se borra un owner activo (pre-check en el núcleo, espejo de `admin.DeactivateAccount` en `internal/admin/accounts.go:135-186`).
- Solo filas `is_active=0`. Un cambio de estado entre el listado y la escritura se re-lee antes de borrar (el snapshot es display-only, mismo patrón documentado para Deactivate).
- Idempotente: si ya no hay inactivas, es un outcome, no un error.
- Ninguna cuenta activa, ni un profesional, ni un servicio, ni una reserva se toca.

### 3.2 Shape del sub-comando (evaluación)

| Opción | Alineación con ADR | Trade-off |
|---|---|---|
| **(a) Capacidad del menú `mcp-server admin tui`** (entrada nueva en `menuItems()` + espejo consola) | ADR-0016 Decision 1: las cuentas viven en la TUI; ADR-0010 rechaza CLI bash y MCP | Sin superficie nueva de sub-comandos; `purge-inactive` deja de ser literalmente "sub-comando" del PRD (`docs/PRD.md:904`) |
| **(b) Sub-comando dedicado `mcp-server admin purge-inactive`** que comparte el núcleo `internal/admin` | Honra el literal del PRD; el operador puede scriptearlo | Nueva superficie de CLI a versionar; duplica el patrón de entrada (`admin tui`) y exige su propia paridad TTY/consola |

**Recomendación (a)** como camino principal, **con (b) como opción del owner** (OQ-1). Ambas comparten `admin.PurgeInactiveAccounts`, así que no hay re-trabajo: el núcleo es el mismo y la diferencia es solo el envoltorio de presentación. La decisión requiere owner (no es inferible del PRD).

### 3.3 FK-safety (inventario verificado)

- **Ninguna tabla referencia `accounts`:** `grep "REFERENCES accounts"` sobre `internal/` devuelve vacío (E20). Borrar filas de `accounts` no puede violar un `ON DELETE RESTRICT` entrante — no existe.
- **`accounts.professional_id` no tiene FK** (`internal/db/schema.go:123-129`, sin `REFERENCES`), el mismo hecho que ADR-0016 Context fact 3 documenta ("sin FK `professional_id`"). Por lo tanto borrar una cuenta `staff` **no** cascada ni rompe `professionals`.
- **FKs RESTRICT de `bookings`** (`internal/db/schema.go:96-98`) apuntan a `professionals`/`services`, no a `accounts`; `purge-inactive` no las toca.
- **Consecuencia:** el borrado es FK-safe **por construcción** para purgar solo `accounts`. El riesgo real es la fila huérfana de `professionals` que quedaba vinculada por `professional_id` (sin FK). **Alcance de purge:** ¿solo `accounts`, o también profesionales huérfanos? → **OQ-3** (decisión de owner). El default seguro es **solo `accounts`** (el PRD habla de "cuentas desactivadas", E18).
- Si en el apply se agrega la FK `accounts.professional_id → professionals(id)`, se debe elegir `ON DELETE SET NULL`/`RESTRICT` con cuidado; **no es parte de este change** (sería cambio de schema con migración) y queda como no-goal.

### 3.4 Núcleo compartido

`internal/admin.PurgeInactiveAccounts(ctx, accounts AccountsPurge, opts PurgeOptions) (PurgeOutcome, error)`:

- puerto nuevo `AccountsPurge` (solo el mínimo: listar inactivas + borrar por id) — los puertos anchos actuales (`AccountsAdmin`, `internal/admin/accounts.go:19-35`) declaran explícitamente que el hard delete está ausente; este change **agrega** la capacidad como puerto separado para no aflojar el puerto existente.
- `TUIContext` (caller fabricado de owner, `internal/admin/seed.go:25-40`) igual que el resto del núcleo.
- errores semánticos en español, nunca dumps del driver.

---

## 4. S4 — Corrección de la línea stale del PRD

**Única superficie:** `docs/PRD.md:864`. La línea actual:

> `8. ⏳ **Fase 2+ — TUI menú operacional con seed del owner** — pendiente (era opcional en Fase 2; sigue planificado, ver §3.8.8 y RF9).`

**Reescritura propuesta (una línea, estilo del archivo):**

> `8. ✅ **Fase 2+ — TUI menú operacional con seed del owner — MVP ENTREGADO (2026-09-16, ADR-0016; ver §3.8.8, RF9 y §7 Fase N): seed del primer owner, Add Staff, Deactivate (soft delete), listados, Transfer Ownership y Add Yourself as Client.** Residuales operativos trackeados en el change `feat-tui-ops-residuals`: vista de audit log persistido, normalización de day-keys `"1".."7"` vs `0..6` y sub-comando `purge-inactive` con confirmación extra.`

Referencias cruzadas usadas: `docs/PRD.md:1343` (MVP ENTREGADO), `docs/architecture/0016-admin-tui-scope.md` (ADR-0016), `docs/PRD.md:904` y `:1364` (purge-inactive). **No se toca ninguna otra línea** de `docs/PRD.md`.

> Nota de inconsistencia detectada (no corregible en esta superficie): `docs/PRD.md:1343` atribuye el follow-up de day-keys a **ADR-0015**, pero el texto normativo de la asimetría de day-keys está en **ADR-0016 Decision 3.4**, no en ADR-0015 (verificado: ADR-0015 no menciona day-keys). Se registra como OQ-4 para que el owner decida corregir el cross-ref en un change futuro.

---

## 5. Matriz de test (apply)

| Capa | Foco | Fixtures |
|---|---|---|
| Persistencia (repo/DLL) | DDL aditiva idempotente (`initSchema` dos veces = mismo estado); `INSERT` con prepared statement; sin PII en `phone_masked`; lectura `ORDER BY ts DESC LIMIT n` | `t.TempDir()` + `db.NewDatabase` sobre archivo temporal (patrón de `internal/db/database_test.go`) |
| Núcleo `internal/admin` | `PurgeInactiveAccounts`: refusa owner activo, borra solo inactivas, idempotente, re-lee antes de escribir; `ListAuditEvents`: proyección read-only, sin `any`; errores semánticos en español | mocks de puerto (patrón `internal/admin/*_test.go`, sin DB) |
| UI Bubble Tea (MVU) | `capabilityAuditLog` alcanzable y read-only; `purge-inactive` exige confirmación fuerte (rechaza `s`; acepta el token); mensajes en español; snapshot de la tabla | `teatest`/golden (patrón `go-testing`, `internal/tui/app_test.go`); escritura sobre `t.TempDir()` |
| Consola (sin TTY) | Paridad 1:1 de la vista y del purge; la confirmación fuerte re-pregunta ante input inválido; nunca borra sin confirmar | `cmd/mcp-server/admin_tui_test.go` + `internal/tui/skill_console_test.go:45` (patrón de paridad) |
| Day-keys | Traducción completa `0..6`; frontera `DayKey` ↔ `ScheduleWeekday`; fail-closed en clave corrupta | unit tests `internal/domain/entity`, `internal/domain/service` |
| Integración | Purge sobre una DB real borra solo inactivas y deja activas/booking/professional intactos; la vista de audit refleja una mutación real | `t.TempDir()` + repo real |
| Race | Todos los paquetes tocados | `go test -race ./...` |

Los comandos de pipeline corren en los gates de fase (no por unidad): `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...`, `go build -o /dev/null ./...`, `go test -v -race ./...` (AGENTS.md). `openspec/config.yaml` fija `apply.tdd: true` ⇒ RED→GREEN→TRIANGULATE→REFACTOR por unidad.

---

## 6. Security review

| Superficie | Control |
|---|---|
| **Exposición externa** | Ninguna. La TUI y el (eventual) sub-comando corren como sub-comando local del binario en la VPS; no abren puerto. El MCP sigue en loopback `127.0.0.1:3000`. `purge-inactive` **no** es tool MCP (ADR-0010/0015), así que un LLM comprometido no lo alcanza. |
| **Escalada de privilegios** | El núcleo usa el caller fabricado de owner (`TUIContext`), igual que el resto de `internal/admin`; el gatekeeper es el admin del OS por SSH (ADR-0010 "Enforcement"). No se agrega ni se relaja RBAC de tools. |
| **Borrado destructivo** | Doble confirmación (paso extra/TTY + token fuerte/consola); nunca owner activo; solo `is_active=0`; FK-safe por construcción (§3.3). El snapshot es display-only y el estado se re-lee antes de escribir. |
| **Audit inmutable** | Append-only a nivel de API: el adaptador no expone `UPDATE`/`DELETE` sobre `accounts_audit`. Límite honesto: un operador con shell puede alterar la DB (mismo límite que `slog`/journal). Trigger de inmutabilidad opcional en OQ-2. |
| **PII en audit** | `phone_masked` reutiliza `maskPhone` (`internal/application/usecase/registration_audit.go:38-46`); `actor_ref` es opaco/hasheado (E5); `detail` no puede contener `display_name` ni datos de cliente. No se persiste teléfono completo. |
| **SQL injection** | Prepared statements con `?` en `INSERT` y `SELECT`; el límite de la lectura se valida como entero antes del bind. Sin concatenación (AGENTS.md). |
| **Input del operador** | El token de confirmación se valida por igualdad contra un valor derivado del objetivo; input inválido re-pregunta (nunca se interpreta un typo como consentimiento, patrón `promptConfirm`). |
| **Fuga de errores** | El núcleo devuelve `*domain.SemanticError` en español; los errores de driver se envuelven y no se imprimen al operador. |
| **Dependencias** | Sin dependencias nuevas. |

---

## 7. Riesgos, rollback y preguntas abiertas

### 7.1 Riesgos

| Riesgo | Mitigación |
|---|---|
| Doble escritura (slog + DB) puede desincronizarse | La fila de DB es la fuente consultable; el `slog` es operativo. Test de integración que verifica la fila por mutación. Si la escritura de audit falla, la mutación **no** se revierte (mismo criterio que el `slog` actual); se documenta como comportamiento esperado. |
| Borrado accidental de una cuenta aún necesaria | Doble confirmación; solo inactivas; re-lectura pre-write; nunca owner activo. |
| Cambio de tipos nominales (S2) rompe firmas | Los valores persistidos no cambian (cero migración); el compilador marca todos los call sites; el blast radius está inventariado (§2.2). |
| DDL nueva en DBs existentes | `CREATE TABLE IF NOT EXISTS` es idempotente y aditivo; `schema_version` agrega una fila sin `ALTER` (E22). |
| La vista de audit expone más de lo debido | Proyección tipada mínima (`AuditEventView`), sin PII, read-only. |

### 7.2 Rollback (git revert)

- **Estrategia general:** cada unidad de apply es un commit acotado y revertible con `git revert <sha>`.
- **S1:** revertir el commit de la tabla deja la DB con `accounts_audit` presente pero sin escritores; como la tabla es nueva y aditiva, no rompe el schema existente ni los repos. La vista TUI se revierte con su commit. Sin migración que deshacer.
- **S2:** revertir los tipos nominales vuelve a `int`; como no hay cambio de datos, no hay rollback de datos. Un test de regresión fallaría primero si la convención se rompe.
- **S3:** revertir el commit del núcleo + presentación elimina la capacidad; ninguna fila de negocio fue alterada por el diseño (solo se borran cuentas inactivas, y eso ya ocurrió si el operador lo ejecutó). El rollback de código no reintroduce filas borradas — por eso el doble gate es la protección real.
- **S4:** revertir el commit de una línea restaura la línea stale; es el artefacto más barato de revertir.
- **Orden de rollout:** S4 (docs, una línea) → S2 (refactor de tipos, sin datos) → S1 (schema aditivo + vista) → S3 (superficie destructiva, último y con máxima revisión). Cada paso es independiente.

### 7.3 Preguntas abiertas (owner)

| # | Pregunta | Por qué no la puede resolver el worker |
|---|---|---|
| OQ-1 | ¿`purge-inactive` se implementa como capacidad del menú `admin tui` (recomendado) o como sub-comando dedicado `mcp-server admin purge-inactive`? | El PRD dice "sub-comando"; ADR-0016/0010 gobiernan la TUI. Es una decisión de producto/gobernanza. |
| OQ-2 | ¿Se agrega un trigger SQLite de inmutabilidad sobre `accounts_audit` (rechazar `UPDATE`/`DELETE`) o basta el append-only de API? | Trade-off entre defensa extra y superficie de DDL; el owner decide cuánta defensa quiere. |
| OQ-3 | ¿El alcance de purge son **solo cuentas** o también profesionales huérfanos vinculados por `professional_id` (sin FK)? | El PRD habla de "cuentas desactivadas"; ampliar a profesionales es cambio de alcance. |
| OQ-4 | ¿Se corrige en un change futuro el cross-ref de `docs/PRD.md:1343` que atribuye day-keys a ADR-0015 en lugar de ADR-0016 Decision 3.4? | Está fuera de la superficie permitida de este change (solo `:864`). |
| OQ-5 | ¿Ventana de retención del audit persistido (p. ej. purgar filas > N días) o retención indefinida? | Decisión de operación/producto; H2/~ de volver a evaluar. |
| OQ-6 | ¿La vista de audit pagina o limita por N eventos? ¿Cuál es el default visible? | UX de operador; el default propuesto es un límite validado (p. ej. 200). |
