# Port default 3001 — MCP_PORT/MCP_BIND precedence and product default

Feature: change the product default MCP port from 3000 to 3001 and correct the documented precedence.
Three PRs, in this order: code+installer+tests → operations docs → records/contract/specs.

## Specs

- **S1** (default del producto): el default del MCP es **3001** en los tres lados — constante del código, fallback del instalador, y el `.env` que se crea al instalar. El owner: *"usaremos para todo el puerto 3001"* (2026-11-13).
- **S2** (precedencia, sin cambios de comportamiento): orden de resolución **1- Shell (entorno del proceso), 2- archivo `.env`, 3- default del código**. El owner: *"Ok me parece bien los 3 niveles y el órden...1-Shell, 2-archivo .env y 3-default."* Esto ya es lo que hace el código (`firstNonEmpty(os.Getenv(...), dotenv[...], default)`); el cambio es **documental**.
- **S3** (corregir la documentación): los documentos que invierten la precedencia se alinean a S2. El owner: *"debemos arreglar y que se haga correctamente: 'env vars > .env > defaults'"*.
- **S4** (el `.env` nace explícito): al crearse el `.env`, se escribe `MCP_PORT=3001` explícito. El owner: *"Al crearse el archivo .env ya debemos asignar el MCP_PORT=3001"*.
- **S5** (`MCP_BIND`): el default sigue siendo `127.0.0.1`; **no cambia**. El bind es palanca de seguridad (loopback), no de comodidad.
- **S6** (sin migración): las instalaciones existentes conservan su puerto; `ensure_env_file` no reescribe un `.env` existente. Ninguna instalación desplegada se mueve.
- **S7** (`SDD.md` congelado): *"El docs/SDD.md dejalo fuera y congelado."* No se toca. Balde A queda en 26 archivos.
- **S8** (cosmético en PR 2): *"Ok mete lo de design en el PR 2."* El banner SVG y los dos chips de `design/` entran en el PR 2.
- **S9** (las 8 menciones del bridge NO se tocan): el `3000` del bridge de Hermes/Baileys sigue siendo correcto y verdadero. No se edita; lo que se retira es la *recomendación* de movernos a 3001, ya innecesaria.

## Tasks

- **T1** · S1,S2,S4,S5,S9 · código+instalador+tests · rama `feat/port-default-3001-code` · PR 1 · commit único
  - `internal/mcp/config.go` (constante + doc), `cmd/mcp-server/main.go` (doc), `scripts/install.sh` (4 literales: 1517, 1759, 1796 comment, 1803)
  - tests: `config_test.go` `TestLoadConfigDefaults` + `TestLoadConfigMissingDotEnvIsSilent`; `install_deploy_test.sh` `test_ensure_env_file_creates`, `test_print_post_install_summary`, `test_mcp_effective_port_precedence` (solo el caso "sin .env → 3001"), `test_ensure_env_file_honors_exported_port` (solo el caso sin export)
- **T2** · S3,S8,S9 · docs de operación · rama `feat/port-default-3001-ops-docs` · PR 2
  - `README.md` (3 A + 2 D intactas), `AGENTS.md` (3), `docs/installation.md` (21 A + 3 D intactas), `docs/deployment.md` (23), `docs/maintenance.md` (3), `docs/demo-plan.md` (5)
  - retirar la recomendación obsoleta "si corrés WhatsApp poné MCP_PORT=3001" (README:80,101 · installation:55-63,174-188,465-471 · maintenance:311-315 · deployment:542-553 · install.sh:1882)
  - cosmético: `docs/assets/mcp-crm-banner.svg:46` + `design/mcp-crm-logo/01-terminal-card.html:79` + `02-calendar-minimal.html:61`
- **T3** · S2,S3 · registros, contrato y specs · rama `feat/port-default-3001-records` · PR 3
  - `docs/architecture/0007-server-config.md` (8), otros ADRs (14), `docs/gateway-contract.md` (3 — **§6.1 línea 97 se reescribe entero: es A y D en el mismo párrafo**)
  - `openspec/` specs vivas (9), incluido el **MUST** de `specs/install-service/spec.md:124`
  - `docs/PRD.md`: fila nueva de changelog (las filas 1.28/1.30/1.34 son historia: NO se reescriben; se corrige con una fila nueva)

## Trampas verificadas (no repetir)

1. `docs/gateway-contract.md:97` es A+D mezclado. Un string-replace afirmaría que el bridge se mudó. Reescribir el párrafo.
2. `scripts/install.sh:1499` ("used to hardcode 3000") es comentario histórico y **sigue siendo verdad**. No tocar.
3. El changelog del PRD es append-only. Las filas con la precedencia invertida (1.30, 1.34) no se corrigen en el lugar: se agrega una fila.
4. `openspec/changes/archive/**` (55 hits) está congelado. No se toca.
5. `docs/SDD.md` congelado (S7).

## Advisories del review del PR 1 (review-5c8578023bfdfd87, APPROVED + burned)

