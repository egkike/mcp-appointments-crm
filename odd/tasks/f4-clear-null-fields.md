# Feature: F-4 — clear-null fields in `update_business_profile` (lat/long)

- **Created**: 2026-09-30
- **Status**: implemented + verified — pending T7 (commit decision del owner + gate)
- **Backlog refs**: PRD §7 Fase N / §3.8.9 location contract (F-4 registered as WhatsApp-bot prerequisite, commit 55d3c42); `docs/demo-plan.md` smoke v0.6.1 hallazgos; scout map task `muofg6sx-1-5kc0` (gentle-ai-explore, COMPLETE).
- **Route**: delegated ODD (scout → worker → verify → native review). No SDD selected.

## Goal

Un pin cargado por error queda atrapado: `update_business_profile` hoy solo sobreescribe. Con go-sdk v1.7.0 los args llegan como struct tipado decodificado, y el campo puntero (`*float64`) colapsa `ausente` y `null` explícito al mismo `nil`, por lo que `{"latitude": null}` terminaba en «no se proporcionaron campos para actualizar».

**Comportamiento implementado**: `null` explícito en `latitude`/`longitude` limpia el campo (escribe `SQL NULL`); key ausente conserva la semántica vigente «dejar el valor almacenado»; payload null-only es un update válido (deja de disparar el guard de payload vacío). Alcance limitado a esos dos campos de ubicación: ningún otro campo pasa a ser clearable.

## Out of scope

- `maps_url` derivado en `get_business_profile` (consumidor: change del bot WhatsApp). ADR-0018 no lo menciona.
- Auto-registro de desconocidos vía `get_or_create_client`.
- `mcp-server hermes chat` (stub vigente).
- Cambios de repositorio o de schema (el `UPDATE` full-row preparado ya persiste `NULL`; lat/long son `REAL` nullables sin CHECK).

## Contracts (del scout, verificados a la hora de editar)

- Args llegan por `internaljson.Unmarshal` del SDK tras validar contra el schema inferido; la distinción presencia/clear se captura en Go, no en el schema.
- `UnmarshalJSON` custom con tipo alias anti-recursión + `map[string]json.RawMessage` de presencia; marcadores unexported → wire schema sin cambios (pinned por test).
- Mensajes semánticos user-facing en español; code comments en inglés; sin DI/decoradores; prepared statements.

## Tasks

- [x] **T1** — DTO + usecase: flags `ClearLatitude`/`ClearLongitude` con doc comment del contrato; `hasProfileUpdates` cuenta los flags (payload null-only válido, mensaje español intacto para payload genuinamente vacío); `applyProfileReferenceUpdates` chequea clear-primero y setea `nil`. Test: `TestUpdateBusinessProfileUseCase_ClearLocation` (5 casos: clear both/lat/long, payload sin flags sigue rechazando, nil sin flags keep-stored). Table partial-merge existente intacta y verde.
- [x] **T2** — MCP handler: `UnmarshalJSON` custom (`updateBusinessProfileInAlias` + marcadores `latitudeProvided`/`longitudeProvided`); mapper setea flags en provided&&nil. Tests: `TestToolUpdateBusinessProfileNullLocationClears` (5 casos) y `TestToolUpdateBusinessProfilePresenceMarkersStayOutOfSchema` (probe `additionalProperties:false` — los marcadores nunca se filtran al wire schema).
- [x] **T3** — Integración mux (`TestIntegrationMaintenanceBusinessProfileClearLocation`): seed SQL con coords → omit-key update preserva → `null` limpia → `get_business_profile` sin lat/long → columnas `SQL NULL` (`sql.NullFloat64`) → repeat null aceptado → `{}` sigue rechazando con `-32002` y el mensaje español exacto.
- [x] **T4** — Verificación writer: `go build -o /dev/null ./...`: OK; `go test -race -count=1 ./internal/mcp/ ./internal/application/...`: ok (mcp 9.552s, dto 1.026s, usecase 1.027s); `gofmt -l .`: vacío (1 fix de formato aplicado durante la corrida).
- [x] **T5** — Docs parent: PRD §3.8.9 bullet F-4 → «RESUELTO (2026-09-30)»; changelog row 1.20; header PRD version 1.18 residual → 1.20. demo-plan no tiene refs a F-4 (nada que disponer ahí). `openspec/specs/mcp-transport/spec.md` fila `update_business_profile` NO editada: es no-contradictoria (no niega el clear); la formalización de la semántica null en specs canónicas queda para el change del bot (OpenSpec discipline).
- [x] **T6** — Pipeline pre-commit: `gofmt -l .` vacío; `go vet ./...` ok; `go build -o /dev/null ./...` ok; `golangci-lint run ./...` → `0 issues`; `go test -race -count=1 ./...` → sin fallas (15 packages ok); spot check del parent: re-run del comando focused del writer → ok (mcp 8.598s, dto 1.014s, usecase 1.020s).
- [ ] **T7** — Gate: commit del work-unit en `feat/f4-clear-null` (decisión del owner) + review nativo (RDD on, routing default) + PR a main.

## Acceptance cartilla (estado)

- `{"latitude":null}` limpia lat ✅ (tests handler/usecase/integración)
- `{"latitude":null,"longitude":null}` limpia ambas ✅
- Payload null-only no dispara «no se proporcionaron campos para actualizar» ✅ (usecase, mensaje español exacto intacto para `{}`)
- Key ausente = keep stored ✅ (handler existing test intacto + integración omit-key)
- Marcadores de presencia no fugan al wire schema ✅ (probe `additionalProperties:false`)

## Routes + evidence

| Work | Trigger | Route |
|---|---|---|
| Mapa F-4 | 4+ files | delegated — `gentle-ai-explore` (task `muofg6sx-1-5kc0`, COMPLETE) |
| T1–T3 implementación | write 2+ non-trivial files | delegated — `gentle-ai-worker` (task `muofnym9-2-5lan`, COMPLETE) |
| T4 spot check + T6 pipeline | verification | parent inline ( RDD on: writer report = verification of record; re-run conforme) |
| T7 gate | RDD on | native review sobre el work-unit commit |

## Notas de scope (aceptadas/compradas)

1. **Cláusula de discovery en la descripción del tool (ES)** — añadida por el writer fuera del design delegado (1 línea): «Enviar latitude o longitude como null borra la coordenada almacenada; omitir la clave la deja como está.». **ACEPTADA por el parent**: sin ella la capability es invisible para Hermes (contrato de ubicación pedía exactamente esa corrección por chat); string user-facing consistente con las descripciones existentes; revertible con revert de la línea sola si el owner la decline.
2. **Diffstat 331 LOC** (326+5): ~80 sobre el forecast 150-250, dentro del budget de slice única (<400). Motivo: probes extra en tests (fuga de schema, `{}` guard, repeat-clear) — valor real de regresión, no padding.

## Delivery strategy

`ask-on-risk`(default), slice única, estrategia `single-pr` (branch `feat/f4-clear-null` → PR a main). **TDD**: off (source: AGENTS.md — tests obligatorios, RED-first solo SDD; runner `go test -race ./...`).

## Next step

T7 sobre el work-unit commit; luego push + PR (requiere decisión del owner).
