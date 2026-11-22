# Agent surface — tools/list por rol y búsqueda de turnos (PROPUESTA)

Feature: reducir la superficie MCP que recibe el agente y cerrar el read-gap de turnos disponibles, partiendo de la evaluación de "code mode" descartada. **Estado: PROPUESTA — sin issue, sin rama, sin código.** Nada de este documento está aprobado para ejecución.

Origen: análisis de Codemode (harness de Pi) → descartado para el server → quedan dos unidades medidas.

## Evidencia medida (paso 0, 2026-11-13)

Medición sobre la **capa HTTP de producción en proceso**: `newIntegrationMux` (misma cadena `AuthMiddleware` + `/mcp` que arma `cmd/mcp-server/main.go`) + `postMCPCaller(t, mux, caller, {"jsonrpc":"2.0","id":1,"method":"tools/list"})`, midiendo `len(result)` del envelope. El puerto es solo el listener: el payload es idéntico byte a byte al que recibe Hermes.

| Métrica | Valor |
|---|---|
| Tools registrados | **25** |
| `tools/list` result | **20.151 bytes ≈ 5.037 tokens** |
| `outputSchema` | 9.365 B = **46,7%** (~2.341 tok) — los 25 tools lo emiten |
| `inputSchema` | 5.236 B = 26,1% (~1.309 tok) |
| `description` | 3.593 B = 17,9% (~898 tok) [3.516 B en los literales Go, sin escape JSON] |
| `name` + `annotations` + meta | 1.864 B = 9,3% |

Top 5 por peso: `update_business_profile` 2.340 B · `get_business_profile` 1.276 B · `create_hours_exception` 1.036 B · `update_professional` 975 B · `get_professional_schedules` 915 B.

**`tools/list` no filtra por rol.** Mismo request, distinto caller:

| Caller | Tools | Bytes |
|---|---|---|
| `owner-1` | 25 | 20.151 |
| `staff-1` | **25** | **20.151** |
| sin `X-Caller-Id` | error `-32000` | — |

Cruzado con el `ToolRBAC` de `cmd/mcp-server/main.go`, el catálogo útil por persona:

| Persona | Tools usables | Bytes necesarios | Hoy recibe | Desperdicio |
|---|---|---|---|---|
| owner | 25 | 20.058 | 20.058 | 0% |
| admin | 13 | 8.995 | 20.058 | 55% |
| staff | 9 | 6.490 | 20.058 | 68% |
| **client** | 5 | **3.476** | 20.058 | **83%** |

La persona `client` es el flujo de la recepcionista por WhatsApp (ADR-0018): es el caso central del producto y el que más desperdicia.

## Specs

- **S1** (gobierna el alcance) — El owner: *"Solo el documento, con las dos"* (2026-11-13). Se documentan las dos unidades; **no** se crea issue, rama ni código hasta decisión explícita.
- **S2** (U1 — filtrado por rol) — `tools/list` MUST devolver solo las tools que el caller resuelto puede ejecutar según el `ToolRBAC` del composition root. *[propuesta — pendiente de aprobación]* Las tools sin entrada en el mapa ("any authenticated": `check_availability`, `search_clients_advanced`, `search_services_advanced`, `search_professionals_advanced`, `register_client`) MUST seguir visibles para todo caller autenticado.
- **S3** (U1 — sin pérdida de capacidad) — El filtrado MUST NOT quitar ninguna tool que el caller podía invocar antes. La reducción es coincidencia exacta con el `ToolRBAC`; las violaciones de RBAC siguen respondiendo `-32000` igual que hoy.
- **S4** (U2 — búsqueda por rango) — MUST existir una tool de lectura que responda los turnos libres de un servicio en un rango de fechas, con cota de respuesta. **Hoy no existe**: `check_availability` responde un solo slot booleano, así que "¿qué hay mañana?" obliga al LLM a iterar o a inventar. *[propuesta — pendiente de aprobación]*
- **S5** (U2 — rol consumidor) — `find_available_slots` SHOULD ser "any authenticated", molde de `check_availability`, porque el consumidor natural es la recepcionista del canal. **Decisión abierta del owner.** *[propuesta]*
- **S6** (U2 — sin duplicar la regla) — La regla "¿este profesional trabaja este día y hora?" MUST extraerse de `check_availability` a un helper compartido. Si no se puede reutilizar limpio, la unidad no se ejecuta. `openspec/specs/availability/spec.md` REQ-AV-1/REQ-AV-2 exigen que `CheckAvailability` conserve firma, códigos de error y orden de cortocircuito **byte-idénticos**.
- **S7** (descartado, no re-litigar) — MUST NOT implementarse: ejecución de código/intérprete en el server (code mode); supresión de `outputSchema`; dieta de descripciones; encogimiento de DTOs de salida de tools de escritura; discovery surface (`search_tools`). Razones medibles abajo.
- **S8** (disparador de reapertura) — Discovery surface vuelve a la mesa solo si `tools/list` supera **40 tools** o **12.000 tokens**.

## Tasks

