# Spec: client-registration

> Reference: `openspec/changes/feat-whatsapp-bot/proposal.md` (D2, D3), `design.md` §1, `exploration.md` §1–§3, ADR-0018, ADR-0011 · Change: feat-whatsapp-bot · Status: NEW

## Purpose

A phone that writes to the business channel for the first time must become a `client` without operator action. The only trusted identity signal is the gateway-injected `X-Caller-Id`, so registration works from that header alone: validate the phone, refuse phones already in `accounts`, apply a placeholder name when Hermes collects none, rate-limit per phone, and create the row with `id == phone` so the next call resolves through the read-only resolver chain.

## Requirements

### Requirement: `register_client` is the only anonymous-allowlisted tool

The server MUST expose a tool named `register_client` reachable without a resolved caller, and it MUST be the only one. Its path MUST be admitted by a static, code-owned allowlist evaluated before caller resolution (see `auth-middleware`). Every other tool MUST keep requiring a resolved caller and MUST answer `401` for unknown phones. A missing or empty `X-Caller-Id` MUST fail before the allowlist seam with the universal HTTP `401` (`"no se proporcionó X-Caller-Id"`, translated to JSON-RPC `-32000`, see `auth-middleware`): no caller is invented and no registration is attempted.

#### Scenario: Unknown phone registers through the anonymous path

- GIVEN no row in `accounts` and none in `clients` with `id = '+5491100999999'`
- WHEN an MCP request to `register_client` carries `X-Caller-Id: +5491100999999`
- THEN the request MUST reach the handler without resolution and without a fabricated role, and the use case MUST run

#### Scenario: Other tools still reject an unknown phone

- GIVEN no row in `accounts` or `clients` for `+5491100999999`
- WHEN an MCP request to any tool other than `register_client` carries that header
- THEN the server MUST answer HTTP `401` translated to JSON-RPC `-32000` and the handler MUST NOT execute

### Requirement: The phone comes from the header, never from the request body

The input MUST carry an optional `display_name` and MUST NOT carry any field that sets the phone or caller identity. Decoding MUST be strict: a phone-like key (`phone`, `caller_id`, `id`) or any unknown key MUST fail with a semantic error instead of being silently ignored.

> **Implementation note (apply, 2026-10-08):** the strict rejection is enforced by the SDK schema (`additionalProperties: false` inferred from the single-field input struct), so an unknown key fails as the SDK's tool-error envelope naming the key (protocol-layer validation, in English), never as a silent accept — consistent with the transport's existing protocol-validation precedent (REQ-MT-006 unknown tool → `-32601`; SDK arg validation → `-32602`). Business/semantic failures of the registration flow itself remain `-32002` Spanish (`SemanticError`).

#### Scenario: Body cannot set the phone

- GIVEN a request with `X-Caller-Id: +5491100999999` and body `{"display_name":"Ana","phone":"+5491100000000"}`
- WHEN the handler decodes the payload
- THEN the request MUST fail with a semantic error naming the unauthorized field, no row MUST be created for either number
- AND a body with only `display_name` MUST create the row with `clients.id = '+5491100999999'`

### Requirement: Clients are created with `id == phone`

The use case MUST create the `clients` row with `id` equal to the header phone and `clients.phone` set to the same value. It MUST NOT generate a UUID and MUST NOT delegate to `ClientsRepo.GetOrCreate` (`internal/repository/clients.go:223`), whose UUID id can never be resolved. The stored value MUST be byte-identical to the header — no normalization, no characters added or removed.

#### Scenario: Registered phone resolves on the next call

- GIVEN a successful registration of `+5491100999999`
- WHEN a subsequent client-role tool call carries `X-Caller-Id: +5491100999999`
- THEN the resolver MUST find the row by `id` and the caller MUST have `Role = "client"` with `ClientID` pointing at it

#### Scenario: Idempotent registration

- GIVEN an existing `clients` row with `id = '+5491100999999'` (and, separately, two concurrent calls for the same phone)
- WHEN `register_client` is called again with the same header
- THEN no duplicate row MAY exist and the response MUST report that no new client was created

### Requirement: Phones that already belong to `accounts` are rejected

Before any write, the use case MUST verify that no `accounts` row exists for the header phone — active or inactive. When one exists, registration MUST fail with a semantic Spanish error and MUST NOT create a client row.

#### Scenario: Account phone is rejected

