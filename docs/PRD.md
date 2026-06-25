# PRD: mcp-appointments-crm

> **Estado**: Aprobado
> **Owner**: Kike
> **Versión**: 1.0
> **Última actualización**: 2026-06-24

---

## 1. Contexto y Problema

### 1.1 Contexto

Los pequeños y medianos negocios de servicios por turnos (médicos, veterinarios, masajistas, fisioterapeutas, peluqueros, barberos) viven de su tiempo. La administración efectiva de reservas es crítica para su facturación, pero la gran mayoría no cuenta con los recursos humanos ni económicos para contratar un administrativo dedicado, ni para adquirir e implementar sistemas de gestión costosos o complejos. El mercado ofrece SaaS verticales con precios mensuales significativos o suites genéricas que requieren consultoría e instalación profesional. Ninguno de los dos extremos resuelve la fricción real del segmento.

La convergencia de tres tendencias hace viable un nuevo enfoque: (1) agentes de IA conversacionales (como Hermes, basados en el Model Context Protocol) que pueden actuar como interfaz natural por chat, (2) binarios en Go ultraligeros y auto-contenidos, y (3) SQLite como motor embebido maduro con FTS5 nativo. La combinación elimina la necesidad de UI web/móvil y la dependencia de infraestructura de servidor de base de datos.

### 1.2 Problema

Hoy estos negocios gestionan sus reservas con agendas de papel, planillas de Excel o memorias humanas. El dolor concreto es doble:

- **Pérdida directa de ingresos por olvidos y dobles reservas**: sin recordatorios automáticos ni un canal reactivo para reprogramar, los clientes faltan a turnos que se podrían haber ocupado.
- **Falta de seguimiento del cliente**: no hay ficha, no hay historial de preferencias, no hay forma de identificar clientes fieles o de ofrecer incentivos. El cliente es anónimo después de pagar.

Cuantificación aproximada: un negocio típico pierde entre un 10% y un 20% de ingresos por no-shows no gestionados. La ficha del cliente, cuando existe, vive en la cabeza del dueño y se pierde cuando el cliente cambia de profesional o de local.

### 1.3 Solución Propuesta

Un **Servidor MCP en Go con persistencia en SQLite** que se ejecuta en la propia VPS o PC del cliente. El sistema **no tiene UI propia**; expone un conjunto de herramientas (tools) al protocolo MCP. Un agente de IA conversacional (Hermes) consume esas herramientas y actúa como la interfaz para clientes finales y administradores. El sistema es single-tenant (una DB por negocio) pero multi-staff (varios profesionales por instalación). La configuración inicial se realiza mediante un asistente TUI en Go (Bubble Tea) que valida y exporta JSON. El despliegue en la VPS del cliente se automatiza con un script `curl | bash` que instala Docker y levanta el contenedor.

---

## 2. Objetivos y Métricas de Éxito

### 2.1 Objetivos (SMART)

- [ ] **O1**: Lanzar un binario MCP server en Go v1.0 que exponga al menos 12 tools funcionales (gestión de identidad, recursos, reservas, alertas, fidelización) antes del 2026-Q4.
- [ ] **O2**: Alcanzar un tiempo de instalación en una VPS Ubuntu limpia (sin Docker) inferior a 5 minutos, medido desde `curl | bash` hasta el log "Servidor MCP Activo".
- [ ] **O3**: Soportar al menos 50 reservas concurrentes sin colisiones ni locks visibles al usuario, con `busy_timeout=5000` y WAL activo.
- [ ] **O4**: Cero SQL injections verificable: 100% de las queries usan prepared statements; cobertura de tests sobre el repository layer superior al 80%.

### 2.2 KPIs

| Métrica | Baseline | Target | Plazo |
|---------|----------|--------|-------|
| Tools MCP expuestas | 0 | 12+ | 2026-Q4 |
| Latencia SSE p95 en `check_availability` | TBD | < 100 ms | 2026-Q4 |
| Tiempo de instalación en VPS limpia | TBD | < 5 min | 2026-Q4 |
| Cobertura de tests del repository layer | 0% | > 80% | 2026-Q4 |
| Tamaño del binario compilado (linux/amd64) | TBD | < 25 MB | 2026-Q4 |
| `% uptime` en VPS propia del cliente | N/A | > 99% (depende del cliente) | ongoing |

---

## 3. Alcance

### 3.1 In Scope

