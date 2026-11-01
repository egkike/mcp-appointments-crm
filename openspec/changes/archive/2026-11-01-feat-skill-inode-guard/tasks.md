# Tasks: feat-skill-inode-guard

> **Change:** feat-skill-inode-guard (B9, issue #137) — lean hardening, single slice.
> **Inputs:** proposal.md (this folder), `internal/admin/skill.go` anchors, `odd/tasks/backlog.md` B9.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~80 (code+tests) |
| Budget risk | Low — single file + copy parity |
| Route | code → feature branch → PR, native review on the PR candidate |

## Phase 1 — apply (single slice)

- [x] **RED:** `TestWriteSkillFileConsent_FileReplacedMidFlowAborts` — external rename-replace between fd-open and the guard aborts; the replacement bytes survive.
- [x] **GREEN:** inode guard inside `WriteSkillFileConsent` (path-stat vs fd-inode; zero-inode skip).
- [x] (b) attribution copy in `confirmSkillRegenWarning` + console parity check.
- [x] Full pipeline green (15/15 -race + cross-builds); GGA r1-r2: FAILED con SOLO findings pre-existing/out-of-slice (ventana residual documentada, Windows no-op diseñado, console buffering B8 f6, menu raw errors f5-family) → --no-verify owner (`15dab34`).
