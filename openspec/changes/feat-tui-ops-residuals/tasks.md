# Tasks: feat-tui-ops-residuals

> **Change:** feat-tui-ops-residuals
> **Status:** tasks drafted (spec phase) — doc-first; este change cierra la fase de especificación. El apply de código es un change posterior (T3/T4 de `odd/tasks/feat-tui-ops-residuals.md`).
> **Inputs:** `proposal.md`, `design.md`, `specs/tui-operational-residuals/spec.md`, `odd/tasks/feat-tui-ops-residuals.md`
> **Hard rule:** cada unidad de apply ≤ ~100 líneas cambiadas; slices totales ≤ ~400 líneas de review. Test-first para toda unidad Go (`openspec/config.yaml` `apply.tdd: true`).

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines (docs de este change) | ~705 (proposal 98 + design 265 + spec 153 + tasks 185 + PRD 1 + tracking 5) |
| Estimated changed lines (apply futuro) | ~900–1200 (≈450 impl + ≈550 tests + ≈100 docs/schema), repartidos entre S1/S2/S3 |
| 400-line budget risk (docs de este change) | Bajo (docs lane, un solo commit, structural readback) |
| 400-line budget risk (apply) | **Alto** (`openspec/config.yaml` `review_budget_lines: 400`) |
| Chained PRs recommended | **Sí** (apply, por residual: S2 → S1 → S3) |
| Suggested split (apply) | PR-S2 day-keys (refactor, sin datos) → PR-S1 audit table + vista → PR-S3 purge-inactive |
| Delivery strategy | ask-on-risk; docs lane sin native review (structural readback); apply con native review por slice |
| Chain strategy (apply) | stacked-to-main (cada PR a `main` desde su feature branch, merge en orden) |

`Decision needed before apply: Yes` (OQ-1..OQ-6, Phase 1). `Chained PRs recommended: Yes`. `Chain strategy: stacked-to-main`. `400-line budget risk: High` para el apply.

---

## Phase 1 — Spec Q&A con el owner (bloqueante antes del apply)

> Preguntas que el worker **no puede resolver solo** porque implican producto, gobernanza o trade-offs de defensa. Cada una necesita una decisión explícita del owner antes de aplicar T3/T4.

### TASK-1.1 — Shape de `purge-inactive` (OQ-1)

- [ ] ¿Capacidad del menú `mcp-server admin tui` (recomendado, alineado con ADR-0016/0010) **o** sub-comando dedicado `mcp-server admin purge-inactive` (honra el literal de `docs/PRD.md:904`)?
- [ ] Si sub-comando: fijar su nombre exacto y su contrato de salida/exit code.
- [ ] Si menú: fijar la etiqueta en español de la entrada.
- **Bloquea:** TASK-2.5, TASK-2.6.

### TASK-1.2 — Inmutabilidad del audit (OQ-2)

- [ ] ¿Alcanza el append-only de API o se agrega un trigger SQLite que rechace `UPDATE`/`DELETE` sobre la tabla de auditoría?
- **Bloquea:** TASK-2.4 (DDL).

### TASK-1.3 — Alcance de purge (OQ-3)

- [ ] ¿La purga borra **solo cuentas** (default seguro, según `docs/PRD.md:904`) o también profesionales huérfanos vinculados por `accounts.professional_id` (sin FK)?
- **Bloquea:** TASK-2.5.

### TASK-1.4 — Cross-ref PRD (OQ-4)

- [ ] ¿Se corrige en un change futuro `docs/PRD.md:1343`, que atribuye el follow-up de day-keys a ADR-0015 cuando el texto normativo está en ADR-0016 Decision 3.4? (No corregible en la superficie de este change.)
- **No bloquea** el apply; registra deuda documental.

### TASK-1.5 — Retención del audit (OQ-5)

- [ ] ¿Retención indefinida o purga de filas > N días? Si hay purga, ¿quién la ejecuta y con qué cadencia?
- **Bloquea:** TASK-2.4 si se decide incluir retención.

### TASK-1.6 — Límite de la vista (OQ-6)

- [ ] ¿La vista pagina o limita por N eventos? ¿Default visible (propuesto: 200)?
- **Bloquea:** TASK-2.3.

---

## Phase 2 — Apply (change posterior; slices por residual)

> Cada slice se dimensiona a ≤ ~100 líneas por unidad de review; si una unidad supera el presupuesto, se divide antes de apply. Test-first obligatorio.

### Slice PR-S2 — Normalización de day-keys (refactor, sin migración de datos)

#### TASK-2.1 — RED: tests de traducción y frontera

