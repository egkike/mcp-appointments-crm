# Spec: setup-loader

> Reference: `openspec/changes/feat-setup-import/proposal.md` Scope 1–3, D6–D7; `openspec/changes/feat-setup-import/exploration.md` §1, §3, §6; Issue #71
> Change: feat-setup-import
> Status: NEW (no prior spec existed)

## Purpose

`internal/config` must fulfill its documented contract: locate the setup directory per OS (mirroring `install.sh resolve_paths`), read the three JSON files produced by the setup wizard (`setup_business.json`, `setup_staff.json`, `setup_services.json`), decode them into typed structs, and transform the wizard's day-name `business_hours` representation into the numeric-string representation consumed by `BusinessProfile`. This capability is strictly read-only: the wizard remains the sole writer of the setup JSONs.

## Requirements

### Requirement: Per-OS setup directory resolution

The loader MUST resolve the setup directory with the same rules as `install.sh resolve_paths`:

- Linux: `${XDG_CONFIG_HOME:-$HOME/.config}/mcp-appointments-crm/setup/`
- macOS: `$HOME/Library/Application Support/MCP Appointments CRM/setup/`

An explicit `MCP_SETUP_DIR` environment variable MUST take precedence over the per-OS default on every platform. On any other operating system, the resolver SHOULD apply the Linux (XDG) rule unless `MCP_SETUP_DIR` is set.

#### Scenario: Linux default path

- GIVEN a Linux host with neither `XDG_CONFIG_HOME` nor `MCP_SETUP_DIR` set
- WHEN the resolver is invoked
- THEN it MUST return `$HOME/.config/mcp-appointments-crm/setup/`

#### Scenario: XDG_CONFIG_HOME honored on Linux

- GIVEN a Linux host with `XDG_CONFIG_HOME=/custom/config` and no `MCP_SETUP_DIR`
- WHEN the resolver is invoked
- THEN it MUST return `/custom/config/mcp-appointments-crm/setup/`

#### Scenario: macOS default path

- GIVEN a macOS host with no `MCP_SETUP_DIR` set
- WHEN the resolver is invoked
- THEN it MUST return `$HOME/Library/Application Support/MCP Appointments CRM/setup/`

#### Scenario: MCP_SETUP_DIR override wins on any OS

- GIVEN any host where `MCP_SETUP_DIR=/tmp/test-setup` is set
- WHEN the resolver is invoked
- THEN it MUST return `/tmp/test-setup` regardless of the per-OS default

### Requirement: Typed structs pinning the three wizard JSON shapes

The loader MUST decode each file into concrete struct types; no field of any setup struct MAY be `any`/`interface{}`.

- `setup_business.json`: a single object with the 18 wizard fields (`name`, `industry`, `country`, `address`, `latitude`, `longitude`, `cover_photo_url`, `public_phone`, `messenger_platform`, `messenger_id`, `contact_email`, `website_url`, `general_description`, `accepted_payment_methods`, `currency_code`, `currency_symbol`, `timezone`, `slot_interval_minutes`) plus a nested `business_hours` object keyed by day name (`monday`..`sunday`), where each day is either `null` (closed) or `{open, close}` in `HH:MM`.
- `setup_staff.json`: a JSON array of `{name, role_specialty, status, email, phone, specialties, schedule}` where `schedule` is an array of `{day_of_week, start_time, end_time}` with `day_of_week` an integer `0–6` (`0`=Sunday).
- `setup_services.json`: a JSON array of `{name, description, duration_minutes, price, is_active}`.

Numeric fields MUST be typed as JSON numbers: `latitude`/`longitude`/`price` as floating point, `duration_minutes`/`slot_interval_minutes`/`is_active`/`day_of_week` as integers.

#### Scenario: Valid wizard output decodes without error

- GIVEN the three files exactly as written by `install.sh finalize()`
- WHEN the loader decodes them
- THEN decoding MUST succeed and every struct field MUST be populated from the file content

### Requirement: Loader reports missing and malformed files with semantic Spanish errors

The loader MUST read each of the three files from the resolved setup directory and JSON-decode it. When a file is missing or fails to decode, the loader MUST return an error whose message is a semantic Spanish string that names the offending file and distinguishes "no existe" (missing) from "formato inválido" (malformed). Raw system dumps MUST NOT be exposed.

#### Scenario: Happy path loads all three files

- GIVEN a setup directory containing the three valid JSON files
- WHEN the loader is invoked
- THEN it MUST return the three decoded values with no error

#### Scenario: Missing file is reported by name

- GIVEN a setup directory where `setup_staff.json` does not exist
- WHEN the loader is invoked
- THEN it MUST return an error naming `setup_staff.json` and indicating the file is missing

#### Scenario: Malformed file is reported by name

- GIVEN a setup directory where `setup_services.json` contains invalid JSON
- WHEN the loader is invoked
- THEN it MUST return an error naming `setup_services.json` and indicating the content is malformed

### Requirement: business_hours day-name to numeric-string mapping

The loader MUST transform the wizard's `business_hours` representation into the storage representation consumed by `BusinessProfile`: day-name keys map to numeric-string keys with `1`=Monday through `7`=Sunday (`monday→"1"`, `tuesday→"2"`, `wednesday→"3"`, `thursday→"4"`, `friday→"5"`, `saturday→"6"`, `sunday→"7"`). A day whose wizard value is `null` MUST be absent from the output object (the entity treats a missing key as closed). Open days MUST keep their `{open, close}` `HH:MM` values unchanged. The output MUST be a JSON object string parseable by `BusinessProfile.parseBusinessHours`.

#### Scenario: monday maps to "1"

- GIVEN a wizard `business_hours` where `monday` is `{"open":"09:00","close":"18:00"}`
- WHEN the mapping is applied
- THEN the output JSON MUST contain the key `"1"` with `{"open":"09:00","close":"18:00"}`

#### Scenario: sunday maps to "7"

- GIVEN a wizard `business_hours` where `sunday` is `{"open":"10:00","close":"14:00"}`
- WHEN the mapping is applied
- THEN the output JSON MUST contain the key `"7"` with `{"open":"10:00","close":"14:00"}`

#### Scenario: null day becomes an absent key

- GIVEN a wizard `business_hours` where `sunday` is `null`
- WHEN the mapping is applied
- THEN the output JSON MUST NOT contain the key `"7"` (and MUST NOT contain `"sunday"`, `""`, or a `null` value for it)

#### Scenario: Full week maps to seven numeric keys

- GIVEN a wizard `business_hours` with all seven day names present, six open and one `null`
- WHEN the mapping is applied
- THEN the output JSON MUST contain exactly the six numeric-string keys of the open days, each with its original `open`/`close` values

### Requirement: Setup files are read-only to the server

The server MUST NOT create, modify, or delete any setup JSON file at any point (D6: files remain as audit/recovery reference after a successful seed).

#### Scenario: Successful load leaves files untouched

- GIVEN a setup directory with the three wizard JSONs
- WHEN the loader reads and decodes all three files
- THEN every file's content and modification state MUST be unchanged afterwards

## Notes

- The wizard guarantees at least 1 professional and at least 1 service per run; the loader does not re-enforce those minimums (shape pinning only).
- The staff schedule `day_of_week` convention (`0–6`, `0`=Sunday) differs from `business_hours` numeric keys (`1–7`, `1`=Monday); staff schedules are passed through 1:1 with no transformation (D7).
- Setup JSONs contain no secrets and are written 0600 by the wizard for the same single-user service that reads them (exploration R7).
