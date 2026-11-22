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
  - **Limitación documentada (scoping de staff)**: la lectura de agenda es `FindByProfessionalAndDay` (la misma que usa `CheckAvailability`), porque `SchedulesRepo.FindByProfessional` es **owner-only en el adaptador** y consumirla rompería S2. El adaptador de día reescribe el id del profesional para un caller staff, así que un caller `staff` con `professional_id` omitido ve **solo su propia agenda**; los demás profesionales simplemente no producen slots. El use case lo hace **fail-closed**: descarta las filas cuyo `ProfessionalID` no coincide con el solicitado. Ampliar el scoping exige tocar `SchedulesRepo.FindByProfessional`, que está fuera de alcance.
  - **Rango enteramente pasado (invariante de eficiencia, sin cambio de contrato)**: `rangeStart` se clampea a hoy en la zona del negocio antes de cualquier lectura. Los días pasados no pueden producir slots —la cadena rechaza todo candidato pasado en su paso 1— así que la respuesta es **idéntica** y solo se evita el trabajo. MUST NOT convertirse en un error: la firma y la semántica de respuesta no cambian.
  - **Corrupción de horas en una excepción de horario**: si una excepción **abierta** declara horas malformadas o invertidas, el use case MUST devolver un error interno (`ErrCodeInternal`), nunca un día silenciosamente sin slots. Es la misma postura que el horario semanal declara en `businessWindow`.
- **S12** (hogar del spec) — El contrato normativo va a `openspec/specs/availability/spec.md` como sección **ADDED** (sin reescribir el header `DELTA` existente, que referencia `feat-booking-validator-service`) y la fila nueva en `openspec/specs/mcp-transport/spec.md`. MUST NOT crearse `openspec/changes/<change>/`: el workflow SDD está retirado (L11 del doc padre).
- **S13** (no-goals) — No autoriza T1 (`tools/list` por rol). No cambia `check_availability`, `create_booking` ni el RBAC existente.

## Tasks

Una unidad de trabajo (un commit, ~350–450 líneas con tests). Rama `feat/find-available-slots`.

- **T-1** · S9,S10 · lectura por rango (**sin código nuevo de puerto**)
  - Verificado: `BookingsRepo.ListBookingsForRange(ctx, start, end)` **ya existe** (`internal/domain/repository/bookings.go:38-39`) con el mismo predicado que `FindOverlapping` (`status != 'cancelled' AND start_datetime < ? AND end_datetime > ?`) y el mismo `applyAuthFilter`. **No se agrega método de puerto ni migración.**
  - Solo verificar que el adaptador devuelve lista vacía (no error) para un rango sin reservas.
- **T-2** · S3,S4,S6,S7,S8,S11 · use case + dto
  - `internal/application/usecase/find_available_slots.go`: resolver servicio/perfil; resolver candidatos (profesional único o lista de activos por especialidad); leer bookings del rango **una vez**; leer excepciones del rango; para cada slot candidato, `ValidateBookingTimeSlot` con un `BookingOverlapReader` en memoria; filtrar errores; ordenar; contar; aplicar `limit`.
  - `internal/application/dto`: carrier de entrada/salida con doc comments cross-layer.
- **T-3** · S1,S2,S5 · tool + wiring + fixtures
  - `internal/mcp/ports.go`: declarar `FindAvailableSlotsPort` (convención de puerto del consumidor ya establecida ahí).
  - `internal/mcp/config.go`: campo `FindAvailableSlots FindAvailableSlotsPort` en `Config` (patrón nil-port-omite-tool).
  - `internal/mcp/server.go`: llamar al registrar nuevo desde `registerTools`.
  - `internal/mcp/tools_slots.go` (nuevo): struct `*In` con tags JSON, handler, `auth.RequireCaller`.
  - `cmd/mcp-server/main.go`: wiring del use case. **Sin** entrada en `ToolRBAC` (S2).
  - `internal/mcp/test_helpers_test.go`: `expectedToolCount` 25 → **26**.
  - `internal/mcp/tools_test.go`: mock port + `newMockPorts` + `configFromMockPorts` (sin esto `TestServerToolCountTracksRegisteredTools` falla 25≠26).
  - `internal/mcp/server_integration_test.go`: cablear el use case en `newIntegrationMuxWithOptions` (sin esto fallan las 6 aserciones de conteo del suite).
