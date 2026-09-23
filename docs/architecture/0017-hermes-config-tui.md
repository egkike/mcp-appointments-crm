# ADR-0017: TUI "configurar Hermes" (bootstrap del config de Hermes)

- **Status**: accepted
- **Date**: 2026-09-24
- **Authors**: Kike
- **Related**: ADR-0016 (alcance admin-tui), ADR-0009 (modelo de autorización), PRD §7 Fase N

## Context

El smoke de instalación fresca de v0.4.0 (2026-09-18) dejó un hallazgo operativo: un install
nuevo requiere **editar YAML a mano** en la VM para que Hermes pueda hablar con el MCP server
(`docs/demo-plan.md`, Paso 5). El owner del negocio — no técnico — no puede hacer eso
(riesgo R5 del PRD: *"El dueño del negocio no sabe cómo configurar Hermes ni apuntarlo al
MCP server"*, probabilidad Alta / impacto Alto).

La forma soportada hoy es escribir en `~/.hermes/config.yaml`:

```yaml
mcp_servers:
  mcp-appointments:
    url: http://127.0.0.1:3000/mcp
    headers:
      X-Caller-Id: <teléfono-del-owner>
```

La TUI admin (`mcp-server admin tui`) ya es la puerta de identidad del sistema, pero
[ADR-0016](./0016-admin-tui-scope.md) acota su alcance a **identidad y cuentas**; una
capacidad de configuración de Hermes es un alcance nuevo que ADR-0016 explícitamente no
cubre.

## Decision

**La TUI admin gana una opción "configurar Hermes" que bootstrap-nea el config de Hermes
del host, sin YAML manual.**

### Decision 1: salida doble (write con merge + snippet de fallback)

- **Primario**: la TUI lee `~/.hermes/config.yaml` (si existe), hace merge de la entrada
  `mcp_servers.mcp-appointments` preservando todo lo demás (otras servers, otras claves) y
  escribe atómicamente.
- **Fallback**: si el write falla o `~/.hermes` no existe/no es escribible, la TUI muestra
  en pantalla el bloque YAML exacto para copiar a mano.
- Re-ejecutar el flujo es idempotente: actualiza la misma entrada, no duplica.

### Decision 2: contrato de formato del header

```yaml
headers:
  X-Caller-Id: "+5491100000000"
```

El valor **siempre entre comillas y con `+`** (E.164 subset del proyecto). Es exactamente
lo que Hermes aceptó en el smoke de v0.4.0. Rationale técnica adicional: YAML 1.1 parsea
`+5491100000000` sin comillas como **entero** (regex de int `[-+]?[0-9]+`); las comillas
son obligatorias para que cualquier parser YAML lo lea como string. La validación del
teléfono reutiliza el único punto existente (`admin.ValidatePhone`), nunca una copia.

### Decision 3: fuente de datos

- **Teléfono**: prefill desde el owner activo (`accounts`), editable por el operador con
  validación en cada input. Sin owner activo el flujo no aplica (gate de seed anterior).
- **URL del endpoint**: derivada de `MCP_BIND`/`MCP_PORT` (misma configuración del server),
  nunca hardcodeada en el core; path fijo `/mcp`.
- **Nombre de la entrada**: `mcp-appointments` (único nombre usado en la documentación).

### Decision 4: dependencia YAML

Se agrega `gopkg.in/yaml.v3`. El merge seguro de YAML existente exige un parser real;
serializar a mano rompería configs de Hermes que no controlamos.

## Non-goals

- **`hermes chat`**: sigue siendo el stub reservado de `main.go` (implementación futura).
- **Bot WhatsApp per-sender**: Fase N independiente (PRD §3.8.9).
- **Detección/instalación de Hermes**: se asume `~/.hermes` como ubicación estándar; si no
  existe, el fallback de snippet cubre el caso.
- **Editar cualquier otro campo del config de Hermes**: la TUI solo escribe la entrada
  `mcp_servers.mcp-appointments`.
- **Superficie MCP nueva**: no se agregan ni consumen tools.

## Consecuencias

- El dueño deja de editar YAML a mano en la VM: la misma TUI que siembra el owner deja el
  sistema completo operativo (identidad + Hermes apuntado).
- El gate de verificación corre por ruteo default (superficie de escritura nueva +
  dependencia nueva en `go.mod`) → review nativo.
- `go.mod` incorpora la primera librería YAML del repo; queda en la superficie de review.
