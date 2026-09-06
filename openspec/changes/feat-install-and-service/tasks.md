# Tasks — feat-install-and-service

**Change:** feat-install-and-service
**Phase:** 5 — tasks
**Status:** ready for apply
**skill_resolution:** paths-injected (gentle-ai, chained-pr, work-unit-commits)
**Inputs leídos:** proposal.md, design.md, 5 specs (install-service 13 REQ, backup 4, service-units 5, install-docs 4, binary-version 3), openspec/config.yaml, `cmd/mcp-server/main.go`, `internal/buildinfo/buildinfo.go`, `scripts/tests/run_tests.sh`.

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1250–1400 (additions + deletions) |
| 400-line budget risk | **High** |
| Chained PRs recommended | **Yes** |
| Suggested split | PR 1 → PR 2 → PR 3 → PR 4 (see below) |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
```

### Forecast breakdown

| Component | Estimated LOC | Notes |
|---|---|---|
| `cmd/mcp-server/main.go` + test | ~50 | Go flag + unit test |
| `setup/service/` (3 templates) | ~70 | systemd unit + launchd plist + NSSM md |
| `scripts/backup.sh` | ~60 | New script |
| `scripts/tests/backup_test.sh` | ~100 | shunit2 suite |
| `scripts/install.sh` (deploy extension) | ~380 | ~12 new functions + main/usage extend |
| `scripts/tests/install_deploy_test.sh` | ~280 | shunit2 suite (large matrix) |
| `docs/installation.md` | ~180 | Spanish, step-by-step |
| `docs/maintenance.md` | ~160 | Spanish, annual operations |
| `docs/deployment.md` + `docs/PRD.md` alignment | ~20 | Minor edits (reservas.db) |
| **Total** | **~1300** | |

### Suggested PR chain (stacked-to-main)

| PR | Scope | Est. LOC | Budget status |
|---|---|---|---|
| **PR 1** | `feat(binary): add --version flag` — T1 | ~50 | ✅ Under budget |
| **PR 2** | `feat(install): add service templates and backup script` — T2 + T3 | ~330 | ✅ Under budget |
| **PR 3** | `feat(install): extend install.sh with deploy pipeline and tests` — T4 + T5 | ~660 | ⚠️ size:exception — one cohesive work unit (deploy pipeline + its tests); honest split would break the pipeline mid-function |
| **PR 4** | `docs: add installation and maintenance manuals` — T6 + T7 | ~360 | ✅ Under budget |

**PR 3 rationale for size:exception:** The deploy pipeline (`run_deploy` + 12 helper functions) and its shunit2 tests form a single deliverable behavior — "piped install downloads, verifies, installs binary, registers service, verifies running." Splitting the functions across two PRs would leave an intermediate state where the binary is downloaded but no service is registered (broken deliverable), or the test file would be artificially halved (tests reference functions from both halves). After one honest slicing pass, this is the minimum cohesive unit. Recommend `size:exception` for PR 3.

---

## Task Legend

- **🔵 Go** = touches Go source
- **🟢 Bash** = touches shell scripts
- **🟡 Templates** = service unit templates
- **📝 Docs** = documentation only
- **🧪 Test** = test creation/extension
- **✅ Verify** = verification gate

---

## T1 — Binary `--version` flag (Go)

**Scope:** REQ-BVER-001, REQ-BVER-002, REQ-BVER-003
**Files:** `cmd/mcp-server/main.go`, `cmd/mcp-server/main_test.go` (new)
**Dependencies:** none
**Type:** 🔵 Go + 🧪 Test

### - [x] T1.1 — RED: write test for `--version` flag

- **REQ:** REQ-BVER-001, REQ-BVER-002
- **Files:** `cmd/mcp-server/main_test.go` (new)
- **Description:** Extract a pure helper `wantsVersion(args []string) bool` concept (or test via `exec` of built binary). Write tests:
  - `["mcp-server", "--version"]` → true, output equals `buildinfo.Version`
  - No args → false (byte-identical semantics to Fase 4)
  - Other args → false
  - If exec-based: build binary, run with `--version`, assert stdout = version string, exit 0, no port listening
- **Done:** test file exists, tests compile, `go test ./cmd/mcp-server/...` FAILS (RED)

### - [x] T1.2 — GREEN: implement `--version` guard in `main.go`

- **REQ:** REQ-BVER-001, REQ-BVER-002
- **Files:** `cmd/mcp-server/main.go`
- **Description:** Add before `run()` call in `main()`:
  ```go
  if len(os.Args) > 1 && os.Args[1] == "--version" {
      fmt.Println(buildinfo.Version)
      return
  }
  ```
  - No `flag` package introduction (REQ-BVER-002)
  - Zero changes to `internal/mcp/config.go`, `internal/db/`, or defaults
  - Import `buildinfo` if not already imported
- **Done:** `go test -v -race ./cmd/mcp-server/...` PASSES (GREEN); `go build` succeeds; no regression in `go test -v -race ./...`

### - [x] T1.3 — VERIFY: Go quality gates

- **REQ:** REQ-BVER-002
- **Command:** `go fmt ./... && go vet ./... && go build -o /dev/null ./... && go test -v -race ./...`
- **Done:** all green, no fmt diff, no vet warnings

---

## T2 — Service templates

**Scope:** REQ-SU-001, REQ-SU-002, REQ-SU-003, REQ-SU-004, REQ-SU-005
**Files:** `setup/service/mcp-appointments-crm.service`, `setup/service/com.mcp.appointments.server.plist`, `setup/service/nssm-install.md`
**Dependencies:** none (independent of T1)
**Type:** 🟡 Templates

### - [x] T2.1 — Create systemd user unit template

- **REQ:** REQ-SU-001, REQ-SU-002, REQ-SU-005
- **Files:** `setup/service/mcp-appointments-crm.service` (new)
- **Description:** Create systemd user unit with:
  - `[Unit]` section: Description, `After=network.target`
  - `[Service]` section: `Type=simple`, `EnvironmentFile=%h/.config/mcp-appointments-crm/.env`, `Environment=MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db` (literal, per D9), `ExecStart=@@BIN_DIR@@/mcp-server`, `Restart=on-failure`, `RestartSec=5`
  - `[Install]` section: `WantedBy=default.target`
  - No `User=` directive, no `/etc` or `/usr` paths (user-level, REQ-SU-002)
  - `%h` specifier for home-portability (REQ-SU-005)
- **Done:** file exists, content matches design §5.3.1 exactly

### - [x] T2.2 — Create launchd plist template

- **REQ:** REQ-SU-001, REQ-SU-003, REQ-SU-005
- **Files:** `setup/service/com.mcp.appointments.server.plist` (new)
- **Description:** Create launchd plist with:
  - Label: `com.mcp.appointments.server`
  - ProgramArguments: `@@BIN_DIR@@/mcp-server`
  - EnvironmentVariables: `MCP_DB_PATH` = `@@DATA_DIR@@/reservas.db`
  - `RunAtLoad` = true, `KeepAlive` = true
  - StandardOutPath/StandardErrorPath: `@@LOG_DIR@@/mcp-server.out.log` / `.err.log`
  - Valid XML, paths with spaces handled (macOS `Library/Application Support/...`)
- **Done:** file exists, valid XML, content matches design §5.3.2

### - [x] T2.3 — Create NSSM placeholder document

- **REQ:** REQ-SU-001, REQ-SU-004
- **Files:** `setup/service/nssm-install.md` (new)
- **Description:** Spanish document covering:
  - Layout: `%APPDATA%\MCP Appointments CRM\`
  - `MCP_DB_PATH` corresponding path
  - NSSM command line equivalent (`nssm install MCPAppointmentsCRM ...`)
  - Explicit statement: installer does NOT automate Windows in Fase 5
- **Done:** file exists, in Spanish, exactly 3 files in `setup/service/` (REQ-SU-001)

### - [x] T2.4 — VERIFY: templates exist and match contracts

- **REQ:** REQ-SU-001..005
- **Done:** `ls setup/service/` shows exactly 3 files; grep confirms key directives (`EnvironmentFile`, `MCP_DB_PATH`, `@@BIN_DIR@@`, `Label`, `RunAtLoad`)

---

## T3 — Backup script

**Scope:** REQ-BKP-001, REQ-BKP-002, REQ-BKP-003, REQ-BKP-004
**Files:** `scripts/backup.sh` (new), `scripts/tests/backup_test.sh` (new)
**Dependencies:** none (independent of T1, T2)
**Type:** 🟢 Bash + 🧪 Test

### - [x] T3.1 — RED: write `backup_test.sh` shunit2 suite

- **REQ:** REQ-BKP-001, REQ-BKP-002, REQ-BKP-003, REQ-BKP-004
- **Files:** `scripts/tests/backup_test.sh` (new)
- **Description:** shunit2 suite (same style as existing `install_*_test.sh`):
  - `test_prereqs_missing_fails`: PATH without `sqlite3` → non-zero, names missing tool, no `backups/` created, DB untouched
  - `test_happy_path`: fixture DB (create with `sqlite3` + insert row) → `backups/reservas-YYYYMMDD.db.gz` exists; `gunzip` → `PRAGMA integrity_check` = `ok`; SELECT returns the row
  - `test_db_not_found`: nonexistent path → non-zero, path in message, no `backups/`
  - `test_rerun_same_day`: re-run → exit 0, file updated (REQ-BKP-002 scenario 3)
  - `test_no_scheduling`: `crontab -l` / `systemctl --user list-timers` before/after → no diff (REQ-BKP-004)
  - Uses `mktemp -d` for fixture isolation, cleanup on EXIT
- **Done:** test file exists, `bash scripts/tests/backup_test.sh` FAILS (RED — script doesn't exist yet)

### - [x] T3.2 — GREEN: implement `scripts/backup.sh`

- **REQ:** REQ-BKP-001, REQ-BKP-002, REQ-BKP-003, REQ-BKP-004
- **Files:** `scripts/backup.sh` (new, ~60 lines)
- **Description:** Per design §5.2:
  - `#!/bin/bash`, `set -u`, `umask 077`, NO `pipefail`/`set -e`, Bash 3.2 floor
  - `trap EXIT/INT/TERM/HUP` → `rm -f "$TMP"` (existing pattern)
  - `usage()` → stderr
  - `main()`: validate arg, check prereqs (`sqlite3`, `gzip` — name missing), check DB exists, compute `BACKUP_DIR`, `STAMP`, `FINAL`, `TMP` (mktemp same partition), `sqlite3 .backup`, `gzip -c`, `rm` uncompressed, `mv` atomic
  - Output: `Backup: $FINAL (N bytes)`
  - `chmod +x`
