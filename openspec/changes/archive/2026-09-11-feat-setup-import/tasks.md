# Tasks: feat-setup-import — Seed DB from wizard JSONs on first boot

> **Change:** feat-setup-import
> **Status:** Tasks complete — ready for apply
> **Inputs:** `proposal.md`, `specs/setup-loader/spec.md`, `specs/setup-seeder/spec.md`, `design.md`
> **skill_resolution:** paths-injected (`golang-patterns`, `work-unit-commits`)

---

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~1050–1300 (≈530 impl + ≈520 tests + ≈50 golden fixtures + ≈20 wiring/docs) |
| 400-line budget risk | **High** |
| Chained PRs recommended | **Yes** |
| Suggested split | PR 1 → PR 2 → PR 3 (see phases below) |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main (each PR targets `main` via feature branches, merged in sequence) |

```text
Decision needed before apply: Yes
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: High
```

---

## Phase 1 — Setup resolver + structs + loader + business_hours mapping + unit tests

> **PR 1 candidate:** `feat-setup-import-loader`
> Self-contained: no DB dependency, no boot-path changes. Pure functions over injected deps + file I/O against `t.TempDir()`.
> Estimated ~700 changed lines (≈310 impl + ≈320 tests + ≈70 golden fixtures).

### TASK-1.1 — Setup JSON structs (`setup_types.go`)

- [x] Create `internal/config/setup_types.go` with concrete types pinning the 3 wizard JSON shapes:
  - `BusinessHoursEntry` (`open`, `close` strings)
  - `SetupBusiness` — 18 wizard fields + `BusinessHours map[string]*BusinessHoursEntry`
  - `SetupScheduleEntry` (`day_of_week int`, `start_time`, `end_time`)
  - `SetupStaffMember` (`name`, `role_specialty *string`, `status`, `email *string`, `phone *string`, `specialties []string`, `schedule []SetupScheduleEntry`)
  - `SetupService` (`name`, `description *string`, `duration_minutes int`, `price float64`, `is_active int` — **int, not bool**, ADR-SD-3)
  - `SetupData` — bundles the three decoded files
- [x] No field may be `any`/`interface{}`; pointer optionals exactly where wizard allows `null`
- [x] Verify: `go build ./internal/config/...` compiles

### TASK-1.2 — Per-OS setup directory resolver (`setup_resolver.go`)

- [x] Create `internal/config/setup_resolver.go` with:
  - `ResolveSetupDir() (string, error)` — exported, calls `resolveSetupDirOS(runtime.GOOS, os.Getenv)`
  - `resolveSetupDirOS(goos string, getenv func(string) string) (string, error)` — unexported core, injectable for tests
- [x] Resolution order (mirrors `install.sh resolve_paths` lines 542–580, **minus symlink checks** per ADR-SD-2):
  1. `MCP_SETUP_DIR` env wins on every OS (empty string = unset)
  2. `HOME` empty → Spanish error: `"no se puede resolver el directorio de setup: la variable HOME no está definida"`
  3. `darwin` → `$HOME/Library/Application Support/MCP Appointments CRM/setup`
  4. default (linux + other) → `${XDG_CONFIG_HOME:-$HOME/.config}/mcp-appointments-crm/setup`
- [x] Use `filepath.Join` + `filepath.Clean` for path composition
- [x] Verify: `go build ./internal/config/...` compiles

### TASK-1.3 — Resolver unit tests (`setup_resolver_test.go`)

- [x] Create `internal/config/setup_resolver_test.go` — table-driven over `(goos, env map[string]string)`:
  - Linux default (`HOME=/home/user`, no XDG, no MCP_SETUP_DIR) → `$HOME/.config/mcp-appointments-crm/setup`
  - Linux with `XDG_CONFIG_HOME=/custom/config` → `/custom/config/mcp-appointments-crm/setup`
  - macOS default → `$HOME/Library/Application Support/MCP Appointments CRM/setup`
  - `MCP_SETUP_DIR=/tmp/test-setup` wins on linux
  - `MCP_SETUP_DIR=/tmp/test-setup` wins on darwin
  - Empty `HOME` → error contains `"HOME no está definida"`
  - `windows`/unknown OS → XDG rule (spec fallback)
  - Empty `MCP_SETUP_DIR` treated as unset (falls through to per-OS default)
- [x] Run: `go test -v -race ./internal/config/...` — all pass

### TASK-1.4 — Golden test fixtures (`testdata/`)

