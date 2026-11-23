# feat-staff-self-read — registro propio y agenda propia para `staff` (#276)

Issue: #276 · Rama: `feat/feat-staff-self-read` · Riesgo: ítem 2 + ítem 3 (autorización + contrato de superficie) · Origen: decisión del owner 2026-11-23.

## Specs

- **S1 (`get_professional`, tool nueva)** — MUST existir una tool de lectura `get_professional` con fila `ToolRBAC{owner, admin, staff}`. Input `{professional_id?}`: ausente ⇒ el propio profesional (requiere `Caller.ProfessionalID`; si el caller no es un profesional ⇒ error semántico de input, nunca un panic ni una lista vacía). Presente ⇒ owner/admin cualquier id; `staff` **solo su propio id**.
- **S2 (alcance propio, código correcto)** — Un `staff` que pide un `professional_id` ajeno MUST recibir un error semántico **FORBIDDEN** con `auth.MsgForbiddenTool`, que el transporte emite como **`-32002`**. MUST NOT recibir el `ErrCodeUnauthenticated` que hoy devuelve `ProfessionalsRepo.FindByID` para un id fuera de alcance: el caller está autenticado. **Corrección (L5)**: la versión original de este spec decía `-32001`; ese código pertenece al gate por **nombre de tool**, que aquí admite al staff — el id ajeno se decide por debajo de un transporte que nunca parsea argumentos.
- **S3 (agenda)** — `get_professional_schedules` MUST admitir `{owner, admin, staff}` en **las tres capas**: fila `ToolRBAC`, `RequireRole` del use case y el guard de `SchedulesRepo.FindByProfessional`. `staff` MUST poder leer solo su propia agenda, con la misma semántica de rechazo que S2 (FORBIDDEN ⇒ `-32002`).
- **S4 (PII a nivel de campo)** — El registro MUST devolver `id`, `name`, `role_specialty?`, `status`, `specialties[]` para todo caller admitido. `email` y `phone` MUST incluirse **solo** cuando el caller es `owner` o el propio profesional; para `admin` MUST omitirse (`omitempty`, molde `professionalOut`).
- **S5 (superficie)** — El contrato observable pasa a **owner 27 / admin 16 / staff 13 / client 11**. El rol `client` MUST NOT ver ninguna de las dos tools. `expectedToolCount` MUST pasar a 27 y cada conteo literal MUST actualizarse junto con sus escenarios.
- **S6 (docs)** — MUST enmendarse: `REQ-MT-005` (conteos, escenario de staff con los 13 nombres, y el escenario de defensa en profundidad), `REQ-MT-015` (filas de ambas tools), `auth-roles/spec.md:26` ("el único que puede … leer la agenda"), el conteo "12 de las 26 owner-only" en `auth-roles/spec.md` y `ADR-0009:112` (pasa a **11 de las 27**), y `docs/PRD.md:731`.

## Tasks

- **T1** · S1,S2,S4 · dominio/puerto si hace falta + use case `get_professional` + handler + wiring (`ports.go`, `config.go`, `cmd/mcp-server/main.go`) + fila RBAC
- **T2** · S3 · ensanchar las tres capas de la agenda con el scope propio del staff
- **T3** · S5,S6 · conteos, escenarios, tablas y prosa de specs/ADR/PRD
- **T4** · tests de las tres capas: handler/mux, use case, repositorio; RED observado por cada comportamiento nuevo

## Notas de implementación (verificadas en el mapa)

- `Caller.ProfessionalID` es `*string` y el schema lo garantiza para staff (`db/schema.go:131`): `staff` sin `professional_id` no puede existir.
- `ProfessionalsRepo.FindByID` ya restringe al staff a su propio registro, pero con `ErrCodeUnauthenticated` — S2 exige cambiarlo a Forbidden en el camino de la tool, sin romper el uso interno que dependa de ese caso (verificar quién lo consume: `delete_schedule` y otros).
- `SchedulesRepo.FindByProfessional` es la tercera capa y hoy exige owner.
- Moldes de "self" a reutilizar: `find_available_slots` (id ajeno rechazado), `applyAuthFilter` (predicado SQL), `FindActive`.
- El `client` no debe ver las tools: por eso `get_professional` lleva fila RBAC y no entra en `UngatedTools()`.

## Log

