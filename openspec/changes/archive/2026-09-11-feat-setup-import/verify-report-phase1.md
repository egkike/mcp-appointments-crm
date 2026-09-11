# Verify Report — Phase 1 (feat-setup-import-loader)

> **Change:** feat-setup-import
> **Scope:** Phase 1 only (TASK-1.1 → TASK-1.7) — setup resolver + structs + loader + business_hours mapping + unit tests
> **Branch:** `feat-setup-import-loader` · **PR:** 1 of 3 (stacked-to-main chain)
> **Verifier:** sdd-verify executor · **skill_resolution:** paths-injected (`golang-patterns`, `go-testing`)
> **Attempt:** req-verify-phase1-20260911-01 / verify-phase1-loader

---

## Status: **PASS (Phase-1 slice)** — with 1 WARNING to resolve before Phase 2

Phase 1 is implementation-complete and spec-conformant. All 13 setup-loader scenarios are covered by tests that pass with `-race`. One design deviation (WARNING) must be resolved in Phase 2 planning; it does not block PR 1.

---

## 1. Spec conformance — `specs/setup-loader/spec.md` (5 REQs, 13 scenarios)

### REQ 1 — Per-OS setup directory resolution ✅

| Scenario | Coverage | Evidence |
|----------|----------|----------|
| Linux default path | ✅ | `TestResolveSetupDirOS/linux_default` → `$HOME/.config/mcp-appointments-crm/setup` |
| XDG_CONFIG_HOME honored on Linux | ✅ | `linux with XDG_CONFIG_HOME` → `/custom/config/mcp-appointments-crm/setup` |
| macOS default path | ✅ | `macos default` → `$HOME/Library/Application Support/MCP Appointments CRM/setup` |
| MCP_SETUP_DIR override wins on any OS | ✅ | Tested on linux + darwin; implementation checks `MCP_SETUP_DIR` first, before the OS switch, so it wins on every OS by construction. Empty value treated as unset (extra case). |

Implementation (`setup_resolver.go`): precedence order `MCP_SETUP_DIR` → per-OS default; empty `HOME` → Spanish error `"…la variable HOME no está definida"`; non-linux/darwin falls to XDG rule (spec SHOULD); `filepath.Join`/`filepath.Clean` used; symlink checks intentionally omitted per ADR-SD-2. ✅ Conformant.

### REQ 2 — Typed structs pinning the three wizard shapes ✅ (one minor test gap)

| Scenario | Coverage | Evidence |
|----------|----------|----------|
| Valid wizard output decodes without error | ✅ partial | `TestLoadSetup_DecodeFixtures` decodes all 3 fixtures into the TASK-1.1 structs; `TestLoadSetup_HappyPath` asserts populated fields. |

- All struct fields are concrete types; zero `any`/`interface{}` **fields** (verified by read; pointer optionals exactly where wizard allows null). ✅
- `is_active` is `int` (ADR-SD-3) ✅; `day_of_week` int ✅; `latitude`/`longitude`/`price` float ✅.
- **Minor gap (SUGGESTION):** the spec scenario says *"every struct field MUST be populated"*. `TestLoadSetup_HappyPath` asserts a representative subset (name, currency, payment methods, business_hours, staff name/schedule, services, price-0, is_active) but not each of the 18 business fields individually (industry, country, address, lat/long, cover_photo_url, phone, messenger_*, email, website_url, description are unasserted). Fixtures do populate them; decode success + subset assertions give high confidence. Not a blocker; recommend widening assertions in a follow-up or during PR review.

### REQ 3 — Missing/malformed files → semantic Spanish errors ✅

| Scenario | Coverage | Evidence |
|----------|----------|----------|
| Happy path loads all three files | ✅ | `TestLoadSetup_HappyPath` (no error, decoded values) |
| Missing file reported by name | ✅ | `TestLoadSetup_MissingFile` → error contains `setup_staff.json` + `no existe` |
| Malformed file reported by name | ✅ | `TestLoadSetup_MalformedFile` → error contains `setup_services.json` + `formato inválido` |

Plus design extras: oversized file → `supera el tamaño máximo permitido (1 MiB)` (`TestLoadSetup_OversizedFile`, SD-4 cap via `os.Open` + `io.LimitReader(max+1)`). Errors name the **file only** — no directory paths in any error string (ADR-SD-4, verified by read). ✅ Conformant.

### REQ 4 — business_hours day-name → numeric-string mapping ✅

| Scenario | Coverage | Evidence |
|----------|----------|----------|
| monday → "1" | ✅ | `TestMapBusinessHours/monday_maps_to_1` (value round-tripped) |
| sunday → "7" | ✅ | `sunday_maps_to_7` |
| null day → absent key | ✅ | `null_sunday_becomes_absent` → empty output map; no `"sunday"`, no `""`, no null value |
| Full week (6 open + 1 null) → exactly 6 numeric keys | ✅ | `full_week_6_open_plus_1_null` — exact key-set equality asserted in **both** directions (missing + unexpected keys) |

