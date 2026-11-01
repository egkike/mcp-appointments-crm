# Spec: tui-operational-residuals

> Reference: `openspec/changes/feat-tui-ops-residuals/design.md`, ADR-0010, ADR-0013, ADR-0015, ADR-0016 · Change: feat-tui-ops-residuals · Status: NEW

## Purpose

Cerrar los tres residuales operativos diferidos por el owner el 2026-09-16 (audit log persistido, normalización de day-keys, `purge-inactive`) y corregir la línea stale del PRD que los declara pendientes cuando el MVP ya fue entregado. Todo el trabajo vive en la capa de operador (TUI + núcleo `internal/admin` + persistencia), **nunca** en la superficie MCP: la gestión de cuentas está fuera del alcance del LLM por defense-in-depth (ADR-0010, ADR-0015 Decision 2, ADR-0016 Decision 1).

## Requirements

### Requirement: La auditoría de cuentas se persiste en la base de datos

El sistema MUST persistir exactamente una fila de auditoría por cada mutación de cuenta exitosa (`create`, `update`, `deactivate`, `transfer`) y por cada rechazo relevante del single-owner invariant (`second_owner_rejected`, `internal/repository/accounts.go:81`), en una tabla dedicada creada por el DDL canónico (`internal/db/schema.go`). La persistencia MUST ser aditiva e idempotente (`CREATE TABLE IF NOT EXISTS`) y MUST NOT requerir migración de datos existentes. La lectura (`list`) MUST NOT generar filas de auditoría.

#### Scenario: Cada tipo de mutación deja su fila

- GIVEN una base de datos inicializada y un owner activo
- WHEN el operador crea una cuenta, la actualiza, la desactiva y transfiere ownership
- THEN existe exactamente una fila de auditoría por cada mutación, con `action` en `create`/`update`/`deactivate`/`transfer` y el `target_id`/`target_role` de la cuenta afectada

#### Scenario: DDL aditiva sobre una base existente

- GIVEN una base de datos existente con datos de negocio y cuentas
- WHEN el servidor la abre y ejecuta `initSchema` dos veces
- THEN la tabla de auditoría existe y el estado final es idéntico, sin `ALTER`, sin pérdida de filas y sin error de idempotencia

#### Scenario: El rechazo del invariante queda auditado

- GIVEN un owner activo ya existente
- WHEN se intenta crear un segundo owner activo y el repositorio lo rechaza
- THEN se persiste una fila con `action = reject` y `event` de rechazo de owner, además del `slog` de advertencia existente

### Requirement: La auditoría persistida no contiene PII

Toda fila de auditoría MUST almacenar el teléfono enmascarado a sus últimos 4 dígitos (o `"***"` cuando el valor es demasiado corto) reutilizando la regla ya existente (`maskPhone`, `internal/application/usecase/registration_audit.go:38-46`), MUST usar una referencia opaca/hasheada del actor (nunca el caller id crudo) y MUST NOT almacenar el teléfono completo, nombres de clientes ni contenido conversacional.

#### Scenario: El teléfono nunca se persiste completo

- GIVEN una mutación sobre una cuenta cuyo id es un teléfono
- WHEN se escribe la fila de auditoría
- THEN el valor persistido termina en los últimos 4 dígitos precedidos de `***` (o es `"***"`), y el teléfono completo no aparece en ninguna columna

### Requirement: La vista de auditoría es read-only

El menú `mcp-server admin tui` MUST exponer una vista de los eventos de auditoría que MUST ser de solo lectura: no ofrece ninguna acción de escritura, no promueve eventos a mutaciones y MUST mostrar una proyección tipada mínima de cada evento. El flujo de consola sin TTY MUST ofrecer la misma vista.

#### Scenario: La vista no permite mutar

- GIVEN el operador abre la vista de auditoría
- WHEN inspecciona las acciones disponibles
- THEN solo puede leer/paginar/regresar; no existe ninguna acción que escriba, borre o modifique un evento o una cuenta

#### Scenario: Paridad de consola

- GIVEN una invocación no interactiva (sin TTY)
- WHEN el operador abre la vista de auditoría desde el menú de consola
- THEN se renderizan los mismos eventos que la vista Bubble Tea, a través del mismo núcleo `internal/admin`

### Requirement: Una única convención de day-key con un único punto de traducción

El proyecto MUST exponer un único punto de traducción entre `time.Weekday` / `schedules.day_of_week` (0..6, domingo=0) y `business_hours` (`"1".."7"`, lunes=1). Los call sites de producción MUST NOT mezclar ambas convenciones mediante `int(time.Weekday())` crudo, y la convención MUST quedar anclada por tests. Ningún valor persistido en `schedules` ni en `business_hours` MUST cambiar.

#### Scenario: Traducción completa y explícita

- GIVEN el entry point de traducción
- WHEN se traduce cada `time.Weekday`
- THEN domingo produce `7` para `business_hours` y `0` para `schedules`, lunes produce `1` en ambas, y la tabla completa `0..6` queda cubierta por un test

#### Scenario: Frontera fail-closed

