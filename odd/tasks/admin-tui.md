# Admin TUI (identity & accounts) — ODD feature

**Status**: in_progress
**Entry point**: `mcp-server admin tui` (frozen, ADR-0016 §5; binary `mcp-server`, code in `cmd/mcp-server/admin_tui.go`)
**Scope authority**: ADR-0016 (docs/architecture/0016-admin-tui-scope.md), ADR-0009/0010/0011
**Workflow**: ODD (chosen over SDD 2026-09-16; ADR-0016 frozen scope serves as the design)
**TDD**: OFF (no TDD mode configured in this project); each task ships with focused repo-level tests following existing `go-sqlmock` patterns
**Review budget**: ~400 diff lines per PR (ADR-0016 D4) → chained PRs expected, no silent `size:exception`

## Objective

Give the local operator a Bubble Tea TUI sub-command that manages accounts so that a clean
install becomes usable **without manual SQL**. The system unblocker is the owner seed
gateway: today every authenticated MCP call is rejected with `-32000` "regístrate primero"
until an `accounts` row exists (currently created by hand — docs/installation.md §3.4,
docs/demo-plan.md:142).

## Scope (ADR-0016 Decision 1)

- Owner seed gateway (first owner + `caller-id` file)
- Add Staff (professional picker, no free text)
- Deactivate (soft delete)
- Transfer Ownership (2 steps, single-owner invariant)
- List views (read-only)
- Add Yourself as Client (ADR-0011)

## Non-goals (explicit, do not expand)

- Editing business profile / services / professionals / schedules → ADR-0015 (Hermes)
- `purge-inactive` → Phase 2+
- New MCP tools surface
- Replacing the `install.sh` wizard (ADR-0008)
- **Audit log view → DEFERRED** (owner decision 2026-09-16): audit is slog-only today; no
  persisted store exists. Follow-up change will introduce audit persistence + view.
- **Day-key normalization ("1".."7" vs 0..6) → DEFERRED to ADR-0015** (owner decision
  2026-09-16): this TUI never reads/writes day data.

## Verified design facts (scout + inline reads, 2026-09-16)

1. `hashCallerID` (SHA-256) is log-only PII scrubbing; `accounts.id` is stored in clear
   (the phone/handle). The `caller-id` file holds the raw caller id that Hermes sends in
   the `X-Caller-Id` header. — internal/auth/middleware.go:151, internal/auth/resolver.go:62
2. `caller-id` file is fully greenfield: no Go code reads/writes it; documented path in
   docs is `~/.config/mcp-appointments-crm/caller-id` (docs/architecture/0012-hermes-chat-local.md:22).
3. No non-HTTP `auth.Caller` construction path exists except exported `auth.WithCaller`
   (internal/auth/caller.go:28, used by tests). TUI = local privileged path that
   fabricates the Caller after OS-admin gatekeeping; DB triggers still enforce the
   single-owner invariant regardless of call path (internal/db/schema.go:225-247).
4. `AccountsRepo` is NOT constructed in `cmd/mcp-server/main.go` (only 6 other repos, :146-155);
   sub-command wiring and repo construction are new.
5. `TransferOwnership` does not exist; composed as: create new account `role=owner,
   is_active=0` (trigger counts only ACTIVE owners), then in one transaction deactivate
   old owner + activate new. Never two active owners.
6. Add-self-as-client must insert `clients.id = accounts.id` (the phone): CallerResolver
   step 2 looks up `SELECT id FROM clients WHERE id = ?` with the caller id
   (internal/auth/resolver.go:76). `GetOrCreate` inserts a UUID id — NOT usable here.
7. Phone validation: `accounts` has no phone column (id IS the phone); validation must
   reuse/extend `entity.Client.HasValidPhone` as the single validation point.
8. No FK `accounts.professional_id` → staff picker must list active professionals
   (`ProfessionalsRepo.FindActive`) and prefill phone from `professionals.phone`.
9. Bubble Tea (bubbletea/bubbles/lipgloss) NOT in go.mod → new dependency → RDD gate.
10. Tests mirror existing patterns: `go-sqlmock` (`newMockDB`) for repos/auth, tmp-file
    SQLite for integration (WAL pragma rejects `:memory:`).

## Tasks

- [x] **T1 — Sub-command wiring**: arg dispatch in `cmd/mcp-server/main.go` (`admin tui`
      before serve mode; `--version` preserved; unknown args → semantic error exit).
      AccountsRepo + ClientsRepo construction shared with serve mode (`newIdentityDeps`).
      No TUI internals yet. **Done 2026-09-16** (delegated writer, 516+/39- across
      main.go/main_test.go + 2 new files 150 lines).
      Checks OBSERVED: `go build -o /dev/null ./...` OK; `go test -race ./...` all ok
      (cmd included); `go vet ./...` clean; `gofmt -l` clean; `golangci-lint run ./...`
      0 issues; manual E2E of all dispatch cases by worker.
      Deviations accepted: unknown args (incl. `--help`) now fail fast (ADR-0016 §5);
      TUI path skips ValidateLoopback (no HTTP transport dependency, ADR-0016 D3.5);
      runHermesChat stub lives in main.go (reserved name).
      ⚠ Review budget: candidate is ~557 diff lines (> 400 budget; tests ≈66%) →
      owner accepted explicit `size:exception` for this slice at delivery.
