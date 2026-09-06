# Proposal: feat-install-and-service

**Change:** feat-install-and-service
**Phase:** 5 — install-and-service
**PRD reference:** PRD § Fase 5, §3.5 (XDG layout)
**Created:** 2026-09-06 (init) — finalized 2026-09-06 after sdd-explore
**Status:** final (exploration questions Q1–Q5 resolved; decisions D1–D6 taken, see Decision Log)
**skill_resolution:** paths-injected (gentle-ai SKILL.md)

---

## Business Problem

The end customer of mcp-appointments-crm is a small-service business (salon, studio, practice) whose booking data must live on **their own** infrastructure — a rented VPS — rather than a third-party SaaS. Today, going from "clean VPS" to "working self-hosted booking system" requires manual binary copying, hand-written service files, ad-hoc database path decisions, and undocumented backup knowledge spread across `docs/deployment.md` (the de-facto spec). That means:

- **Deployment is expert-only.** A non-sysadmin integrator cannot deliver the system in a single session; every manual step is a chance to diverge from the documented XDG layout.
- **Silent state divergence.** The binary defaults to `./data/appointments.db` (CWD-relative) while every doc promises `~/.local/share/mcp-appointments-crm/reservas.db`. Two customers, two databases, neither knows which one their backups protect.
- **No support story.** There is no maintenance manual, no portable backup script, and no upgrade path, so an integrator who sets this up today cannot hand it over a year later.

## Product Outcome

After Fase 5, an integrator (or the business owner with a terminal and one curl command) can go from a clean Ubuntu 22+ VPS to a **running MCP server that survives logout and reboot**, with the database in the documented XDG location, a verified binary, a tested backup command, and a Spanish maintenance manual to hand to whoever inherits the server. The moment of delivery becomes a single command instead of a checklist, and support becomes a document instead of tribal knowledge.

## Scope

### In scope (Fase 5)

- **`scripts/install.sh` (extend-only)** — complete the Fase 4 script into a `curl | bash` deployment pipeline:
  - New `--version vX.Y.Z` flag (pinned release tag; see Q4/deployment.md contract: `bash -s -- --version v0.3.0`).
  - OS/arch detection (`uname -s` / `uname -m` → GoReleaser names: `Linux`/`Darwin` × `x86_64`/`arm64`), asset download from `https://github.com/egkike/mcp-appointments-crm/releases/download/{tag}/{asset}`, SHA256 verification against `checksums.txt`, archive extraction (`mcp-server` / `mcp-server.exe`).
  - Extension of `resolve_paths()` to resolve `DATA_DIR`, `BIN_DIR`, `LOG_DIR` per OS (Q4/D4).
  - User-level service registration from `setup/service/` templates: systemd user unit (Linux, incl. `loginctl enable-linger`), launchd LaunchAgent (macOS), NSSM placeholder (Windows, template + docs only).
  - Post-install guidance: suggested `backup.sh` command line, "Recommended additional tools" block, final log with the MCP endpoint URL (`http://127.0.0.1:3000/mcp`).
  - Non-TTY degradation: `curl | bash` without a TTY skips interactive prompts with a clear message (D6).
- **`mcp-server --version` flag (minimal, ~20 LOC in `cmd/mcp-server/main.go`)** — prints `buildinfo.Version` and exits 0 (D2).
- **Portable `scripts/backup.sh`** — `sqlite3 {db} ".backup '{tmp}'"` + `gzip` → `backups/reservas-YYYYMMDD.db.gz` under the data dir; requires only bash, sqlite3, gzip; works on Linux and macOS.
- **`setup/service/`** (new directory): `mcp-appointments-crm.service` (systemd user), `com.mcp.appointments.server.plist` (launchd), `nssm-install.md` (Windows placeholder).
- **`docs/installation.md`** — step-by-step installation manual, Spanish.
- **`docs/maintenance.md`** — annual support/operations manual, Spanish (backups, upgrades, log rotation, troubleshooting, service management per OS).
- **Doc unification**: wherever `appointments.db` is used as the production DB name, docs converge on `reservas.db` (D5); `docs/deployment.md` stays the detailed reference, `installation.md` is the customer-facing path.

