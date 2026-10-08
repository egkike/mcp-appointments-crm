# Feature: feat-whatsapp-bot — COMPLETE

**Status:** COMPLETE (Phases 1–6 merged + archived 2026-10-08)
**Change dir:** `openspec/changes/archive/2026-10-08-feat-whatsapp-bot/`
**Insumos frozen:** ADR-0018 (`docs/architecture/0018-communication-channels.md`, NO re-discutir), PRD §3.8.9 / §3.8.10 / §7 Fase N (changelog 1.21), F-4 clear-null resolved (merge `3b86db9`).

## Product decisions confirmed with owner (2026-10-08, this session)

| # | Decision | Value |
|---|----------|-------|
| D1 | Scope in this repo | Server-side contract (auto-registration + derived `maps_url` + gateway contract docs) **plus** config for the private Telegram owner-bot (token / allowlist chat / owner phone as source of truth) **plus** TUI option 7 extension (ADR-0017) writing them into `~/.hermes/config.yaml`. No Go gateway in this repo. |
| D2 | Auto-registration mechanics | New MCP tool `register_client`, anonymous-allowed via middleware allowlist; calls new use case `get_or_create_client`; creates `clients` row with `id == phone` (consistent with resolver step 2 and ADR-0011); resolver stays read-only. Gateway flow: on 401 → ask name via chat (optional) → `register_client` → retry. |
| D3 | Registration friction / anti-spam | Display name optional — placeholder `Cliente {phone}` when Hermes does not collect it; configurable registration rate limit in server config (conservative default, e.g. 10/hour) with semantic error on exhaustion. |
| D4 | Location contract | Derived `maps_url` (`https://maps.google.com/?q=lat,long`) in `get_business_profile` output **and** `geo:` URI (RFC 5870, `geo:lat,long`) accepted as alternative input for `update_business_profile`. Both in this change. |

## Key evidence (scout, 2026-10-08)

- Unknown phone → `authError{msgNotRecognized, ErrUnauthenticated}` (`internal/auth/resolver.go:110-112`) → middleware 401 (`internal/auth/middleware.go:83-90`) → JSON-RPC `-32000` via `internal/mcp/auth_translator.go:94-118`.
- No `get_or_create_client` use case exists. Closest: `ClientsRepo.GetOrCreate` (`internal/repository/clients.go:223`, UUID id — **trap**: resolver only matches `clients.id == phone`, so UUID-id clients never resolve; `admin.AddSelfAsClient` `internal/admin/clients.go:84-152` deliberately uses `id == phone`).
- X-Caller-Id read only in `internal/auth/middleware.go:76`; RBAC map `cmd/mcp-server/main.go:396-414`; path→tool bridge `internal/mcp/auth_translator.go:69-72`; sanitization `internal/mcp/errors.go:32-48`.
- `business_profile` has `messenger_platform`/`messenger_id` + CHECK (`internal/db/schema.go:19-33`); `latitude`/`longitude` REAL nullable, no range CHECK; `get_business_profile` wire output has lat/long (`internal/mcp/tools_profile.go:25-49`), **no** `maps_url`.
- No MCP client-creation tool; phone validation = `entity.Client.HasValidPhone` (optional `+`, 4–15 digits, NO normalization); `internal/validation` is a doc-only stub.

## Tasks — Phase A: Spec (this session)

- [x] A1 — `exploration.md`: evidence base from scout report (done source: subagent report 2026-10-08).
- [x] A2 — `proposal.md`: problem, outcome, scope, non-goals, decision log D1–D4 (+ D2a accounts-collision guard added by writer, flagged for owner veto), risks, success criteria.
- [x] A3 — `design.md`: register_client flow + middleware anonymous allowlist seam, get_or_create_client use case (id==phone), rate limiter, maps_url derivation, geo: URI parsing, Telegram config + TUI option 7 extension, security review.
- [x] A4 — Spec deltas (6): NEW `client-registration`, NEW `hermes-config-tui`, NEW `clients` (port method), MODIFIED `business-profile`, MODIFIED `auth-middleware` (precedence empty-header → allowlist → resolution), MODIFIED `mcp-transport` (19→20 tools, maps_url, location_uri).
- [x] A5 — `tasks.md`: implementation phases with Review Workload Forecast + chained-PR strategy; work units ≤ ~100 lines (RDD band rule, obs 1105/1170).
- [x] A6a — Verification loop: first writer run failed (over-reading, no writes) → relaunched with bounded-reading discipline → structural readback by verifier (1 CRITICAL: missing mcp-transport delta; 5 WARNING; 4 SUGGESTION) → surgical fix pass F1–F7 (7/7) → orchestrator spot-checks PASS (mcp-transport delta verbatim except named reqs; auth-middleware precedence contradiction resolved; 15-digit cap scoped to registration path). client-registration at 131 lines accepted over the soft 120 writer cap (generation discipline, not a repo rule).
- [x] A6b — Owner review + commit on feature branch `feat/feat-whatsapp-bot-spec` (Conventional Commit; GGA hook must pass; ff-merge to main after approval).

