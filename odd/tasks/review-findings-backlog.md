# Review findings backlog — consolidated informational findings from native reviews

**Type**: backlog doc (informational findings only — no blockers in any review)
**Created**: 2026-10-12 — consolidates every native-review finding left as "separate later work"
**Source lineages** (all APPROVED + acknowledged, authorities burned on gentle-ai 4.1.1):

| Lineage | Candidate | Findings |
|---|---|---|
| `review-aa28c86d3b625bed` | F-4 clear-null fields (PR #105) | 7 (1 WARNING + 6 SUGGESTION) |
| `review-d30af8ec9a65d464` | backlog sweep batch (PR #115, squash 237cdc8) | 4 SUGGESTION |
| `review-5534a1380c6262fe` | hours-exception phase 1 (PR #116, squash 2105f9d) | 3 SUGGESTION |
| `review-688b86f460c4be9c` | hours-exception phase 2 (PR #117, squash 4467f51) | 1 SUGGESTION |

Plus the 2 GGA transport suggestions from the #115 commit-hook review. Everything below is non-blocking by definition (no correction transition was offered for any candidate); each item is a candidate for a small future refactor batch.

## From F-4 review (review-aa28c86d3b625bed)

1. WARNING (readability) — `internal/mcp/tools_maintenance_test.go:316-322`.
2. SUGGESTION — `internal/application/usecase/update_business_profile.go:171-173`.
3. SUGGESTION — `docs/PRD.md:1464`.
4. SUGGESTION — `internal/application/dto/maintenance.go:34-35`.
5. SUGGESTION — `internal/mcp/tools_maintenance_test.go:317-322`.
6. SUGGESTION — `internal/mcp/tools_maintenance.go:106-109`.
(Exact wording in the F-4 review store, revision `sha256:a757093a…`; recorded in `odd/tasks/f4-clear-null-fields.md` § Cierre del review nativo.)

## From the sweep review (review-d30af8ec9a65d464)

- **R3-log-attrs** — `internal/mcp/errors.go:60`: the infra-failure log embeds the raw error (may carry caller-derived fragments). Server-side only; use bounded/summary attrs if a log sink ever forwards records.
- **R3-empty-query-mux** — `internal/mcp/tools_search_test.go:41`: empty-query tests run through the integration fixture inside the unit test file (temp SQLite per test); heavier failure mode in constrained CI.
- **R3-rule-column** — `internal/repository/validation_test.go:209`: the shape-contract `rule` column is documentation-only (branch assignment pinned indirectly via distinct messages in TestValidateFTSQuerySemanticMessages).
- **R3-isopenon-footgun** — `internal/domain/entity/business_profile.go:76`: `IsOpenOn` stays an exported lenient wrapper (corrupt → false) with no production caller; consider unexporting if none materializes.

## From the phase-1 review (review-5534a1380c6262fe)

- **R3-readback-window** — `internal/application/usecase/create_hours_exception.go:79-90`: create read-back TOCTOU (concurrent delete → generic -32603, fail-closed, design-accepted §1.1); a `Create … RETURNING` port widening (the #114 pattern) would close it when the no-repository-change constraint is lifted.
- **R3-dto-wire-tags** — `internal/application/dto/maintenance.go:141-184`: the exception DTOs' json tags are dead on the wire (phase-2 handlers own the shape); mirrors the file convention — bookkeeping only.
- **R3-design-prefix-examples** — `openspec/changes/feat-hours-exception-tool/design.md:144`: §4 example messages carry an "Error: " prefix the actual validateExceptionDate messages never had (post geo-unification convention); drop the prefix in the examples.

## From the phase-2 review (review-688b86f460c4be9c)

- **R3-config-literal-triplication** — `internal/mcp/tools_hours_exception_test.go:600`: the 23-field Config literal is now triplicated across test construction sites (newToolServer, newIntegrationMuxWithOptions, newHoursExceptionUseCaseServer) plus main.go; a shared Config builder with per-test port overrides would collapse them (the expectedToolCount safety net only fires when the count constant is bumped too).

## From the GGA commit-hook review of #115

- English HTTP-level texts in `internal/mcp/errors.go` (:73, :77) vs the Spanish sibling (`auth_translator.go:61`) — consistency fix.
- Triple POST-body buffering per request (auth_translator.go:57 → errors.go:71 → errors.go:152, each up to 1 MiB+1); the translator could pass the parsed name/body down.

## Suggested next batch

A single small refactor PR could absorb the mechanical items: the errors.go HTTP texts (2 strings), the IsOpenOn unexport decision (needs the config round-trip test moved), the dto json-tags cleanup (drop dead tags or document), and the Config-builder consolidation. The F-4 WARNING and the remaining items need individual judgment — see each entry.
