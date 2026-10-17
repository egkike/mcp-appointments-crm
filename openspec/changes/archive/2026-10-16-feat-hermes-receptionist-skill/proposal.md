# Proposal: feat-hermes-receptionist-skill — Conversational policy for Hermes on the business channel

> **Change:** feat-hermes-receptionist-skill
> **Status:** proposed (spec phase)
> **Inputs:** `odd/tasks/feat-hermes-receptionist-skill.md` (S1–S3 frozen), ADR-0020, ADR-0018 (channel model), ADR-0017 (Hermes config TUI), ADR-0009 (authorization), `docs/gateway-contract.md`
> **Other artifacts:** `design.md`, `specs/hermes-receptionist-skill/spec.md`, `tasks.md`
> **skill_resolution:** paths-injected (`cognitive-doc-design`)

---

## Executive Summary

ADR-0018 froze the channel model and `feat-whatsapp-bot` delivered the server side of self-service registration. What remains undefined is the **conversational layer**: how Hermes behaves when a client or unknown sender writes something that is not about the business — "Hola" with nothing else, a weather question, small talk.

The MCP server cannot solve this by design: it is a tool-call pipe, and those messages never reach it (ADR-0020 Context §3). The conversational policy must live **inside Hermes**, installed before the first message arrives, as a **static receptionist skill** written into the Hermes home. This change specifies that skill, its installation surface (install.sh copy + a TUI regeneration option), and a pointer from the gateway contract — with **zero Go server changes, zero new MCP tools, and zero schema changes** (ADR-0020 Decision 1).

Off-topic tone (owner decision 2026-10-16): **cordial y orientador** — greet with the name, state the role, offer the business's core services in 1–2 sentences, and never answer the off-topic content.

## Business Problem

- **Undefined role.** With no system-level instructions for the business channel, Hermes behaves as a generic assistant: it answers weather, jokes, generic advice — on the business's WhatsApp number.
- **Registration widens the surface.** `register_client` (delivered, rate-limited 10/hour per phone) means any stranger who writes to the business number becomes a `client` and may chat freely. Without policy, each conversation burns tokens of the LLM running in the owner's VPS.
- **Causal constraint.** A "Hola" never triggers a tool call, so a dynamically fetched policy cannot exist: the message is already processed before any tool could deliver it. The only causally valid model is install-at-setup.

## Product Outcome

- Hermes on the business channel is unambiguously the **assistant of the business** (`business_profile.name`), never a general assistant and never a human.
- Off-topic messages get a cordial 1–2 sentence redirect to the business's services; no tools are called for them; the conversation is not fed.
- The unknown-sender registration loop from `docs/gateway-contract.md` is folded into the skill as one more rule, plus explicit privacy guardrails (minimum data, no other clients' PII, semantic messages only).
- The owner can repair or refresh the policy from the admin TUI without hand-editing `~/.hermes/`.

## Scope

1. **Skill template** (`setup/templates/recepcionista-negocio/SKILL.md`): the receptionist policy in the **official Hermes skill format** (SKILL.md + frontmatter `name`/`description`/`version`, house section order When to Use → Procedure → Pitfalls → Verification, ≤60-char description). Placeholder `{business_name}` resolved at install time.
2. **install.sh block**: copy the template into the official Hermes skills directory (`~/.hermes/skills/recepcionista-negocio/SKILL.md` — standard officially documented at `hermes-agent.nousresearch.com/docs/user-guide/features/skills`) on fresh install; never overwrite a modified existing skill silently.
3. **Admin TUI, second step inside option 7 "Configurar Hermes" (owner decision 2)**: regenerate the skill from the repo template, with an explicit confirm before replacing an existing file. No new menu item.
4. **Gateway contract pointer**: a new section in `docs/gateway-contract.md` referencing the conversational policy and its bounds.

## Non-goals

- **No Go server changes, no new MCP tools, no schema changes.** The server keeps delivering surface only (ADR-0020 Decision 1); the 23 existing tools are sufficient.
- **No policy engine, no dynamic policy tool.** Static text only (ADR-0020 Decision 1 and its rationale).
- **No tone configurability in this change.** The owner chose one default ("cordial y orientador"); an override layer is a separate change if ever needed.
- **No Hermes runtime modifications.** The skill is data for the Hermes harness; how Hermes loads it is verified at apply, not implemented here.

## Storage/loading standard (resolved)

`~/.hermes/skills/<skill-slug>/SKILL.md` with SKILL.md frontmatter is the **officially documented** Hermes standard (verified 2026-10-16 against `docs/user-guide/features/skills`; the earlier apply-time assumption is closed). Loading is progressive disclosure — the system prompt indexes name + description and the full text loads on demand; the skill therefore advertises itself as applying to every conversation on the business channel. Runtime behaviors of note: the index refreshes at the next session start / compaction, and Hermes itself can modify or delete skills in `~/.hermes/skills/` (procedural memory) — the repo template stays the source of truth for regeneration.
