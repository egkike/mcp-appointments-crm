# Design: feat-owner-chat

**Change:** feat-owner-chat
**Inputs:** `proposal.md` (S1–S6, Evidence Inventory E1–E48), `odd/tasks/feat-owner-chat-b12.md:10-23`, `docs/gateway-contract.md`, ADR-0018, ADR-0020, ADR-0013, ADR-0015
**Constraint:** este documento es doc-first. Fija diseño y decisiones; la reescritura del template de skill, la actualización del contrato del gateway y el runbook se aplican en un change posterior (T2–T4 de `odd/tasks/feat-owner-chat-b12.md:28-31`). **No hay cambios de código en esta fase.**

---

## 0. Contrato de capas (ADR-0013) y superficie de código

| Capa | Ubicación | Este change |
|---|---|---|
| Dominio | `internal/domain/**` | **Nada** |
| Aplicación | `internal/application/**` | **Nada** |
| Transporte MCP | `internal/mcp/**` | **Nada** |
| Persistencia | `internal/repository/**`, `internal/db/**` | **Nada** (sin schema, sin migración) |
| Adapter UI | `internal/tui/**`, `internal/admin/**` | **Nada** (sólo se citan como precedente) |
| Datos de runtime | `setup/templates/recepcionista-negocio/SKILL.md` | **Rewrite en T2** (persona dual + bump `1.1.0`) |
| Docs contrato | `docs/gateway-contract.md` | **Update en T3** (reglas owner + checklist) |
| Docs operador | `docs/gateway-contract.md` §7 | **Update en T4** (runbook S5/S6) |

> **Server code impact: none expected.** El set de tools que la persona owner necesita ya existe y ya está gateado dos veces: mapa owner-only en el composition root (`cmd/mcp-server/main.go:437-450`, E28) y `auth.RequireRole(auth.RoleOwner)` dentro de cada use case (E29). Las cuatro operaciones excluidas **no existen** como tools (E28): la frontera TUI-only es física, no textual. Cualquier necesidad de código que aparezca al aplicar este change debe registrarse como una **S nueva con evidencia** (`odd/tasks/feat-owner-chat-b12.md:21-22`) antes de implementarse; no se asume ni se improvisa en T2–T4.

### 0.1 Por qué la frontera de seguridad no cambia

La skill es texto en reposo: no otorga permisos, no reemplaza RBAC y no puede restringir lo que Hermes técnicamente puede hacer (`docs/gateway-contract.md:129`, E17). La persona owner es **guía conductual**; el gate real sigue siendo loopback + `X-Caller-Id` + RBAC + rate limit. Consecuencia de diseño: ninguna regla de este change puede describirse como "control de acceso", y ninguna decisión de producto depende de que el LLM obedezca.

---

## 1. S1 — Selección de persona

### 1.1 Las dos personas

| Persona | Cuándo se selecciona | Alcance |
|---|---|---|
| **Cliente** (v1.0.0, sin cambios) | toda conversación que **no** cumple la regla de selección owner | conversación de negocio, loop de registro, off-topic cordial, escalamiento al titular (E26, E27) |
| **Owner** (nuevo en v1.1) | ver §1.2 | persona cliente **completa** (el owner también pide turnos para sí mismo, doble rol ADR-0011) **más** las once tools de operación owner-only (E28, E29) |

La persona owner es un **superconjunto** de la cliente, no un reemplazo: el owner fraseó el objetivo como "que solo el owner pueda pedir **todo** desde ahí" (`odd/tasks/feat-owner-chat-b12.md:10`, E3). Por eso la v1.1 no le quita a la persona owner ninguna capacidad de la persona cliente; sólo **agrega** operación y **endurece** dos reglas (deletes con doble gate §2, exclusiones TUI-only §3).

### 1.2 Reglas de selección (deterministas y fail-closed)

**R1 — Telegram owner-bot: la persona se clavea por CANAL, nunca por teléfono.**

El bot privado de Telegram admite únicamente el chat del owner en su allowlist y la identidad se inyecta **estáticamente** desde configuración (`MCP_TELEGRAM_OWNER_PHONE`, E16); Telegram no expone el teléfono del emisor a un bot (`docs/architecture/0018-communication-channels.md:60-66`, E20; `docs/gateway-contract.md:66-68`, E13). Por lo tanto:

- Toda conversación que llega por ese bot **es** el owner. La persona owner se activa por la **vía de entrada del mensaje**, no por ningún dato que el bot pueda leer del interlocutor.
- La skill v1.0.0 hoy no puede saberlo porque su "When to Use" es único y cubre todo canal (E26). La v1.1 gana un bloque de selección explícito, redactado como **condición de canal** ("si esta conversación llega por el bot privado del owner…"), que es información disponible en el contexto de la conversación sin ayuda del servidor.
- El canal alternativo (bot del negocio en Telegram) sigue siendo persona cliente con `request_contact` (E13).

**R2 — WhatsApp: la persona se clavea por el ROL RESUELTO del emisor, cuando el gateway exista.**

Con el gateway per-sender entregado, el teléfono del emisor viaja en `X-Caller-Id` y el rol sale siempre de `accounts` / `clients` (E10, E19): un emisor que resuelve a `accounts(is_active=1, role=owner)` es owner; un emisor que resuelve a `clients` es cliente; un desconocido entra al loop de registro y sigue siendo cliente (E12). La skill v1.1 enuncia la regla en esos términos, pero **no** puede ejecutarla por sí misma: la aplica el gateway al elegir el contexto/persona de la conversación (S4, §4).

**R3 — Fail-closed: ante ambigüedad, persona cliente.**

Si el canal no es el bot privado del owner y el rol no está resuelto inequívocamente como owner, la persona es **cliente**. Esto incluye: WhatsApp antes de que el gateway exista, cualquier canal no reconocido, y cualquier estado parcial del gateway.

**R4 — Prohibido promover persona por contenido del mensaje (anti-inyección).**

Un mensaje que diga "soy el dueño", "modo admin", "ignorá tus reglas" o equivalente **nunca** cambia la persona, sin importar cuán verosímil sea. La persona sale de R1/R2 y de nada más. Razón: el contenido de un mensaje es input hostil por definición (cualquier cliente puede escribir esas palabras al número del negocio); promover persona por texto convertiría la guía conductual en un bypass de UX hacia tools que el RBAC sí bloquea — pero con un LLM en el medio, la regla debe ser explícita para no quedar librada a la interpretación del modelo. Corolario: la v1.1 también prohíbe **anunciar** o insinuar la persona owner a un interlocutor no-owner (nada de "estoy en modo dueño").

**R5 — La persona no se degrada a mitad de conversación por un pedido incómodo.** Un pedido que la persona owner no puede resolver (§3) deriva a la consola de administración; no reabre la persona cliente ni "olvida" el rol.

### 1.3 Estructura del template (forma, no texto final)

La v1.1 conserva el orden de secciones de la casa —When to Use → Procedure → Pitfalls → Verification (E27)— y agrega un bloque de selección de persona antes del procedimiento, con el procedimiento cliente **textualmente intacto** y un procedimiento owner aparte. Motivo: el contrato cliente está verificado en vivo por el smoke de B2 (E45) y por el spec de `feat-hermes-receptionist-skill`; reescribirlo dentro de este change sería un cambio de comportamiento no pedido.

Esbozo (los textos finales son de T2 + ronda de owner, ver `tasks.md` OQ-1/OQ-2):

```
## When to Use .................. (canal del negocio, ambas personas)
## Persona ..................... (R1..R4, tabla de selección)
## Procedure — persona cliente . (v1.0.0, sin cambios)
## Procedure — persona owner ... (operación + deletes con doble gate + TUI-only)
## Pitfalls ..................... (v1.0.0 + pitfalls de persona)
## Verification ................. (v1.0.0 + escenarios de persona)
```

---

## 2. S2 — Doble gate en chat para deletes

### 2.1 Precedente: el doble gate de `purge-inactive`

`purge-inactive` es el único borrado permanente ya entregado y su UX fija el patrón que este change reusa (decisión del owner: "el pürg pattern", E4):

