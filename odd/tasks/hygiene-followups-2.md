# Feature: hygiene-followups-2 — disposition de los 8 hallazgos informativos del gate de hygiene

**Status**: COMPLETE (merged)
**Started**: 2026-09-25
**Branch**: TBD (desde main @ 3929610)
**Origin**: los 8 hallazgos informativos (WARNING/SUGGESTION, no bloqueantes) del review
nativo `review-bf9ad65843a50df7` (high tier, 4/4 lenses, 12 archivos, 771 líneas) que
aprobó PR #100. El contenido textual nunca se persistió — solo IDs (R2-01..04,
R3-001..003, R4-001). Este feature los re-deriva, los dispositiona y fixea lo que el
owner apruebe, ANTES del feature grande de WhatsApp por-sender (baseline limpio para
la región de identidad/auth/X-Caller-Id).

## Decisión de arranque (owner, 2026-09-25)

Sí a hygiene-2 como primer paso del backlog. Formato: feature compacto único, todos los
findings en un solo PR (~100-200 líneas estimadas). Backlog obs 914 queda: (5) smoke VM
después de esto, (3)+(4) cuando llegue el teléfono real.

## Mapeo de lens (confirmado contra agentes del runtime)

| Lens | Findings | Severidad |
|------|----------|-----------|
| R1 Risk | 0 | — |
| R2 Readability | R2-01..04 (4) | WARNING/SUGGESTION |
| R3 Reliability | R3-001..003 (3) | WARNING/SUGGESTION |
| R4 Resilience | R4-001 (1) | WARNING/SUGGESTION |