### Explicit non-goals (out of scope)

- **CI/CD pipelines and `.goreleaser.yml` / `release.yml`** — Fase 6+ (ADR-0014 target state; releases exist as a workflow contract, automation does not).
- **Docker/container deployment** — permanent non-goal (ADR-0001).
- **Automatic backup scheduling** — `backup.sh` is manual-only (ADR-0005); scheduling guidance lives in `maintenance.md` as an optional customer task.
- **Windows service registration automation** — NSSM template + Spanish docs only; no `install.ps1` automation in this phase.
- **Changing the Go binary's default DB path or env semantics** — service unit carries `MCP_DB_PATH` instead (D1); no rework of `internal/mcp/config.go` beyond the `--version` flag.
- **Rewriting the Fase 4 interactive setup flow** — prompt engine, validators, `finalize()` checkpoints remain untouched.

## Affected Areas

| Area | Change type |
|---|---|
| `scripts/install.sh` (1053 lines, Bash 3.2 floor) | extend `main()`, `resolve_paths()`; add deploy pipeline post-`finalize()` |
| `cmd/mcp-server/main.go` | add `--version` flag (~20 LOC) |
| `scripts/backup.sh` | new |
| `scripts/tests/` (shunit2) | extend: new deploy-flow tests alongside existing validator/e2e suites |
| `setup/service/` | new (3 files) |
| `docs/installation.md`, `docs/maintenance.md` | new (Spanish) |
| `docs/deployment.md`, `docs/PRD.md` | minor alignment only (DB name unification) |

## Decision Log (exploration risks, resolved)

**D1 — Database path conflict (Q3). `MCP_DB_PATH` in the service unit; no Go change.**
The unit sets `Environment=MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db` (or the macOS equivalent via the plist). *Tradeoff:* minimal blast radius (Go binary untouched, no migration for direct-run users), but a manual `./mcp-server` invocation still defaults to `./data/appointments.db`. We accept that: direct-run is the dev path, the service is the supported path, and `installation.md` states this explicitly. Changing the Go default (option b) would silently relocate databases for anyone running the binary directly today — a worse risk for zero product gain in this phase.

**D2 — `mcp-server --version` gap. Add the minimal flag in Fase 5.**
`docs/deployment.md` already promises `mcp-server --version`, and the install pipeline's post-install verification step (and future upgrade checks) depend on it. *Tradeoff:* ~20 LOC of Go + one test, versus a documented interface that doesn't exist and a `--version` UX the installer cannot verify. Deferring to Fase 6 would leave Fase 5 shipping docs that lie. Decision: add it. `install.sh --version` (installer flag) and `mcp-server --version` (binary flag) are distinct interfaces; both exist after this phase.

**D3 — Bash 3.2 floor and existing tests are sacred.**
All new install.sh code follows the established constraints (no associative arrays, no `mapfile`, no `declare -n`, no `${var,,}`, no `pipefail`/`set -e`; `set -u`, `umask 077`, existing trap/cleanup). The two existing shunit2 suites (`install_validators_test.sh`, `install_e2e_test.sh`) must keep passing unmodified; new deploy tests are added alongside, not replacing. *Tradeoff:* some new code is more verbose (byte-wise string handling for asset-name composition), accepted for portability.

