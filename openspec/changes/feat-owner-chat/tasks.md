# Tasks: feat-owner-chat

> **Change:** feat-owner-chat
> **Status:** Phase 1 DONE (doc-first landed) · **expansión S7/S8 landed 2026-10-19** — el apply (T2.0a → T5.1) es un change posterior.
> **Inputs:** `proposal.md`, `design.md`, `specs/owner-chat/spec.md`, `odd/tasks/feat-owner-chat-b12.md` (especificación autoritativa S1–S8, T1–T5, L1–L7)
> **Hard rule:** **S1–S6 no tocan código Go.** Si el apply descubre una necesidad de código distinta de S7/S8, se registra como **S nueva con evidencia `path:line`** antes de escribirla (`odd/tasks/feat-owner-chat-b12.md:21-22`). **S7 (dos tools read) y S8 (data hygiene) son la excepción aprobada** (`:40`): su superficie está inventariada en `design.md` §10.1 y no se amplía slice a slice. Cada unidad de apply ≤ ~100 líneas cambiadas; slices totales ≤ ~400 líneas de review (`openspec/config.yaml` `review_budget_lines: 400`); lo que exceda se encadena (§Forecast).

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines (docs de este change — Phase 1) | ~1.150 (proposal ~260 + design ~470 + spec ~330 + tasks ~130) tras la expansión S7/S8 |
| Estimated changed lines (apply — Phase 2, S7 code) | **~620–760**: S7a ~200–240 (puerto + repo + DTO + usecase + tool + wiring) y S7b ~150–190, más docs de inventario ~40–60 |
| Estimated changed lines (apply — Phase 2, tests S7) | ~220–300 (repositorio sqlmock, 2 use cases, handler, integración, `expectedToolCount`) |
| Estimated changed lines (apply — Phase 2, skill/contrato/runbook) | ~300–420 (skill v1.1 ~120–160; gateway-contract ~120–160; runbook ~60–100) |
| Estimated changed lines (apply — S8) | ~20–40 (datos vía tools: sin código; + nota en el runbook) |
| 400-line budget risk (Phase 1) | **Bajo** — docs lane, structural readback |
| 400-line budget risk (apply) | **Alto** — S7 sola ya roza/excede el budget con tests; el resto también |
| Chained PRs recommended (apply) | **Sí, obligatorio**: PR-1 S7a (+tests+inventario) · PR-2 S7b (+tests) · PR-3 skill v1.1 · PR-4 gateway-contract + runbook (incluye el paso S8.1) |
| Delivery strategy | Phase 1: docs lane (structural readback). Phase 2: native review (tools nuevas con RBAC + comportamiento conversacional del canal) |
| Chain strategy (Phase 2) | stacked-to-main, en el orden del `design.md` §8.4 |

`Decision needed before apply: No` (alcance y decisiones de producto cerrados por el owner; **OQ-4** y **OQ-5** son de datos/configuración y no bloquean S7). `Chained PRs recommended: Yes (mandatory)`.

---

## Phase 1 — Doc-first (este change) — COMPLETA

> **Sin preguntas bloqueantes.** Las decisiones del owner son finales (`odd/tasks/feat-owner-chat-b12.md:10-15,35-36`): dos personas, doble gate para deletes, TUI-only para purge/deactivate/transfer/audit, gateway Hermes-side fuera de este repo, runbook de operador.

### TASK-1.1 — Change abierto con la estructura de la casa

- [x] `openspec/changes/feat-owner-chat/proposal.md` — resumen, problema, outcome, scope S1–S6 mapeado 1:1, non-goals, constraints/fences, Evidence Inventory E1–E48
- [x] `openspec/changes/feat-owner-chat/design.md` — selección de persona, doble gate conversacional, exclusiones TUI-only, frontera gateway-side + checklist, runbook, notas de render, impacto en código
- [x] `openspec/changes/feat-owner-chat/specs/owner-chat/spec.md` — 8 requirements con GIVEN/WHEN/THEN (S1–S6 + persona cliente intacta)
- [x] `openspec/changes/feat-owner-chat/tasks.md` — este archivo
- **Edit surface:** `openspec/changes/feat-owner-chat/**` (nuevo)

