# Sync Report — feat-setup-import

> **Change:** `feat-setup-import`
> **Status:** **synced** — canonical specs updated, RD-7 superseded
> **Date:** 2026-09-11
> **Attempt token:** `sha256:ee05c215cf4a1a1b26cf19c273f8fc9350e27d03223c5a0b121ec02501fd1dd` · **Request ID:** `req-sync-20260911-01` · **Work-unit:** `sync-canonical-specs`
> **skill_resolution:** `paths-injected` (`cognitive-doc-design`)
> **artifactStore:** `openspec` · **workspaceRoot:** `/home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm`
> **Verify prerequisite:** `gentle-ai.verify-result/v1` · `verdict: pass` · `12/12 requirements, 29/29 scenarios` · `build OK` · `go test -race OK`

Sync promoted the two `feat-setup-import` delta specs into `openspec/specs/` and resolved the **RD-7 canonical drift** — `business-profile` now pins the numeric-string `business_hours` contract (`"1"`..`"7"`, missing = closed) that the shipped `BusinessProfile` entity and seeder actually use. No re-seed or destructive delta required; next step is `sdd-archive`.

## Quick path

1. **Verify canonicals:** `ls openspec/specs/setup-loader/spec.md openspec/specs/setup-seeder/spec.md openspec/specs/business-profile/spec.md`
2. **Confirm RD-7 fix:** `grep -n '\"1\"' openspec/specs/business-profile/spec.md` — numeric-string contract present, inline `Amended by feat-setup-import` notice present
3. **Proceed to archive:** `sdd-archive` — change is `all_done` + `verify clean` + `sync done`, no unchecked tasks, no same-domain collisions

## Details

| Topic | Decision |
|-------|----------|
| **Sync mode** | `openspec` filesystem sync — no `engram` persistence; `sync-report.md` written to change dir per `openspec/specs` convention |
| **Delta → canonical mapping** | Archive layout `specs/{domain}/spec.md` maps 1:1 to `openspec/specs/{domain}/spec.md` (verified against `openspec/changes/archive/2026-07-29-feat-db-layer`) |
| **New capabilities** | `setup-loader` and `setup-seeder` had no prior canonical spec — copied verbatim (byte-identical to delta) |
| **RD-7 conflict** | `business-profile` `Weekly schedule stored as JSON` pinned day-name keys + literal `null`; shipped code uses numeric-string `"1"`..`"7"` with absent = closed — **superseded in-place** with precedence note, not silently overwritten |
| **Destructive risk** | No `REMOVED` requirements; one `MODIFIED` with in-place amendment — no large destructive delta requiring explicit approval beyond this report |
| **Collisions** | `sameDomainActiveChanges: []` — zero active same-domain changes (native status) |
| **Verification gate** | `verify-report.md` envelope `gentle-ai.verify-result/v1` with `verdict: pass`, `blockers: 0`, `critical_findings: 0` — sync blocker cleared |

## Domains synced

| Domain | Delta source | Canonical target | Action |
|--------|-------------|------------------|--------|
| `setup-loader` | `openspec/changes/feat-setup-import/specs/setup-loader/spec.md` | `openspec/specs/setup-loader/spec.md` | **ADDED** (NEW capability, no prior spec) |
| `setup-seeder` | `openspec/changes/feat-setup-import/specs/setup-seeder/spec.md` | `openspec/specs/setup-seeder/spec.md` | **ADDED** (NEW capability, no prior spec) |
| `business-profile` | — (RD-7 conflict) | `openspec/specs/business-profile/spec.md` | **MODIFIED** (superseded Weekly schedule requirement) |

## Canonical files updated

| File | Action | SHA256 (post-sync) |
|------|--------|--------------------|
| `openspec/specs/setup-loader/spec.md` | Created | `8d7ecd3259914aea0ffed30cdd6c7013007ef5e31885957ad170216f68bb0fe5` |
| `openspec/specs/setup-seeder/spec.md` | Created | `62f5983d3454d4311d7bbd960be9f5507cb968b0f0f4af2ee41a8d1cca7d663d` |
| `openspec/specs/business-profile/spec.md` | Modified (in-place amendment) | `5002a4df35143f070659d1344af0cabc07fa01dd2df607e9e553192892b92416` |