Beyond spec: all-closed → `"{}"` ✅, unknown day → error ✅, bad HH:MM (open and close) → error ✅, **entity round-trip** through `entity.BusinessProfile.IsOpenOn(1)/GetOpenClose(1)/IsOpenOn(7)` ✅, output determinism ✅. Mapping is a pure function over the fixed `monday→1…sunday→7` table; `json.Marshal` of a map yields sorted keys (SD-5). ✅ Conformant.

### REQ 5 — Setup files are read-only to the server ✅

| Scenario | Coverage | Evidence |
|----------|----------|----------|
| Successful load leaves files untouched | ✅ | `TestLoadSetup_ReadOnly` — content bytes **and** `ModTime` identical before/after load |

Static check: `grep` for `os.Create|os.WriteFile|os.Remove|os.Chmod|os.Rename|f.Write` across the three implementation files → **zero matches**. ✅ Conformant.

**Spec coverage total: 13/13 scenarios covered (1 with subset-level field assertions — SUGGESTION, not a gap in scenario coverage).**

---

## 2. Strict TDD compliance

`openspec/config.yaml` sets `tdd: true`. `apply-progress.md` contains a **TDD Cycle Evidence** table with RED→GREEN per component:

| Component | RED claim | Credibility |
|-----------|-----------|-------------|
| Resolver | `TestResolveSetupDirOS` failed to compile (function absent) | Credible — compile-failure RED is the standard Go RED; injectable `getenv` design matches |
| Loader | `TestLoadSetup_*` compile-failure RED | Credible |
| Mapping | `TestMapBusinessHours_*` compile-failure RED | Credible |
| Validation | `TestValidateForSeed` added as triangulation post-GREEN | Credible and correctly labeled |

Cross-reference: all claimed test files exist (`setup_resolver_test.go`, `setup_loader_test.go`, `setup_mapping_test.go`) and all claimed cases are present and pass. RED evidence is narrative (not independently replayable post-hoc) — noted, not flagged; the GREEN state is verified directly below.

**Assertion quality audit:** no tautologies; no type-only assertions; error tests assert semantic substrings (`no existe`, `formato inválido`, `HH:MM`, `día desconocido`) not raw dumps; the read-only test asserts real observable state (bytes + ModTime); mapping tests assert exact key-set equality both directions; entity round-trip tests real consumer behavior (`IsOpenOn`/`GetOpenClose`). No smoke-only tests. ✅ Clean.

---

## 3. Verification commands (run synchronously by this verifier)

| Command | Observed result |
|---------|-----------------|
| `go test -v -race ./internal/config/...` | **PASS** — 24 test functions / all subtests pass (resolver 8 cases, loader happy/missing/malformed/oversized/read-only/decode-fixtures, validateForSeed 5 cases, mapping 8 + round-trip + determinism). Re-run with `-count=1` (cache-busting): `ok github.com/egkike/mcp-appointments-crm/internal/config 1.059s` |
| `go build -o /dev/null ./...` | **OK** (whole module compiles) |
| `go vet ./internal/config/...` | **OK** (no issues) |
| `gofmt -l internal/config/` | **Clean** (empty output). Note: `gofmt -l ./internal/config/...` is invalid — gofmt takes paths, not package patterns; rerun correctly as above |
| `golangci-lint run ./internal/config/...` | **0 issues** |

---

## 4. Review workload boundary

| Field | Value |
|-------|-------|
| Actual Phase-1 changed lines vs `main` | **1033 insertions, 9 files** (`git diff main --numstat`; all additions, 0 deletions) |
| Budget | 400 lines → **overage: +633 lines (2.6×)** |
| Forecast | tasks.md estimated ~700 for Phase 1 — actual 1033 is within forecast band when test verbosity is considered; the *aggregate* forecast (~1050–1300) anticipated this |
| Chain strategy | `stacked-to-main`, PR 1 of 3 — **respected**: this slice is exactly the PR-1 scope (types + resolver + loader + tests + fixtures), self-contained, no DB, no boot-path changes |
| `size:exception` | **Not used** — correct; the chain handles the overage, no exception acceptance occurred |

The 400-line overage is **explicitly stated**: 1033 > 400. It is mitigated by the pre-approved chained-PR strategy (PR 1 of 3), which is the mechanism the preflight prescribed for over-budget work under `ask-on-risk`. Per-file breakdown: loader_test 306, loader 260, mapping_test 188, resolver_test 110, types 68, resolver 42, fixtures 59 (29+16+14).

---

## 5. Out-of-scope changes

| Area | Status |
|------|--------|
| `cmd/mcp-server/main.go` | ✅ untouched (`git diff main -- cmd/ docs/ scripts/ internal/db/` → 0 files) |
| `docs/` | ✅ untouched |
| `scripts/` | ✅ untouched |
| `internal/db/schema.go` | ✅ untouched |
| Scope of diff vs main | Only `internal/config/*` (impl + tests + testdata) — exactly the PR-1 boundary |

