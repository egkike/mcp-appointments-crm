## ADDED Requirements

### Requirement: delete_schedule preserves the caller-presence error code
The `delete_schedule` use case MUST NOT mask the distinction between a context without a caller (`ErrCodeUnauthenticated`) and a caller with the wrong role (`ErrCodeForbidden`). When `auth.RequireRole` returns a `*domain.SemanticError`, the use case MUST return it unchanged; only a non-Semantic underlying error falls back to the generic Forbidden wrapper (fail-closed).

- **GIVEN** an execute context with no caller
- **WHEN** the use case runs
- **THEN** the returned `SemanticError` carries `ErrCodeUnauthenticated` and the auth message unchanged (NOT the generic Forbidden message).

- **GIVEN** an owner-role mismatch on a caller present in the context
- **WHEN** the use case runs
- **THEN** the returned error keeps `ErrCodeForbidden` with its role message.

- **GIVEN** an owner caller on an existing schedule slot
- **WHEN** the delete succeeds
- **THEN** console behavior and audit log line are unchanged (differential: same stdout shape, same message).

### Requirement: availability validates the business-local wall clock
`CheckAvailability` MUST hand `ValidateBookingTimeSlot` the start converted to the business timezone (`localStart`), never the parsed instant that retains the RFC3339 input offset.

- **GIVEN** an input RFC3339 datetime whose offset differs from the business timezone such that the business-local wall day differs from the offset-local wall day
- **WHEN** availability runs
- **THEN** the validator evaluates weekday/hours of the business-local day (asserted via the exact closed-day semantic message with the business-local day name).

### Requirement: booking starts are exact to the minute
`ValidateBookingTimeSlot` MUST reject a start whose seconds or sub-second components are non-zero with an invalid-input semantic error, instead of silently truncating seconds in wall-clock comparisons.

- **GIVEN** a start of `15:00:30` and a close bound of `15:00`
- **WHEN** validation runs
- **THEN** it fails with the exact-to-minute input error; the slot is NEVER accepted.

- **GIVEN** a start of `15:00:00` and any non-zero sub-second component
- **WHEN** validation runs
- **THEN** it fails with the exact-to-minute input error.

- **GIVEN** a minute-exact start inside opening hours
- **WHEN** validation runs
- **THEN** behavior is unchanged from today.

### Requirement: ambiguous business_hours day keys fail closed
Stored or written `business_hours` JSON MUST reject a document in which two distinct keys normalize to the same `DayKey` (e.g. `"1"` and `"01"`), with an invalid-input error naming both keys. A zero-padded key as the sole form of its day keeps reading (legacy read compatibility).

- **GIVEN** stored business_hours `{"1": {...}, "01": {...}}`
- **WHEN** the profile parses (read path)
- **THEN** it fails with invalid input naming both `1` and `01`; neither value silently wins.

- **GIVEN** stored business_hours `{"01": {...}}` as the only form of that day
- **WHEN** the profile parses
- **THEN** it reads as DayKey 1 exactly as today (compatibility preserved).

- **GIVEN** the business_hours write path receives the ambiguous document
- **WHEN** validation runs
- **THEN** it is rejected with the same rule (single rule, read and write never drift).

### Requirement: console secret input has a single code path
The console TUI secret-entry plumbing MUST NOT carry unused secret-reader hooks: `promptSecret` MUST NOT accept an ignored `secretLine` parameter, and the `stdinSecretLine` hook MUST be removed from call sites. Behavior is unchanged: terminal stdin enters no-echo via `consoleInput.Secret()`'s `term.ReadPassword`, piped stdin enters echo-free via the shared consoleSource reader.

- **GIVEN** the Hermes-config console flow on a terminal
- **WHEN** the operator enters the bot token
- **THEN** the value is not echoed (unchanged behavior, now through the single path).

- **GIVEN** piped stdin (tests, scripts)
- **WHEN** the secret is read
- **THEN** the flow completes without terminal interaction, same as today.

### Requirement: purge failures are diagnosed server-side and reported honestly
The purge core MUST log the per-row cause server-side, with the account identifier MASKED, before returning the wrapped client error; the operator-facing text stays semantic and points at the service log. The batch report MUST label itself "Purga parcial" — never "Purga completa" — when the batch did not finish, in BOTH presentations (console flow and TUI), with the completed per-account rows listed before the sanitized hint.

- **GIVEN** a mid-batch `PurgeInactive` failure
- **WHEN** the batch aborts
- **THEN** the raw cause is written to the service log with the masked account id (never the raw id), and the returned error keeps the masked-id context.

- **GIVEN** the same aborted batch in the console flow
- **WHEN** the report is written
- **THEN** the count line reads `Purga parcial: N cuenta(s) eliminada(s) permanentemente de M pedidas`, the completed masked rows follow it, and the sanitized hint closes the report.

- **GIVEN** the same aborted batch in the TUI
- **WHEN** the result screen renders
- **THEN** the same `Purga parcial` wording and the sanitized hint are shown, with no raw cause and no raw identifier.

- **GIVEN** a batch that completes for every requested id
- **WHEN** the report is written
- **THEN** the count line reads `Purga completa` and no partial hint is appended.

## Final dispositions recorded (no requirements)
- **f5**: repo slog keeps raw ids — owner-signed plain-hash deviation (2026-08-20) stands as final posture; loopback-only, single trusted client.
- **f7**: purge per-outcome report stays count-based masked — the R3-001 masking prevails; enrichment with per-account outcomes would reopen the sealed raw-id surface.