### TASK-1.2 — Evidencia anclada

- [x] Cada afirmación del proposal cita `path:line` (E1–E48)
- [x] Citados: `docs/gateway-contract.md` (identidad §1, loop §3, Telegram §5, runbook §7, política §8), ADR-0018 (D1/D2/D3b), ADR-0020 (D1–D5), `setup/templates/recepcionista-negocio/SKILL.md` v1.0.0 (versión, "When to Use", regla 5, Pitfalls, Verification), PRD 1.21/1.22 (`docs/PRD.md:1466`, `:1467`), RBAC del composition root (`cmd/mcp-server/main.go:427-450`), `auth.RequireRole` por use case, doble gate de purge (`internal/tui/purge_flow.go:43-52`, `internal/tui/model.go:461-473`, `internal/admin/purge.go:87-98`), charset/placeholder (`internal/domain/entity/business_profile.go:191-205,223-233`; `internal/admin/skill.go:47-49,155-186`), cascada (`internal/admin/skill.go:196-225,533-560`; `scripts/install.sh:1106-1120`; `scripts/tests/install_skill_test.sh:77-82`), smoke B2 (`odd/tasks/backlog.md:50`)
- [x] Registro del doc-first en `odd/tasks/feat-owner-chat-b12.md` (Log L3)

### TASK-1.3 — Verificación de la fase

- [x] `git diff --stat` limitado a `openspec/changes/feat-owner-chat/**` + `odd/tasks/feat-owner-chat-b12.md`
- [x] Pipeline Go **no aplica** (no se tocó código): sin `go build`, sin `go test`
- [x] Readback de markdown: cada S1–S6 tiene requirement(s) con escenarios; ninguna afirmación de negocio inventada

---

## Phase 1 — OQ (redacción, no alcance; se cierran en la ronda de owner previa a T2.2)

> Ninguna bloquea el apply como diseño. OQ-1/2/3 bloqueaban el **texto literal** de la skill v1.1 (T2.3) y ya están resueltas. OQ-4/OQ-5 son **de datos/configuración**, no de diseño: OQ-4 se resuelve al ejecutar T2.1 (valores del host) y OQ-5 queda fuera de este change.

### OQ-1 — Saludo/texto de la persona owner — **RESUELTO 2026-10-19: saludo neutro (b)**

- ¿Cómo abre la persona owner una conversación? El saludo cliente está congelado por el owner ("cordial y orientador", ADR-0020 Decisión 3) y su ejemplo usa `{nombre}`/`{business_name}`; para el owner ese registro no existe (su nombre viene de `accounts.display_name`, que la skill no ve).
- Opciones: (a) saludo corto sin nombre ("Estoy en modo dueño de {business_name}. ¿Qué hacemos?") — **default propuesto**; (b) saludo neutro idéntico al cliente hasta que el owner pida algo; (c) sin saludo, ir directo a la tarea.
- **Bloquea:** el texto final de T2 (no el diseño).

### OQ-2 — Fraseo literal del doble gate — **RESUELTO 2026-10-19: default (precedente TUI adaptado a chat)**

- ¿El texto exacto de G1/G2 lo redacta el owner o se acepta el default derivado del precedente TUI (`"Esta acción ELIMINA PERMANENTEMENTE …"` / `"ÚLTIMA CONFIRMACIÓN: … es permanente y sin vuelta atrás"`)?
- **Default propuesto:** reusar el fraseo del precedente adaptado a chat (sin ids crudos, con nombre + identificador del recurso).
- **Bloquea:** el texto final de T2 (no el diseño: la semántica de dos gates está cerrada).