Byte-identity checks:
```
diff delta → canonical: setup-loader identical, setup-seeder identical
```

## Requirement delta

### `setup-loader` — ADDED (5 requirements, 13 scenarios)

| Requirement | Scenarios |
|-------------|-----------|
| Per-OS setup directory resolution | 4 |
| Typed structs pinning the three wizard JSON shapes | 1 |
| Loader reports missing and malformed files with semantic Spanish errors | 3 |
| business_hours day-name to numeric-string mapping | 4 |
| Setup files are read-only to the server | 1 |

No `MODIFIED` or `REMOVED` — capability is net-new.

### `setup-seeder` — ADDED (7 requirements, 16 scenarios)

| Requirement | Scenarios |
|-------------|-----------|
| Fresh guard on business_profile.name | 3 |
| Single atomic transaction | 2 |
| Direct parameterized SQL, not repositories | 3 |
| business_profile singleton upsert with all wizard fields | 2 |
| Boot-time failure policy | 3 |
| Boot wiring in the composition root | 2 |
| First-boot seeding is documented | 1 |

No `MODIFIED` or `REMOVED` — capability is net-new.

### `business-profile` — MODIFIED (1 requirement amended, 0 added, 0 removed)

| Requirement | Change | Detail |
|-------------|--------|--------|
| **Weekly schedule stored as JSON** | **MODIFIED** | Previous: `monday`..`sunday` keys, `null` = closed. New: numeric-string `"1"`..`"7"` (`"1"`=Monday..`"7"`=Sunday), missing key = closed. Scenarios updated accordingly: `Valid weekly schedule parses without error` now uses `"1"`..`"6"` + absent `"7"`; `Closed day represented as null` replaced by `Closed day is absent (not null)`. Inline blockquote `Amended by feat-setup-import (2026-09-11)` records precedence and deprecates old contract; notes backward-compat guidance for readers. |

All other `business-profile` requirements unchanged (Singleton row constraint, Lazy-init semantics, Accepted payment methods, Default configuration values, Messenger fields, CHECK constraint, Fresh install with empty business_hours rejects all bookings).

## Critical known conflict — RD-7

| Item | Handling |
|------|----------|
| **Conflict** | `openspec/specs/business-profile/spec.md` `Weekly schedule stored as JSON` contradicted shipped behavior: entity `BusinessProfile.parseBusinessHours` parses numeric-string keys `"1"`..`"7"` (`internal/domain/entity/business_profile.go:18,53-74`: `BusinessHours string // JSON: {"1":{"open":"09:00","close":"18:00"},...}`), seeder writes mapped numeric-string via `setup-loader` mapping (`monday→"1"`..`sunday→"7"`), but canonical still required day-name + `null` |
| **Resolution** | **In-place supersession** — canonical requirement updated to numeric-string contract; inline `> **Amended by feat-setup-import (2026-09-11) — supersedes previous day-name/null contract.**` added with precedence statement. Previous `null` contract deprecated, not deleted silently. Alternative (follow-up amendment file) was not needed because canonical format permits in-place amendment |
| **Why not a delta file** | Task fallback (`if format forbids in-place amendment, record delta as follow-up amendment file`) does not apply — format allows direct edits; verified against archive precedents where canonical specs are edited directly |
| **Follow-up** | No follow-up amendment file created; sync report itself is the audit trail for the supersession |

## Active same-domain collisions

- **None.** Native status: `sameDomainActiveChanges: []`, `collisions: []`, `relationships.conflictsWith: []`
- No other active change touches `setup-loader`, `setup-seeder`, or `business-profile` — no archive/sync ordering decision required

## Destructive sync approvals / blockers

