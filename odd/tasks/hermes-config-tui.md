# Feature: hermes-config-tui — TUI "configurar Hermes"

**Status**: COMPLETE (merged)
**Started**: 2026-09-24
**Branch**: `feat/hermes-config-tui` (code), docs lane for ADR
**Backlog**: obs 914 item 2 (micro-fixes done in obs 959/967)
**Origin**: v0.4.0 fresh-install smoke — new installs need a manual YAML bootstrap for Hermes
(`~/.hermes/config.yaml` + `X-Caller-Id` header; docs/demo-plan.md Paso 5). PRD §7 Fase N;
PRD risk R5 ("El dueño del negocio no sabe cómo configurar Hermes ni apuntarlo al MCP server").

## Decisions (owner, 2026-09-24)

1. **Output**: BOTH — primary = direct YAML merge write to `~/.hermes/config.yaml`;
   fallback = copy-paste snippet shown on screen when the write fails or `~/.hermes` is absent/unwritable.
2. **Header value format**: `X-Caller-Id: "+5491100000000"` — **quoted, with `+`** (exactly what Hermes
   accepted in the v0.4.0 smoke). YAML-typing rationale: unquoted `+54...` parses as an integer per
   YAML 1.1 int regex; quoting keeps it a string.
3. **Phone source**: prefill from the active owner (`admin.ActiveOwner`) with manual override
   (validated with the existing single validation point `admin.ValidatePhone` /
   `entity.Client.HasValidPhone`).
4. **Scope authority**: new small **ADR-0017** (`docs/architecture/0017-hermes-config-tui.md`),
   docs lane, before any code. ADR-0016 (identity & accounts only) explicitly does NOT cover this.

## Fixed design defaults (derived; not owner-blocking)

- **Merge semantics**: load existing `~/.hermes/config.yaml` (if present) into a generic map,
  set `mcp_servers.mcp-appointments = {url, headers{X-Caller-Id}}`, preserve every other key and
  server, atomic write, dir 0700 if we create it / file 0600 if we create it (no chmod on
  pre-existing Hermes-owned files).
- **Server key**: `mcp-appointments` (only name used in docs/demo-plan.md).
- **Endpoint source**: honor `MCP_BIND`/`MCP_PORT` via `mcp.LoadConfig` (already loaded in the
  composition root for the admin-TUI path but not yet passed into `tui.Deps`), default
  `http://127.0.0.1:3000/mcp` when unset. Path is always `/mcp`.
- **URL validation**: new validator in the admin core (`net/url.Parse` + host present; loopback
  warn-only is NOT in scope — validate format only).
- **YAML library**: `gopkg.in/yaml.v3` (merge semantics demand a real parser; no YAML lib exists
  in go.mod today → new dependency, part of the review surface).
- **Gate**: default routing → native review (new write surface + new dependency).
- **Idempotency**: re-running the flow updates the same `mcp_servers.mcp-appointments` entry;
  no backup file; snippet fallback shows the exact merged block when write fails.

## Constraints (from exploration, evidence in scout report)

- Two menu registration points must stay in sync: Bubble Tea `internal/tui/view.go menuItems()` +
  `internal/tui/model.go openMenuItem()`; console fallback `cmd/mcp-server/admin_tui.go
  adminMenuOptions()`. Scope-pin test `internal/tui/app_test.go TestMenuItemsMirrorTheFrozenScope`
  intentionally breaks on a new option — update it as part of T2.
- Framework-free core in `internal/admin` (no Bubble Tea imports), TUI owns zero business rules —
  same layering as seed/staff/transfer.
- `internal/validation` is a doc-only stub; do not use it.
- Business profile is NOT needed (Hermes config shape has no business name).
- The TUI does NOT emit JSON to stdout (that convention belongs to install.sh); flows render
  Spanish screens and write files directly (precedent: `admin.WriteCallerID`, 0600).
- `hermes chat` remains an unimplemented stub (`runHermesChat`, main.go) — non-goal here.
- Non-goals: WhatsApp per-sender bot, `hermes chat` implementation, Hermes install/detection,
  editing any other Hermes config field.

## Tasks

- [ ] **T0 — ADR-0017** (docs lane: branch → commit → ff to main, no PR): decision, format
      contract (quoted E.164-with-`+`), merge/idempotency contract, non-goals. Docs = read-only
      for this feature's review (committed before the code branch).