- [x] Create `internal/config/testdata/setup_business.json` — hand-written, mirrors `install.sh finalize()` output exactly: 18 fields, `business_hours` with day-name keys (6 open + 1 null for sunday), `accepted_payment_methods` as JSON array
- [x] Create `internal/config/testdata/setup_staff.json` — 1 staff member, 2 schedule entries (e.g., monday + wednesday), `specialties: []`
- [x] Create `internal/config/testdata/setup_services.json` — 2 services, one with `price: 0` (pins D5), `is_active: 1` as integer
- [x] Verify fixtures decode with `json.Unmarshal` into the TASK-1.1 structs

### TASK-1.5 — Loader + business_hours mapping (`setup_loader.go`)

- [x] Create `internal/config/setup_loader.go` with:
  - `LoadSetup(dir string) (*SetupData, error)` — reads 3 files from dir, JSON-decodes each
  - `readFileCapped(path string, maxBytes int64) ([]byte, error)` — `os.Open` + `io.LimitReader(maxBytes+1)`; >maxBytes → error naming file + `"supera el tamaño máximo permitido (1 MiB)"`
  - `mapBusinessHours(in map[string]*BusinessHoursEntry) (string, error)` — pure function:
    - `dayNameToNumber` table: `monday→1 … sunday→7`
    - `nil` entry → skip (closed = absent key)
    - Unknown day name → error `"business_hours contiene un día desconocido: %q"`
    - HH:MM regex validation: `^([01]\d|2[0-3]):[0-5]\d$`
    - Output: `json.Marshal(map[string]BusinessHoursEntry)` → deterministic sorted keys
  - `validateForSeed(data *SetupData) error` — pre-tx shape checks (design §8.3):
    1. `Name` non-empty (trim) → `"el nombre del negocio no puede estar vacío"`
    2. `CurrencyCode`, `CurrencySymbol`, `Timezone` non-empty
    3. `SlotIntervalMinutes > 0`
    4. `AcceptedPaymentMethods` entries non-empty (if non-nil)
    5. Construct `entity.BusinessProfile` + call `Validate()` (defense-in-depth)
    6. Per staff: construct `entity.Professional` + call `Validate()`
    7. Per schedule: `0 ≤ day_of_week ≤ 6`, HH:MM, `start < end`
    8. Per service (NOT `entity.Service.Validate` — D5): name non-empty, `duration > 0`, `price >= 0`, `is_active ∈ {0,1}`
- [x] All errors are semantic Spanish, name the **file only** (no directory paths in error strings — ADR-SD-4)
- [x] Loader is strictly read-only: no `Create`/`Write`/`Chmod`/`Remove` calls
- [x] Verify: `go build ./internal/config/...` compiles

### TASK-1.6 — Loader + mapping unit tests (`setup_loader_test.go`, `setup_mapping_test.go`)

- [x] Create `internal/config/setup_loader_test.go`:
  - Happy path: load `testdata/` fixtures → all fields populated, no error
  - Missing file: remove `setup_staff.json` from `t.TempDir()` → error contains `"setup_staff.json"` + `"no existe"`
  - Malformed file: write invalid JSON to `setup_services.json` → error contains `"formato inválido"` + filename
  - Oversized file: write >1 MiB file → error contains `"supera el tamaño máximo permitido"`
  - Read-only: verify file content bytes + `ModTime` identical before/after load
- [x] Create `internal/config/setup_mapping_test.go`:
  - `monday` → key `"1"` with original `{open,close}`
  - `sunday` → key `"7"` with original `{open,close}`
  - `null` sunday → key `"7"` absent (and no `"sunday"`, `""`, or null)
  - Full week (6 open + 1 null) → exactly 6 numeric keys
  - All closed → `"{}"`
  - Unknown day name → error
  - Bad HH:MM → error
  - **Entity round-trip**: map output → `entity.BusinessProfile.BusinessHours` → `IsOpenOn(1)` true + `GetOpenClose(1)` = 09:00–18:00, `IsOpenOn(7)` false
- [x] Run: `go test -v -race ./internal/config/...` — all pass

### TASK-1.7 — Phase 1 verification gate

