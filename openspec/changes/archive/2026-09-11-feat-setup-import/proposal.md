# Proposal: feat-setup-import — Seed DB from wizard JSONs on first boot

**Change:** feat-setup-import
**Issue:** #71 — Setup import: wizard JSONs have no consumer (seed stays manual SQL)
**Status:** proposed
**skill_resolution:** paths-injected

---

## Executive Summary

The setup wizard (via `install.sh finalize()`) writes three JSON files into `SETUP_DIR` — `setup_business.json`, `setup_staff.json`, `setup_services.json` — but **no Go code ever reads them**. `internal/config/doc.go` already claims it loads these files; that claim is aspirational. Operators currently must seed the DB manually with SQL or leave the profile empty. This change makes the server **seed the database automatically on first boot** from the wizard JSONs, in a single atomic transaction, guarded by the "fresh DB" signal (`business_profile.name == ''`). After this change, completing the wizard and starting the service yields a fully operational CRM with zero manual SQL.

Product decisions (D1–D7) were confirmed with the owner before this proposal; no open product questions remain.

## Business Problem

- The wizard's final artifacts are dead files: the installation flow *looks* complete but the server starts with an empty profile, professionals, and services.
- Seeding today requires manual SQL from an operator — an operational cost and a footgun (typo-prone, no transaction, no day-key mapping).
- `internal/config`'s documented contract ("loads and validates the JSON configuration files produced by the config-wizard TUI") is unimplemented — the codebase lies to its maintainers.

## Product Outcome

- After `install.sh run_deploy` + `systemctl start`, the server boots once, seeds business profile + professionals + schedules + services from the wizard JSONs, logs success, and serves normally.
- No re-seed ever happens on subsequent boots (guard), so runtime edits are safe.
- Malformed wizard output fails fast at first boot with a semantic Spanish error, instead of a silently half-configured CRM.

## Scope

1. **Setup-dir resolver** (`internal/config`): per-OS mirror of `install.sh resolve_paths` — Linux `${XDG_CONFIG_HOME:-$HOME/.config}/mcp-appointments-crm/setup/`, macOS `~/Library/Application Support/MCP Appointments CRM/setup/` — with `MCP_SETUP_DIR` env override for tests.
2. **Setup JSON structs + loader** (`internal/config`): typed structs pinning the 3 wizard shapes; loader reads and JSON-decodes each file (0600 files, single-user service).
3. **`business_hours` mapping**: transform day-name keys (`monday`..`sunday`) to numeric-string keys `"1"`..`"7"` (1=Monday..7=Sunday); `null` day → key absent (closed).
4. **Seeder** (`internal/config` or sibling internal package): single SQLite transaction — `BEGIN` → UPSERT `business_profile` singleton (all 18 fields) → insert professionals (UUID via `idgen.NewUUID`) → insert schedules → insert services (UUID) → `COMMIT`. Any error → `ROLLBACK` → fatal. Direct parameterized SQL (`?` placeholders) — not repos (see D3). Entity `Validate()` calls retained as defense-in-depth where shapes match.
5. **Fresh guard**: seed only when `business_profile.name == ''`. Loader/seeder skipped entirely (log-and-continue) when not fresh.
6. **Wiring** (`cmd/mcp-server/main.go`): run guard + seed immediately after `db.NewDatabase`, before repo construction and HTTP serving (single-threaded window).
7. **Docs**: `docs/installation.md` — replace "JSONs are final artifacts" wording with "El servidor siembra la DB en el primer arranque si el perfil está vacío".
8. **Tests**: resolver per-OS logic, loader (happy/missing/malformed), business_hours mapping (monday→"1", sunday→"7", null→absent), fresh guard, transaction rollback on mid-seed failure, boot wiring. `go test -v -race ./...`.

## Non-goals

- No `--seed` flag or CLI subcommand (B rejected — operator-forget risk).
- No re-seed / force-import path (C deferred until real demand).
- No `install.sh` changes (run_deploy never touched DB, stays that way).
- No cleanup/deletion of setup JSONs after seed (D6).
- No fix for the `Service.Validate` price>0 vs wizard price>=0 repo-validation mismatch (D5 follow-up, out of scope).
- No changes to wizard JSON shapes or `internal/db/schema.go`.
- No multi-day/multi-profile imports; single wizard run → single seed, ever.

## Affected Areas

| Area | Files | Change |
|------|-------|--------|
| Setup loader | `internal/config/` (new file(s)) | Setup structs, per-OS dir resolver, JSON loader |
| Seeder | new internal package or `internal/config` | Transactional seed SQL + business_hours mapping |
| Composition root | `cmd/mcp-server/main.go` | Guard + seed hook after `db.NewDatabase` |
| Docs | `docs/installation.md` | First-boot seeding wording |
| Tests | alongside each new file | resolver / loader / mapping / guard / rollback / wiring |

