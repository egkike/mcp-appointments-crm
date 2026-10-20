# Proposal: feat-owner-chat — persona owner en la skill v1.1 (dos personas), reglas de gateway y pasos de operador

> **Change:** feat-owner-chat
> **Status:** proposed (spec phase) — doc-first; el apply (skill template + gateway-contract + runbook) va en un change posterior (T2–T5 de `odd/tasks/feat-owner-chat-b12.md`)
> **Inputs:** `odd/tasks/feat-owner-chat-b12.md` (especificación autoritativa, S1–S6 congeladas, `:10-15`), `odd/tasks/backlog.md:23` (B12), `odd/tasks/backlog.md:51` (L22, decisión del owner), `docs/gateway-contract.md`, ADR-0018, ADR-0020, ADR-0013, ADR-0015, `setup/templates/recepcionista-negocio/SKILL.md` (v1.0.0)
> **Other artifacts:** `design.md`, `specs/owner-chat/spec.md`, `tasks.md`
> **Evidence base:** cada afirmación cita `path:line` (inventario completo en "Evidence Inventory")

---

## Executive Summary

El canal owner-only de Telegram existe desde la feature 1.21 (`docs/PRD.md:1466`) y la skill recepcionista desde la 1.22 (`docs/PRD.md:1467`), pero la skill v1.0.0 implementa **una sola persona**: el cliente. El owner que escribe por su bot privado de Telegram —el único chat que la allowlist admite (`docs/architecture/0018-communication-channels.md:60-66`)— recibe el tratamiento de un cliente: saludo cordial genérico y, ante cualquier pedido de operación, la regla 5 de la skill lo deriva a un humano (`setup/templates/recepcionista-negocio/SKILL.md:40-43`).

Eso contradice la razón de ser del canal: el owner habilitó Telegram exactamente para "pedir todo desde ahí" (`odd/tasks/feat-owner-chat-b12.md:10`). Este change especifica la **skill v1.1 con dos personas** (cliente sin cambios + owner), el **doble gate en chat** para deletes, las **exclusiones TUI-only**, la **actualización del contrato del gateway** con la checklist de aceptación del build Hermes-side, y los **pasos de operador** (Transfer Ownership + `messenger_platform` + re-aplicación en cascada de la skill).

**No hay código Go en este change**: la superficie MCP de operación ya existe y es owner-only (`cmd/mcp-server/main.go:437-450`, `internal/application/usecase/*.go` con `auth.RequireRole(auth.RoleOwner)`); el gateway per-sender es código de la plataforma Hermes, fuera de este repo (`odd/tasks/feat-owner-chat-b12.md:13`).

## Business Problem

- **El owner es tratado como cliente en su propio canal.** La skill v1.0.0 no tiene ninguna noción de rol: su "When to Use" cubre "TODA conversación que entra por el canal de mensajería del negocio" (`setup/templates/recepcionista-negocio/SKILL.md:11-15`) y su regla 5 manda los pedidos de operación (incluido "borrar datos") al titular (`:40-43`). En el bot privado de Telegram el titular **es** el interlocutor: la regla produce un absurdo (el owner derivándose a sí mismo).
- **La superficie de operación por chat ya está construida y es owner-only.** Once tools de mantenimiento (perfil, servicios, profesionales, agendas, excepciones de horario) tienen doble gate de RBAC: mapa en el composition root (`cmd/mcp-server/main.go:437-450`) + `auth.RequireRole(auth.RoleOwner)` en cada use case (`internal/application/usecase/delete_service.go:39`, `delete_schedule.go:40`, `delete_hours_exception.go:35`, `update_business_profile.go:50`, entre otros). La capacidad existe; lo que falta es la **política conversacional** que la use desde el canal correcto.
- **Sin política de persona, el texto de la skill sólo produce fricción.** El owner tiene que salir del chat e ir a la TUI para cualquier cambio operativo, aunque la tool que lo hace esté a un mensaje de distancia.
- **El canal WhatsApp sigue sin resolución per-sender del lado Hermes.** El contrato de identidad está documentado (`docs/gateway-contract.md:9-20`, `:29-46`) pero el gateway no está implementado en este repo (`odd/tasks/feat-owner-chat-b12.md:13`). Sin la regla explícita de selección de persona, la persona owner quedaría indefinida en WhatsApp hasta que ese build exista — y podría "resolverse" por heurísticas peligrosas (por ejemplo, creerle al mensaje cuando dice "soy el dueño").

