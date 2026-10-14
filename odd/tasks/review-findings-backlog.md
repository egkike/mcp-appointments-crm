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
| `review-86d213546c1defe7` | A3 parity guard (PR #123, squash da0dd93, approved + acknowledged 2026-10-14) | 2 SUGGESTION |

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

## Suggested next batch (EXECUTED 2026-10-14 — see disposition below)

A single small refactor PR could absorb the mechanical items: the errors.go HTTP texts (2 strings), the IsOpenOn unexport decision (needs the config round-trip test moved), the dto json-tags cleanup (drop dead tags or document), and the Config-builder consolidation. The F-4 WARNING and the remaining items need individual judgment — see each entry.

## Disposition (2026-10-14) — mechanical batch EXECUTED

Delivered via PR #121 (tracking issue #120, squash `0a04f7c` on main; branch `feat/backlog-mechanical-batch`, 4 commits: `a5ca74a` → `3b6898d` → `7daa347` → `6464781`; CI green; native assess `reviewDue=false` medium/under_budget; human review by owner).

**Resolved by the batch:**

- GGA transport suggestions: errors.go HTTP texts EN→ES (pinned by `TestJSONParseGuard_HTTPBodyMessagesSpanish`).
- `R3-isopenon-footgun`: resolved further than proposed — the lenient wrapper was removed entirely (no production caller ever materialized); lookups go through `GetOpenClose`, config round-trip test rewired.
- `R3-dto-wire-tags`: dead tags dropped from the three exception INPUT carriers; `DeleteHoursExceptionResult` documented as the live wire shape; `Caller json:"-"` restored + guard-pinned (a GGA CRITICAL on the first draft — tag dropped by mistake — was fixed RED→GREEN).
- `R3-config-literal-triplication`: mock sites share `configFromMockPorts` with per-site overrides; integration literal documented as the intentional composition-root mirror.
- Batch-grown (GGA rounds on the entity candidate): mandatory identity scalars + slot-interval validation (wizard-exact semantics), uniform `*domain.SemanticError` in `Validate()` (detailed Spanish messages now reach the LLM), NaN/Inf coordinate guards, write-gate strict day keys (read leniency stays pinned), context-wrapped `json.Unmarshal` read error, `businessHoursDay` in-code exception comment.

**Still open (follow-ups, owner-approved):**

1. ~~**A2 — ADR line**~~ — **EXECUTED 2026-10-14** (commit `a7e365c`, ahead of this list): ADR-0013 Positive/Dominio puro now carries the wire-tag carve-out note; the in-code comment and the ADR cross-reference.
2. ~~**A3 — parity test**~~ — **EXECUTED 2026-10-14**: reflect-based parity guard in `internal/application/usecase/update_business_profile_parity_test.go` (PR #122/#123, review `review-86d213546c1defe7` approved; RED verified via three temporary drift mutations). A future unlisted field can no longer be silently dropped or accepted-without-apply. Its two new informational findings are listed below.
3. ~~**A4 — RBAC inversion**~~ — **EXECUTED 2026-10-14, owner decision: document the port as generic** (tightening was rejected as coupling the adapter to one caller's policy, diverging from the nine sibling repo-layer re-asserts). `domainrepo.BusinessProfileRepo.Update` now carries a role-generic doc comment (PR #124/#125, squash `9835b9f`; commit `docs(domain): …` on `test/backlog-a4-port-doc`): caller-scoped owner-only narrowing stays in the use case (ADR-0015 D2). Native review `review-be71c677a3fbb501` approved (medium/review-reliability, comment-only 8 lines) + acknowledged on glm-5.3-flash relay.
4. **Advisory — INPUT-tag duplication pattern**: input DTOs duplicating wire tags that live in the transport (`create_booking` precedent, pinned by `TestDTOFieldTags`) — repo-wide convention, needs a design decision (same family as R3-dto-wire-tags).
5. All remaining informational findings listed above (F-4 WARNING, TOCTOU read-back window, R3-empty-query-mux, R3-log-attrs, R3-rule-column, design-prefix examples) — none reassessed since this doc was created.

### From the A3 parity-guard review (review-86d213546c1defe7)

- **A3-R3-1** — `internal/application/usecase/update_business_profile_parity_test.go:41-46` (SUGGESTION, reliability).
- **A3-R3-2** — `internal/application/usecase/update_business_profile_parity_test.go:107-109` (SUGGESTION, reliability).
