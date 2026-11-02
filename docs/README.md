# Docs index — language conventions

> **Language rule (owner decision 2026-11-01, option C):** one canonical language
> per document, chosen by **audience** — never duplicated per-document translations.
> A document's language is part of its contract; translations live in summaries
> only if ever needed, not as full copies.

| Document | Language | Audience | Why |
|---|---|---|---|
| [`README.md`](../README.md) | EN | GitHub public + any MCP agent integrator | repo landing; the **standard install is Hermes**; other agents → connector guide |
| [`AGENTS.md`](../AGENTS.md) | EN | dev agents + contributors | coding conventions read by automated agents |
| [`gateway-contract.md`](./gateway-contract.md) | EN | protocol/contract readers (any agent framework) | it is a cross-system contract: protocol-level artifacts stay EN (same rule as code comments and PR titles) |
| [`deployment.md`](./docs/deployment.md) | EN | release/infra ops | release engineering with `release.yml`/GoReleaser; **migration ES = pending task (optional)** — same runbook audience as `installation.md` |
| [`installation.md`](./installation.md) | ES | the owner/operator doing a from-zero install | operator manual |
| [`PRD.md`](./PRD.md) | ES | the owner (product decisions, changelog, roadmap) | product source of truth |
| [`SDD.md`](./SDD.md) | ES | the owner (original analysis) | historical product doc |
| [`maintenance.md`](./maintenance.md) | ES | the owner/operator (annual support) | operator manual |
| [`demo-plan.md`](./demo-plan.md) | ES | the owner/operator (demos and smokes) | operator manual |
| [`architecture/*` (ADRs)](./architecture/) | EN | future maintainers + agents | decision records follow the EN convention of contracts |
| [`odd/*` (feature tracking)](../odd/) | mixed (task body follows the delegation language) | the owner + dev agents | ODD tracking docs |

**Principles**

1. **No per-document duplicated translations** (no `README.es.md` style pairs).
   The audience decides; keeping two full copies alive is a drift guarantee.
2. **Mixed-in-one-file is allowed only as a summary**, never as parallel full
   translations (the pattern used by the root README: English body + Spanish
   quoted notes where the owner's operating language helps).
3. **New docs pick a language at birth** following two questions: (a) Who reads
   it first? (b) Is it a product artifact (EN, like contracts/code) or an
   operator manual (ES)? Record the choice in the table above.
4. Technical artifacts embedded in the product (code comments, wire strings,
   tests, PR titles) are outside this table — they follow [AGENTS.md](../AGENTS.md)
   conventions (English).