---

## 6. Task completion status (Phase 1)

- TASK-1.1 → TASK-1.6: **all `[x]`**, verified against actual files.
- TASK-1.7: 4/5 `[x]` (`go fmt`, `go vet`, `go build`, `go test -race` — all re-verified clean by this report). One line unchecked:
  - `- [ ] Commit: feat(config): add setup directory resolver, JSON loader, and business_hours mapping`

**Classification:** this is **not** an implementation-completeness failure. It is a parent-owned delivery action, explicitly deferred in `apply-progress.md` ("parent owns commits/review/delivery"). No unchecked implementation-code tasks remain in Phase 1. It does, trivially, block archive (as does every commit), and the overall change archive remains **not ready** regardless:

- Phase 2/3 tasks: 27 unchecked (`setup_seeder.go`, seeder/boot tests, `main.go` hook, docs, final pipeline) — **remaining scope**, correctly out of this verification's scope. The native status engine lists them as "unchecked blockers"; for PR-1 verification they are out-of-phase scope, not Phase-1 defects.

---

## 7. Findings

### WARNING — W1: duplicate-day pre-check contradicts design §8.3 #9 and breaks the planned Phase-2 rollback test vector

- **What:** `validateForSeed` in `setup_loader.go` adds a `seenDays` map rejecting duplicate `(professional, day_of_week)` schedules pre-transaction (`"el profesional %q tiene más de un horario para el día %d"`).
- **Why it matters:** design §8.3 row 9 and TASK-1.5 explicitly state duplicates are **deliberately NOT pre-checked** — the DB `UNIQUE(professional_id, day_of_week)` constraint is the authority, and the resulting mid-tx failure is the designated rollback-test vector for TASK-2.2(c). With the pre-check in place, that fixture will be rejected **before** `BeginTx`, so the transactional rollback path (ROLLBACK → pre-seed state) will never be exercised by the planned test — the test would silently pin pre-validation instead of transactional rollback.
- **Compounding:** `apply-progress.md` claims "Design Deviations: None" — inaccurate.
- **Disposition:** do **not** block PR 1 (the check is sound defense-in-depth on its own and tests pass). **Resolve in Phase 2 planning**: either (a) remove the pre-check to restore the DB-constraint-authority design, or (b) keep it and switch the TASK-2.2(c) rollback vector to a failure that passes pre-validation but violates a DB constraint (e.g., corrupt the `UNIQUE` index path or use a DB-level CHECK violation Go doesn't pre-check). Pick one before TASK-2.2 is written.

### SUGGESTION — S1: happy-path field assertions are a subset

The spec scenario "every struct field MUST be populated" is asserted on a representative subset. Consider asserting the remaining 11 optional business fields (or a golden-file comparison of the decoded struct) in a follow-up. Non-blocking.

### NOTE — N1: `target interface{}` in `LoadSetup`'s local file table

The spec forbids `any`/`interface{}` in **setup struct fields** (satisfied). The local `target interface{}` for `json.Unmarshal` is idiomatic and necessary; not a violation.

### NOTE — N2: helper signature drift from design pseudocode

`statusOr(status string)` and `stringSliceToJSON([]string) (*string, error)` differ cosmetically from design §8.2 pseudocode (`statusOr(m)`, `specialtiesJSON`). Semantically equivalent; no action.

---

## 8. Structured status & actionContext findings

- `artifactStore: openspec` (authoritative); status engine reports `nextRecommended: sdd-apply` — stale relative to the parent's Phase-1 completion; this report supersedes it for the Phase-1 boundary.
- `actionContext.mode: repo-local`, `allowedEditRoots` = workspace root — implementation ownership proven: all changed files are inside `internal/config/` under the workspace root. ✅
- `isNonAuthoritative: false`; no collisions, no same-domain active changes.

---

## 9. Exact blockers

**For PR-1 delivery:** none (proceed to RDD gate → commit/push/PR per chain).
**For change archive:** Phase-1 commit unchecked (parent action) + Phases 2–3 unimplemented (27 unchecked tasks) + W1 disposition decision. Archive stays blocked until all are resolved — no override.

---

## 10. Verdict

| Dimension | Result |
|-----------|--------|
| Spec coverage (5 REQs / 13 scenarios) | ✅ 13/13 (1 subset-level) |
| Task completion (Phase 1) | ✅ complete (commit deferred to parent, documented) |
| Tests | ✅ all pass with `-race`, `-count=1` |
| Strict TDD evidence | ✅ present and credible |
| Assertion quality | ✅ clean |
| Review workload / PR boundary | ✅ chain respected; overage 1033 vs 400 explicitly stated, handled by pre-approved chain |
| Out-of-scope changes | ✅ none |
| Design coherence | ⚠️ W1 (Phase-2-impacting, non-blocking for PR 1) |
