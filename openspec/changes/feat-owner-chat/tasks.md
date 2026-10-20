# Tasks: feat-owner-chat

> **Change:** feat-owner-chat
> **Status:** Phase 1 DONE (doc-first landed) — el apply (T2–T4) es un change posterior.
> **Inputs:** `proposal.md`, `design.md`, `specs/owner-chat/spec.md`, `odd/tasks/feat-owner-chat-b12.md` (especificación autoritativa S1–S6, T1–T5)
> **Hard rule:** este change no toca código Go. Si el apply descubre una necesidad de código, se registra como **S nueva con evidencia `path:line`** antes de escribirla (`odd/tasks/feat-owner-chat-b12.md:21-22`). Cada unidad de apply ≤ ~100 líneas cambiadas; slices totales ≤ ~400 líneas de review (`openspec/config.yaml` `review_budget_lines: 400`).

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines (docs de este change — Phase 1) | ~700 (proposal ~135 + design ~265 + spec ~215 + tasks ~90) |
| Estimated changed lines (apply — Phase 2) | ~330–420 (skill template v1.1 ~120–160; gateway-contract update ~120–160; runbook opcional como sección de `gateway-contract.md` §7 ~60–100) |
| 400-line budget risk (Phase 1) | **Bajo** — docs lane, single commit, structural readback |
| 400-line budget risk (apply) | **Medio** — cabe en un PR si el runbook va como sección del contrato; si se separa en un doc propio, chained (T2+T3 → T4) |
| Chained PRs recommended (apply) | **Sí, condicional**: si el diff supera 400 líneas, PR-1 = skill v1.1 (T2), PR-2 = contrato + checklist + runbook (T3+T4) |
| Delivery strategy | Phase 1: docs lane (structural readback). Phase 2: native review (skill/contrato afectan comportamiento conversacional del canal) |
| Chain strategy (Phase 2) | stacked-to-main |

`Decision needed before apply: No` (alcance y decisiones de producto cerrados por el owner; ver OQ). `Chained PRs recommended: Conditional`.

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

## Phase 1 — OQ (redacción, no alcance; se cierran en la ronda de owner previa a T2)

> Ninguna bloquea el apply como diseño. Todas bloquean el **texto literal** de la skill v1.1 (T2). Si el owner no responde, T2 usa el default propuesto y lo registra en el Log.

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

---

## Phase 2 — Apply (change posterior; una unidad por T)
- [ ] **T2.0 (S7 — NUEVA, ronda owner 1)** · tool de lectura de profesionales/agendas (owner-only, snippet Go en el tramo transport/usecase) para el verificación del delete_schedule en el doble gate de chat · code lane → PR


> Gate de apply: native review (el cambio afecta el comportamiento conversacional del canal). Cada unidad se revisa y commitea por separado; el worker autor reporta rondas GGA y log path.

### TASK-2.1 (T2, S1–S3) — RED: escenarios de la skill v1.1 como checklist de verificación

- [ ] Convertir los escenarios de `specs/owner-chat/spec.md` en una checklist ejecutable manual (smoke guiado) o en asserts del seed cuando aplique (`scripts/tests/install_skill_test.sh` cubre el render, no la conducta)
- [ ] Registrar la línea base: la v1.0.0 **falla** los escenarios de persona owner (evidencia: regla 5 deriva deletes al titular, `setup/templates/recepcionista-negocio/SKILL.md:40-43`) — RED documentado
- **Edit surface:** `odd/tasks/feat-owner-chat-b12.md` (sección de verificación) o `scripts/tests/install_skill_test.sh` si se agrega assert de render
- **Slice estimate:** ~30–50 líneas

### TASK-2.2 (T2, S1–S3) — GREEN: template v1.1