- **Done:** `bash scripts/tests/backup_test.sh` PASSES (GREEN); all 5 test functions pass

### - [x] T3.3 — VERIFY: backup tests + existing suites unmodified

- **REQ:** REQ-BKP-001..004, REQ-INS-012
- **Command:** `bash scripts/tests/run_tests.sh` (runs all `*_test.sh`)
- **Done:** backup_test.sh green; `install_validators_test.sh` and `install_e2e_test.sh` pass unmodified (`git diff` empty on both)

---

## T4 — Install pipeline: core deploy functions

**Scope:** REQ-INS-001, REQ-INS-002, REQ-INS-003, REQ-INS-004, REQ-INS-005, REQ-INS-006, REQ-INS-007, REQ-INS-012
**Files:** `scripts/install.sh` (extend), `scripts/tests/install_deploy_test.sh` (new)
**Dependencies:** T1 (binary must support `--version`), T2 (templates must exist for render tests)
**Type:** 🟢 Bash + 🧪 Test

### - [x] T4.1 — RED: write `install_deploy_test.sh` (core functions)

- **REQ:** REQ-INS-002, REQ-INS-004, REQ-INS-006, REQ-INS-007, REQ-INS-012
- **Files:** `scripts/tests/install_deploy_test.sh` (new)
- **Description:** shunit2 suite covering core deploy functions (source `install.sh` functions in test context):
  - `test_compose_asset_name_matrix`: 4 combos (Linux/Darwin × x86_64/arm64) → exact asset names; unmapped (`Windows_NT`/`i686`) → non-zero naming the combo (REQ-INS-002)
  - `test_validate_tag`: `v0.3.0` → ok; `latest`, `0.3.0`, `vX`, empty → non-zero (REQ-INS-001)
  - `test_require_setup_files_missing`: 0/1/3 JSONs → exit code + message naming each missing file (REQ-INS-004)
  - `test_resolve_paths_linux_default`: layout without XDG overrides (REQ-INS-006)
  - `test_resolve_paths_linux_xdg`: with `XDG_DATA_HOME`/`XDG_BIN_HOME`/`XDG_STATE_HOME` set (REQ-INS-006)
  - `test_resolve_paths_darwin`: `HOME` fixture + Darwin detection → `Library/...` paths (REQ-INS-006)
  - `test_resolve_paths_symlink_rejected`: symlink in `DATA_DIR` → rejection (REQ-INS-006)
  - `test_ensure_env_file_creates`: no `.env` → creates with `MCP_BIND=127.0.0.1`/`MCP_PORT=3000`, perms 0600 (REQ-INS-007)
  - `test_ensure_env_file_preserves`: existing `.env` with `MCP_PORT=3100` → byte-identical after re-run (REQ-INS-007)
  - `test_sha256_valid_fixture`: fixture release → passes verification (REQ-INS-003)
  - `test_sha256_tampered_fixture`: 1-byte-flipped archive → non-zero, no binary in `BIN_DIR` (REQ-INS-003, DoD 4)
  - `test_sha256_missing_in_checksums`: `checksums.txt` without asset line → non-zero explicit (REQ-INS-003)
  - Uses `MCP_RELEASE_BASE` pointing to local fixture dir; `HOME`/`CONFIG_DIR` fixture via `mktemp -d`
