# Design: feat-install-prompts (Fase 4)

> Change: feat-install-prompts
> Inputs: `proposal.md` (question round ANSWERED), `specs/install-setup/spec.md` (12 REQs, 41 scenarios)
> References: PRD §3.5 (Install Layout — canonical), §3.7.1/.2/.4/.5/.6; ADR-0008 (inline prompts), ADR-0010 (TUI is ops-only), ADR-0014 (Fase-5 deploy end-state), ADR-0005 (no required external tools)
> Store: openspec. Chain: stacked-to-main. Base: `main` @ `3208e6e`.

## 1. Technical Approach

One new bash script, `scripts/install.sh`, plus a vendored shunit2 test suite under `scripts/tests/`. Zero Go files, zero schema changes, no existing file is modified.

The script is a **single-file bash 3.2 program** structured as a function library with a thin `main` dispatcher, so unit tests can `source` it without executing the flow. All persistent state lives in a **flat dotted-key store** (see §6.2) that serializes 1:1 to the checkpoint JSON — this is the central simplification: one data representation serves checkpoint persistence, resume position derivation, and final JSON assembly.

Execution flow:

```
main()
 ├─ umask 077, set -u, locale guards
 ├─ resolve paths (config_dir / setup_dir / checkpoint) per §3.5
 ├─ [checkpoint exists?] ──► R/S/Q menu (before anything else, REQ-IS-7)
 ├─ [3 JSONs exist, no checkpoint?] ──► confirm-and-reconfigure (REQ-IS-9)
 ├─ run_setup()
 │   ├─ business profile section  (18 fields, fixed order)
 │   ├─ business hours section    (7 day prompts → §3.7.2 object)
 │   ├─ staff loop                (≥1 professional + 7-day schedule each, → §3.7.5)
 │   ├─ services loop             (≥1 service)
 │   ├─ summary + confirm         (design decision D8)
 │   └─ finalize()                (3 atomic writes, then checkpoint removal, REQ-IS-8)
 └─ optional jq/python3 self-check (REQ-IS-10 MAY)
```

Every valid answer: update flat store → render checkpoint → atomic rewrite (tmp + rename). Interrupting at any point therefore leaves a parseable checkpoint reflecting all validated answers (REQ-IS-6).

## 2. bash 3.2 Constraints (REQ-IS-12)

Target floor: macOS stock `/bin/bash` 3.2. The rules below are normative for this file and enforced by review:

| Constraint | Rule |
|---|---|
| No associative arrays | state lives in the flat dotted-key store (newline-separated `key=value` list in one variable) + fixed variable names |
| No `mapfile`/`readarray` | line iteration via `while IFS= read -r` or `for` over `$'\n'`-split content |
| No namerefs (`declare -n`) | dynamic read via `${!var}` indirection (3.2-safe); dynamic write via `eval "$name=\$VALUE"` where `$name` comes **only** from our own field registry, never from user input (see Threat T2) |
| No `${var,,}` / `^^` | uppercase via `tr`? No — external tool. Use per-char `case` expansion loop or `printf '%s' "$c" | tr` is banned; implement `str_toupper` with a `case`-based translation table (a-z → A-Z) |
| Octal pitfall | all numeric parsing of operator input uses `$((10#$n))` — `09` must not explode as octal |
| Arithmetic | `$(( ))` integer math only; decimal range checks decompose int/frac parts (§4) |
| Regex | `[[ $x =~ regex ]]` ERE only, no `\d`/`\w` PCRE classes (`[0-9]`, `[A-Za-z]`) |
| Locale | `LC_ALL=C` set/restored **locally** inside `json_escape`/char-iteration helpers so byte semantics hold; the rest of the script never changes locale (UTF-8 prompts stay intact) |
| Shebang | `#!/bin/bash` — deterministic on both Linux and macOS; avoids `/usr/bin/env bash` resolving to a Homebrew bash 5 where 3.2-isms would go unnoticed |
| Strictness | `set -u` (all store vars initialized from the registry before first read); **no `set -e`** (prompt loops and validators return non-zero by design); no `pipefail` (no pipes on critical paths — writes go directly to file descriptors) |
| External tools | none required. `mv`, `mkdir`, `rm`, `chmod` are the only commands invoked beyond bash builtins (`mv`/`rm`/`mkdir` are POSIX-core, present on both targets; ADR-0005 spirit holds — `jq`/`python3` only ever optional) |

