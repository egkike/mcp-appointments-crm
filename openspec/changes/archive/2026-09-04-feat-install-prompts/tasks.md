# Tasks: feat-install-prompts (Fase 4)

> Change: feat-install-prompts
> Inputs: `proposal.md`, `specs/install-setup/spec.md` (12 REQs, 41 scenarios), `design.md`
> Store: openspec. Chain: stacked-to-main. TDD: strict. Base: `main` @ `3208e6e`.

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated changed lines | ~700 authored LOC (script + tests); vendored shunit2 excluded |
| 400-line budget risk | Medium — split across 2 PRs at ~380 each |
| Chained PRs recommended | Yes |
| Suggested split | PR 1 (skeleton + validators + unit tests, ~380) → PR 2 (checkpoint + prompt engine + finalization + E2E, ~380) |
| Delivery strategy | ask-on-risk |
| Chain strategy | stacked-to-main |

```text
Decision needed before apply: No
Chained PRs recommended: Yes
Chain strategy: stacked-to-main
400-line budget risk: Medium
```

---

## Work Unit 1 — PR 1: Skeleton + Validators + Unit Tests

**Goal:** Ship `scripts/install.sh` with all pure-function infrastructure (string helpers, 16 validators, transforms, `json_escape`/`json_unescape`, `atomic_write`, path resolution) and a full unit-test suite. The script is sourceable without executing the flow. `main --help` works; the interactive setup flow is a stub printing "disponible en el siguiente PR".

**Files:**
- `scripts/install.sh` (new, ~250 LOC)
- `scripts/tests/lib/shunit2` (new, vendored — excluded from line count)
- `scripts/tests/run_tests.sh` (new, ~20 LOC)
- `scripts/tests/install_validators_test.sh` (new, ~130 LOC)

**Estimated lines:** ~380 authored (excl. vendored shunit2)

**Test harness:**
- Framework: shunit2 2.1.x vendored at `scripts/tests/lib/shunit2`
- Runner: `bash scripts/tests/run_tests.sh`
- Test-only dependency: `python3` for JSON parseability assertions (never a runtime dep)

**Verification:**
- `bash -n scripts/install.sh` — syntax clean
- `bash scripts/tests/run_tests.sh` — all unit tests green
- Source guard: `source scripts/install.sh` does NOT execute `main`

**Rollback:** `git revert` of PR 1 removes script + tests. No existing file is modified. Zero operational impact — no installer exists today.

---

### PR 1 Tasks (TDD: RED → GREEN per batch)

#### Task 1.1 — Test infrastructure scaffold

**TDD:** N/A (harness setup, no production code yet)

Create the test harness so every subsequent task can run RED → GREEN:

1. Download shunit2 2.1.x source into `scripts/tests/lib/shunit2` (single file, unmodified upstream)
2. Create `scripts/tests/run_tests.sh`:
   - Discover and execute every `*_test.sh` under `scripts/tests/`
   - Exit non-zero if any suite fails
   - Print per-suite pass/fail summary
3. Verify: `bash scripts/tests/run_tests.sh` runs zero tests and exits 0

**Files:** `scripts/tests/lib/shunit2`, `scripts/tests/run_tests.sh`

---

#### Task 1.2 — String helpers + tests (RED → GREEN)

**TDD:** Write `install_validators_test.sh` with failing tests for `trim_value`, `str_toupper`, `is_blank` → implement in `install.sh` → tests pass.

Implement and test pure string helpers (design §4):

| Function | Contract |
|---|---|
| `trim_value <s>` | Strip leading/trailing whitespace including trailing CR (T7) |
| `str_toupper <s>` | Case-based a-z→A-Z translation (no `tr`, no `${var^^}` — bash 3.2) |
| `is_blank <s>` | Exit 0 if empty or whitespace-only |

Tests cover: empty input, CR stripping, accented chars pass through `trim_value`, `str_toupper` with mixed case, `is_blank` with spaces/tabs.

**Files:** `scripts/install.sh` (helpers section), `scripts/tests/install_validators_test.sh`

---

#### Task 1.3 — Validators batch 1: basic format (RED → GREEN)

**TDD:** Write failing tests for each validator → implement → tests pass.

Implement and test the first validator batch (design §4 table):