1. **Gate 1 — confirmar la acción.** Se resuelve el lote real, se muestran los ítems afectados y se pregunta explícitamente (`internal/tui/purge_flow.go:43-52`, E30). Los **ids viajan capturados** en el estado del confirm, para que los gates no puedan divergir del lote real (`:15-17`, E30).
2. **Gate 2 — confirmar la permanencia.** El primer gate aceptado no ejecuta nada: abre un segundo confirm que repite el resumen y explicita la consecuencia irreversible — "ÚLTIMA CONFIRMACIÓN: la eliminación es permanente y sin vuelta atrás" — con **los mismos ids** (`internal/tui/model.go:461-473`, E31; el tipo `confirmPurgeFinal` documenta el propósito en `:66-70`).
3. **Resumen único compartido.** Las dos pantallas repiten el mismo resumen (`admin.PurgeQuestion`, `internal/admin/purge.go:87-98`, E32): una sola fuente de verdad de qué se va a borrar.
4. **Resultado honesto por fila.** La ejecución reporta el resultado de cada ítem, distingue lote completo de parcial y nombra lo que realmente se borró (`internal/tui/purge_flow.go:68-101`).

### 2.2 El gate conversacional (misma semántica, otro transporte)

| Paso | Regla del chat | Análogo TUI |
|---|---|---|
| G0 — resolver | El owner nombra el recurso; Hermes lo resuelve con la tool de lectura **disponible** (matriz §2.3) y determina **un** objetivo concreto. Cuando no hay lectura posible, el objetivo es lo que el owner declaró y Hermes lo dice | `purgeDataCmd` resuelve el lote antes del gate (E30) |
| G1 — confirmar acción | Hermes dice exactamente qué va a borrar (nombre + identificador cuando hay lectura; el par declarado cuando no la hay, §2.3) y qué efecto lateral puede tener (p. ej. conflicto de FK con turnos, que el server devuelve como error semántico) y pide confirmación | primer confirm (E30) |
| G2 — confirmar permanencia | Sólo si el owner respondió afirmativamente a G1, Hermes abre un **turno nuevo**: repite el objetivo y explicita que la eliminación es permanente y sin vuelta atrás, y pide un segundo "sí" | `confirmPurgeFinal` (E31) |
| G3 — ejecutar | Sólo con el segundo "sí" explícito Hermes llama la tool de delete **una vez**, con **el id capturado en G1** | `purgeFinalCmd` con los ids de `confirmState` (E31) |
| G4 — reportar | Hermes reporta el resultado real que devolvió la tool (identidad + `status`, E48) y, cuando existe lectura posterior (§2.3), la verifica; sin adornos ni claims de éxito no verificados | `purgeSummary` por fila (E30) |

### 2.3 Matriz de verificabilidad (lo que el chat PUEDE y NO PUEDE comprobar)

Evidencia: formas de input de los tres deletes (E46) e inventario real de tools (E47).

| Delete | Identidad que recibe la tool | Lectura previa (G0) | Verificación posterior (G4) |
|---|---|---|---|
| `delete_service` | `service_id` (`internal/mcp/tools_maintenance.go:205-207`) | `search_services_advanced` → nombre + id reales | `search_services_advanced`: el servicio ya no aparece |
| `delete_schedule` | `professional_id` + `day_of_week` (`:246-249`) | **ninguna**: no existe tool MCP de lectura de profesionales ni de agendas (E47) | **ninguna** más allá del payload de la tool (`professional_id`, `day_of_week`, `status`, E48) |
| `delete_hours_exception` | `exception_id` (`internal/mcp/tools_hours_exception.go:45-48`) | `list_hours_exceptions` sobre el rango que incluya la fecha | `list_hours_exceptions`: la excepción ya no aparece |

**Consecuencia de diseño (honesta):** para `delete_schedule` el gate no puede "resolver el lote" como hace la TUI; sólo puede repetir el par declarado por el owner (`professional_id` + día) y exigir las dos confirmaciones sobre ese par. Reglas derivadas:

