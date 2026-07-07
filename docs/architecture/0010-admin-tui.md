# ADR-0010: TUI menú operacional para gestión de cuentas admin/staff

- **Status**: accepted
- **Date**: 2026-06-29
- **Authors**: Kike
- **Supersedes**: ADR-0008 partial (only the TUI part; inline prompts for setup remain)

## Context

El modelo de autorización (ADR-0009 + PRD §3.8) introduce la tabla `accounts` con roles `owner`/`admin`/`staff`. El primer `owner` se crea vía TUI menú (seed en Fase 2). Después del setup inicial, el admin necesita poder:

- Agregar staff (alta de cuentas con `role='staff'`, `professional_id` válido)
- Desactivar cuentas (soft delete: `is_active=0`)
- Transferir ownership (desactivar owner actual, crear nuevo owner)
- Listar cuentas (read-only views)

La pregunta es: **¿quién hace estas operaciones y con qué herramienta?**

**Opciones que descartamos antes:**

1. **LLM (Hermes) hace admin CRUD via MCP tools**: el LLM es responsable de declarar su rol en cada request. **Descartado** porque un LLM comprometido puede escalar privilegios (defense-in-depth débil). El PRD §3.8.5 ya documenta que el LLM no puede falsificar `X-Caller-Id`, pero puede actuar como admin si conoce un phone whitelisted.

2. **CLI de bash con sub-comandos** (`mcp-appointments-crm admin create <phone>`, etc.): simple, sin dependencias externas, consistente con ADR-0005 (no external runtime tools). **Considerado** pero rechazado: bash es menos type-safe; el admin tiene que recordar los sub-comandos correctos; el feedback es menos amigable que un TUI.

3. **Dashboard web local** (servidor HTTP en otro puerto, ej. `127.0.0.1:3001` con HTML form): amigable, separado del MCP server. **Descartado** porque es un proyecto grande (Fase 6+). El TUI es suficiente para Fase 2.

4. **TUI menú en binario Go independiente** (rechazado en ADR-0008 para setup): la justificación de ADR-0008 fue que un TUI para **setup one-time** es over-engineering. Pero la justificación cambia cuando el TUI es **reusable** (setup + ops futuras): el costo de implementación (~300-500 LOC) se amortiza con el uso operacional continuo. **ADOPTADO**.

## Decision

**Introducir un sub-comando TUI menú en el binario principal**: `mcp-appointments-crm admin tui`. Es el mismo binario (no un binario separado), corre en la VPS como sub-comando, no es invocable por el LLM.

### Capacidades del TUI

- **Alta de staff** (`Add Staff`): el owner/admin ingresa `phone`, `display_name`, `professional_id`. El TUI valida formato (regex de phone), consulta al repo, muestra el resultado.
- **Desactivar cuenta** (`Deactivate`): muestra las cuentas activas, el owner/admin selecciona una, confirma, el TUI llama al repo.
- **Transferir ownership** (`Transfer Ownership`): el owner actual se desactiva (`Deactivate`), después se crea un nuevo owner. El TUI maneja el flujo de 2 pasos.
- **Listar cuentas** (`List All`, `List Owners`, `List Admins`, `List Staff`): read-only views.
- **Audit log view** (opcional): muestra los logs recientes de cambios de cuentas (del `*slog.Logger`).

### Stack

- **Lenguaje**: Go (mismo binario que el MCP server, sin external runtime tools per ADR-0005).
- **Librería TUI**: [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Go puro, sin CGo, ~1.5MB al binario). **No es external runtime tool** — es una librería Go que compila dentro del binario.
- **Entry point**: `cmd/mcp-server/admin_tui.go` (sub-comando del binario principal `mcp-appointments-crm`). El sub-comando se activa con `mcp-appointments-crm admin tui`. **No es un binario separado** — el TUI vive en el mismo proceso que el MCP server; comparte el `*slog.Logger`, el `*sql.DB`, y el `*slog.Logger` para audit log.

### Enforcement en el TUI

