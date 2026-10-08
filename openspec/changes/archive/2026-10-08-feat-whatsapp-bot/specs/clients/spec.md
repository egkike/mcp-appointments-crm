# Delta for clients

> **Change**: feat-whatsapp-bot · D2
> **Domain**: clients — one ADDED requirement: the phone-keyed get-or-create port method used by registration
> **Type**: ADDED requirements only. Every existing `clients` requirement (phone uniqueness, no messenger fields, FTS/trigger sync, REQ-CL-AUTH-001..005) keeps its current text and scenarios.

## ADDED Requirements

### REQ-CL-PORT-001 — `GetOrCreateByPhone` creates with `id == phone`

The client repository port MUST expose the phone-keyed get-or-create method `GetOrCreateByPhone(ctx context.Context, phone, displayName string) (entity.Client, bool, error)` (name per `design.md` §1.4). The adapter MUST, inside a single transaction, insert with `id` equal to `phone` and `clients.phone` equal to the same value, using prepared statements with `?` placeholders only, and then read back the authoritative row; the returned bool MUST report whether this call created the row (derived from the insert's `RowsAffected()`, never from a prior read). When a row with `id == phone` already exists the method MUST return it unchanged, MUST NOT duplicate it and MUST NOT overwrite its name; the `phone` `UNIQUE` constraint remains the hard backstop. The stored `id` MUST be byte-identical to `phone`: no normalization, no characters added or removed.

Why this shape: `CallerResolver` matches `accounts.id` and then `clients.id`, so a client row whose `id` is not the phone can never resolve again.

**Explicitly not used by this flow:** `ClientsRepo.GetOrCreate` (`internal/repository/clients.go:223`) generates a UUID id, so the resolver can never match it from a phone. The registration path MUST NOT delegate to it (see `client-registration`, Requirement "Clients are created with `id == phone`").

**Scope:** this method is called by the registration use case before any caller has been resolved, so REQ-CL-AUTH-005 (own-phone-only for `client` callers) does not govern it — there is no role at that point. Its guards live in the use case: the `accounts` collision rejection and the per-phone rate limit (`client-registration`).

#### Scenario: Absent phone creates a resolvable row

- GIVEN no row in `clients` with `id = '+5491100999999'`
- WHEN `GetOrCreateByPhone(ctx, '+5491100999999', 'Cliente +5491100999999')` is called
- THEN a row MUST be created with `id = '+5491100999999'`, `phone = '+5491100999999'` and the given name
- AND the returned bool MUST be `true`

#### Scenario: Existing row is returned unchanged

- GIVEN an existing row with `id = '+5491100999999'` and name `Ana`
- WHEN `GetOrCreateByPhone(ctx, '+5491100999999', 'Otro Nombre')` is called
- THEN the existing row MUST be returned, the returned bool MUST be `false` and the stored name MUST still be `Ana`
- AND no second row MAY exist for that phone

#### Scenario: Concurrent calls for the same phone yield one row

- GIVEN two concurrent `GetOrCreateByPhone` calls for the same phone
- WHEN both complete
- THEN exactly one row MUST exist for that `id` and exactly one call MUST report `created = true`

#### Scenario: Statements are parameterized

- GIVEN the adapter source after this change
- WHEN the new statements are reviewed
- THEN every value MUST be bound with `?` (no concatenation) and the insert MUST run inside one transaction with rollback on error