- Binario `mcp-server` en Go que expone tools MCP vía SSE en `127.0.0.1:3000`.
- Persistencia en SQLite (archivo local) con WAL, `busy_timeout=5000`, `foreign_keys=ON`, `synchronous=NORMAL`.
- Soporte FTS5 con triggers `AFTER INSERT/UPDATE/DELETE` para sincronización automática.
- Binario `config-wizard` (TUI en Bubble Tea) para configuración inicial con validación regex/string.
- Script `install.sh` que instala Docker, levanta el contenedor y configura el cron de backups.
- `Dockerfile` y `docker-compose.yml` con el binario ejecutándose bajo un usuario sin privilegios (`appuser`).
- Endpoint SSE expuesto **únicamente** en `127.0.0.1:3000` (loopback estricto).
- Manejo de errores con mensajes semánticos en español, sin stack traces al LLM.
- Tests unitarios sobre el repository layer con `go-sqlmock`.
- Linter `golangci-lint` con defaults (errcheck, govet, ineffassign, staticcheck, unused).
- Hook pre-commit `GGA` configurado para revisar `*.go`, `*.mod`, `*.sum`.

### 3.2 Out of Scope

- UI web o aplicación móvil (la interfaz es Hermes).
- Autenticación de usuarios (el sistema corre en loopback y confía en el cliente conectado).
- HTTPS, TLS, certificados (el transporte es SSE plano en loopback).
- Rate limiting HTTP (la contención de concurrencia se maneja a nivel de SQLite).
- Panel de administración web (el dueño opera vía Hermes).
- Integración directa con WhatsApp/Telegram (esos canales son responsabilidad de Hermes).
- Sincronización multi-device o cloud (single-tenant, single-install).
- Migración desde otros sistemas de reservas (no hay importador).
- App móvil nativa o PWA.

### 3.3 Asunciones

- El cliente final del producto (no del sistema, sino del dueño del negocio) tiene una VPS Linux propia o está dispuesto a contratar una (Hetzner, DigitalOcean, etc. desde $3.50/mes).
- El cliente instala y configura Hermes de forma autónoma en la misma máquina.
- Hermes soporta MCP sobre SSE y puede configurarse para apuntar a `http://127.0.0.1:3000/mcp`.
- La base de datos SQLite cabe en una sola VPS; no se anticipa necesidad de sharding ni replicación.
- El upstream LLM que mueve Hermes es capaz de traducir los mensajes semánticos en español al lenguaje del usuario final.
- El stack Go + SQLite (vía `modernc.org/sqlite`) sigue siendo soportado por las herramientas estándar del ecosistema.

### 3.4 Approach Técnico (alto nivel)

- **Repository pattern** sobre `*sql.DB`, con una capa de repos por tabla (`clients`, `services`, `professionals`, `bookings`, etc.) que centraliza las queries con prepared statements.
- **MCP server framework**: evaluar e integrar una librería MCP para Go (oficial de `modelcontextprotocol/go-sdk` o equivalente); si no hay una estable al momento, se implementa el protocolo a mano.
- **FTS5 sync via triggers** SQL declarados en el schema, no en código Go. La fuente de verdad es la tabla relacional; el FTS es un índice derivado.
- **Containerizado en Docker** con `modernc.org/sqlite` (pure Go, sin CGo) para mantener la imagen pequeña y portable. Binario corre como usuario no-root (`appuser`).
- **TUI con MVU estricto** (Bubble Tea), con validación regex/string por campo antes de permitir avanzar.
- **Trazabilidad de errores** con `fmt.Errorf("...: %w", err)` y mensajes semánticos en español para devolver al LLM.
- **Tradeoff principal**: usar `modernc.org/sqlite` (pure Go) a cambio de un binario ~5 MB más grande que el driver CGo. Se acepta porque simplifica el build (no requiere toolchain C) y la imagen Docker.

### 3.5 Affected Areas

- `cmd/mcp-server/` — entry point del servidor MCP.
- `cmd/config-wizard/` — entry point del TUI de configuración.
- `internal/db/` — conexión, pragmas, schema (ya existe `database.go`).
- `internal/repository/` — nuevo: repos por tabla con prepared statements.
- `internal/mcp/` — nuevo: handlers de tools MCP, registro del server.
- `internal/model/` — nuevo: structs de dominio (Client, Service, Booking, etc.).
- `internal/tui/` — nuevo: modelo Bubble Tea del config-wizard.
- `scripts/install.sh` — script de despliegue para VPS del cliente.
- `setup/Dockerfile`, `setup/docker-compose.yml` — contenedores de despliegue.
- `openspec/specs/{core,clients,services,bookings,business-profile}/` — delta specs por dominio.
- `openspec/changes/<fase>/` — carpetas por fase del SDD workflow.