## Operational rules

- English in artifacts, Rioplatense in chat. Owner decides product; orchestrator coordinates; writer delegates.
- Docs routing: feature branch → owner approval → `git merge --ff-only` to main → push → delete branch. No PR for docs.
- RDD gate for this spec phase: structural readback (documentation). Native review applies to future implementation work units (≤ ~100 lines per slice while relay defect #3991/#5226 is unfixed).

## Evidence

- **MERGE CHAIN (all squash, all CI green):** #107 → `5a5335f` (Phase 1+2) · #108 → `cfb9f7b` (Phase 3) · #109 → `bd4b894` (Phase 4) · #110 → `b80741d` (Phase 5) · #111 → `0da5c27` (Phase 6 docs). Commits within PRs: 3a0d0fd (Save UPSERT fix), 391847f (Phase 1), 5947bff (Phase 2), aa98dd7 (Phase 3), ca67ea2 (Phase 4), 799a0ea (admin core), 6e0b551 (TUI), 5850828 (lint fix), b0efcee (docs).
- **Delivered**: anonymous registration (seam + register_client + get_or_create_client + rate limit + 5 masked audit events), location contract (maps_url + geo: URI + domain range invariant), TUI option 7 Telegram owner-bot config, gateway contract docs + operator runbook, PRD 1.21.
- **Gate-review catches (substantive, fixed at the correct layer)**: coordinate range invariant moved to entity.Validate; SetHermesServer env-preservation (silent data-loss path); E.164 cap in the domain (doc made true); maintenanceHHMM dedupe; orphaned doc + length caps.
- **Remaining operator/production steps (documented in docs/gateway-contract.md §7, not code)**: Transfer Ownership to the real phone, messenger_platform → whatsapp, Hermes gateway implementation per the contract.
- **Follow-ups recorded**: message-prefix convention unification; businessHoursDay JSON tags relocation; live token textinput echo; FindClientByPhoneAny data-minimization; FTS invalid-input message translation (search use case); geo: for other tools; phone normalization; internal/validation stub.

- **Spec commit `841e55d` on main (2026-10-08)** — 11 files, +1269; branch `feat/feat-whatsapp-bot-spec` ff-merged and deleted; GGA no issues (docs-only); CI run **success** on 841e55d; owner approved commit + full docs flow.
- **Implementation PR1 (branch `feat/feat-whatsapp-bot-registration`)**: Phase 1 RED→GREEN by writer (port `GetOrCreateByPhone` +17, adapter +68 with BEGIN IMMEDIATE/dedicated conn/RowsAffected-created, test file 117 lines, mock compile-fix +12/-4). Full pipeline green (fmt/vet/lint 0/build/test -race, 18 packages).
- **Native review attempt (canary) FAILED as predicted**: lineage `review-afab2cbc89fa6de9` created (medium tier, lens review-reliability, 224 changed lines, correction budget 112); capture forecast 1 model run via `pi_host_relay`; run died `reviewer-empty-output` / `stopReason: length` at ~49 s (no timeout — budget starvation). Consistent with #5226 band data (226 lines died ×2). Per owner policy: NO retry until upstream fix; lineage left open; fallback gate active (independent verifier + GGA + CI + PR human review).
- **Phase 2 (2026-10-08, branch)**: 2A use case `get_or_create_client` (RED→GREEN, 9→10 subtests) + auth-free `RegistrationLookup` port/adapter (HIGH-risk gap found by writer: FindByID/FindByPhone enforce RBAC; fixed following the `ClientsSelfService` precedent; guards untouched) + 2B per-phone fixed-window rate limiter (injected clock, default 10/h, 0=fail-closed, `MCP_REGISTRATION_RATE_LIMIT`) + 4+1 masked-phone audit events (`***9999`; 5th event `registration_rejected_legacy_id` added by orchestrator to close the spec's every-outcome audit requirement for the legacy branch) + DI wiring. Full suite green.
- **GGA upstream**: defect confirmed as known issue [#113](https://github.com/Gentleman-Programming/gentleman-guardian-angel/issues/113) (owner had prior confirmation there); addendum posted with owner consent (comment 6067254147): run-to-run nondeterminism, code anchors, fix direction, `OPENCODE_VARIANT`/`--variant` breakage noted. GGA cache-hit PASS on the Phase 2 commit; lint gap (errcheck in test file) caught by CI and fixed (5850828) — full pre-flight incl. golangci-lint is now mandatory before every push.
- **Status: IMPLEMENTATION — PR1 (Phase 1 complete under fallback gate).** Apply plan: `openspec/changes/feat-whatsapp-bot/tasks.md` (work units ≤ ~100 lines; test-first for Go). Pending: TASK-2.2 legacy-row edge decision (verifier WARNING 2026-10-08); three at-apply confirmations (`location_uri` naming, rate-limit config shape, Telegram MCP_* key spellings).
