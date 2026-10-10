# Code hardening follow-ups (F1 data-minimization, F2 FTS errors, F3 validation dedup)

**Type**: refactor/fix (follow-ups of feat-whatsapp-bot, recorded 2026-10-08)
**Branch**: `feat/code-hardening-followups` (code → feature branch → PR)
**Status**: IN PROGRESS — tracking created 2026-10-10

## Scope

Three follow-ups selected by the owner from the feat-whatsapp-bot close-out list:

- **F1** — `FindClientByPhoneAny` data-minimization: narrow return shape (id + name only).
- **F2** — FTS invalid-input errors must reach the LLM as semantic `-32002`, not generic `-32603`.
- **F3** — deduplicate the E.164 15-digit cap (use case re-implementation vs entity validator); resolve the `internal/validation` stub follow-up with an explicit decision.

Out of scope: all other recorded follow-ups (message-prefix, JSON tags relocation, token echo, geo: for other tools, phone normalization).

## Decisions

- **D1 (F1)**: port `RegistrationLookup.FindClientByPhoneAny` returns narrow `repository.ClientIdentity{ID, Name string}` (type defined next to the port in `internal/domain/repository/registration.go`) instead of `*entity.Client`. Sole production caller (`get_or_create_client.go:113`) consumes only ID+Name; adapter SELECT drops `email, preferences, created_at, updated_at` (PII minimization). Method name and `ErrNotFound` contract unchanged. Side-effect: the `Active: true` hard-coded entity materialization disappears.
- **D2 (F2)**: `validateFTSQuery` (`internal/repository/validation.go:69-81`) returns `*domain.SemanticError{Code: ErrCodeInvalidInput, Message: <Spanish>, Cause: fmt.Errorf(..., domain.ErrInvalidInput)}`. `Unwrap()` keeps the `errors.Is(err, domain.ErrInvalidInput)` contract that existing tests assert; `errors.As` at `internal/mcp/errors.go:44-49` maps to `-32002` automatically. Precedent for SemanticError in repo adapters: `pending_alerts.go:173-176`. Messages stay Spanish, cleaned of sentinel text. Scope: only the four `validateFTSQuery` errors; the worker checks whether `ftsQueryRe` can still produce native SQLite `fts5: syntax error` paths (e.g. unbalanced quotes) and reports — if reachable, record a new follow-up, do not expand this change.
- **D3 (F3)**: `validRegistrationPhone` delegates to `entity.Client.HasValidPhone` (it already enforces the 4–15 digit cap — doc and implementation agree); delete `registrationMaxDigits`, `countDigits` and the stale comment at `get_or_create_client.go:16-17`. `internal/validation` stays doc-only: spec `data-access` forbids `internal/repository` importing it, no duplicated logic remains to promote, and forcing content would be premature abstraction (repo design note: "validators live next to their use until that package earns real content"). `doc.go` gets the recorded decision.

## Work units (≤ ~100 lines each — RDD relay defect #3991/#5226 band rule)

- [x] **WU1** — F1: `ClientIdentity` narrow port (domain port + repo adapters + use case + mocks/tests). Structural refactor, behavior-preserving (existing suite as safety net). **Done 2026-10-10** (worker mv2oa0j4-2-6hxm; 110 lines across 7 files — excess over ~100 band is mandatory cross-file test/mock signature adaptation, no extra behavior). Orchestrator diff review PASS: SELECT `id, name` only, ErrNotFound mapping byte-identical, legacy-row conflict untouched, domain imports nothing outer. Focused: build OK, 346 PASS / 0 FAIL (3 packages), vet + gofmt clean.
- [x] **WU2** — F2: SemanticError for the four FTS validation errors. Test-first RED→GREEN at three layers (repo / use case / mux). **Done 2026-10-10** (worker mv2of3pk-3-36fo; +177/−7 total — production net +21 in `validation.go`, excess is test-only with rationale: three-layer triangulation + mandated strengthening). RED observed (`code = -32603; want -32002`), GREEN: build OK, 1115 PASS / 0 FAIL, `-race` ok ×3, vet + gofmt clean. Deviation accepted by orchestrator: test-only `internal/repository` import in usecase tests (mirrors mcp integration convention; `no_repo_import_test.go` guard scans non-test sources only).
- [x] **WU3** — F3: delegate phone validation to entity; delete dup helpers + stale comment; `internal/validation/doc.go` decision note. **Done 2026-10-10** (worker mv2owpwo-4-r2dq; net production −13 lines, well under band). Orchestrator diff review PASS: `validRegistrationPhone` → single-sourced `entity.HasValidPhone`, `registrationMaxDigits`/`countDigits` deleted with the stale comment, `registrationMaxNameRunes` untouched, WU1 rename intact (lands in this commit per the file-level staging note), SemanticError message byte-identical. Boundary already pinned by existing subtest (16-digit rejected / 15-digit accepted through Execute). Focused: build OK, 16/16 usecase subtests, vet + gofmt clean, no dangling references.

### Recorded during this change (out of scope)

- **F4 (candidate follow-up, from WU2 scope-check)**: `ftsQueryRe` allowlist (`\p{L}\p{N}\s\-`) admits trailing-hyphen (`"geo-"` → native `fts5: syntax error`) and hyphen-before-space (`"a- b"` → `no such column`) shapes that pass validation and still surface as `-32603`/`-32002 internal`. Fix candidate: reject those two shapes in `validateFTSQuery`. Quotes/`*`/`+`/parens/`:` are allowlist-rejected, so no other native-syntax path is reachable. Worker probe evidence (real DB via mux, modernc.org/sqlite v1.58.0); probe file deleted after use.