Tier `high` (por `process_boundary`/`shell_source` en `scripts/install.sh`), 4 lentes, 4 corridas. Todos **informational, no bloqueantes**: ninguno abrió corrección ni habilita re-revisar este candidato.

| ID | Lente | Ubicación | Severidad |
|---|---|---|---|
| `R1-001` | risk | `scripts/install.sh:1799-1804` (`mcp_effective_port`) | WARNING |
| `R4-001` | resilience | `internal/mcp/config.go:22` | WARNING |
| `R2-1` | readability | `scripts/install.sh:1517-1803` | SUGGESTION |
| `R2-2` | readability | `internal/mcp/config.go:15-19` | SUGGESTION |

**Pendiente de disposición**: se tratan como trabajo posterior. Si se deciden arreglar, va en su propio candidato (los textos del reviewer no sobreviven al burn: reconstruir desde la ubicación con `git show 8d1fd42:<path> | sed -n '<ini>,<fin>p'`).

## Disposición de los advisories (rama `refactor/dispose-advisories`, issue #231)

Los textos del reviewer **no** sobrevivieron al burn: cada advisory se reconstruyó desde el código de su propio commit (`8d1fd42` para el PR 1, `370a02f` para el PR 2) y se clasificó sólo si el defecto era verificable en esa ubicación. Lo que no se pudo sustanciar queda como NO-ACTION, no como parche especulativo.

| ID | Clase | Qué se hizo | Por qué |
|---|---|---|---|
| `R1-001` (risk, `install.sh:1799-1804`) | **REAL** | `mcp_effective_port` parsea el valor del `.env` como `config.LoadDotEnv` (trim + comillas) y `preflight_port_available` valida el puerto resuelto (entero 1-65535, fail-closed nombrando el archivo) | El `awk` crudo devolvía `"3005"` para `MCP_PORT="3005"`: el sondeo no matchea nunca, el pre-flight declaraba libre un puerto ocupado y la unit volvía al crash-loop que el chequeo existe para evitar |
| `R4-001` (resilience, `config.go:22`) | **NO-ACTION** (by design) | nada | "Sin fallback automático" es decisión registrada en ADR-0007: el server no reasigna puerto al chocar, y las dos palancas recíprocas están documentadas. El ángulo de valor inválido se cierra en la capa que recibe input del operador (instalador), no en `LoadConfig` |
| `R2-1` (readability, `install.sh:1517-1803`) | **REAL** | El literal `3001` pasa a la global única `DEFAULT_MCP_PORT`, y `print_post_install_summary` resuelve con `mcp_effective_port` en vez de su propio `${MCP_PORT:-3001}` | El default estaba escrito tres veces y el resumen ya **divergía**: con `.env` en 3100 y nada exportado imprimía `:3001` mientras la unit bindeaba 3100 (upgrade = todo host con `.env` previo) |
| `R2-2` (readability, `config.go:15-19`) | **COSMETIC** | El párrafo que explica el 3001 se movió del bloque `const` a la constante `defaultPort` | Colgaba arriba de `defaultBind` sin nombrarlo: quien buscaba por qué el bind es loopback leía una explicación de puertos |
| `R3-001` (reliability, `installation.md:448-470`) | **REAL** | Paso nuevo antes de elegir salida: leer el `MCP_PORT` real del `.env` y quién ocupa el puerto (`ss`/`lsof`), más la nota de que un install previo al 2026-11-13 conserva el puerto con que se creó | El bloque mandaba "elegí un puerto libre" sin forma de saber cuál es el del host ni quién lo tiene, y presentaba `3001` como default del host cuando manda el `.env` |
| `R4-001` (resilience, `maintenance.md:101`) | **REAL** | La verificación post-upgrade resuelve el puerto desde el `.env` en vez de hardcodear `3001` | §2.1 dice que el upgrade **preserva** el `.env` con su puerto personalizado: en cualquier host personalizado el `curl` hardcodeado falla y un upgrade sano se lee como roto |

Fuera de la tabla, error factual: la fila 1.47 del PRD citaba `#229` como PR de entrega cuando `#229` es la issue — el PR que entregó registros, contrato y specs es **#230** (`Closes #229`). Corregido en la fila, sin otro cambio en ella.

Baja confianza, y por eso la disposición se apoya en la evidencia, no en el texto perdido: en `R4-001` del PR 1 no se tocó nada (una constante correcta; el defecto supuesto es una decisión de ADR-0007) y en `R2-2` el cambio es sólo de forma.

## Advisories del review del PR #232 — ronda final (`docs/close-advisory-thread`, issue #233)

Ocho advisories con **una sola causa raíz en cinco de ellos**. La lente se deriva del prefijo del ID (`R1` risk, `R2` readability, `R3` reliability, `R4` resilience), igual que en la ronda anterior. Esta es la **última** ronda por decisión del owner: lo que genere va al backlog, no hay iteración siguiente.

