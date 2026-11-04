# Proposal: feat-whatsapp-mode-choice

## Resumen

Hacer explícito, de primera clase, que el canal del negocio (WhatsApp, ADR-0018)
se puede operar en **dos modos a elección del usuario/operador**, con el mismo
modelo de negocio y el mismo código del server:

- **Modo B — número dedicado al bot** (contrato escenario A de
  `docs/gateway-contract.md`): un número con WhatsApp Business (o WhatsApp
  normal en un segundo dispositivo) recibe los mensajes de clientes, owner y
  staff. `messenger_id` = número del bot.
- **Modo A — número propio del owner (self-chat)** (contrato escenario B):
  la cuenta personal del owner ES el bot; el owner se escribe a sí mismo para
  hablar con el agent y los clientes que tengan su número lo usan igual.
  `messenger_id` = el número del owner (verificado en vivo 2026-11-01).

La elección **ya existe en el runtime** (`hermes whatsapp` pregunta el modo:
`WHATSAPP_MODE=bot|self-chat` — `_whatsapp_choose_mode`,
`hermes_cli/main_platform_setup.py:28-63`) y **el modelo de negocio ya es
agnóstico de la distinción**: la persona sale del rol resuelto del emisor (R2
de la skill v1.1.x / `gateway-contract.md:201`), no del número del bot. El gap
es de **documentación operatoria y contrato**, no de código del server.

## Problema

Hoy el runbook go-live (§7.5) y el contrato describen el flujo como si fuera
uno solo: transfer ownership + `messenger_platform=whatsapp` + `messenger_id`.
No dice: ¿el `messenger_id` es el número del bot o el del owner? ¿Qué cambia
según el modo? El smoke del 2026-11-01 (backlog, reporte del onboarding) corrió
solo el modo A; un usuario nuevo con número dedicado no tiene pasos ni criterios
de aceptación propios. Riesgo: operador elige un modo "por accidente", la skill
funciona igual (fail-closed) pero la configuración pública del negocio
(`messenger_id` publicado a clientes) queda mal.

## Outcome

Un usuario/operador elige modo **antes del onboarding**, la guía le da los pasos
exactos de su modo y los criterios de verificación por modo; los dos modos
quedan formales en el contrato. Cero cambios de Go, cero servicios nuevos.

## Scope (S1-S3)

- **S1 — Runbook §7.5 con rama por modo**: secuencia condicional por
  `WHATSAPP_MODE`, con los pasos y post-condiciones de cada uno.
- **S2 — Contrato: sección de modos** en `docs/gateway-contract.md`: matriz
  Modo A / Modo B, semántica de `messenger_id`, funnel de registro por modo y
  checklist de aceptación (C1..Cn) por modo.
- **S3 — Registro del gap de oferta en setup** (no se implementa acá): el
  wizard de la opción 7 / `install.sh` de este repo no incluye el paso WhatsApp
  (gap detectado 2026-11-01 en el smoke) — se registra como superficie futura
  con su decisión owner separada (es código Go en `internal/tui`/`cmd`).

## Non-goals

- Ningún cambio en `internal/**`, `cmd/**` (el server ya soporta ambos modos).
- No se cambia la skill ni su `version` (R1-R5 ya son canal/rol-resolved).
- No se decide aquí cuándo entra el wizard del S3: queda registrado con su
  criterio (ver tasks).

## Constraints / fences

- Todo lo que toque `docs/PRD.md` o changelog se hace en el PR de delivery,
  no en el doc-first.
- El smoke real de modo B (número dedicado) queda **fuera de alcance**: no hay
  número dedicado disponible; la verificación de S1/S2 es readback estructural
  + consistencia con la evidencia del modo A ya corrido (2026-11-01).

## Evidence Inventory

- E1: `hermes_cli/main_platform_setup.py:28-63` — `_whatsapp_choose_mode` escribe
  `WHATSAPP_MODE` = `bot` | `self-chat` (elegido por el usuario, opción 1/2).
- E2: `hermes_cli/main_platform_setup.py:133-210` — `cmd_whatsapp` guía bridge+pairing en ambos modos (un solo path; el modo es contexto de la guía, no una rama del código).
- E3: `docs/gateway-contract.md:75` — tabla §4 con **escenario B** (owner cuyo
  número es el número del negocio, self-chat → resuelve owner).
- E4: `docs/gateway-contract.md:201` (R2) — persona por **rol resuelto**, no por
  número del bot; `:230` (C5) proyección bot-side yaPublicado.
- E5: `docs/PRD.md:209` — schema `messenger_platform` CHECK (`whatsapp|telegram|NULL`).
- E6: smoke 2026-11-01 (backlog, reporte del onboarding) — modo A corrido
  end-to-end: `messenger_id` = `+5492617132740` con self-chat; persona owner
  activa; S7a chain MCP; fail-closed humano registrando un bypass (v1.1.1).
- E7: `setup/templates/recepcionista-negocio/SKILL.md:26-36` — la tabla de
  selección de persona es por canal + rol resuelto; invariante que este change
  NO toca.
- E8: gap del wizard (S3): reporte del backlog 2026-11-01 — la opción 7 escribe
  solo server-entry + Telegram (S11); no hay paso WhatsApp en ningún wizard.
