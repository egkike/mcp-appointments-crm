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

## Log

- **L1** 2026-11-13 (owner): "Tomando en cuenta que el órden habitual y recomendado o ideal es: 1- Instalar y dejar funcionando Hermes en el Server. 2- Instalar nuestro MCP y configurarlo. Cual de los caminos consideras el más conveniente para nuestro proyecto? Yo tenía entendido que nuestro MCP primero lee el puerto definido en el archivo .env y usa eso, y si no hay .env usa el 3000... es así?"
- **L2** 2026-11-13 (owner): "Lo primero: debemos arreglar y que se haga correctamente: 'env vars > .env > defaults' y poner como default 3000 o 3001 según cual sea más estandard y adecuado para nosotros. Segundo: Al crearse el archivo .env ya debemos asignar el MCP_PORT=3001."
- **L3** 2026-11-13 (owner): "Ok me parece bien los 3 niveles y el órden...1-Shell, 2-archivo .env y 3-default." → confirma que la precedencia queda como está; el defecto es documental.
- **L4** 2026-11-13 (owner): "Lo que sí ya te confirmo es que usaremos para todo el puerto 3001."
- **L5** 2026-11-13 (owner): "Ok mide el desglose y después me cuentas" → medición: 279 hits / 63 archivos; balde A 124 hits / 27 archivos (26 con SDD congelado). El "220 archivos" del backlog era falso (contaba `.git/`).
- **L6** 2026-11-13 (owner): "El docs/SDD.md dejalo fuera y congelado. ... Ok mete lo de design en el PR 2. Dale con los 3 PRs en ese órden."
- **L7** 2026-11-13 (hallazgo de implementación): `MCP_BIND` se resuelve con la misma `firstNonEmpty` y el mismo orden; su default `127.0.0.1` **no cambia**. El pre-flight del instalador resuelve solo el puerto, no el bind.