| Validator | Rule | Key test cases |
|---|---|---|
| `v_nonempty` | non-blank after trim | empty→fail, spaces→fail, "x"→pass |
| `v_country` | `^[A-Za-z]{2}$` | "ar"→pass, "ARG"→fail, ""→fail |
| `v_email` | `^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$` | "no-arroba"→fail, "a@b.c"→pass |
| `v_phone` | `^\+[0-9]{8,15}$` | "+5491122334455"→pass, "1234"→fail |
| `v_url` | `^https?://[^[:space:]]+$` | "https://x.com"→pass, "ftp://x"→fail |
| `v_messenger_platform` | `whatsapp` or `telegram` | "sms"→fail, "whatsapp"→pass |
| `v_symbol` | non-blank after trim | ""→fail, "$"→pass |

Each validator: exit 0 valid / exit 1 invalid, Spanish error to stderr on failure. Pure: no reads, no writes, no globals.

**Files:** `scripts/install.sh` (validators section), `scripts/tests/install_validators_test.sh`

---

#### Task 1.4 — Validators batch 2: numeric + complex (RED → GREEN)

**TDD:** Write failing tests → implement → tests pass.

| Validator | Rule | Key test cases |
|---|---|---|
| `v_latitude` | decimal in [-90, 90], pure bash | "95"→fail, "90.0"→pass, "-90"→pass, ""→fail |
| `v_longitude` | decimal in [-180, 180], pure bash | "180"→pass, "-180.5"→fail |
| `v_currency` | `^[A-Za-z]{3}$` | "ARS"→pass, "ar"→fail |
| `v_timezone` | IANA path shape `^[A-Z][A-Za-z0-9_+\-]*(/...){0,2}$` | "UTC"→pass, "America/Argentina/Buenos_Aires"→pass, "GMT-3"→fail |
| `v_positive_int` | `^[0-9]+$` and > 0, `$((10#$n))` for octal safety | "0"→fail, "30"→pass, "09"→pass (not octal) |
| `v_price` | `^[0-9]+(\.[0-9]+)?$`, ≥ 0 | "0"→pass, "1500.50"→pass, "-100"→fail |
| `v_payment_list` | comma-split, ≥ 1 non-empty trimmed item | ""→fail, "efectivo, tarjeta"→pass, ",,"→fail |
| `v_hhmm` | `^([01][0-9]|2[0-3]):[0-5][0-9]$` | "09:00"→pass, "9:00 AM"→fail, "25:00"→fail |
| `v_time_pair <s> <e>` | both `v_hhmm` + start < end (minutes math, `$((10#...))`) | "09:00"/"18:00"→pass, "18:00"/"09:00"→fail |

Decimal range checks: match `^-?[0-9]+(\.[0-9]+)?$`, strip sign, split int/frac, integer-compare against bound; when int equals bound, fraction must be all zeros. No `awk`, no `bc`.

**Files:** `scripts/install.sh` (validators section), `scripts/tests/install_validators_test.sh`

---

#### Task 1.5 — Transforms (RED → GREEN)

**TDD:** Write failing tests → implement → tests pass.

| Transform | Contract |
|---|---|
| `t_trim <s>` | = `trim_value` (alias for registry use) |
| `t_upper <s>` | = `str_toupper` (applied after `v_country`/`v_currency` validation) |

Tests: `t_upper "ar"` → `"AR"`, `t_upper "ars"` → `"ARS"`, `t_trim "  x  "` → `"x"`.

**Files:** `scripts/install.sh`, `scripts/tests/install_validators_test.sh`

---

#### Task 1.6 — `json_escape` / `json_unescape` + tests (RED → GREEN) (REQ-IS-10)

**TDD:** Write failing tests for every escape case → implement → tests pass.

`json_escape <string>` → stdout (design §6.6):
- `LC_ALL=C` scoped locally
- Iterate bytes via `${s:i:1}`
- `\` → `\\`, `"` → `\"`, newline → `\n`, tab → `\t`, CR → `\r`
- Bytes < 0x20 (other controls) and 0x7F → `\u00XX`
- Bytes ≥ 0x80 → verbatim (UTF-8 multibyte passthrough)

`json_unescape <string>` → stdout (exact inverse, only fed own output).

Tests must cover:
- Quotes: `D'Átelier "La Casa"` → parseable round-trip
- Backslashes: `a\b` → `a\\b` → round-trip
- Embedded newline/tab
- Emoji: `Corte ✂️ clásico` → verbatim round-trip
- Accented UTF-8: `ñ`, `Á`, `é`
- Control bytes (0x01, 0x1F)
- Empty string

