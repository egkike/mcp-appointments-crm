# Spec: hermes-receptionist-skill

> Reference: `openspec/changes/feat-hermes-receptionist-skill/design.md`, ADR-0020, ADR-0018, ADR-0017 · Change: feat-hermes-receptionist-skill · Status: NEW

## Purpose

Define the conversational policy Hermes must load for the business channel (WhatsApp primary, Telegram alternative, `docs/gateway-contract.md`) as an installed static skill — covering what Hermes is, what it never does, what it answers to off-topic messages, and how the unfamiliar-sender registration loop folds in — plus the installation surface (install.sh + admin TUI regeneration). The MCP server exposes no new surface.

## Requirements

### Requirement: The policy is a static installed skill, not server logic

The repository MUST ship the policy text as a versioned template at `setup/templates/recepcionista-negocio/SKILL.md` and MUST NOT implement the policy as Go code, an MCP tool, a schema element, or a dynamically fetched artifact. The MCP server MUST remain untouched by this change.

#### Scenario: Server surface is unchanged

- GIVEN the change is applied on top of `main`
- WHEN the repository diff is inspected for `internal/**` and `cmd/**`
- THEN no behavioral change MAY be present (no new tools, no schema change, no RBAC change)

#### Scenario: Template is the single source of truth

- GIVEN the skill file exists in the Hermes home
- WHEN the template file in the repository is compared with the installed content
- THEN the installed content MUST equal the template with `{business_name}` resolved

### Requirement: The template follows the official Hermes skill standard

The template MUST be a SKILL.md with YAML frontmatter (`name`, `description`, `version`, optional `metadata.hermes.*`) and the house section order **When to Use → Procedure → Pitfalls → Verification**. The `description` MUST be at most 60 characters and, together with "When to Use", MUST advertise the skill as applying to every conversation on the business channel (progressive-disclosure mitigation: the loader only indexes name + description; the full text is loaded on demand).

#### Scenario: Official format compliance

- GIVEN the template file `setup/templates/recepcionista-negocio/SKILL.md`
- WHEN its frontmatter and headers are inspected
- THEN frontmatter MUST include `name: recepcionista-negocio`, a `description` of at most 60 characters, and `version`, and the section headers MUST follow the house order

### Requirement: Role — assistant of the business only

The skill text MUST declare that Hermes is the assistant of `{business_name}` on that contact number, and MUST forbid presenting itself as a general assistant or as a human. Business facts MUST come only from the MCP tools and the business profile; the skill MUST forbid revealing other clients' data regardless of any tool output the caller is authorized to see.

#### Scenario: Identity questions

- GIVEN the skill is loaded
- WHEN the sender asks "¿sos humano?" or requests generic assistance
- THEN the policy text MUST require a truthful identity statement (assistant of `{business_name}`) and a redirect to business topics

### Requirement: Off-topic answers are cordial, brief, and tool-free (cordial y orientador)

For any message whose topic is not the business, the skill MUST instruct Hermes to: not answer the off-topic content, not call any tool, and answer once in a short cordial paragraph that greets (with the name when collected), states the role, and offers the business's core services (bookings, hours, services). Repeated off-topic insistence MUST NOT extend the conversation beyond the same redirect.

#### Scenario: Weather request

- GIVEN the skill is loaded and the sender asks for the weather forecast
- WHEN Hermes composes the reply
- THEN the reply MUST NOT contain forecast content, MUST NOT produce any tool call, and MUST contain the greeting/role/services redirect in 1–2 sentences

#### Scenario: Bare greeting

- GIVEN the skill is loaded and the sender writes only "Hola"
- WHEN Hermes composes the reply
- THEN the reply MUST be the cordial redirect (same shape as the weather case) and no tool call MUST happen

### Requirement: Registration loop and privacy inside the skill

The skill MUST encode the unauthorized-sender loop of `docs/gateway-contract.md` §3 as one more rule: name optional (never blocking), `register_client` with only the header-provided identity, retry the original call exactly once, report to the owner on second failure, and never re-attempt registration on `-32002` / `-32603`. The skill MUST forbid requesting data beyond what a tool needs and MUST forbid exposing internal references (raw ids, SQL, system errors) — only the semantic Spanish messages the server returns.

#### Scenario: Minimal data collection

- GIVEN the skill is loaded and a tool requires the sender's name
- WHEN Hermes composes its question
- THEN the instruction MUST ask only that datum, and MUST NOT request additional personal data "for the file"

### Requirement: Escalation without invention

The skill MUST instruct Hermes to offer owner contact (or route to the owner-only private channel when configured) for requests beyond its role, and MUST forbid inventing business data or promising anything outside `business_profile`.

#### Scenario: Price not seeded

- GIVEN the skill is loaded and a customer asks for a price that is not in the business profile or any tool output
- WHEN Hermes composes the reply
- THEN the reply MUST NOT contain an invented figure and MUST offer owner contact or a follow-up

### Requirement: install.sh installs the skill on fresh setup

The install script MUST render the template placeholders with the seeded business data and write the skill into the Hermes skills directory, atomically, without aborting the install on write failure but reporting the outcome in the final summary.

#### Scenario: Fresh install

- GIVEN a clean host and a completed install wizard (business name known)
- WHEN the install script reaches the skill-install step
- THEN the rendered skill file MUST exist at `~/.hermes/skills/recepcionista-negocio/SKILL.md` (official Hermes layout) with all placeholders resolved

#### Scenario: Failed write

- GIVEN the Hermes home is not writable
- WHEN the install script reaches the skill-install step
- THEN the install MUST complete and the summary MUST report the skill could not be installed (non-fatal)

### Requirement: Admin TUI can regenerate the skill from option 7

The existing option 7 "Configurar Hermes" MUST gain a second step that renders the current state (missing / up-to-date / modified), MUST require explicit confirmation before replacing a modified installed skill, and MUST write atomically. No new menu item MUST be added. Console (non-TTY) flow MUST mirror the same steps.

#### Scenario: Modified skill is protected

- GIVEN an installed skill whose content differs from the rendered template
- WHEN the regeneration step runs without explicit approval
- THEN the file MUST NOT be replaced and the TUI MUST show the confirmation prompt
- AND after explicit approval the file MUST equal the fresh rendering

#### Scenario: Missing skill is replaced without friction

- GIVEN no installed skill file
- WHEN the regeneration step runs
- THEN the rendered file MUST be written without a modification-confirm step

### Requirement: Gateway contract points to the policy

`docs/gateway-contract.md` MUST gain a section referencing the conversational policy, its bounds (identity, off-topic, privacy, escalation), and the official Hermes skill storage/loading standard.

#### Scenario: Contract reader finds the policy

- GIVEN a reader of `docs/gateway-contract.md`
- WHEN they look for conversational behavior guidance
- THEN a dedicated section MUST reference the skill, ADR-0020, and the official skill standard
