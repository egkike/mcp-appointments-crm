# ADR-0020: Política conversacional de Hermes en el canal del negocio (skill recepcionista)

- **Status**: accepted
- **Date**: 2026-10-16
- **Authors**: Kike
- **Related**: ADR-0018 (modelo de canales), ADR-0017 (config de Hermes / TUI opción 7), ADR-0009 (modelo de autorización), `docs/gateway-contract.md` (identidad per-sender), `openspec/changes/feat-hermes-receptionist-skill/` (spec del change doc-first)

## Context

ADR-0018 congeló el modelo de canales y `feat-whatsapp-bot` entregó el lado server del
registro self-service. Pero el contrato conversacional de Hermes quedó solo en specification
en dos dimensiones y completamente indefinido en una tercera:

1. **`docs/gateway-contract.md` cubre identidad, no conversación.** Define inyección
   `X-Caller-Id` per-sender, el loop de registro del desconocido y la interpretación de los
   códigos JSON-RPC. No dice una palabra sobre qué debe responder Hermes ante mensajes que no
   versan sobre el negocio.
2. **El registro automático amplía la superficie conversacional.** Con `register_client`
   entregado (10/hora por teléfono, fail-closed), cualquier desconocido que escriba al número
   del negocio queda registrado como cliente y puede conversar libremente con Hermes. Sin
   política explícita, el dueño terminó de dar un asistente generalista gratuito sobre su
   número de WhatsApp: consultas de clima, chistes, consejos — y cada conversación es tokens
   del LLM que corre en la VPS del dueño.
3. **El server no puede ver estos mensajes.** El MCP es un pipe de tool calls: Hermes solo
   contacto el servidor cuando decide llamar una tool. Un mensaje tipo "Hola" o "¿qué tiempo
   hace?" nunca llega al server — vive íntegramente dentro del runtime de Hermes. Cualquier
   control tipo interceptor o filtro de mensajes es imposible desde el lado MCP sin convertir
   el server en un proxy de chat, lo que rompería la arquitectura de ADR-0013/0018.

La pregunta es: ¿dónde vive la política conversacional de Hermes? Verificado contra el repo
el 2026-10-16: no existe ningún artefacto de política, prompt de sistema ni skill para las
conversaciones del canal del negocio. El único punto de contacto nuestro con el home de
Hermes es `~/.hermes/config.yaml`, escrito por la TUI opción 7 (ADR-0017).

## Decision

### Decisión 1 — La política conversacional es un artefacto instalado, no código Go

La política se define como una **skill estática de Hermes** (un archivo de instrucciones en texto plano,
 `SKILL.md`) escrita en el home de Hermes en el layout oficial (`~/.hermes/skills/<slug>/`,
 verificado contra `hermes-agent.nousresearch.com/docs/user-guide/features/skills`; ver Consecuencias), cuyo contenido vive
 versionado en este repo como template (`setup/templates/recepcionista-negocio/SKILL.md`)
 y se instala por `scripts/install.sh` (fresh install) con regeneración como segundo paso de la
 TUI opción 7 "Configurar Hermes" (decisión del owner 2026-10-16: sin item nuevo de menú).

Motivos (consecuencia directa del punto 3 del Context):

- **La skill debe estar cargada ANTES del primer mensaje.** Un "Hola" no dispara tool calls;
  una política dinámica vía tool (que Hermes tendría que fetchear) no puede existir: el mensaje
  ya se procesó sin ella. La inyección en el setup es el único modelo causalmente válido.
- **Defense-in-depth intacta**: la skill es texto, sin secretos, sin permisos, sin lógica.
  El RBAC del server (ADR-0009) es y sigue el único gate de privilegios; la skill nunca
  restringe (no puede) — solo **modela** cómo actúa Hermes con la superficie que el server ya
  le dio. La seguridad real sigue siendo: loopback + `X-Caller-Id` + RBAC + rate limits.
- **El server queda sin cambios**: no hay nuevas tools MCP ni schema. La superficie MCP que la
  skill usa ya existe (`get_business_profile`, `get_services`, `check_availability`,
  `get_professionals`, herramientas de bookings/archivos; el set completo de 25 tools —
  `search_professionals_advanced` se sumó después, por feat-owner-chat S7a, y
  `get_professional_schedules` (owner-only, doble capa) por S7b, que cierra el
  read-gap del `delete_schedule` en el doble gate).

### Decisión 2 — Rol e identidad: asistente del negocio, no asistente general

La skill declara que Hermes es el **asistente del negocio** cargado en
`business_profile.name` (placeholder reemplazado al instalar), con persona declarada:

- nunca es un asistente generalista; el universo de temas es el negocio;
- jamás se presenta como humano;
- las afirmaciones sobre el negocio salen solo de las tools y del perfil del negocio (sin
  datos de otros clientes: solo info pública del perfil).

### Decisión 3 — Mensajes fuera de tema: cordial y orientador (decisión del owner 2026-10-16)

Para cualquier mensaje cuya materia no es el negocio ("Hola" solo, clima, chistes, consejos):

- **No** responder el contenido off-topic (sin chistes, sin consejos, sin clima).
- Responder cordial en **1–2 oraciones**: saludar (con nombre si lo obtuvo del registro),
  declarar su rol ("en este número atiendo lo del negocio") y ofrecer los 2–3 servicios
  principales del negocio (turnos, horarios, servicios).
- **No** llamar tools para mensajes off-topic: no gastar tokens de LLM ni arrancar flujos.

Ejemplo del template (placeholders resueltos al instalar):

> «¡Hola {nombre}! En este número te atiende el asistente de {business_name}: te puedo
> ayudar con turnos, horarios y servicios. ¿Querés ver si hay algún turno disponible?»

La decisión del tono es del owner y quedó registrada: "Cordial y orientador — Saluda con
nombre y ofrece ayuda del negocio en un párrafo breve (1-2 oraciones); sin responder el
off-topic."

### Decisión 4 — Registro del desconocido y privacidad dentro de la skill

El loop de `docs/gateway-contract.md` (pedir nombre opcional → `register_client` → retry
once) se incorpora como guía de la skill, con agregados de privacidad explícitos:

- pedir el nombre de forma opcional (si no lo da, continuar sin nombre; el server guarda
  `Cliente {phone}`);
- pedir exclusivamente el dato que la tool necesita — nunca datos de más "para tenerlo en
  el archivo";
- no exponer datos PII de otros clientes ni referencias internas (no ids crudos, no SQL,
  no mensajes de error de sistema: solo los mensajes semánticos en español que el server
  devuelve, que ya están sanitizados por `internal/mcp/errors.go`).

### Decisión 5 — Pedidos que exceden el rol → escalamiento al dueño, sin inventar

Cuando un pedido excede su rol (p. ej. negociaciones de precio fuera de lo sembrado,
pedidos de deletes, cobertura de agenda especial): ofrecer contacto del titular de forma
cordial, o (si el owner-bot del canal privado está configurado) derivar la consulta al owner
(sin listar datos crudos). Simplemente **decir "no lo resuelvo"** y ofrecer contacto humano
está bien — la regla que prohíbe es inventar datos del negocio o promesas fuera de
`business_profile`.

## Consecuencias

**Positive**:

- Definición formal del ámbito conversacional de Hermes sobre el canal del negocio, con
  costo de tokens acotado por diseño (off-topic corta la conversación, no la alimenta).
- El dueño no se convierte en un chatbot gratuito: la puerta es el negocio, no el modelo.
- No hay Go nuevo ni schema nuevo: el change es doc-first con un template de skill y
  como mucho una opción de TUI y su bloque en install.sh (change de apply posterior).
- Consistente con la familia: igual patrón que TUI opción 7 que ya escribe en `~/.hermes/`,
  mismo estándar "el server entrega superficie, el contrato documenta el próximo paso".

**Negative**:

- La skill es texto estático: cambios de política = re-instalar (por eso la opción de TUI de
  regeneración en la Decisión 1).
- **Reconciliación cerrada (2026-10-16, instrucción del owner)**: el layout `~/.hermes/skills/<slug>/SKILL.md` con frontmatter SKILL.md + progressive disclosure (índice name+description en el system prompt, texto completo on demand vía `skill_view`) es el estándar oficial, verificado contra `https://hermes-agent.nousresearch.com/docs/user-guide/features/skills`. Nieto que la Política es válida: la descripción y "When to Use" de la skill se redactan como aplicables a toda conversación del canal del negocio.
- No está garantizado por código que Hermes en verdad cargue la skill: es una política de
  runtime del harness de Hermes, no un contract del server. El nivel de cumplimiento lo
  marca el runtime de Hermes, y el smoke test del change de apply lo debe verificar.

## Follow-ups (fuera de scope, registrados)

- El smoke de "Hermes contesta 'Hola' sin herramientas y con el mensaje cordial" va en el change de apply (el formato de skill ya quedó resuelto contra la doc oficial).
- Si el negocio pide tono configurable (hoy no: el owner eligió un default), evaluar un
  placeholder overlay (`~/.hermes/skills/<name>/overrides.yaml`) en un change posterior.
- Integración parcial en la familia de skills del propio repo: la skill del template es
  para el runtime de Hermes, no para el agente de coding de este repo.
