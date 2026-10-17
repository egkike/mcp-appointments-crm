# Repo backlog (live) — reconciled 2026-10-17

**Type**: tracking doc (single live backlog; supersedes the "backlog de sesión" P1-P3 notes in Engram obs 1291/1323)
**Reconciled**: 2026-10-17 from obs 1291/1323 session summaries + feat-hermes-receptionist-skill Log + repo evidence
**Corrections vs stale notes**: the old "P1 — v0.7.0 release + backend bot WhatsApp (ADR-0018)" is CLOSED: v0.7.0 shipped 2026-10-16 (tag, changelog 1.22, 6 GoReleaser assets, CI green). `review-findings-backlog.md` is archived history (all items disposed); it is no longer the live backlog.

**Policy context (2026-10-17)**: delegated worker commits formalized in AGENTS.md (`ecebff0`: worker runs pre-flight → commit with GGA hook → fix → re-commit, ≤5 rounds; ambiguous verdicts round-trip to the owner — never worker-side `--no-verify`). GGA stays on gpt-6-luna; STRICT_MODE stays ON; upstream #113 watched passively.

## Backlog

| ID | Item | Source | Route | Status |
|---|---|---|---|---|
| **B1** | Close OpenSpec change `feat-hermes-receptionist-skill`: mark the 10 unchecked boxes in `openspec/changes/feat-hermes-receptionist-skill/tasks.md` with existing evidence (PR #135, CI 38984381069, native `review-c522aef035ce7e3c`) and archive as `openspec/changes/archive/2026-10-16-feat-hermes-receptionist-skill/` (5 prior changes follow the dated convention) | ODD feature `feat-hermes-receptionist-skill.md` L16 | docs lane → main (ff-only, no PR) | OPEN |
| **B2** | Smoke manual del host (owner): `mcp-server admin tui` → option 7 → step 2 cascade ("skill instalada"), then WhatsApp "Hola" a secas → cordial reply WITHOUT tool call, and a business query → normal tool flow. Last uncovered end-to-end verification of the receptionist skill | obs 1291 Next Steps 1; feat doc L16 | owner manual | OPEN (owner hands) |
| **B3** | Hygiene PR: `golang.org/x/text v0.3.8` indirect → latest (v0.43.0). Transitive via bubbletea/bubbles, unreachable from project code, govulncheck clean — hygiene only | obs 1241 Next Steps 2 (2026-10-13) | code → feature branch → PR | OPEN |
| **B4** | Advisory-lock boundary: external process writing the Hermes config while the server runs is outside the flock (check-to-use race is covered; external-writer competition is a documented design disposition АDR-0020 design §1). Candidate as issue `type:refactor`; refactor itself NOT scoped | obs 1291 Next Steps 2b | issue creation (owner decision) | OPEN (optional) |
| **B5** | Roadmap post-v0.7.0: P2 REST/HTTP transport endpoints; P3 metrics/observability. NO scope, spec, or owner requirement yet — directions only; must enter the normal ODD doc-first flow when picked | obs 1291 Next Steps 3 (stale P1 removed) | owner decision → doc-first | OPEN (needs scope) |
| **B6** | GGA upstream vigilance: Gentleman-Programming/gentleman-guardian-angel#113 OPEN without triage since 2026-10-11 (4 owner comments, last 6227034534; maintainer silent). On upstream fix → update local `.gga` (homebrew upgrade) | obs 1323 Next Steps 4 | passive watch | OPEN (external) |
| **B7** | CI check of `ecebff0` before the NEXT push to main (docs-only push bypassed the "Verify Go code" required check; Post-Merge CI Sequencing rule applies) | AGENTS.md push 2026-10-17 | single check on demand | OPEN (micro) |

## Closed / superseded (record kept)

- ~~Old P1: v0.7.0 release + WhatsApp bot backend~~ — **CLOSED 2026-10-16**: v0.7.0 published (tools 20→23, ADR-0018 backend, hardening; changelog PRD 1.22).
- ~~GGA STRICT_MODE mitigation strategies (deepseek/OPENCODE_AGENT/scheduled check)~~ — **SUPERSEDED 2026-10-17**: owner picked gpt-6-luna + delegated-worker GGA loop (AGENTS.md); STRICT_MODE stays ON; `--no-verify` remains owner-only per ambiguous case.
- ~~Native acknowledge burn schema-incompatible (2026-10-13)~~ — **superseded**: burns worked in the 2026-10-16 session; no recurrence reported.
- ~~Hermes rename-race outside-model disposition~~ — **DISPOSED**: documented ADR-0020 design §1, no action.
- ~~r22/r23 drain-queued signal edge~~ — **DISPOSED**: `dedupeSignalTrigger` prod/tests split shipped in PR #135.

## Log

- **L1** 2026-10-17 — owner: "Primero actualiza el backlog y luego me vuelves a mostrar los pendientes". Backlog reconciled from stale session-summary notes; stale P1 corrected against repo evidence (tags + PRD changelog). Item B1 discovered fresh (OpenSpec change left open with 10 unchecked boxes).
