# Exploration: feat-setup-import — Setup import: wizard JSONs have no consumer

**Change:** feat-setup-import  
**Issue:** #71 — Setup import: wizard JSONs have no consumer (seed stays manual SQL)  
**Status:** complete  
**skill_resolution:** paths-injected

---

## Executive Summary

Issue #71 confirmed: `install.sh finalize()` writes three 0600 JSONs into `SETUP_DIR` (`${XDG_CONFIG_HOME:-~/.config}/mcp-appointments-crm/setup/` Linux, `~/Library/Application Support/MCP Appointments CRM/setup/` macOS), but **zero Go code reads them** — `internal/config` is only `doc.go` + `dotenv.go`, and its doc comment already claims it "loads and validates the JSON configuration files produced by the config-wizard TUI" (aspirational, unimplemented). `mcp-server` has only `--version`; DB path is `MCP_DB_PATH` env → `./data/appointments.db`, with the service unit pointing at `~/.local/share/mcp-appointments-crm/reservas.db`.

The three shapes are fully pinned (18-field business object + day-name `business_hours`, staff array with `schedule` entries `day_of_week 0–6 (0=Sunday)`, services array with `is_active: 1`). Recommended direction: **seed-on-first-boot** in `main.run()` right after `db.NewDatabase`, guarded by the fresh signal `business_profile.name == ''` (the lazy-init placeholder; wizard enforces non-empty name), executed in a **single transaction**.

---

## 1. Setup JSONs — location & shape

- Writer: `scripts/install.sh` `finalize()` (lines ~975-1020) → `render_setup_business/staff/services` piped through `atomic_write` into `SETUP_DIR`, then `jq`/`python3` parse check, then checkpoint deleted.
- Locations (`resolve_paths`, install.sh:542-590): Linux `${XDG_CONFIG_HOME:-$HOME/.config}/mcp-appointments-crm/setup/`; macOS `~/Library/Application Support/MCP Appointments CRM/setup/`. Files mode 0600 (e2e test asserts stat 600). Checkpoint at `$CONFIG_DIR/setup.json.tmp`.
- `setup_business.json`: single object, 18 fields in `BP_KEYS` order (name, industry, country, address, latitude, longitude, cover_photo_url, public_phone, messenger_platform, messenger_id, contact_email, website_url, general_description, accepted_payment_methods, currency_code, currency_symbol, timezone, slot_interval_minutes) + nested `business_hours` object with 7 day-name keys (monday..sunday), each `{open, close}` or `null` (closed day). `BP_TYPES`: lat/lon as JSON numbers, `slot_interval_minutes` integer, `accepted_payment_methods` as JSON array (`l` type), rest strings. Only `name` is required; currency/timezone/slot defaults `ARS`/`$`/`UTC`/`30` baked at wizard level.
- `setup_staff.json`: JSON array; entry = `{name, role_specialty, status:"active", email, phone, specialties: [], schedule: [{day_of_week 0-6 (0=Sunday), start_time "HH:MM", end_time}]}`. `specialties` ALWAYS emitted as empty array (no service linking in wizard). At least 1 professional required.
- `setup_services.json`: JSON array; entry = `{name, description, duration_minutes (int), price (JSON number), is_active: 1}`. At least 1 service required. Wizard spec allows `price ≥ 0`.

## 2. DB schema (internal/db/schema.go)

- `business_profile`: `id TEXT PK CHECK (id='singleton')`, `name NOT NULL`, `currency_code D 'ARS'`, `currency_symbol D '$'`, `timezone D 'UTC'`, `slot_interval_minutes D 30`, `business_hours TEXT NOT NULL D '{}'`.
- `professionals`: `id TEXT PK`, `name`, `role_specialty`, `status CHECK active/inactive`, `email`, `phone`, `specialties TEXT` (JSON array of service IDs).
- `schedules`: `INTEGER PK AUTOINCREMENT`, `professional_id FK CASCADE`, `day_of_week 0-6 CHECK`, `UNIQUE(professional_id, day_of_week)`, `start_time < end_time`.
- `services`: `id TEXT PK`, `name`, `description`, `duration_minutes CHECK >0`, `price REAL NOT NULL` (no `price>0` CHECK), `is_active CHECK 0/1`. FTS5 `services_fts` + `AFTER INSERT/DELETE/UPDATE` triggers — direct SQL inserts still populate FTS (triggers fire on any insert).
- `accounts`, `schema_version`, `bookings`, `clients`, `pending_alerts`, `business_hours_exception` also defined; `initSchema` idempotent.

## 3. CRITICAL convention mismatch — business_hours day keys

- Entity `BusinessProfile.BusinessHours` doc: `{"1":{"open":"09:00","close":"18:00"},...}`; `parseBusinessHours` maps string keys → int; `IsOpenOn`/`GetOpenClose` use `1=Monday..7=Sunday`.
- `setup_business.json` uses day NAMES with `null`=closed.
- Seeder MUST transform: `monday→"1" ... sunday→"7"`; `null` days → key ABSENT (entity treats missing key as closed). Staff schedule days (`0=Sunday..6=Saturday`) map 1:1 — no transformation (archived design explicitly says schedules shape mirrors canonical capability so "Fase-5 loader seeds rows without transformation").

## 4. Repo layer & the auth gate

- ALL repo writes call `auth.RequireRole(ctx, RoleAdmin, RoleOwner)` → a boot-time seeder with plain `ctx` FAILS (unauthenticated). Options: (a) direct parameterized SQL in a seeder (bypasses repos; entity `Validate()` can still be called for defense-in-depth), (b) `auth.WithCaller(ctx, Caller{Role: owner})` — `auth.WithCaller` exists.
- `BusinessProfileRepo.Get`: lazy-init `INSERT OR IGNORE (id='singleton', name='')` → idempotent, concurrency-safe; then `SELECT`.
- `ProfessionalsRepo.Save`: assigns UUID via `idgen.NewUUID()`; validates specialties exist; defaults status active.
- `ServicesRepo.Save`: does NOT generate ID — caller supplies `s.ID` (seeder must generate IDs).
- `SchedulesRepo.Upsert`: validates `day_of_week` + `HH:MM` times; insert-then-update-on-unique-violation.

