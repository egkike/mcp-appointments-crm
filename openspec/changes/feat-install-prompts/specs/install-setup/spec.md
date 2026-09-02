# install-setup Specification

> Reference: `docs/PRD.md` §5.1 RF1 (AC1–AC3), §7 Fase 4, §3.5 (Install Layout), §3.7.1, §3.7.2, §3.7.4, §3.7.5, §3.7.6; `docs/architecture/0008-install-prompts.md`; `docs/architecture/0014-release-and-deploy-workflow.md`
> Change: feat-install-prompts
> Status: NEW (no prior spec existed)
> Decisions: proposal question round ANSWERED (see `../proposal.md`, "Decisions from Proposal Question Round"): optional `business_profile` fields prompted; ≥1 professional and ≥1 service required, with weekly schedule per professional; re-run with existing JSONs = confirm-and-reconfigure; Spanish prompt copy; Linux + macOS (bash-3.2), Windows deferred to Fase 5.

## Purpose

El instalador interactivo (`scripts/install.sh`) debe capturar la configuración inicial del negocio — perfil del negocio, profesionales con su horario semanal y catálogo de servicios — a través de prompts inline en bash con validación por campo antes de avanzar, protegido por un checkpoint a prueba de interrupciones, y persistirla como tres archivos JSON válidos en el directorio de configuración platform-native según §3.5 (`~/.config/mcp-appointments-crm/setup/` en Linux). Esta fase produce los archivos; su consumo (seeding a la DB, descarga del binario, registro de servicio) es Fase 5.

## ADDED Requirements

### REQ-IS-1: Fresh run finalizes to three valid JSON files and removes the checkpoint

On a fresh completed run, the installer MUST write `setup_business.json`, `setup_staff.json` and `setup_services.json` into the platform-native setup directory (§3.5: `~/.config/mcp-appointments-crm/setup/` on Linux with `XDG_CONFIG_HOME` respected, `~/Library/Application Support/MCP Appointments CRM/setup/` on macOS) and MUST delete the checkpoint `setup.json.tmp` only after all three JSON files are in place. The setup and config directories MUST be created with owner-only permissions when they do not exist. The JSON files MUST NOT contain entity IDs; the future Go loader assigns UUIDs at seed time.

#### Scenario: Fresh install happy path (RF1 AC1)

- GIVEN a machine with no prior setup artifacts
- WHEN the operator completes every prompt of `install.sh`
- THEN the three files `setup_business.json`, `setup_staff.json` and `setup_services.json` exist in the platform-native setup directory, each parseable as JSON
- AND `setup.json.tmp` does not exist

#### Scenario: Setup directory permissions

- GIVEN the setup directory does not exist
- WHEN the installer creates it
- THEN the directory MUST NOT be readable or writable by group or other

#### Scenario: No IDs in the produced files

- GIVEN a completed run
- WHEN the three JSON files are inspected
- THEN they MUST NOT contain any `id` field for the business profile, professionals or services

### REQ-IS-2: Business profile prompts with per-field validation before advancing

The installer MUST prompt for every `business_profile` field listed below and MUST validate each answer before advancing: an invalid answer MUST produce a Spanish error message and re-ask the same field in a retry loop; the flow MUST NOT advance past an invalid field (RF1 AC2). Optional fields left blank MUST be persisted as JSON `null`, never as an empty string. Validation for `country`, `currency_code` and `timezone` is format-level (regex shape, not database membership); prompts SHOULD show accepted examples.

| Field | Required | Validation (format-level) | Default on blank |
|---|---|---|---|
| `name` | yes | non-empty after trimming | — (re-ask) |
| `industry` | no | free text | `null` |
| `country` | no | two alphabetic characters; stored uppercase (ISO 3166-1 alpha-2 format) | `null` |
| `address` | no | free text | `null` |
| `latitude` | no | decimal degrees in `-90`..`90` | `null` |
| `longitude` | no | decimal degrees in `-180`..`180` | `null` |
| `cover_photo_url` | no | http(s) URL format | `null` |
| `public_phone` | no | E.164-style: `+` followed by 8–15 digits | `null` |
| `messenger_platform` | no | `whatsapp`, `telegram`, or blank | `null` |
| `messenger_id` | no | free text (bot number or handle) | `null` |
| `contact_email` | no | email format (`local@domain`) | `null` |
| `website_url` | no | http(s) URL format | `null` |
| `general_description` | no | free text | `null` |
| `accepted_payment_methods` | no | comma-separated list with at least one non-empty item if any input is given; persisted as a JSON array of trimmed strings | `null` |
| `currency_code` | no | three alphabetic characters; stored uppercase (ISO 4217 format) | `ARS` |
| `currency_symbol` | no | non-blank text | `$` |
| `timezone` | no | IANA path format `Area[/Region[/City]]`, e.g. `America/Argentina/Buenos_Aires` | `UTC` |
| `slot_interval_minutes` | no | positive integer | `30` |

