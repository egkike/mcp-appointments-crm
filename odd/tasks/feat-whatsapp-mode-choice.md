# Feature: WhatsApp mode choice (modos A/B del canal del negocio)

**Type**: docs (contract + runbook) — sin Go
**Status**: doc-first landed 2026-11-01 (change `feat-whatsapp-mode-choice`); delivery pendiente
**Origin**: owner 2026-11-01: "se debería poder utilizar cualquiera de las 2 opciones
(a elección del usuario)... usuarios que quieran utilizar su propio nro para el negocio
y otros que tengan un nro dedicado a su negocio. Podemos hacerlo así?" → respuesta técnica:
la arquitectura ya lo soporta (persona por rol resuelto, escenarios A/B del contrato);
el gap es doc-first de runbook+contrato.

## Specs

- **S1 (runbook por modo)**: §7.5 con rama por `WHATSAPP_MODE` (`bot`/`self-chat`,
  E1) con pasos y post-condiciones de cada modo, reuso del protocolo §7.1/§7.2.
- **S2 (contrato de modos)**: matriz D1 + semántica de `messenger_id` (D5) +
  checklist de aceptación por modo (C-común/C-modA/C-modB/C-gate) extendiendo §9.
- **S3 (gap wizard)**: la oferta del modo en el setup de este repo queda REGISTRADA
  (superficie propuesta D6), decisión owner pendiente, no se implementa acá.

## Tasks

- **T1** (S1-S3) · route docs · doc-first (este change) — **done 2026-11-01**.
- **T2** (S1+S2) · route docs · delivery: un PR docs con runbook por modo +
  matriz/criterios en el contrato + PRD changelog.
- **T3** (S3) · bloqueado por decisión owner (wizard o no; si sí, change de code lane aparte).

## Log

- **L1** 2026-10-31 — smoke modo A end-to-end en el VM (backlog, reporte del
  onboarding): self-chat owner, S7a chain MCP, bypass execute_code documentado
  (skill v1.1.1 tools-only). Base de evidencia de C-modA.
- **L2** 2026-11-01 — owner aprueba operar los DOS modos como elección de
  usuario (cita verbatim arriba). Decision técnica: cero Go; doc-first
  `feat-whatsapp-mode-choice` (proposal/design/spec/tasks landed, E1-E8).
- **L3** 2026-11-01 — enmienda de redacción: sin wizard S3 ninguno del modo se
  puede elegir hoy desde el TUI de este repo (la elección vive en el wizard
  runtime `hermes whatsapp`); el change lo registra bajo decisión owner (T3).