- [x] `go fmt ./internal/config/...` clean
- [x] `go vet ./internal/config/...` clean
- [x] `go build -o /dev/null ./...` passes
- [x] `go test -v -race ./internal/config/...` — all pass
- [x] Commit: `feat(config): add setup directory resolver, JSON loader, and business_hours mapping` (f81d333 on feat-setup-import-loader, PR #72)

---

## Phase 2 — Seeder transaction + guard + boot integration tests

> **PR 2 candidate:** `feat-setup-import-seeder`
> DB-dependent. Builds on Phase 1 types + loader. Uses file-based temp DB (`db.NewDatabase` against `t.TempDir()`).
> Estimated ~580 changed lines (≈230 impl + ≈350 tests).

### TASK-2.1 — Fresh guard + seeder (`setup_seeder.go`)

- [x] Create `internal/config/setup_seeder.go` with:
  - `SeedOnBoot(ctx context.Context, conn *sql.DB, logger *slog.Logger) error` — orchestrator:
    1. `isFreshDB(ctx, conn)` — guard FIRST (ADR-SD-9)
    2. Not fresh → `logger.Info("importación de setup omitida: ...")` → return nil
    3. Fresh → `ResolveSetupDir()` → `LoadSetup(dir)` → `validateForSeed(data)` → `seed(ctx, conn, data)`
    4. Success → `logger.Info("base de datos sembrada desde la configuración inicial", ...)`
  - `isFreshDB(ctx, conn) (bool, error)` — mirrors repo lazy-init:
    - `INSERT OR IGNORE INTO business_profile (id, name) VALUES ('singleton', '')`
    - `SELECT name FROM business_profile WHERE id = 'singleton'`
    - Return `name == ""`
  - `seed(ctx, conn, data) error` — single transaction:
    - `BEGIN` → `defer` rollback (committed flag pattern)
    - (1) `UPDATE business_profile SET ... WHERE id = 'singleton'` — 18 fields + mapped `business_hours` + `updated_at`; assert `RowsAffected == 1`
    - (2) Per staff: `idgen.NewUUID()` → `INSERT INTO professionals` → per schedule: `INSERT INTO schedules`
    - (3) Per service: `idgen.NewUUID()` → `INSERT INTO services` (FTS triggers fire)
    - `COMMIT`; set `committed = true`
  - `translateConstraint(err) error` — maps `UNIQUE constraint failed: schedules.professional_id, schedules.day_of_week` → `"ya existe un horario para ese día"`, etc.
  - Helpers: `statusOr(m SetupStaffMember) string`, `specialtiesJSON(ss []string) interface{}` (nil → SQL NULL, else `json.Marshal`)
- [x] All SQL uses `?` placeholders — zero string concatenation
- [x] All errors are semantic Spanish, wrapped with `fmt.Errorf("...: %w", err)`
- [x] Verify: `go build ./internal/config/...` compiles

### TASK-2.2 — Seeder unit tests (`setup_seeder_test.go`)

- [x] Create `internal/config/setup_seeder_test.go` — all tests use file-based DB via `db.NewDatabase(ctx, filepath.Join(t.TempDir(), "seed.db"))` (**never `:memory:`** — `verifyPragmas` requires WAL; pin as test-file comment per ADR-SD-10):
  - **(a) Happy path:** full seed from golden fixtures → assert:
    - Singleton `id='singleton'`, exactly 1 row, all 18 fields round-tripped
    - `business_hours` = `{"1":...}` JSON (numeric keys)
    - Professionals count matches staff JSON
    - Schedules count matches schedule entries
    - `services_fts MATCH` finds a seeded service (FTS sync verified)
  - **(b) Zero-price service:** `setup_services.json` with `price: 0` → seeds successfully (D5)
  - **(c) Rollback:** staff member with duplicate `day_of_week` in schedule → error returned; then verify:
    - `business_profile.name == ''` (singleton placeholder intact)
    - `COUNT(professionals) == 0`
    - `COUNT(services) == 0`
  - **(d) Guard:** fresh DB → `isFreshDB` returns true; after seed → false; after deleting all professionals → still false (no re-seed)
- [x] Run: `go test -v -race ./internal/config/...` — all pass

### TASK-2.3 — Boot integration tests (`setup_boot_test.go`)

- [x] Create `internal/config/setup_boot_test.go` — end-to-end `SeedOnBoot` tests (file-based DB):
  - Fresh + valid JSONs via `MCP_SETUP_DIR` env → seeded; second `SeedOnBoot` call → no-op, DB unchanged (guard pins no-re-seed)
  - Fresh + malformed JSON → error naming file, DB pre-seed state (`name == ''`, no professionals/services)
  - **Not fresh + missing setup dir entirely** → `nil` error, boots normally (guard-first: loader never runs)
  - Fresh + unset `HOME` (and no `MCP_SETUP_DIR`) → error with Spanish message
- [x] Run: `go test -v -race ./internal/config/...` — all pass

### TASK-2.4 — Phase 2 verification gate

- [x] `go fmt ./internal/config/...` clean
- [x] `go vet ./internal/config/...` clean
- [x] `go build -o /dev/null ./...` passes
- [x] `go test -v -race ./internal/config/...` — all pass
- [x] `golangci-lint run ./internal/config/...` clean
- [x] Commit: `feat(config): add first-boot seeder with transactional guard and rollback` (fdd373b on feat-setup-import-seeder, PR #73)

---

## Phase 3 — Boot wiring + docs + final pipeline verification

> **PR 3 candidate:** `feat-setup-import-wiring`
> Additive composition-root hook + docs update. Minimal diff (~20 lines).
> Estimated ~20 changed lines.

### TASK-3.1 — Wire `SeedOnBoot` in `cmd/mcp-server/main.go`

- [x] Edit `cmd/mcp-server/main.go` — add import for `internal/config`
- [x] Insert the seed hook **immediately after** the `defer database.Close()` block, **before** the `// ── Construct repositories` comment:
  ```go
  // Setup import (feat-setup-import): seed the DB from the wizard JSONs on
  // first boot. Runs in the single-threaded window before repo construction
  // and HTTP serving. No-op when the profile is already seeded; fatal when
  // the DB is fresh and the wizard output is missing or malformed.
  if err := config.SeedOnBoot(ctx, database.Conn, logger); err != nil {
      return fmt.Errorf("importar configuración inicial: %w", err)
  }
  ```
- [x] Verify: `go build -o /dev/null ./cmd/mcp-server/...` compiles
- [x] Verify: no existing tests broken — `go test -v -race ./...`

### TASK-3.2 — Update `docs/installation.md`

- [x] Edit `docs/installation.md` — after the Paso 1 JSON listing, add/replace wording:
  - State that the server siembra la DB automáticamente en el primer arranque si el perfil del negocio está vacío
  - Replace any "los JSONs son artefactos finales sin consumidor" implication
  - Mention that subsequent boots skip the seed (guard) and runtime edits are safe
- [x] Keep the document in Spanish (matching existing style)

### TASK-3.3 — Final pipeline verification

- [x] `go fmt ./...` clean
- [x] `go vet ./...` clean
- [x] `golangci-lint run ./...` clean
- [x] `go build -o /dev/null ./...` passes
- [x] `go test -v -race ./...` — all pass
- [x] Commit: `feat(cmd): wire first-boot seed hook and update installation docs` (committed on feat-setup-import-wiring, PR 3 — this commit)

---

## Dependency Graph

```text
TASK-1.1 (types)
    │
    ├── TASK-1.2 (resolver) ── TASK-1.3 (resolver tests)
    │
    ├── TASK-1.4 (golden fixtures)
    │
    └── TASK-1.5 (loader + mapping + validation) ── TASK-1.6 (loader/mapping tests)
                                                        │
                                                        TASK-1.7 (Phase 1 gate) ← PR 1 boundary
                                                        │
TASK-2.1 (seeder) ◄── Phase 1 types + loader
    │
    ├── TASK-2.2 (seeder tests)
    │
    └── TASK-2.3 (boot integration tests)
            │
            TASK-2.4 (Phase 2 gate) ← PR 2 boundary
            │
TASK-3.1 (main.go wiring) ◄── Phase 2 SeedOnBoot
    │
    ├── TASK-3.2 (docs)
    │
    └── TASK-3.3 (final pipeline) ← PR 3 boundary
```

## PR Chain Summary

| PR | Branch | Scope | Est. lines | Targets |
|----|--------|-------|-----------|---------|
| 1 | `feat-setup-import-loader` | Types + resolver + loader + mapping + all unit tests + golden fixtures | ~700 | `main` |
| 2 | `feat-setup-import-seeder` | Seeder + guard + seeder tests + boot integration tests | ~580 | `main` (after PR 1 merge) |
| 3 | `feat-setup-import-wiring` | `main.go` hook + `docs/installation.md` update | ~20 | `main` (after PR 2 merge) |

## Rollback Boundaries

| PR | Removable without affecting unrelated work |
|----|-------------------------------------------|
| 1 | Delete `internal/config/setup_types.go`, `setup_resolver.go`, `setup_loader.go`, `setup_*_test.go`, `testdata/` — no boot-path change, server behavior identical |
| 2 | Delete `internal/config/setup_seeder.go`, `setup_seeder_test.go`, `setup_boot_test.go` — no boot-path change, Phase 1 types still compile |
| 3 | Revert `main.go` hook + `docs/installation.md` — server returns to pre-change behavior (empty profile, manual SQL) |
