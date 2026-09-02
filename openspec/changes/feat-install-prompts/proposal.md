# Proposal: feat-install-prompts (Fase 4)

> Status: proposal phase complete — question round ANSWERED (see "Decisions from Proposal Question Round" at the end)
> Store: openspec (repo-local). Chain: stacked-to-main. Estimación: S.

## Intent

**Business problem.** A new operator runs the installer on a clean VPS today and ends up with a server that cannot serve the business. The MCP server (11 tools, Fase 3 archived 2026-09-02) boots against a lazy-initialized `business_profile` singleton full of schema defaults (`ARS`, `UTC`, empty `business_hours`), with zero professionals and zero services. Until the business identity, hours, staff and catalog exist, Hermes cannot answer "when are you open?", book a slot, or send a meaningful alert. There is currently **no `install.sh` at all** (`scripts/` contains no installer) and no path to capture initial business data.

RF1 (Must) requires capturing `business_profile`, initial professionals and initial services through **interactive prompts with per-field validation**, cancel-safe via a checkpoint. ADR-0008 settled the mechanism: inline `read -p` + regex in bash — no TUI (a one-time setup TUI was rejected as over-engineering; ADR-0010 keeps any TUI scoped to the operational admin menu, Fase 2+).

This change delivers the interactive setup flow as `scripts/install.sh`, producing the 3 setup JSON files that Fase 5's deploy flow (ADR-0014) will consume. Fase 4 = capture and persist config; it does not install the binary, register services, or touch Go code.

## Scope

### In Scope

- **New `scripts/install.sh`** (bash only, zero Go changes) implementing the RF1 interactive setup flow per ADR-0008.
- **Business profile prompts** (fields → §3.7.1 columns; §3.7 mapping reference):

| Prompt | Column (§3.7.1) | Validation | Default |
|---|---|---|---|
| Business name | `name` (NOT NULL) | non-empty, length-bounded | — (required) |
| Industry | `industry` | free text (e.g. "veterinaria") | — |
| Country | `country` | `^[A-Z]{2}$` (ISO 3166-1 alpha-2 **format**) | — |
| Address | `address` | free text | — |
| Latitude / Longitude | `latitude`, `longitude` | decimal degrees; `[-90,90]` / `[-180,180]`; optional (blank = null) | — |
| Cover photo | `cover_photo_url` | URL format | — |
| Public phone | `public_phone` | phone format (E.164-style) | — |
| Contact email | `contact_email` | email regex — RF1 AC2 test target | — |
| Currency | `currency_code` (+ `currency_symbol`) | `^[A-Z]{3}$` (ISO 4217 **format**) | `ARS` / `$` |
| Timezone | `timezone` | IANA path format (e.g. `America/Argentina/Buenos_Aires`) | `UTC` |
| Slot interval | `slot_interval_minutes` | positive integer | `30` |
| Business hours | `business_hours` | per-day `HH:MM` 24h or closed → assembled §3.7.2 JSON | `{}` |

  > **Added by the answered question round (normative)**: the optional §3.7.1 fields `messenger_platform` (`whatsapp`|`telegram`|blank), `messenger_id`, `website_url`, `general_description` and `accepted_payment_methods` (comma-separated → JSON array) are ALSO prompted; blank → `null` (or the field default where defined).