- **T-4** · S12 · spec y contrato
  - `openspec/specs/availability/spec.md`: sección ADDED.
  - `openspec/specs/mcp-transport/spec.md`: fila del contrato.

## Trampas verificadas (no repetir)

1. **`expectedToolCount` NO es la fuente única** (suposición del agente, **corregida durante la implementación**). Hay **dos literales propios** fuera de `test_helpers_test.go`: `internal/mcp/tools_hours_exception_test.go` (2 asserts con `25`) y `cmd/mcp-server/schema_contract_test.go` (`const wiredToolCount = 25`). Ambos son test-only y el fix es mecánico: rutear el primero a `expectedToolCount` y subir la constante del segundo a 26.
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
- **L6** (defecto de routing del agente, corregido): el primer despacho al worker traía una superficie de edición **defectuosa** — omitía `internal/mcp/config.go`, `server.go`, `tools_test.go` y `server_integration_test.go`, que son exactamente donde vive el patrón Config-port + `registerTools` y las fixtures que sostienen las aserciones de conteo. El worker **bloqueó en vez de adivinar**, correctamente. Causa nombrada: superficie incompleta, no falta de capacidad. Corregido por opción **A** (patrón idiomático, sin tocar archivos congelados).
- **L7** (hallazgo del worker, verificado por el orquestador): **T-1 no requiere código de puerto**. `BookingsRepo.ListBookingsForRange` ya existe (`internal/domain/repository/bookings.go:38-39`) con el mismo predicado y el mismo `applyAuthFilter` que `FindOverlapping`, así que el lector en memoria puede igualar la semántica sin agregar nada. Reduce T-1 a verificación del adaptador.
- **L8** (interpretaciones del worker, aceptadas): (1) grid anclado en `max(apertura negocio, inicio turno profesional)` y acotado por `min(cierre negocio, fin turno)`, paso = duración del servicio, con `ValidateBookingTimeSlot` decidiendo cada candidato — el generador solo acota la iteración, nunca reemplaza un chequeo; (2) `hhmmToMinutes` no es exportado en el paquete congelado, así que el use case parsea HH:MM localmente con `time.Parse("15:04")`, manteniendo `entity.ValidHHMM` como regla única de formato.
- **L9** (review nativo del slice de spec): lineage `review-af5b0e7fea8cc993`, lente `readability`, **APPROVED**, autoridad quemada. Dos advisory no bloqueantes, ambos reales y corregidos en el commit siguiente: el heading de REQ-MT-005 seguía diciendo 25 y la nota histórica no explicaba el salto 25→26.
- **L10** (defecto de diseño encontrado por los tests del writer, no por razonamiento): `SchedulesRepo.FindByProfessional` es **owner-only en el adaptador**. Consumirla habría hecho que `find_available_slots` devolviera `FORBIDDEN` a admin/staff/client, **rompiendo S2 directamente**. Fix: usar `FindByProfessionalAndDay` (la misma lectura que `CheckAvailability`) con caché por `(profesional, día de semana)`. Es el tipo de defecto que el spec no podía atrapar porque el RBAC vive en el adaptador, no en la firma del puerto.
- **L11** (riesgo de fuga detectado y cerrado): el fix anterior expuso que la lectura de día **reescribe el id del profesional** para un caller staff. Se cierra descartando las filas cuyo `ProfessionalID` no coincide con el solicitado, con test dedicado (`TestFindAvailableSlotsDiscardsAStaffScopedSchedule`). La limitación de scoping que queda se documenta en S11.
- **L12** (defecto de routing del agente, segundo): la trampa #1 del doc afirmaba que `expectedToolCount` era la fuente única con 17 referencias. **Es falso**: `tools_hours_exception_test.go` y `cmd/mcp-server/schema_contract_test.go` tienen literales propios. El worker los detectó y bloqueó en vez de salirse de su superficie. Corregida la trampa y ampliada la superficie solo a esos dos archivos de test.
- **L13** (owner, decisión): ante ~2.355 líneas authored para un solo comportamiento, el owner eligió **reorganizar antes de commitear** en vez de aceptar un PR de ese tamaño o partirlo en PRs encadenados. Split aplicado, sin cambio de comportamiento: producción en 4 archivos (`find_available_slots.go` 373 · `_agenda.go` 160 · `_grid.go` 94 · `_reader.go` 52) y tests en 4 (`_test.go` 516 · `_agenda_test.go` 60 · `_grid_test.go` 95 · `_helpers_test.go` 262).
- **L14** (prueba del split): **no se aceptó por inspección visual**. El orquestador fijó la línea base antes de delegar — **38** `=== RUN` para `TestFindAvailableSlots*` y **326** para el paquete `usecase` — y el resultado los preserva exactos. El worker probó además el movimiento por multiset de líneas no vacías y no-import: tests con delta cero, producción idéntica salvo `slotsClockLayout`, que necesitó su propio `const` al salir del bloque compartido (mismo valor `"15:04"`, mismo comentario). Única desviación conceptual del split, declarada.
- **L15** (deuda declarada del split): `indexExceptionsByDate`, `businessWindow`, `parseSlotWindow` y `clockMinutes` **no tienen test directo**; se cubren a través de `Execute`. Agregarlos habría movido el invariante 38/326, así que no se agregaron en este movimiento. Queda como candidato a follow-up, no como defecto.
- **L16** (corte de entrega, registro obligatorio): el ciclo suma **2.613 líneas authored** desde `f1f2d49`, **6,5× el corte de ~400** por unidad de review. Forma elegida: **un solo PR con lectura guiada** — el owner rechazó los dos PRs encadenados y eligió reorganizar primero (L13). Los tres commits son las unidades de review, en orden de capas: `7567d59` spec · `7fb28aa` aplicación (dto + use case + tests) · `668e5b4` transporte (port, config, tool, wiring, fixtures). Lectura recomendada al revisor: `7fb28aa` primero entero (es donde está la lógica), `668e5b4` después (es mecánico salvo el `Config` field y la ausencia deliberada de `ToolRBAC`).
- **L17** (deuda de estimación, del agente): la propuesta estimó **300–450 líneas**; el resultado fue **2.613**. La causa no es el writer: es que la estimación contó el código y no la matriz de tests — 11 reglas de spec con sus bordes cuestan ~1.500 líneas de tests. Para futuras propuestas de esta forma, estimar **por regla** (~200 líneas/regla con matriz) en vez de por intuición.
- **L18** (owner, decisión): *"Ahora solo limpiemos todos los Advisories de la review primero"* → issue **#244** (`type:refactor` + `status:approved`) sobre rama `refactor/slots-advisories`. Los cinco advisories de `review-7622a1055addc683` se resuelven en un solo PR. Dos decisiones de diseño tomadas acá: (a) `R3-past-day-scan` se resuelve **clampeando** `rangeStart` a hoy, **no** rechazando — el reviewer sugería rechazo, pero eso cambiaría el contrato observable (hoy un rango viejo devuelve lista vacía) y rompería el caso "desde hoy"; el clamp da el mismo ahorro con respuesta idéntica; (b) `R3-silent-exception-window` pasa de silencio a error interno, alineado con la postura que el comentario vecino de `businessWindow` ya declara.
- **L19** (owner, decisión): *"Clock seam primero"* — antes de mergear #245 se cierra el riesgo de flake que el agente había declarado en el body del PR. El clamp leía `time.Now()` sin costura, así que un test que deriva "hoy" y un `Execute` que lo reevalúa podían discrepar cruzando medianoche del negocio (~1e-5 por corrida; **exposición nueva**, no heredada: el test preexistente de slots pasados usa fecha fija). Fix: se replica el patrón que el repo ya tiene (`Clock` + `systemClock{}` + `SetClock`, molde de `get_pending_alerts.go`), **sin cambiar la firma del constructor** para no tocar los fixtures de `internal/mcp`. El helper `slotsPinToday` fija el reloj al **mediodía del mismo día que el test derivó**, no a una fecha arbitraria: así desaparece la carrera sin cambiar día de semana ni aritmética de fechas, que es la diferencia entre eliminar un flake y cambiar lo que el test mide.