> **Convenciones de nomenclatura del modelo de datos** (alinear antes de Fase 1 db-layer):
> - La tabla de reservas se llama **`bookings`** (no `appointments`).
> - El campo de duración se llama **`duration_minutes`** (no `duration_mins`).
> - Los campos `messenger_platform` y `messenger_id` van en **`business_profile`** (canal del bot del negocio), no en `clients`.
> - Los repos Go se nombran en plural para colecciones (`BookingsRepo`) y singular para agregados (`Booking`).

### 3.6 Rollback Plan

- **Estrategia**: una vez commiteado, cada fase del SDD es revertible con `git revert <sha>` sobre el branch de feature antes de merge a `main`. Para releases ya desplegados, el contenedor Docker se puede bajar con `docker compose down`, restaurando la imagen anterior con `docker compose pull <tag-anterior>`.
- **Tiempo estimado de rollback**: < 5 minutos por commit en entorno de desarrollo; < 15 minutos en VPS de cliente con el cron de backup activado.
- **Riesgo residual si rollback falla**: la base de datos SQLite queda en un estado inconsistente con el binario. Mitigación: el backup diario (`/opt/mcp-server/backups/reservas-YYYYMMDD.db.gz`) permite restaurar el `.db` a un punto anterior y volver a levantar el contenedor contra ese backup.

---

## 4. Usuarios y Personas

### 4.1 Personas

| Persona | Necesidad principal | Frecuencia de uso |
|---------|---------------------|-------------------|
| Dueño del negocio (admin) | Configurar el sistema, ver reportes, gestionar staff, agregar servicios, recibir avisos | Diaria |
| Profesional/Staff (peluquero, médico, etc.) | Ver su agenda del día, reprogramar excepciones, agregar notas a una reserva | Diaria |
| Cliente final (vía Hermes/WhatsApp) | Consultar disponibilidad, reservar, cancelar, reprogramar, recibir recordatorios | Esporádica |
| Soporte técnico (Kike o tercerizado) | Conectarse por SSH a la VPS del cliente para diagnosticar, actualizar, restaurar | Mensual o ante incidentes |

### 4.2 User Stories de Alto Nivel

- Como **dueño del negocio**, quiero **agregar un nuevo servicio con su precio y duración hablando con Hermes**, para **no tener que meterme en un panel web**.
- Como **dueño del negocio**, quiero **preguntarle a Hermes quiénes son mis clientes más fieles este mes**, para **ofrecerles un descuento y fidelizarlos**.
- Como **profesional**, quiero **ver mi agenda de mañana y agregar una nota a una reserva**, para **prepararme con contexto del cliente**.
- Como **cliente final**, quiero **pedirle a Hermes un turno disponible con María para el jueves a las 16**, para **no tener que llamar por teléfono**.
- Como **cliente final**, quiero **recibir un recordatorio 24 hs antes de mi turno**, para **no olvidarme y poder reprogramar si no puedo ir**.
- Como **soporte técnico**, quiero **poder conectarme por SSH a la VPS del cliente y bajar/levantar el contenedor**, para **aplicar updates o restaurar backups sin pedirle nada al cliente**.

---

## 5. Requerimientos

### 5.1 Requerimientos Funcionales (RF)

**RF1: Configuración inicial del negocio vía TUI**
- **Descripción**: El sistema debe proveer un binario `config-wizard` que captura los datos de `business_profile`, `professionals` iniciales y `services` iniciales a través de una interfaz de terminal con validación por campo. La salida son archivos JSON en `/opt/mcp-server/setup/`.
- **Prioridad**: Must
- **Criterios de Aceptación** (formato Gherkin):
  - [ ] Dado que el usuario ejecuta `config-wizard` por primera vez, cuando completa todos los pasos, entonces el sistema genera `setup_business.json`, `setup_staff.json` y `setup_services.json` válidos en `/opt/mcp-server/setup/`.
  - [ ] Dado que el usuario ingresa un email con formato inválido en `contact_email`, cuando intenta avanzar, entonces el TUI muestra un error de validación y no permite continuar.
  - [ ] Dado que el usuario ingresa un horario `start_time` que no respeta el formato `HH:MM`, cuando intenta guardar, entonces el TUI rechaza el input y pide reintentar.

**RF2: Exposición de identidad del negocio vía MCP**
- **Descripción**: El sistema debe exponer los tools `get_business_profile()` y `update_business_profile(fields...)` que leen y modifican la tabla `business_profile` a través del protocolo MCP.
- **Prioridad**: Must
- **Criterios de Aceptación**:
  - [ ] Dado que el primer boot del contenedor terminó exitosamente, cuando Hermes invoca `get_business_profile()`, entonces el sistema retorna un JSON con todos los campos del negocio actual.
  - [ ] Dado que Hermes invoca `update_business_profile({"public_phone": "+5491112345678"})`, cuando la operación es exitosa, entonces el sistema retorna `OK` y el nuevo teléfono queda persistido.
  - [ ] Dado que Hermes invoca `update_business_profile` con un campo que no existe en la tabla, cuando el sistema intenta aplicarlo, entonces retorna un mensaje semántico `Error: campo desconocido 'foo'. Campos válidos: ...`.

