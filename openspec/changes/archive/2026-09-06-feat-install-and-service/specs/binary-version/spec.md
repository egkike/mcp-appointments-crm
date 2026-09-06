# binary-version Specification

> **Change**: feat-install-and-service · Fase 5
> **Domain**: binary-version (NEW — no prior canonical spec in `openspec/specs/`)
> **Reference**: D2 (decisión del proposal); `docs/deployment.md` (interfaz ya prometida); ADR-0014 (versionado via ldflags en `buildinfo.Version`); capability `install-service` REQ-INS-011 (consumidor)
> **DoD coverage**: soporta items 1 y 13 (verificación post-install y upgrade); D2

## Purpose

Cerrar el gap D2: `docs/deployment.md` promete `mcp-server --version` y no existe. Agregar el flag mínimo (~20 LOC en `cmd/mcp-server/main.go`) que imprime `buildinfo.Version` y sale 0, sin alterar ningún otro comportamiento del binario. Es interfaz distinta de `install.sh --version` (flag del instalador, capability `install-service` REQ-INS-001); ambas existen tras esta fase.

## Requirements

### REQ-BVER-001 — `mcp-server --version` imprime la versión y sale 0

El binario `mcp-server` MUST reconocer el flag `--version` como primer argumento: imprimir `buildinfo.Version` (stamped via `ldflags`, `dev` en builds sin stamp) en stdout y salir con exit code 0, sin iniciar el servidor MCP ni tocar la base de datos. La salida MUST contener la cadena de la versión de forma parseable por el instalador (ver REQ-INS-011, que la compara contra el tag instalado).

#### Scenario: Build stamped imprime la versión del release

- GIVEN un binario compilado con `-ldflags` inyectando `buildinfo.Version=v0.3.0`
- WHEN se ejecuta `mcp-server --version`
- THEN imprime `v0.3.0` en stdout y sale con código 0

#### Scenario: Build de desarrollo imprime dev

- GIVEN un binario compilado sin stamp (por ejemplo `go build ./cmd/mcp-server`)
- WHEN se ejecuta `mcp-server --version`
- THEN imprime `dev` (o el valor default de `buildinfo.Version`) y sale con código 0

#### Scenario: --version no arranca el servidor

- GIVEN cualquier entorno, con o sin DB accesible
- WHEN se ejecuta `mcp-server --version`
- THEN el proceso termina inmediatamente: no escucha en ningún puerto, no crea ni abre base de datos

### REQ-BVER-002 — Cambio aislado: cero alteración de semántica existente

La incorporación del flag MUST ser un cambio aislado en `cmd/mcp-server/main.go`: el default de DB (`./data/appointments.db`), la semántica de env vars (`MCP_DB_PATH`, `MCP_BIND`, `MCP_PORT`, precedencia ADR-0007) y el comportamiento de arranque sin `--version` MUST permanecer idénticos (D1: el path de DB de producción lo fija el service unit, no el binario). Ejecutar el binario sin argumentos MUST comportarse exactamente como antes de esta fase. El flag MUST tener su propio test (unit o e2e) en el árbol de tests Go existente.

#### Scenario: Arranque sin flags idéntico a Fase 4

- GIVEN el binario nuevo ejecutado sin argumentos con `MCP_DB_PATH` definido
- WHEN arranca
- THEN usa la DB indicada, lee `.env`/env vars con la precedencia de ADR-0007 y expone el endpoint MCP igual que antes del cambio

#### Scenario: Test propio del flag

- GIVEN el árbol de tests del repositorio tras esta fase
- WHEN se ejecuta `go test ./...`
- THEN existe al menos un test que ejercita `--version` (versión impresa + exit 0) y pasa

### REQ-BVER-003 — Interfaces `--version` del instalador y del binario son distintas

La documentación (`installation.md`, `maintenance.md`, `deployment.md`) MUST distinguir las dos interfaces de versión: `install.sh --version vX.Y.Z` selecciona el release a desplegar (tag pinned, REQ-INS-001); `mcp-server --version` reporta la versión del binario instalado (verificación post-install y de upgrade, REQ-INS-011/REQ-INS-013). Ambas MUST existir y estar documentadas tras esta fase (cierre del gap de `deployment.md` que prometía una interfaz inexistente).

#### Scenario: Ambas interfaces coexisten tras el install

- GIVEN un despliegue completado con `install.sh --version v0.3.0`
- WHEN el operador ejecuta `$BIN_DIR/mcp-server --version` y consulta los docs
- THEN el binario reporta `v0.3.0` y los docs explican la diferencia entre el flag del instalador y el del binario
