# Proposal: feat-b8-hygiene-triage — B8 triage f1–f7 owner decisions applied

**Backlog**: B8 (`odd/tasks/backlog.md`, queue from B5/B9 pre-existing findings) · **Route**: code → feature branch → single hygiene PR (owner decision 2026-10-19: "PR único higiene B8") · **Scope estimate**: ~150–200 lines (code + tests), one slice

## Summary

The B8 queue collected seven pre-existing findings across the B5 and B9 implementations. All seven were triaged in one owner batch on 2026-10-19. This change applies the five actionable decisions (f1, f2, f3, f4, f6) and records the two final dispositions (f5, f7). No new behavior is introduced — every item restores an intended contract or removes dead plumbing.

## Outcome & scope

- **(S1/f1) delete_schedule auth-code passthrough**: `DeleteScheduleUseCase.Execute` currently collapses ALL `auth.RequireRole` failures into `ErrCodeForbidden` ("no tienes permiso"), erasing `ErrCodeUnauthenticated` from a missing caller. Fix: `errors.As`-based passthrough of the original `*domain.SemanticError` so the LLM sees the real code (unauthenticated = missing caller identity; forbidden = role mismatch). Scope is delete_schedule only; the shared use-case pattern elsewhere is a documented non-goal.
- **(S2/f3) availability validator input**: `availability.go` resolves day/exception with `localStart := startTime.In(loc)` but passes the raw parsed `startTime` (which keeps the RFC3339 input offset) to `ValidateBookingTimeSlot`. With an input offset different from the business timezone, `slot.Start.Format("15:04")` validates the wrong wall-hour. Fix: pass `localStart`, aligning availability with the `ResolveSlotContext`/`LocalStart` convention already used by create/reschedule.
- **(S3/f4) fail-closed on sub-minute starts**: the validator truncates seconds via `slot.Start.Format("15:04")`; a `15:00:30` start passes a `15:00` close bound. Owner decision: **fail-closed** — a start with non-zero seconds or sub-second components is rejected as invalid input ("la hora de inicio debe ser exacta al minuto (sin segundos)"), never accepted by truncation.
- **(S4/f2) business_hours normalized-duplicate ambiguity is corrupt data**: `parseBusinessHoursDayKey` normalizes zero-padded legacy keys (`"01"`, `"007"`) for read compatibility (decision recorded upstream by the R4-001 correction in review-e463099b71c8f9e1: read-only compat, sign-prefixed keys stay rejected). When stored JSON contains TWO distinct keys normalizing to the same DayKey (`"01"` and `"1"`), the sorted last key silently overwrites the first. Owner decision: **design amendment now** — read normalization stays, but a stored document containing duplicate normalization targets is rejected fail-closed (`ErrInvalidInput`, semantic message naming the day), on both read and write validation paths. The canonical emitted form remains `"1".."7"`.
- **(S5/f6) console secret plumbing cleanup**: `promptSecret(..., secretLine secretLineReader)` ignores its `secretLine` argument entirely (calls `input.Secret()`, which correctly covers tty no-echo via `term.ReadPassword` and piped echo-free `Line`). The `stdinSecretLine` hook is plumbed through three call sites and two intermediate functions for nothing. Owner decision: **limpieza** — remove the dead parameter and the dead hook plumbing (behavior-preserving; no functional change, `input.Secret()` remains the single secret path).

## Final dispositions recorded (no code)

- **(f5) repo slog raw ids**: owner-signed plain-hash deviation (2026-08-20) stands; loopback-only, single trusted client, keyed-HMAC adds debug surface without a real threat change. Recorded as final.
- **(f7) purge per-outcome enrichment** (advisory R4-001, review-679ea111686a7190): the masked partial report prevails; adding "which accounts" per outcome would reopen the exact raw-id surface the R3-001 correction sealed. Recorded as final.

## Non-goals

- No rewrite of the RequireRole/forbidden pattern in other use cases (delete_schedule is the scoped fix; a cross-cutting readback goes to a future hygiene batch if evidence accumulates).
- No migration of existing stored business_hours JSON (zero-padded keys keep reading fine; only ambiguous duplicates are rejected).
- No new tool, no schema change, no dependency change.
- Windows/tui-local-echo behavior changes: none (S5 is dead-code removal).

## Evidence anchors (verified against main, 2026-10-19)

- `internal/application/usecase/delete_schedule.go:38-40` — unconditional Forbidden mapping after `RequireRole`.
- `internal/auth/helpers.go:32-47` — `RequireRole` passes through `RequireCaller`'s error (which yields `ErrCodeUnauthenticated` without a caller).
- `internal/domain/service/availability.go:103-131` — `startTime` parsed, `localStart` computed, validator receives `Start: startTime`.
- `internal/domain/service/slot_context.go:95-110` + `internal/application/usecase/create_booking.go:145` — `LocalStart` convention.
- `internal/domain/service/booking_time_validator.go:181` — `Format("15:04")` truncation.
- `internal/domain/entity/business_profile.go:107-119` (parse) — `:147-169` (silent-overwrite loop; sorted keys `"01"` < `"1"`).
- `cmd/mcp-server/admin_tui.go:949-982` (`stdinSecretLine`, `promptSecret`), `:1106-1118` (`consoleInput.Secret()` real path), `:194/:209/:252` (`stdinSecretLine` call sites), `:412/:538/:1321` (dead parameter transport).