**RF3: Búsqueda de clientes y servicios con FTS5**
- **Descripción**: El sistema debe exponer `search_clients_advanced(query_text)` y `search_services_advanced(query_text)` que ejecutan consultas contra las tablas virtuales FTS5 (`clients_fts`, `services_fts`).
- **Prioridad**: Must
- **Criterios de Aceptación**:
  - [ ] Dado que existen 100 clientes y al menos 3 tienen "alergia" en `preferences`, cuando Hermes invoca `search_clients_advanced("alergia")`, entonces el sistema retorna los 3 clientes relevantes ordenados por rank FTS5.
  - [ ] Dado que `clients_fts` está sincronizado vía triggers, cuando un cliente se inserta o actualiza en `clients`, entonces la fila correspondiente en `clients_fts` se crea o actualiza automáticamente sin código Go adicional.
  - [ ] Dado que `query_text` contiene caracteres especiales de FTS5 (paréntesis, comillas), cuando Hermes invoca la búsqueda, entonces el sistema escapa los caracteres y retorna resultados válidos o un mensaje semántico claro, nunca un error de SQL.

**RF4: Gestión de profesionales, servicios y horarios**
- **Descripción**: El sistema debe exponer `add_professional()`, `add_service()` y `set_professional_schedule()` que crean registros en las tablas correspondientes.
- **Prioridad**: Must
- **Criterios de Aceptación**:
  - [ ] Dado que Hermes invoca `add_service({"name": "Corte de pelo", "duration_minutes": 30, "price": 5000})`, cuando la operación es exitosa, entonces el servicio queda persistido y su fila se inserta automáticamente en `services_fts`.
  - [ ] Dado que Hermes invoca `set_professional_schedule()` con un `day_of_week` fuera de `0..6`, cuando el sistema valida, entonces retorna `Error: day_of_week debe estar entre 0 (Domingo) y 6 (Sábado)`.

**RF5: Gestión de la ficha del cliente (CRM ligero)**
- **Descripción**: El sistema debe exponer `get_or_create_client()`, `update_client_preferences()` y `get_client_history()` para mantener y consultar la ficha del cliente.
- **Prioridad**: Must
- **Criterios de Aceptación**:
  - [ ] Dado que un cliente con `phone=+5491112345678` ya existe, cuando Hermes invoca `get_or_create_client({"phone": "+5491112345678", "name": "Juan"})`, entonces el sistema retorna el cliente existente sin crear duplicado.
  - [ ] Dado que un cliente tiene 5 reservas previas, cuando Hermes invoca `get_client_history(client_id)`, entonces el sistema retorna las 5 reservas ordenadas por `start_datetime` descendente.

**RF6: Ciclo de vida de reservas**
- **Descripción**: El sistema debe exponer `check_availability()`, `create_booking()`, `cancel_booking()` y `reschedule_booking()` con validación de reglas de negocio.
- **Prioridad**: Must
- **Criterios de Aceptación**:
  - [ ] Dado que el profesional X tiene horario Lunes 9-18 y hay una reserva de 10:00 a 11:00, cuando Hermes invoca `check_availability(professional_id=X, start=10:30)`, entonces el sistema retorna `false` y un mensaje `Error: el profesional X ya tiene una reserva en ese horario`.
  - [ ] Dado que el profesional X no trabaja los domingos, cuando Hermes invoca `create_booking` con `start_datetime` en domingo, entonces el sistema retorna `Error: el profesional X no trabaja los domingos`.
  - [ ] Dado que se cancela una reserva, cuando la operación es exitosa, entonces la fila se marca con `status='cancelled'` (no se borra) y el slot queda libre para `check_availability`.

> **Máquina de estados de `bookings.status`**: valores permitidos `pending`, `confirmed`, `cancelled`. Transiciones válidas: `pending → confirmed`, `confirmed → cancelled`, `pending → cancelled`. No se permiten transiciones inválidas (ej. `cancelled → confirmed`, `cancelled → pending`); si el LLM las pide, el sistema retorna `Error: transición de estado inválida de {from} a {to}`.