- [ ] **T1 — admin core** (`internal/admin/hermes.go` + tests): types for the Hermes config shape
      (generic map for unknown keys), load/merge/atomic-write, snippet renderer, URL + phone
      validation reuse, no YAML emit of unquoted `+` values (quote-force), unit tests incl.
      merge-with-existing-servers and YAML-type regression (value stays a string).
- [ ] **T2 — TUI wiring** (`internal/tui/`): new capability + screen + menu item + form
      (`formStep` phone prefill w/ override) + `tea.Cmd` calling the admin core + view screens
      (summary/snippet fallback) + update the scope-pin test. No business logic in the TUI.
- [ ] **T3 — composition root** (`cmd/mcp-server/admin_tui.go`, `cmd/mcp-server/main.go`):
      pass `mcp.Config` (bind/port) into the deps so the TUI can build the endpoint URL;
      extend `tui.Deps` with the Hermes-config port.
- [ ] **T4 — console fallback** (`adminMenuOptions` + action func): same capability, line-based
      flow reusing the same admin core.
- [ ] **T5 — close-out**: full pre-flight pipeline (fmt/vet/golangci/build/test -race), functional
      checks (fresh-path write, merge path, snippet fallback), native review gate (default
      routing), issue + issue-first PR, owner merge.

## Evidence

- T0 (ADR-0017): docs lane `74dc04b` on main (GGA n/a — docs excluded by pattern).
- T1 (admin core): `362dccc` — internal/admin/hermes.go + 25 tests; golangci 0; fresh GGA run
  STATUS: PASSED captured (/tmp/gga-t1-fresh.log) after the parse-window flake; trivial test
  comma-ok fix applied (GGA suggestion). go.mod gains gopkg.in/yaml.v3 v3.0.1.
- T2+T3 (TUI wiring + composition root): `6a76a6f` — capability/screen/form/cmds/view with
  snippet fallback, Deps.Hermes{Path, EndpointURL}, scope-pin test updated to 7 items.
  First commit attempt hit the GGA parse-window flake; fresh run returned substantive
  STATUS: FAILED with 2 pre-existing WARNINGs (duplicated presentation helpers across the
  two entry points; writeConsole ...any) + 1 SUGGESTION — all fixed in the same commit:
  internal/admin/presentation.go now owns AccountLabel / ListableRoles / ReactivationHint /
  SuccessorCandidates (single source for console + TUI), writeConsole carries an
  accepted-deviation comment (middleware.go:149 precedent), capacity hint corrected.
  Fresh GGA after fixes: STATUS: PASSED (/tmp/gga-t23-fix.log).
- T4 (console option): `d8246a9` — "7 / Configurar Hermes" in adminMenuOptions, shared
  resolveHermesBootstrap resolution site for both presentations, console tests
  (happy path with foreign-server preservation, invalid phone, no active owner,
  write-failure → snippet). Fresh GGA: STATUS: PASSED (/tmp/gga-t4-fresh.log).
- T5 (gate): native review review-eb6b809fafd682a0 (medium, single lens review-reliability,
  13 files, 2341 original lines, budget 200, committed range base 5818a81) — APPROVED first
  pass, no correction transition offered, receipt burned (authority: burned, store_revision
  aa954ecd). 4 informational findings (all non-blocking, later work):
  R3-hermes-eager-bootstrap (WARNING, admin_tui.go:167 — bootstrap resolved even when the
  capability is not used), R3-lossy-config-roundtrip (SUGGESTION, hermes.go:183 — unknown
  scalar shapes can be re-typed by the round-trip), R3-wildcard-bind-endpoint (WARNING,
  hermes.go:147-151 — MCP_BIND=0.0.0.0 would emit a non-connectable client URL),
  R3-windows-mode-assertions (WARNING, hermes_test.go:537-547 — mode assertions skipped on
  Windows). Issue #97 tracks the feature.
- **Merge**: squash-merged by owner as `c0fdba3` on main (PR #98, CI green); issue #97
  auto-closed; branch deleted local+remote; post-merge sanity green (build + test -race
  admin/tui/cmd).

## Notes

- Owner decisions recorded 2026-09-24 (output/format/phone/ADR).
- YAML quoting rationale recorded in ADR-0017 (the "+ becomes an int" YAML 1.1 trap).
- Gate may hit gentle-shell#1324 validator seam if a correction is required → fresh START
  workaround (validated 2x).