- La skill MUST NOT afirmar que verificó la agenda, ni inventar el `professional_id` ni el nombre del profesional.
- Si el owner no puede dar el par exacto (o duda entre dos profesionales), el gate se abre igual pero con la advertencia explícita de que el chat no puede listar agendas: es la única salida honesta sin una tool de lectura nueva.
- **Si el owner quiere verificación real de agendas por chat, eso es una S nueva** (candidata: una tool de lectura de profesionales/agendas) con su evidencia — no se inventa acá ni se implementa en T2 (`odd/tasks/feat-owner-chat-b12.md:21-22`).

**Reglas duras del gate** (todas verificables en el texto de la skill):

- **Dos turnos, no dos frases.** G2 exige un mensaje **entrante nuevo** del owner. Hermes no puede autoconfirmarse, ni combinar las dos confirmaciones en un mismo mensaje, ni aceptar "sí, sí" en un solo turno como dos gateadas.
- **No re-targeting.** El objetivo se fija en G1. Si el mensaje del owner en G2 menciona **otro** recurso distinto, el gate se cancela y se vuelve a G1 con el objetivo nuevo (nunca se "actualiza" el target en silencio).
- **Cancelación total ante ambigüedad.** Cualquier respuesta que no sea un sí inequívoco (duda, pregunta, cambio de tema, silencio) cancela el gate: no se ejecuta el delete y no se reabre sin un pedido explícito nuevo.
- **Un gate por delete.** No hay borrados en lote ni "borrá los tres" en un mismo gate: tres recursos = tres cadenas G1–G4. Razón: el gate es sobre un objeto identificable y verificable, y el precedente TUI gatea un lote cuyos ids están capturados — en chat no existe un carrier estructural equivalente, así que se acota el alcance.
- **Sin deletes fuera de persona owner.** Para la persona cliente, un pedido de delete sigue siendo escalamiento al titular (E26, regla 5): no hay gate que abrir.
- **Sin delete especulativo.** Hermes no llama la tool para "ver qué pasa" ni para descubrir el error; si la resolución de G0 es ambigua, pregunta antes de G1.

**Por qué el gate vive en la skill y no en el servidor:** el servidor no puede distinguir "el owner ya confirmó" de "el owner pidió" — no tiene estado conversacional, y agregarlo convertiría el MCP en un gestor de sesiones de chat, rompiendo ADR-0013/ADR-0020 Decisión 1 (E21). La consecuencia aceptada y explícita: **el gate es una garantía de UX, no un control de seguridad**; la seguridad es el RBAC owner-only que ya está en su lugar (E28, E29).

---

## 3. S3 — Exclusiones duras TUI-only

| Operación excluida del chat | Por qué es TUI-only | Frontera real |
|---|---|---|
| `purge-inactive` (purga de cuentas) | borrado permanente de cuentas; singleton destructivo con doble gate humano (`internal/tui/purge_flow.go`) | no existe como tool (E28) |
| Desactivación de cuenta | inhabilitación de acceso de una persona; gestión de cuentas fuera del alcance del LLM (ADR-0010/ADR-0015) | no existe como tool (E28) |
| Transfer Ownership | invariante de owner único; un error deja al negocio sin owner resoluble | no existe como tool (E28) |
| Listado del audit log | trail de auditoría: se registra siempre, nunca se renderiza en un chat (público potencialmente hostil) | no existe como tool (E28) |

**Texto exigido (S3, E5):** ante un pedido excluido, la persona owner responde derivando a la consola de administración ("por estos pedidos te derivo a la consola de administración") **sin** describir internos ni rutas inventadas. Prohibiciones explícitas para la v1.1:

- No ofrecer hacerlo "por otra vía", "desde acá con un comando" ni prometer una capacidad futura.
- No listar eventos de auditoría "resumidos" aunque el owner los pida: la exclusión cubre el **render en chat**, no sólo la operación.
- No responder con el error crudo del servidor si alguna tool no existe: la persona owner sabe de antemano que esas cuatro operaciones no son parte del chat.

**Nota de defensa en profundidad:** el audit sigue registrando las operaciones de la persona owner como cualquier mutación (`internal/application/usecase/*`, audit por mutación en la capa que conoce la mutación). La exclusión es de **render**, no de registro.

---

## 4. S4 — Gateway per-sender (frontera Hermes-side)

### 4.1 Qué se documenta en ESTE repo

