# fix-availability-scope — la disponibilidad no puede depender del rol del caller

Feature: corregir el cálculo de disponibilidad para que no use el lector de reservas scopeado por rol. Hoy un caller `client` ve libres los turnos tomados por **otros** clientes, y un caller `staff` ve vacía la agenda de **cualquier otro** profesional. Pre-existente en `check_availability`; amplificado por `find_available_slots` (#242).

Issue: **#243** (`type:bug` + `status:approved`). Origen: observación 1 del verificador independiente de #242.

## Specs

- **S1** (el defecto) — La disponibilidad MUST calcularse contra **todas** las reservas no canceladas de la ventana solicitada. El rol del caller MUST NOT cambiar si un slot se reporta libre u ocupado. Owner y admin ya son correctos (no reciben filtro); el defecto afecta a `client` y a `staff`.
- **S2** (alcance) — `check_availability` y `find_available_slots` MUST usar el lector role-neutral. **Cambio de comportamiento aceptado** (ítem 3): un `client` que consulta un turno tomado por otro cliente pasa de `available: true` a `ErrCodeBookingOverlap`; un `staff` que consulta a otro profesional pasa de "libre" a la verdad.
- **S3** (implementación) — MUST existir un tipo adaptador separado en `internal/repository` (`AvailabilityBookingsReader`) que implemente `FindOverlapping` y `ListBookingsForRange` **sin** `applyAuthFilter`. MUST NOT implementarse mediante un flag, un marcador en el `ctx`, un parámetro booleano ni un método nuevo en `BookingsRepo`: un tipo distinto solo se puede usar si el composition root te cableó a él.
- **S4** (predicado único) — El SQL de solapamiento y el SQL de rango MUST salir cada uno de **un solo builder** compartido por el reader filtrado y el no filtrado. La versión sin filtro MUST diferir **solo** por no agregar la cláusula de `applyAuthFilter`. Duplicar el predicado está prohibido: dos copias derivan y nadie lo nota.
- **S5** (acceso a datos intacto) — Todos los métodos de datos de `BookingsRepo` (`FindByID`, `Update`, `Cancel`, `Reschedule`, `FindByStaffAndRange`, `SearchByNotes`, `UpdateStatus`) MUST seguir aplicando el filtro de forma incondicional. `applyAuthFilter` MUST NOT ganar ninguna forma de bypass.
- **S6** (auth en disponibilidad) — El reader de disponibilidad MUST NOT exigir caller: la disponibilidad es una pregunta sobre el calendario, no sobre los datos del llamante. Esto **alinea** con la doc ya existente de `CheckAvailability` ("non-authoritative preview tool … does NOT require authentication"). Ningún camino de acceso a datos pierde su requisito de caller.
- **S7** (PII) — Ningún camino de disponibilidad MUST exponer identidad de clientes. `check_availability` sigue devolviendo `{available, message?}` con el mensaje de solapamiento nombrando al profesional y la ventana; `find_available_slots` sigue devolviendo `{start_datetime, professional_id, professional_name}`.
- **S8** (regresión congelada) — `CheckAvailability` MUST conservar firma, códigos de error y orden de cortocircuito (REQ-AV-1/REQ-AV-2), y `internal/domain/service/availability_test.go` MUST pasar sin modificaciones.
- **S9** (wiring y su prueba) — `service.AvailabilityDeps.Bookings` MUST tipar la interfaz angosta (`BookingOverlapReader`), no el puerto completo, para que el root no pueda pasar el repo filtrado por inercia. La prueba de que el cableado es correcto MUST ser de integración a través de la composición real: un caller `client` NO recibe como libre un slot tomado por otro cliente.
- **S10** (no-goals) — No se introduce la proyección de intervalos sin datos de cliente (queda como follow-up: aislaría PII por construcción pero toca `BookingOverlapReader` y la cadena congelada). No se toca el RBAC, ni `AggregateByClient`, ni el scoping de ningún camino de datos.

## Tasks

Una unidad de trabajo. Rama `fix/availability-scope`.

- **T-1** · S3,S4,S5 · adaptador + builder compartido
  - `internal/repository/bookings.go`: extraer el predicado de solapamiento y el de rango a builders únicos reutilizables dentro del paquete.
  - `internal/repository/bookings_availability.go` (nuevo): `AvailabilityBookingsReader` con `FindOverlapping` y `ListBookingsForRange` sin filtro, sobre los mismos builders.
  - `internal/repository/bookings_availability_test.go` (nuevo): el reader no filtrado devuelve reservas de **otros** clientes/profesionales; el filtrado las sigue ocultando.
- **T-2** · S6,S8 · dominio
  - `internal/domain/service/availability.go`: `AvailabilityDeps.Bookings` pasa a `BookingOverlapReader`. Sin otros cambios; la cadena queda intacta.
- **T-3** · S2,S7,S9 · consumidores y wiring
  - `internal/application/usecase/find_available_slots.go`: el campo de lectura de rango pasa a una interfaz angosta del consumidor; el constructor la recibe.
  - `cmd/mcp-server/main.go`: construir el `AvailabilityBookingsReader` una vez e inyectarlo en `AvailabilityDeps` y en `NewFindAvailableSlotsUseCase`.
  - `internal/mcp/server_integration_*_test.go`: prueba de integración por la composición real (S9).

## Trampas verificadas (no repetir)

1. **`applyAuthFilter` devuelve `(\"\", nil, err)` con caller nil.** Si el reader no filtrado copia el patrón llamando a `auth.RequireCaller`, reintroduce el error de "no autenticado" en un camino que por diseño no lo requiere — y `check_availability` dejaría de funcionar sin header. El reader NUEVO no llama a `RequireCaller` (S6).
2. **Hay tests que asertan el comportamiento sin caller.** Antes de asumir que S6 es inocuo, buscar los tests que esperan `-32000`/`ErrCodeUnauthenticated` en el camino de disponibilidad; si existen, su expectativa cambia y hay que decidirlo explícitamente, no "arreglar el test".
3. **El narrow interface no impide el bug por sí solo.** `BookingsRepo` satisface `BookingOverlapReader`, así que el root podría pasar el repo filtrado y compilar. La única prueba real es la de integración de S9, no el tipo.
4. **`AggregateByClient` es el precedente y NO se toca.** Ya saltea el filtro y su contención es el RBAC owner/admin de la tool de fidelidad.
5. **El lector en memoria de `find_available_slots` no cambia.** Sigue igualando el predicado; lo que cambia es qué filas recibe.
6. **Un turno pasado sigue siendo un filtro, no un error** en `find_available_slots`; el clamp a hoy no se toca.

## Verificación

- Pipeline completo (`fmt`/`vet`/`lint`/`build`/`test -race`/`govulncheck`).
- Tests obligatorios: (a) repo — reader no filtrado ve reservas ajenas, filtrado las oculta; (b) use case — un `client` recibe **no disponible** un slot tomado por otro cliente; (c) use case — un `staff` consultando a otro profesional ve la verdad; (d) integración — el mismo caso por la composición real (S9); (e) camino de datos intacto — un `client` sigue sin poder leer reservas de otro cliente por las tools de datos.
- Cada test MUST afirmar el efecto observable completo: resultado exacto, código de error cuando aplique, y que un input rechazado no altera datos ni contadores.

## Log

- **L1** (owner, verbatim): *"arranquemos #243"*. Issue #243 con `status:approved` + `type:bug`.
- **L2** (hallazgo al abrir el ciclo, del agente): el issue estaba **incompleto**. `applyAuthFilter` mete `AND professional_id = ?` para staff, así que un caller staff consultando a otro profesional obtiene cero filas y "libre". El defecto no es "del cliente": es de **cualquier rol cuyo filtro recorte el conjunto**.
- **L3** (precedente verificado): de los 10 métodos de `BookingsRepo`, solo `Create` y `AggregateByClient` saltean el filtro. `AggregateByClient` es el precedente de read no scopeado, contenido por el RBAC owner/admin de la tool de fidelidad.
- **L4** (owner, verbatim, decisión): *"Adaptador separado (recomendado)"* → opción A. Descartadas explícitamente: el flag/marcador en `ctx` (haría el filtro salteable por accidente) y el método `*Unscoped` en el puerto (quedaría a un autocompletado de cualquier caller de datos).
- **L5** (decisión del agente, corregible): se difiere la proyección de intervalos sin datos de cliente (opción B del owner) como follow-up. Aislaría PII por construcción, pero exige cambiar `BookingOverlapReader` y la cadena congelada; el blast radius no corresponde a un fix de corrección.