## 5. Entity validation gotchas

- `BusinessProfile.Validate`: messenger_platform whitelist, payment_methods JSON array of non-empty strings, business_hours valid JSON object, IANA timezone. Name has NO non-empty check in `Validate` (DB `NOT NULL` only) — `name==''` is the reliable "unseeded" signal since wizard requires non-empty name.
- `Service.Validate` requires `Price > 0` — MISMATCH with wizard spec (`price ≥ 0` allowed). A 0-price service from the wizard would fail `repo.Save` validation. Direct SQL insert would pass (no DB CHECK). Needs decision.
- `Professional.Validate`: name non-empty, status active/inactive.

## 6. Config layer

- `internal/config` = `doc.go` + `dotenv.go` ONLY. `doc.go` explicitly claims the package "loads and validates the JSON configuration files produced by the config-wizard TUI" — aspirational, unimplemented. Natural home for setup JSON structs + loader. No loader exists anywhere; grep confirms zero Go readers of the setup JSONs.
- `mcp.LoadConfig` (`internal/mcp/config.go`): only `MCP_BIND`/`MCP_PORT` via env > `$HOME/.config/mcp-appointments-crm/.env` > defaults. NOTE inconsistency: `.env` path hardcoded `~/.config/...` even on macOS, while install.sh macOS `CONFIG_DIR` is "Application Support" and `ENV_FILE` is also `~/.config/...` — `.env` is the outlier. A Go setup-dir resolver must mirror `resolve_paths` per-OS logic (XDG on Linux, Application Support on macOS); `MCP_SETUP_DIR` env override recommended for tests.

## 7. Composition root (cmd/mcp-server/main.go)

- `run()`: `LoadConfig` → `ValidateLoopback` → `dbPath = MCP_DB_PATH` env or `./data/appointments.db` → `db.NewDatabase` (DSN pragmas: `busy_timeout(5000)`, `journal_mode(WAL)`, `foreign_keys`; `initSchema` idempotent) → construct 9 repos → wire use cases → auth (`CallerResolver` + `ToolRBAC`) → `mcp.Run`. Natural seed hook: immediately after `db.NewDatabase`, before repo construction/serving (single-threaded window, no concurrency risk; WAL + single writer anyway). Service unit sets `Environment=MCP_DB_PATH=${DATA_DIR}/reservas.db`; canonical DB file is `reservas.db` in `DATA_DIR` (`~/.local/share/mcp-appointments-crm` on Linux).
- No `--seed`/`--setup` flag exists (only `--version` short-circuit).

## 8. run_deploy blast radius

- `run_deploy` (install.sh:1457): `resolve_paths` → `refuse_root` → `require_deploy_prereqs` → `require_setup_files` (requires the 3 JSONs; shunit2-tested) → platform/download/extract → service install → verify. NEVER touches DB. Seed-on-boot approach requires NO `install.sh` changes; explicit-import approach requires `docs/installation.md` new step (+ possibly a flag) — docs currently document JSONs as final artifacts only.

## 9. Risks

| # | Risk | Mitigation |
|---|--- |--- |
| R1 | Re-run overwrites runtime edits | One-shot guard on `name==''` |
| R2 | Partial seed (professional without schedules) | Single tx, rollback on failure |
| R3 | Boot concurrency | Seed before HTTP serve; WAL+busy_timeout already set; lazy-init Get stays concurrency-safe |
| R4 | Wrong business_hours mapping corrupts availability | Tests pin monday→"1", sunday→"7", null→absent |
| R5 | Price-0 services rejected by repo validation | Decision needed (reject semantic vs direct SQL) |
| R6 | Setup-dir resolution drift bash↔Go | Mirror resolve_paths + `MCP_SETUP_DIR` override |
| R7 | Secrets/PII | None in JSONs; files already 0600, same-user service — low |
| R8 | Review budget ~600+ lines > 400 | `ask-on-risk` pause at delivery; candidate chained PR (loader+seeder \| wiring+docs) |

## 10. Alternatives

- **A (recommended):** seed-on-first-boot — `internal/config` Setup structs + loader; internal seeder applies all three JSONs in one tx when fresh; failure policy decision (fatal vs warn); `docs/installation.md` updated ("El servidor siembra la DB en el primer arranque").
- **B:** explicit import — `mcp-server --seed-setup` (or subcommand) run once per docs; zero boot magic; risk: operator forgets → same empty-profile symptom.
- **C:** hybrid — A + explicit flag to force re-import; more surface, only if re-seed demand is real.

## 11. Questions for proposal

1. Seed-on-boot (A) vs explicit import (B) vs hybrid (C)?
2. Fresh signal: `name==''` alone, or `name==''` AND professionals empty AND services empty?
3. Auth bypass: direct parameterized SQL vs `auth.WithCaller(owner)` through repos?
4. Malformed/missing JSONs at boot: fatal exit 1 or warn-and-continue with empty profile?
5. Price-0 services: reject with semantic error or accept?
6. Delete setup JSONs after successful seed, or keep (guard prevents re-seed)?
7. Confirm business_hours mapping (monday→"1"…sunday→"7", closed→absent)?

## Next

Proceed to proposal phase, resolving questions Q1–Q7 (Q1→A, Q4 fatal-fail-fast, and R8 chaining strategy are the load-bearing calls).