- [ ] Sección `Persona` (R1–R4: Telegram por canal, WhatsApp por rol resuelto, fail-closed, prohibición de promoción por contenido) + `Procedure — persona cliente` copiado sin cambios + `Procedure — persona owner` (alcance operativo, doble gate G0–G4, exclusiones TUI-only, derivación a la consola de administración) + `Pitfalls` y `Verification` extendidos
- [ ] Bump de frontmatter a `version: 1.1.0`; `description` ≤ 60 caracteres **sin** mencionar owner; `{business_name}` como único placeholder renderizado; `{nombre}` intacto
- [ ] Implementar la matriz de verificabilidad del design §2.3 en el texto del gate: servicios y excepciones se resuelven/verifican por lectura; `delete_schedule` NO (no hay tool de lectura de agendas/profesionales, E47) y la skill MUST NOT afirmarlo
- [x] OQ-1/OQ-2/OQ-3 cerradas con el owner (2026-10-19: neutro / default / sin gate) y registradas en el Log L5 de la feature doc
- [ ] **Gate:** `bash -n scripts/install.sh`, `bash scripts/tests/install_skill_test.sh` (si se tocó el test), shellcheck si está disponible; sin Go
- **Edit surface:** `setup/templates/recepcionista-negocio/SKILL.md`, opcionalmente `scripts/tests/install_skill_test.sh`
- **Slice estimate:** ~120–160 líneas

### TASK-3.1 (T3, S4) — Gateway contract: reglas owner + checklist de aceptación

- [ ] Nueva subsección de la persona owner en `docs/gateway-contract.md`: activación (canal en Telegram, rol resuelto en WhatsApp), alcance operativo, doble gate de deletes como contrato conversacional, exclusiones TUI-only
- [ ] Checklist de aceptación C1–C10 (design §4.2) con referencia explícita a que el build vive en el repo del gateway
- [ ] Verificar que el protocolo de identidad (§1–§3) y §5 quedan **sin cambios** de semántica
- **Edit surface:** `docs/gateway-contract.md`
- **Slice estimate:** ~120–160 líneas

### TASK-4.1 (T4, S5–S6) — Runbook de operador

- [ ] Secuencia numerada: Transfer Ownership (`+5491100000000` → `+5492617132740`) → `messenger_platform`/`messenger_id` → opción 7 (cascada v1.1), con post-condiciones y la restricción de orden (transfer antes de persistir el teléfono del owner)
- [ ] Nota de rollback honesta (transferencia se revierte repitiéndola; no hay "undo")
- [ ] Nota de fricción esperada de la cascada: `SkillStateModified` con la v1.0.0 instalada no significa edición del usuario
- [ ] Verificar que §7.1/§7.2 existentes quedan coherentes (sin duplicar el protocolo)
- **Edit surface:** `docs/gateway-contract.md` §7
- **Slice estimate:** ~60–100 líneas

### TASK-5.1 (T5) — Gates de entrega

- [ ] `bash -n scripts/install.sh` + `bash scripts/tests/install_skill_test.sh` si algún artefacto de instalación se tocó
- [ ] Pipeline Go completo **sólo si** algo Go cambió (no esperado: si cambió, TASK-1.1 falló y hay que abrir la S nueva)
- [ ] Rondas GGA + review nativo (routing: el cambio es sensible — comportamiento conversacional del canal del negocio)
- [ ] PR por routing; si el diff de apply supera 400 líneas, chained (T2 → T3+T4)
- **Edit surface:** ninguna (verificación)

---

## Coverage check (S ↔ requirement ↔ task)

| Spec del feature doc | Requirement(s) del spec | Tasks |
|---|---|---|
| S1 persona owner (dos personas + selección) | "dos personas y regla de selección determinista"; "persona owner opera con las tools owner-only" | T2 |
| S2 deletes doble gate | "los tres deletes exigen doble confirmación en chat" | T2 (+ T3 para el contrato conversacional del gateway) |
| S3 exclusiones TUI-only | "cuatro operaciones quedan excluidas del chat" | T2 (+ T3) |
| S4 gateway per-sender (Hermes-side) | "el contrato del gateway publica reglas + checklist" | T3 |
| S5 operator steps | "el runbook fija Transfer Ownership, `messenger_platform` y la cascada" | T4 |
| S6 cascada de re-aplicación | "se distribuye por el template y la cascada existente" | T2 + T4 |
| Contrato cliente intacto (fence) | "la persona cliente conserva su contrato verificado" | T2 |