### OQ-3 — ¿Confirmación simple en writes no-delete? — **RESUELTO 2026-10-19: sin gate**

- La decisión del owner cubre **deletes**. ¿Un write no destructivo pero de alto impacto (p. ej. `update_business_profile` con `messenger_platform`) pide una confirmación simple, o se ejecuta directo?
- **Default propuesto:** **sin gate** — no se inventa fricción que el owner no pidió; la persona owner ejecuta y reporta el cambio.
- **Bloquea:** nada del diseño; define si T2 agrega un renglón.

### OQ-4 — Valores exactos de `specialties` del demo — **ABIERTA (owner)**

- ¿Qué servicios declara cada uno? Romina → Lavado de Cabello (confirmado por el owner en L6); **Santiago y Joaquín**: ¿Corte de Cabello, Corte de barba, o ambos? Los ids concretos salen del host (`search_services_advanced`), no de este documento.
- **Default registrado:** Santiago y Joaquín → Corte de Cabello **y** Corte de barba (el operador corrige en el chat antes de aplicar).
- **Bloquea:** el paso S8.1 (T2.1) y la post-condición del runbook (T4.1). **No** bloquea S7a/S7b ni el texto de la skill.

### OQ-5 — ¿El setup/`install.sh` emite specialties reales? — **REGISTRADA, fuera de este change**

- Una instalación nueva nace con `"specialties": []` (`scripts/install.sh:930`) y el seed inserta staff antes de services (`internal/config/setup_seeder.go:151-152`, `:178`): emitir la relación exige invertir el orden y un mapeo nombre→id (contrato nuevo en `setup_staff.json`).
- **Default:** no se hace en este change; se registra como follow-up con su propio diseño. S8.1 cubre el host existente.
- **Bloquea:** sólo la promesa "instalación nueva ya trae la relación cargada".

---

## Phase 2 — Apply (change posterior; una unidad por T)

> **Gate de apply:** native review (S7 introduce tools con RBAC; el resto afecta el comportamiento conversacional del canal). Cada unidad se revisa y commitea por separado; el worker autor reporta rondas GGA y log path.
>
> **Renumeración documentada (2026-10-19):** S7 y S8 entran **delante** en el orden de apply, así que los IDs se corren: el antiguo `TASK-2.1` (skill RED) pasa a **`TASK-2.2`** y el antiguo `TASK-2.2` (skill GREEN) a **`TASK-2.3`**. No hay referencias externas a esos sub-IDs (`odd/tasks/feat-owner-chat-b12.md` referencia T2–T5, no los sub-IDs).
>
> **Orden duro** (`design.md` §8.4): `T2.0a` → `T2.0b` → `T2.1` (datos) → `T2.3` (texto de la skill). El texto no puede afirmar una verificación que el transporte todavía no tiene.

### TASK-2.0a (T2.0, S7a) — RED-first: `search_professionals_advanced`

**Contrato:** `design.md` §8.2 (D1–D3 + edge fail-closed); `specs/owner-chat/spec.md` requirement (S7a).