`docs/gateway-contract.md` (T3) gana:

1. Una subsección de **interacción de la persona owner**: cuándo se activa (§1.2 R1/R2), qué puede pedir (E28), qué deriva a la TUI (§3), y el doble gate de deletes (§2) descrito como contrato conversacional del lado gateway/agent.
2. Una **checklist de aceptación** (§4.2) que el build Hermes-side debe cumplir. El protocolo de identidad **no cambia**: la sección existente de inyección per-sender y el loop de registro se conservan tal cual (E11, E12).

### 4.2 Checklist de aceptación del build Hermes-side (deliverable del otro repo)

| # | Criterio | Fuente |
|---|---|---|
| C1 | `X-Caller-Id` se inyecta **por emisor y por request**, verbatim, sin normalizar | E11 |
| C2 | Emisor desconocido (`-32000` "no te reconozco") → nombre opcional → `register_client` → retry **una** vez | E12 |
| C3 | `-32002` y `-32603` no se reintentan ni se realimentan a `register_client` | E12 |
| C4 | Bot privado de Telegram: allowlist del chat del owner; `MCP_TELEGRAM_OWNER_PHONE` inyectado **estáticamente**; todo emisor fuera de la allowlist se rechaza **en el gateway** antes de cualquier tool call | E13, E16, E20 |
| C5 | Selección de persona: WhatsApp con emisor resuelto a `accounts` owner → persona owner; cualquier otro caso → persona cliente (fail-closed) | §1.2 R2/R3 |
| C6 | El body de la request **nunca** transporta rol ni persona; el server los ignora | E10 |
| C7 | Ningún caller id se cachea entre emisores (sin estado conversacional compartido) | E11 |
| C8 | La skill v1.1 está instalada y indexada **antes** del primer mensaje de la conversación | E17, E21 |
| C9 | Todo el tráfico al server va por `127.0.0.1:3000` (invariante loopback) | E17 |
| C10 | Un pedido de delete del owner produce **una sola** llamada a la tool, después de las dos confirmaciones (§2.2 G3/G4) | §2, E31 |

Ninguno de C1–C10 se valida en este repo: se validan en el repo del gateway. Este repo sólo publica el contrato (T3) y verifica que el documento sea coherente con el protocolo ya entregado.

### 4.3 Por qué no se implementa acá

Verificado el 2026-10-19 (`odd/tasks/feat-owner-chat-b12.md:13`, E6): `scripts/` contiene `install.sh` y sus tests; no hay cliente de mensajería, ni adaptador de WhatsApp/Telegram, ni nada que lea un `from`. El gateway es código de la plataforma Hermes. Implementar un transporte de mensajería en este repo violaría ADR-0018 (el server es un pipe MCP detrás de loopback) y ADR-0013 (el transporte del servidor es MCP/SSE).

---

## 5. S5 — Runbook de operador: Transfer Ownership + `messenger_platform`

Ambos pasos son capacidades **existentes** (E14, E15); el change sólo los ordena y les fija post-condiciones.

### 5.1 Secuencia

| # | Paso | Quién | Post-condición verificable |
|---|---|---|---|
| 1 | `mcp-server admin tui` → **Transferir ownership** → teléfono real `+5492617132740` + nombre visible | owner (hands) | el sucesor resuelve como `owner`; el invariante de owner único se mantiene durante la transferencia en dos pasos; queda entrada de auditoría de transferencia |
| 2 | `update_business_profile` con `messenger_platform: "whatsapp"` y `messenger_id` = número del negocio | persona owner por chat, o tools/TUI | `get_business_profile` devuelve `messenger_platform = whatsapp` y `messenger_id` = número del negocio |
| 3 | Re-ejecutar la opción 7 "Configurar Hermes" (cascade de skill, §6) | owner (hands) | skill instalada = render de v1.1; claves del owner-bot escritas con el teléfono **real** |
| 4 | Smoke mínimo de persona | owner | ver §5.3 |