- GIVEN an `accounts` row with `id = '+5491100000000'` (whether `is_active = 1` or `0`)
- WHEN `register_client` is called with `X-Caller-Id: +5491100000000`
- THEN the request MUST fail with a semantic Spanish error stating the number belongs to a business account, and no `clients` row MUST be created

### Requirement: Display name is optional with a placeholder

The stored name MUST be the trimmed `display_name` when supplied and non-empty; otherwise the use case MUST store `Cliente {phone}`. Absence of a name MUST NOT block or delay registration.

#### Scenario: Name handling

- GIVEN a request with `{"display_name":"  Ana Gómez  "}`
- WHEN registration succeeds
- THEN the stored name MUST be `Ana Gómez`
- AND a request with an empty or absent `display_name` for `+5491100999999` MUST store `Cliente +5491100999999`, usable for bookings

### Requirement: Phone format is validated

The header phone MUST satisfy `entity.Client.HasValidPhone` (optional leading `+`, minimum 4 digits — 5 with the `+` — and no upper bound in the entity validator) and MUST additionally carry at most 15 digits (E.164 maximum, per the requirement "Registration rejects phones longer than 15 digits"); no other rewriting is allowed because the stored `id` must match the exact bytes the gateway sends next time.

#### Scenario: Invalid phone is rejected

- GIVEN a request carrying `X-Caller-Id: 'abc'`
- WHEN the use case runs
- THEN the request MUST fail with a semantic Spanish error describing the accepted format and no row MUST be created

### Requirement: Registration rejects phones longer than 15 digits

`register_client` MUST reject a header phone carrying more than 15 digits (the E.164 maximum) with a semantic Spanish error and MUST NOT create a row. The cap applies to the registration path only: phone normalization/canonicalization for any other flow stays out of scope (see `design.md` §8).

#### Scenario: Over-long phone is rejected

- GIVEN a request carrying an `X-Caller-Id` header with 16 digits
- WHEN the use case runs
- THEN the request MUST fail with a semantic Spanish error describing the accepted format (4–15 digits, optional `+`) and no row MUST be created
- AND a 15-digit header phone MUST still register successfully

### Requirement: Registration is rate-limited per phone

The server MUST enforce a configurable per-phone limit with a conservative default of 10 per hour, tracked per phone (not globally, not per IP). Exhaustion MUST fail with a semantic Spanish error and MUST NOT create a row. A configured limit of `0` MUST disable auto-registration entirely (fail closed).

#### Scenario: Limit exhausted or disabled

- GIVEN a limit of 10/hour and 10 attempts already recorded for `+5491100999999` in the current window
- WHEN an 11th attempt arrives for that phone
- THEN the request MUST fail with a semantic Spanish error explaining the limit was reached and no row MUST be created; and with a configured limit of `0` any phone MUST receive the same rate-limit error with no row created

### Requirement: Registration is audited without exposing full phone numbers

Every outcome (created, no-op, account rejection, legacy-id rejection, rate-limit rejection) MUST emit an audit event whose phone value is masked to its last 4 digits. Audit events MUST NOT contain the full phone number and MUST NOT log secrets.

#### Scenario: Successful registration is audited

- GIVEN a successful registration for `+5491100999999`
- THEN the audit event MUST identify the event type and a masked phone, and MUST NOT contain the unmasked value

### Requirement: Only semantic errors reach the client

Registration failures MUST be `*domain.SemanticError` with Spanish natural-language messages. Internal errors (SQL, driver, filesystem) MUST collapse to the generic JSON-RPC `-32603` through `internal/mcp/errors.go:41-48` and MUST NOT leak SQL, paths or stack traces.

### Requirement: Gateway registration contract is documented

The repository MUST document the gateway-side protocol this contract enables: per-sender `X-Caller-Id` injection; the `401 → ask name (optional) → register_client → retry once` loop; the Telegram `request_contact` step needed to obtain a phone; and the scenarios A (owner number ≠ business number), B with WhatsApp self-chat (owner number = business number) and the first-time client case.

#### Scenario: Documentation covers the loop and the self-chat case

- GIVEN the gateway contract document after this change
- WHEN a reader looks for unknown-phone behavior
- THEN it MUST describe the 401 → register → retry-once sequence, state that the display name is optional and that a self-chat sender is a valid sender resolved through `accounts`

## Notes

- The seam grants no role; the use case MUST NOT accept a role parameter. The rate limiter is in-memory and single-process (reset on restart is accepted: loopback server, operator-initiated restarts). Phone normalization (E.164) is out of scope; see `design.md` §8.
