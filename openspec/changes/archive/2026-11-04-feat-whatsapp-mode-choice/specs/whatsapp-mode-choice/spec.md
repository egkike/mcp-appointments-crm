# Spec: whatsapp-mode-choice

## Purpose

Fijar que el canal WhatsApp del negocio admite dos modos de instalación a
elección del operador — número dedicado al bot (escenario A) o número propio
del owner con self-chat (escenario B) — con los mismos tools, la misma skill y
el mismo modelo de negocio, y dar los pasos y criterios de aceptación de cada
modo. Invariante central: la persona sale del rol resuelto del emisor (R2,
`gateway-contract.md:201`), no del modo.

## Requirements

### Requirement: El runbook go-live guía el modo elegido del operador

**GIVEN** un operador que eligió un modo (`WHATSAPP_MODE` = `bot` o
`self-chat`, E1)
**WHEN** sigue el runbook go-live de `docs/gateway-contract.md` §7.5
**THEN** ejecuta la rama correspondiente a su modo con pasos y post-condiciones
propios, sin pasos del otro modo, y deja un `messenger_id` semánticamente
correcto (D5) verificado con `get_business_profile`.

#### Scenario: modo B — número dedicado

WHEN el modo elegido es `bot` (escenario A)
THEN el runbook indica disponibilidad del número dedicado en un
dispositivo/SIM, `hermes whatsapp` opción 1, pairing en el dispositivo del bot,
`messenger_id` = número del bot, y las verificaciones C-común + C-modB.

#### Scenario: modo A — número propio del owner (self-chat)

WHEN el modo elegido es `self-chat` (escenario B)
THEN el runbook indica `hermes whatsapp` opción 2, pairing con el teléfono
personal, advertencia de privacidad, `messenger_id` = número personal, y las
verificaciones C-común + C-modA.

### Requirement: El contrato documenta la matriz de modos

**GIVEN** la tabla §4 del gateway-contract (escenarios A/B, E3) y la matriz D1
de este change
**WHEN** publica la sección de modos
**THEN** cada modo declara: número receptor, semántica de `messenger_id` (D5),
funnel de desconocidos (§3) y criterios de aceptación (C-común, C-modA,
C-modB, C-gate), quedando el checklist §9 extendido por modo.

#### Scenario: idempotencia de la persona entre modos

WHEN un mismo `accounts.role=owner` escribe al canal en modo A o en modo B
THEN la persona y procedimientos aplicados son idénticos (R1-R5 vigentes, E7);
ningún comportamiento conversacional depende de la diferencia del modo.

### Requirement: Los G-gates y exclusiones son invariantes al modo

**GIVEN** el doble gate G0-G4 de deletes y las exclusiones TUI-only
(purge/deactivate/transfer/audit)
**WHEN** el canal corre en modo A o modo B
THEN los gates y exclusiones aplican sin variación; la única diferencia entre
modos es operatoria (quién recibe, quién aparece publicado como receptor).

### Requirement: La superficie de oferta en setup queda registrada (S3)

**GIVEN** que la opción 7 de este repo no incluye paso WhatsApp (E8)
**WHEN** este change documenta el gap
**THEN** registra la superficie futura (files: `cmd/mcp-server/admin_tui.go`,
`internal/tui/*`, `internal/admin/hermes.go`), su criterio de entrada (decisión
owner explícita sobre la oferta del modo en setup) y explícitamente NO la
implementa (non-goal vigente mientras no haya decisión).