- [ ] Tests de `entity.DayKey` / `entity.ScheduleWeekday`: tabla completa `0..6`, domingo (`DayKey=7`, `ScheduleWeekday=0`), lunes (`1`/`1`)
- [ ] Test de frontera `GetOpenClose`: día cerrado (`ok=false, err=nil`) vs clave corrupta (`err != nil`, fail-closed)
- [ ] Registrar el fallo observado (RED)
- **Edit surface:** `internal/domain/entity/business_profile_test.go`, `internal/domain/entity/daykey_test.go` (new)

#### TASK-2.2 — GREEN: tipos nominales + call sites

- [ ] `internal/domain/entity/daykey.go` (new) con `DayKey` (1..7) y `ScheduleWeekday` (0..6) y sus constructores desde `time.Weekday`
- [ ] Cambiar firma de `GetOpenClose(day DayKey)` y del puerto `SchedulesRepo.FindByProfessionalAndDay(..., day ScheduleWeekday)`
- [ ] Reemplazar los `int(...Weekday())` crudos en `slot_context.go:89`, `availability.go:108`, `booking_time_validator.go:116`
- [ ] Correr los tests verdes
- [ ] **Gate:** `go fmt`, `go vet`, `golangci-lint`, `go build -o /dev/null ./...`, `go test -race ./internal/domain/...`
- **Edit surface:** `internal/domain/entity/daykey.go`, `internal/domain/entity/business_profile.go`, `internal/domain/repository/schedules.go`, `internal/domain/service/slot_context.go`, `internal/domain/service/availability.go`, `internal/domain/service/booking_time_validator.go`
- **Slice estimate:** ~120–180 líneas (impl + tests). Si excede, dividir entity vs service.

### Slice PR-S1 — Auditoría persistida + vista read-only

#### TASK-2.3 — RED: tests de auditoría (repo/núcleo)

- [ ] Tests de DDL aditiva e idempotente (`initSchema` x2), `INSERT` por tipo de mutación, ausencia de PII (`phone_masked`), lectura `ORDER BY ts DESC LIMIT ?`, proyección tipada `AuditEventView` (sin `any`)
- [ ] Registrar fallos (RED)
- **Edit surface:** `internal/repository/accounts_audit_test.go` (new), `internal/admin/audit_test.go` (new)

#### TASK-2.4 — GREEN: tabla + adaptador (aplica OQ-2)

- [ ] DDL `accounts_audit` en `internal/db/schema.go` (nuevo builder compuesto por `initSchema`), con índices `ts` y `target_id`
- [ ] Adaptador `internal/repository/accounts_audit.go` con prepared statements (`Append`, `List`)
- [ ] Puerto en `internal/domain/repository` si la escritura vive fuera del repo de cuentas
- [ ] Conectar `Append` en los puntos de emisión existentes (`internal/repository/accounts.go`, `internal/application/usecase/maintenance_audit.go`) manteniendo el `slog`
- [ ] **Edit surface:** `internal/db/schema.go`, `internal/repository/accounts_audit.go` (new), `internal/domain/repository/accounts_audit.go` (new), `internal/repository/accounts.go`, `internal/application/usecase/maintenance_audit.go`

#### TASK-2.5 — GREEN: núcleo + vista (aplica OQ-6)

- [ ] `internal/admin.ListAuditEvents` con proyección mínima
- [ ] `capabilityAuditLog` en `internal/tui/view.go` + dispatch en `internal/tui/model.go` + pantalla read-only
- [ ] Entrada espejo en `adminMenuOptions` (`cmd/mcp-server/admin_tui.go`) con el mismo núcleo
- [ ] Tests MVU (read-only, mensajes en español) + test de paridad de consola
- [ ] **Gate:** pipeline completo + `go test -race ./...`
- **Edit surface:** `internal/admin/audit.go` (new), `internal/tui/view.go`, `internal/tui/model.go`, `internal/tui/cmds.go`, `cmd/mcp-server/admin_tui.go`, tests de paridad
- **Slice estimate:** ~300–380 líneas totales; el slice se divide en (a) tabla+adaptador y (b) núcleo+vista si supera el presupuesto.

### Slice PR-S3 — `purge-inactive` (aplica OQ-1/OQ-3)

#### TASK-2.6 — RED: tests de purga

- [ ] Tests del núcleo: refusa owner activo, borra solo inactivas, idempotente, re-lectura pre-write, errores semánticos en español
- [ ] Tests de presentación: confirmación fuerte requerida (TTY + consola), input débil re-pregunta, cancelación no borra
- [ ] Test de FK-safety: profesional + reservas sobreviven a la purga de la cuenta inactiva
- [ ] Registrar fallos (RED)
- **Edit surface:** `internal/admin/purge_test.go` (new), `internal/tui/purge_console_test.go` (new), `internal/repository/accounts_purge_test.go` (new)

#### TASK-2.7 — GREEN: núcleo `PurgeInactiveAccounts` + puerto `AccountsPurge`

