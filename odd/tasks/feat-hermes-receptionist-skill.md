# feat-hermes-receptionist-skill

Doc-first feature: conversational policy for Hermes on the business channel, delivered as an installed receptionist skill + ADR + openspec change.

## Specs

- **S1** (user request, 2026-10-16, verbatim): "Se trata del comportamiento que tendría el agente Hermes en las comunicaciones que tenga atravez de Whatsapp con Clientes o Desconocidos que le contacten... y me refiero a consultas o mensajes que le escriban a Hermes pero que no tengan nada que ver con el negocio (por ejemplo que alguien le escribe y le pide que le diga el pronostico del clima) o simplemente le diga "Hola" y no mencione nada acerca del negocio... Cómo debería responder Hermes? Tenemos forma de controlar esas situaciones desde nuestro mcp? Sería convewniente diseñar Skills y Harness para aplicar a Hermes desde nuestro mcp?"
- **S2** (owner decision 2026-10-16): "ADR + spec ODD ahora" — ADR + openspec change (docs-first); la skill se instala vía install.sh que copia el template a `~/.hermes/skills/` y TUI con opción para regenerar la skill. El contenido Go es mínimo; el diseño es del doc.
- **S3** (owner decision 2026-10-16): tono del default para mensajes fuera de tema = "Cordial y orientador" — "Saluda con nombre y ofrece ayuda del negocio en un párrafo breve (1-2 oraciones); sin responder el off-topic."

## Tasks

- **T1** (S1,S2) · route docs · ADR-0020 `docs/architecture/0020-hermes-conversational-policy.md` — **DONE** (pendiente commit): Context con 3 baselines, 5 decisiones, consecuencias, follow-ups.
- **T2** (S2,S3) · route docs · openspec change `openspec/changes/feat-hermes-receptionist-skill/` — **DONE** (pendiente commit): proposal.md, design.md, tasks.md, specs/hermes-receptionist-skill/spec.md (7 requirements con escenarios GIVEN/WHEN/THEN).
- **T3** (S3) · route docs · Skill template draft — **DONE** como appendix en design.md §3 (placeholder `{business_name}` resuelto al instalar, off-topic cordial y orientador, registro, privacidad, escalamiento).
- **T4** (S1,S2) · route docs · `docs/gateway-contract.md` — **DONE** (pendiente commit): §8 "Conversational policy — receptionist skill" + fila en docs/architecture/README.md.

## Log

- **L1** = S1 verbatim (solicitud original del owner).
- **L2** 2026-10-16 — Owner elige "ADR + spec ODD ahora" y tono "Cordial y orientador" (ver S2/S3). Scope: doc-first; la implementación de install.sh/TUI queda como change de apply posterior.
- **L3** 2026-10-16 — Reconciliación documentada como supuesto en T2: se asume que Hermes consume skills desde `~/.hermes/skills/<name>/SKILL.md` (análogo a `~/.hermes/config.yaml` del TUI opción 7); a verificar contra el runtime de Hermes en apply.
- **L4** 2026-10-16 — T4 ampliado: además del §8 del gateway contract, ADR-0020 se indexa en `docs/architecture/README.md` (convención de hermanos ADR-0018/0019).
- **L5** 2026-10-16 — Readback estructural completo: sin tokens corruptos, S1-S3 cubiertos, sin claims de cambios Go; 3 preguntas abiertas para el owner quedaron en tasks.md del change (nombre del skill dir, label de la opción TUI — nueva opción vs segundo paso dentro de la 7, ejemplo canónico del off-topic).
- **L7** 2026-10-16 — Owner round 1 (verbatim): "1. Verifica en la doc de hermes: https://hermes-agent.nousresearch.com/ en qué directorio debemos poner oficialmente la Skill y respetemos el estandar. 2. segundo paso dentro de la opción 7 \"Configurar Hermes\". 3. Está bien \"Cordial y orientador\"... Ajusta estos puntos primero y luego te digo como continuar".
- **L8** 2026-10-16 — Doc oficial verificada (`/docs/user-guide/features/skills`): storage `~/.hermes/skills/<slug>/SKILL.md` ES el estándar oficial; template reescrito al estándar (frontmatter name/description/version, secciones When to Use → Procedure → Pitfalls → Verification, description ≤60 chars); slug `recepcionista-negocio`. Hallazgo nuevo incorporado al diseño: progressive disclosure (índice name+description siempre; texto completo on demand) → la skill se redacta como aplicable a toda conversación del canal; índice refresca al próximo arranque/compaction; TUI avisa ese detalle tras regenerar. Ajustes aplicados en los 5 artefactos (ADR-0020, proposal, design, spec, gateway §8) y preguntas abiertas resueltas en tasks.md del change.
- **L9** 2026-10-16 — Commit slice registrado (docs lane): C1 `5912e18` docs(adr) — odd/tasks + ADR-0020 + índice README (164 líneas); C2 `0c1f888` docs(openspec) — change folder + gateway-contract §8 (463 líneas). Total 627 líneas en 2 commits > cap 400: slice boundary documentada en el reporte GGA; no es un PR de código (docs lane). GGA corrió sobre ambos commits: sin hallazgos (solo .md; los patterns de GGA miden Go).
- **L10** 2026-10-16 — Pendiente: merge ff-only a main + push (decisión de publishing del owner) → luego el change de apply (template + install.sh + TUI opción 7 step 2 + smoke manual en el host).