## Product Outcome

- **Dos personas explícitas, seleccionadas por canal/rol — nunca por el contenido del mensaje.** En el bot privado de Telegram la persona es **owner** porque el canal es owner-only por configuración; en WhatsApp la persona es **owner** sólo cuando el emisor resuelve a `accounts` con rol owner (post-gateway), y **cliente** en cualquier caso ambiguo (fail-closed).
- **El owner opera el negocio desde el chat**: perfil, servicios, profesionales, agendas, excepciones de horario — con las mismas tools y el mismo RBAC que hoy.
- **Los deletes exigen doble confirmación en chat** (confirmar acción + confirmar permanencia), replicando el patrón ya entregado y probado de `purge-inactive` (`internal/tui/purge_flow.go:43-52`, `internal/tui/model.go:461-473`).
- **Cuatro operaciones siguen siendo TUI-only por diseño** (purge de cuentas, desactivación de cuentas, Transfer Ownership, listado de audit): la skill lo dice explícitamente y deriva a la consola de administración, sin describir internos.
- **El contrato del gateway publica las reglas de interacción de la persona owner y la checklist de aceptación** que el build Hermes-side debe satisfacer.
- **La skill v1.1 llega a los hosts ya instalados** por el camino que ya existe: la opción 7 detecta `SkillStateModified` y ofrece el reemplazo con consentimiento (`internal/admin/skill.go:196-225`, `:533-560`).

## Scope

> Mapeo 1:1 de las specs del feature doc (`odd/tasks/feat-owner-chat-b12.md:10-15`). Sin agregados, sin recortes.

### S1 — Persona owner en la skill v1.1

- **Deliverable:** `setup/templates/recepcionista-negocio/SKILL.md` pasa a **dos procedimientos**: persona cliente (v1.0.0, texto y comportamientos sin cambios) y persona **owner**, seleccionada por (a) canal Telegram owner-bot —clave de canal, no de teléfono— y (b) emisor WhatsApp que resuelve a `accounts` owner una vez que el gateway per-sender exista.
- **Alcance operativo de la persona owner:** `update_business_profile`, `create/update/delete_service`, `create/update_professional`, `upsert/delete_schedule`, `create/delete/list_hours_exceptions` — las tools owner-only ya existentes (`cmd/mcp-server/main.go:437-450`).
- **Evidencia actual:** la v1.0.0 no distingue roles (`setup/templates/recepcionista-negocio/SKILL.md:11-15`, `:40-43`) y el frontmatter declara `version: 1.0.0` (`:4`).

### S2 — Deletes por chat con doble gate (decisión del owner)

- **Deliverable:** `delete_service`, `delete_schedule` y `delete_hours_exception` se permiten en la persona owner **sólo** detrás de un doble gate conversacional: (1) confirmar **la acción** con el recurso exacto identificado, (2) confirmar **la permanencia** ("no se puede deshacer") con un segundo "sí" explícito en un turno posterior.
- **Precedente citado:** el doble gate de `purge-inactive` (primer gate `internal/tui/purge_flow.go:43-52`; segundo gate con la consecuencia permanente explícita `internal/tui/model.go:461-473`; resumen repetido por `admin.PurgeQuestion`, `internal/admin/purge.go:87-98`).
- **Decisión del owner (verbatim en el feature doc):** "owner puede borrar servicios/schedules/horas-exceptions via chat con doble confirmación explícita" (`odd/tasks/feat-owner-chat-b12.md:11`).

### S3 — Exclusiones duras TUI-only

- **Deliverable:** la skill v1.1 declara que la persona owner **no** opera por chat: `purge-inactive` (purga de cuentas), desactivación de cuentas, Transfer Ownership y listado del audit log. La respuesta es una derivación explícita a la consola de administración, sin internos ni promesas de hacerlo "por otra vía".
- **Evidencia de que la frontera ya es física:** ninguno de esos cuatro nombres existe en el mapa de RBAC de tools (`cmd/mcp-server/main.go:427-450`); son capacidades de TUI (`internal/tui/purge_flow.go`, `internal/admin/purge.go`) y la gestión de cuentas queda fuera del alcance del LLM por ADR-0010/ADR-0015.

### S4 — Gateway per-sender (frontera Hermes-side)