- **L1** (owner, verbatim, decisiones de este ciclo): *"Tool nueva pero debería ser para Owner, Admin y Staff... Qué opinas?"* · *"También admin (cualquiera)"* · *"Incluir email/teléfono propios"*.
- **L2** (agente, opinión aceptada como base): owner+admin+staff con PII a nivel de campo — el admin recibe el registro **sin** email/teléfono, porque su función es oversight operativo y exponerle PII de todo el plantel no la sirve; el molde `professionalOut` ya soporta `omitempty` para esos campos.
- **L3** (agente, verificado): la agenda tiene **tres** gates, no dos; ensanchar solo la fila RBAC habría dejado la tool respondiendo `-32001` desde el repositorio.
- **L4** (agente, corregido en implementación — el código NO sigue al pie de la letra el `-32001` de S2/S3, y esto es deliberado): el gate `ToolRBAC` del middleware keyea por **nombre de tool** y nunca parsea el cuerpo de `tools/call`, así que un `staff` que nombra un `professional_id` ajeno pasa la fila `{owner, admin, staff}` y es rechazado **debajo** del transporte. Ese rechazo es un `*domain.SemanticError{Code: ErrCodeForbidden, Message: auth.MsgForbiddenTool}` (exactamente lo que pide la Nota de implementación) y el transporte lo mapea a **`-32002`**, no a `-32001`: `-32001` es el código del middleware (`internal/auth/middleware.go`) y emitirlo desde un handler confundiría "la tool no está permitida" con "ese id no está permitido". El precedente del repo ya fija esa convención — el doble gate de la agenda documenta el rechazo del use case como `-32002` semántico (`TestToolGetProfessionalSchedulesForbiddenIsSemantic`) y el del mapa como `-32001` (`TestIntegrationClientRoleForbiddenTools`). Lo observable que S2/S3 exigen — "Forbidden, no `UNAUTHENTICATED`", con `auth.MsgForbiddenTool` — se cumple; el número del sobre es `-32002` en ambas tools. Quien quiera `-32001` para el mismo caso debe cambiar el contrato de errores del transporte (mapear `ErrCodeForbidden` → `-32001` en `internal/mcp/errors.go`), decisión que no corresponde a este slice.
- **L5** (agente, corrección de mi propio spec): S2/S3 decían `-32001` para el rechazo de id ajeno. Es falso: `-32001` es el rechazo del gate por **nombre de tool** en el middleware, que no parsea argumentos; aquí el staff **sí** está admitido por nombre, así que el rechazo por id nace en el use case como error semántico `FORBIDDEN` y el transporte lo emite como `-32002`, igual que el resto de los rechazos de negocio. El intento observable ("Forbidden, no UNAUTHENTICATED, con el mensaje que ya emite el producto") se cumple; el número cambia y así quedó en `REQ-MT-005`.
- **L6** (verificador independiente, 2026-11-23): **PASS**, 0 CRITICAL, sin bypass encontrado — enumeró cada camino (id ajeno, vacío, con espacios, repo directo, catálogo) y confirmó que la identidad nunca sale de los argumentos y que el predicado corre antes de toda lectura. Aplicado en el ciclo:
  - su **WARNING** (spec `mcp-transport:106` reescribía el conteo histórico de T1 de 26 a 27, contradiciendo el "Previously 26" de arriba) → restaurado a *"the registry stayed at 26"*;
  - su **SUGGESTION** sobre PII: `CanReadProfessionalContact` admitía "cualquier caller con `professional_id` vinculado", y el `CHECK` de `accounts` permite que un **admin** cargue ese campo → un admin vinculado habría recibido el email/teléfono de ese profesional, contradiciendo la decisión del owner. La rama "self" ahora exige rol `staff` (el único que el schema ata a un profesional) y hay caso de test para el admin vinculado.
- **Follow-ups registrados, no ejecutados** (fuera de este slice): (a) `ProfessionalsRepo.FindByID` conserva una cuarta copia de la regla que devuelve `ErrCodeUnauthenticated` para id ajeno y sigue siendo alcanzable vía `find_available_slots` — el mismo razonamiento de S2 (el caller está autenticado) aplica ahí; (b) el e2e del binario cubre el rechazo del `client` en la agenda pero no en `get_professional` (lo cubre la integración); (c) `docs/architecture/0020-*.md:57` dice "25 tools" y es histórico — un ADR no se reescribe, se le agrega nota.
