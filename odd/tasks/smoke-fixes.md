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

- [ ] **T1 — Wizard closed-day acceptance**: accept `no trabaja` (and `n`) in prompt_day_hours;
      keep the prompt copy as-is (it already advertises both). Add shunit2 coverage for the
      closed-day acceptance matrix in scripts/tests/.
- [ ] **T2 — XDG_DATA_HOME support**: resolveDBPath prefers `$XDG_DATA_HOME/mcp-appointments-crm/reservas.db`
      when XDG_DATA_HOME is set and absolute; falls back to `$HOME/.local/share/...` otherwise;
      MCP_DB_PATH still wins; unresolvable home stays a hard error. Table test in main_test.go.
- [ ] **T3 — Stale deployment docs**: update deployment.md macOS/Windows sections to reflect
      the v0.4.0 GoReleaser matrix (5 assets + checksums); keep the Windows "not implemented yet"
      distinction (install.ps1/`--register-service` genuinely absent) but fix the false asset claims.
- [ ] **T4 — Gate + PR**: AGENTS.md pre-flight pipeline (fmt/vet/golangci/build/test -race +
      shunit2 suites), native review gate per Verification & Review Protocol routing, then PR.

## Evidence / commits

(recorded per task on feature branch)

## Notes / deferred

- V3 (plist divergence) recorded as follow-up for a Mac-verified session.
- v0.5.0 tag after merge would ship T2 to users (DB-default XDG fix already on main + these).