- [ ] **RED 1 (use case)** — `internal/application/usecase/search_professionals_advanced_test.go` (nuevo): caller con rol **cliente** pasa; `service_id` filtra por `specialties`; `specialties` vacío → `results: []` (**nunca** el catálogo); `service_id` inexistente → `ErrCodeNotFound` semántico. Capturar el fallo esperado: `undefined: NewSearchProfessionalsAdvancedUseCase`
- [ ] **RED 2 (repositorio)** — `internal/repository/professionals_test.go`: `FindAllActive` devuelve **todos** los activos aun con caller staff (contraste con `FindActive`, E55)
- [ ] **RED 3 (transporte)** — `internal/mcp/tools_professionals_test.go` (nuevo): mapeo `query_text`/`service_id`, `nil` result → `{results:[]}`, caller propagado, wire **sin** `email`/`phone`; e `internal/mcp/test_helpers_test.go:10` (`expectedToolCount` 25) + `internal/mcp/tools_test.go:358` (lista de nombres) fallando
- [ ] **GREEN** — puerto `ProfessionalsRepo.FindAllActive` (`internal/domain/repository/professionals.go`) → implementación prepared-statement en `internal/repository/professionals.go` → `dto.ProfessionalSearchEntry`/`SearchProfessionalsAdvancedInput`/`Result` (`internal/application/dto/search.go`) → use case (validación de `service_id` con `ServicesRepo.FindByID`, filtro `HasSpecialty`, match textual sobre `name`+`role_specialty`, DTO PII-free) → `SearchProfessionalsAdvancedPort` (`internal/mcp/ports.go`) + campo de `Config` → handler en `internal/mcp/tools_professionals.go` (nuevo) + `registerProfessionalsReadTools` desde `internal/mcp/server.go` → wiring en `cmd/mcp-server/main.go` (**sin** entrada de `ToolRBAC`) → actualizar el comentario del open-set (`:409-421`, de tres a cuatro tools sin entrada)
- [ ] **TRIANGULATE** — catálogo con un profesional `inactive` (no aparece), `query_text` con operadores FTS (error semántico reusando `validateFTSQuery`, E52), profesional con JSON inválido en `specialties` (excluido, sin pánico: base `HasSpecialty`)
- [ ] **Inventario** — `docs/PRD.md:492,1054,1271,1285`; `docs/SDD.md:400`; `docs/demo-plan.md:202` (25 tools; RF3 ampliado **sin** FTS)
- **Edit surface:** `internal/domain/repository/professionals.go`, `internal/repository/professionals.go`, `internal/application/dto/search.go`, `internal/application/usecase/search_professionals_advanced.go` (nuevo), `internal/mcp/ports.go`, `internal/mcp/config.go`, `internal/mcp/tools_professionals.go` (nuevo), `internal/mcp/server.go`, `cmd/mcp-server/main.go`, tests citados, docs de inventario
- **No tocar:** `internal/db/**`, `internal/auth/**`, `internal/repository/services.go`, `setup/**`
- **Slice estimate:** ~200–240 líneas de código + ~120–150 de tests
- **Gate:** `go fmt ./...` · `go vet ./...` · `golangci-lint run ./...` · `go build -o /dev/null ./...` · `go test -v -race ./...`

### TASK-2.0b (T2.0, S7b) — RED-first: `get_professional_schedules`

**Contrato:** `design.md` §8.3 (D4–D5); `specs/owner-chat/spec.md` requirement (S7b).

- [ ] **RED 1 (use case)** — `internal/application/usecase/get_professional_schedules_test.go` (nuevo): owner lee; `admin`/`staff`/`client` → `ErrCodeForbidden`; id inexistente → NotFound; existente sin filas → lista vacía
- [ ] **RED 2 (repositorio)** — `internal/repository/schedules_test.go`: `FindByProfessional` ordena por `day_of_week` y distingue `ErrNotFound` de lista vacía
- [ ] **RED 3 (transporte)** — `internal/mcp/tools_professionals_test.go`: input mapping, error semántico sin filtración (`internal/mcp/errors.go`), caller propagado (la `expectedToolCount` ya quedá en 25 desde T2.0a)
- [ ] **GREEN** — `SchedulesRepo.FindByProfessional` (contrato + adaptador) → `dto.GetProfessionalSchedulesInput/Result` (`internal/application/dto/maintenance.go`, familia schedule) → use case con `auth.RequireRole(auth.RoleOwner)` reinyectando el caller (molde `delete_schedule.go:41-51`) → `GetProfessionalSchedulesPort` + `Config` → handler en `internal/mcp/tools_professionals.go` reusando el shape `scheduleOut` → `ToolRBAC{"get_professional_schedules": {auth.RoleOwner}}` en el composition root + wiring
- [ ] **TRIANGULATE** — días fuera de rango (el adaptador ya rechaza 0–6), profesional `inactive` (se devuelve con su `status`, no se oculta: el gate necesita saber contra quién confirma), `admin` recibe `-32002` (no `-32603`)
- [ ] **Inventario** — mismos docs que T2.0a (25 tools, ahora con la lectura owner-only)
- **Edit surface:** `internal/domain/repository/schedules.go`, `internal/repository/schedules.go`, `internal/application/dto/maintenance.go`, `internal/application/usecase/get_professional_schedules.go` (nuevo), `internal/mcp/ports.go`, `internal/mcp/config.go`, `internal/mcp/tools_professionals.go`, `cmd/mcp-server/main.go`, tests citados, docs de inventario
- **No tocar:** `internal/domain/service/availability.go` (el read path de disponibilidad no cambia), `internal/db/**`
- **Slice estimate:** ~150–190 líneas de código + ~100–150 de tests
- **Gate:** pipeline completo (§Pre-Commit Checklist de `AGENTS.md`)

