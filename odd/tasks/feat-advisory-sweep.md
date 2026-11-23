# feat-advisory-sweep — cerrar advisories de #268 y #274

Issues: #268 (advisories de T1) + #274 (advisories de #273) · Rama: `refactor/advisory-sweep` · Riesgo: ítem 3 (mueve el predicado de autorización a un solo lugar) · Origen: reviews nativos `review-97623be979209182` y `review-8c05a20a505d86e4`.

## Specs

- **S1 (predicado único)** — La decisión "¿este rol puede invocar esta tool?" MUST vivir en UNA sola función de `internal/auth` (`CanInvoke(rbac, tool, role)`), consumida por `AuthMiddleware` (paso RBAC) y por el filtro de descubrimiento de `internal/mcp`. `mcp.canInvoke` MUST desaparecer. Regla que la función MUST preservar byte a byte: fila ausente **o vacía** ⇒ permitido a cualquier caller autenticado; fila poblada ⇒ el rol debe figurar.
- **S2 (registro condicional y ruidoso)** — `NewServer` MUST registrar el middleware de descubrimiento **solo** si hay política (`len(cfg.ToolRBAC) > 0`). Con `ToolRBAC` nil MUST emitir **un** `Warn` al construir, para que un `Config` parcial no degrade el descubrimiento en silencio; un `Config` nil NO MUST filtrar (mismo comportamiento observable que hoy).
- **S3 (invariante de paginación pineado)** — El invariante "el filtro recorta DESPUÉS de paginar, así que `PageSize` debe quedar en 0" MUST quedar pineado de forma **observable**: la respuesta `tools/list` MUST NOT traer `nextCursor`, y el literal de `ServerOptions` MUST documentar por qué no se pagina.
- **S4 (cinturón de cardinalidad, #274)** — `TestToolRolesCoverTheRegisteredSurface` MUST volver a aseverar `len(ToolRoles())+len(UngatedTools()) == len(registrados)`, con un comentario que declare que es **deliberadamente redundante** con los cuatro loops (dos revisores lo pidieron).
- **S5 (pins citados por símbolo, #274)** — Los comentarios de decisión de `internal/repository/bookings.go`, `business_profile.go` y `business_hours_exception.go` MUST citar el **nombre del test** que pinea el gate superior, no solo el archivo (un rename no puede dejar el rationale apuntando a la nada).
- **S6** — Ningún contrato cambia: mismos códigos, mismo texto de rechazo, misma superficie por rol (owner 26 / admin 14 / staff 11 / client 11).

## Tasks

- **T1** · S1 · `internal/auth` (nueva `CanInvoke` + tabla de tests) · `internal/auth/middleware.go` (usarla) · `internal/mcp/tool_filter.go` (borrar `canInvoke`, usar `auth.CanInvoke`)
- **T2** · S2 · `internal/mcp/server.go` (registro condicional + `Warn`) + test que capture el log con `ToolRBAC` nil
- **T3** · S3 · assert de `nextCursor` vacío en `tool_filter_integration_test.go` + comentario en el literal de opciones
- **T4** · S4,S5 · `internal/mcp/tool_roles_test.go` (restaurar el assert) + los tres comentarios de repositorio con el símbolo del test

Pins existentes que hay que citar (verificar el nombre exacto con grep antes de escribirlo):
- `TestGetLoyaltyReportUseCase` (subtests `staff rejected` / `client rejected`) → gate de DEBT-1.
- `TestListHoursExceptionsUseCase_Forbidden` → gate del LIST de excepciones.
- El perfil: la tool MCP acepta los cuatro roles (`tool_roles.go`), `update_business_profile` es owner-only; el pin del camino TUI es `internal/tui` (skill console) — citar el que exista, sin inventar símbolos.

## Evidencia de mutación exigida

- Quitar la llamada a `auth.CanInvoke` del filtro ⇒ los tests por rol MUST caer.
- Quitar el `Warn` del caso nil ⇒ el test de S2 MUST caer.

## Log

- **L1** (owner, verbatim): *"Arregla los puntos 1 y 3 para limpiar los advisories"* → punto 1 = issue #268 (advisories de T1), punto 3 = advisories de #273, que se abren como #274 para que el `pr-gate` tenga un issue cerrable por PR.
- **L2** (agente, rationale de S1): tras compartir la función, una "prueba de equivalencia entre consumidores" deja de tener sentido (serían la misma función); la garantía pasa a ser estructural. La tabla de tests de `CanInvoke` cubre las tres formas de fila × los cuatro roles, y los tests de integración siguen probando el resultado observable (superficie por rol y `-32001` sobre tool oculta).
- **L3** (worker, implementación + evidencia). S1–S5 implementados. `CanInvoke` vive en `internal/auth/can_invoke.go` junto a `roleAllowed` (movido desde `middleware.go`); el paso 6 de `AuthMiddleware` y `visibleTools` lo llaman, `mcp.canInvoke` borrado. Pins nuevos: `TestCanInvoke` (3 formas × 4 roles + membresía/case), `TestServerRegistersNoDiscoveryFilterWithoutAPolicy` (Warn única + filtro NO registrado, probado por la ausencia del warn de "sin caller"), `TestToolsListCarriesNoNextCursor` (4 roles), y el assert de cardinalidad restaurado en `TestToolRolesCoverTheRegisteredSurface`.
  - S2 implementado como `len(cfg.ToolRBAC) == 0` (nil **o** mapa vacío): un mapa vacío degrada el descubrimiento exactamente igual que nil, y el spec solo fija el caso nil. Es un superconjunto, no una desviación.
  - S5: símbolos verificados con grep antes de escribirlos — `TestGetLoyaltyReportUseCase` (subtests "staff rejected"/"client rejected"), `TestListHoursExceptionsUseCase_Forbidden`, `TestToolsListFiltersByCallerRole` + `TestRegenerateSkillForConsoleOutcomes` para el perfil. Hallazgo: **no existe** test de caso de uso que pinee el `RequireRole(owner)` de `UpdateBusinessProfileUseCase` (ningún subtest "forbidden" en `update_business_profile_test.go`); el gate de escritura solo está pineado por el mapa de tools. El comentario del repo se limita a afirmar el hecho, sin citar un pin inexistente.
  - Evidencia de mutación (observada, no inferida): `visibleTools` con `if true` ⇒ rojo `TestToolsListFiltersByCallerRole` (admin/staff/client 26 tools), `TestToolsCallOnAFilteredToolStaysForbidden`, `TestVisibleToolsMirrorsTheTransportGate`, `TestToolsListStaffSurfaceMatchesTheSpec`. Quitar el `Warn` del caso nil ⇒ rojo `TestServerRegistersNoDiscoveryFilterWithoutAPolicy` ("0 warn records; want exactly 1"). Registrar el filtro también en la rama nil ⇒ rojo por el segundo aserto ("2 warn records").
  - Pipeline (worktree limpio tras restaurar mutaciones): `gofmt -l ./cmd ./internal` 0 líneas (exit 0) · `go vet ./...` exit 0 · `golangci-lint run ./...` 0 issues (exit 0) · `go build -o /dev/null ./...` exit 0 · `go test -race -count=1 ./...` exit 0 — 15 paquetes `ok`, 0 `FAIL`, 3 sin ficheros de test.
  - S4: `len(ToolRoles())+len(UngatedTools()) == 26 == expectedToolCount` se comprobó a mano tras restaurar las mutaciones; el assert nuevo es la única comprobación aritmética del TAMAÑO (los cuatro loops prueban la partición).
- **L3** (implementación, 2026-11-23): S1-S5 implementadas por el writer delegado, con las tres mutaciones ejecutadas y restauradas (filtro ignorando `CanInvoke` ⇒ rojo en 4 tests; sin el `Warn` de S2 ⇒ rojo; `Warn` + filtro registrado ⇒ rojo). Desvío declarado y aceptado: S2 dispara con política **vacía** (nil o mapa vacío), no solo nil — un mapa vacío degrada el descubrimiento igual, así que avisar también es correcto. Hallazgo del writer: **no existe** test de use case que pinee el `RequireRole(RoleOwner)` de `UpdateBusinessProfile` (el archivo solo usa `ownerCaller()`); el comentario de `business_profile.go` afirma el hecho sin inventar un pin, y el gate sí está pineado en el transporte por `TestToolsCallOnAFilteredToolStaysForbidden`.
- **L4** (verificador independiente, 2026-11-23): **PASS**, 0 CRITICAL. Confirmó equivalencia de tabla de verdad del predicado movido (incluye fila vacía, rol con distinto case, mapa nil), definición única de `roleAllowed`, `cfg.Logger` no nulo en el punto del `Warn`, y que los cuatro símbolos citados existen. Aplicadas sus dos sugerencias: el pin de `nextCursor` es **sensible al tamaño** (rojo con `PageSize: 5`, verde con `PageSize: 30` porque el SDK no emite cursor si entra todo en una página) y el comentario del perfil citaba como "pin" un test que inyecta un fake y sobreviviría a un gate real.
- **L5** (review nativo `review-3210d41f513779ce`, 2026-11-23): **APPROVED**, authority burned (high por escalada ítem 3, lenses risk/readability/reliability). Un advisory WARNING **falso positivo**: afirma que el `Warn` dereferencia `cfg.Logger` nil y paniquea; `NewServer` normaliza `cfg.Logger` a `slog.Default()` en su primera sentencia (`internal/mcp/server.go:26-28`), antes del `Warn`. No se toca código por un hallazgo que la lectura del archivo desmiente.
- **L6** (slice): **442 líneas autoradas en un solo commit**, por encima del corte suave de ~400. No se parte: el cambio es un predicado movido (código + su tabla de tests) más los pins que lo hacen observable; separar `internal/auth` del filtro de `internal/mcp` dejaría un intermedio donde el guard y el menú ya usan funciones distintas — exactamente el bug que este PR cierra. Del total, ~190 líneas son tests nuevos y ~120 comentarios de rationale.