**RF7: Sistema de alertas pendientes (pull-based)**
- **Descripción**: El sistema debe mantener una tabla `pending_alerts` con notificaciones generadas por la lógica de negocio. Hermes las consume con `get_pending_alerts()` y las marca como enviadas con `mark_alert_as_sent()`.
- **Prioridad**: Should
- **Criterios de Aceptación**:
  - [ ] Dado que se creó una reserva para mañana a las 10:00, cuando el sistema evalúa las alertas pendientes, entonces existe una `pending_alert` con `type='reminder_24h'`, `scheduled_datetime=24h antes` y `status='pending'`.
  - [ ] Dado que Hermes consume la alerta y llama `mark_alert_as_sent(alert_id)`, cuando la operación es exitosa, entonces la fila tiene `status='sent'`.

**RF8: Reporte de fidelización (CRM intelligence)**
- **Descripción**: El sistema debe exponer `get_loyalty_report(period)` que devuelve los clientes más frecuentes en el período solicitado.
- **Prioridad**: Should
- **Criterios de Aceptación**:
  - [ ] Dado que hay 50 clientes con al menos una reserva en el último mes, cuando Hermes invoca `get_loyalty_report("last_month")`, entonces el sistema retorna el Top N de clientes ordenados por cantidad de reservas descendente, junto con su `client_id`, `name`, `phone` y `booking_count`.

**RF9: Despliegue automatizado con `install.sh`**
- **Descripción**: El sistema debe proveer un script `install.sh` ejecutable vía `curl | bash` que instala Docker, levanta el contenedor y configura el cron de backups.
- **Prioridad**: Must
- **Criterios de Aceptación**:
  - [ ] Dado que el script se ejecuta en una VPS Ubuntu limpia sin Docker, cuando termina exitosamente, entonces el contenedor `mcp-appointments-crm` está corriendo y el log final imprime `http://127.0.0.1:3000/mcp`.
  - [ ] Dado que el script se ejecuta sin los archivos JSON de `setup/`, cuando el sistema valida los prerrequisitos, entonces imprime `Error: ejecute primero config-wizard` y termina con exit code 1 sin instalar Docker.

> **Nota**: los criterios Gherkin de §5.1 se traducen a `scenarios` en el delta spec
> (`openspec/changes/<fase>/specs/<domain>/spec.md`) usando el formato
> `### Requirement: <nombre>` + `#### Scenario: <nombre>`. La prioridad
> `[Must | Should | Could | Won't]` se mapea a palabras clave RFC 2119
> (MUST / SHOULD / MAY) en el spec.

### 5.2 Requerimientos No Funcionales (RNF)

| Categoría | Requerimiento | Métrica / Target |
|-----------|---------------|------------------|
| Concurrencia SQLite | Múltiples readers + 1 writer concurrentes desde tools MCP | WAL + `busy_timeout=5000`; 0 colisiones en pruebas de carga con 50 goroutines |
| Latencia SSE | Latencia del endpoint `check_availability` | p95 < 100 ms en VPS con 2 vCPU / 2 GB RAM |
| Tamaño binario | Tamaño del binario compilado (linux/amd64) | < 25 MB con `modernc.org/sqlite` |
| Portabilidad | Debe correr en Linux/amd64, Linux/arm64 y macOS/amd64, macOS/arm64 | Compilación cross-platform verificada en CI |
| Disponibilidad | El contenedor debe reiniciarse automáticamente ante crash | Política `restart: unless-stopped` en `docker-compose.yml` |
| Mantenibilidad | Cobertura de tests sobre el repository layer | > 80% con `go test -cover` |
| Seguridad | 100% de queries con prepared statements | Linter custom o test de auditoría que falle si hay concatenación |
| Seguridad | Puerto 3000 expuesto solo en loopback | `docker-compose.yml` con `"127.0.0.1:3000:3000:3000"` |
| Seguridad | Contenedor corre como usuario no-root | `USER appuser` en Dockerfile |
| Seguridad | Permisos de directorio restrictivos al crear el path del SQLite | `os.MkdirAll(dir, 0750)` en `internal/db/database.go` |
| Observabilidad | Logs estructurados en stdout (JSON) | `slog` de Go stdlib; nivel configurable vía env var |
| Resiliencia | Backup diario del archivo SQLite | Cron en `install.sh` que comprime `/opt/mcp-server/data/reservas.db` |

---

## 6. Restricciones Técnicas

### 6.1 Stack Técnico

- **Backend**: Go 1.26.4 (binarios `mcp-server` y `config-wizard`)
- **Base de Datos**: SQLite vía `modernc.org/sqlite` v1.53+ (pure Go, sin CGo) con FTS5 nativo
- **MCP**: Protocolo MCP sobre SSE en `http://127.0.0.1:3000/mcp`
- **TUI**: Charm Bubble Tea ecosystem (`bubbletea`, `bubbles`, `lipgloss`)
- **Infraestructura**: Docker + Docker Compose en la VPS del cliente
- **Build**: `go build -o /dev/null ./...`, `go test -v -race ./...`, `golangci-lint run ./...`
- **Distribución**: Script `install.sh` descargable vía `curl | bash` desde GitHub

