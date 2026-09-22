# Feature: smoke-fixes

**Date**: 2026-09-20
**Type**: fix/docs (small PR, aggregated smoke follow-ups)
**TDD**: OFF (not configured in this project); every task ships focused tests following existing patterns

## Goal

Close the actionable findings from the v0.4.0 fresh-install functional smoke on the HomeLab VM
(2026-09-18) plus the stale documentation they exposed, as one small reviewable PR.

## Scope

- V1 — install.sh wizard copy vs validator: the day-hours prompt promises `cerrado/no trabaja`
  but the validator only accepts `cerrado|c|no` (scripts/install.sh:651/658).
- V2 — resolveDBPath must honor XDG_DATA_HOME before falling back to `$HOME/.local/share`
  (cmd/mcp-server/main.go:525), matching ADR-0002 XDG layout semantics.
- D1/D2 — deployment.md stale "not distributable today" notes (macOS ~246, Windows ~286)
  written for v0.3.0; v0.4.0 ships 5 platform assets + checksums.txt.

## Non-goals

- V3 macOS LaunchAgent plist DB-path divergence (touches service template; needs a Mac to verify) — separate follow-up.
- V4 TUI "configurar Hermes" option and the WhatsApp per-sender bot (features, PRD Fase N backlog).
- hermes-maintenance informational follow-ups (pluralization, Atoi strictness, R2/R3/R4 refactors) — unclaimed backlog.

## Tasks

- [x] **T1 — Wizard closed-day acceptance** ✅ commit `afef85b`
- [x] **T2 — XDG_DATA_HOME support** ✅ commit `80e37d0`
- [x] **T2b — stale repos count log fix** ✅ commit `ce56650` (GGA observation on 80e37d0;
      9 repos not 11, same class as d7b65f7)
- [x] **T3 — Stale deployment docs** ✅ commit `12d62e9`
- [ ] **T4 — Gate + PR**

## Evidence / commits

- T1: `afef85b` — fix(install): accept "no trabaja" as a closed day in the wizard
- T2: `80e37d0` — fix(config): honor XDG_DATA_HOME in the default database path
- T2b: `ce56650` — fix(mcp): correct stale repos count in startup log
- T3: `12d62e9` — docs(deployment): refresh macOS/Windows install sections for v0.4.0
- GGA incidents: opencode provider timed out 5x (~25 min) during T2 commit; owner chose
  retry-over-provider-switch, commit passed on retry. T2b commit hit STRICT_MODE ambiguous
  STATUS (3 fails) — cache clear + retry passed. GGA observations recorded: deps.config
  write-only field, commandServe-with-error smell, wantsVersion(os.Args) convention mix
  (non-blocking, unclaimed backlog).

## Notes / deferred

- V3 (plist divergence) recorded as follow-up for a Mac-verified session.
- v0.5.0 tag after merge would ship T2 to users (DB-default XDG fix already on main + these).

## T4 gate status (2026-09-20)

- Native review lineage review-132759522ab84ba5 created (high/4-lens, budget 106, base=main
  committed range). Reviewer relay blocked at slot 0: opencode Go 400 MissingSessionID x2
  (deterministic). Root cause confirmed upstream: pi-ai 0.86.1 withSessionHeader requires
  options.sessionId; gentle-ai 3.4.0 in-process reviewer completion passes none. Duplicate
  confirmed on Gentleman-Programming/gentle-shell#1260 (+#1242/#1235); repro comment posted
  with pi-ai code pointer (issuecomment-5751553667). Nothing captured/burned; branch verified
  via pipeline + shunit2 + GGA. Gate pending upstream fix or provider switch.
