# Feature: hygiene-followups — limpieza de follow-ups vivos

**Status**: COMPLETE (merged)
**Started**: 2026-09-24
**Branch**: `feat/hygiene-followups` (desde main @ 94ee8fa)
**Origin**: follow-ups acumulados del gate de hermes-config-tui (4 informativos R3) + micro-debt
pre-existente de micro-fixes T5. Reporte upstream de GGA ya despachado (comentario en
gentleman-guardian-angel#113, 5803910920).

## Alcance: 6 fixes + 1 disposition

| # | Item | Fix | Decisión owner |
|---|------|-----|----------------|
| 1 | R3-hermes-eager-bootstrap | Lazy bootstrap: el error de resolución deja de bloquear el startup de `admin tui`; se surfacea al abrir la opción 7 | Fix (40-80 líneas, 5 archivos) |
| 2 | R3-wildcard-bind-endpoint | Validación inline en `HermesEndpointURL` (rechaza bind no especificado / no loopback con error semántico español; sin import admin→mcp) | Fix (10-20 líneas) |
| 3 | R3-windows-mode-assertions | Guard explícito `runtime.GOOS == "windows"` en los 2 tests de modos (premise del hallazgo corregida: hoy NO están salteados — falta el guard) | Fix (test-only) |
| 4 | R3-lossy-config-roundtrip | Desviación aceptada documentada + test de regresión de preservación semántica de escalares típicos | Documentar (0-15 líneas) |
| 5 | Nil-AuthMiddleware panic | **DESMENTIDO**: `Server.AuthHandler` ya tiene guard explícito con panic de diseño (`server.go:97-100`) + test que fija la intención ("must fail fast at wiring time") | Sin código — disposition |
| 6 | methodGate Handler() rebuild | Cache del unauth handler junto a postChain (verificado seguro: registro de tools es write-once desde NewServer) | Fix (1-3 líneas) |
| 7 | openCommandDependencies ctx | Plumbing completo: `signal.NotifyContext` en main() → executeCLI → cliRunners → 3 runners → openDatabase | Fix (30-60 líneas) |

## Decisiones (owner, 2026-09-24)

- **#4**: documentar + test (NO reescritura node-based — coste 100-200 líneas vs valor bajo:
  solo afecta escalares de claves que no son nuestras).
- **#7**: plumbing completo (signal.NotifyContext), no cosmético.
- **#5**: premisa del hallazgo desmentida por el scouting — ya está guardado; se registra la
  disposición y no se toca.

## Interacciones relevantes (del scouting)

- #1 y #7 comparten hunks en `admin_tui.go` (call sites de `openCommandDependencies` y
  `resolveHermes*`) → mismo worker, plumbing primero.
- #2 (qué valida `HermesEndpointURL`) e #1 (cuándo se surfacea el error) cambian juntos el
  camino de resolución de Hermes → validar combinados en el gate.

## Presupuesto

~110-190 líneas estimadas (con tests) — dentro del presupuesto ~400. Ruteo del gate:
default → review nativo.

## Tasks

- [ ] **T1 — internal/admin** (`internal/admin/hermes.go`, `hermes_test.go`): #2 validación
      de bind en `HermesEndpointURL` (rechaza unspecified + no-loopback, mensajes semánticos),
      #3 guard Windows en los 2 tests de modos, #4 doc de desviación aceptada + test de
      regresión semántica.
- [ ] **T2 — plumbing + lazy** (`cmd/mcp-server/main.go`, `cmd/mcp-server/admin_tui.go`,
      `internal/tui/{ports,model,cmds}.go`, `internal/tui/app_test.go`,
      `cmd/mcp-server/admin_tui_test.go`, `cmd/mcp-server/main_test.go`): #7 signal context
      plumbing completo + #1 bootstrap lazy (Deps.Hermes pasa a resolver diferido; el error
      se surfacea en la opción, no en el startup).
- [ ] **T3 — inline**: #6 cache del unauth handler en `methodGate` (`internal/mcp/server.go`).
- [ ] **T4 — close-out**: pipeline completo, review nativo, issue-first PR.

## Evidence

- T1 (#2 #3 #4): `c28743d` — HermesEndpointURL valida bind (hostname/unspecified/no-loopback,
  mensajes semánticos, import admin→mcp sigue en cero); guard Windows en los 2 tests de
  modos; desviación aceptada del round-trip documentada + test de regresión semántica
  (5 escalares no propios). Primera corrida de GGA: STATUS: FAILED con 1 CRITICAL
  sustantivo (HermesDocument map[string]any — único tipo exportado con any del árbol) →
  fix en el mismo commit: HermesDocument es ahora struct exportado que envuelve un
  map genérico unexported con métodos tipados (SetHermesServer con receiver puntero +
  lazy init); superficie exportada sin any (verificado con go doc). GGA tras fix:
  STATUS: PASSED (/tmp/gga-hy1-fix.log).
- T2 (#7 #1): `1cc9f23` — signal.NotifyContext en main() (SIGTERM/SIGINT), ctx thread
  executeCLI → commandRunner → 3 runners → openCommandDependencies → openDatabase;
  serve mode reutiliza el mismo ctx (coexistencia con el signal.Notify de mcp.Run
  documentada); flujos interactivos usan el signal ctx SOLO para el open compartido
  (writes del operador corren a término). Bootstrap de Hermes lazy en ambas
  presentaciones: consola resuelve en la acción de la opción 7 ("Error: %v" y menú
  reabre); TUI resuelve dentro de hermesDataCmd vía Deps.Hermes = HermesBootstrap
  (resolver cacheado con sync.OnceValues en el composition root). Tests negativos
  nuevos: bind inválido → startup sin efecto, error solo al abrir la opción (consola
  y TUI). GGA: STATUS: PASSED (/tmp/gga-hy2.log).
- T3 (#6): `230b453` — unauthChain cacheado junto a postChain en methodGate
  (registro de tools write-once desde NewServer, verificado). GGA: STATUS: PASSED
  (/tmp/gga-hy3.log).
- **#5 (disposición, sin código)**: hallazgo desmentido — `Server.AuthHandler` ya tiene
  guard explícito con panic de diseño (internal/mcp/server.go:97-100: "mcp: AuthHandler
  requires a non-nil AuthMiddleware") + test que fija la intención
  (TestAuthHandlerPanicsOnNilMiddleware, "must fail fast at wiring time, not per-request").
  Cambiarlo a (http.Handler, error) contradiría la intención documentada del test.
- **Merge**: squash-merged by owner as `5694316` on main (PR #100, CI green); issue #99
  auto-closed; branch feat/hygiene-followups deleted local+remote; post-merge sanity green
  (build + test -race admin/tui/mcp/cmd). main @ 5694316 clean.

## Notes

- #5 registrado como hallazgo con premisa desmentida — el guard existe y está test-pinned;
  cambiarlo a `(http.Handler, error)` contradiría la intención documentada del test.
- El reporte upstream del parse window de GGA está hecho (no forma parte de este PR).