#### Scenario: Invalid email re-asks without advancing (RF1 AC2)

- GIVEN the installer is prompting `contact_email`
- WHEN the operator enters `no-arroba` and presses Enter
- THEN the installer MUST show a Spanish validation error and re-ask `contact_email`
- AND MUST NOT proceed to the next field until a valid email or blank is entered

#### Scenario: Blank optional field persists as null

- GIVEN the installer is prompting `website_url`
- WHEN the operator presses Enter without input
- THEN the persisted `setup_business.json` MUST contain `"website_url": null`

#### Scenario: Default applied on blank defaulted field

- GIVEN the installer is prompting `timezone` with its default shown
- WHEN the operator presses Enter without input
- THEN the persisted value MUST be `UTC`

#### Scenario: Latitude out of range rejected

- GIVEN the installer is prompting `latitude`
- WHEN the operator enters `95`
- THEN the installer MUST show a Spanish validation error and re-ask

#### Scenario: Country normalized to uppercase

- GIVEN the installer is prompting `country`
- WHEN the operator enters `ar`
- THEN the persisted value MUST be `AR`

#### Scenario: Non-IANA timezone rejected

- GIVEN the installer is prompting `timezone`
- WHEN the operator enters `GMT-3`
- THEN the installer MUST show a Spanish validation error and re-ask

#### Scenario: Non-positive slot interval rejected

- GIVEN the installer is prompting `slot_interval_minutes`
- WHEN the operator enters `0`
- THEN the installer MUST show a Spanish validation error and re-ask

#### Scenario: Payment methods list becomes a JSON array

- GIVEN the installer is prompting `accepted_payment_methods`
- WHEN the operator enters `efectivo, tarjeta , transferencia`
- THEN the persisted value MUST be the JSON array `["efectivo","tarjeta","transferencia"]`

#### Scenario: Messenger platform restricted

- GIVEN the installer is prompting `messenger_platform`
- WHEN the operator enters `sms`
- THEN the installer MUST show a Spanish validation error and re-ask until `whatsapp`, `telegram` or blank is entered

### REQ-IS-3: Business hours captured per day and assembled per §3.7.2

The installer MUST prompt business hours for each of the seven weekdays and MUST accept, per day, either a closed marker or an opening pair `HH:MM`–`HH:MM` in 24-hour format with open strictly earlier than close. The persisted `business_hours` value MUST be a JSON object with all seven day keys (`monday` … `sunday`), where closed days are `null` and open days are `{"open": "HH:MM", "close": "HH:MM"}`.

#### Scenario: Assembled weekly schedule

- GIVEN the operator answers Saturday `09:00`–`13:00`, Sunday closed, and the remaining days `09:00`–`18:00`
- WHEN the run finalizes
- THEN `setup_business.json` MUST contain a `business_hours` object with all seven day keys, with `"saturday": {"open": "09:00", "close": "13:00"}` and `"sunday": null`

#### Scenario: Malformed time rejected

- GIVEN the installer is prompting the opening time for a day
- WHEN the operator enters `9:00 AM`
- THEN the installer MUST show a Spanish validation error and re-ask

#### Scenario: Open not earlier than close rejected

- GIVEN the installer is prompting the opening and closing times for a day
- WHEN the operator enters open `18:00` and close `09:00`
- THEN the installer MUST show a Spanish validation error and re-ask

### REQ-IS-4: At least one professional with a weekly schedule is required

The installer MUST require at least one professional before the staff section can finish: an attempt to finish with zero professionals MUST be refused with a Spanish message and the loop MUST continue (question-round decision: minimums required). For each professional the installer MUST capture `name` (non-empty after trimming) and optionally `role_specialty`, `phone` (E.164-style) and `email` (email format); `status` MUST default to `active` without prompting. For each professional the installer MUST capture a weekly schedule by prompting all seven weekdays in a fixed order, where each weekday accepts either not-working or a `HH:MM`–`HH:MM` pair with start strictly earlier than end. The persisted `setup_staff.json` MUST be an array of professional objects, each carrying a `schedule` array aligned with the canonical `schedules` capability (§3.7.5): at most one entry per weekday, `day_of_week` as an integer `0..6` (0 = Sunday), `start_time`/`end_time` in `HH:MM`; weekdays marked not-working MUST have no schedule entry. `specialties` is not prompted (service IDs do not exist at setup time).

#### Scenario: Zero professionals refused

- GIVEN the installer is in the staff section with no professional captured
- WHEN the operator attempts to finish without entering any professional
- THEN the installer MUST refuse with a Spanish message and keep prompting until at least one professional is captured