**Files:** `scripts/install.sh` (JSON helpers section), `scripts/tests/install_validators_test.sh`

---

#### Task 1.7 — `atomic_write` + path resolution + tests (RED → GREEN)

**TDD:** Write failing tests → implement → tests pass.

`atomic_write <dest>` (design §6.2):
- Read content from stdin into `<dest>.new.$$` (same directory)
- `mv -f` to `<dest>` (POSIX-atomic on same filesystem)
- Global `CURRENT_TMP` + EXIT trap `rm -f` guard

Path resolution function `resolve_paths` (design §5):
- Detect OS via `uname -s`
- Linux: `${XDG_CONFIG_HOME:-$HOME/.config}/mcp-appointments-crm`
- macOS: `$HOME/Library/Application Support/MCP Appointments CRM`
- `$HOME` unset → Spanish fatal error, exit 1
- Symlink guard: config root exists and is symlink → Spanish fatal, exit 1
- Variables: `CONFIG_DIR`, `SETUP_DIR`, `CHECKPOINT_PATH`

Tests:
- `atomic_write`: write to temp dir, verify content, verify no leftover `.new.$$`
- Path resolution: mock `uname -s` for Linux/macOS, verify XDG respected on Linux, native path on macOS
- Symlink guard: create symlink, verify fatal exit

**Files:** `scripts/install.sh` (atomic_write + path resolution), `scripts/tests/install_validators_test.sh`

---

#### Task 1.8 — Script skeleton + source guard + `main --help` (REQ-IS-12)

**TDD:** N/A (structural, no new testable logic)

Complete the script skeleton so PR 1 is a valid, runnable file:

1. Header comment documenting bash 3.2 floor and forbidden constructs (design §2)
2. Guards: `umask 077`, `set -u` (no `set -e`, no `pipefail`) — design D4/D5
3. Source guard: `[ "${BASH_SOURCE[0]}" = "$0" ] && main "$@"`
4. `main` function:
   - `--help` → Spanish usage, exit 0
   - `--setup-only` / no args → stub: print "Setup flow: disponible en el siguiente PR", exit 0
   - Unknown args → Spanish error, exit 1
5. Verify: `bash -n scripts/install.sh` clean, `source scripts/install.sh` does not execute main, `install.sh --help` prints usage

**Files:** `scripts/install.sh` (header, guards, main, source guard)

---

#### Task 1.9 — PR 1 integration verification

**TDD:** N/A (verification gate)

1. Run `bash scripts/tests/run_tests.sh` — all unit tests green
2. Run `bash -n scripts/install.sh` — syntax clean
3. Run `source scripts/install.sh` — no side effects
4. Run `scripts/install.sh --help` — Spanish usage, exit 0
5. Count authored lines (excl. vendored shunit2): must be ≤ 400
6. Verify no bash-4+ constructs: no `declare -A`, no `mapfile`, no `readarray`, no `declare -n`, no `${var,,}`, no `${var^^}`

**Rollback:** `git revert` of this PR removes `scripts/install.sh` + `scripts/tests/`. No existing file modified.

---

## Work Unit 2 — PR 2: Checkpoint + Prompt Engine + Finalization + E2E

**Goal:** Complete `scripts/install.sh` with the full interactive setup flow: flat store, checkpoint system, R/S/Q menu, REQ-IS-9 gate, prompt engine, all four section runners, summary confirm, JSON assembly, ordered finalization, optional self-check. Full E2E test suite covering all 10 scenarios.

**Files:**
- `scripts/install.sh` (extend, ~300 additional LOC)
- `scripts/tests/install_e2e_test.sh` (new, ~80 LOC)

**Estimated lines:** ~380 authored

**Depends on:** PR 1 merged to main.

**Test harness:** Same as PR 1 (`bash scripts/tests/run_tests.sh` now also runs E2E suite).

**Verification:**
- All unit tests (PR 1) still green
- All E2E scenarios green
- Proposal success criteria demonstrable locally
- `go test -v -race ./...` still green (no Go changes)

**Rollback:** `git revert` of PR 2 removes the interactive flow + E2E tests; PR 1's skeleton remains. Operator-machine artifacts (setup JSONs, checkpoint) are inert data files, hand-deletable.

---

### PR 2 Tasks (TDD: RED → GREEN per batch)

#### Task 2.1 — Flat dotted-key store + tests (RED → GREEN)