**Restricción de orden (la única dura):** el paso 1 debe preceder al paso 3. La opción 7 persiste el teléfono del owner en la config de Hermes (`MCP_TELEGRAM_OWNER_PHONE`, E16) y esa inyección estática es la identidad de la persona owner en Telegram (E20); correrla antes de la transferencia deja el teléfono **demo** como identidad estática. Los pasos 2 y 3 son independientes entre sí.

**Efecto esperado del paso 2 sobre el canal:** `messenger_platform` describe el canal del negocio (WhatsApp primario, E18). El bot privado de Telegram **no** depende de ese campo: es un canal owner-only separado con su propia config (E20). Cambiar el campo no rompe el acceso del owner por Telegram.

### 5.2 Rollback

- Transfer Ownership no se deshace desde el chat ni desde la TUI con un botón: si el teléfono real es incorrecto, se repite la transferencia al teléfono correcto (invariante de owner único, E14). Anotarlo en el runbook evita que alguien busque un "undo" inexistente.
- `messenger_platform` se revierte con la misma tool (`update_business_profile`), sujeto a la validación del `CHECK` del schema (`whatsapp` | `telegram` | NULL).

### 5.3 Verificación de aceptación del runbook

1. Un mensaje desde el teléfono real por WhatsApp resuelve como owner y la conversación entra en persona owner (requiere C1/C5 del gateway).
2. Un mensaje desde el bot privado de Telegram entra en persona owner (R1).
3. Un mensaje desde un teléfono desconocido conserva persona cliente y loop de registro (R3, E12).
4. Un pedido de delete dispara la cadena G1–G2 y **no** ejecuta nada antes del segundo "sí" (C10).
5. Un pedido de purge/transfer/audit deriva a la consola de administración (§3).

---

## 6. S6 — Cascada: la skill v1.1 llega a hosts instalados

No hay código nuevo: el mecanismo está entregado y verificado.

| Paso | Mecanismo | Evidencia |
|---|---|---|
| Fuente de verdad | template del repo `setup/templates/recepcionista-negocio/SKILL.md` | E37 |
| Host nuevo | `install.sh` renderiza y siembra la skill en `~/.hermes/skills/recepcionista-negocio/SKILL.md` | E36, E40 |
| Host existente sin modificar | opción 7 detecta `SkillStateUpToDate` y no reescribe | E38 |
| Host con la v1.0.0 instalada (caso real) | opción 7 detecta `SkillStateModified` (los bytes difieren), **exige consentimiento explícito** y recién entonces reemplaza con snapshot de bytes | E38, E39 |
| Host sin skill | opción 7 escribe sin fricción (`SkillStateMissing`) | E38 |
| Nunca sobrescribe en silencio | `install.sh` avisa y deriva a la opción 7 ante una skill modificada | E40 |
| Cobertura | tests del seed (`skills existentes`, `modificada no se sobrescribe`) | E41 |
| Precedente en vivo | cascada corrida por el owner sobre v0.8.0 durante el smoke B2 | E45 |

**Fricción esperada y honesta:** en un host con la v1.0.0 instalada, la opción 7 va a reportar "modificada" — no porque el owner la haya editado, sino porque el template cambió. El runbook debe decir eso explícitamente para que el operador no interprete el warning como una edición propia perdida. El consentimiento sigue siendo obligatorio (es el mecanismo que protege una skill realmente editada por el usuario).

---

## 7. Restricciones de render (charset, placeholder, versión)

| Restricción | Regla | Evidencia |
|---|---|---|
| Versión | `version: 1.1.0` en el frontmatter (bump menor: agrega persona, no rompe el contrato cliente) | E26 (`:4`) |
| Placeholder único | `{business_name}` es la **única** variable que el renderer sustituye; `{nombre}` es variable de runtime y debe sobrevivir verbatim | E36 |
| Charset del nombre | el nombre del negocio sigue pasando por `entity.BusinessNameHasUnsafeChars` (marcado markdown + delimitadores) | E33, E35 |
| Cotas del nombre | ≤ 120 runas, sin `\r\n\t` | E34 |
| Compatibilidad bash/clave | el render de `install.sh` sustituye por línea con parameter expansion; ningún texto nuevo puede depender de otros placeholders | E36 |
| Slug | `recepcionista-negocio` no cambia (ruta instalada) | E37 |
| Idioma | español por defecto; si el owner escribe en otro idioma, responder en ese idioma (regla cliente existente, se hereda) | E26 |
| Regla de oro del texto | todo el contenido nuevo es **instrucción de conducta**, sin `path` de repo, sin nombres de tablas, sin ids internos, sin nombres de variables de entorno de la config del server | E17 |