| ID | Lente | Clase | Qué se hizo | Por qué |
|---|---|---|---|---|
| `R1-2` | risk | **REAL** | `docs/installation.md` §puerto en uso: el `awk` crudo pasa a `sed -n 's/^MCP_PORT=//p' … \| tr -d '"' \| tail -n 1` | El `awk` devolvía `"3005"` **con comillas**: `grep ":$PORT"` no matcheaba y el doc declaraba libre un puerto ocupado |
| `R2-1` | readability | **REAL** (misma causa raíz) | mismo snippet de `docs/installation.md`, misma corrección | El snippet documentaba un parser que divergía del binario: la misma familia de error que arregló el PR #232 en el instalador |
| `R3-001` | reliability | **REAL** (misma causa raíz) | mismo snippet de `docs/installation.md`, misma corrección | Con el valor entrecomillado el `ss \| grep` no encontraba al ocupante: diagnóstico falso en el paso que existe para diagnosticar |
| `R3-002` | reliability | **REAL** | `docs/maintenance.md` §2.1: mismo `sed`+`tr` en la verificación post-upgrade | El `health check` quedaba en `http://127.0.0.1:"3005"/healthz`, una URL rota |
| `R4-001` | resilience | **REAL** | `docs/maintenance.md` §2.1: el snippet escribe `PORT` en vez de `MCP_PORT` | El snippet **pisaba** la variable `MCP_PORT` del operador en su propia shell |
| `R1-1` | risk | **NO-ACTION** | Nada de código; sólo se amplió una línea del comentario de `mcp_effective_port` (`scripts/install.sh`) para apuntar al test | No hay fix: un parser Go (`config.LoadDotEnv`) no se comparte con un script bash. La paridad ya está acotada **conductualmente** por `test_mcp_effective_port_strips_quotes_like_the_binary` |
| `R2-2` | readability | **COSMETIC** | El comentario de validación dentro de `preflight_port_available` pasa de 8 a 5 líneas, conservando las tres razones (fail-closed, el `.env` no se valida en ningún otro lado, corre antes de tocar la unit) | El bloque de comentario era más largo que el código que explicaba |
| `R3-003` | reliability | **COSMETIC** | El comentario de `defaultPort` (`internal/mcp/config.go`) pasa de 5 a 3 líneas | Conserva el porqué de `3001` sin dominar el bloque `const` |

**Evidencia de la disposición `R1-1` (NO-ACTION, no pereza):** `test_mcp_effective_port_strips_quotes_like_the_binary` (`scripts/tests/install_deploy_test.sh:824`) escribe `MCP_PORT="3005"` en el `.env` y afirma que `mcp_effective_port` resuelve `3005`. La duplicación del parser queda pinneada por test, que es la única forma de pinneable cuando los dos lados son lenguajes distintos. El comentario ahora lo dice, para que nadie "arregle" la duplicación rompiendo la paridad.

Fuera de la tabla, verificado en la misma corrida: el `sed`+`tr` propuesto se probó contra un archivo con `MCP_PORT="3005"` y devuelve `3005` (el `awk` devolvía `"3005"`). `tr -d '"'` no cubre comillas simples (`MCP_PORT='3005'` sale como `'3005'`): se deja así a propósito — el `.env` lo escribe el instalador, que usa comillas dobles — y queda anotado acá para no re-descubrirlo.

## Log

- **L1** 2026-11-13 (owner): "Tomando en cuenta que el órden habitual y recomendado o ideal es: 1- Instalar y dejar funcionando Hermes en el Server. 2- Instalar nuestro MCP y configurarlo. Cual de los caminos consideras el más conveniente para nuestro proyecto? Yo tenía entendido que nuestro MCP primero lee el puerto definido en el archivo .env y usa eso, y si no hay .env usa el 3000... es así?"
- **L2** 2026-11-13 (owner): "Lo primero: debemos arreglar y que se haga correctamente: 'env vars > .env > defaults' y poner como default 3000 o 3001 según cual sea más estandard y adecuado para nosotros. Segundo: Al crearse el archivo .env ya debemos asignar el MCP_PORT=3001."
- **L3** 2026-11-13 (owner): "Ok me parece bien los 3 niveles y el órden...1-Shell, 2-archivo .env y 3-default." → confirma que la precedencia queda como está; el defecto es documental.
- **L4** 2026-11-13 (owner): "Lo que sí ya te confirmo es que usaremos para todo el puerto 3001."
- **L5** 2026-11-13 (owner): "Ok mide el desglose y después me cuentas" → medición: 279 hits / 63 archivos; balde A 124 hits / 27 archivos (26 con SDD congelado). El "220 archivos" del backlog era falso (contaba `.git/`).
- **L6** 2026-11-13 (owner): "El docs/SDD.md dejalo fuera y congelado. ... Ok mete lo de design en el PR 2. Dale con los 3 PRs en ese órden."
- **L7** 2026-11-13 (hallazgo de implementación): `MCP_BIND` se resuelve con la misma `firstNonEmpty` y el mismo orden; su default `127.0.0.1` **no cambia**. El pre-flight del instalador resuelve solo el puerto, no el bind.
