# Verify Report: feat-install-prompts (Fase 4) — slice PR2a + review fixes

> Date: 2026-09-04 · Branch: `fix/install-perf-checkpoint` @ `259d219` · Store: openspec (Engram HTTP server DOWN — engram persistence unavailable this session)
> Scope verified: tasks + specs against the **implemented slice** (PR1 #55, PR2a #57 merged to main; fixes 6492acc + 259d219 on branch). Tasks + specs verified; design coherence checked lightly against design-referenced sections of the script.

## Status: PASS (for assigned slice PR1+PR2a) — ARCHIVE NOT READY (partial change)

The implemented slice conforms to the spec for the REQs in its scope. The change as a whole is **incomplete**: PR2b (tasks 2.7–2.13) is explicitly stubbed in `run_setup` (`"Secciones hours/staff/services: disponible en PR2b."`). REQ-IS-1, REQ-IS-3, REQ-IS-4, REQ-IS-5 and REQ-IS-8 are not implementable until PR2b lands.

## Spec coverage (per REQ)

| REQ | Status | Evidence |
|---|---|---|
| REQ-IS-1 (3 valid JSONs, checkpoint removed, 0700 dirs, no IDs) | NOT IMPLEMENTED | `finalize` / `render_setup_*` absent — PR2b scope (code comment `# PR2b: hours/staff/services, resumen y finalize (tasks 2.7-2.13).`) |
| REQ-IS-2 (business profile, per-field validation) | **PASS (slice)** | 18-field `BP_KEYS` registry in spec order (regression test `test_bp_keys_order` green); validators `v_nonempty/v_country/v_email/v_phone/v_url/v_messenger_platform/v_symbol/v_latitude/v_longitude/v_currency/v_timezone/v_positive_int/v_price/v_payment_list` implemented with Spanish stderr errors; `t_upper` transform for country/currency; defaults (ARS/$/UTC/30) applied on blank; blank optional → `null` sentinel; retry loop in `prompt_field` never advances on invalid. E2E: `test_invalid_email_reasks`, `test_blank_optional_and_defaults`, `test_fresh_happy_path` |
| REQ-IS-3 (business hours) | NOT IMPLEMENTED | `run_hours_section` absent (PR2b) — `prompt_day_hours` helper already exists and validates via `v_hhmm`+`v_time_pair` |
| REQ-IS-4 (≥1 professional, schedule) | NOT IMPLEMENTED | `run_staff_section` absent (PR2b) |
| REQ-IS-5 (≥1 service) | NOT IMPLEMENTED | `run_services_section` absent (PR2b) |
| REQ-IS-6 (atomic checkpoint after every valid answer) | **PASS (slice)** | `checkpoint_save` = `checkpoint_render | atomic_write` called in `prompt_field` and `prompt_day_hours` after every valid answer; `atomic_write` writes `<dest>.new.$$` in same dir + `mv -f`, EXIT/INT/TERM/HUP trap cleanup; per-field progress preserved on kill. List-entry progress (professionals/services) = PR2b |
| REQ-IS-7 (R/S/Q menu, resume, EOF=cancel) | **PASS (slice)** | `prompt_rsq` offered before any other prompt when checkpoint exists (run_setup order verified); Q exits 0 without modification; S deletes checkpoint; EOF → Spanish cancel message, exit 1, checkpoint preserved; resume revalidation via `checkpoint_load`+`revalidate_all` (D6). E2E: `test_rsq_start_over_deletes_checkpoint`, `test_rsq_quit_preserves_checkpoint`, `test_save_load_roundtrip` |
| REQ-IS-8 (JSONs first, checkpoint last) | NOT IMPLEMENTED | `finalize` absent (PR2b) |
| REQ-IS-9 (confirm-and-reconfigure) | **PASS (slice)** | `prompt_reconfigure` when 3 JSONs exist and no checkpoint; decline → exit 0, nothing modified. E2E: `test_reconfigure_decline_leaves_files` |
| REQ-IS-10 (adversarial input → parseable JSON) | **PASS (slice)** | `json_escape`: `\`→`\\`, `"`→`\"`, `\n/\r/\t`, controls <0x20 & 0x7F → `\u00XX`, UTF-8 ≥0x80 verbatim; `json_unescape` exact inverse. **checkpoint_render escapes BOTH keys and values** — key escaping confirmed at `scripts/install.sh:363` (`\"$(json_escape "$key")\":`, commit `259d219`, review lineage review-9b0da603 correction). Unit `test_json_escape_roundtrip` covers quotes/backslash/newline-tab/emoji `Corte ✂️ clásico`/accents/0x01/0x1F/empty; E2E uses `D'Átelier "La Casa"` and asserts parseable checkpoint |
| REQ-IS-11 (Spanish copy, UTF-8) | **PASS (slice)** | All prompts/errors/menus in Spanish (spot-checked across script) |
| REQ-IS-12 (bash 3.2, no required external tools) | **PASS (slice)** | `umask 077`, `set -u` (no `set -e`/pipefail); grep for `declare -A|mapfile|readarray|declare -n|${var,,}|${var^^}` matches only comment lines; source guard `[ "${BASH_SOURCE[0]}" = "$0" ]`; `bash -n` clean; no runtime external deps (optional jq/python3 self-check is PR2b) |

## Task completion

- PR1 tasks 1.1–1.9: complete (PR #55, merged).
- PR2a tasks 2.1–2.6: complete (PR #57, merged).
- **Tasks 2.7–2.13: NOT IMPLEMENTED (PR2b pending).**

### Unchecked implementation task markers (`- [ ]`) — exact lines (tasks.md:545–554, Task 2.13 checklist)

```
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
```

These are acceptance-criteria checkboxes for full-flow verification; most are satisfiable only after PR2b. **Archive is not ready while these remain.**

## Findings

### CRITICAL
1. **`apply-progress.md` does not exist** for this change (neither on `main` nor the branch; Engram unreachable). Strict TDD is active (`openspec/config.yaml`: `tdd: true`; system flag enabled) and the contract requires a `TDD Cycle Evidence` table. No RED→GREEN evidence table can be audited. Test files exist and pass, and tasks are structured RED→GREEN, but the mandatory artifact is missing → archive blocker. Remedy: persist `sdd/feat-install-prompts/apply-progress` (or `openspec/changes/feat-install-prompts/apply-progress.md`) before archive.
2. **Change incomplete: PR2b (tasks 2.7–2.13) not implemented** → REQ-IS-1/3/4/5/8 unverifiable; 10 unchecked acceptance checkboxes above.

### WARNING
1. `checkpoint_load` key regex `^..."([^"]+)"...` cannot match keys containing escaped quotes. Safe today (keys are registry-controlled, no operator input in keys — Threat T2 honored), but any PR2b work that ever puts dynamic content in keys would corrupt load. Guard this invariant in PR2b.
2. E2E `test_fresh_happy_path` asserts the `PR2b` stub message; it must be updated when PR2b lands (already TODO-commented in the test file).

### SUGGESTION
1. `run_business_section` has a duplicated dead assignment `n=${#BP_KEYS[@]}` (line appears twice).

## Verification commands (run this session, exact)

| Command | Result |
|---|---|
| `bash -n scripts/install.sh` | PASS (syntax clean) |
| `bash scripts/tests/run_tests.sh` | PASS — 2/2 suites, 22 tests (9 E2E + 13 unit), 0 failures |
| `go test -v -race ./...` | PASS — 0 FAIL packages (no Go surface touched by this change, as tasks declare) |

## Strict TDD compliance

- Test files exist and cross-reference correctly: `scripts/tests/install_validators_test.sh` (13 tests, incl. `test_bp_keys_order`, `test_json_escape_roundtrip`, `test_atomic_write`, path-resolution tests), `scripts/tests/install_e2e_test.sh` (9 tests).
- All GREEN at HEAD `259d219`.
- **`TDD Cycle Evidence` table: NOT AUDITABLE — apply-progress artifact missing (CRITICAL #1).**
- Assertion quality: no tautologies observed; assertions are behavioral (parseability via real JSON parse, stderr Spanish-error greps, checkpoint file state after R/S/Q, round-trip value equality incl. adversarial `D'Átelier "La Casa"` and control bytes). No ghost loops, no smoke-only suites, no implementation-detail CSS.

## Review workload / PR boundary

- `Chained PRs recommended: Yes`, `Chain strategy: stacked-to-main`, `Delivery strategy: ask-on-risk`, per-PR ≤400 lines: **respected** (PR1 #55 ~390 authored, PR2a #57; fix branch touches only `scripts/install.sh`, 25 ins / 25 del across commits 6492acc+259d219). No scope creep beyond the assigned slice; `size:exception` not used (not needed). Review lineage review-9b0da603 approved + acknowledged; its correction (`json_escape "$key"` at install.sh:363) confirmed present in HEAD.

## Blockers (exact)

1. Implement PR2b (tasks 2.7–2.13) or get an approved partial-slice decision; archive blocked by the 10 unchecked lines in Task 2.13.
2. Persist `apply-progress.md` with `TDD Cycle Evidence` (strict TDD active).
3. (Non-blocking, noted) Engram persistence unavailable this session — this report persisted to openspec only.
