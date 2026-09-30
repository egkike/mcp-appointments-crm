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