**Riesgo aceptado:** la v1.1 es más larga que la v1.0.0, y la skill se carga por progressive disclosure (índice name+description, texto completo on demand — E17). Mitigación de diseño: el bloque de selección de persona va **primero** y es corto, y la `description` del frontmatter debe seguir ≤ 60 caracteres (convención del change anterior de la skill) sin mencionar "owner" — si la descripción anunciara una persona owner, todo interlocutor vería ese texto en el índice del system prompt, insinuando una capacidad que no le corresponde (R4). El detalle de persona queda en el cuerpo, adonde llega la conversación real.

---

## 8. Impacto en código del servidor

**Ninguno esperado**, y este diseño es explícito sobre por qué:

1. Todas las tools de la persona owner ya existen (`cmd/mcp-server/main.go:437-450`, E28).
2. Todas ya exigen owner dos veces (E29).
3. Las cuatro operaciones excluidas no existen como tools (E28), así que la exclusión no necesita gate de servidor: es imposible alcanzarlas por MCP.
4. El doble gate de deletes es conversacional por diseño (§2), y agregarlo al servidor exigiría estado de sesión de chat, que rompe ADR-0013/ADR-0020 Decisión 1 (E21).
5. El gateway es de otro repo (§4.3).

**Si el apply descubre lo contrario** (por ejemplo: una tool de lectura que la persona owner necesita y no existe, o un gap de RBAC), la regla es la del feature doc: se registra como **S nueva con evidencia `path:line`** antes de escribir código (`odd/tasks/feat-owner-chat-b12.md:21-22`, E9). No se resuelve "de paso" en T2–T4.

---

## 9. Riesgos y mitigaciones

| Riesgo | Impacto | Mitigación |
|---|---|---|
| El LLM no obedece el doble gate | un delete se ejecuta con una sola confirmación | aceptado y declarado (§2.2): el gate es UX; el RBAC es la seguridad. Los deletes son recuperables recreando el recurso; el runbook lo dice |
| `delete_schedule` no es verificable por lectura | el owner confirma a ciegas un par profesional+día | se declara en la skill (§2.3), nunca se afirma verificación; si se necesita de verdad, S nueva con tool de lectura |
| La persona owner intenta operar agendas/profesionales sin lectura previa | pedidos ambiguos, confirmaciones sobre datos que el chat no puede mostrar | G1 admite el estado "no puedo listarlo": se pide el par exacto antes de abrir el gate |
| El gateway no distingue owner de cliente en WhatsApp | persona owner inactiva en WhatsApp (queda cliente) o, peor, activa por error | R3 fail-closed + C5 en la checklist §4.2 |
| Promoción de persona por contenido del mensaje | escalada de UX hacia tools owner-only | R4 explícita en el texto + pitfall dedicado en la skill |
| Regresión de la persona cliente | rompe comportamientos verificados en B2 | procedimiento cliente copiado sin cambios; escenarios de la persona cliente repetidos en `Verification` |
| Inyección por el nombre del negocio | markdown/instrucciones inyectadas en la skill | gates existentes en dominio y render (E33, E34, E35) — la v1.1 no agrega placeholders |
| El operador interpreta el warning "modificada" como pérdida de edición | reemplazo no consentido o cancelación innecesaria | §6 lo documenta explícitamente en el runbook |
| La description de la skill anuncia la persona owner | insinúa capacidad a todo interlocutor | restricción de la `description` en §7 |

---

## 10. Open questions diferidas (no bloquean este doc; bloquean el texto literal de T2)

Ver `tasks.md` §Phase 1 — OQ. Todas son de **redacción**, no de alcance: las decisiones de producto (doble gate, TUI-only, selección por canal/rol, fail-closed) están cerradas por el owner (`odd/tasks/feat-owner-chat-b12.md:10-15,35-36`, E3–E9).
