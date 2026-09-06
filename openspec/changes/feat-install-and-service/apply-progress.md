# Apply Progress — feat-install-and-service

**Change:** feat-install-and-service  
**Phase:** 5 — apply (PR1 / T1)  
**Status:** PR1 T1 complete; remaining work delegated to PR2–PR4  
**Executor:** sdd-apply  
**Updated:** 2026-09-05

---

## Structured Status Consumed

| Field | Value |
|-------|-------|
| changeName | feat-install-and-service |
| artifactStore | openspec |
| applyState | ready (PR1 T1) |
| actionContext.mode | repo-local |
| allowedEditRoots | `cmd/mcp-server/**`, `internal/buildinfo/**` |
| delivery_strategy | ask-on-risk (approved: 4 chained PRs stacked-to-main, PR3 size:exception) |
| chain_strategy | stacked-to-main |
| review_budget | 400 |

---

## Completed Tasks

- [x] T1.1 RED: crear `cmd/mcp-server/main_test.go` con tests para `--version`
- [x] T1.2 GREEN: implementar guard `--version` en `cmd/mcp-server/main.go`
- [x] T1.3 VERIFY: `go fmt ./... && go vet ./... && go build -o /dev/null ./... && go test -v -race ./...`

---

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `cmd/mcp-server/main_test.go` | New test file | ~95 |
| `cmd/mcp-server/main.go` | Extend `main()` with `--version` guard + pure helpers | ~20 insertions |

**Total changed lines (PR1):** ~115 additions — within 400-line budget.

---

## TDD Cycle Evidence

Strict TDD active (`openspec/config.yaml` → `rules.apply.tdd: true`, `test_command: go test -v -race ./...`).

| Task | Test File | Layer | Safety Net | RED | GREEN | TRIANGULATE | REFACTOR |
|------|-----------|-------|------------|-----|-------|-------------|----------|
| T1.1 | `cmd/mcp-server/main_test.go` | Unit + Exec | N/A (no prior tests in package) | ✅ Written (undefined `wantsVersion`/`printVersion`) | ✅ Passed | ✅ 4 cases for `wantsVersion` + 1 output test + 1 exec-based binary test | ✅ Extracted pure `wantsVersion` and `printVersion`; `main()` delegates to `os.Stdout` |

### Test Summary
- **Total tests written:** 3 top-level tests (6 subtests + 1 exec test)
- **Total tests passing:** 6/6 unit subtests + 1/1 exec test
- **Layers used:** Unit (6), Exec/E2E binary (1)
- **Approval tests:** None — no refactoring tasks
- **Pure functions created:** 2 (`wantsVersion`, `printVersion`)

---

## Verification Commands Run

```bash
# Safety net / focused package test
go test -v -race ./cmd/mcp-server/...

# Full quality gates (T1.3)
go fmt ./... && go vet ./... && go build -o /dev/null ./... && go test -v -race ./...
```

**Result:** All green. No `gofmt` diff, no `go vet` warnings, full test suite passes (including existing repository tests).

---

## Design Deviations

None. Implementation matches design §5.4 and task spec exactly:
- `--version` guard runs **before** `run()` — no DB/network side effects.
- No `flag` package introduced.
- Zero changes to `internal/mcp/config.go`, `internal/db/`, or DB defaults.
- `buildinfo` was already imported; no new import needed for the package, only `io` for testability.

---

## Remaining Tasks

PR2–PR4 remain unchecked and are out of scope for this apply execution:

- [ ] T2.1 Create systemd user unit template
- [ ] T2.2 Create launchd plist template
- [ ] T2.3 Create NSSM placeholder document
- [ ] T2.4 VERIFY: templates exist and match contracts
- [ ] T3.1 RED: write `scripts/tests/backup_test.sh`
- [ ] T3.2 GREEN: implement `scripts/backup.sh`
- [ ] T3.3 VERIFY: backup tests + existing suites unmodified
- [ ] T4.1 RED: write `scripts/tests/install_deploy_test.sh` (core functions)
- [ ] T4.2 GREEN: implement core deploy functions in `scripts/install.sh`
- [ ] T4.3 VERIFY: core deploy tests + no regression
- [ ] T5.1 RED: extend `install_deploy_test.sh` (service + verify + summary)
- [ ] T5.2 GREEN: implement service registration, verification, summary
- [ ] T5.3 VERIFY: all suites green, no regression
- [ ] T6.1 Write `docs/installation.md`
- [ ] T6.2 Write `docs/maintenance.md`
- [ ] T6.3 Align `docs/deployment.md` and `docs/PRD.md`
- [ ] T7.1 Go quality gates (final)
- [ ] T7.2 Shell test suites
- [ ] T7.3 File inventory check
- [ ] T7.4 Manual VM checklist

---

## Workload / PR Boundary

- **PR1 scope:** T1 only (`feat/binary-version-flag`).
- **Changed lines:** ~115 — under 400-line budget.
- **Chain continuation:** PR2 (`feat/service-templates-and-backup`) can be started once this PR is merged/rebased onto main.
- **No commit performed** (per rules — only explicit user request).

---

## Risks

- **None identified for PR1.** The change is isolated, well-tested, and introduces no behavioral change when `--version` is absent.
- **Chain risk:** PR3 is approved as `size:exception` (~660 LOC). PR2 and PR4 are within budget.

---

## Notes

- `cmd/mcp-server/main_test.go` includes an exec-based test (`TestBinaryVersionFlag`) that builds the binary and runs `--version` to confirm end-to-end behavior matches `buildinfo.Version` without starting a listener.
- The `--version` output is the bare version string (`dev` or release tag), per design §5.4 decision to defer richer format to Fase 6.