Script header comment documents the 3.2 floor and the forbidden-construct list so future editors don't regress it.

## 3. Script Module Layout (function boundary for Fase 5)

`scripts/install.sh` internal sections, in order:

```
1.  Header & constraints comment
2.  Guards: umask 077, set -u
3.  Constants & field registry        # canonical field order, types, validators, defaults
4.  String helpers                    # trim (incl. CR), str_toupper, ord
5.  Validators (pure)                 # v_nonempty … v_time_pair — §4
6.  JSON helpers                      # json_escape, json_render_value, atomic_write
7.  Flat store                        # store_set/store_get/store_has/store_unset, checkpoint_render
8.  Checkpoint I/O                    # checkpoint_save (atomic), checkpoint_load (line parser)
9.  Prompt engine                     # prompt_field, prompt_day_hours, prompt_list_section
10. Section runners                   # run_business_section, run_hours_section, run_staff_section, run_services_section
11. Assembly & finalization           # render_setup_business/staff/services, finalize
12. Entry points                      # run_setup, main (arg parse: --help, --setup-only)
13. Source guard + main "$@"
```

**Composable boundary (Fase 5).** `run_setup()` is the complete setup flow with no deploy logic inside. `main()` parses args today as:

- `install.sh` (no args) → `run_setup` (Fase 4 behavior — this change *is* the setup flow)
- `install.sh --setup-only` → `run_setup` (identical today; the flag exists so ADR-0014's Fase-5 composition — detect OS, download, verify, register service, linger, health check around/before setup — can wrap or gate it without restructuring)
- `install.sh --help` → usage in Spanish, exit 0

Fase 5 flips the default to full-install and keeps `--setup-only`; nothing in this design hardcodes the Fase-4 default into library functions. The unit-test source guard (`[ "${BASH_SOURCE[0]}" = "$0" ] && main "$@"`) keeps the file dual-use (executable + sourced library).

## 4. Validator Functions (pure, unit-testable — REQ-IS-2/.3/.4/.5)

Contract: every validator is `v_<field> <string>` → exit 0 valid / exit 1 invalid, printing the Spanish error to **stderr** on failure. Pure: no reads, no writes, no globals. Tests source the script and assert exit codes + stderr text.

| Validator | Rule (format-level) |
|---|---|
| `v_nonempty` | non-blank after trim |
| `v_country` | `^[A-Za-z]{2}$` → normalized uppercase by transform |
| `v_latitude` | decimal in [-90, 90] |
| `v_longitude` | decimal in [-180, 180] |
| `v_url` | `^https?://[^[:space:]]+$` |
| `v_phone` | `^\+[0-9]{8,15}$` (E.164-style) |
| `v_email` | `^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$` |
| `v_messenger_platform` | `whatsapp` or `telegram` |
| `v_currency` | `^[A-Za-z]{3}$` → uppercase transform |
| `v_symbol` | non-blank after trim |
| `v_timezone` | `^[A-Z][A-Za-z0-9_+\-]*(/[A-Z][A-Za-z0-9_+\-]*){0,2}$` (IANA path shape; prompt shows examples) |
| `v_positive_int` | `^[0-9]+$` and value > 0 |
| `v_price` | `^[0-9]+(\.[0-9]+)?$` (≥ 0; `0` accepted per REQ-IS-5) |
| `v_payment_list` | splits on `,`; at least one non-empty trimmed item |
| `v_hhmm` | `^([01][0-9]|2[0-3]):[0-5][0-9]$` |
| `v_time_pair <start> <end>` | both `v_hhmm` and `$((10#${start%%:*}*60 + 10#${start##*:})) < $((10#${end%%:*}*60 + 10#${end##:*}))` (strictly earlier) |

**Decimal range checks in pure bash** (`v_latitude` as the pattern): match `^-?[0-9]+(\.[0-9]+)?$`; strip sign; split `int.frac`; integer-compare `int` against the bound; when `int` equals the bound, the fraction must be all zeros (`^[0.]*$` after stripping the dot → zero check). No `awk`, no `bc`.

**Normalization transforms** (applied after validation, before store): `t_upper` for `country`/`currency_code`; `t_trim` for every input. Registry entries carry an optional transform function name.

**Prompt engine** (`prompt_field <var> <prompt-text> <validator> <mode> [default]`): reads one line, trims (including trailing CR — CRLF-hardening, Threat T7); blank + default → apply default; blank + optional → store `null` sentinel; blank + required → Spanish error, re-ask; non-blank → run validator, on failure print its stderr and re-ask. On `read` failure (EOF): print Spanish cancel message, exit 1 with checkpoint preserved (REQ-IS-7).

## 5. Paths & Permissions (§3.5 — includes the macOS divergence decision)

| Artifact | Linux | macOS |
|---|---|---|
| Config root | `${XDG_CONFIG_HOME:-$HOME/.config}/mcp-appointments-crm` | `$HOME/Library/Application Support/MCP Appointments CRM` |
| Setup JSONs | `<config>/setup/setup_{business,staff,services}.json` | `<config>/setup/setup_{business,staff,services}.json` |
| Checkpoint | `<config>/setup.json.tmp` | `<config>/setup.json.tmp` |

- `XDG_*` variables are honored **on Linux only** (`uname -s` gate). On macOS the platform-native path is deterministic; power-user XDG overrides are intentionally ignored (§3.5: "en macOS y Windows, las convenciones nativas").
- The checkpoint lives at the **config root**, not inside `setup/` — spec REQ-IS-6 says "in the platform-native config directory", while REQ-IS-1 puts the three JSONs "into the platform-native setup directory". The proposal's `setup{,/setup.json.tmp}` shorthand resolves to: setup dir for JSONs, config root for the checkpoint.
- `$HOME` unset → Spanish fatal error (no fallback guessing).
- Permissions: global `umask 077` at entry ⇒ directories 0700, checkpoint and JSON files 0600 — satisfies REQ-IS-1 "owner-only" and the proposal's PII concern with one mechanism instead of per-file `chmod` calls.
- Symlink guard: if the config root exists but is a symlink, refuse with a Spanish error (Threat T5).

### Decision D1 — macOS config-path divergence (the proposal's open item)

ADR-0014 Decision 2, step 6 writes `.env` at `~/.config/mcp-appointments-crm/.env` without a platform qualifier, while PRD §3.5's table assigns macOS config to `~/Library/Application Support/MCP Appointments CRM/`. **Resolution: PRD §3.5 governs.** The PRD itself states that Linux paths are canonical-reference examples and per-OS values come from the table; the spec already encodes the §3.5 macOS path (REQ-IS-1). ADR-0014's step-6 example is therefore Linux-canonical shorthand, not a macOS norm. For this change: macOS config root = `~/Library/Application Support/MCP Appointments CRM`. The `.env` path itself is Fase-5 scope; Open Question Q1 records the follow-up to amend ADR-0014's wording (or consciously keep `.env` XDG-style on macOS) when Fase 5 lands.

## 6. Data Flow & Contracts

### 6.1 Field registry (canonical order)

A set of parallel indexed arrays (3.2-safe) defines the business-profile prompt sequence, e.g.:

```
BP_KEYS    = (name industry country address latitude longitude cover_photo_url
              public_phone messenger_platform messenger_id contact_email
              website_url general_description accepted_payment_methods
              currency_code currency_symbol timezone slot_interval_minutes)
BP_TYPES   = (s s s s n n s s s s s s s l s s s i)      # s=string n=number l=list i=int
BP_REQUIRED= (1 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0 0)
BP_VALID   = (v_nonempty "" v_country "" v_latitude v_longitude v_url
              v_phone v_messenger_platform "" v_email v_url "" v_payment_list
              v_currency v_symbol v_timezone v_positive_int)
BP_DEFAULTS= (""  ""   ""        ""  ""        ""         ""
              ""   ""                 ""    ""   ""  ""
              "ARS" "$"  "UTC" "30")
```

Same pattern for staff subfields (`name role_specialty phone email` + schedule) and service subfields (`name description duration_minutes price`). The registry is the single source of prompt order, validation, defaults, and **resume order** (§6.3).

### 6.2 Flat dotted-key store ← the checkpoint

All captured state lives in one variable as newline-separated lines `key=value` (raw string values; a literal `null` marks answered-blank-optional; numbers stored as their digit strings). Keys:

```
business.<field>                  # e.g. business.contact_email=null
hours.<day>.open / .close         # absent pair ⇒ day closed
staff.<i>.name / .role_specialty / .phone / .email / .status
staff.<i>.sched.<day>.open / .close     # day ∈ 0..6 (§3.7.5), absent ⇒ not working
services.<i>.name / .description / .duration_minutes / .price
```

**Checkpoint rendering** (after every valid answer, REQ-IS-6): each store line becomes one JSON member on its own line, keys quoted verbatim (dotted keys are legal JSON object keys), values rendered by type from the registry — strings through `json_escape`, numbers bare, `null` literal:

```json
{
  "version": 1,
  "business.name": "D'Átelier \"La Casa\"",
  "business.industry": null,
  "business.country": "AR",
  "hours.monday.open": "09:00",
  "hours.monday.close": "18:00",
  "staff.0.name": "Ana",
  "staff.0.sched.1.open": "09:00",
  "staff.0.sched.1.close": "18:00",
  "services.0.name": "Corte ✂️ clásico",
  "services.0.duration_minutes": "30",
  "services.0.price": "1500"
}
```

Why flat: (a) it is valid JSON satisfying REQ-IS-6 ("valid JSON … values and per-field progress, including completed professionals with their schedules"); (b) **one line per scalar makes it parseable by our own pure-bash reader** — no nesting, no array tracking; (c) progress *is* key presence against the registry order, so checkpoint and resume logic can never desync (no separate cursor to maintain). Trade-off: the checkpoint is not the final files' shape — acceptable, it is internal state; the final JSONs are assembled separately (§6.4).

**Checkpoint load** (Resume): read line-by-line; match `^  "([^"]+)": (.*)$`; split key/value; un-escape strings with `json_unescape` (exact inverse of `json_escape`, only ever fed our own writer's output); unknown keys or `version != 1` → degraded menu: warn in Spanish, offer only Start over / Quit (resume impossible, Threat T4).

**Atomicity**: `atomic_write <dest>` reads content from stdin into `<dest>.new.$$` (same directory ⇒ same filesystem ⇒ POSIX-atomic `mv -f`), with a global `CURRENT_TMP` + `EXIT` trap `rm -f` guard so kills don't litter temp files. Ctrl+C, EOF, `kill -9` — all leave the last fully-renamed checkpoint intact; there is never a half-written `setup.json.tmp`.

**R/S/Q menu** (before any other prompt, REQ-IS-7): `[R]eaudar / [S]tart over / [Q]uit` → **R**=continuar, **S**=empezar de nuevo, **Q**=salir (Spanish labels; single-letter selection; invalid letter re-asks). R/S/Q precedence over REQ-IS-9: if both a checkpoint and the three JSONs exist (interrupted reconfiguration), the checkpoint menu comes first and Resume continues that reconfiguration. Start over deletes the checkpoint. Quit exits 0 touching nothing.

### 6.3 Resume position derivation

No stored cursor. Position is derived from key presence in registry order:

1. First `business.<field>` key absent → resume there; all present → section done.
2. First `hours.<day>` without `.open` → resume that day.
3. Staff: `N` = highest `staff.<i>` index. Entry `<N>` complete ⇔ name present **and** all 7 `sched.<day>` pairs decided (present or deliberately absent — not-working days leave no key, so completeness = the *next* day after the last decided one; decided days are tracked monotonically Monday→Sunday). Incomplete entry → resume at its first unanswered subfield (its answered subfields are kept). Complete → offer add-another.
4. Services: same rule (complete ⇔ `duration_minutes` and `price` present).
5. Everything complete (e.g., killed mid-finalization, REQ-IS-8 scenario) → skip straight to the summary confirm and re-finalize; the three writes are idempotent re-writes of identical content.

**Resume revalidation** (hardening, Threat T4): each restored value is re-run through its validator; a value that fails (hand-edited checkpoint) is discarded and re-prompted. This does not violate REQ-IS-7's "MUST NOT re-ask already-answered fields" — the guarantee covers validated answers, and a tampered value is by definition not one.

### 6.4 Final JSON schemas (the Fase-5 contract)

Assembled from the store; every string passes `json_escape`; no `id`, `created_at`, or `updated_at` anywhere (REQ-IS-1); keys in the fixed order shown.

**`setup_business.json`** — one object, all 18 fields + hours (§3.7.1/.2):

```json
{
  "name": "D'Átelier \"La Casa\"",
  "industry": null,
  "country": "AR",
  "address": null,
  "latitude": null,
  "longitude": null,
  "cover_photo_url": null,
  "public_phone": "+5491122334455",
  "messenger_platform": "whatsapp",
  "messenger_id": null,
  "contact_email": "hola@latel.com",
  "website_url": null,
  "general_description": null,
  "accepted_payment_methods": ["efectivo","tarjeta","transferencia"],
  "currency_code": "ARS",
  "currency_symbol": "$",
  "timezone": "America/Argentina/Buenos_Aires",
  "slot_interval_minutes": 30,
  "business_hours": {
    "monday":    { "open": "09:00", "close": "18:00" },
    "tuesday":   { "open": "09:00", "close": "18:00" },
    "wednesday": { "open": "09:00", "close": "18:00" },
    "thursday":  { "open": "09:00", "close": "18:00" },
    "friday":    { "open": "09:00", "close": "18:00" },
    "saturday":  { "open": "09:00", "close": "13:00" },
    "sunday":    null
  }
}
```

**`setup_staff.json`** — array; each entry mirrors §3.7.4 columns + a `schedule` array mirroring §3.7.5 (at most one entry per weekday, `day_of_week` 0–6 with 0 = Sunday, no entry for not-working days) so the Fase-5 loader seeds rows without transformation:

```json
[
  {
    "name": "Ana",
    "role_specialty": "Estilista",
    "status": "active",
    "email": null,
    "phone": "+5491122334455",
    "specialties": [],
    "schedule": [
      { "day_of_week": 1, "start_time": "09:00", "end_time": "18:00" },
      { "day_of_week": 2, "start_time": "09:00", "end_time": "18:00" },
      { "day_of_week": 3, "start_time": "09:00", "end_time": "18:00" },
      { "day_of_week": 4, "start_time": "09:00", "end_time": "18:00" },
      { "day_of_week": 5, "start_time": "09:00", "end_time": "18:00" }
    ]
  }
]
```

`status: "active"` and `specialties: []` are emitted by the script unprompted (REQ-IS-4). `specialties` is an explicit empty array — documents the shape for Fase 5; service IDs cannot exist at prompt time.

**`setup_services.json`** — array; `is_active` emitted as `1` (SQLite boolean convention used across the repo, not JSON `true`):

```json
[
  {
    "name": "Corte ✂️ clásico",
    "description": null,
    "duration_minutes": 30,
    "price": 1500,
    "is_active": 1
  }
]
```

### 6.5 Finalization order (REQ-IS-8)

1. `atomic_write setup_business.json` → 2. `atomic_write setup_staff.json` → 3. `atomic_write setup_services.json` → 4. `rm -f checkpoint` → 5. optional self-check (`command -v jq || command -v python3`; if found, parse-validate the three files; absent ⇒ skip silently, REQ-IS-10/12) → 6. Spanish success message with absolute file paths.

An interruption anywhere in 1–3 leaves a superset of finalized JSONs plus the checkpoint ⇒ re-run offers R/S/Q ⇒ Resume → summary → re-finalize (idempotent). The checkpoint is removed strictly last.

### 6.6 JSON escaping helper (REQ-IS-10)

`json_escape <string>` → stdout. `LC_ALL=C` scoped; iterate **bytes** via `${s:i:1}`:

- `\` → `\\` ; `"` → `\"` ; newline → `\n` ; tab → `\t` ; CR → `\r`
- bytes < 0x20 (other controls) and 0x7F → `\u00XX`
- bytes ≥ 0x80 → verbatim (UTF-8 multibyte sequences pass through untouched — REQ-IS-10/11 round-trip)

`json_unescape` is its exact inverse and is only ever applied to bytes produced by `json_escape` (checkpoint load). Inputs are short (names, emails); per-byte looping cost is irrelevant.

### 6.7 CLI & exit codes

| Invocation | Behavior | Exit |
|---|---|---|
| `install.sh` / `install.sh --setup-only` | setup flow (D9: Fase-4 default) | — |
| `install.sh --help` | Spanish usage | 0 |
| completed run | files written, checkpoint removed | 0 |
| R/S/Q → Quit ; REQ-IS-9 decline | nothing modified | 0 |
| EOF mid-prompt / cancel | Spanish message, checkpoint preserved | 1 |
| SIGINT | default disposition (130), checkpoint preserved | 130 |
| unreadable/invalid environment (no `$HOME`, symlinked config root, unknown checkpoint version on Resume attempt) | Spanish fatal | 1 |

Validation failures never exit — they re-ask (REQ-IS-2).

## 7. File Changes

| Path | Status | Content |
|---|---|---|
| `scripts/install.sh` | new | §3 layout; est. 550–650 LOC |
| `scripts/tests/lib/shunit2` | new (vendored) | upstream shunit2 2.1.x single file, unmodified |
| `scripts/tests/install_validators_test.sh` | new | unit tests: every validator, transforms, `json_escape`/`json_unescape`, `v_time_pair` math, store helpers |
| `scripts/tests/install_e2e_test.sh` | new | stdin-driven scenarios (§9) |
| `scripts/tests/run_tests.sh` | new | runner: executes `*_test.sh` under `scripts/tests/` |

No existing file changes. The vendored shunit2 file is excluded from review-budget accounting (third-party, verbatim); authored lines only.

## 8. Architecture Decisions

- **D1 — macOS path divergence → §3.5 wins** (§5). Config root on macOS = `~/Library/Application Support/MCP Appointments CRM`; ADR-0014 step-6 `.env` example is Linux-canonical shorthand; amendment deferred to Fase 5 (Q1).
- **D2 — flat dotted-key checkpoint** (§6.2) instead of nested JSON mirroring the final files. One representation drives persistence, resume-position derivation, and assembly; pure-bash loadable; progress = key presence (no cursor desync class of bugs).
- **D3 — validators are pure stderr-reporting functions**, sourced and unit-tested directly; the prompt engine owns all I/O. Testability split per the proposal.
- **D4 — no `set -e`; `set -u` with registry-initialized variables.** Interactive loops intentionally return non-zero; explicit error paths everywhere else.
- **D5 — `umask 077` global** instead of per-artifact chmod: one mechanism yields 0700 dirs / 0600 files everywhere (REQ-IS-1 + PII risk).
- **D6 — resume revalidates restored values** (§6.3): tamper-hardening at near-zero cost, spec-compliant.
- **D7 — checkpoint at config root, JSONs in `setup/`** (§5), resolving the proposal's `setup{,/setup.json.tmp}` ambiguity in favor of the spec's literal wording.
- **D8 — summary + confirm before finalization**: after the services loop the operator sees a Spanish summary of every captured value and confirms `[S]í / [n]o`. Decline aborts (exit 1, checkpoint preserved ⇒ resumable). Not demanded by any scenario, but it is the only guard against a fat-fingered field forcing a full re-run, and it honors the "never silent" spirit of REQ-IS-9. Explicitly a design addition — flagged for spec-tolerance in verify.
- **D9 — `--setup-only` flag lands now, no-args = setup**: Fase 5 inverts the default without touching `run_setup()`'s boundary (§3).
- **D10 — `is_active: 1` and `status: "active"` as literals** matching SQLite conventions (§6.4), not JSON booleans.
- **D11 — shunit2 vendored** (proposal's primary; bats remains the documented fallback). Single sourced file, zero install, matches the CI-less repo state.

## 9. Testing Strategy

Framework: **shunit2 2.1.x vendored** at `scripts/tests/lib/shunit2`; runner `scripts/tests/run_tests.sh` (local `bash scripts/tests/run_tests.sh`; no CI yet — macOS bash-3.2 verification stays manual until Fase 5+, per proposal risk list). Test-only dependency: `python3` (JSON parseability assertions); it is never a runtime dependency (REQ-IS-12 unaffected).

**Unit (`install_validators_test.sh`)** — source `install.sh`, assert per validator: the spec's table cases plus boundary negatives (`95` latitude, `GMT-3`, `0` slot/`0` duration, `-100` price, `9:00 AM`, `18:00`–`09:00` inversion, `sms` platform, `no-arroba` email) and positives (`0` price, `90.0` latitude, `ar`→`AR` via transform). `json_escape`/`json_unescape` round-trips: quotes, backslashes, embedded newline/tab, `✂️` emoji, accented UTF-8, control bytes. Store helpers: set/get/has/unset, overwrite-in-place, checkpoint render/parse round-trip.

**E2E (`install_e2e_test.sh`)** — pipe scripted stdin into the executed script with `HOME` and `XDG_CONFIG_HOME` redirected into a per-test temp dir:

1. **Fresh happy path** (RF1 AC1): complete all sections → three parseable JSONs, correct shapes (§6.4 golden comparisons after `python3 -m json.tool`), checkpoint gone, dirs 0700/files 0600, no `id` keys.
2. **Invalid-then-valid retry** (RF1 AC2): bad email then good → error text in stderr is Spanish, flow does not advance (next prompt is still the same field).
3. **Cancel-by-EOF + Resume** (RF1 AC3): feed EOF mid-staff → exit ≠ 0, checkpoint parses; re-run with `R` + remaining answers → no re-ask of completed business fields or the completed professional (asserted by feeding *no* input for those fields — any re-ask would desync the script and fail).
4. **Cancel + Start over**: `S` → checkpoint deleted, first prompt presented.
5. **Quit**: `Q` → exit 0, checkpoint + pre-existing JSONs byte-identical.
6. **REQ-IS-9 confirm-and-reconfigure**: pre-seed three JSONs, no checkpoint → confirmation prompt before any field; decline → files untouched; accept → full flow overwrites.
7. **Adversarial input** (REQ-IS-10): `D'Átelier "La Casa"` business name, `Corte ✂️ clásico` service → files parse, values verbatim after round-trip.
8. **Mid-finalization resume** (REQ-IS-8): craft a complete-values checkpoint fixture + one pre-existing JSON → `R` → confirm → all three rewritten, checkpoint removed.
9. **Optional-field null/default behavior**: blank `website_url` → `"website_url": null`; blank `timezone` → `"UTC"`; `efectivo, tarjeta , transferencia` → `["efectivo","tarjeta","transferencia"]`.
10. **Zero staff / zero services refusal**: blank name on first entry → Spanish refusal, loop continues.

**TDD**: PR1 is written validator-test-first (RED → GREEN per validator batch); PR2's checkpoint/finalization behaviors get their failing E2E skeletons committed alongside implementation. Go surface untouched — `go test -v -race ./...` stays green by definition (no Go edits).

## 10. Threat Matrix

| # | Threat | Vector | Mitigation |
|---|---|---|---|
| T1 | Invalid JSON via unescaped metacharacters | quotes/backslash/newline/control bytes in any free-text answer | `json_escape` on **every** string interpolation (single choke point: renderers + checkpoint writer); adversarial unit + E2E scenarios; optional runtime self-check (§6.5) |
| T2 | `eval` code injection | dynamic variable assignment during store load | `eval "$key=\$VALUE"` form — only the *identifier* (registry-controlled) is interpolated; operator data enters via referenced variable, never into the evaluated string |
| T3 | Torn/partial checkpoint on crash | kill between write and rename | same-dir tmp + `mv -f` (atomic); no truncate-in-place anywhere; `CURRENT_TMP` EXIT-trap cleanup |
| T4 | Checkpoint tampering / foreign version | local user edits `setup.json.tmp` | `version: 1` gate → degraded menu (Start over / Quit only) for unknown versions; resume revalidation discards invalid values (D6) |
| T5 | Symlinked config root | attacker pre-points config dir elsewhere | refuse when config root exists and is a symlink (Spanish fatal) |
| T6 | PII exposure (name/phone/email at rest) | world-readable checkpoint/JSONs on shared machines | `umask 077` ⇒ 0600 files, 0700 dirs (D5) |
| T7 | CRLF-polluted stdin | piping input prepared on Windows | trim strips trailing CR before validation |
| T8 | Locale-dependent byte handling | exotic `LC_ALL` breaks escape/ord | `LC_ALL=C` scoped inside escape helpers; prompts/display untouched |
| T9 | Scope creep into Fase 5 deploy steps | ADR-0014 steps 1–10 leaking into this change | `run_setup()` boundary (§3) + explicit out-of-scope list in proposal; `--setup-only` is the only Fase-5-shaped surface shipped now |

## 11. Migration / Rollout — PR Slicing (stacked-to-main)

~700 authored LOC (script + tests) vs the 400-line review budget ⇒ two stacked PRs. Vendored shunit2 excluded from the count.

**PR 1 — skeleton + validators + unit tests** (~380 authored lines)
- `scripts/install.sh`: header/constraints, guards (`umask`, `set -u`), path resolution (§5 incl. D1 + symlink guard), string helpers, **all validators + transforms**, `json_escape`/`json_unescape`, `atomic_write`, flat-store helpers, source guard, `main` with `--help`/`--setup-only` stubs (setup flow prints "disponible en el siguiente PR" — interim; nothing existing breaks since no installer exists today).
- `scripts/tests/`: vendored shunit2, `run_tests.sh`, `install_validators_test.sh` (full unit matrix).
- Exit criteria: all unit tests green; `bash -n` clean; review ≤ 400 lines.

**PR 2 — checkpoint + prompt engine + finalization + E2E** (~380 authored lines, stacked on PR 1)
- Field registry, checkpoint render/load/save, R/S/Q, REQ-IS-9 gate, prompt engine, four section runners, summary confirm (D8), renderers + ordered finalization, optional self-check, full Spanish copy.
- `install_e2e_test.sh`: scenarios 1–10 (§9).
- Exit criteria: all tests green end-to-end; success criteria of the proposal demonstrable locally; review ≤ 400 lines.

Rollback: `git revert` of both PRs removes script + tests; operator-machine artifacts (`setup/` JSONs, `setup.json.tmp`) are inert until Fase 5 and hand-deletable. No data risk, no service impact.

## 12. Open Questions

- **Q1 (Fase 5)**: amend ADR-0014 step 6 so the `.env` example is platform-qualified per §3.5 (macOS: `~/Library/Application Support/MCP Appointments CRM/.env`), or consciously keep `.env` XDG-style on macOS for systemd-parity of docs. D1 adopts §3.5 for everything Fase 4 writes; the `.env` call belongs to Fase 5.
- **Q2 (tasks phase, default settled)**: shunit2 vendored is the proposal's primary and this design's assumption (D11); bats remains acceptable if tasks-phase sizing says otherwise — a swap touches only the runner and test-file boilerplate, not `install.sh`.
- **Q3 (Fase 5, informational)**: whether Fase 5 keeps `--setup-only` as the composable entry or also adds `--reconfigure`; the boundary in §3 supports either without rework.