## Gate & policy (owner decision: same as previous session)

- **RDD**: review switch is ON globally, but NO native review retry while the relay budget-starvation defect (#3991/#5226) is unfixed (canary lineage `review-afab2cbc89fa6de9` died `reviewer-empty-output` at ~224 lines). Fallback gate: independent verifier (structural readback) + GGA + CI + PR human review.
- **GGA**: known defect #113 (nondeterminism, code anchors, `--variant` broken). Do not rely on GGA alone: full pre-flight incl. `golangci-lint run ./...` before every push. GGA findings are fixed, cross-checked against the known-defect list; no `--no-verify` without owner approval.
- **Commits**: Conventional Commits, one work-unit commit per WU, tests with code. Owner approval before committing (AGENTS.md GGA rule).

## Evidence (scout 2026-10-10)

- Port `internal/domain/repository/registration.go:37-42`; SQL adapter `internal/repository/clients.go:249-272` (7 columns); delegating adapter `internal/repository/registration.go:43-45`; sole caller `internal/application/usecase/get_or_create_client.go:113,117,119`.
- FTS messages `internal/repository/validation.go:69-81` (all Spanish, `fmt.Errorf` + `ErrInvalidInput`); repo wraps `clients.go:462-463`, `services.go:154-155`; use case wraps `search_clients_advanced.go:30-32`, `search_services_advanced.go:33-35`; transport mapping `internal/mcp/errors.go:44-49` (only `*SemanticError` → `-32002`), fallback `-32603` `auth_translator.go:28`.
- `SemanticError{Code, Message, Cause}` with `Unwrap` — `internal/domain/errors.go`.
- Entity cap enforced: `internal/domain/entity/client.go:24-40`; dup in use case `get_or_create_client.go:14-19,174-192`.
- Tests: `registration_test.go:81`, `clients_getorcreatebyphone_test.go`, `get_or_create_client_test.go`, `mocks_test.go:217-231`, `validation_test.go:116-124`, `clients_test.go:499-511`, `services_test.go:659-671`, `tools_search_test.go:122-131`.
- Tool surface: `register_client` (F1, F3), `search_clients_advanced` / `search_services_advanced` (F2).

## Gate evidence (2026-10-10)

- Fallback gate PASS (verifier mv2ozbdl-5-rzxa): gofmt clean (238 tracked .go), vet clean, golangci-lint 2.14.0 → 0 issues, build OK, `go test -race ./...` 18 packages — 15 ok / 0 FAIL (fresh, uncached). Readback: D1/D2/D3 VERIFIED (diff pinned md5 12b7b658, 15 files +255/−82, no wire tags added, no SQL outside `internal/repository`, domain imports nothing outer). Caveats recorded: concurrent tracking-doc write during verification (all 15 code files predate the pipeline; diff hash stable — pipeline valid for code bytes); test-only `internal/repository` import in usecase tests accepted (no guard exists in that package, mirrors mcp integration convention).
- Commit plan on `feat/code-hardening-followups` (branch off main per AGENTS.md): C1 = WU1 files → `refactor(registration): narrow FindClientByPhoneAny to ClientIdentity`; C2 = WU2 files → `fix(search): surface FTS invalid input as semantic -32002 error`; C3 = WU3 files → `refactor(registration): single-source phone cap in entity validator`. GGA hook fires per commit (known defect #113 — findings get fixed, pre-flight repeated if needed). Tracking doc (odd/) closes on main at change close-out per docs flow.

## Delivery record (2026-10-10)

- **Branch + commits (all GGA PASSED)**: C1 `572cd3f` (WU1 narrow port, 6 files) → C2 `f8bf5d2` (WU2 semantic errors + F4 hyphen boundary, 7 files) → C3 `dfc70ce` (WU3 dedup + doc.go rewrite, 2 files).
- **GGA rounds**: C2 initially BLOCKED with 5 findings on validation.go (validateExceptionDate asymmetry = same defect class as F2; term-boundary hyphens = F4 escalated from follow-up to must-fix; false file-header claim; 2 contradicting comments; dead '+' branch) — all fixed by worker (mv2pd4k9-6-uhh9 surfaces: validation.go + validation_test.go only), pre-flight re-run green, PASSED. C3 blocked twice by ambiguous verdicts (no findings, empty provider output — GGA #113 nondeterminism); root cause was a poisoned cache entry, resolved with `gga cache clear`, PASSED.
- **doc.go rewrite** (C3): GGA correctly flagged architecture-doc inaccuracy — the doc promised a fictional validator API (ValidateEmail/Latitude/...), nonexistent consumers (tui/mcp), and the wrong SemanticError location (repository/errors.go instead of domain/errors.go). Rewritten factual; decision note leads.
- **PR**: https://github.com/egkike/mcp-appointments-crm/pull/113 (no repo PR template; body follows AGENTS.md PR requirements).
- **New follow-ups recorded**: (a) MaxFTSQueryLen mirror in internal/mcp kept in sync only by test — both relocation directions spec-forbidden (data-access forbids repo→validation import; REQ-MT-012 forbids mcp→repository), needs a design decision; (b) BusinessHoursExceptionRepo.Create has no use-case caller — wire through a use case mapping ErrInvalidInput before exposing to tools; (c) GGA C2 non-blocking suggestions: table-driven test asserting each rejected FTS shape (regex drift lock), one-line note if a log sink ever forwards Cause verbatim.