- [x] **T2 — Owner seed gateway**: first-boot flow: detect zero active owners → guided
      console creation (phone validated via entity.Client.HasValidPhone, display_name) →
      `AccountsRepo.Create` under fabricated owner Caller (`admin.TUICaller`, ADR-0016 D3.1)
      → write `caller-id` file (0600, `~/.config/mcp-appointments-crm/caller-id`,
      `MCP_CONFIG_DIR` override). Owner exists → semantic message + exit 0, with
      `admin.EnsureCallerID` repair of a missing file (R4-partial-seed-wedge fix).
      **Done 2026-09-16** (delegated worker + 1 authorized assertion fix + lint fixes +
      1 bounded review correction, 197/200 diff lines).
      Checks OBSERVED: build OK, `go test -race` all green (internal/admin 19 tests + cmd),
      vet/gofmt clean, golangci-lint 0 issues.
      Native review: lineage review-2754520390d5344b (high, 4 lenses + refuter) →
      CRITICAL R4-partial-seed-wedge → 1 bounded correction (EnsureCallerID repair path)
      → targeted validator PASS → **approved**, acknowledged (rev c3e2744b).
- [ ] **T3 — Professional picker + Add Staff**: list active professionals
      (`FindActive`), prefill phone, validate against repo; create staff account.
      Checks: picker data mapping tests, staff creation repo tests (sqlmock).
- [ ] **T4 — Deactivate + List views**: soft delete with confirmation screen;
      read-only list views (all accounts, by role). Checks: repo tests, view-model tests.
- [ ] **T5 — Transfer Ownership**: 2-step flow (T5a create inactive owner, T5b swap in
      transaction), single-owner invariant verified, audit via existing slog attrs.
      Checks: invariant tests (attempt two active owners → conflict), transaction tests.
- [ ] **T6 — Add Yourself as Client**: insert `clients` row with `id = accounts.id`
      (phone), duplicate-phone → semantic conflict message; resolver round-trip test
      (resolve → ClientID set). Checks: resolver integration test (tmp SQLite).
- [ ] **T7 — TUI assembly**: Bubble Tea screens/state machine wiring all flows, key
      bindings, error rendering (semantic messages), Ctrl+C safety. go.mod deps added.
      Checks: `go build`, teatest-style or model-level unit tests where feasible.
- [ ] **T8 — Docs**: README + docs/installation.md §3.4 (owner creation now via TUI),
      docs/demo-plan.md manual-SQL step replaced. Docs-only commit.

## Acceptance criteria

- Clean install → `mcp-server admin tui` → owner created + caller-id file written →
  authenticated MCP call succeeds (no manual SQL).
- All ADR-0016 Decision 1 capabilities (minus the two deferred above) work locally.
- Single-owner invariant never violated (DB triggers + repo pre-checks).
- Full pipeline green: `go fmt`, `go vet`, `golangci-lint run`, `go build -o /dev/null ./...`,
  `go test -v -race ./...`.
- RDD gate per candidate (assess → consent/review if medium/high).

## Progress

- 2026-09-16: feature started; ADR-0016 + code verified; audit-view and day-keys deferred
  by owner decision; tasks T1-T8 planned. No source written yet.
- 2026-09-16: T1 done and verified (full pipeline green). Native RDD review ran on the
  candidate: lineage review-3855447bdd3ccb2e, risk high (process boundary in tests), 4
  lenses (risk/resilience/readability/reliability), 705 diff lines → **approved**;
  acknowledgement burned (store rev 88e6ea61). 8 non-blocking advisory findings recorded
  below as follow-ups. Pending owner call: commit/PR strategy.

## Review follow-ups (non-blocking, informational — fold into later tasks)

T1 (fold candidates: T7 assembly / T8 docs):
- R2-misleading-wiring-proof (WARNING) cmd/mcp-server/admin_tui.go — resolved by T2 rewrite.
- R4-1 (WARNING) cmd/mcp-server/admin_tui.go:19-24 — superseded by T2 rewrite.
- R2-implicit-serve-on-error main.go:149-163; R3-1 main_test.go:262-266; R3-2 main.go:404-407;
  R3-3 main.go:472-476; R4-2 main.go:88-92 (SUGGESTIONs).

T2 (lineage review-2754520390d5344b, corrected candidate):
- R4-deactivated-owner-seed-deadend (WARNING) internal/admin/seed.go:52-63 — NeedsSeed counts
  only ACTIVE owners; a deactivated owner blocks seeding but needs-seed says false. Candidate
  follow-up with T5 transfer flow.
- R2-conflict-classification (WARNING) internal/admin/seed.go:129-135; R3-001 (WARNING)
  cmd/mcp-server/admin_tui.go:57-64.
- R1-symlink-and-mode-window + R4-callerid-loose-mode-window (config.go:99-113); R3-002,
  R3-003, R2-unexplained-d1-ref, R2-unexplained-fact-ref, R4-ambiguous-post-commit-failure
  (SUGGESTIONs).

## Next step

T2 delivered (uncommitted): owner gate decision — commit/PR strategy for T2 (~1070 + 197
diff lines, tests ≈ 60%). Then T3 — professional picker + Add Staff.
