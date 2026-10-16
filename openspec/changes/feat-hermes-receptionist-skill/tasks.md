# Tasks: feat-hermes-receptionist-skill

> **Change:** feat-hermes-receptionist-skill
> **Status:** tasks drafted (spec phase) — doc-first change; implementation follows in an apply change
> **Inputs:** `proposal.md`, `design.md`, `specs/hermes-receptionist-skill/spec.md`, `odd/tasks/feat-hermes-receptionist-skill.md`
> **Skill type:** non-LLM-reviewed? No — this change modifies **no Go code paths**; it ships skill template + install script block + TUI entry in the apply change. This tasks file covers only the doc-first deliverables. Implementation units land in a follow-up change after spec review.

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines (docs) | ~700 (ADR 130 + change docs 420 + gateway pointer 15 + odd tracking 40) |
| 400-line budget risk | Medium-high (golden threshold is for review-heavy code PRs; doc PRs stay under it in 2 commits) |
| Chained PRs recommended | **Yes** (docs flow, branch → review → ff-only merge) |
| Suggested split | C1: odd tracking + ADR-0020 · C2: openspec change folder + gateway-contract pointer |
| Delivery strategy | ask-on-commit; no native review needed (docs lane, structural readback) |

---

## Phase 1 — Architectural record (C1)

### TASK-1.1 — ODD feature document

- [x] `odd/tasks/feat-hermes-receptionist-skill.md` with S1–S3 (user strings verbatim), T1–T4, and Log entries

### TASK-1.2 — ADR-0020

- [x] `docs/architecture/0020-hermes-conversational-policy.md` with Context (3 baselines), 5 Decisions, Consequences, Follow-ups
- [x] Style matched to sibling ADRs (Spanish, Status/Date/Authors/Related header)

## Phase 2 — Openspec change (C2)

### TASK-2.1 — Proposal

- [x] `openspec/changes/feat-hermes-receptionist-skill/proposal.md` (summary, business problem, product outcome, scope, non-goals, apply-reconciliation assumptions)

### TASK-2.2 — Design

- [x] `openspec/changes/feat-hermes-receptionist-skill/design.md` (layering contract unchanged, install model, runtime-consumption assumption, skill design per ADR-0020 Decisions 2–5, template draft appendix, TUI option design, testing approach, security review, rollout/rollback)

### TASK-2.3 — Spec

- [x] `openspec/changes/feat-hermes-receptionist-skill/specs/hermes-receptionist-skill/spec.md` (7 requirements with scenarios in the repo's GIVEN/WHEN/THEN shape)

### TASK-2.4 — Gateway contract pointer

- [x] New §8 in `docs/gateway-contract.md` referencing the receptionist skill, its bounds, ADR-0020, and the official skill standard

## Phase 3 — Verification (docs lane)

### TASK-3.1 — Structural readback

- [ ] Read back ADR-0020 + change folder in full; cross-check every S# from `odd/tasks/feat-hermes-receptionist-skill.md` is covered by at least one artifact
- [ ] Verify no Go/server behavior is mentioned as changed (search diff for `internal/`, `cmd/`, "tool" claims)
- [ ] Cross-link check: ADR-0020 ↔ gateway-contract.md §8 ↔ change folder (bidirectional or one-way-ref consistent with sibling docs)

### TASK-3.2 — Per-AGENTS verification & gates

- [ ] Docs lane: no `go build`/`go test` required (no code paths changed); confirm with `git diff --stat` that only `.md` files changed
- [ ] Full pipeline run still green on `main` baseline before branch merge (per the "always ask before commit" rule with the owner)

---

## Owner review round 1 (2026-10-16) — resolved

1. ~~Skill name / directory~~ → **RESOLVED**: verified against the official Hermes docs (`hermes-agent.nousresearch.com/docs/user-guide/features/skills`); storage `~/.hermes/skills/<slug>/SKILL.md`, frontmatter name/description/version, house sections When to Use → Procedure → Pitfalls → Verification, description ≤60 chars. Slug: `recepcionista-negocio`; progressive-disclosure nuance incorporated (design §1).
2. ~~TUI option label~~ → **RESOLVED (owner)**: second step inside option 7 "Configurar Hermes", no new menu item.
3. ~~Tone verbatim check~~ → **RESOLVED (owner)**: "Cordial y orientador" confirmed for the template's canonical redirect.