#### Scenario: Schedule entries recorded per working weekday

- GIVEN a professional whose Monday–Friday answers are `09:00`–`18:00` and whose Saturday and Sunday answers are not-working
- WHEN the run finalizes
- THEN that professional's object in `setup_staff.json` MUST contain exactly five `schedule` entries with `day_of_week` 1..5, `start_time` `09:00` and `end_time` `18:00`, and no entries for `day_of_week` 0 or 6

#### Scenario: Invalid schedule range rejected

- GIVEN the installer is prompting a professional's schedule for a weekday
- WHEN the operator enters start `18:00` and end `09:00`
- THEN the installer MUST show a Spanish validation error and re-ask that weekday

#### Scenario: Status defaults to active

- GIVEN a professional captured through the prompts
- WHEN the run finalizes
- THEN the professional's object MUST carry status `active` without the operator having been asked for it

### REQ-IS-5: At least one service is required

The installer MUST require at least one service before the catalog section can finish, refusing an empty catalog with a Spanish message and continuing the loop. For each service the installer MUST capture `name` (non-empty after trimming), `description` (optional free text), `duration_minutes` (positive integer, required) and `price` (decimal ≥ 0 in the business currency, required); `is_active` MUST default to `1` without prompting. The persisted `setup_services.json` MUST be an array of service objects.

#### Scenario: Zero services refused

- GIVEN the installer is in the services section with no service captured
- WHEN the operator attempts to finish without entering any service
- THEN the installer MUST refuse with a Spanish message and keep prompting until at least one service is captured

#### Scenario: Zero price allowed

- GIVEN the installer is prompting a service's `price`
- WHEN the operator enters `0`
- THEN the value MUST be accepted

#### Scenario: Negative price rejected

- GIVEN the installer is prompting a service's `price`
- WHEN the operator enters `-100`
- THEN the installer MUST show a Spanish validation error and re-ask

#### Scenario: Non-positive duration rejected

- GIVEN the installer is prompting a service's `duration_minutes`
- WHEN the operator enters `0`
- THEN the installer MUST show a Spanish validation error and re-ask

### REQ-IS-6: Checkpoint written atomically after every valid answer

The installer MUST maintain a checkpoint file `setup.json.tmp` in the platform-native config directory (§3.5), containing valid JSON with the captured values and per-field progress, including completed professionals (with their schedules) and completed services. The checkpoint MUST be rewritten atomically (write to a temporary file in the same directory, then rename) after every valid answer. At any interruption point (Ctrl+C, EOF, kill), the checkpoint left on disk MUST be parseable JSON reflecting every answer validated so far.

#### Scenario: Kill at any point leaves a parseable checkpoint

- GIVEN a run interrupted at an arbitrary prompt after several valid answers
- WHEN the checkpoint file is inspected
- THEN it MUST parse as JSON and contain every already-validated answer

#### Scenario: Checkpoint tracks completed list entries

- GIVEN one professional with schedule fully captured and a second professional partially captured
- WHEN the run is interrupted
- THEN the checkpoint MUST contain the first professional as a completed entry and the second professional's captured answers

### REQ-IS-7: Cancel and re-run offer Resume / Start over / Quit (RF1 AC3)

A cancelled run (Ctrl+C or EOF mid-flow) MUST exit non-zero with the checkpoint preserved. On re-run with a checkpoint detected, the installer MUST offer exactly three choices — Resume, Start over, Quit — with one-letter selection, before any other prompt. Resume MUST continue from the checkpoint without re-asking already-answered fields, including finished professionals and services. Start over MUST discard the checkpoint and begin from the first prompt. Quit MUST exit without modifying anything.

#### Scenario: Cancel preserves checkpoint and re-run offers the menu (RF1 AC3)

- GIVEN a run interrupted with Ctrl+C after several valid answers
- WHEN the operator re-executes `install.sh`
- THEN the installer MUST detect `setup.json.tmp` and offer the Resume / Start over / Quit choices before any other prompt

#### Scenario: Resume skips answered fields (RF1 AC3)

- GIVEN a checkpoint where all business profile fields are answered
- WHEN the operator selects Resume
- THEN the installer MUST NOT re-ask business profile fields and MUST continue at the next unanswered prompt

#### Scenario: Resume skips completed list entries

- GIVEN a checkpoint with two completed professionals and the services section not yet started
- WHEN the operator selects Resume
- THEN the installer MUST NOT re-ask the two professionals and MUST continue in the services section

#### Scenario: Start over discards progress

- GIVEN a checkpoint with partial answers
- WHEN the operator selects Start over
- THEN the checkpoint MUST be discarded and the first prompt of the flow presented

#### Scenario: Quit changes nothing