**TDD:** Write failing tests for store operations → implement → tests pass.

Implement the flat store (design §6.2):

| Function | Contract |
|---|---|
| `store_set <key> <value>` | Set/overwrite key in `STORE` variable (newline-separated `key=value`) |
| `store_get <key>` | Print value for key, empty if absent |
| `store_has <key>` | Exit 0 if key present |
| `store_unset <key>` | Remove key from store |
| `checkpoint_render` | Render current store as valid JSON to stdout (flat dotted keys, values by type) |

Store is one variable `STORE` with newline-separated `key=value` lines. Literal `null` marks answered-blank-optional. Registry controls key names — no user input in keys (Threat T2).

Tests: set/get/has/unset, overwrite-in-place, checkpoint render produces valid JSON, null values render as JSON `null`, dotted keys render quoted.

**Files:** `scripts/install.sh` (flat store section), `scripts/tests/install_validators_test.sh`

---

#### Task 2.2 — Checkpoint save/load + tests (RED → GREEN)

**TDD:** Write failing tests → implement → tests pass.

`checkpoint_save` (design §6.2):
- Call `checkpoint_render` → pipe to `atomic_write "$CHECKPOINT_PATH"`
- Called after every valid answer (REQ-IS-6)

`checkpoint_load`:
- Read `setup.json.tmp` line-by-line
- Match `^  "([^"]+)": (.*)$`
- Split key/value, `json_unescape` string values
- `version != 1` → return error (degraded mode)
- Unknown keys → ignored with warning

Tests:
- Save → file parses as JSON → content matches store
- Load after save → store restored exactly
- Round-trip with escaped values (quotes, unicode)
- Unknown version → error return
- Missing file → no error, empty store

**Files:** `scripts/install.sh` (checkpoint I/O section), `scripts/tests/install_validators_test.sh`

---

#### Task 2.3 — Field registry (data, no tests needed)

**TDD:** N/A (data definition)

Define the canonical field registry as parallel indexed arrays (design §6.1):

```
BP_KEYS, BP_TYPES, BP_REQUIRED, BP_VALID, BP_DEFAULTS
```

18 business-profile fields in fixed order. Same pattern for staff subfields (`name role_specialty phone email` + schedule) and service subfields (`name description duration_minutes price`).

Also define day-name mapping for business hours (monday..sunday) and day-of-week integer mapping (0=Sunday..6=Saturday, §3.7.5).

> Add regression guard in `install_validators_test.sh` verifying BP_KEYS order equals spec REQ-IS-2 field order (18 fields) — BP_KEYS is the source of truth for resume order, so a drift against the spec table breaks resume.

**Files:** `scripts/install.sh` (constants & field registry section)

---

#### Task 2.4 — R/S/Q menu + REQ-IS-9 gate + tests (RED → GREEN)

**TDD:** Write failing E2E tests for menu behavior → implement → tests pass.

`prompt_rsq` (design §6.2):
- Checkpoint exists → offer `[R]eaudar / [S]tart over / [Q]uit`
- R → `checkpoint_load` + resume revalidation (D6) → return "resume"
- S → delete checkpoint → return "start_over"
- Q → exit 0 → return "quit" (actually exits)
- Invalid letter → re-ask

`prompt_reconfigure` (REQ-IS-9):
- Three JSONs exist, no checkpoint → ask confirmation
- Y → proceed with setup flow
- N → exit 0, nothing modified

Tests (via E2E stdin-driven):
- Checkpoint present → R/S/Q prompt appears before any field prompt
- S → checkpoint deleted
- Q → exit 0, nothing modified
- Three JSONs present, no checkpoint → confirmation prompt
- Decline → exit 0, files untouched

**Files:** `scripts/install.sh` (entry points section), `scripts/tests/install_e2e_test.sh`

---

#### Task 2.5 — Prompt engine + tests (RED → GREEN)

**TDD:** Write failing tests for prompt behavior → implement → tests pass.

`prompt_field <var> <prompt-text> <validator> <mode> [default]` (design §4):
- Read one line via `read -r`
- `trim_value` (including CR — T7)
- Blank + default → apply default
- Blank + optional → store `null` sentinel
- Blank + required → Spanish error, re-ask
- Non-blank → run validator; on failure print stderr, re-ask
- `read` failure (EOF) → Spanish cancel message, exit 1, checkpoint preserved (REQ-IS-7)

