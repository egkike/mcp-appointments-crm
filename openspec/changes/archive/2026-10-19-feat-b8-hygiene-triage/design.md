# Design: feat-b8-hygiene-triage

Adapter map per ADR-0013 (dependencies point inward):

| Spec | Layer touched | File anchors |
|---|---|---|
| S1 | Application (use case auth mapping) | `internal/application/usecase/delete_schedule.go` |
| S2 | Domain service (availability) | `internal/domain/service/availability.go` |
| S3 | Domain service (validator) | `internal/domain/service/booking_time_validator.go` |
| S4 | Domain entity (business_hours parse) | `internal/domain/entity/business_profile.go` + write-path validator |
| S5 | Transport/UI adapter (console) | `cmd/mcp-server/admin_tui.go` |

No schema, no wire shapes, no new imports beyond stdlib.

## S1 — delete_schedule auth passthrough

Current code (delete_schedule.go:37-41):

```go
caller, err := auth.RequireRole(ctx, auth.RoleOwner)
if err != nil {
    return nil, &domain.SemanticError{Code: domain.ErrCodeForbidden, Message: "no tienes permiso para realizar esta acción", Cause: domain.ErrForbidden}
}
```

`RequireRole` returns either the pass-through error of `RequireCaller` (a
`*domain.SemanticError` with `ErrCodeUnauthenticated` when the context has no
caller) or its own `ErrCodeForbidden`. The blanket wrapper erases the
distinction. New behavior: preserve the original SemanticError unchanged.

```go
caller, err := auth.RequireRole(ctx, auth.RoleOwner)
if err != nil {
    return nil, domain.SemanticErrorAs(err) // helper, or inline errors.As unwrap
}
```

Both outcomes keep a `*domain.SemanticError` (transport adapter relies on
type identity in `internal/mcp/errors.go`); messages stay the ones
auth/helpers already produce. If the underlying error is somehow NOT a
SemanticError (programmer error surface), fall back to the current
Forbidden wrapper — fail-closed for the client, Cause carries the original.

Test matrix (usecase layer):
- no caller in ctx → `ErrCodeUnauthenticated` preserved (RED today: returns Forbidden).
- wrong role → `ErrCodeForbidden` preserved.
- happy path unchanged (existing test).

## S2 — availability passes localStart

`availability.go:130`: `Start: startTime` → `Start: localStart`.
Rationale: both operands of Step 1's past-check are the same instant;
`time.Now().In(slot.Start.Location())` is location-display only, so the
past-check is unaffected. Every downstream step (weekday, exception date,
`Format("15:04")`, `Format("2006-01-02")`) then runs in the business
timezone, matching `ResolveSlotContext` exactly.
The local `localStart` variable stays; `startTime` becomes the parse-error
holder only.

Test matrix (domain service, table-driven): for a fixed RFC3339 input with a
UTC offset that lands the instant in the NEXT business day (e.g. business
TZ `America/Argentina/Buenos_Aires`, input `23:30:00-03:00` when the
business day boundary is crossed), the validator's day-of-hour must come
from the business-local wall clock. Assert via the semantic message the
validator produces for a closed business day (spanishDayNames day label).

## S3 — fail-closed sub-minute starts

In `ValidateBookingTimeSlot` step 4 (before `slotStartHHMM`), add:

```go
if slot.Start.Nanosecond() != 0 || slot.Start.Second() != 0 {
    return &domain.SemanticError{
        Code:    domain.ErrCodeInvalidInput, // or the input validation code used by this validator — see note
        Message: "No se puede reservar en una hora que no sea exacta al minuto (sin segundos).",
    }
}
```

Note (resolved during实现): the validator's existing input-class failures
use domain error codes; the exact code follows the existing step-4 mechanism
(if step 4 already marks contract violations with `ErrCodeInvalidInput` or
`ErrValidation`, reuse it — no new codes).
Placement: step 4, BEFORE `Format("15:04")`, AFTER the nil-Service guard
(the contract check stays first). `Truncate(time.Minute)` everywhere after
becomes safe by construction.

Test matrix (validator unit): `15:00:30` against a `15:00` close → rejected
exact-to-minute; `15:00` → passes as today; `15:00:00.500` → also rejected.

## S4 — reject ambiguous business_hours duplicates

`parseBusinessHours` (entity) gains, after the sorted-keys loop maps each
unique key, a duplicate detection: when two different raw keys normalize to
the same `DayKey`, return a semantic invalid-input error naming both keys
and the day. Detection is O(1) per key with a small
`map[DayKey]string` "first key seen" alongside `result`:

```go
seen := make(map[DayKey]string, len(raw))
...
if prev, dup := seen[day]; dup {
    return nil, fmt.Errorf("la configuración de business_hours define el día %s dos veces (%s y %s): %w", day, prev, k, domain.ErrInvalidInput)
}
seen[day] = k
```

Design amendment recorded:
- Zero-padded legacy keys (`"01"`, `"007"`) remain readable **as the sole
  form of their day** — read-compat decision from the upstream
  review-e463099b71c8f9e1 R4-001 correction stands.
- An ambiguous stored document (two keys → same DayKey) is corrupt data:
  read path fails closed instead of silently letting the sort order pick a
  winner; write path rejects the same shape via
  `validateBusinessHoursJSON` (shared single-rule property of
  `parseBusinessHoursDayKey`, same comment contract).
- Test matrix (entity unit): `"{"1":{...},"01":{...}}"` → error; `"{"01":{...}}"`
  alone → reads as DayKey 1 (compat preserved); sort-order determinism
  assert the error names both keys; write-path validator shows the same
  rejection; single "1" unchanged.

## S5 — dead secret plumbing removal (behavior-preserving)

`promptSecret` drops the `secretLine` parameter (useless since
`consoleInput.Secret()` owns the tty/piped split). Call chain cleanup:
`stdinSecretLine` type + function, the three `stdinSecretLine(stdin)`
arguments at menu construction, the `secretLine secretLineReader` parameter
through `adminMenuOptions` / `runAdminMenu` / `runConfigureHermesFlow` /
prompt callers. The `secretLineReader` type and the GGA-round-15 comment
contract is updated to describe the surviving single path
(`consoleInput.Secret()`); behavior is unchanged (tests pipe stdin and
exercise the same branch as today). Any test currently passing a non-nil
hook stub is updated to pipe stdin directly (same technique
admin_tui uses today).

## Risk & rollback

- One slice, ~150–200 authored lines; revert = `git revert` of the PR squash.
- No persisted-data writes; S4 only makes previously-silently-tolerated
  ambiguous documents fail closed (documented as deliberate trigger).
- No new errors message shapes for the MCP client beyond S1's code
  fidelity (Unauthenticated is a standard code the transport already maps).