- GIVEN una clave de `business_hours` fuera de `1..7` o un día ausente
- WHEN se consulta `GetOpenClose`
- THEN un día legítimamente cerrado devuelve "cerrado sin error" y una clave corrupta devuelve error (fail-closed), sin leer el horario de un día distinto

#### Scenario: Cero migración de datos

- GIVEN `schedules.day_of_week` en `0..6` y `business_hours` en `"1".."7"`
- WHEN se aplica la normalización
- THEN ningún valor almacenado cambia y el `CHECK (day_of_week BETWEEN 0 AND 6)` de `internal/db/schema.go` se mantiene

### Requirement: `purge-inactive` borra solo cuentas inactivas y refusa activas

El sistema MUST ofrecer un camino de borrado físico de cuentas que solo elimine filas con `is_active=0`, MUST refusar el owner activo y MUST NOT tocar ninguna cuenta activa, profesional, servicio ni reserva. La operación MUST ser idempotente: si no quedan cuentas inactivas, el resultado MUST ser un outcome en español, no un error.

#### Scenario: Se refusa un owner activo

- GIVEN un owner activo
- WHEN el operador intenta purgarlo
- THEN la operación no borra nada y devuelve un mensaje semántico en español indicando que no se puede purgar una cuenta activa

#### Scenario: Purga de una cuenta inactiva

- GIVEN una cuenta `staff` con `is_active=0`
- WHEN el operador confirma la purga
- THEN la fila desaparece de `accounts` y las cuentas activas, profesionales, servicios y reservas quedan intactos

#### Scenario: Idempotencia

- GIVEN un conjunto de cuentas sin ninguna fila inactiva
- WHEN el operador ejecuta la purga
- THEN el resultado es "no hay cuentas inactivas para purgar" y no hay error

### Requirement: `purge-inactive` exige confirmación extra en ambos flujos

La purga MUST requerir un paso adicional de confirmación más allá del sí/no estándar, tanto en el flujo Bubble Tea (TTY) como en el flujo de consola (sin TTY). Un input que no sea la confirmación fuerte MUST re-preguntarse y MUST NOT interpretarse como consentimiento. La operación MUST NOT ejecutarse sin la confirmación fuerte.

#### Scenario: La confirmación débil no alcanza (TTY)

- GIVEN el operador llegó al paso de purga en el TUI
- WHEN responde la confirmación estándar (p. ej. `s`) en lugar del token fuerte requerido
- THEN la purga no se ejecuta y el flujo re-pregunta

#### Scenario: Confirmación fuerte en el flujo de consola

- GIVEN una ejecución sin TTY
- WHEN el operador provee el token de confirmación fuerte
- THEN la purga se ejecuta y reporta el conteo de filas borradas; si el token no coincide, no se borra nada

#### Scenario: Cancelación explícita

- GIVEN el paso de confirmación de purga
- WHEN el operador cancela
- THEN no se borra ninguna fila y el mensaje de cancelación se muestra en español

### Requirement: El borrado de cuentas es FK-safe

La purga MUST ser segura respecto de las foreign keys: ninguna tabla referencia `accounts` y `accounts.professional_id` no declara FK (`internal/db/schema.go:123-129`), por lo que borrar filas de `accounts` MUST NOT violar un `ON DELETE RESTRICT` ni cascadear a `professionals`, `services`, `bookings` o `schedules` (`internal/db/schema.go:96-98`).

#### Scenario: Booking y profesional sobreviven a la purga

- GIVEN un profesional con reservas y una cuenta `staff` inactiva vinculada por `professional_id`
- WHEN se purga la cuenta inactiva
- THEN el profesional y sus reservas siguen existiendo y ninguna FK queda violada

### Requirement: `purge-inactive` no es alcanzable por el LLM

La purga y la vista de auditoría MUST NOT ser tools MCP ni modificar la superficie o el RBAC de tools existentes. El acceso MUST quedar limitado al operador con shell en la VPS (sub-comando del binario), coherente con ADR-0010, ADR-0015 Decision 2 y ADR-0016.

#### Scenario: La superficie MCP no cambia

- GIVEN el change aplicado
- WHEN se inspecciona `tools/list` y el mapa `ToolRBAC` del composition root
- THEN no hay tools nuevos ni cambia el gating de ningún tool existente

### Requirement: La línea stale del PRD refleja el MVP entregado y los residuales

`docs/PRD.md:864` MUST reescribirse para declarar el MVP de la TUI operacional como entregado (fecha y ADR-0016) y enumerar los tres residuales trackeados por este change. Ninguna otra línea de `docs/PRD.md` MUST modificarse.

#### Scenario: La línea deja de contradecir el estado real

- GIVEN `docs/PRD.md:864` declara el MVP como pendiente y `docs/PRD.md:1343` lo declara entregado
- WHEN se reescribe la línea del punto 8
- THEN la línea indica "MVP ENTREGADO (2026-09-16, ADR-0016)" y referencia los residuales (audit persistido, day-keys, `purge-inactive`), y el diff del archivo contiene exactamente una línea modificada