After valid answer: apply transform if registered → `store_set` → `checkpoint_save`.

Tests (E2E stdin-driven):
- Valid input → stored + checkpoint updated
- Invalid input → Spanish error on stderr, same field re-asked
- Blank optional → null stored
- Blank with default → default stored
- EOF → exit 1, checkpoint preserved

**Files:** `scripts/install.sh` (prompt engine section), `scripts/tests/install_e2e_test.sh`

---

#### Task 2.6 — Business profile section + tests (RED → GREEN)

**TDD:** Write failing E2E test for business profile flow → implement → tests pass.

`run_business_section`:
- Iterate `BP_KEYS` in order, call `prompt_field` for each
- After section completes, all `business.<field>` keys present in store

Tests (E2E):
- Feed all 18 business-profile answers → all keys present in checkpoint
- Invalid email mid-flow → Spanish error, re-ask, does not advance
- Blank `website_url` → `"website_url": null` in checkpoint
- Blank `timezone` with default → `"UTC"` in checkpoint
- `ar` country → `"AR"` (uppercase transform)
- `efectivo, tarjeta , transferencia` → payment list stored correctly

**Files:** `scripts/install.sh` (section runners), `scripts/tests/install_e2e_test.sh`

---

#### Task 2.7 — Business hours section + tests (RED → GREEN)

**TDD:** Write failing E2E test → implement → tests pass.

`run_hours_section`:
- 7 day prompts (monday..sunday)
- Per day: accept closed marker or `HH:MM`–`HH:MM` pair
- Use `v_hhmm` + `v_time_pair` (start strictly earlier than close)
- Store as `hours.<day>.open` / `hours.<day>.close`; closed day → no keys

Tests:
- Saturday 09:00–13:00, Sunday closed → correct keys in checkpoint
- Malformed time `9:00 AM` → Spanish error, re-ask
- Open ≥ close → Spanish error, re-ask

**Files:** `scripts/install.sh` (section runners), `scripts/tests/install_e2e_test.sh`

---

#### Task 2.8 — Staff section + tests (RED → GREEN)

**TDD:** Write failing E2E test → implement → tests pass.

`run_staff_section` (REQ-IS-4):
- Loop: prompt professional (name required, role_specialty/phone/email optional)
- After basic fields: 7 weekday schedule prompts (not-working or HH:MM–HH:MM)
- Store as `staff.<i>.name`, `staff.<i>.sched.<day>.open/.close`
- After each complete professional: offer add-another
- ≥1 professional required: blank name on "add another" when zero captured → Spanish refusal
- `status` defaults to `active` without prompting

Tests:
- One professional Mon–Fri 09:00–18:00, Sat–Sun not-working → 5 schedule entries, day_of_week 1..5
- Zero professionals → blank name refused, loop continues
- Invalid schedule range → Spanish error, re-ask
- Status defaults to `active`

**Files:** `scripts/install.sh` (section runners), `scripts/tests/install_e2e_test.sh`

---

#### Task 2.9 — Services section + tests (RED → GREEN)

**TDD:** Write failing E2E test → implement → tests pass.

`run_services_section` (REQ-IS-5):
- Loop: prompt service (name required, description optional, duration_minutes required positive int, price required decimal ≥ 0)
- Store as `services.<i>.name`, etc.
- After each complete service: offer add-another
- ≥1 service required
- `is_active` defaults to `1` without prompting

Tests:
- One service with all fields → correct keys in checkpoint
- Zero services → refused, loop continues
- Price `0` → accepted
- Price `-100` → Spanish error, re-ask
- Duration `0` → Spanish error, re-ask

**Files:** `scripts/install.sh` (section runners), `scripts/tests/install_e2e_test.sh`

---

#### Task 2.10 — JSON assembly + finalization + tests (RED → GREEN)

**TDD:** Write failing E2E tests for finalization behavior → implement → tests pass.

Assembly functions (design §6.4):
- `render_setup_business` → JSON object with 18 fields + `business_hours` nested object
- `render_setup_staff` → JSON array with schedule arrays (day_of_week 0..6, 0=Sunday)
- `render_setup_services` → JSON array with `is_active: 1`

`finalize` (design §6.5, REQ-IS-8):
1. `atomic_write setup_business.json`
2. `atomic_write setup_staff.json`
3. `atomic_write setup_services.json`
4. `rm -f "$CHECKPOINT_PATH"` (strictly last)
5. Optional self-check (`command -v jq || command -v python3`; skip silently if absent)
6. Spanish success message with absolute paths