### TASK-2.1 (T2.1, S8) — Data hygiene de `specialties`

**Contrato:** `design.md` §9; `specs/owner-chat/spec.md` requirement (S8).

- [ ] **S8.1 datos (sin código, owner hands o persona owner por chat):** `search_services_advanced` → ids reales de **Lavado de Cabello** y de los cortes; luego `update_professional(professional_id, specialties: [ids])` para Romina, Santiago y Joaquín. **Valores exactos: OQ-4** (default documentado en `design.md` §12.2)
- [ ] **Verificación (obligatoria, es la definición de hecho de S8.1):** `search_professionals_advanced(service_id = Lavado de Cabello)` devuelve **sólo** a Romina; `search_professionals_advanced(service_id = Corte de Cabello)` **no** devuelve a Romina
- [ ] **S8.2 registro (sin código):** dejar escrito que el form de Add Staff **no** lleva specialties porque crea cuentas, no profesionales (E61), y que el origen del `[]` en hosts nuevos es `scripts/install.sh:930` + el orden del seed (E60). La consecuencia práctica ("una instalación nueva nace sin relación cargada") va al runbook de `TASK-4.1`
- [ ] **OQ-5 registrada, no ejecutada:** que el setup emita specialties reales (invertir orden del seed + mapeo nombre→id) es un change propio si el owner lo quiere. **No** se toca `scripts/**` ni `internal/config/**` acá
- [ ] **Sin regresión de TUI:** confirmar que el diff no toca `internal/tui/**` ni `internal/admin/**`
- **Edit surface:** `docs/gateway-contract.md` §7 (paso S8.1 del runbook, vía `TASK-4.1`) + datos del host. **Ningún archivo Go**
- **Nota de honestidad:** el dataset demo **no está versionado en este repo** (los nombres no aparecen en `scripts/`, `internal/config/` ni `setup/`): vive en el VM. Por eso S8.1 no puede verificarse con tests; se verifica con las dos llamadas de arriba sobre el host
- **Slice estimate:** ~20–40 líneas (documentación del paso) + cero código
- **Gate:** no aplica pipeline Go (no hay cambio Go); el pipeline corre completo en el PR donde aterrice el runbook (`TASK-4.1`)

### TASK-2.2 (T2.2, S1–S3) — RED: escenarios de la skill v1.1 como checklist de verificación

- [ ] Convertir los escenarios de `specs/owner-chat/spec.md` en una checklist ejecutable manual (smoke guiado) o en asserts del seed cuando aplique (`scripts/tests/install_skill_test.sh` cubre el render, no la conducta)
- [ ] Registrar la línea base: la v1.0.0 **falla** los escenarios de persona owner (evidencia: regla 5 deriva deletes al titular, `setup/templates/recepcionista-negocio/SKILL.md:40-43`) — RED documentado
- **Edit surface:** `odd/tasks/feat-owner-chat-b12.md` (sección de verificación) o `scripts/tests/install_skill_test.sh` si se agrega assert de render
- **Slice estimate:** ~30–50 líneas