- **El admin del OS** (SSH a la VPS) es el gatekeeper primario. El TUI es "internal admin tooling", no expone nada al exterior. Sin passphrase explícita en esta versión (se puede agregar como opcional en Fase 2+).
- **El LLM NO puede invocar el TUI**: el TUI es un sub-comando del binario, no un tool MCP. El LLM (Hermes) solo ve los tools MCP que el middleware HTTP expone. Defense-in-depth: aunque el LLM esté comprometido, no puede escalar privilegios vía el TUI.

### Single-owner invariant enforcement (complementa ADR-0009)

- El TUI NO permite crear un segundo `owner` activo (el repo enforce via `SELECT COUNT(*)` y el trigger SQLite rechazan).
- El TUI muestra un mensaje claro si el admin intenta transferir ownership mientras hay un owner desactivado: "ya hay un owner desactivado; reactívalo primero o purga la cuenta antes de crear otra".

## Consequences

**Positive**:
- Defense-in-depth real: el LLM no puede escalar privilegios (admin ops están fuera del MCP).
- Mismo binario para setup (futuro) + ops (Fase 2+) + MCP server. No hay binarios separados que mantener.
- Reusability amortiza el costo de Bubble Tea: el TUI no es solo para setup, sirve para ops futuras.
- Type-safe Go validation (vs bash regex) en operaciones críticas (admin create).
- Audit log estructurado (vía `*slog.Logger` en el repo) que el TUI puede consultar.

**Negative**:
- Bubble Tea suma una dep interna (~1.5MB al binario). Es Go puro, no external runtime tool, no viola ADR-0005.
- Reversibilidad del ADR-0008: este ADR-0010 NO revierte la decisión de inline prompts para **setup one-time** (eso sigue siendo bash). Solo agrega el TUI como **herramienta operacional reusable** (Fase 2+). ADR-0008 sigue siendo válido para su scope.
- El TUI NO resuelve el caso de "LLM que quiere hacer admin ops" (el LLM nunca podrá, por diseño). Si el admin quiere delegar una operación admin al LLM (raro, no recomendado), debe hacerlo via TUI manualmente.

**Rejected alternatives** (re-iteradas para claridad):
- (a) LLM hace admin CRUD: defense-in-depth débil. **Rechazado**.
- (b) CLI de bash: menos type-safe, menos amigable. **Rechazado** (puede ser Fase 2+ si el TUI es over-engineering para el MVP).
- (c) Dashboard web: proyecto grande (Fase 6+). **Rechazado**.

## Reversibility

- Si el TUI resulta over-engineering o poco usado, se puede revertir: el binario queda como `mcp-appointments-crm` (MCP server + admin TUI como sub-comando), y el admin puede usar el repo directamente desde un script bash con `sqlite3` CLI (Fase 2+ ya no necesita el TUI).
- Si en algún momento se quiere migrar a un dashboard web, el `AccountsRepo` no cambia; solo se reemplaza el TUI por un HTTP server que use el mismo repo.

## Implementation order

1. **`feat-authorization` PR 1** (data layer, en curso): incluye schema, model, repo con `Deactivate` + audit log. El TUI es Fase 2+.
2. **`feat-authorization` PR 2** (auth primitives, en curso): incluye `Caller` + `Resolver` + `Middleware`. El TUI no usa el middleware (es otro proceso), pero comparte el `Caller` struct.
3. **Fase 2+**: implementar el TUI menú como sub-comando del binario. Diseño y desarrollo posterior.

## References

- `docs/PRD.md` §3.8 (Modelo de Autorización) + §3.8.8 (TUI menú operacional)
- `docs/architecture/0009-authorization-model.md` (ADR-0009, contexto y consecuencias)
- `openspec/changes/feat-authorization/` (5 SDD artifacts)
- `openspec/changes/feat-authorization/tasks.md` (Future work: TUI menú)
- ADR-0005 (no external runtime tools; Bubble Tea es Go puro, no viola)
- ADR-0008 (inline prompts for setup, sigue válido para su scope; este ADR-0010 NO lo revierte, solo agrega una herramienta operacional)
