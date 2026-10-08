# Delta: business-profile (feat-whatsapp-bot)

> Reference: `openspec/changes/feat-whatsapp-bot/proposal.md` D4, `design.md` §2–§3, PRD §7 Fase N ("Contrato de ubicación").
> Change: feat-whatsapp-bot
> Type: ADDED requirements only — existing `business-profile` requirements (singleton row, lazy-init, messenger fields, `messenger_platform` CHECK) are unchanged. No schema migration.

## ADDED Requirements

### Requirement: Derived `maps_url` in `get_business_profile` output

When both `latitude` and `longitude` are set on the business profile, the `get_business_profile` MCP response MUST include a derived `maps_url` field whose value is `https://maps.google.com/?q=<lat>,<long>`, formatted with plain decimal notation. When either coordinate is `NULL`, the response MUST omit `maps_url` entirely. The existing `latitude`/`longitude` output fields MUST remain unchanged. The derivation MUST happen in the MCP transport layer only (no domain, port or repository change), because it is a wire-shape concern.

#### Scenario: Both coordinates set yields a maps link

- GIVEN a business profile with `latitude = -34.6037` and `longitude = -58.3816`
- WHEN `get_business_profile` is called
- THEN the response MUST contain `maps_url` equal to `https://maps.google.com/?q=-34.6037,-58.3816`
- AND it MUST still contain `latitude = -34.6037` and `longitude = -58.3816`

#### Scenario: Coordinates missing omits the field

- GIVEN a business profile whose `latitude` and `longitude` are `NULL`
- WHEN `get_business_profile` is called
- THEN the response MUST NOT contain `maps_url`

#### Scenario: Single coordinate omits the field

- GIVEN a business profile with `latitude` set and `longitude` `NULL`
- WHEN `get_business_profile` is called
- THEN the response MUST NOT contain `maps_url`
- AND the present coordinate MUST still be returned

#### Scenario: Formatting never leaks scientific notation

- GIVEN a business profile with `latitude = 0.0000001`
- WHEN `get_business_profile` is called
- THEN `maps_url` MUST contain `0.0000001` and MUST NOT contain an exponent such as `1e-07`

### Requirement: `geo:` URI accepted as alternative input for `update_business_profile`

`update_business_profile` MUST accept an optional `location_uri` string field carrying an RFC 5870 `geo:` URI in the strict form `geo:lat,long` (optional sign, decimal `.`, no whitespace, no parameters). When present, the transport adapter MUST parse it and populate the numeric latitude/longitude values passed to the use case; the use case MUST keep receiving normalized numbers only. Invalid, out-of-range (`lat ∉ [-90,90]`, `long ∉ [-180,180]`) or parameter-carrying URIs MUST fail with a semantic Spanish error and MUST NOT write a partial update. Supplying `location_uri` together with numeric `latitude`/`longitude`, or together with the F-4 clear flags, MUST fail with a semantic Spanish error.

#### Scenario: `geo:` input stores the same values as numeric input

- GIVEN a business profile with no coordinates
- WHEN `update_business_profile` is called with `location_uri = "geo:-34.6037,-58.3816"`
- THEN the stored `latitude` MUST be `-34.6037` and `longitude` MUST be `-58.3816`

#### Scenario: Malformed URI is rejected

- GIVEN any business profile
- WHEN `update_business_profile` is called with `location_uri = "geo:abc,def"`
- THEN the request MUST fail with a semantic Spanish error describing the expected `geo:lat,long` form
- AND the stored coordinates MUST NOT change

#### Scenario: Out-of-range coordinates are rejected

- GIVEN any business profile
- WHEN `update_business_profile` is called with `location_uri = "geo:120.0,10.0"`
- THEN the request MUST fail with a semantic Spanish error
- AND the stored coordinates MUST NOT change

#### Scenario: URI parameters are not silently ignored

- GIVEN any business profile
- WHEN `update_business_profile` is called with `location_uri = "geo:-34.6037,-58.3816;u=100"`
- THEN the request MUST fail with a semantic Spanish error
- AND the stored coordinates MUST NOT change

#### Scenario: Mixed URI and numeric input is rejected

- GIVEN any business profile
- WHEN `update_business_profile` is called with both `location_uri` and `latitude`/`longitude`
- THEN the request MUST fail with a semantic Spanish error stating that both forms cannot be combined
- AND the stored coordinates MUST NOT change

#### Scenario: Clear flags remain the only way to unload a coordinate

- GIVEN a business profile with coordinates set
- WHEN `update_business_profile` is called with the F-4 clear semantics (`latitude: null`)
- THEN the coordinate MUST be cleared exactly as before this change
- AND `location_uri` MUST NOT be involved

## Notes

- The derived field is additive; clients that ignore `maps_url` keep working unchanged.
- The use case signature does not change: `geo:` is a transport-level alias for the numeric fields (F-4 layering precedent, `internal/mcp/tools_maintenance.go:106`).
- The wire field name `location_uri` is selected to avoid colliding with `latitude`/`longitude`; renaming it before apply requires updating this delta and the `tasks.md` transport work unit.