### 6.2 Integraciones Externas

| Sistema | Tipo | Propósito | Criticidad |
|---------|------|-----------|------------|
| Hermes (agente IA) | MCP over SSE | Interfaz conversacional con clientes y admin | Bloqueante |
| LLM (OpenAI, Anthropic, local, etc.) | API HTTP (vía Hermes) | Cerebro del agente; lo configura el cliente | Bloqueante (depende de Hermes) |
| WhatsApp / Telegram | API HTTP (vía Hermes) | Canal de mensajería con clientes finales | Importante |
| GitHub Releases | HTTPS | Descarga del binario y del `install.sh` | Bloqueante |

### 6.3 Compliance y Seguridad

- **Regulaciones aplicables**: ninguna explícita. El sistema maneja datos personales (PII) del cliente final (nombre, teléfono, email, preferencias) y datos de negocio, por lo que el dueño del negocio es responsable de cumplir las regulaciones locales (Ley 25.326 de Protección de Datos Personales en Argentina, GDPR si aplica, etc.). El sistema **no está certificado para manejar PCI-DSS ni datos financieros regulados** más allá de los precios de los servicios.
- **Datos sensibles manejados**: PII (nombre, teléfono, email, preferencias del cliente), historial de reservas, datos de negocio.
- **Controles de seguridad requeridos**: prepared statements (100%), puerto loopback estricto, usuario no-root en contenedor, validación regex/string en TUI, mensajes semánticos sin stack traces al LLM, HTTPS para descarga del `install.sh` desde GitHub.

---

## 7. Roadmap y Fases

> **Regla**: 1 fase = 1 SDD (openspec/changes/<nombre-de-la-fase>/).

### Fase 1: db-layer (Estimación: M)

**Objetivo**: sentar las bases de persistencia con repository pattern, prepared statements, FTS5 sync via triggers y tests con `go-sqlmock`.

**Entregables**:
- `internal/db/database.go` extendido con los triggers FTS5
- `internal/repository/{clients,services,professionals,bookings,business_profile,pending_alerts}.go`
- Tests con `go-sqlmock` cubriendo >80% del repository
- Índices secundarios en `bookings (start_datetime, professional_id, client_id)` y `pending_alerts (scheduled_datetime, status)`

**Definition of Done**:
- [ ] Todos los métodos del repository usan prepared statements (verificable con `grep`/`go vet` o test de auditoría)
- [ ] Insertar/actualizar/borrar en `clients` refleja automáticamente en `clients_fts`
- [ ] Insertar/actualizar/borrar en `services` refleja automáticamente en `services_fts`
- [ ] Cobertura de tests > 80%
- [ ] `go test -v -race ./...` pasa
- [ ] `golangci-lint run ./...` pasa
- [ ] Aprobado en code review + pasa CI

### Fase 2: mcp-server-core (Estimación: L)

**Objetivo**: levantar el servidor MCP, registrar el primer set de tools, exponerlos vía SSE en `127.0.0.1:3000`.

**Entregables**:
- `cmd/mcp-server/main.go` con loop de SSE
- `internal/mcp/server.go` con registro de tools
- Implementación de tools RF2, RF4, RF5, RF6 (mínimo viable: identidad, recursos, ficha de cliente, ciclo de reservas)
- `internal/model/` con structs de dominio
- `internal/errs/` con códigos y mensajes semánticos en español
- `Dockerfile` multi-stage con `USER appuser`
- `docker-compose.yml` con `"127.0.0.1:3000:3000:3000"`

**Definition of Done**:
- [ ] 6+ tools MCP registrados y funcionales
- [ ] Endpoint SSE responde en `http://127.0.0.1:3000/mcp`
- [ ] El contenedor corre como `appuser` (verificable con `docker exec`)
- [ ] El puerto 3000 NO es accesible desde la red del host (`curl 192.168.x.x:3000` falla)
- [ ] Todos los errores lógicos retornan mensajes en español, sin stack traces
- [ ] `go test -v -race ./...` pasa
- [ ] Documentación breve en `docs/` sobre cómo conectar Hermes

### Fase 3: mcp-server-advanced (Estimación: M)

**Objetivo**: incorporar las capacidades que diferencian al producto: búsqueda FTS5, alertas pull-based y reporte de fidelización.

