# feat-tools-list-role-filter — tools/list por rol (agent surface T1)

Issue: #265 · Rama: `feat/feat-tools-list-role-filter` · Riesgo: ítem 3 (contrato observable) · Origen: `odd/tasks/feat-agent-surface-proposal.md` (S2, S3) + hand-off `t1-tools-list-handoff`.

## Specs

- **S1** — `tools/list` MUST devolver solo las tools que el caller resuelto puede ejecutar según el `ToolRBAC` del composition root. Las tools sin entrada en el mapa ("any authenticated") MUST seguir visibles para todo caller autenticado.
- **S2** — El filtrado MUST NOT quitar ninguna tool que el caller podía invocar antes. La reducción es coincidencia exacta con el `ToolRBAC`; las violaciones de RBAC siguen respondiendo igual que hoy.
- **S3** — El caller `owner` MUST seguir viendo el registro completo (26 tools); `expectedToolCount` y el E2E del binario no cambian.
- **S4** — `tools/list` sin `X-Caller-Id` MUST seguir respondiendo `-32000` con `"no se proporcionó X-Caller-Id"`, nunca una superficie vacía o completa.
- **S5** — El filtro es de **descubrimiento**, no de autorización: `tools/call` sobre una tool oculta MUST seguir rechazado por el RBAC con `-32001` y `auth.MsgForbiddenTool`.
- **S6** — El filtro MUST leer el rol del **contexto del request** (`auth.RequireCaller`), nunca del path del request: `auth.ToolRBAC` keyea sobre `r.URL.Path`, que solo se reescribe al nombre de la tool para `tools/call`.

## Tasks

- **T1** · S1-S6 · implementación + tests + spec · commit único
  - `internal/mcp/tool_filter.go`: receiving middleware (`toolsListFilter`) con el predicado espejo del gate (`canInvoke`).
  - `internal/mcp/config.go`: campo `ToolRBAC auth.ToolRBAC` (nil = sin filtro).
  - `internal/mcp/server.go`: `AddReceivingMiddleware` en `NewServer`.
  - `cmd/mcp-server/main.go` + `internal/mcp/server_integration_test.go`: inyectar el mismo mapa que autoriza.
  - `internal/mcp/tool_filter_integration_test.go`: por rol, ungated visibles, defensa en profundidad, header ausente, predicado unitario.
  - `openspec/specs/mcp-transport/spec.md`: REQ-MT-005 reescrita con escenarios por rol.

## Log

- **L1** (owner, verbatim): *"Arrancamos T1 — tools/list filtrado por rol, el único pendiente del plan agent surface. El hand-off completo está en memoria: hacé mem_search "T1 tools/list handoff" y leé la observación t1-tools-list-handoff antes de tocar código; ahí está el diseño verificado del SDK, la trampa y las incógnitas a resolver. Reglas de la casa: issue primero (status:approved + un type:*), RED antes de GREEN, verificador independiente antes del review nativo, assess con ítem 3 escalado, y preguntame antes de commit y antes de merge."*
- **L2** (agente, verificado): SDK v1.7.0 — `Middleware = func(MethodHandler) MethodHandler`, `MethodHandler(ctx, method string, req Request)`; el método llega como parámetro y el `*ListToolsResult` se muta por puntero (la serialización ocurre después de la cadena); sin API nativa de filtro. La ctx del request HTTP es la ctx de la conexión Streamable → `auth.RequireCaller` ve el caller que inyectó `AuthMiddleware`.
- **L3** (agente, corrección de hecho del hand-off): la violación RBAC sobre `tools/call` se observa como **-32001** (403 → traductor), no -32000; conteos por rol tras T2: owner 26 / admin 14 / staff 11 / client 11.
- **L4** (RED observado, 2026-11-23): con el harness sin el mapa, admin/staff/client recibían 26 tools y `TestToolsCallOnAFilteredToolStaysForbidden` fallaba en su precondición. GREEN mínimo → los 4 roles pasan. Mutación (quitar `AddReceivingMiddleware`) → 2 tests caen; restaurado.
- **L5** (verificador independiente `gentle-ai-verify`, 2026-11-23): **PASS** — 0 CRITICAL, 1 WARNING, 3 SUGGESTION; build y `go test -race -count=1 ./...` verdes; confirmado que el rol sale de ctx (traza `middleware.go:163` → `streamable.go:703` → SDK), que `canInvoke` es espejo exacto del paso RBAC del middleware, y que ningún tool invocable queda oculto. Aplicado en el mismo ciclo: WARNING F4 (log del fail-open sin caller), SUGGESTION F2 (test literal de los 11 nombres de `staff` contra REQ-MT-005) y F3 (escenario `-32000` en la spec). No aplicado: F4-note (el test de ungated es guard de drift, no detector de mutación — documentado como tal).
- **L6** (pipeline + gate, 2026-11-23): `gofmt` · `go vet` · `golangci-lint run ./...` (0 issues) · `go build -o /dev/null ./...` · `go test -race -count=1 ./...` → exit 0, 15 paquetes `ok`, cero `FAIL`. `assess` con escalate ítem 3 → `risk: high`, plan con verificador independiente. **Review nativo `review-5accc44d4fbed904` APPROVED, authority burned** (3 lenses: risk, readability, reliability; 3 model runs). 8 advisory findings **no bloqueantes** (R1-001, R2-1..R2-3, R3-01..R3-04): ninguno abrió corrección. Resumen de lo que señalan, como follow-up y no como defecto de este candidato: el fail-open del filtro depende de que el transporte siga entregando el caller (ahora con `Warn`), el comentario de `canInvoke` puede leerse como si el espejo del predicado se mantuviera solo, la nota de paginación vive en prosa (no hay assert de arranque), un rol vacío/desconocido se degrada silenciosamente a "solo tools sin fila", y los pines literales de conteo exigen editar cuatro lugares ante un cambio de política.
- **L7** (cierre, 2026-11-23): commit `cac6cb8` (`feat(mcp): filter tools/list by caller role`) — **un solo work unit de 487 líneas autoradas**, por encima del corte suave de ~400: no se puede partir sin dejar un intermedio inerte, porque código + tests + spec describen un único comportamiento (el filtro y su contrato son el mismo artefacto).
- **L8** (cierre, 2026-11-23): PR #266 mergeado como **`16f2b87`** (squash), issue #265 cerrada, CI de `main` verde (`Verify Go code: success`). Segundo review nativo sobre el **rango commiteado** `154bbc6..16f2b87`: `review-97623be979209182` **APPROVED**, authority burned, 10 advisories no bloqueantes (2 WARNING). Follow-ups vivos en el issue **#267**; el fail-open del filtro sin caller se resolvió como **decisión de contrato**: se corrige la spec para que describa el comportamiento real, no se cambia el código (owner, 2026-11-23).
