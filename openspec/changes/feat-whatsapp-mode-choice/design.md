# Design: feat-whatsapp-mode-choice

## D1 — Matriz de modos (forma del contrato)

Un solo modelo de negocio, dos config de envoltura:

| Dimensión | Modo B — número dedicado (escenario A) | Modo A — número propio (escenario B, self-chat) |
|---|---|---|
| Número que recibe mensajes | número del bot (WhatsApp Business en un segundo dispositivo/SIM) | número personal del owner |
| `messenger_id` | número del bot | número personal del owner (verificado 2026-11-01) |
| Emisor owner | los `accounts.role=owner` escriben al bot desde su teléfono | el owner se auto-escribe (JID de self-chat); se resuelve por `accounts` → owner |
| Usuarios sender | clientes/owner/staff con su teléfono real | mismo mecanismo; cualquier conocedor del número entra al funnel |
| Desconocidos | registration loop (§3) | registration loop (§3), con rate limit por teléfono |
| Riesgo operador | setup de SIM/Business extra; separación total de vidas | el número personal queda como cara del negocio; el owner depende de su propia sesión del bridge |
| `WHATSAPP_MODE` | `bot` | `self-chat` |

La elección es **pre-onboarding y por instalación** (columna `WHATSAPP_MODE`,
E1): no es un flag en caliente ni un cambio de runtime.

## D2 — Invariante de identidad (no se toca)

R2 del §8.1 (`gateway-contract.md:201`): la persona sale del rol resuelto del
emisor por canal, no de qué número es el bot. Consecuencia de diseño: **ambos
modos comparten el 100% del server, la skill y la cascada** (E7). El change no
agrega branching de negocio; solo fija la semántica de `messenger_id` y los
pasos de operador.

## D3 — Runbook §7.5 con rama por modo (S1)

Cada modo recibe su tabla de pasos con pre→paso→post-condición, reusando el
protocolo §7.1/§7.2 (sin redeclararlo):

- Común: transfer ownership (si aplica) → `update_business_profile`
  (`messenger_platform=whatsapp`) → verify `get_business_profile`.
- **Modo B específico**: disponibilidad del número dedicado (SIM/Business),
  `hermes whatsapp` opción 1, pairing en el dispositivo del bot,
  `messenger_id` = número del bot, prueba de funnels con dos emisores reales
  (owner + cliente desconocido).
- **Modo A específico**: `hermes whatsapp` opción 2, pairing con el teléfono
  personal, advertencia de privacidad explícita (la cuenta personal queda como
  cara del negocio y el self-chat es el lane del agent),
  `messenger_id` = número personal.

Criterios de aceptación por modo (C1..C5) van en la spec; el runbook los
referencia (no los duplica).

## D4 — Checklist de aceptación por modo (S2)

- **C-común**: `get_business_profile` lee `whatsapp` + `messenger_id` correcto;
  skill instalada/actualizada y con tabla de selección vigente.
- **C-modA**: saludo por self-chat; probe de agenda → conducta owner; el
  emisor `accounts.owner` resuelve dueño (evidencia del smoke 2026-11-01).
- **C-modB**: mensaje desde el owner phone → persona owner; mensaje desde un
  desconocido → registro (§3) y luego persona cliente; (opcional) un sans
  staff/control de contrato de staff → cliente fail-closed (R3).
- **C-gate del desvío**: los mismos G0-G4 y exclusiones TUI-only valen en
  ambos modos (no dependen del modo).

## D5 — Semántica de `messenger_id` (página de dispute)

Texto normativo corto en el contrato: `messenger_id` **identifica el número
que recibe los mensajes del canal elegido** (E1 del grupo de contrato); en modo
A es el número del owner **por decisión de instalación**, no por inferencia del
server — el server no distingue la diferencia (D2).

## D6 — S3: superficie wizard (registrada, no implementada)

La oferta del modo en el setup de este repo sería un step nuevo en la opción 7
(Go en `internal/tui`/`cmd`) que delega el pairing a `hermes whatsapp` con el
modo pre-elegido. Decisión explícita del owner requerida antes de cualquier
apply (supficie tocada: `cmd/mcp-server/admin_tui.go`, `internal/tui/*`,
`internal/admin/hermes.go`). Este change solo la registra con su criterio de
entrada y evidencia del smoke (E8).

## Riesgos / rollback

- Riesgo principal: documental (un operador sigue el runbook del modo equivocado
  y publica `messenger_id` mal). Mitigación: checklist por modo C-común como
  primer paso de verificación.
- Rollback: revert del PR de delivery (docs); ningún artefacto runtime cambia.

## Evidence Inventory

E1..E8 idénticos a proposal.md (mismo estado de repo al doc-first).