**Entregables**:
- Tools `search_clients_advanced`, `search_services_advanced` (RF3)
- Tools `get_pending_alerts`, `mark_alert_as_sent` (RF7)
- Tool `get_loyalty_report` (RF8)
- Lógica de generación de alertas al crear/cancelar/reprogramar reservas
- Tests de integración con SQLite real (no mock) para FTS5 y alerts

**Definition of Done**:
- [ ] Las búsquedas FTS5 retornan resultados ordenados por rank
- [ ] Las alertas se generan automáticamente al crear una reserva
- [ ] El reporte de fidelización retorna el Top N correcto con datos agregados
- [ ] `go test -v -race ./...` pasa

### Fase 4: config-wizard (Estimación: M)

**Objetivo**: binario TUI en Bubble Tea que captura `business_profile`, `professionals` y `services` con validación por campo, y exporta JSON.

**Entregables**:
- `cmd/config-wizard/main.go`
- `internal/tui/` con el modelo MVU, validadores regex/string, vistas con `lipgloss`
- `setup_business.json`, `setup_staff.json`, `setup_services.json` como output
- Tests del TUI con `teatest` (Bubble Tea testing library)

**Definition of Done**:
- [ ] El TUI guía al usuario paso a paso
- [ ] Cada campo valida antes de permitir avanzar (regex para email, formato `HH:MM` para horarios, coordenadas geográficas)
- [ ] Al finalizar, los 3 JSON están en `/opt/mcp-server/setup/` con schema válido
- [ ] Tests con `teatest` pasan

### Fase 5: install-and-docker (Estimación: S)

**Objetivo**: script de despliegue automatizado para la VPS del cliente + cron de backups + documentación de instalación.

**Entregables**:
- `scripts/install.sh` ejecutable vía `curl | bash`
- Verificación de prerrequisitos (JSON existen, OS soportado)
- Instalación de Docker y Docker Compose si no están
- `docker compose up -d` del stack
- Configuración del cron de backups diarios
- Log final con el endpoint `http://127.0.0.1:3000/mcp`
- `docs/installation.md` con el manual de uso del script
- `docs/maintenance.md` con el manual de soporte anual

**Definition of Done**:
- [ ] En una VPS Ubuntu 22.04 limpia, el comando `curl -fsSL <url> | bash` deja el sistema corriendo en < 5 minutos
- [ ] El cron de backups crea un `.gz` diario en `/opt/mcp-server/backups/`
- [ ] El script falla con mensaje claro si los JSON no existen
- [ ] Manual de instalación en español, paso a paso

### Fase N: soporte y mejoras (ongoing)

**Objetivo**: mantener el sistema actualizado, agregar features reportadas por los primeros clientes, optimizar performance.

**Entregables**:
- Releases regulares con changelog
- Respuesta a issues de GitHub en < 48 hs hábiles
- Backups verificados periódicamente

**Definition of Done**:
- [ ] CI pasa en cada release
- [ ] Changelog actualizado
- [ ] Backups probados (restore en staging) cada 3 meses

---

## 8. Riesgos y Dependencias

### 8.1 Riesgos

| # | Riesgo | Probabilidad | Impacto | Mitigación |
|---|--------|--------------|---------|------------|
| R1 | El ecosistema MCP para Go no tiene una librería estable al momento de iniciar Fase 2 | Media | Alto | Plan B: implementar el protocolo MCP a mano sobre el stdlib (es JSON-RPC sobre SSE, no es complejo). Empezar a evaluar a partir de Fase 1. |
| R2 | `modernc.org/sqlite` introduce un overhead de performance vs. `mattn/go-sqlite3` (CGo) | Media | Bajo | Benchmark en Fase 1. Si el overhead es > 30%, reevaluar y considerar migrar a CGo con `CGO_ENABLED=1`. |
| R3 | El script `curl | bash` es vector de ataque si alguien compromete el repo o el dominio | Baja | Alto | Servir el script siempre por HTTPS desde GitHub. Documentar la verificación de integridad (checksum) en el manual de instalación. |
| R4 | Concurrencia real (50+ requests simultáneos) genera locks visibles al LLM | Media | Alto | WAL + `busy_timeout=5000` configurado desde Fase 1. Pruebas de carga antes de Fase 2. Mensajes semánticos claros cuando busy_timeout expira. |
| R5 | El dueño del negocio no sabe cómo configurar Hermes ni apuntarlo al MCP server | Alta | Alto | Documentación de instalación paso a paso + script que imprime la URL final. Soporte anual incluye setup remoto por SSH. |
| R6 | La base de datos SQLite crece sin control con el historial de reservas | Baja | Medio | Política de archivado anual: mover reservas > 2 años a tabla `bookings_archive`. Evaluar en Fase 3. |
| R7 | Cambios en la API o pricing de OpenAI/Anthropic dejan a Hermes sin LLM funcional | Media | Alto | El sistema MCP es agnóstico del LLM; el cliente puede cambiar de proveedor. Documentar alternativas (modelos locales, otros SaaS) en `docs/`. |