Superficie del candidato (diff de PR #100, commit 5694316):
`cmd/mcp-server/{main,admin_tui}*.go`, `internal/admin/hermes*.go`,
`internal/mcp/server.go`, `internal/tui/{cmds,model,ports}.go` + tests.

## Tasks

- [x] **T1 — Re-derivación (scout read-only)**: reconstruir los 8 hallazgos contra el
      diff de `5694316`: file:line, categoría/lens, severidad, descripción, y si el
      hallazgo sigue vigente en main (algunos pueden haber sido absorbidos por cambios
      posteriores). Salida: tabla de candidatos + disposición propuesta por item.
- [x] **T2 — Disposiciones del owner**: presentar la tabla y decidir por item:
      fix / desmentido (disposition) / defer-documentado.
- [~] **T3-T6 — Fixes agrupados**: según disposiciones (superficies a confirmar tras T2).
  - [x] T3 presentación Hermes (R2-01/02/03 + R4-001) — commit `63d3bda`, GGA PASSED
  - [x] T4 admin core (R2-04 + R3-001 + reubicación ApplyHermesConfig) — commit
    `0f619d2` (amended desde dfb8363; el hook GGA y el índice absorbieron los 10
    archivos en un solo commit, mensaje corregido por amend)
  - [x] T5 mcp + cmd tests (R3-002 + R3-003) — commit `fd68be6`
  - [x] T6 close-out: pipeline completo (fmt/vet/golangci 0/build/test -race 15/15) +
    **gate nativo APPROVED primera pasada** (lineage review-7b524be75481095e, tier high,
    4/4 lens materialize, 17 paths / 958 líneas, budget 200 sin correcciones, receipt
    ddfaba23 quemado 2026-09-25, 5 hallazgos advisory SUGGESTION) + issue-first PR
- [ ] **T7 — Close-out**: pipeline completo (fmt/vet/golangci/build/test -race), gate
      nativo por routing (default → review nativo), issue-first PR, merge por owner.

## Disposiciones (owner, 2026-09-25): 8/8 FIX

- R2-01/02/03: fix conjunto (helper + tipo compartido + HermesConfig explícito).
- R2-04: fix (pointer receiver + invariante pinneada en test).
- R3-001: fix (predicado compartido en paquete neutro, cero import admin→mcp).
- R3-002: fix (Handler() construido una vez + test de invariante).
- R3-003: fix (test de propagación/cancelación del signal ctx).
- R4-001: fix (invalidación de error: éxito cacheado, fallo reintenta; paridad con consola).

**Agrupación por superficie** (evita hunks solapados):
- **T3 — presentación Hermes** (R2-01/02/03 + R4-001; mismos archivos: `cmd/mcp-server/
  admin_tui.go`+test, `internal/tui/{cmds,model,ports}.go`+app_test.go): dos commits
  (plumbing primero, invalidación después) del mismo worker.
- **T4 — admin core** (R2-04 + R3-001; `internal/admin/hermes.go`+test + nuevo paquete
  neutro de predicado loopback consumido por admin y mcp).
- **T5 — mcp + cmd tests** (R3-002 `internal/mcp/server.go`+test; R3-003
  `cmd/mcp-server/main_test.go`).
- **T6 — close-out**: pipeline completo, gate nativo (default), issue-first PR.

Presupuesto estimado: ~180-280 líneas con tests — dentro de 400.

## T1 — Re-derivación (scout mufr, 2026-09-25)

Distribución exacta recuperada: 4 R2 (Readability) + 3 R3 (Reliability) + 1 R4
(Resilience); R1 Risk y refuter sin reporte. Todos vigentes en main (los commits
posteriores son docs-only). Los textos son reconstrucciones: alta confianza en
R2-01/02/03, R3-002/003, R4-001; enmarcado plausible en R2-04 y R3-001.

| ID | Lens | Sev. | Ubicación (main) | Hallazgo | Disposición propuesta |
|----|------|------|------------------|----------|----------------------|
| R2-01 | Readability | SUGGESTION | internal/tui/cmds.go:375; cmd/mcp-server/admin_tui.go:1091 | Cadena load→Set→Write→snippet duplicada entre wizard Bubble Tea y consola (runConfigureHermesFlow), sin mecanismo que fuerce paridad → riesgo de drift | fix (helper compartido) o defer |
| R2-02 | Readability | SUGGESTION | cmd/mcp-server/admin_tui.go:86,100,112; internal/tui/ports.go:52 | Los mismos 2 facts Hermes en 3 shapes ((path,url), tui.HermesConfig, consoleHermes) con mapeo manual y casing inconsistente | fix (un tipo compartido) o defer |
| R2-03 | Readability | WARNING | internal/tui/model.go:762 → cmds.go:375 | El write de Hermes lee deps.Hermes.Path/EndpointURL poblados por un handler no relacionado (onHermesData); flujo de datos implícito entre dos handlers | fix (pasar HermesConfig explícito) |
| R2-04 | Readability | SUGGESTION | internal/admin/hermes.go:88,102,273 | HermesDocument sigue envolviendo map[string]any; storage() con value receiver + lazy init exige reasignación manual (d.fields = fields) que un método futuro puede olvidar | fix (pointer receiver + invariante) o defer |
| R3-001 | Reliability | SUGGESTION | internal/admin/hermes.go:176; internal/mcp/loopback.go:19 | Política loopback duplicada: HermesEndpointURL re-implementa ValidateLoopback con semántica distinta para 0.0.0.0 → riesgo de divergencia | fix (predicado compartido) o defer |
| R3-002 | Reliability | SUGGESTION | internal/mcp/server.go:101,105 | Cache de methodGate sin test que fije el invariante write-once; Handler() construido dos veces en construction (postChain + unauthChain) | fix (construir una vez + test) |
| R3-003 | Reliability | SUGGESTION | cmd/mcp-server/main.go:103,107; main_test.go:221 | Plumbing signal-ctx sin verificación conductual: TestExecuteCLIDispatch usa context.Background() y no aserta propagación/cancelación | fix (test de propagación) |
| R4-001 | Resilience | WARNING | cmd/mcp-server/admin_tui.go:65 | sync.OnceValues cachea también el error del bootstrap Hermes → fallo sticky toda la sesión sin retry ni hint; además diverge de la consola (resolveHermes re-resuelve en cada apertura) | defer-documented (intent) o fix |

Considerados y desmentidos (excluidos): puerto no numérico aceptado por
HermesEndpointURL (falso — ValidateHermesURL lo rechaza); signal ctx cosmético (falso —
NewDatabase usa ctx en pragmas/schema). Pre-existente observado, no del PR: lista
hand-maintained de wiredRepoInventory + copia duplicada en test (era R3-003 de
micro-fixes).

## Evidence

### T3 — presentación Hermes (worker mufw2vrm-2-w1xc + verificación orchestrator)

6 archivos, +209/−115 (neto +94). Pipeline completo verde localmente: fmt/vet/golangci 0
issues/build OK/test -race 14/14 paquetes.

- **R2-01**: `tui.ApplyHermesConfig(path, endpointURL, phone)` — cadena única
  Load→SetHermesServer→Write→snippet-fallback; wizard (`hermesConfigCmd`) y consola
  (`runConfigureHermesFlow`) la consumen; `writeHermesSnippetFallback` ya no re-renderea
  (imprime el snippet que devolvió la cadena → drift imposible).
- **R2-02**: `resolveHermesBootstrap` devuelve el único shape exportado `tui.HermesConfig`;
  eliminados `resolveHermesConfig`, `consoleHermes` y el type local `hermesResolver`;
  casing unificado `Path`/`EndpointURL`.
- **R2-03**: `hermesConfigCmd(hermes HermesConfig, phone string)` sin `Deps`; el confirm
  pasa `m.deps.Hermes.HermesConfig` explícito (model.go:423). El store en
  `m.deps.Hermes` queda documentado como estado de modelo (view.go lo renderiza).
- **R4-001**: `tui.NewCachedHermesResolver` (mutex, cachea SOLO el éxito, el fallo
  reintenta en la próxima apertura); reemplaza `sync.OnceValues` en ambos composition
  roots; contrato de retry unificado con la consola.
- **Tests nuevos**: `TestNewCachedHermesResolverCachesSuccessAndRetriesFailure`
  (internal/tui) + `TestResolveHermesBootstrapRetriesAfterAFailure` (cmd) — fallarían
  bajo el OnceValues viejo.
- **Residuales aceptados**: (a) el helper vive en `internal/tui` porque
  `internal/admin` no estaba en la superficie del worker → reubicación a
  `internal/admin/presentation.go` ~20 líneas, plegada al scope de T4; (b) view.go
  sigue leyendo de `m.deps.Hermes` (sin cambio de comportamiento, intencional).

GGA pendiente de commit (gate en T6).

### T4 — admin core (worker mufxz1qw-4-8xxl + verificación orchestrator)

~+114/−74 neto + paquete nuevo (~85). Pipeline completo verde: fmt/vet/golangci 0/build/
test -race 14/14 (incluye el paquete nuevo).

- **R2-04**: `HermesDocument.storage()` ahora con pointer receiver — la inicialización
  del storage cero ocurre in-place y `SetHermesServer` ya no necesita la reasignación
  manual; invariante pinneada por `TestHermesDocument_ZeroValueMutationPersists`.
- **R3-001**: paquete nuevo `internal/loopback` (solo stdlib, cero deps internas) con
  `Classify(bind) (net.IP, Reason)` (NotAnIP/Unspecified/NotLoopback/OK, orden
  load-bearing Unspecified antes de NotLoopback); consumido por
  `mcp.ValidateLoopback` y `admin.HermesEndpointURL`, que mapean cada Reason a sus
  mensajes existentes verbatim. Import admin→mcp sigue en cero.
- **Reubicación**: `ApplyHermesConfig` movida a `internal/admin/presentation.go` (el
  hogar convencional de helpers compartidos por los dos entry points); call sites en
  `hermesConfigCmd` y `runConfigureHermesFlow` actualizados; cero residuos de
  `tui.ApplyHermesConfig` (grep 0).
- **Corrección del orchestrator sobre el worker**: el mapeo verbatim de Unspecified
  daba al bind `::` el mensaje wildcard que nombra "MCP_BIND=0.0.0.0" (históricamente
  `::` recibía el mensaje genérico de no-loopback). Restaurada la fidelidad histórica:
  solo el literal "0.0.0.0" recibe el mensaje wildcard; `::` hace fallthrough al
  genérico (mismo accept/reject en todos los casos). Verificado: fmt/vet/lint 0 +
  test -race mcp/loopback green.

GGA pendiente de commit (gate en T6).

### T5 — invariantes (worker mufyashd-5-0dz4 + verificación orchestrator)

3 archivos, +93/−10. Pipeline completo verde: fmt/vet/golangci 0/build/test -race 15/15
paquetes.

- **R3-002**: el handler `/mcp` se construye UNA vez en `NewServer` (post-registerTools,
  campo `s.handler`); `Handler()` y ambas ramas de `AuthHandler` derivan del mismo valor
  cacheado. `TestServerHandlerChainIsStable` pinnea el comportamiento servido (dos
  fetches → respuestas idénticas); el doc-comment honesto: la estabilidad de alocación
  no es observable desde afuera, el build-once vive en `NewServer`.
- **R3-003**: `TestExecuteCLIForwardsProcessContext` — identidad del ctx
  (`received != ctx`) + observación de cancelación pre-despacho, en las 3 ramas de
  dispatch (`serve`, `admin tui`, `hermes chat`), reusando el seam `cliRunners` sin
  hooks de producción.
- Nota del child (correcta): el reminder RDD del child fue dispositionado por el
  orchestrator — el gate corresponde en T6 por decisión explícita del owner (plan
  aprobado 2026-09-25: commits por work-unit sin gate individual, gate nativo único
  sobre el candidato completo).

### T5 — invariantes (commit fd68be6)

`test(mcp,cmd): pin the method-gate cache invariant and signal-context
propagation` — 3 archivos +93/−10 (+ doc). GGA PASSED. Árbol limpio.

### Arqueo final del feature (T6)

8/8 findings implementados: R2-01/02/03 + R4-001 (63d3bda), R2-04 + R3-001 +
reubicación (0f619d2), R3-002 + R3-003 (fd68be6). Disposiciones previas: 2 hallazgos
desmentidos por el scout (sin código), 2 warnings GGA pre-existentes documentados.
Branch feat/hygiene-followups-2: 7 commits sobre main @ 3929610.

GGA pendiente de commit (gate en T6).

### T6 — gate nativo (lineage review-7b524be75481095e, APPROVED)

- START committed range (baseRef main, committedOnly): high tier, 4 lens, 17 archivos,
  958 líneas, correction budget 200 (sin uso — cero transición de corrección).
- Group capture 4/4 admitidos; closure native-last-event; acknowledged authority BURNED
  (consumed_revision ddfaba23, burn evidence gentle-ai.review-acknowledged/v1).
- **5 hallazgos advisory no-bloqueantes (SUGGESTION) → trabajo posterior**:
  1. R2-applyhermes-placement — internal/admin/presentation.go:36-52 (colocación del
     helper; la home actual es la elegida, revisar si conviene separar del core).
  2. R2-loopback-fallthrough — internal/mcp/loopback.go:33-42 (el fallthrough 0.0.0.0/::
     puede leerse mejor con un sub-mapeo explícito).
  3. R2-loopback-ip-unused — internal/loopback/loopback.go:33-44 (el net.IP devuelto por
     Classify no lo consume ningún caller; considerar simplificar la firma).
  4. R3-001 — internal/mcp/server_test.go:218-225 (naming/cobertura del test de chain).
  5. R3-002 — internal/admin/presentation_test.go:174 (idem, test de ApplyHermesConfig).

### Merge identity

- Issue #101 (status:approved) → PR #102 `fix(hygiene): disposition de los 8 hallazgos
  informativos del gate de hygiene` — squash-merged by owner as `1d773b5` on main
  (2026-09-25); issue auto-closed; branch deleted local+remote. CI: Detect Go changes +
  Verify Go code PASS. Post-merge sanity green (build + test -race 15/15). main @ 1d773b5.
- Delivery complete: backlog obs 914 item de higiene cerrado. Sigue: (5) smoke VM v0.6.0,
  (4) bot WhatsApp por-sender, (3) transición a producción.

## Notes

- Routing del gate: default → review nativo (Go code). GGA on commits con patrón
  owner-autorizado.
- Runtime: gentle-ai/gentle-pi 3.7.0 — bindings de capture desde FACADE status
  (camelCase), START committed range con {"mode":"ordinary","baseRef":"main",
  "committedOnly":true}.
- Watch: gentle-shell#1324/#924 (validator bug, sin fix — workaround para corrections:
  fresh START sobre candidato corregido), #1352, #1371, GGA #131.