- **Deliverable en ESTE repo:** `docs/gateway-contract.md` actualizado con (a) las reglas de interacción de la persona owner y (b) la **checklist de aceptación** que el build Hermes-side debe satisfacer (inyección per-sender, loop 401→`register_client`→retry, no-reintento en `-32002`/`-32603`, inyección estática del owner-bot de Telegram, selección de persona por rol resuelto, prohibición de transportar rol/persona en el body).
- **Fuera de este repo (deliverable de otro repositorio, owner-owned):** la implementación del gateway en el código de la plataforma Hermes. Este repo no tiene superficie de gateway (`odd/tasks/feat-owner-chat-b12.md:13`).
- **Evidencia actual:** protocolo de identidad `docs/gateway-contract.md:9-20`; loop de registro `:29-46`; Telegram `request_contact` + owner-bot estático `:66-68`; claves del owner-bot `:106-116`.

### S5 — Pasos de operador (Transfer Ownership + `messenger_platform`)

- **Deliverable:** runbook exacto: Transfer Ownership del owner demo (`+5491100000000`) al teléfono real (`+5492617132740`) por la TUI de administración, y luego `messenger_platform` → `whatsapp` en el perfil del negocio vía `update_business_profile`, con post-condiciones verificables.
- **Evidencia actual:** ambos pasos ya están documentados como capacidades existentes (`docs/gateway-contract.md:82-96`); no requieren código nuevo.

### S6 — Re-aplicación en cascada de la skill v1.1

- **Deliverable:** paso de operador documentado: la skill v1.1 vive en el template del repo (fuente de verdad); en hosts instalados la opción 7 detecta la diferencia (`SkillStateModified`) y ofrece el reemplazo con consentimiento (`internal/admin/skill.go:196-225`, `:533-560`). Sin código nuevo: ya verificado en vivo durante el smoke de B2 (`odd/tasks/backlog.md:50`).
- **Evidencia actual:** el seed de install.sh nunca sobrescribe una skill modificada (`scripts/install.sh:1106-1120`); el comportamiento está cubierto por tests (`scripts/tests/install_skill_test.sh:77-82`).

## Non-goals

- **Sin cambios de código del servidor.** Ni tools nuevas, ni schema, ni cambios de RBAC, ni cambios de transporte. Si el doc-first descubre una necesidad de código, debe surgir como una **S nueva con evidencia** (`odd/tasks/feat-owner-chat-b12.md:21-22`) — no se implementa acá ni se asume.
- **Sin implementación del gateway en este repo.** El build Hermes-side es un deliverable de otro repositorio (owner-owned); acá sólo se documenta el contrato y su checklist de aceptación.
- **Sin cambios a la persona cliente.** Los comportamientos verificados en B2 (saludo sin tool, loop de registro, privacidad) quedan intactos (`odd/tasks/feat-owner-chat-b12.md:22`).
- **Sin re-versionado de la skill en la TUI ni en install.sh.** El bump es de frontmatter (`1.0.0` → `1.1.0`), no del mecanismo de instalación.
- **Sin renombrar la skill ni el slug.** `recepcionista-negocio` es la ruta instalada (`internal/admin/skill.go:34-41`, `:193`); renombrar orfanaría las instalaciones existentes.
- **Sin gate adicional para writes no-delete.** La decisión del owner cubre deletes; ver OQ-3.
- **Sin cambios al PRD en esta fase.** El changelog del PRD se actualiza cuando el apply cierre (`T5`), no en el doc-first.

## Constraints & fences

Tomadas del feature doc (`odd/tasks/feat-owner-chat-b12.md:17-23`) y re-afirmadas acá porque son gates de diseño:

| Fence | Regla | Evidencia |
|---|---|---|
| Versionado | frontmatter `version: 1.1.0` | `setup/templates/recepcionista-negocio/SKILL.md:4` |
| Charset render-safe | el nombre del negocio sigue pasando por `entity.BusinessNameHasUnsafeChars` | `internal/domain/entity/business_profile.go:191-205`; gate en `internal/admin/skill.go:180` |
| Placeholder único | `{business_name}` es la **única** variable renderizada; `{nombre}` es variable de runtime y se preserva | `internal/admin/skill.go:47-49`, `:150-153`; `scripts/install.sh:1059-1069` |
| Identidad | el server nunca aprende el emisor de Telegram: la persona owner en Telegram se clavea por **canal** | `docs/architecture/0018-communication-channels.md:60-66`; `docs/gateway-contract.md:66-68` |
| RBAC doble | la persona es guía conductual, nunca capa de seguridad | `cmd/mcp-server/main.go:386-391`; ADR-0020 Consecuencias (`docs/architecture/0020-hermes-conversational-policy.md:110-137`) |
| Capas | sin código; dependencias hacia adentro intactas | ADR-0013 |