**Orden decidido por el owner (L9): T2 primero, luego T1.** T2 está autorizada y vive en `odd/tasks/feat-find-available-slots.md` (issue #241). T1 sigue **sin autorizar**: se lista para dimensionar.

- **T1 (sin autorizar)** · S2,S3 · U1 · ~120–200 líneas · riesgo ítem 3
  - `internal/mcp/server.go`: `AddReceivingMiddleware` con filtro sobre `req.Method == "tools/list"`
  - `internal/mcp/config.go`: campo nuevo con `auth.ToolRBAC` inyectado desde el composition root
  - `cmd/mcp-server/main.go`: pasar el mapa ya construido al `mcp.Config`
  - tests: un caso por rol (owner 25 / admin 13 / staff 9 / client 5), tools sin entrada visibles para todos, y que un caller no-owner siga recibiendo `-32000` al invocar una tool filtrada
- **T2 (propuesta)** · S4,S5,S6 · U2 · ~300–450 líneas · riesgo ítem 3 + ítem 6
  - `internal/domain/service/`: helper de disponibilidad extraído (sin cambiar `CheckAvailability`)
  - `internal/application/usecase/find_available_slots.go` + `internal/application/dto`
  - `internal/repository/`: lectura de bookings del rango con prepared statement único; el cómputo de slots queda en el use case
  - `internal/mcp/tools_booking.go` (o archivo propio): struct `*In` con tags + handler
  - `openspec/specs/availability` + tabla de contratos `mcp-transport` **antes** del código (SDD)
  - tests 3 capas: bordes de excepción de horario, DST en `America/Argentina/Buenos_Aires`, solapamiento, rango vacío, `limit`/`truncated`

### Firma propuesta de T2

```
find_available_slots(
  service_id, professional_id?, from, to, limit?
) → { slots: [{start_datetime, professional_id, professional_name}],
      total_candidates, truncated }
```

Racionales: devuelve `start_datetime` + profesional en vez de `BookingView` completo (result shaping); `limit` default 10 / máx 50 con `truncated` (cota dura); rango máx 14 días (sin cota, un rango de 6 meses es un scan inútil bajo `busy_timeout`); `professional_id` opcional porque "¿quién puede atenderme?" es el caso real.

## Descartado con evidencia

| Descartado | Razón medida |
|---|---|
| **Code mode / intérprete en el server** | El beneficio de code mode es volumen de tokens, no round trips (benchmarks públicos: requests al modelo bajan 1–22%). Ese beneficio se captura con T1+T2 sin sandbox. Un intérprete metería ejecución de código arbitrario en el proceso que custodia el SQLite de reservas, exigiría `any` réflexivo (prohibido por `AGENTS.md`) y pondría orquestación en el adaptador de transporte (violación de ADR-0013). |
| **Suprimir `outputSchema`** (47% del payload) | `go-sdk v1.7.0` `server.go:345` lo omite solo si `Out == any`. Prohibido por la regla zero-`any` del repo. |
| **Dieta de descripciones** | 898 tok = 18% del payload. Bajarlas a la mitad ahorra ~450 tok y empeora el guidance al LLM. No paga. |
| **Encoger DTOs de salida de escrituras** | Contract change (ítem 3) sobre tools owner-only — la persona que *no* tiene problema de contexto. |
| **`end_datetime` / `reason` como defecto** | **No son defectos.** REQ-MT-015 los exige y `verify-pr2-report.md:146` los declara consistentes; el código los documenta (`internal/mcp/tools_booking.go:24-27,47-53`). Tocarlos es romper un contrato que Hermes ya usa. |
| **Discovery surface / `search_tools`** | Prematuro con 25 tools. Ver S8. |

## Riesgos

- **ítem 3 (contratos que otros consumen)** — T1 cambia el contenido observable de `tools/list`; T2 agrega una tool. Mitigación T1: la reducción es restrictiva y coincide exactamente con el RBAC, ninguna capacidad autorizada se pierde (S3). Mitigación T2: aditivo puro, ninguna tool existente cambia.
- **ítem 6 (regresión que ningún test atraparía)** — T2 toca lógica de disponibilidad con muchos bordes. Mitigación: S6 (helper compartido, `CheckAvailability` intacto) + la matriz de tests de T2.

## Ruta de entrega

1. Issue con `type:feature` + `status:approved` (owner) — gate `pr-gate` obligatorio.
2. Rama por unidad: `feat/tools-list-filter` · `feat/find-available-slots`.
3. Spec update (SDD) antes del código.
4. Test-first en las 3 capas afectadas.
5. Pre-Commit Checklist completo (`fmt` / `vet` / `golangci-lint` / `build` / `test -race`).
6. `assess` decide el gate de review.

## Trampas verificadas (no repetir)

1. `tools/list` **no** pasa por el `ToolRBAC`: el mapa keyea sobre `r.URL.Path`, que el traductor JSON-RPC reescribe a nombre de tool **solo para `tools/call`**. Por eso el catálogo completo se filtra a cualquier caller autenticado.
2. El orden de las tools en `tools/list` lo decide el SDK (alfabético); los tests de conteo existentes comparan por conjunto, no por índice.
3. `expectedToolCount = 25` (`internal/mcp/test_helpers_test.go`) es la fuente única del conteo. T2 lo mueve a 26 y rompe en cascada los fixtures de e2e/integración que lo asertan — actualizar el literal, no los asserts.
4. `openspec/specs/availability/spec.md` es un **delta** cuyo gate de regresión exige que los 16 subtests de `availability_test.go` pasen **sin modificar**. T2 no puede tocar ese archivo de tests.
5. `openspec/changes/archive/**` está congelado: no se reescribe historia al agregar T2.
6. El changelog de `docs/PRD.md` es **append-only**: se agrega fila, no se corrigen filas viejas.
7. El puerto `3001` es default del producto desde `d126cab` / #226 y está en v0.13.0; `3000` es del bridge de Hermes. No confundir al diagnosticar "el server no responde".

## Log

- **L1** (2026-11-13, owner, verbatim): *"Hola capo, me gustaría que solo analicemos el tema Codemode y en qué consiste, cual es su aplicación adecuada y si podría ser util o traería alguna mejora a este proyecto de mcp. En esta página hay info sobre eso: https://docs.pi.dev/latest/codemode así que revisa y luego solo me explicas... Solo quiero saber y entender al respecto de esto."*
- **L2** (owner, verbatim): *"Y Codemode como concepto? osea no utilizar lo de pi ni otra herramienta externa sino aplicar un codemode propio al proyecto?"*
- **L3** (owner, verbatim): *"Prepara una propuesta concreta para el proyecto con los 4 puntos de 'Qué sí haría (y es casi todo el beneficio)', si la propuesta es factible y vale la pena entonces lo documentaremos. Veamos primero la propuesta concreta y luego te digo como seguimos."*
- **L4** (owner, verbatim): *"Haz primero el paso 0 para tener toda la info necesaria, luego me vuelves a plantear la propuesta. Ya levanté la VM, te parece bien?"*
- **L5** (owner, verbatim): *"Creo que nuestro server quedó en el puerto 3000 porque antiguamente era así... Recien desde la ultima release lo pasamos al puerto 3001. Si quieres verificar"* → verificado: confirmado por `git log -S` y `v0.13.0`. No hay nada escuchando en 3000/3001 en el host: el server corre en la VM. La medición no se ve afectada (se hizo en proceso).
- **L6** (owner, verbatim): *"Antes de decidir me gustaría ver algo más... en una parte mencionaste que No teniamos tool para consultar un turno en específico, por ejemplo de mañana. Está esto contemplado en la propuesta?"* → precisión: "un turno en específico" **sí** existe (`check_availability`); lo que falta es la **búsqueda por rango**, que es T2. Se buscó si era decisión diferida: no aparece en `docs/PRD.md` ni en `openspec/specs/` (grep `slots|turnos disponibles|rango de fechas|franja`). Precedente del mismo molde: `get_professional_schedules` (v0.9.0, 24→25 tools) cerró el read-gap del chat del owner.
- **L7** (decisión del agente, corregible): el ranking original puso U1 primero por **costo/riesgo**, no por valor de producto. Si la prioridad es que la recepcionista pueda ofrecer horarios, el orden debe invertirse (T2 primero).
- **L8** (owner, verbatim, decisión de este ciclo): *"Solo el documento, con las dos"*.
- **L9** (owner, verbatim, decisión): *"Mañana arrancaremos por T2 primero, luego T1... por si ya quieras anotarlo"* → orden fijado **T2 → T1**. Prevalece sobre el ranking por costo/riesgo de L7.
- **L10** (owner, verbatim, decisión): *"Ah y el rol consumidor de find_available_slots — será any authenticated como recomiendas, anotalo también"* → **S5 resuelto**: `any authenticated`, sin entrada en `ToolRBAC`.
- **L11** (owner, verbatim + decisión): *"la carpeta openspec/specs/ es la forma antigua que usabamos con SDD (ya descontinuado). Ahora usamos solo ODD así que considera esto para la doc necesaria. Es así?"* → verificado a medias: lo retirado es el **workflow** SDD (`openspec/changes/` solo tiene `archive/`; `docs/SDD.md` congelado por PRD 1.47), no el árbol — `openspec/specs/` se mantuvo hasta `4bcfd4e` (#230, 13-nov) y es el único hogar de la tabla de contratos de tools. **Decisión: opción C**, se sigue actualizando en su lugar, sin crear `openspec/changes/<change>/`.
- **L12** (owner, verbatim): *"Listo el merge"* → PR #240 mergeado como `f1f2d49`; CI del merge commit en verde (run 44935140654).
- **L13** (owner, verbatim + decisión): *"Hola capo, arranquemos el T2 si estamos en condiciones"* → aprobado el camino completo (issue + spec + writer). **T2 en ejecución**: issue #241 (`type:feature` + `status:approved`). S6 resultó **ya cumplida** por `ValidateBookingTimeSlot` (`internal/domain/service/booking_time_validator.go:85`): no hace falta extraer nada, se compone vía `BookingOverlapReader`.
