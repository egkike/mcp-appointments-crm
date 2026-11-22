# find_available_slots — búsqueda de turnos por rango (T2 de agent surface)

Feature: agregar la tool MCP `find_available_slots` para responder "¿qué turnos hay libres entre estas fechas?" en una sola llamada, reutilizando la regla de disponibilidad existente sin duplicarla. Cierra el read-gap que hoy obliga al llamador a iterar `check_availability` slot por slot.

Issue: **#241** (`type:feature` + `status:approved`). Origen y descarte de alternativas: `odd/tasks/feat-agent-surface-proposal.md` (T2).

## Specs

- **S1** (la tool existe y con qué forma) — MUST existir `find_available_slots` con esta entrada y salida:
  ```
  find_available_slots(
    service_id:        string   requerido
    professional_id?:  string   opcional
    from:              string   requerido, YYYY-MM-DD, inclusivo
    to:                string   requerido, YYYY-MM-DD, inclusivo
    limit?:            int      default 10, máximo 50
  ) → { slots: [{start_datetime, professional_id, professional_name}],
        total_candidates: int, truncated: bool }
  ```
  El owner: *"Mañana arrancaremos por T2 primero, luego T1"* (L9 del doc padre).
- **S2** (rol) — MUST ser **`any authenticated`**, sin entrada en `ToolRBAC`, molde exacto de `check_availability`. El owner: *"el rol consumidor de find_available_slots — será any authenticated como recomiendas"*.
- **S3** (reutilizar, no duplicar) — MUST evaluar cada slot candidato a través de `service.ValidateBookingTimeSlot` (`internal/domain/service/booking_time_validator.go`), inyectándole un `BookingOverlapReader` respaldado por los bookings del rango **ya leídos en memoria**. MUST NOT reimplementar el chequeo de horario, excepción, agenda, ventana ni solapamiento. MUST NOT modificar `CheckAvailability` ni `internal/domain/service/availability_test.go` (su `Regression Gate` exige los 16 subtests sin cambios).
- **S4** (semántica de filtro) — `find_available_slots` **filtra**, no reporta. Un `*domain.SemanticError` devuelto por `ValidateBookingTimeSlot` MUST interpretarse como "este slot no está disponible" y MUST NOT propagarse al cliente. Distinto de `check_availability`, que sí reporta el motivo.
- **S5** (PII) — La respuesta MUST NOT exponer identidad de clientes con reservas existentes. Solo `start_datetime`, `professional_id` y `professional_name`.
- **S6** (cotas) — `limit` default 10, máximo 50. El rango `to - from` MUST rechazarse si supera **14 días** (error semántico, no lista vacía). `limit` fuera de rango MUST rechazarse como error semántico. `truncated` MUST ser `true` si y solo si hubo más slots válidos que el `limit` aplicado.
- **S7** (granularidad, decisión de diseño) — Los slots candidatos se generan con paso igual a la **duración del servicio** (`entity.Service.Duration`), desde la apertura hasta que el slot completo entre en la ventana. No se introduce un paso arbitrario ni un parámetro nuevo.
- **S8** (orden determinístico) — Los slots MUST devolverse ordenados por `start_datetime` ascendente y, ante empate, por `professional_name`. `total_candidates` MUST contar los slots válidos **antes** de aplicar `limit`.
- **S9** (una sola lectura de bookings) — El rango completo MUST leerse con **una** consulta de bookings (prepared statement, `?`). MUST NOT consultarse por slot ni por profesional dentro del loop. Verificable con un spy que cuente invocaciones.
- **S10** (capas, ADR-0013) — Puerto de lectura en `internal/domain/repository`; orquestación en `internal/application/usecase/find_available_slots.go` (+ `dto`); tool en `internal/mcp`; wiring y `ToolRBAC` en `cmd/mcp-server`. `internal/domain` no importa capas externas.
- **S11** (casos de borde, comportamiento fijado) —
  - `service_id` inexistente o inactivo → error semántico (problema de input, se propaga).
  - `professional_id` informado e inexistente/inactivo → error semántico (se propaga, igual que `check_availability`).
  - `professional_id` omitido → candidatos = profesionales **activos** que ofrecen ese servicio. Los inactivos se excluyen en silencio.
  - Sin turnos en el rango → `{slots: [], total_candidates: 0, truncated: false}`, **no** error.
  - Excepción de horario `is_closed=true` en una fecha → esa fecha no produce slots; con ventana propia → se usa esa ventana.
- **S12** (hogar del spec) — El contrato normativo va a `openspec/specs/availability/spec.md` como sección **ADDED** (sin reescribir el header `DELTA` existente, que referencia `feat-booking-validator-service`) y la fila nueva en `openspec/specs/mcp-transport/spec.md`. MUST NOT crearse `openspec/changes/<change>/`: el workflow SDD está retirado (L11 del doc padre).
- **S13** (no-goals) — No autoriza T1 (`tools/list` por rol). No cambia `check_availability`, `create_booking` ni el RBAC existente.

## Tasks

Una unidad de trabajo (un commit, ~350–450 líneas con tests). Rama `feat/find-available-slots`.