## Evidence Inventory

| ID | Afirmación | Evidencia |
|---|---|---|
| E1 | B12 registrado en el backlog con el alcance del paquete completo | `odd/tasks/backlog.md:23` |
| E2 | Decisión del owner: paquete completo + doble-gate para deletes, resto TUI-only | `odd/tasks/backlog.md:51` |
| E3 | S1: cita verbatim del owner y alcance operativo de la persona owner | `odd/tasks/feat-owner-chat-b12.md:10` |
| E4 | S2: deletes por chat con doble confirmación (decisión del owner) | `odd/tasks/feat-owner-chat-b12.md:11` |
| E5 | S3: exclusiones duras TUI-only + texto de derivación exigido | `odd/tasks/feat-owner-chat-b12.md:12` |
| E6 | S4: no hay superficie de gateway en este repo; el build es Hermes-side | `odd/tasks/feat-owner-chat-b12.md:13` |
| E7 | S5: teléfonos del Transfer Ownership y del `messenger_platform` | `odd/tasks/feat-owner-chat-b12.md:14` |
| E8 | S6: cascada por opción 7 con `SkillStateModified`, ya verificada en B2 | `odd/tasks/feat-owner-chat-b12.md:15` |
| E9 | Fences de versionado/charset/placeholder/identidad/RBAC | `odd/tasks/feat-owner-chat-b12.md:17-23` |
| E10 | El canal transporta identidad, nunca rol | `docs/gateway-contract.md:7` |
| E11 | Inyección `X-Caller-Id` per-sender, verbatim, por request | `docs/gateway-contract.md:9-20` |
| E12 | Loop de registro del desconocido (retry exactamente una vez) | `docs/gateway-contract.md:29-46` |
| E13 | Telegram: `request_contact` para el canal del negocio; el owner-bot inyecta un teléfono estático | `docs/gateway-contract.md:66-68` |
| E14 | Runbook existente: Transfer Ownership por TUI | `docs/gateway-contract.md:82-88` |
| E15 | Runbook existente: `messenger_platform` → `whatsapp` + verificación | `docs/gateway-contract.md:90-96` |
| E16 | Claves del owner-bot escritas por la opción 7; `MCP_TELEGRAM_OWNER_PHONE` con inyección estática | `docs/gateway-contract.md:106-116` |
| E17 | Sección de política conversacional y sus bounds (texto ≠ permiso) | `docs/gateway-contract.md:118-131` |
| E18 | Un solo canal del negocio: WhatsApp primario, Telegram alternativa | `docs/architecture/0018-communication-channels.md:19-26` |
| E19 | Identidad por emisor; el canal no otorga rol | `docs/architecture/0018-communication-channels.md:27-51` |
| E20 | Bot privado de Telegram: allowlist del chat del owner e identidad estática desde config | `docs/architecture/0018-communication-channels.md:60-66` |
| E21 | La política conversacional es un artefacto instalado, no código Go | `docs/architecture/0020-hermes-conversational-policy.md:37-58` |
| E22 | Rol: asistente del negocio, nunca asistente general ni humano | `docs/architecture/0020-hermes-conversational-policy.md:59-68` |
| E23 | Off-topic: cordial y orientador, tool-free (decisión del owner 2026-10-16) | `docs/architecture/0020-hermes-conversational-policy.md:69-87` |
| E24 | Loop de registro + privacidad dentro de la skill | `docs/architecture/0020-hermes-conversational-policy.md:88-100` |
| E25 | Escalamiento al owner sin inventar (regla que la persona owner reemplaza para operación) | `docs/architecture/0020-hermes-conversational-policy.md:101-109` |
| E26 | Skill v1.0.0: versión, "When to Use" único sin roles, regla 5 que deriva deletes al titular | `setup/templates/recepcionista-negocio/SKILL.md:4,11-15,40-43` |
| E27 | Estructura de la skill: Procedure / Pitfalls / Verification | `setup/templates/recepcionista-negocio/SKILL.md:17-44,46-55,57-63` |
| E28 | Mapa RBAC: tools de mantenimiento y excepciones owner-only; no existen tools de purge/deactivate/transfer/audit | `cmd/mcp-server/main.go:427-450` |
| E29 | Doble gate de RBAC: `auth.RequireRole(auth.RoleOwner)` dentro de cada use case | `internal/application/usecase/delete_service.go:39`, `delete_schedule.go:40`, `delete_hours_exception.go:35`, `update_business_profile.go:50`, `create_service.go:38`, `update_service.go:38`, `create_professional.go:44`, `update_professional.go:43`, `upsert_schedule.go:47`, `create_hours_exception.go:43`, `list_hours_exceptions.go:36` |
| E30 | Precedente del doble gate: primer gate con el lote resuelto e ids capturados | `internal/tui/purge_flow.go:43-52` |
| E31 | Precedente del doble gate: segundo gate con la consecuencia permanente explícita y los MISMOS ids | `internal/tui/model.go:461-473`, `:66-70` |
| E32 | Resumen que repiten los dos gates (no pueden divergir del lote real) | `internal/admin/purge.go:87-98` |
| E33 | Charset render-safe: fuente única de verdad | `internal/domain/entity/business_profile.go:191-205` |
| E34 | Cotas del nombre del negocio: 120 runas, sin `\r\n\t`, sin caracteres de marcado | `internal/domain/entity/business_profile.go:223-233`, `MaxBusinessNameLen` `:185-189` |
| E35 | Gate de render en `internal/admin` (segunda barrera) | `internal/admin/skill.go:155-186` |
| E36 | Placeholder único `{business_name}`; `{nombre}` preservado | `internal/admin/skill.go:47-49`, `:150-153`; `scripts/install.sh:1059-1069` |
| E37 | Slug/ruta instalada de la skill | `internal/admin/skill.go:34-41`, `:193` |
| E38 | Detección de estado: missing / up-to-date / modified | `internal/admin/skill.go:196-225` |
| E39 | Máquina de reemplazo con consentimiento y snapshot de bytes | `internal/admin/skill.go:533-560` |
| E40 | install.sh nunca sobrescribe en silencio una skill modificada | `scripts/install.sh:1106-1120` |
| E41 | Cobertura de tests del seed de la skill (incluye el caso modificado) | `scripts/tests/install_skill_test.sh:10,77-82` |
| E42 | PRD 1.21: campos del owner-bot de Telegram escritos por la opción 7 | `docs/PRD.md:1466` |
| E43 | PRD 1.22: introducción del set de tools de excepciones de horario y del backend del bot | `docs/PRD.md:1467` |
| E44 | Estado declarado del canal: server-side entregado, gateway per-sender pendiente | `docs/PRD.md:1356` |
| E45 | Smoke B2: cascada de la skill desde la opción 7 funcionando sobre v0.8.0 | `odd/tasks/backlog.md:50` |
| E46 | Identidad de los tres deletes: `service_id`; `professional_id` + `day_of_week`; `exception_id` | `internal/mcp/tools_maintenance.go:205-207`, `:246-249`; `internal/mcp/tools_hours_exception.go:45-48` |
| E47 | Inventario de tools (23 registros `mcpTool("…")`): existen `search_services_advanced`, `list_hours_exceptions`, `get_business_profile`; **no** existe lectura de profesionales ni de agendas | enumeración de `mcpTool("…")` en `internal/mcp/tools_alerts.go`, `tools_booking.go`, `tools_loyalty.go`, `tools_profile.go`, `tools_maintenance.go`, `tools_hours_exception.go`, `tools_search.go`, `tools_client_registration.go`; gate por rol en `cmd/mcp-server/main.go:409-450` (verificado 2026-10-19) |
| E48 | Los resultados de delete devuelven la identidad + `status` (base del reporte G4) | `internal/application/dto/maintenance.go:83-86`, `:135-139`, `:178-181` |

## Out-of-repo deliverable (registrado, no implementado acá)

El build del gateway per-sender corre en el código de la plataforma Hermes (repo separado, owner-owned): `odd/tasks/feat-owner-chat-b12.md:13`. Este change le entrega dos cosas verificables en `docs/gateway-contract.md`: las **reglas de interacción de la persona owner** (cuándo se activa, qué puede pedir, qué deriva) y una **checklist de aceptación** (design §4.2) que el build debe cumplir para que la persona owner sea correcta y no una heurística. Nada de esa checklist se valida en este repo: se valida en el repo del gateway.