- GIVEN a checkpoint present and JSON files from an earlier complete run
- WHEN the operator selects Quit
- THEN the installer MUST exit without modifying the checkpoint or the JSON files

#### Scenario: EOF behaves as cancel

- GIVEN a run receiving EOF on stdin mid-prompt
- WHEN the installer exits
- THEN it MUST exit non-zero with the checkpoint preserved

### REQ-IS-8: Atomic ordered finalization — JSONs first, checkpoint last

Finalization MUST write each of the three JSON files atomically (temporary file plus rename) into the setup directory and MUST delete the checkpoint only after all three renames succeeded. An interruption at any point during finalization MUST leave either the previous state or a superset of finalized JSONs plus a valid checkpoint, such that a re-run resumes instead of corrupting state.

#### Scenario: Checkpoint removed only after all three files exist

- GIVEN finalization in progress
- WHEN the three JSON renames have not all completed
- THEN `setup.json.tmp` MUST still exist

#### Scenario: Interruption mid-finalization resumes

- GIVEN a finalization interrupted after `setup_business.json` was renamed but before the other two
- WHEN the installer is re-executed
- THEN the installer MUST offer Resume / Start over / Quit from the checkpoint and complete the remaining files

### REQ-IS-9: Re-run with the three JSONs present and no checkpoint requires explicit confirmation

When all three JSON files already exist and no checkpoint is present, the installer MUST ask the operator to confirm reconfiguration explicitly before prompting any field, and MUST never silently overwrite existing files. On decline, the installer MUST exit without modifying the existing JSON files and without creating a checkpoint.

#### Scenario: Confirm and reconfigure

- GIVEN the three JSON files exist from an earlier complete run and no checkpoint exists
- WHEN the operator re-executes `install.sh` and confirms reconfiguration
- THEN the installer MUST run the setup flow again and, on completion, overwrite the JSON files

#### Scenario: Decline leaves files untouched

- GIVEN the same state
- WHEN the operator declines reconfiguration
- THEN the installer MUST exit without modifying any existing file

> Design D8 summary+confirm before finalization is spec-tolerant (honors the "never silent" spirit of REQ-IS-9, not a gap).

### REQ-IS-10: Adversarial input still yields parseable JSON

Every captured value MUST be JSON-escaped (at minimum backslash, double quote, newline and control characters) so that any operator input — including quotes, backslashes, accents, emoji and other multi-byte UTF-8 — produces parseable JSON in all three files and in the checkpoint. If `jq` or `python3` is present, the installer MAY self-check the finalized files; this check MUST be skipped without error when neither tool exists.

#### Scenario: Business name with quotes and accents

- GIVEN the operator enters `D'Átelier "La Casa"` as the business name
- WHEN the run finalizes
- THEN `setup_business.json` MUST parse as JSON and contain the name verbatim

#### Scenario: Unicode survives the round-trip

- GIVEN the operator enters `Corte ✂️ clásico` as a service name
- WHEN the run finalizes
- THEN `setup_services.json` MUST parse as JSON and contain the name verbatim

### REQ-IS-11: Spanish prompt copy, UTF-8 safe

All prompts, defaults, validation errors and menu options MUST be in Spanish (UTF-8). Input MUST be handled as UTF-8: accented and multi-byte characters in any free-text answer MUST be captured verbatim.

#### Scenario: Spanish validation error

- GIVEN any validated field
- WHEN the operator enters an invalid value
- THEN the error message MUST be in Spanish and the same field MUST be re-asked

### REQ-IS-12: Portability — bash 3.2 on Linux and macOS, no required external tools

The installer MUST run on Linux and macOS with the stock bash 3.2 interpreter (no bash-4+ constructs such as associative arrays or `mapfile`) and MUST NOT require any external tool beyond bash itself; optional self-checks are limited to `jq`/`python3` when present. Windows is out of scope for this change (deferred to `install.ps1`, Fase 5).

#### Scenario: macOS stock bash compatibility

- GIVEN a macOS machine whose `/bin/bash` is version 3.2
- WHEN `install.sh` executes the complete setup flow
- THEN it MUST complete without relying on any bash-4+ feature

#### Scenario: No external tool dependency

- GIVEN an environment with neither `jq` nor `python3`
- WHEN a complete run finalizes
- THEN the three JSON files MUST be produced without any error about missing tools

## Notes

- Consumption of the three JSON files (DB seeding, binary download, service registration) is Fase 5 per ADR-0014; this spec only governs their production.
- The `schedule` shape in `setup_staff.json` intentionally mirrors the canonical `schedules` capability (§3.7.5) so the Fase-5 loader can seed rows without transformation rules.
- See `professionals`, `services` and `business-profile` canonical specs for the downstream DB semantics of the captured values; this spec governs the installer's prompt-time contract only.
