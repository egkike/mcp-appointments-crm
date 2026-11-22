# debt-cleanup-availability — cierra L5, L7 y L15 de #246

Feature: cerrar tres de las cinco deudas declaradas al mergear #246 (`1268876`). Un PR, tres commits. Fuera de alcance por decisión explícita: **S11** (política de permisos, no deuda técnica) y **L11** (exige refactorizar el composition root).

Issue: **#247** (`type:refactor` + `status:approved`).

## Specs

- **S1** (L7 — el inventario nombra el adaptador) — `wiredRepoInventory` (`cmd/mcp-server/main.go:645`) MUST incluir `AvailabilityBookingsReader`, pasando de 9 a **10** handles; el comentario que dice "9 handles" MUST actualizarse; y el set documentado de `TestWiredRepoInventoryContract` (`cmd/mcp-server/main_test.go:513`) MUST ganar la entrada correspondiente. Nombre sugerido: `bookings_availability`, espejo del archivo.
- **S2** (L15 — los cuatro helpers con test directo) — `indexExceptionsByDate`, `businessWindow`, `parseSlotWindow` y `clockMinutes` (`internal/application/usecase/find_available_slots_agenda.go`) MUST tener test directo, cubriendo lo que `Execute` no distingue: excepción con ventana propia vs. `is_closed`, HH:MM malformado e invertido en cada rama, fecha de excepción vacía o duplicada, y las 7 claves de día. Los tests existentes de `Execute` MUST quedar intactos.
- **S3** (L5 — el reader no carga lo que no usa) — `AvailabilityBookingsReader` MUST seleccionar **solo** las columnas que sus consumidores leen: `professional_id`, `start_datetime`, `end_datetime`, `status`. MUST NOT seleccionar `client_id`, `notes` ni `payment_method`. Un test MUST afirmar que las entidades que devuelve tienen esos tres campos en su valor cero.
- **S3a** (enmienda a la S4 de `fix-availability-scope`) — Lo compartido entre el reader filtrado y el no filtrado es el **predicado** (el `WHERE` y sus argumentos), no la lista de columnas. El builder MUST parametrizarse por proyección: `bookingOverlapQuery(columns, staffID, start, end)` y `bookingRangeQuery(columns, start, end)`, con constantes `bookingColumns` (completa, para los caminos de datos) y `availabilityBookingColumns` (angosta, para el reader). Duplicar el predicado sigue prohibido.
- **S4** (sin cambio observable) — Ninguna tool MUST cambiar su respuesta. Las aserciones de grid exacto, `limit`, `truncated` y orden de `find_available_slots` MUST pasar sin modificarse, igual que el gate de regresión de `availability` (15 subtests nombrados de `TestCheckAvailability`).
- **S5** (no-goals) — No se toca `internal/domain/service/**`, ni `internal/domain/repository/**`, ni `internal/repository/auth_filter.go`, ni el RBAC. No se toca S11 ni L11. No se agrega ninguna tool.
- **S6** (trade-off documentado) — Un consumidor futuro que leyera `ClientID`, `Notes` o `PaymentMethod` de las entidades del reader obtendría el valor cero **en silencio**. El tipo MUST documentarlo: la contención pasa a ser estructural (el dato no se carga) a cambio de que ese acceso sea silenciosamente vacío en vez de silenciosamente expuesto.
- **S7** (ancla normativa de la asimetría de horas corruptas, agregada por L9) — Hay **tres** posturas para la misma clase de corrupción, y esto las fija como decisión, no como accidente:
  1. **Horario semanal ilegible** (`profile.GetOpenClose` falla) → error interno, **loud**. Ya era así.
  2. **Excepción de horario abierta ilegible** → error interno, **loud**. Se decidió en #245.
  3. **Horario semanal que parsea pero da límites inválidos** (`parseSlotWindow` rechaza) → el día se cierra **en silencio**. Se mantiene así: `businessWindow` resuelve **cotas de iteración**, la autoridad sigue siendo la cadena, y esa rama puede ser inalcanzable porque `GetOpenClose` valida antes.
  Cambiar la postura 3 exige su propia decisión y su propio test; MUST NOT cambiarse como efecto colateral de un cleanup.

## Tasks

Una unidad de trabajo. Rama `refactor/declared-debt`.

- **T-1** · S1 · inventario
  - `cmd/mcp-server/main.go`: entrada en la lista de `wiredRepoInventory` + comentario 9→10.
  - `cmd/mcp-server/main_test.go`: set documentado + el comentario que dice "9 today".
- **T-2** · S2 · tests de los helpers
  - `internal/application/usecase/find_available_slots_agenda_test.go`: los cuatro grupos. Sin tocar los tests de `Execute` ni sus fixtures salvo lo mínimo para compartir un helper si hace falta.
- **T-3** · S3,S3a · proyección acotada
  - `internal/repository/bookings.go`: parametrizar los dos builders por columnas, con las dos constantes.
  - `internal/repository/bookings_availability.go`: usar `availabilityBookingColumns` y documentar el trade-off (S6).
  - `internal/repository/bookings_availability_test.go`: el test de campos vacíos.

## Trampas verificadas (no repetir)