**D4 — XDG-ish paths per OS via `resolve_paths()` extension.**
Linux: `DATA_DIR=${XDG_DATA_HOME:-$HOME/.local/share}/mcp-appointments-crm`, `BIN_DIR=${XDG_BIN_HOME:-$HOME/.local/bin}`, `LOG_DIR=${XDG_STATE_HOME:-$HOME/.local/state}/mcp-appointments-crm`. macOS: `DATA_DIR=$HOME/Library/Application Support/MCP Appointments CRM`, `LOG_DIR=$HOME/Library/Logs/MCP Appointments CRM`, `BIN_DIR=$HOME/.local/bin` (PATH guidance in docs). Windows: documented layout (`%APPDATA%\MCP Appointments CRM\`) in docs/templates only (non-goal: automation). The existing symlink rejection on `CONFIG_DIR` is extended to the new dirs.

**D5 — Database name unification: `reservas.db`.**
All production docs, backups, and the service unit use `reservas.db` (matching PRD §3.5 and `backups/reservas-YYYYMMDD.db.gz`). The Go code's internal default string (`appointments.db`) remains as the fallback for dev direct-runs; no functional rename in Go (consistent with D1). Docs mentioning `appointments.db` as a production path are corrected.

**D6 — `curl | bash` without a TTY degrades explicitly.**
`install.sh` checks `[ -t 0 ]`: interactive setup flow runs only with a TTY; non-TTY requires the setup JSONs (or a completed checkpoint) to already exist and proceeds straight to the deploy pipeline, printing a clear Spanish/English message when it must stop instead ("run `bash install.sh` in a terminal first to complete setup"). Never block silently, never assume answers to prompts. *Tradeoff:* a piped install on a virgin VPS cannot complete — correct, because business data must come from an interactive session anyway.

**Additional resolved interface (from Q4):** `install.sh` writes `~/.config/mcp-appointments-crm/.env` with `MCP_BIND=127.0.0.1`, `MCP_PORT=3000` — created if absent, **never overwritten**. Precedence remains `env vars > .env > defaults` (binary semantics, ADR-0007, untouched). Bind stays loopback-only (`127.0.0.0/8`, `::1`); the installer never writes a non-loopback bind.

## Risks

1. **Extending a 1053-line script without breaking Fase 4.** Mitigation: extend-only (no rewrite of prompt/validator/finalize), shunit2 suites gate every step, `--setup-only` behavior unchanged.
2. **Go binary touched in a "scripts" phase (D2).** Mitigation: single isolated flag, ~20 LOC, own unit test; no path/env semantics change.
3. **Download/verification failures on flaky VPS networks.** Mitigation: SHA256 verification with `checksums.txt`, atomic write pattern for the binary, explicit error with the URL to fetch manually.
4. **systemd user-session quirks across distros** (no user bus over plain ssh, linger behavior). Mitigation: linger enabled by installer; troubleshooting section in `installation.md`/`maintenance.md` covers the known `systemctl --user` cases.
5. **DB split-brain risk** (someone runs the binary manually alongside the service, hitting `./data/appointments.db`). Mitigation: post-install log states the exact `MCP_DB_PATH` in use; `maintenance.md` documents the manual-run caveat.
6. **Windows customer expectation.** Mitigation: NSSM placeholder + Spanish docs set the boundary explicitly; automation is a declared non-goal for this phase.

## Rollback Plan (< 5 minutes, per host)

All changes are additive files plus an extend-only modification of `install.sh`; the Go change is a single isolated flag.

1. `systemctl --user stop mcp-appointments-crm && systemctl --user disable mcp-appointments-crm && rm ~/.config/systemd/user/mcp-appointments-crm.service && systemctl --user daemon-reload` (Linux; equivalent `launchctl unload` for macOS).
2. `rm "$BIN_DIR/mcp-server"` — no other system location is touched; user-level only, never root (ADR-0002).
3. Revert the git commit(s): Fase 4 `install.sh` (with `--setup-only`) becomes fully functional again; delete `backup.sh`, `setup/service/`, new docs.
4. Data and config are **never** removed by rollback: `~/.local/share/mcp-appointments-crm/reservas.db`, `~/.config/mcp-appointments-crm/` (setup JSONs, `.env`) stay intact, so re-running the previous installer or a fixed one loses no bookings.
5. `loginctl disable-linger $USER` only if the operator wants to undo the linger side effect (harmless to keep).

## Definition of Done (measurable)

1. `curl -fsSL <url> | bash -s -- --version vX.Y.Z` on a **clean Ubuntu 22.04+ VPS with no TTY** and with setup JSONs present exits 0 and leaves `systemctl --user is-active mcp-appointments-crm` → `active` within **< 5 minutes** total.
2. With no setup JSONs present, `install.sh` exits non-zero with a specific message naming the missing file(s) (`setup_business.json` / `setup_staff.json` / `setup_services.json`) — tested in shunit2.
3. OS/arch matrix tested: `uname -s/-m` maps to exactly the 4 GoReleaser assets (`mcp-appointments-crm_{Linux,Darwin}_{x86_64,arm64}.tar.gz`); unmapped combos fail with a clear error.
4. A tampered binary (one flipped byte) makes `install.sh` fail SHA256 verification, exit non-zero, and leave no binary in `BIN_DIR` (atomic write / cleanup verified).
5. `systemctl --user` shows the unit `enabled` and `active`; after a simulated logout (lingered session) and reboot, the service restarts automatically.
6. `loginctl show-user $USER` reports `Linger=yes` after install on Linux.
7. Final output includes a copy-pasteable `backup.sh` command line referencing the actual `MCP_DB_PATH` in use.
8. Final output includes the "Recommended additional tools" block (non-empty, in Spanish).
9. Final output includes the MCP endpoint URL (`http://127.0.0.1:3000/mcp`) and the resolved DB path.
10. `bash scripts/backup.sh` produces `backups/reservas-YYYYMMDD.db.gz` in the data dir, and the gzipped file restores (`gunzip` + `sqlite3 .recover`/integrity check passes) on both Linux and macOS.
11. `backup.sh` runs clean with only bash, sqlite3, gzip on PATH (checked against a minimal CI-like container/matrix).
12. `docs/installation.md` exists in Spanish and its commands are verifiable against the same clean-VPS flow of item 1 (a reviewer can follow it top-to-bottom without guessing).
13. `docs/maintenance.md` exists in Spanish covering: backup execution/restore, upgrade (`--version vX.Y.Z` re-run), log inspection (journal + `LOG_DIR`), service control per OS, and the manual-run DB caveat (D5/D1).
14. `setup/service/` contains the systemd user unit, launchd plist, and NSSM placeholder; the systemd unit carries `EnvironmentFile=%h/.config/mcp-appointments-crm/.env` and `MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db`.

**Gate for all items:** existing shunit2 suites (`install_validators_test.sh`, `install_e2e_test.sh`) pass unmodified; new tests added in the same shunit2 style.

## Success Criteria

- A single documented command (installation.md) goes from clean VPS to active, reboot-surviving service in under 5 minutes.
- Every environment artifact lands in the PRD §3.5 layout (config, data, logs separated), eliminating the DB split-brain risk for service-managed deployments.
- No regression in Fase 4 interactive setup (tests unmodified and green).
- The support handover is complete: installation + maintenance docs in Spanish, backup tested on both OSes, `--version` interfaces (installer and binary) exist as documented.

## Technical Notes

- Go binary: pure Go SQLite (`modernc.org/sqlite`, `CGO_ENABLED=0`), version stamped via `ldflags` into `buildinfo.Version` — the D2 flag just prints it.
- Release URL pattern: `https://github.com/egkike/mcp-appointments-crm/releases/download/{tag}/{asset}`; tag format `vMAJOR.MINOR.PATCH` (annotated), no `latest`.
- Upgrade path: re-running `install.sh --version <newer>` replaces the binary atomically and restarts the service; documented in maintenance.md.
- `.env` handling: create-if-absent with `MCP_BIND=127.0.0.1` / `MCP_PORT=3000`; never overwrite (user customizations survive upgrades).

## Review Budget

~300–400 LOC across 1–2 PRs: `install.sh` extension is the bulk; Go flag, backup.sh, templates, and docs are individually small but numerous. Docs review is content-level (Spanish, correctness against DoD), not line-level.

## Open Questions

None — Q1–Q5 from the init proposal were resolved by sdd-explore (2026-09-06) and the six risks are decided in the Decision Log above. Any revision to D1/D2/D5 (the customer-visible decisions) should happen in spec review before tasks are generated.