- [ ] Puerto mínimo `AccountsPurge` (listar inactivas + borrar por id) + `DELETE ... WHERE id = ? AND is_active = 0` con prepared statement
- [ ] `admin.PurgeInactiveAccounts` con `TUIContext`, guarda de owner activo y re-lectura pre-write
- [ ] **Edit surface:** `internal/admin/purge.go` (new), `internal/admin/accounts.go` (puerto), `internal/repository/accounts.go`

#### TASK-2.8 — GREEN: presentación (menú TUI y/o sub-comando según OQ-1) + paridad consola

- [ ] Capacidad de menú Bubble Tea **o** sub-comando `mcp-server admin purge-inactive`, según decisión del owner
- [ ] Confirmación fuerte en ambos flujos; mensajes en español; nunca borra sin confirmar
- [ ] **Edit surface:** `internal/tui/view.go`, `internal/tui/model.go`, `cmd/mcp-server/admin_tui.go` (o `cmd/mcp-server/admin_purge.go` si sub-comando)
- **Slice estimate:** ~250–350 líneas totales; dividir núcleo y presentación si supera el presupuesto.

### Slice PR-S4 — Corrección del PRD (una línea)

#### TASK-2.9 — Reescritura de `docs/PRD.md:864`

- [ ] Reescribir **exactamente** la línea 864 según design §4 (MVP ENTREGADO 2026-09-16 + residuales), manteniendo el estilo español del archivo
- [ ] Verificar con `git diff --stat docs/PRD.md` que el diff es 1 línea (`1 insertion(+), 1 deletion(-)`)
- [ ] **Edit surface:** `docs/PRD.md:864` únicamente
- **Slice estimate:** ~1 línea

---

## Phase 3 — Gates de cierre (por PR, no por unidad)

- [ ] `go fmt ./...` y `go vet ./...` limpios; `golangci-lint run ./...` limpio
- [ ] `go build -o /dev/null ./...` pasa; `go test -v -race ./...` pasa
- [ ] Sin violaciones de capas: SQL solo en `internal/repository`; audit escrito desde persistencia, nunca desde la UI; tipos wire solo en `internal/mcp`; `internal/domain` no importa capas externas
- [ ] Sin `any`; sin concatenación de SQL; prepared statements en todo `INSERT`/`SELECT`
- [ ] Sin tools MCP nuevos ni cambios de RBAC (verificación explícita del no-goal)
- [ ] Structural readback (docs de este change) / native review (apply) según el Verification & Review Protocol; GGA pasa; aprobación del owner (nunca `--no-verify`)
- [ ] Slices dentro del presupuesto de ~400 líneas; CI verde en cada SHA antes del siguiente push
- [ ] `docs/PRD.md` diff = exactamente una línea

---

## Traceability

| Requirement (spec) | Tasks |
|---|---|
| Auditoría persistida por mutación + DDL aditiva | 2.3, 2.4 |
| Auditoría sin PII (teléfono enmascarado, actor opaco) | 2.3, 2.4 |
| Vista de auditoría read-only (TTY + consola) | 2.3, 2.5 |
| Convención única de day-key + entry point + pin por test | 2.1, 2.2 |
| Cero migración de datos (S2) | 2.2 |
| `purge-inactive` solo inactivas, refusa activas, idempotente | 2.6, 2.7 |
| Confirmación extra en ambos flujos | 2.6, 2.8 |
| FK-safe | 2.6, 2.7 |
| Purge/audit fuera del MCP | 2.4, 2.5, 2.7, 2.8 (verificado en Phase 3) |
| Línea del PRD corregida (1 línea) | 2.9 |

## Open questions embedded (owner)

| # | Pregunta | Task | Bloquea |
|---|---|---|---|
| OQ-1 | Shape de `purge-inactive`: menú TUI vs sub-comando dedicado | 1.1 | 2.5/2.8 |
| OQ-2 | Trigger SQLite de inmutabilidad vs append-only de API | 1.2 | 2.4 |
| OQ-3 | Alcance de purge: solo cuentas vs también profesionales huérfanos | 1.3 | 2.7 |
| OQ-4 | Corregir cross-ref ADR de day-keys en `docs/PRD.md:1343` (change futuro) | 1.4 | — (deuda documental) |
| OQ-5 | Retención del audit persistido | 1.5 | 2.4 |
| OQ-6 | Límite/paginación de la vista de audit | 1.6 | 2.5 |


---

> **OWNER ROUND 1 RESUELTO (2026-10-18):** OQ-1 menú TUI · OQ-2 trigger+append-only · OQ-3 solo cuentas · OQ-4 fix PRD:1343 ahora · OQ-5 retención indefinida · OQ-6 últimos 200 sin paginación. Phase 1 cerrada.
