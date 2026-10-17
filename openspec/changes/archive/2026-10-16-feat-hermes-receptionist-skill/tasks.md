# Tasks: feat-hermes-receptionist-skill

> **Change:** feat-hermes-receptionist-skill
> **Status:** CLOSED 2026-10-17 — all tasks verified against delivery evidence; smoke manual transferred to backlog B2 (`odd/tasks/backlog.md`). Archived as `openspec/changes/archive/2026-10-16-feat-hermes-receptionist-skill/`. Original draft status below. | tasks drafted (spec phase) — doc-first change; implementation follows in an apply change
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

- [x] Read back ADR-0020 + change folder in full; cross-check every S# from `odd/tasks/feat-hermes-receptionist-skill.md` is covered by at least one artifact — **done 2026-10-16 pre-merge** (feat doc Log L5)
- [x] Verify no Go/server behavior is mentioned as changed (search diff for `internal/`, `cmd/`, "tool" claims) — docs-only diff confirmed (feat doc Log L9)
- [x] Cross-link check: ADR-0020 ↔ gateway-contract.md §8 ↔ change folder (bidirectional or one-way-ref consistent with sibling docs) — done 2026-10-16 (ADR indexed in docs/architecture/README.md, feat doc Log L4)

### TASK-3.2 — Per-AGENTS verification & gates

- [x] Docs lane: no `go build`/`go test` required (no code paths changed); confirm with `git diff --stat` that only `.md` files changed — confirmed on commits 5912e18/0c1f888 (feat doc Log L9)
- [x] Full pipeline run still green on `main` baseline before branch merge (per the "always ask before commit" rule with the owner) — green baseline; merge 5912e18→badb8a9 ff-only 2026-10-16

---

## Owner review round 1 (2026-10-16) — resolved

1. ~~Skill name / directory~~ → **RESOLVED**: verified against the official Hermes docs (`hermes-agent.nousresearch.com/docs/user-guide/features/skills`); storage `~/.hermes/skills/<slug>/SKILL.md`, frontmatter name/description/version, house sections When to Use → Procedure → Pitfalls → Verification, description ≤60 chars. Slug: `recepcionista-negocio`; progressive-disclosure nuance incorporated (design §1).
2. ~~TUI option label~~ → **RESOLVED (owner)**: second step inside option 7 "Configurar Hermes", no new menu item.
3. ~~Tone verbatim check~~ → **RESOLVED (owner)**: "Cordial y orientador" confirmed for the template's canonical redirect.

---

## Phase 4 — Apply (owner-approved 2026-10-16)

> Rama `feat/feat-receptionist-skill-apply`. Unit content per `odd/tasks/feat-hermes-receptionist-skill.md` T5–T8. Advisory R2-001 (readability, design.md:39, informational) read/disposed: style-only, no apply obligation.

### TASK-4.1 — Template

- [x] `setup/templates/recepcionista-negocio/SKILL.md`: official format (frontmatter name/description/version, When to Use → Procedure → Pitfalls → Verification), single `{business_name}` placeholder (canonical example only), content per design §3

### TASK-4.2 — install.sh block

- [x] Skill-install step after the seed: render `{business_name}`, atomic `0600` write into `~/.hermes/skills/recepcionista-negocio/SKILL.md`, no silent overwrite of a modified skill, non-fatal failure reported in the summary — commit `2659c51`
- [x] bash tests under `scripts/tests/` (shunit2: 8 unit + 2 e2e, GREEN; run_tests.sh 6/6 suites; shellcheck 0 new)

### TASK-4.3 — TUI option 7 step 2

- [x] Regeneration step inside option 7: state detection (missing / up-to-date / modified), confirm on modified, atomic write, Spanish messages incl. index-refresh note — delivered in `8d94434` (internal/tui/skill.go cascade; feat doc Log L13/L15)
- [x] go tests (MVU + atomic write in `t.TempDir()`) — delivered in `8d94434` (skill_console_test.go console parity)

### TASK-4.4 — Verification & delivery

- [x] Full go pipeline + structural readback + native review of the code candidate — native review `review-c522aef035ce7e3c` APPROVED + burned (after `scope_changed` recovery of lineage f32febf9c0f9d664); 15 pkgs -race green, golangci 0, bash 6/6, cross-builds linux/darwin/windows (feat doc Log L15)
- [x] Smoke manual en el host (skill lista, `hermes skills` la indexa, comportamiento off-topic) — **transferred to backlog B2** (`odd/tasks/backlog.md`): owner-hand verification, outside this change's code scope; change closes with smoke pending by owner decision 2026-10-17
- [x] PR gh + squash merge + CI green — PR #135 squash-merged as `da231d3`; CI runs 38984018852 (pre-merge) / 38984381069 (post-merge) green (obs 1291)