### 8.2 Dependencias

| # | Dependencia | Tipo | Estado | Owner |
|---|-------------|------|--------|-------|
| D1 | Hermes agent con soporte MCP sobre SSE | Bloqueante | Externa, se asume disponible | Cliente |
| D2 | VPS Linux del cliente (Ubuntu/Debian recomendados) | Bloqueante | Aprovisionar por el cliente | Cliente |
| D3 | Suscripción a un LLM (OpenAI, Anthropic, etc.) | Bloqueante | Aprovisionar por el cliente | Cliente |
| D4 | Cuenta de WhatsApp Business / Telegram Bot | Paralela | Configurar por el cliente vía Hermes | Cliente |
| D5 | Librería MCP para Go (oficial o comunitaria) | Bloqueante para Fase 2 | A evaluar al inicio de Fase 2 | Kike |
| D6 | Docker + Docker Compose | Bloqueante para Fase 5 | Instalado por `install.sh` | Kike / script |

---

## 9. Glosario y Referencias

### 9.1 Glosario

- **MCP (Model Context Protocol)**: protocolo abierto que permite a un agente de IA invocar herramientas (tools) de un servidor externo de forma estandarizada. Spec: <https://modelcontextprotocol.io>.
- **SSE (Server-Sent Events)**: estándar HTTP que permite al servidor enviar mensajes push al cliente sobre una conexión persistente. Usado en este proyecto para que Hermes consuma los tools MCP.
- **Hermes**: agente de IA conversacional open source que actúa como interfaz de usuario natural para el cliente final. No es parte de este proyecto; el cliente lo instala por separado.
- **Loopback / `127.0.0.1`**: dirección IP que apunta a la propia máquina. El puerto 3000 está expuesto solo en loopback para que solo procesos locales (como Hermes en la misma VPS) puedan acceder.
- **WAL (Write-Ahead Logging)**: modo de SQLite donde las escrituras se appendean a un log antes de aplicarse al archivo principal. Mejora la concurrencia entre readers y writers.
- **`busy_timeout`**: cantidad de milisegundos que SQLite espera a que se libere un lock antes de retornar `SQLITE_BUSY`. Configurado en 5000 ms.
- **FTS5**: extensión de SQLite para búsquedas de texto completo con ranking por relevancia. Tablas virtuales que se sincronizan con triggers.
- **TUI (Terminal User Interface)**: interfaz de usuario en la terminal, sin GUI. En este proyecto se usa Bubble Tea con el patrón MVU (Model-View-Update).
- **MVU (Model-View-Update)**: patrón de arquitectura para TUI donde el estado (Model) se actualiza por mensajes (Update) y se renderiza por una función pura (View).
- **Self-hosted / Self-Hosted**: modelo de despliegue donde el software corre en infraestructura del cliente, no en la nube del proveedor.
- **Single-tenant**: una instalación del sistema sirve a un único negocio. La base de datos es privada de ese negocio.
- **Multi-staff**: dentro de un mismo negocio (single-tenant), el sistema soporta varios profesionales con agendas independientes.
- **Pull-based alerts**: arquitectura donde el sistema genera alertas persistidas y el agente las consume cuando está listo, en lugar de hacer push proactivo.

### 9.2 Referencias

- `docs/SDD.md` — documento original con la idea del proyecto, análisis previos y la especificación técnica de base que dio origen a este PRD.
- `docs/common/prd-template.md` — template usado para estructurar este PRD.
- `openspec/config.yaml` — configuración del SDD workflow (reglas por fase, comandos, TDD).
- `AGENTS.md` — convenciones del proyecto para los agentes AI, coding standards, pre-commit checklist.
- `internal/db/database.go` — implementación actual del schema SQLite (Fase 0 previa al PRD).
- Spec del protocolo MCP: <https://modelcontextprotocol.io>.
- Bubble Tea: <https://github.com/charmbracelet/bubbletea>.
- `modernc.org/sqlite`: <https://pkg.go.dev/modernc.org/sqlite>.

---

## 10. Historial de Cambios

| Fecha | Versión | Autor | Cambios |
|-------|---------|-------|---------|
| 2026-06-24 | 1.0 | Kike | Creación inicial del PRD a partir de `docs/SDD.md` y `docs/common/prd-template.md`. Incluye 9 RF (Must + Should), 11 RNF, 5 fases de roadmap y 7 riesgos identificados. |