### TASK-2.3 (T2.3, S1–S3 + S7/S8 en el texto) — GREEN: template v1.1

> **Prerrequisito:** `TASK-2.0a` y `TASK-2.0b` mergeados (el texto cita las tools; `design.md` §8.4).

- [ ] Sección `Persona` (R1–R4: Telegram por canal, WhatsApp por rol resuelto, fail-closed, prohibición de promoción por contenido) + `Procedure — persona cliente` copiado sin cambios + `Procedure — persona owner` (alcance operativo, doble gate G0–G4, exclusiones TUI-only, derivación a la consola de administración) + `Pitfalls` y `Verification` extendidos
- [ ] **Flujo de reserva (S7a):** el procedimiento cliente MUST instruir el orden **servicio → `search_professionals_advanced(service_id)` → ofrecer sólo esos profesionales**, y MUST prohibir ofrecer nombres que no vengan de la tool. Si la tool devuelve vacío, la respuesta debe decir que no hay relación cargada y derivar, **nunca** completar con el catálogo entero (es el bug del smoke)
- [ ] Bump de frontmatter a `version: 1.1.0`; `description` ≤ 60 caracteres **sin** mencionar owner; `{business_name}` como único placeholder renderizado; `{nombre}` intacto
- [ ] Implementar la matriz de verificabilidad del design **§2.4** en el texto del gate: servicios y excepciones por lectura; `delete_schedule` ahora **sí** se resuelve y verifica con `get_professional_schedules` (S7b). El pitfall de "no afirmar verificación sin lectura" se conserva como regla general, no como excepción de `delete_schedule`
- [x] OQ-1/OQ-2/OQ-3 cerradas con el owner (2026-10-19: neutro / default / sin gate) y registradas en el Log L5 de la feature doc
- [ ] **Gate:** `bash -n scripts/install.sh`, `bash scripts/tests/install_skill_test.sh` (si se tocó el test), shellcheck si está disponible; sin Go
- **Edit surface:** `setup/templates/recepcionista-negocio/SKILL.md`, opcionalmente `scripts/tests/install_skill_test.sh`
- **Slice estimate:** ~130–170 líneas

> **Nota de ID:** este bloque era `TASK-2.2` antes de la expansión S7/S8 (ver la nota de renumeración al inicio de Phase 2).

### TASK-3.1 (T3, S4) — Gateway contract: reglas owner + checklist de aceptación

- [ ] Nueva subsección de la persona owner en `docs/gateway-contract.md`: activación (canal en Telegram, rol resuelto en WhatsApp), alcance operativo, doble gate de deletes como contrato conversacional, exclusiones TUI-only
- [ ] Checklist de aceptación C1–C10 (design §4.2) con referencia explícita a que el build vive en el repo del gateway
- [ ] Verificar que el protocolo de identidad (§1–§3) y §5 quedan **sin cambios** de semántica
- **Edit surface:** `docs/gateway-contract.md`
- **Slice estimate:** ~120–160 líneas

### TASK-4.1 (T4, S5–S6) — Runbook de operador

- [ ] Secuencia numerada: Transfer Ownership (`+5491100000000` → `+5492617132740`) → `messenger_platform`/`messenger_id` → opción 7 (cascada v1.1), con post-condiciones y la restricción de orden (transfer antes de persistir el teléfono del owner)
- [ ] **Paso S8.1 (datos):** cargar `specialties` con `search_services_advanced` + `update_professional` (valores: OQ-4) y verificar con `search_professionals_advanced`; dejar dicho que en una instalación **nueva** la relación nace vacía (E60) hasta que se resuelva OQ-5
- [ ] Nota de rollback honesta (transferencia se revierte repitiéndola; no hay "undo")
- [ ] Nota de fricción esperada de la cascada: `SkillStateModified` con la v1.0.0 instalada no significa edición del usuario
- [ ] Verificar que §7.1/§7.2 existentes quedan coherentes (sin duplicar el protocolo)
- **Edit surface:** `docs/gateway-contract.md` §7
- **Slice estimate:** ~60–100 líneas