- **Initial professionals loop** (→ §3.7.4): `name` (required), `role_specialty`, `phone`, `email`. `status` defaults `'active'`; `specialties` left empty (service IDs don't exist at prompt time). Per the answered question round (normative): **≥1 professional required** before finishing, and each professional's **weekly schedule is captured** (7 weekday prompts: not-working or `HH:MM`–`HH:MM` pair; → §3.7.5 shape, `day_of_week` 0..6).
- **Initial services loop** (→ §3.7.6): `name` (required), `description`, `duration_minutes` (positive int), `price` (decimal ≥ 0, in the business currency). `is_active` defaults 1. Per the answered question round (normative): **≥1 service required** before finishing.
- **Per-field validation before advancing**: invalid input → Spanish error + re-ask loop, never advances (RF1 AC2).
- **Checkpoint** `setup.json.tmp` in the platform-native config dir (§3.5; XDG vars respected on Linux):
  - Rewritten **atomically** (tmp + rename) **after every valid answer** — any interruption (Ctrl+C, EOF, kill) leaves a parseable, current checkpoint.
  - On re-run with checkpoint detected: offer **[R]esume / [S]tart over / [Q]uit** (RF1 AC3); Resume skips already-answered fields, including finished list entries (staff/services).
  - Checkpoint is valid JSON (the name says so) holding values + per-field progress.
- **Atomic finalization** (RF1 AC1): write `setup_business.json`, `setup_staff.json`, `setup_services.json` into `<config-dir>/setup/` via tmp + rename, **then** delete `setup.json.tmp` (order matters: JSONs first, checkpoint last, so an interruption mid-finalization still resumes).
- **Tests for `install.sh`** (PRD Fase 4: "bats, shunit2, o equivalente"): unit tests for every validator function + stdin-driven E2E scenarios (fresh install, cancel + resume, cancel + start-over). Proposed framework: **shunit2 vendored** (single sourced file, zero install — matches ADR-0005 spirit and the repo's CI-less state); bats acceptable alternative.

### Out of Scope

- **TUI / config-wizard binary** — rejected by ADR-0008; ADR-0010's admin TUI is Fase 2+ and unrelated to setup.
- **Binary download, SHA256 verification, service registration, `loginctl enable-linger`, health check, `.env` writing, "Recommended additional tools" block, crontab hint** — all Fase 5 per ADR-0014's 10-step flow. The Fase-4 script is the setup block that Fase 5 composes with those steps.
- **`scripts/backup.sh` and any scheduling of it** — manual/optional forever per PRD; the script itself is a Fase 5 deliverable (PRD §3.1 product scope, roadmap assigns it to Fase 5).
- **Any Go code, DB schema, MCP tool, or repository change** — consumption/seeding of the JSONs into the DB is Fase 5+; this change only *produces* the files.
- **`update_business_profile` MCP tool** (RF2 partial, separate).
- **Optional `business_profile` fields and professional schedules** — scope was DECIDED IN via the question round: `messenger_platform`, `messenger_id`, `website_url`, `general_description`, `accepted_payment_methods` are prompted; each professional's weekly schedule is captured. What remains out of scope: consuming/seeding the JSONs into the DB is Fase 5+, and this change only *produces* the files.
- **`install.ps1` / Windows prompts** — Fase 5 (ADR-0003/0014 Windows paths).
- **Service unit templates** (`setup/service/`) — Fase 5.

## Capabilities

### New Capabilities

- **`install-setup`** (RF1): fresh run → 3 valid JSONs + checkpoint removal; invalid field → retry loop without advancing; cancel + re-run → R/S/Q with skip-on-resume; per-field validator set (email, `HH:MM`, coordinates, IANA/ISO formats, positive integers); checkpoint atomicity; ordered atomic finalization; adversarial-input JSON validity. Per the answered question round: optional `business_profile` fields ARE prompted (`messenger_platform`, `messenger_id`, `website_url`, `general_description`, `accepted_payment_methods`); ≥1 professional (with weekly schedule) and ≥1 service are REQUIRED before finishing; re-run with the 3 JSONs present and no checkpoint = confirm-and-reconfigure, never silent overwrite; prompt copy in Spanish; Linux + macOS bash-3.2 (Windows → Fase 5).

### Modified Capabilities

None — zero Go surface is touched.

## Approach

- **Pure bash, bash-3.2-compatible** (macOS stock `/bin/bash` is 3.2 — no associative arrays, no `mapfile`). `read -p` prompt loop + `[[ =~ ]]` validators.
- **Testability split**: validators are small pure functions in the script; tests source the script and unit-test them directly. The interactive flow is tested E2E by piping scripted stdin (`printf '…\n…' | install.sh`). EOF mid-flow behaves as cancel (non-zero exit, checkpoint preserved) — this is what makes Ctrl+C testable.
- **Checkpoint as progress + values**: valid JSON, written tmp + rename after each valid answer. Resume reconstructs the prompt position; list sections (staff/services) track completed entries.
- **JSON escaping**: a bash escape helper (backslash, quote, newline, control chars) — adversarial inputs (business names with quotes/accents/unicode) MUST still produce parseable JSON; mandatory test scenario. Optional self-check with `jq`/`python3` only when present (never required — ADR-0005).
- **No IDs in the JSONs**: the future Go loader assigns UUIDs via `internal/idgen` at seed time; avoids external `uuidgen` and keeps bash simple.
- **Paths per §3.5** (canonical for this change): `~/.config/mcp-appointments-crm/setup{,/setup.json.tmp}` on Linux (XDG respected); platform-native config dir on macOS. ADR-0014's macOS `.env` path example diverges slightly from the §3.5 table — reconcile once in design; PRD §3.5 governs this change.
- **Composable for Fase 5**: the setup flow is a self-contained block (function boundary / `--setup-only` flag candidate) so ADR-0014's deploy steps can run before/after without rework (ADR-0008's sample flow: prompts → JSONs written → deploy steps).
- **Prompt copy in Spanish** (PRD register, ARS/LatAm target), UTF-8-safe input handling.
- **PR slicing** (chain strategy: stacked-to-main): ~700+ LOC total (script + tests) vs the 400-line budget → **2 stacked PRs**: PR1 = script skeleton + validators + unit tests; PR2 = checkpoint/resume + atomic finalization + E2E scenarios. Review Workload Forecast for tasks phase: chained PRs, budget risk Medium.

## Affected Areas

- `scripts/install.sh` — new (the only product file).
- `scripts/tests/` — new: vendored shunit2 + install tests (unit + E2E).
- `openspec/changes/feat-install-prompts/` — SDD artifacts (this proposal).
- No Go files, no schema, no MCP surface, no `cmd/`, no `internal/`.

## Risks

- **Bash JSON escaping produces invalid JSON** on quotes/unicode/newlines (Medium): escape helper + adversarial tests are mandatory scenarios; optional parser self-check when `jq`/`python3` available.
- **macOS bash 3.2 incompatibility** (Medium): restrict constructs; document the 3.2 constraint in the script header; scenario tests run on Linux (CI-less repo — macOS check is manual until Fase 5+ CI).
- **Scope creep toward Fase 5 deploy steps** (Medium): explicit out-of-scope list; ADR-0014 steps 1–10 stay out.
- **Re-run with complete JSONs and no checkpoint is undefined in the PRD** (Low): DECIDED via question round = confirm-and-reconfigure, never silent overwrite (spec REQ-IS-9).
- **Format-only validation** for IANA tz / ISO country / currency codes — not full database validity (Low): documented at prompt with examples; regex + uppercase normalization.
- **Checkpoints on shared machines** contain PII (name, phone, email) (Low): config dir created with restrictive permissions (`chmod 700`-style), consistent with the repo's `0750` convention for data paths.

## Rollback Plan

Zero Go/schema changes. `git revert` of the stacked PRs removes script + tests cleanly. Artifacts on the operator's machine (setup JSONs, `setup.json.tmp`) are inert data files nothing consumes until Fase 5 — deletable by hand. No data risk, no service impact, rollback < 5 min (PRD §3.6 satisfied trivially).

## Dependencies

- `docs/PRD.md` §7 Fase 4 (entregables + DoD), §5.1 RF1 (AC1–AC3), §3.1 (product scope), §3.5 (Install Layout, canonical), §3.6 (rollback), §3.7.1/.2/.4/.5/.6 (field mapping; §3.7.5 schedules added by question-round decision).
- `docs/architecture/0008-install-prompts.md` — inline prompts, no TUI. **Note**: the `context:` block in `openspec/config.yaml` still says "TUI: Charm Bubble Tea ecosystem … for native setup" — stale pre-ADR-0008 text; ADR-0008/ADR-0010 govern this change, not that line.
- `docs/architecture/0010-admin-tui.md` — confirms setup stays bash; TUI is operational tooling only.
- `docs/architecture/0014-release-and-deploy-workflow.md` — defines the Fase-5 `install.sh` end-state this setup block must remain composable with.
- Base: `main` @ `3208e6e`, no active changes (Fase 3 archived 2026-09-02).

## Success Criteria

- [ ] Fresh run completing all prompts produces valid `setup_business.json`, `setup_staff.json`, `setup_services.json` in the platform-native `setup/` dir, and `setup.json.tmp` is deleted (RF1 AC1).
- [ ] Invalid `contact_email` (and every other validated field) shows a Spanish error and re-asks without advancing (RF1 AC2).
- [ ] Cancel (Ctrl+C/EOF) mid-flow leaves a valid checkpoint; re-run offers R/S/Q; Resume skips answered fields and completed list entries (RF1 AC3).
- [ ] Every valid answer rewrites the checkpoint atomically; killing the script at any point leaves a parseable checkpoint.
- [ ] Finalization is atomic and ordered: 3 JSONs via tmp+rename, then checkpoint removal; interruption between writes resumes cleanly.
- [ ] Zero professionals or zero services is refused: at least 1 professional (with its weekly schedule captured via 7 weekday prompts) and 1 service are required before finishing (question-round decision).
- [ ] Optional `business_profile` fields (`messenger_platform`, `messenger_id`, `website_url`, `general_description`, `accepted_payment_methods`) are prompted; blank → JSON `null` (or the field default where defined) (question-round decision).
- [ ] Re-run with the 3 JSONs present and no checkpoint asks for explicit confirmation before reconfiguring; never silent overwrite (question-round decision).
- [ ] Unit tests green for every validator; E2E scenarios pass: fresh install, cancel + resume, cancel + start-over (Fase 4 DoD).
- [ ] Adversarial inputs (quotes, accents, unicode) still yield parseable JSON.
- [ ] `go test -v -race ./...` remains green and untouched (no Go surface).
- [ ] Delivered as ≤2 stacked PRs, each within the 400-line review budget.

## Proposal Question Round (ANSWERED — decisions locked in)

Each question was answered by the user during the interactive round. These decisions override the proposal's earlier proposed defaults and are normative for the spec:

1. **Optional `business_profile` columns**: **PROMPT THEM NOW** — `messenger_platform`, `messenger_id`, `website_url`, `general_description`, `accepted_payment_methods` are included in the install prompts. (Overrides the proposed default "skip".)
2. **Staff/service minimums and schedules**: **REQUIRE ≥1 + SCHEDULES** — at least 1 professional and 1 service are required before finishing, and each professional's weekly schedule is captured during setup (7 weekday prompts per professional: day_of_week + start/end). (Overrides the proposed default "allow empty; no schedules".)
3. **Re-run with the 3 JSONs present and no checkpoint**: **CONFIRM AND RECONFIGURE** — the installer asks for explicit confirmation and re-prompts from scratch; never silent overwrite. (Confirms the proposed default (a).)
4. **Prompt language**: **SPANISH** (UTF-8), per PRD register. (Confirms the proposed default.)
5. **Platform**: **Linux + macOS, bash-3.2 compatible**; Windows prompts deferred to `install.ps1` in Fase 5. (Confirms the proposed default.)

### Assumptions if defaults stand

> Superseded by the answered round above. Retained for the record:

- Spanish prompts; UTF-8-safe; format-level (not database-level) validation for ISO/IANA codes.
- No IDs in the JSONs (Go loader assigns UUIDs later); no specialties linking at setup time.
- shunit2 vendored as the bash test framework (bats acceptable if preferred); TDD applies to validators and scenario flows.
- Checkpoint is valid JSON written atomically after each valid answer; EOF = cancel; strict TDD for Go remains untouched.
