# Archive Report: feat-repository-auth-integration

> Archived: 2026-08-23
> Archive path: `openspec/changes/archive/2026-08-23-feat-repository-auth-integration/`

## Summary

Repository-layer auth wiring for `clients` (8 methods) and `business_hours_exception` (4 methods). The last auth debt (PRD §3.8.7 item 6) — any reachable caller could leak PII via `SearchFTS`/`FindByPhone` or DoS bookings by planting closing exceptions. This change enforces the 3-layer authorization model (PRD §3.8.4) at the repository layer.

## Final State

- **Branch**: `feat/feat-repository-auth-integration-apply` rebased onto `main` (Go 1.26.7)
- **Fix commit**: `2c270d6` — addresses all 5 JD CRITICALs (GetOrCreate phone anchor changed to `caller.ID`, helper hardened to suffix param, FTS operator check, comments)
- **Verification**: PASS — 7/7 requirements, 22/22 scenarios, 88.5% coverage on `internal/repository`
- **Build & Tests**: `go build`, `go vet`, `golangci-lint`, `go test -v -race` all pass (294 tests)
- **JD Round 2**: Both judges PASS (findings: [])

## Task Completion

| Phase | Tasks | Status |
|-------|-------|--------|
| Foundation | T-01 | ✅ |
| Clients wiring | T-02–T-05 | ✅ |
| BHE wiring | T-06–T-07 | ✅ |
| Test migration | T-08–T-09 | ✅ |
| Verification | T-10 | ✅ |

**10/10 tasks complete. No unchecked implementation tasks.**

## Specs Synced

| Domain | Action | Requirements Added |
|--------|--------|-------------------|
| `clients` | ADDED | REQ-CL-AUTH-001 through REQ-CL-AUTH-005 (5 requirements, 15 scenarios) |
| `business-hours-exception` | ADDED | REQ-BHE-AUTH-001 through REQ-BHE-AUTH-002 (2 requirements, 7 scenarios) |

Main specs updated:
- `openspec/specs/clients/spec.md` — 5 new auth requirements appended
- `openspec/specs/business-hours-exception/spec.md` — 2 new auth requirements appended

## Archive Contents

- `proposal.md` ✅
- `explore.md` ✅
- `specs/clients/spec.md` ✅
- `specs/business-hours-exception/spec.md` ✅
- `design.md` ✅
- `tasks.md` ✅
- `verify-report.md` ✅
- `apply-progress.md` ✅

## Design Decisions

| Decision | Status | Notes |
|----------|--------|-------|
| D1: Local helper in `clients.go`, not `auth_filter.go` | ✅ Shipped | `auth_filter.go` untouched |
| D2: Staff/unknown → fail-fast `ErrForbidden` | ✅ Shipped | No `AND 1=0` |
| D3: Writes gated by `RequireRole` only | ✅ Shipped | No scope column on UPDATE/DELETE |
| D4: BHE reads = `RequireCaller` presence only | ✅ Shipped | Hot path preserved |
| D5: GetOrCreate own-phone anchor | ✅ Shipped | Anchor is `caller.ID` (fix commit 2c270d6) |
| D6: Auth gate ordered before input validation | ✅ Shipped | First statement in every gated method |

## Known Follow-up

**SUGGESTION (non-blocking)**: Design Decision 5 prose in `design.md` still says anchor is `*caller.ClientID` while code uses `caller.ID`. Code is correct per spec and tests; design.md prose is stale. A small doc-follow-up is recommended.

## Verification Readback

```
diff -r <pre-move snapshot> openspec/changes/archive/2026-08-23-feat-repository-auth-integration/
# Result: empty (byte-identity confirmed)
```

## Engram Observation IDs

| Artifact | Observation ID | Topic Key |
|----------|---------------|-----------|
| proposal | (openspec file) | — |
| specs | (openspec files) | — |
| design | (openspec file) | — |
| tasks | (openspec file) | — |
| verify-report | (openspec file) | — |

## SDD Cycle Complete

The change has been fully planned, implemented, verified, and archived. Ready for the next change.