- **REMOVED requirements:** 0 — no destructive removals
- **Large MODIFIED blocks:** 1 (business-profile Weekly schedule — ~12 lines replaced) — below destructive threshold; explicit approval is this sync execution under the task's `Attempt token` / `Request ID`
- **Destructive delta guard:** `lib/openspec-guardrails.ts` equivalent — no `REMOVED` → no parent-approval blocker; RD-7 modification is intentional and documented above
- **RENAMED Requirements:** 0 — no `## RENAMED Requirements` sections in deltas (unsupported path not triggered)

## Validation commands / checks performed

| Command | Result |
|---------|--------|
| `verify-report.md` envelope check | `schema: gentle-ai.verify-result/v1`, `verdict: pass`, `blockers: 0`, `critical_findings: 0`, `requirements: 12/12`, `scenarios: 29/29`, `test_command: go test -race`, `build_command: go build` — clean |
| `ls openspec/specs/setup-loader setup-seeder` | Both canonical dirs/files present post-copy |
| `diff delta → canonical` | `setup-loader` identical, `setup-seeder` identical |
| `grep business-profile` | `Weekly schedule` now contains `"1"`..`"7"` and `missing key means closed`; `Amended by feat-setup-import` present |
| `go vet ./...` | OK (exit 0) |
| `go build -o /dev/null ./...` | OK (exit 0) |
| `git status` | Untracked `sync-report.md` + new `openspec/specs/setup-loader/spec.md`, `setup-seeder/spec.md`, modified `business-profile/spec.md` — no commits made (parent owns delivery) |
| `actionContext` | `mode: repo-local`, `workspaceRoot` inside `allowedEditRoots` — edit roots respected |
| `isNonAuthoritative` | `false` — status is authoritative (not `resolve-via-engram` carve-out) |

## Structured status & actionContext findings

```json
{
  "changeName": "feat-setup-import",
  "artifactStore": "openspec",
  "taskProgress": "51/51 complete",
  "applyState": "all_done",
  "dependencies": { "apply": "all_done", "verify": "ready", "sync": "blocked→resolved", "archive": "blocked→ready-after-sync" },
  "nextRecommendedBeforeSync": "sdd-verify",
  "nextRecommendedAfterSync": "sdd-archive",
  "blockedReasons": [],
  "isNonAuthoritative": false,
  "actionContext": {
    "mode": "repo-local",
    "workspaceRoot": "/home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm",
    "allowedEditRoots": ["/home/kike/Documentos/Kike/Desarrollos_Software/Proyectos/Mcps/mcp-appointments-crm"],
    "warnings": []
  }
}
```

- Sync instructions from status: two delta specs (`setup-loader`, `setup-seeder`) — fulfilled
- Archive prerequisites after sync: clean verify (satisfied), completed sync (this report), zero unchecked tasks (51/51) — all satisfied

## Checklist

- [x] `openspec/specs/setup-loader/spec.md` created (5 REQs, 13 scenarios)
- [x] `openspec/specs/setup-seeder/spec.md` created (7 REQs, 16 scenarios)
- [x] `openspec/specs/business-profile/spec.md` Weekly schedule amended to numeric-string contract with explicit supersession note
- [x] No canonical spec contradicts shipped code after sync
- [x] No `REMOVED`/`RENAMED` deltas left unhandled
- [x] No active same-domain collisions
- [x] Validation commands green (vet, build, diff, grep)
- [x] Not committed — parent owns delivery
- [x] Next recommended: `sdd-archive` to `openspec/changes/archive/YYYY-MM-DD-feat-setup-import`

## Next step

Run `sdd-archive` — move `openspec/changes/feat-setup-import/` to `openspec/changes/archive/YYYY-MM-DD-feat-setup-import/` (zero unchecked tasks, verify PASS, sync done). No further spec edits required.

---
*Generated by sdd-sync executor · attempt `req-sync-20260911-01` · `cognitive-doc-design` shape: lead with answer, progressive disclosure, signposted sections*