- **Done:** test file exists, `bash scripts/tests/install_deploy_test.sh` FAILS (RED — functions don't exist yet)

### - [x] T4.2 — GREEN: implement core deploy functions in `install.sh`

- **REQ:** REQ-INS-001, REQ-INS-002, REQ-INS-003, REQ-INS-004, REQ-INS-005, REQ-INS-006, REQ-INS-007, REQ-INS-012
- **Files:** `scripts/install.sh` (extend-only — new section "Deploy pipeline")
- **Description:** Add new functions (between existing entry points and `usage()`, or after `usage()` — maintain existing section structure):
  1. **`validate_tag()`**: `v<digits>.<digits>.<digits>` via `case`/`expr` (Bash 3.2); rejects `latest`, pre-release; Spanish error
  2. **`resolve_paths()` extension**: add sets for `DATA_DIR`, `BIN_DIR`, `LOG_DIR`, `ENV_FILE` per OS (D4/D8); extend symlink checks to new dirs; **do not modify** existing `CONFIG_DIR` sets
  3. **`refuse_root()`**: `id -u` = 0 → abort with Spanish message (D11)
  4. **`require_deploy_prereqs()`**: check `curl`, `tar`, sha256 tool (`sha256sum` | `shasum -a 256`); name missing
  5. **`require_setup_files()`**: check 3 JSONs in `CONFIG_DIR`; name each missing (positional params, no arrays)
  6. **`detect_platform()`**: `uname -s`/`uname -m` → call `compose_asset_name`
  7. **`compose_asset_name(os, arch)`**: pure function, 4-combo map + fail for unmapped (D10)
  8. **`download_and_verify()`**: curl asset + `checksums.txt` to `CURRENT_TMP`; `sha256_file()` dispatch; compare against checksums line; 3 distinguishable failures with URL
  9. **`extract_archive()`**: `tar -xzf` to `EXTRACT_DIR`
  10. **`ensure_dirs()`**: `DATA_DIR`/`BIN_DIR`/`LOG_DIR` → `mkdir -p` 0700
  11. **`ensure_env_file()`**: create-if-absent with `MCP_BIND=127.0.0.1`/`MCP_PORT=3000`, 0600; never overwrite (D8)
  12. **`install_binary()`**: `cp` → `${BIN_DIR}/.mcp-server.new.$$` → `chmod 0755` → `mv` (atomic, same partition)
  - Extend `main()`: new `--version` case → `validate_tag` + `run_deploy` (stub `run_deploy` that calls implemented steps 1-12; service registration in T5)
  - Extend `usage()`: document `--version vX.Y.Z`
  - Add `run_setup_guard_tty()`: `[ -t 0 ]` → `run_setup`; else → clear message + exit non-zero (REQ-INS-005)
  - **CONSTRAINTS:** Bash 3.2 floor (no arrays for setup file names, no `mapfile`, no `declare -n`, no `${var,,}`); `set -u`, `umask 077`; existing trap/cleanup unchanged; `RELEASE_BASE_URL` overridable via `MCP_RELEASE_BASE` (D10)
- **Done:** `bash scripts/tests/install_deploy_test.sh` PASSES for core function tests (GREEN); existing `install_validators_test.sh` + `install_e2e_test.sh` pass unmodified

### - [x] T4.3 — VERIFY: core deploy tests + no regression

- **REQ:** REQ-INS-012
- **Command:** `bash scripts/tests/run_tests.sh`
- **Done:** all 3 suites green (validators, e2e, deploy core); `git diff` on existing suites is empty

---

## T5 — Install pipeline: service registration, verification, summary

**Scope:** REQ-INS-008, REQ-INS-009, REQ-INS-010, REQ-INS-011, REQ-INS-013
**Files:** `scripts/install.sh` (extend), `scripts/tests/install_deploy_test.sh` (extend)
**Dependencies:** T4 (core functions must exist)
**Type:** 🟢 Bash + 🧪 Test

### - [x] T5.1 — RED: extend `install_deploy_test.sh` (service + verify + summary)

- **REQ:** REQ-INS-008, REQ-INS-009, REQ-INS-010, REQ-INS-011
- **Files:** `scripts/tests/install_deploy_test.sh` (extend)
- **Description:** Add test functions:
  - `test_render_template_default`: `@@BIN_DIR@@` substituted; `%h` literals preserved in default render (D9, REQ-SU-002)
  - `test_render_template_xdg_data`: with `XDG_DATA_HOME` custom → `MCP_DB_PATH` line rewritten to resolved absolute path (D9, REQ-SU-005)
  - `test_render_template_source_archive`: template resolved from `EXTRACT_DIR/setup/service/` first (D7)
  - `test_render_template_source_repo_fallback`: template resolved from repo `setup/service/` when archive lacks it (D7)
  - `test_verify_install_version_match`: mock `mcp-server --version` output → matches tag (REQ-INS-011)
  - `test_verify_install_version_mismatch`: mock output ≠ tag → non-zero (REQ-INS-011)
  - `test_post_install_summary_content`: output contains backup.sh line, tools block (Spanish), URL `127.0.0.1:3000/mcp`, `MCP_DB_PATH` (REQ-INS-010)
  - `test_run_setup_guard_tty_no_tty`: stdin `</dev/null` → non-zero + message, no hang (timeout wrapper) (REQ-INS-005)
- **Done:** new tests FAIL (RED — functions not yet implemented)

### - [x] T5.2 — GREEN: implement service registration, verification, summary

- **REQ:** REQ-INS-008, REQ-INS-009, REQ-INS-010, REQ-INS-011, REQ-INS-013
- **Files:** `scripts/install.sh` (extend)
- **Description:** Add/complete functions:
  1. **`install_service()`** (Linux path):
     - Resolve template: `${EXTRACT_DIR}/setup/service/` → fallback `${SCRIPT_DIR%/scripts}/setup/service/` (D7)
     - Render: `@@BIN_DIR@@` always substituted; `MCP_DB_PATH` line only if `DATA_DIR` non-default (D9)
     - `atomic_write` to `~/.config/systemd/user/mcp-appointments-crm.service` (mkdir -p)
     - `systemctl --user daemon-reload`
     - Detect prior install → `enable --now` (fresh) or `restart` (upgrade) (REQ-INS-013)
     - `loginctl enable-linger "$USER"` (REQ-INS-008)
  2. **`install_service()`** (macOS path):
     - Render plist: `@@BIN_DIR@@`, `@@DATA_DIR@@`, `@@LOG_DIR@@` substituted
     - `atomic_write` to `~/Library/LaunchAgents/com.mcp.appointments.server.plist`
     - `launchctl bootout gui/$UID/... 2>/dev/null || true` (idempotent)
     - `launchctl bootstrap gui/$UID <plist>` (REQ-INS-009)
  3. **`verify_install()`**: `$BIN_DIR/mcp-server --version` output contains `$INSTALL_TAG`; Linux: `systemctl --user is-active` with ≤10s wait loop (REQ-INS-011)
  4. **`print_post_install_summary()`**: (a) copy-pasteable `backup.sh` line with real `DATA_DIR/reservas.db`; (b) "Recommended additional tools" block in Spanish (jq, sqlite3, ufw, hermes doctor); (c) URL `http://127.0.0.1:3000/mcp` + `MCP_DB_PATH` + caveat (REQ-INS-010)
  5. **Complete `run_deploy()`**: wire steps 1-13 in order per design §5.1.2; each step fails → exit non-zero with Spanish message; trap cleans `CURRENT_TMP`
  6. **Wire `main()` dispatch**: `--version` → `validate_tag` + `run_deploy`; `--setup-only`/`""` → `run_setup_guard_tty`; unknown → error
- **Done:** `bash scripts/tests/install_deploy_test.sh` ALL tests PASS (GREEN); `run_deploy()` is complete end-to-end

### - [x] T5.3 — VERIFY: all suites green, no regression

- **REQ:** REQ-INS-012, REQ-INS-013
- **Command:** `bash scripts/tests/run_tests.sh`
- **Done:** all 3 suites green; existing suites unmodified (`git diff` empty on `install_validators_test.sh`, `install_e2e_test.sh`)

---

## T6 — Documentation

**Scope:** REQ-IDOC-001, REQ-IDOC-002, REQ-IDOC-003, REQ-IDOC-004, REQ-BVER-003
**Files:** `docs/installation.md` (new), `docs/maintenance.md` (new), `docs/deployment.md` (edit), `docs/PRD.md` (edit)
**Dependencies:** T4+T5 (docs reference actual commands/paths from the pipeline)
**Type:** 📝 Docs

### T6.1 — Write `docs/installation.md`

- **REQ:** REQ-IDOC-001, REQ-BVER-003
- **Files:** `docs/installation.md` (new, ~180 lines)
- **Description:** Spanish, customer-facing, step-by-step:
  - Prerequisites (Ubuntu 22+/macOS, bash, curl, ssh access)
  - Interactive setup: `bash install.sh` (terminal required, TTY)
  - Deploy: `curl -fsSL <url> | bash -s -- --version vTag`
  - Verification: `systemctl --user is-active`, endpoint check, `mcp-server --version`, DB location
  - Caveat D1/D5: manual `./mcp-server` uses different DB path; service is the supported path
  - Distinction: `install.sh --version` (installer) vs `mcp-server --version` (binary) (REQ-BVER-003)
  - No TUI reference as install step (ADR-0008, REQ-IDOC-001)
  - Reference to `maintenance.md` for ongoing operations
- **Done:** file exists, in Spanish, commands verifiable against DoD 1 flow

### T6.2 — Write `docs/maintenance.md`

- **REQ:** REQ-IDOC-002, REQ-IDOC-003, REQ-IDOC-004
- **Files:** `docs/maintenance.md` (new, ~160 lines)
- **Description:** Spanish, annual operations manual:
  - Backups: execute `backup.sh` (exact line with `MCP_DB_PATH`), restore with `gunzip` + `integrity_check`, data verification (REQ-BKP-002/003)
  - Upgrade: re-run `--version` newer tag; what's preserved (`.env`/DB/JSONs); downgrade path (REQ-INS-013)
  - Logs: `journalctl --user -u …` (Linux) + `LOG_DIR` paths (macOS) (REQ-IDOC-002c)
  - Service control per OS: start/stop/status/restart for systemd user and launchctl (REQ-IDOC-002)
  - Caveat DB manual: never run `./mcp-server` alongside service (D1/D5, REQ-IDOC-002e)
  - Troubleshooting: `systemctl --user` without user bus over ssh (`XDG_RUNTIME_DIR`), linger lost, port occupied, SHA256 mismatch
  - Optional scheduling: cron/systemd timer guide for `backup.sh`, marked as customer decision (REQ-IDOC-004, REQ-BKP-004)
  - `reservas.db` unified naming (REQ-IDOC-003)
- **Done:** file exists, in Spanish, covers all sections above

### T6.3 — Align `docs/deployment.md` and `docs/PRD.md`

- **REQ:** REQ-IDOC-003, REQ-BVER-003
- **Files:** `docs/deployment.md` (edit), `docs/PRD.md` (edit)
- **Description:**
  - `deployment.md`: unify `reservas.db` naming where used as production path; clarify `--version` format (string pelado vs aspirational — design §13 item 3)
  - `PRD.md`: any `appointments.db` production references → `reservas.db` with dev caveat
  - Minimal edits only — no restructure
- **Done:** grep confirms no stale `appointments.db` in production context; `reservas.db` used consistently

---

## T7 — Final verification

**Scope:** all REQs, DoD 1-14
**Dependencies:** T1-T6 complete
**Type:** ✅ Verify

### T7.1 — Go quality gates

- **Command:** `go fmt ./... && go vet ./... && go build -o /dev/null ./... && go test -v -race ./...`
- **Done:** all green

### T7.2 — Shell test suites

- **Command:** `bash scripts/tests/run_tests.sh`
- **Done:** all suites green (install_validators, install_e2e, install_deploy, backup); existing suites unmodified

### T7.3 — File inventory check

- **Done:** verify all expected files exist:
  - `scripts/install.sh` (extended)
  - `scripts/backup.sh` (new, executable)
  - `setup/service/mcp-appointments-crm.service`
  - `setup/service/com.mcp.appointments.server.plist`
  - `setup/service/nssm-install.md`
  - `scripts/tests/install_deploy_test.sh`
  - `scripts/tests/backup_test.sh`
  - `docs/installation.md`
  - `docs/maintenance.md`
  - `cmd/mcp-server/main_test.go`

### T7.4 — Manual VM checklist (DoD 1, 5, 6, 9)

- **REQ:** DoD 1, 5, 6, 9
- **Description:** On a clean Ubuntu 22.04+ VM:
  1. Complete interactive setup: `bash install.sh`
  2. Deploy: `bash install.sh --version vTag` (or piped equivalent with fixture)
  3. Verify: `systemctl --user is-active mcp-appointments-crm` → `active`
  4. Verify: `loginctl show-user $USER` → `Linger=yes`
  5. Verify: endpoint `curl http://127.0.0.1:3000/mcp` responds
  6. Verify: `mcp-server --version` outputs tag
  7. Simulate logout/reboot → service restarts
  8. Run `backup.sh` → restore → integrity check
  9. Record commands + outputs in verify report
- **Done:** all checks pass; verify report artifact created

---

## Dependency Graph

```text
T1 (Go --version)  ──────────────────┐
                                      ├──→ T4 (core deploy) ──→ T5 (service+verify) ──→ T6 (docs) ──→ T7 (final verify)
T2 (templates)     ──────────────────┘         ↑
                                                │
T3 (backup)        ────────────────────────────┘ (tests reference backup.sh existence)
```

**Critical path:** T1 → T4 → T5 → T6 → T7
**Parallelizable:** T1, T2, T3 can proceed in parallel (no interdependencies)

---

## PR Mapping

| Tasks | PR | Branch suggestion |
|---|---|---|
| T1 | PR 1 | `feat/binary-version-flag` |
| T2 + T3 | PR 2 | `feat/service-templates-and-backup` |
| T4 + T5 | PR 3 | `feat/install-deploy-pipeline` (size:exception) |
| T6 + T7 | PR 4 | `docs/installation-and-maintenance` |
