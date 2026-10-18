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

- [ ] **RED:** `TestWriteSkillFileConsent_FileReplacedMidFlowAborts` — external rename-replace between fd-open and the guard aborts; the replacement bytes survive.
- [ ] **GREEN:** inode guard inside `WriteSkillFileConsent` (path-stat vs fd-inode; zero-inode skip).
- [ ] (b) attribution copy in `confirmSkillRegenWarning` + console parity check.
- [ ] Full pipeline green; GGA r1 PASSED deterministically (no findings).