Tests:
- Fresh happy path: all prompts → 3 parseable JSONs, correct shapes, checkpoint gone, dirs 0700/files 0600, no `id` keys
- Checkpoint removed only after all three files exist
- Mid-finalization resume: craft checkpoint fixture + one pre-existing JSON → R → all three rewritten, checkpoint removed
- Adversarial input: `D'Átelier "La Casa"` → parseable JSON, name verbatim
- Unicode: `Corte ✂️ clásico` → parseable JSON, name verbatim
- No `jq`/`python3` → files produced without error

**Files:** `scripts/install.sh` (assembly & finalization section), `scripts/tests/install_e2e_test.sh`

---

#### Task 2.11 — Summary confirm (D8) + Spanish copy

**TDD:** N/A (UX addition, covered by E2E tests in 2.10+)

After services loop, before finalization:
- Display Spanish summary of all captured values
- Prompt `[S]í / [n]o`
- Decline → exit 1, checkpoint preserved (resumable)
- Accept → proceed to `finalize`

Ensure ALL prompts, defaults, validation errors, and menu options are in Spanish (UTF-8). Verify REQ-IS-11 compliance across all text output.

**Files:** `scripts/install.sh` (summary section + all Spanish strings)

---

#### Task 2.12 — Resume position derivation + tests (RED → GREEN)

**TDD:** Write failing E2E tests for resume behavior → implement → tests pass.

Resume logic (design §6.3):
- No stored cursor; position derived from key presence in registry order
- Business: first `business.<field>` absent → resume there
- Hours: first `hours.<day>` without `.open` → resume that day
- Staff: highest `staff.<i>` index; entry complete ⇔ name present AND all 7 sched days decided; incomplete → resume at first unanswered subfield
- Services: same rule (complete ⇔ `duration_minutes` and `price` present)
- Everything complete → skip to summary confirm + re-finalize

Resume revalidation (D6): each restored value re-run through its validator; failed → discarded and re-prompted.

Tests (E2E):
- Cancel mid-staff → re-run R → no re-ask of business fields or completed professional
- Cancel mid-services → re-run R → no re-ask of completed professionals
- Kill mid-finalization → re-run R → summary → re-finalize
- Tampered checkpoint (invalid email) → value discarded, field re-prompted

**Files:** `scripts/install.sh` (resume logic), `scripts/tests/install_e2e_test.sh`

---

#### Task 2.13 — PR 2 integration verification (REQ-IS-12)

**TDD:** N/A (verification gate)

1. Run `bash scripts/tests/run_tests.sh` — ALL tests green (unit + E2E)
2. Run `bash -n scripts/install.sh` — syntax clean
3. Run `go test -v -race ./...` — still green (no Go changes)
4. Count total authored lines (excl. vendored shunit2): must be ≤ 400 for PR 2
5. Verify no bash-4+ constructs in new code
6. Manual check: all prompts/errors in Spanish
7. Verify proposal success criteria:
   - [ ] Fresh run → 3 valid JSONs + checkpoint removed (RF1 AC1)
   - [ ] Invalid email → Spanish error + re-ask (RF1 AC2)
   - [ ] Cancel + re-run → R/S/Q with skip-on-resume (RF1 AC3)
   - [ ] Every valid answer → atomic checkpoint rewrite
   - [ ] Finalization ordered: 3 JSONs then checkpoint removal
   - [ ] Zero professionals/services refused
   - [ ] Optional fields prompted; blank → null
   - [ ] Re-run with 3 JSONs → confirm-and-reconfigure
   - [ ] Adversarial inputs → parseable JSON
   - [ ] No Go surface touched

**Rollback:** `git revert` of PR 2 removes interactive flow + E2E tests. PR 1 skeleton remains functional. Operator-machine artifacts are inert data files.

---

## File Summary

| Path | PR | Status | Est. LOC |
|---|---|---|---|
| `scripts/install.sh` | 1+2 | new | ~550–650 total |
| `scripts/tests/lib/shunit2` | 1 | new (vendored) | excluded |
| `scripts/tests/run_tests.sh` | 1 | new | ~20 |
| `scripts/tests/install_validators_test.sh` | 1 | new | ~130 |
| `scripts/tests/install_e2e_test.sh` | 2 | new | ~80 |

**No existing file is modified.** Zero Go changes.
