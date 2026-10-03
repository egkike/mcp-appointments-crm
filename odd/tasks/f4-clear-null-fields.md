# Feature: F-4 — clear-null fields in `update_business_profile` (lat/long)

- **Created**: 2026-09-30
- **Status**: COMPLETE — DELIVERED y MERGED (squash `3b86db9`, 2026-09-30, PR #105; issue #106 auto-closed). Siguientes: change `feat-whatsapp-bot` (spec SDD, ADR-0018) y `scripts/uninstall.sh`.
- **Backlog refs**: PRD §7 Fase N / §3.8.9 (commit 55d3c42); demo-plan smoke v0.6.1; scout task `muofg6sx-1-5kc0`; issue de tracking **#106** (creado 2026-09-30, linkeado con `Closes #106` desde el PR #105).

## Goal

`null` explícito en `latitude`/`longitude` de `update_business_profile` limpia la coordenada (SQL NULL); key ausente = keep stored; payload null-only es update válido. Alcance limitado a lat/long.

## Out of scope

`maps_url` (consumidor: change del bot), auto-registro de desconocidos vía `get_or_create_client`, `mcp-server hermes chat`, cambios repo/schema.

## Implementación (T1–T3 COMPLETE — writer task `muofnym9-2-5lan`)

- `internal/mcp/tools_maintenance.go`: `UnmarshalJSON` custom (alias anti-recursión + marcadores unexported `latitudeProvided`/`longitudeProvided`; wire schema sin cambios, probe-pinned) + mapper provided&&nil → flags. Cláusula de discovery ES en la descripción del tool (aceptada por el parent: sin ella la capability es invisible para Hermes; revertible con revert de 1 línea).
- `internal/application/dto/maintenance.go`: flags `ClearLatitude`/`ClearLongitude` (`json:"-"`).
- `internal/application/usecase/update_business_profile.go`: guard cuenta las señales de clear (payload null-only válido; mensaje español exacto intacto para `{}`); merge clear-primero → entidad `nil`.
- Tests 3 capas: `TestToolUpdateBusinessProfileNullLocationClears`, `TestToolUpdateBusinessProfilePresenceMarkersStayOutOfSchema`, `TestUpdateBusinessProfileUseCase_ClearLocation`, `TestIntegrationMaintenanceBusinessProfileClearLocation` (SQL NULL verificado con `sql.NullFloat64`; `{}` → `-32002`).

## Verificación (T4–T6 COMPLETE)

- Writer: `go build` OK; `go test -race` focused ok; `gofmt -l` vacío.
- Parent pipeline: `gofmt`/`go vet`/`golangci-lint 0 issues`/`go build` ok; `go test -race -count=1 ./...` 15/15 packages ok; spot check (re-run focused del writer) ok.
- Verificador independiente (task `muog7idw-3-qn2a`): 8/8 comandos exit 0, árbol limpio antes/después, cartilla confirmada, cero cambios repo/schema, observación no-bloqueante (doble decode del payload — costo despreciable).

## Gate (T7)

- Assessment nativo: **risk high** (hot path update en `update_business_profile.go`); 401 lines, correction budget 200; outcome nativo `unknown`.
- Review nativo: lineage `review-aa28c86d3b625bed` (tier high, lentes risk/resilience/readability/reliability) queda **reviewing sin cerrar** — el relay del reviewer produjo 3 veces `reviewer-empty-output (stopReason: length)` para `review-risk` (~173/188/200 s; 0 prepared/0 submitted; sin mutación; la STATUS reofrece el mismo set con revision/generation idéntico).
- **Defecto upstream reportado**: plasma en Gentleman-Programming/gentle-ai#3991 (comentario confirmado `#issuecomment-5917681979`, consentimiento report_and_continue del owner; lookup open+closed: equivalente causal identificado, sin fix publicado — v3.7.0 es el release más nuevo y la build instalada). Sin labels tocados.
- Fallback risk-gated aplicado y satisfecho (assess con `nativeReviewOutcome: unavailable`, `outcome_source: explicit`): self-verificación del writer + verificador independiente + spot check del parent.
- Work-unit commits: `684e9c5` (feat mcp, 326+/5-) y `e6d6e3c` (docs prd, 68+/2-); GGA PASSED en ambos. Incidente intermedio diagnosticado y resuelto: cache-tree del índice corrupto por gc concurrente durante los primeros intentos de commit (fix: `git reset` + rebuild; fsck limpio tras).
- Entrega: **PR #105 MERGED** (squash **`3b86db9`** en `main`, 2026-09-30T19:04:35Z) — CI pass; issue #106 auto-closed. Branch `feat/f4-clear-null` borrada (local y remota).
- openspec/specs/mcp-transport fila `update_business_profile`: NO editada (no-contradictoria); formalización de la semántica null en specs canónicas deferida al change del bot.

## Notas

- Diffstat 331 LOC (326+5) vs forecast 150–250: probes de tests extra; dentro del budget de slice única (<400). Delivery: ask-on-risk, single-pr, branch `feat/f4-clear-null`. TDD off (source AGENTS.md).
- Updated docs held en el work unit (comitados en `e6d6e3c`); los updates del tracking T7 de hoy quedan sin commit hasta decisión del owner (placeholder: se push-an por docs lane después o van al merge).

## Next step — decisión del owner (2026-09-30, cierre)

**RDD: esperar fix upstream antes de seguir con features.** El owner decidió NO aplicar mitigaciones locales (thinking low en las lentes descartado como remedio parcial: no arregla la mitad truncation de #4937) y esperar que gentle-ai publique release con el fix del relay reviewer (vigilar #3991 + #4937 + releases > v3.7.0; ambos issues abiertos, #3991 triageado priority:high con root cause confirmado por terceros: reasoning tokens consumen el budget de output de 16k → contenido vacío con effort high; y el modo truncation del transporte por separado).

- Lineage `review-aa28c86d3b625bed`: queda `reviewing` intacto (sin mutaciones). Al haber release con fix: retry del collect (fresh STATUS → slots en orden); si el relay sigue roto o el owner lo prefiere, abandon con envelope nativo.
- Modo de verificación operativo mientras tanto: fallback a prueba de fallo (pipeline completo + writer self-verify + verificador independiente + GGA) — validado end-to-end en esta feature.
- Después del fix: change `feat-whatsapp-bot` (spec SDD con ADR-0018) — desbloqueado por F-4; `scripts/uninstall.sh` sigue en el backlog.

## Addendum — Verificación RDD 2026-10-03 (post-migración v4.0.0)

Stack local: gentle-ai v4.0.0 (go module v4, vcs `ff77164d`; migración cerrada 2026-10-02), gentle-pi 4.0.0, pi 1.0.0 (npm).

- **Upstream al 03-10**: #3991 y #4937 siguen **OPEN**, sin respuesta del equipo desde el 30-09 y sin fix PR. Release v4.0.0 (2026-10-01, "SDD Retires, ODD Leads") no menciona fix del transporte `pi_host_relay` (el trabajo de relay de la nota es OpenCode V2, otra pata).
- **Canary de verificación en 4.0.0** (consentimiento owner, opción "retry empírico slot 0"): STATUS fresco del lineage `review-aa28c86d3b625bed` confirma que sobrevivió a la migración intacto (`reviewing`, revision `sha256:e4c4824f...`) y reofrece los 4 slots (risk 0, resilience 1, readability 2, reliability 3) con las mismas firmas. Canary único del slot 0 (`review-risk`, order 0, forecast de 1 corrida reconocida): **`pi-host-relay-transport-failure` / `reviewer-empty-output`, `stopReason: "length"`, 172055 ms** (timeout del envelope 942355 ms, `timed_out: false`) — clase EXACTA a la de 3.7.0 y en el mismo perfil de timing (173/188/200 s), lejos del timeout: budget starvation, no un timeout. `mutation_performed: false`; el lineage sigue reviewing con los 4 slots reofrecidos. Costo: 1 corrida de reviewer.
- **Comentario upstream** (consentimiento owner): #3991 (issuecomment-5970978258, 2026-10-03) reporta el repro en 4.0.0 con protocolo + envelope; autor egkike. #4937 intocado (la clase truncation ya está reportada ahí). Post-sync 4.0.0, la lente sigue routando `command-code/z-ai/glm-5.3-flash` con `thinking: high`.
- **Estado posterior**: sin fix publicado; **gate operativo = fallback a prueba de fallo** (writer self-verify + verificador independiente + GGA). Mitigación thinking low sigue descartada (no arregla la clase truncation de #4937). Vigilancia: releases > v4.0.0 y respuesta en #3991; al haber fix, retry del collect (STATUS fresco → slots en orden).
- **Próximo (owner decide)**: arrancar spec `feat-whatsapp-bot` (ODD, consumidor de ADR-0018) o seguir en standby; `scripts/uninstall.sh` ya está en el backlog.
