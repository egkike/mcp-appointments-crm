# Design: feat-hermes-receptionist-skill

> **Change:** feat-hermes-receptionist-skill
> **Inputs:** `proposal.md`, ADR-0020 (architectural record), ADR-0018, ADR-0017, `docs/gateway-contract.md`
> **skill_resolution:** paths-injected

---

## 0. Layering contract (unchanged; ADR-0013)

No Go code paths change. The touchpoints of this change are:

| Layer | Location | This change adds |
|---|---|---|
| Skill content | `setup/templates/recepcionista-negocio/SKILL.md` | Static template, official Hermes skill format |
| Install | `scripts/install.sh` | Copy block into the Hermes skills directory (official layout) |
| UI adapter | `internal/tui` | Second step inside option 7 "Configurar Hermes": regenerate the skill |
| Docs | `docs/gateway-contract.md` | Pointer section to the policy |
| Server (Go) | — | **Nothing.** No tools, no schema, no RBAC changes |

The skill is **data at rest, not code**: it carries no secrets, no permissions and no executable logic. Defense-in-depth stays where it is — loopback binding, `X-Caller-Id` per sender, RBAC map + in-usecase `RequireRole`, registration rate limit (ADR-0009, ADR-0018, gateway-contract §7.3).

## 1. Installation model (ADR-0020 Decision 1)

```
repo: setup/templates/recepcionista-negocio/SKILL.md          (versioned source of truth)
        │ install.sh (fresh install) | admin TUI option 7, step 2 (regenerate)
        ▼
~/.hermes/skills/recepcionista-negocio/SKILL.md               (resolved: {business_name})
```

**Official storage standard (owner instruction 1, verified 2026-10-16):** all skills live in
**`~/.hermes/skills/`** — the primary directory and source of truth (official Hermes skills
docs, `docs/user-guide/features/skills`). Each skill lives in its own directory whose name
becomes the install slug (`~/.hermes/skills/recepcionista-negocio/SKILL.md`); directory names
starting with `.` or `_` are ignored. The skill file uses standard SKILL.md frontmatter
(`name`, `description`, `version`, optional `platforms` / `metadata.hermes.*`) and the house
section order **When to Use → Procedure → Pitfalls → Verification**, with a house-bound
≤60-char description.

- **Fresh install:** the wizard already knows the business name (it seeds `business_profile`); after the seed, the install script renders the placeholder and writes the skill file. `0600`, like the other Hermes-home artifacts. A failed write does not abort the install — the skill is an enhancement, not a dependency — but it MUST be reported in the final summary.
- **Regeneration (owner decision 2 — second step inside option 7 "Configurar Hermes", no new menu item):** idempotent replace from the template. The file contents are computable (template + business name), so the TUI can detect "matches last installed template" vs "modified". Modified files MUST be confirmed before replacement (same convention as the `[R]esumir / [S]tart over` checkpoint of install.sh). After a write, the TUI MUST state that Hermes picks the updated skill index up on the next session start / compaction (official loader behavior) — on fresh installs no extra step is needed, since the file lands before the first conversation.
- **Progressive disclosure nuance (official loader):** the system prompt carries only the level-0 index (name + description); the full text loads on demand via `skill_view`. The skill is therefore not guaranteed in-context at message 1. Mitigation by authoring, not by code: `description` and "When to Use" are phrased as applying to **every conversation on the business channel**, so any messaging turn lists the skill in the index and triggers the load.
- **Agent autonomy note (official docs):** Hermes can modify or delete any skill in `~/.hermes/skills/` (its procedural memory). The repo template remains the source of truth; a regeneration re-seeds from it. Optional hardening (out of scope here): `skills.write_approval: true` in `~/.hermes/config.yaml`.

### 1.1 Storage and loading — resolved against the official standard

Verified against the official Hermes skills documentation
(`https://hermes-agent.nousresearch.com/docs/user-guide/features/skills`, read 2026-10-16):

- storage: `~/.hermes/skills/<skill-slug>/SKILL.md` (our assumed layout IS the official one);
- format: SKILL.md + YAML frontmatter (`name`, `description` ≤60 chars, `version`; optional
  `platforms`, `metadata.hermes.{tags,category,config}`);
- loading: progressive disclosure — level 0 index (name + description) always present in the
  system prompt; full text on demand via `skill_view`; index refresh visible at the next
  session start / compaction (`/reload-skills` refreshes only the slash commands).

