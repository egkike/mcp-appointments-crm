# Archive Report — feat-mcp-transport

**Change**: feat-mcp-transport
**Archived**: 2026-08-19
**Target commit**: `bb86228` on `main`
**Merge refs**: PR #46 (`98d7be3`, PR 1/2 — transport skeleton) + PR #47 (`bb86228`, PR 2/2 — auth + 6 tools + e2e); both merged onto `main`, CI green, GGA passed per commit, JD (2 blind judges) approved @ `1367e7f` (rounds 1–3; findings `faf431a`/`5eac130`/`81afdc5`/`caa9854`/`1367e7f`)
**Verify evidence_revision**: `sha256:7df921a057388f77de02877565456698798979d3f5242ee558fa3eb3cce548ab`
**Status**: CLOSED — SDD cycle complete

## Summary

Change-final verification **PASS** (canonical report `verify-report.md`, supersedes interim `verify-pr1-report.md`/`verify-pr2-report.md` as the final evidence): **24/24 requirements, 34/34 scenarios COMPLIANT**, 0 CRITICAL, 0 WARNING, validator `gentle-ai sdd-verify-validate` verdict pass.

- **First pass FAIL (W-1)**: spec templates in REQ-MT-009/016 described aspirational domain messages not present in production → spec amended 2026-08-19 (maintainer-authorized, precedent `faf431a`) to the real pre-existing domain messages (present at base `0d9628e`). Re-verify PASS against amended text, with emit-site + test-assertion evidence. Amendment is docs-only; the verified contract (verbatim passthrough of `*domain.SemanticError.Message` as `-32002`) is unchanged.
- **Tests**: 276 top-level / 915 with subtests, 0 fail, 0 skip, 0 race, 10/10 packages; changed-file coverage 89.7%; gofmt/vet/build/golangci-lint/govulncheck all clean.
- **Scope**: only new direct dependency `github.com/modelcontextprotocol/go-sdk v1.4.1` + module graph; toolchain `go 1.26.6`. No scope drift in production code; `internal/repository/` untouched.

## Task Completion

| Metric | Value |
|--------|-------|
| Tasks total | 11 (T-01..T-11) |
| Tasks complete | 11 (`[x]` in `tasks.md`) |
| Tasks incomplete | 0 |
| Pre-flight checklist | complete (`[x]` per task and final) |

## Deliverables

- **PRs**: 2/2 merged (`98d7be3` #46, `bb86228` #47), CI green both, GGA passed per commit, JD approved.
- **Specs synced to baseline** (handoff state):
  - `openspec/specs/architecture/spec.md` — 4 ADDED requirements (REQ-ARCH-INTMCP-001..004) appended under new `## Requirements` section; pre-existing C1–C5 contracts preserved.
  - `openspec/specs/auth-middleware/spec.md` — 4 ADDED requirements (REQ-AM-WIRED-001..004) appended to the `## Requirements` section with provenance note; pre-existing requirements preserved.
  - `openspec/specs/mcp-transport/spec.md` — NEW domain baseline created; delta spec (full spec) copied verbatim via mechanical shell copy with empty `diff -r` readback. Amended REQ-MT-009/016 templates and amended REQ-MT-015 travel verbatim into the baseline.
- **No MODIFIED/DEPRECATED/REMOVED requirements** in any change spec — purely additive merge, no destructive delta (per `openspec/config.yaml` archive rule).

## Artifacts Archived (moved via `git mv`, readback `diff -r` empty)

- `proposal.md` ✅
- `specs/architecture/spec.md`, `specs/auth-middleware/spec.md`, `specs/mcp-transport/spec.md` ✅
- `design.md` ✅
- `tasks.md` ✅ (11/11 tasks complete, no unchecked implementation tasks)
- `apply-progress.md` ✅
- `verify-report.md` ✅ (canonical final evidence)
- `verify-pr1-report.md`, `verify-pr2-report.md` ✅ (interim snapshots, superseded by canonical report)

## Follow-ups (SUGGESTION-level, non-blocking, recorded at close)

- **S-1** — 403-denied requests log `caller_role=none` although the caller was resolved before the RBAC gate: annotate the role before the RBAC gate (`internal/auth/middleware.go:96-117`). REQ-MT-011 not violated; improvement only.
- **S-2** — `statusRecorder.Flush()` 0% coverage: SSE path unreachable with `JSONResponse:true`.
- **S-3** — `slot_context.go` 69.6% coverage: error branches covered indirectly; add direct table cases if the resolver grows.
- **S-4** — carried note: `check_availability_test.go:223` mock fixture still carries the old aspirational overlap string (mock input, not a production assertion; optional cleanup).

## Handoff Notes for Orchestrator

- Working-tree state at archive time: `specs/mcp-transport/spec.md` and `tasks.md` amendments (W-1 fix) were uncommitted on `main`; `verify-report.md` was untracked. Both now travel in the archived change folder, and the baseline sync (`openspec/specs/architecture/spec.md`, `openspec/specs/auth-middleware/spec.md`, `openspec/specs/mcp-transport/spec.md`) is new working-tree content. Orchestrator commits per repository doc workflow (docs → `main` direct).
- No production code modified by this archive phase.

## Traceability

Artifacts read from the filesystem during this phase (all paths under `openspec/changes/feat-mcp-transport/` pre-move, under `openspec/changes/archive/2026-08-19-feat-mcp-transport/` post-move): `proposal.md`, `design.md`, `tasks.md`, `apply-progress.md`, `verify-report.md`, `verify-pr1-report.md`, `verify-pr2-report.md`, `specs/{architecture,auth-middleware,mcp-transport}/spec.md`, plus baseline `openspec/specs/{architecture,auth-middleware}/spec.md` and `openspec/config.yaml`.