# Feature: hygiene-followups-3 — limpieza de los 5 advisory SUGGESTION del gate de hygiene-2

**Status**: IN PROGRESS
**Started**: 2026-09-24
**Branch**: TBD (desde main @ b046c5f)
**Origin**: los 5 hallazgos advisory (SUGGESTION, no bloqueantes) del gate nativo
`review-7b524be75481095e` (APPROVED, receipt ddfaba23 quemado) que aprobó PR #102.
Textos no persistidos (el relay no almacena) — re-derivados desde las ubicaciones
exactas del closure.

## Routing (owner, 2026-09-24)

Trivial → **readback estructural** (NO 4-lens: los 5 son SUGGESTION de un review
aprobado hace minutos; otro gate high sería inflación de carga de review).
Presupuesto: ~30-60 líneas. Un solo PR issue-first.

## Tasks

- [x] **T1 — Re-derivación + fixes (worker)**: leer las 5 regiones, fijar la premisa
      de cada advisory, implementar el fix mecánico o la disposition documentada.
- [ ] **T2 — Verificación del orchestrator**: pipeline completo + revisión de diff.
- [ ] **T3 — Commit + readback estructural** (owner aprueba commit).
- [ ] **T4 — Issue-first PR + merge (owner)**.

## Superficies

`internal/loopback/loopback.go`, `internal/mcp/loopback.go`,
`internal/admin/presentation.go`, `internal/mcp/server_test.go`,
`internal/admin/presentation_test.go` (+ tests propios si corresponde).

## Candidate list (re-derivación inicial, a confirmar contra el código exacto)

| # | Ubicación | Lectura | Disposición propuesta |
|---|-----------|---------|----------------------|
| 1 | internal/admin/presentation.go:36-52 (R2-applyhermes-placement) | colocación de ApplyHermesConfig: ¿presentation.go o hermes.go? | decisión de home documentada en el doc-comment |
| 2 | internal/mcp/loopback.go:33-42 (R2-loopback-fallthrough) | fallthrough 0.0.0.0/:: poco legible | fix: sub-mapeo explícito |
| 3 | internal/loopback/loopback.go:33-44 (R2-loopback-ip-unused) | net.IP retornado por Classify sin consumir | fix: simplificar firma a Reason |
| 4 | internal/mcp/server_test.go:218-225 (R3-001) | naming/cobertura del test de chain | fix menor o disposition |
| 5 | internal/admin/presentation_test.go:174 (R3-002) | naming/cobertura del test de ApplyHermesConfig | fix menor o disposition |

## Evidence

### T1 — worker (2-hy3 re-derivation) + completado inline del orchestrator

7 archivos, +48/−29 neto. Pipeline completo verde: fmt/vet/golangci 0/build/test -race
15/15 paquetes.

- **#1 R2-applyhermes-placement (fix-documentado)**: doc-comment de
  `ApplyHermesConfig` ahora fija la decisión de home (presentation.go es el seam
  compartido por los dos entry points; los pasos compuestos viven en hermes.go como
  core framework-free; ADR-0017 Decision 1). Sin move.
- **#2 R2-loopback-fallthrough (fix)**: eliminados `fallthrough`/`default`; el mensaje
  genérico de no-loopback es un return único final (comparte `::`-Unspecified y
  NotLoopback). Mensajes verbatim, accept/reject idénticos.
- **#3 R2-loopback-ip-unused (fix, completado inline)**: `Classify` ahora retorna solo
  `Reason` (el `net.IP` no era consumido por ningún caller); callers en hermes.go:194 y
  mcp/loopback.go:25 actualizados (una línea cada uno); test del clasificador simplificado
  (las aserciones sobre el IP retornado eran de superficie, no de política).
- **#4 R3-001 (fix)**: `TestServerHandlerChainIsStable` ancló la comparación en un
  initialize genuino (decodeRPCEnvelope + serverInfo.version == testServerVersion) antes
  del byte-equality — dos cuerpos de error idénticos ya no satisfacen el test.
- **#5 R3-002 (fix)**: `TestApplyHermesConfigWritesTheMergedEntry` aserta el url de la
  entrada mergeada (hermesEntryFrom), no solo que la sección exista.
- Conflicto de superficie del worker resuelto: hermes.go:194 (1 línea) lo editó el
  orchestrator inline (incluido en la superficie derivada del plan aprobado).

(por completar task a task)