- **T-1** · S9,S10 · puerto + adaptador
  - `internal/domain/repository/bookings.go`: método de lectura por rango (una query, prepared statement). Verificar si ya existe algo equivalente antes de agregarlo.
  - `internal/repository/bookings.go`: adaptador. Devolver `ErrNotFound`-equivalente solo si el contrato del repo lo exige; lista vacía es lista vacía.
- **T-2** · S3,S4,S6,S7,S8,S11 · use case + dto
  - `internal/application/usecase/find_available_slots.go`: resolver servicio/perfil; resolver candidatos (profesional único o lista de activos por especialidad); leer bookings del rango **una vez**; leer excepciones del rango; para cada slot candidato, `ValidateBookingTimeSlot` con un `BookingOverlapReader` en memoria; filtrar errores; ordenar; contar; aplicar `limit`.
  - `internal/application/dto`: carrier de entrada/salida con doc comments cross-layer.
- **T-3** · S1,S2,S5 · tool + wiring
  - `internal/mcp/tools_booking.go` (o archivo propio): struct `*In` con tags JSON, handler, `auth.RequireCaller`.
  - `cmd/mcp-server/main.go`: wiring del use case. **Sin** entrada en `ToolRBAC` (S2).
  - `internal/mcp/test_helpers_test.go`: `expectedToolCount` 25 → **26**.
- **T-4** · S12 · spec y contrato
  - `openspec/specs/availability/spec.md`: sección ADDED.
  - `openspec/specs/mcp-transport/spec.md`: fila del contrato.

## Trampas verificadas (no repetir)

1. **`expectedToolCount = 25`** en `internal/mcp/test_helpers_test.go` es la fuente única y tiene **17 referencias** en el suite. Subir el literal a 26 y no tocar los asserts.
2. **El SDK ordena `tools/list` alfabéticamente.** Los tests existentes comparan por conjunto; un test nuevo que compare índices va a fallar.
3. **`availability_test.go` no se toca.** Los 16 subtests son el `Regression Gate` de `openspec/specs/availability`. Si el refactor obliga a cambiarlos, el diseño está mal.
4. **Validar el wall clock local del negocio, no el instante con offset.** `CheckAvailability` ya lo hace (`availability.go`: `localStart := startTime.In(loc)` con el comentario `B8 f3`). El candidato debe construirse igual, con `time.Date(..., loc)`.
5. **El paso de la iteración es la duración del servicio** (S7), no 15/30 minutos fijos. Un slot que empieza a las 14:10 con servicio de 40 min debe poder existir si la ventana lo permite.
6. **Solapamiento en memoria:** el `BookingOverlapReader` de T-2 debe respetar la misma semántica que `BookingsRepo.FindOverlapping` (comparación de intervalos y exclusión de reservas canceladas). Si la semántica difiere, el resultado cambia sin que ningún test lo note.
7. **Excepciones por fecha, no por día de semana.** `BusinessHoursExceptions.Get(ctx, date)` es por fecha concreta: hay que resolver **una por cada día** del rango.
8. **Rango inclusivo en ambos extremos** (`from` y `to`), y el máximo de 14 días se valida sobre esa ventana inclusiva.

## Verificación

- `go fmt ./...`, `go vet ./...`, `golangci-lint run ./...`, `go build -o /dev/null ./...`, `go test -v -race ./...`.
- Tests obligatorios por capa: (a) integración HTTP `tools/list` cuenta 26 y `tools/call` de la tool nueva devuelve slots; (b) use case: rango vacío, excepción cerrada, solapamiento, límite y `truncated`, rango > 14 días rechazado, profesional inactivo omitido vs. error; (c) repo: una sola query del rango.
- Cada test que cubre un caso de borde de S11 MUST afirmar el efecto observable completo: contenido exacto de `slots`, `total_candidates`, `truncated`, y que un input rechazado no altera datos ni contadores.

## Log

- **L1** (2026-11-21, owner, verbatim): *"Hola capo, arranquemos el T2 si estamos en condiciones"* → se verificaron las condiciones: decisiones cerradas (orden T2→T1, S5 `any authenticated`, spec home C), `main` en `f1f2d49` y árbol limpio.
- **L2** (hallazgo al abrir el ciclo): **S6 del doc padre ya estaba cumplida** — `ValidateBookingTimeSlot` existe (`internal/domain/service/booking_time_validator.go:85`) y `CheckAvailability` ya delega en él. No hay extracción pendiente: T2 **compone** vía `BookingOverlapReader` en vez de refactorizar. Esto reemplaza la condición "se extrae el helper o no se hace".
- **L3** (diseño fijado por el agente, corregible): la trampa del loop naive es el N+1 — `ValidateBookingTimeSlot` hace una lectura de solapamiento por llamada. Por eso S9 exige una sola lectura del rango y un lector en memoria; sin eso, reutilizar la cadena sería peor que no usarla.
- **L4** (diseño fijado por el agente, corregible): granularidad de slots = duración del servicio (S7). Es la definición natural y evita inventar un parámetro.
- **L5** (owner, verbatim): *"Aprobado: issue, spec y writer"* → camino completo autorizado. Issue **#241** creado y verificado (`status:approved` + `type:feature`).