## Decision Log

| # | Decision | Rationale |
|---|----------|-----------|
| D1 | **Strategy A: seed-on-first-boot.** Loader + seeder run at boot when fresh. Explicit `--seed` flag rejected (operator forgets → same empty-profile symptom); hybrid deferred (no re-seed demand). | Zero-operator-touch UX; guard makes it safe; aligns with existing aspirational `internal/config` contract. |
| D2 | **Fresh signal: `business_profile.name == ''` alone.** Wizard guarantees non-empty name; lazy-init `INSERT OR IGNORE` creates the `''` placeholder. Professionals/services emptiness NOT checked — preserves manual deletions without re-seed. | Minimal, reliable, respects user intent. |
| D3 | **Auth: direct parameterized SQL inside a single transaction**, with entity `Validate()` as defense-in-depth. NOT `auth.WithCaller` through repos — repos expose no tx API; atomicity requires one tx. | Repos are RBAC-gated and tx-less; direct SQL is the only way to get all-or-nothing semantics. |
| D4 | **Malformed/missing JSONs at boot:** fatal `exit 1` with semantic Spanish error when fresh (fail-fast); when not fresh, ignore and log — guard prevents re-seed anyway. | A fresh DB with bad wizard output must not yield a silently half-configured CRM. |
| D5 | **Price-0 services: accept** via direct SQL (DB CHECK allows `price >= 0`); wizard spec allows `price >= 0`. Repo `Validate` mismatch documented as follow-up. Seeds must not reject `price == 0`. | Wizard contract wins; DB already permits it. |
| D6 | **Keep setup JSONs after successful seed.** Guard prevents re-seed; files serve as audit/recovery reference. | No destructive surprise for the operator. |
| D7 | **business_hours mapping confirmed:** `monday→"1" .. sunday→"7"`, `null` → key absent (closed). Staff schedule `day_of_week 0–6 (0=Sunday)` maps 1:1, no transformation. | Pinned by entity docs and archived design. |

## Risks & Rollback

| # | Risk | Mitigation |
|---|------|------------|
| R1 | Re-run overwrites runtime edits | D2 one-shot guard (`name == ''`); test pins no-re-seed |
| R2 | Partial seed (professional without schedules) | Single tx; any error → ROLLBACK → fatal; test pins rollback |
| R3 | Boot concurrency | Seed runs before HTTP serving (single-threaded window); WAL + `busy_timeout=5000` already in DSN |
| R4 | Wrong business_hours mapping corrupts availability | Tests pin monday→"1", sunday→"7", null→absent (D7) |
| R5 | Setup-dir resolution drift bash↔Go | Resolver mirrors `resolve_paths` per-OS; `MCP_SETUP_DIR` override; test covers both OS branches |
| R6 | Malformed JSON bricks boot | Intentional (D4 fail-fast) but only when fresh; semantic Spanish error tells operator exactly which file/field to fix |
| R7 | PII/security | JSONs contain no secrets, already 0600, same-user loopback service — low |
| R8 | Review budget: ~600 lines > 400 | `ask-on-risk` pause at delivery; candidate chained PRs (loader+seeder / wiring+docs) — decision deferred to apply/delivery phase per preflight |

**Rollback:** The change is boot-path additive: revert the `main.go` hook commit and the server returns to today's behavior (empty profile, manual SQL). No schema migration, no data transformation — seeded rows are ordinary rows; deleting them + emptying `business_profile.name` restores a fresh DB. Setup JSONs are never modified or deleted by the server (D6), so the original artifacts survive any rollback.

## Success Criteria

- [ ] Fresh DB + valid JSONs → single boot seeds profile/professionals/schedules/services; log confirms; second boot does nothing (guard).
- [ ] Fresh DB + malformed JSON → exit 1 with semantic Spanish error naming the file; DB left at pre-seed state (rollback verified).
- [ ] Non-fresh DB (any `name != ''`) + missing/malformed JSONs → server boots normally, logs, no seed.
- [ ] `business_hours` stored as numeric-string keys `"1".."7"`; closed days absent; availability logic (`IsOpenOn`) reads seeded hours correctly.
- [ ] `price == 0` services seed successfully (D5).
- [ ] `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...` clean; `go build -o /dev/null ./...` passes; `go test -v -race ./...` passes.

## Artifacts

- Exploration: `openspec/changes/feat-setup-import/exploration.md` (complete)
- Proposal: `openspec/changes/feat-setup-import/proposal.md` (this document)

## Next

- **next_recommended:** spec phase — draft deltas for affected capabilities (setup loader, seeder, boot wiring) pinning REQs for each Scope item, then design (transaction layout, resolver algorithm) and tasks.