### TASK-5.1 (T5) — Gates de entrega

- [ ] `bash -n scripts/install.sh` + `bash scripts/tests/install_skill_test.sh` si algún artefacto de instalación se tocó
- [ ] **Pipeline Go completo en PR-1/PR-2** (`TASK-2.0a`/`TASK-2.0b`: `go fmt`, `go vet`, `golangci-lint`, `go build -o /dev/null ./...`, `go test -v -race ./...`). En PR-3/PR-4 (skill/contrato/runbook) el pipeline Go igual se corre como guardia de no-regresión
- [ ] `go test -v -race ./...` verde también en el PR del runbook, porque el número de tools queda pineado en `internal/mcp/test_helpers_test.go:10` y en `internal/mcp/tools_test.go:358` (E62)
- [ ] Rondas GGA + review nativo (routing: el cambio es sensible — tools nuevas con RBAC + comportamiento conversacional del canal del negocio)
- [ ] PR por routing, **chained obligatorio**: PR-1 `TASK-2.0a`, PR-2 `TASK-2.0b`, PR-3 `TASK-2.3`, PR-4 `TASK-3.1`+`TASK-4.1` (incluye el paso S8.1). Cada PR ≤ ~400 líneas de review
- **Edit surface:** ninguna (verificación)

---

## Coverage check (S ↔ requirement ↔ task)

| Spec del feature doc | Requirement(s) del spec | Tasks |
|---|---|---|
| S1 persona owner (dos personas + selección) | "dos personas y regla de selección determinista"; "persona owner opera con las tools owner-only" | T2.2 + T2.3 |
| S2 deletes doble gate | "los tres deletes exigen doble confirmación en chat" | T2.3 (+ T3.1 para el contrato conversacional del gateway) |
| S3 exclusiones TUI-only | "cuatro operaciones quedan excluidas del chat" | T2.3 (+ T3.1) |
| S4 gateway per-sender (Hermes-side) | "el contrato del gateway publica reglas + checklist" | T3.1 |
| S5 operator steps | "el runbook fija Transfer Ownership, `messenger_platform` y la cascada" | T4.1 |
| S6 cascada de re-aplicación | "se distribuye por el template y la cascada existente" | T2.3 + T4.1 |
| **S7a `search_professionals_advanced`** | "es una lectura legible por cualquier caller autenticado" | **T2.0a** (+ T2.3 para el flujo servicio→filtrado→oferta) |
| **S7b `get_professional_schedules`** | "es owner-only y cierra el read-gap del gate" | **T2.0b** (+ T2.3 para el uso dentro de G0/G4) |
| **S8 data hygiene de `specialties`** | "los datos demo declaran las specialidades reales" | **T2.1** (datos + registro) |
| Contrato cliente intacto (fence) | "la persona cliente conserva su contrato verificado" | T2.3 |

### Cobertura por capa de S7 (qué test prueba qué)

| Capa | Test | Requirement cubierto |
|---|---|---|
| Dominio | `entity.Professional.HasSpecialty` (existente, `professional_test.go:26-95`) | base del filtro fail-closed |
| Repositorio | `professionals_test.go`, `schedules_test.go` | `FindAllActive` sin scoping; `FindByProfessional` ordenado y con NotFound vs vacío |
| Use case | 2 archivos nuevos | rol (cliente pasa en S7a; owner-only en S7b), filtro, NotFound |
| Transporte | `tools_professionals_test.go` (nuevo) | input/output, PII-free, error sanitizado |
| Integración | `server_integration_search_test.go`, `server_integration_maintenance_test.go` | RBAC real por header, `tools/list` = 25 |
| FTS | — | **no aplica**: `professionals` no tiene tabla FTS (E51) |