1. **Los caminos de datos no cambian de proyección.** `FindOverlapping`, `FindByStaffAndRange`, `ListBookingsForRange` y el resto siguen con `bookingColumns` completa. Si alguno pasa a la angosta, un consumidor pierde datos en silencio y el suite puede no notarlo (los tests de datos asertan campos, no proyecciones).
2. **El predicado sigue siendo uno solo.** Parametrizar las columnas es aceptable; copiar el `WHERE` no. Si al partir el builder aparecen dos textos de predicado, está mal.
3. **`TestWiredRepoInventoryContract` pinna el set, no solo el conteo**, y rechaza duplicados. Actualizar la lista en `main.go` sin la de `main_test.go` deja el test rojo con un mensaje que nombra la diferencia.
4. **El inventario cuenta implementaciones de repositorio del path de serve**, no stores lógicos: `bookings` y `bookings_availability` son dos entradas porque son dos implementaciones sobre el mismo store.
5. **`scanBooking` espera la lista completa de columnas**; el reader angosto necesita su propio scanner, y ninguno de los dos debe duplicar el mapeo de las columnas que comparten.
6. **El contrato PII del wire no cambia**: `find_available_slots` sigue devolviendo `start_datetime`, `professional_id`, `professional_name`; `check_availability` sigue devolviendo `{available, message?}`.

## Verificación

- Pipeline completo (`fmt`/`vet`/`lint`/`build`/`test -race`/`govulncheck`).
- Tests obligatorios: (a) inventario en 10 y set documentado en sincronía; (b) los cuatro helpers con sus bordes; (c) las entidades del reader con `ClientID`/`Notes`/`PaymentMethod` vacíos; (d) los tests de comportamiento de `find_available_slots` y el gate de `availability` pasan **sin modificarse**.
- Reportar los conteos `=== RUN` antes/después por paquete tocado: se esperan subas solo en los paquetes donde se agregaron tests.

## Log

- **L1** (owner, verbatim): *"Ahora arreglemos lqa Deuda declarada primero y dejemos limpio"* → issue **#247**. Alcance elegido por el owner: **L7 + L15 + L5** (`"L7 + L15 + L5 (recomendado)"`).
- **L2** (decisión del agente, corregible): **L5 se resuelve recortando columnas**, no con la proyección de intervalos que el doc de #246 declaraba. Es más chico, no toca `BookingOverlapReader` ni la cadena congelada, y convierte la contención de procesal en estructural: el dato no se carga, así que no hay nada que un eco pueda filtrar.
- **L3** (hallazgo, del agente): recortar columnas **tensiona** la S4 de `fix-availability-scope` ("la versión sin filtro difiere solo por la cláusula de auth"). Resolución registrada como **S3a**: lo compartido debe ser el **predicado**, no el `SELECT`. El invariante que protege del drift es el `WHERE`; la proyección es legítimamente distinta por consumidor.
- **L4** (no-goals explícitos del owner): **S11** queda como decisión de política (¿puede un staff ver la agenda de otro profesional?); **L11** queda como mejora mayor (harness espejo → refactor del composition root).
- **L5** (corte de entrega): **848 líneas authored en 4 commits** (`832925b` spec · `969f6cd` código · `f2d66c7` corte y desviaciones · `c58cc6c` fixes de los dos advisories), **2,1× el corte de ~400**. El cuarto commit **no estaba previsto acá** cuando se escribió este corte y se agrega a raíz de la review (`review-f94fd31c48e77ce8`): sus dos advisories se corrigieron en vez de arrastrarse, porque el propósito del issue es dejar limpio y uno de ellos era deuda **introducida por este mismo PR**. Forma elegida: **un solo PR** — los ítems comparten el archivo de builders y el mismo reader, y partirlos obligaría a un estado intermedio donde el builder está parametrizado pero el reader todavía pide columnas completas.
- **L6** (prueba de L5 por mutación, del worker): voltear el reader a `bookingColumns` hizo fallar `TestAvailabilityBookingsReaderProjectionIsNarrow` con los valores reales de la fuga (`ClientID="c1"`, `Notes="nota privada"`, `PaymentMethod="cash"`), y restauró byte a byte. Es un RED de verdad sobre el claim de PII, no un razonamiento.
- **L7** (desviación declarada del worker, verificada por el orquestador): `SearchByNotes` se plegó sobre `queryBookings`. Es un camino de datos que el issue no nombraba, así que se comparó contra `main` antes de commitear: slice **no nulo** en ambos (`[]` en el wire, no `null`) y los tres strings de error idénticos (`%s: %w`, `%s: escaneo: %w`, `%s: iteración: %w`). Equivalente.
- **L8** (forma de S3a, desviación menor del worker): la constante compartida contiene **solo el `WHERE`**, no el bloque `FROM`/`ORDER BY`. Es lo que el invariante protege — el predicado — y deja la proyección y el orden explícitos en cada builder. Aceptado.
- **L9** (verificador independiente, 7/7 PASS, sin defectos de código; dos observaciones del proceso, ambas del orquestador):
  - **Blocker de proceso**: detectó que `c58cc6c` (los fixes de advisories) se creó **durante** su verificación y no estaba registrado en este Log, que decía "tres commits". Tenía razón: el corte se escribió antes de la review y no se actualizó. Corregido en L5.
  - **Asimetría sin ancla normativa** (ver **S7**): los tests de `businessWindow` pinnean que horas semanales malformadas **cierran el día en silencio**, mientras la cadena falla **loud** por slot con el mismo valor, y la rama de **excepción** corrupta falla loud desde #245. Tres posturas para la misma clase de corrupción. El verificador lo juzgó un pin honesto pero señaló que el lado "correcto" no está documentado en ninguna norma, así que un cambio futuro de postura parecería un test roto. Se ancla en S7 en vez de cambiar código: esa rama puede ser inalcanzable en la práctica (`GetOpenClose` valida antes) y un cambio de comportamiento merece su propia decisión.
  - Confirmado además: la contención de L5 es **estructural** (la proyección no se usa en ningún otro lado, ningún consumidor lee un campo caído, ni siquiera indirectamente vía métodos de `entity.Booking`), y los cuatro caminos de datos son equivalentes a `main` con los 12 call sites de `applyAuthFilter` alineados.