No further runtime reconciliation remains for apply, beyond the installed-host smoke of §5.

## 2. Skill design (ADR-0020 Decisions 2–5)

### 2.1 Role and identity

- Hermes declares itself the assistant of `{business_name}` — never a general assistant.
- It never claims to be human.
- Business facts come only from the MCP tools and `business_profile`; no other clients' data, even when the caller's RBAC can read it: RBAC is the server's permission, not a conversational license.

### 2.2 Off-topic policy (owner decision "cordial y orientador", verbatim)

> "Saluda con nombre y ofrece ayuda del negocio en un párrafo breve (1-2 oraciones); sin responder el off-topic."

Rules:

1. Detect the topic is not the business (greetings alone, weather, jokes, generic advice).
2. Do NOT answer the content, do NOT call any tool, do NOT start any flow.
3. Reply with one short paragraph: greeting (with name if collected), role statement, offer of the core services (bookings, hours, services).

Canonical redirect for the template (same Rioplatense register as the project's person-facing errors):

> «¡Hola {nombre}! En este número te atiende el asistente de {business_name}: te puedo
> ayudar con turnos, horarios y servicios. ¿Querés ver si hay algún turno disponible?»

### 2.3 Unknown-sender registration (from gateway-contract §3, verbatim contract)

The skill carries the loop as a rule: on `-32000` "no te reconozco", ask for a name (optional, never blocking), call `register_client {display_name?}`, retry the original call exactly once, and report to the owner if the retry still fails. Never feed `-32002`/`-32603` back into `register_client`.

### 2.4 Privacy

- Ask only the data a tool requires — nothing more "for the file".
- Reveal only public business-profile information; never another client's data.
- Never expose internal references (raw ids, SQL, system errors): only the semantic Spanish messages the server returns (they are already sanitized by `internal/mcp/errors.go`).

### 2.5 Escalation (ADR-0020 Decision 5)

Requests beyond the role: offer owner contact cordially (or route through the owner-only private channel when configured). Forbidden: inventing business data, promising anything outside `business_profile`.

### 2.6 Language

Spanish as the default register (Rioplatense, matching the server's semantic errors); reply in the language of the interlocutor when it helps comprehension.

## 3. Skill template draft (apply deliverable; the authoritative text at apply)

```markdown
---
name: recepcionista-negocio
description: Política conversacional del canal del negocio
version: 1.0.0
metadata:
  hermes:
    tags: [negocio, turnos, whatsapp]
    category: negocio
---

# When to Use

Aplica a TODA conversación que entra por el canal de mensajería del negocio
(WhatsApp primario, Telegram alternativo): saludos, consultas de turnos,
horarios, servicios, y cualquier mensaje que no trate del negocio.

## Procedure

1. Te presentás como el asistente de {business_name}; nunca como humano ni
   como asistente general.
2. Los datos del negocio salen solo de las herramientas MCP y del perfil del
   negocio. No compartás datos de otros clientes (nombres, teléfonos, turnos)
   con nadie, aunque alguna herramienta te los muestre.
3. Si el mensaje no trata del negocio (saludo a secas, clima, chistes,
   consejos), respondé UNA sola vez, en 1-2 oraciones cordiales, y no uses
   ninguna herramienta. Ejemplo:

   «¡Hola {nombre}! En este número te atiende el asistente de {business_name}:
   te puedo ayudar con turnos, horarios y servicios. ¿Querés ver si hay algún
   turno disponible?»

   {nombre} es el nombre que la persona dio al registrarse; si no dio ninguno,
   omitilo. Ante insistencia del mismo tema: mismos renglones, no discutir.
4. Si una herramienta responde que no reconoce a la persona, seguí el loop:
   pedí el nombre de forma OPCIONAL (seguís igual si no lo da), registrála con
   la herramienta de registro y reintentá el pedido original UNA sola vez.
   Pedí únicamente el dato que la herramienta necesita. Si el registro o el
   reintento falla, avisale al dueño; nunca loopees ni reintentes registros
   tras un rechazo de negocio.
5. Si el pedido excede tu rol (precios no cargados, borrar datos, casos
   especiales): aclaralo con amabilidad y ofrecé contacto del titular. Nunca
   inventes datos ni prometas lo que el perfil del negocio no respalda.
6. Español por defecto. Si la persona escribe en otro idioma, respondéle en
   su idioma.

## Pitfalls

- Responder el contenido fuera de tema (clima, chistes, consejos): prohibido —
  redirigí al negocio en 1-2 oraciones y nada más.
- Llamar herramientas para mensajes fuera de tema: prohibido.
- Exponer identificadores internos, SQL, errores de sistema: prohibido — solo
  los mensajes semánticos en español que el servidor devuelve.
- Inventar datos del negocio (precios, horarios, promociones): prohibido.
- Escribir texto en inglés a clientes hispanohablantes: salvo que la persona
  escriba en otro idioma, respondé en español.

## Verification

- Ante un saludo a secas o una pregunta de clima, la respuesta es el párrafo
  cordial de redirección y no se produjo ningún tool call.
- Ante una consulta de turnos/servicios, se usaron las herramientas del negocio
  y la respuesta cita solo datos devueltos por ellas.
- Ante un emisor desconocido, el flujo de registro corrió a lo sumo una vez y
  con el dato mínimo necesario.
```

Placeholders at install: `{business_name}` only (from the seed/install data). `{nombre}` is a runtime variable (the name collected during registration), not an install placeholder. Slug: `recepcionista-negocio` (official install slug = directory name).

## 4. TUI — second step inside option 7 "Configurar Hermes" (owner decision 2)

No new menu item: the regeneration flow runs as an additional step of the existing option 7 (`capabilityConfigureHermes`), after the current Telegram owner-bot field group.

- MVU screens: (1) show current installed-vs-template state (missing / up-to-date / modified); (2) confirm replacement when modified; (3) write rendered content atomically (temp file + rename), never echo business data beyond the name; (4) Spanish success/failure messages noting that Hermes picks the skill index up at the next session start / compaction; per-field validation is not needed (no data entry beyond confirmation).
- Console (non-TTY) flow mirrors the same steps (same pattern option 7 already follows for Telegram fields).

## 5. Testing approach

| Layer | Test focus |
|---|---|
| Skill template | structural: official-format compliance (frontmatter name/description ≤60 chars/version; house section order When to Use → Procedure → Pitfalls → Verification), `{business_name}` placeholder present exactly once, no secrets, no runtime |
| install.sh | copy renders placeholder, refuses overwriting a modified skill without flag, reports outcome; shellcheck-clean; tests under `scripts/tests/` |
| TUI | capability unreachable in tests of unrelated flows (differential guard); regen write atomic in `t.TempDir()`; modified-file confirm path; Spanish messages |
| Smoke (Hermes runtime) | "Hola" sin tools + respuesta cordial; pregunta de clima → off-topic redirect; registro del desconocido se dispara solo en `-32000` — verified manually at apply, documented as evidence, not automated in this repo |
| Docs | structural readback of `gateway-contract.md` §8 pointer |

Commands (per AGENTS.md, at phase gates): `go fmt`, `go vet`, `golangci-lint`, `go build -o /dev/null ./...`, `go test -v -race ./...`. For the skill/install/docs work itself, verification is structural per the Verification & Review Protocol routing (docs → readback, not native review).

## 6. Security review

| Surface | Control |
|---|---|
| Secrets | the template carries none; write permissions `0600`; masked output |
| Privilege escalation | zero — the skill grants nothing; RBAC stays the only gate (ADR-0009) |
| Prompt-injection via inbound messages | out of scope for the server (no new surface); recorded as a Hermes-side follow-up: the skill forbids sharing other clients' data regardless of what the message asks |
| Data exposure | skill forces business-profile-only facts + server-semantic errors only |
| Cost/abuse | off-topic short-circuit reduces token spend; registration rate limit (10/hour, fail-closed) unchanged from §7.3 of gateway-contract |
| Atomicity | temp-file + rename writes; failed write leaves the original skill intact |

## 7. Rollout / rollback

**Rollout order:** (1) template file; (2) install.sh block; (3) TUI option; (4) gateway-contract pointer. Steps 2–4 are independently revertible; no migrations, no data changes, nothing destructive.

**Rollback:** delete the skill file in the Hermes home and revert the docs pointer — Hermes falls back to its generic behavior, which is exactly today's state (documented risk accepted in ADR-0020 Consequences).
