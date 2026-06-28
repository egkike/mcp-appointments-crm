# PRD: mcp-appointments-crm

> **Estado**: Aprobado
> **Owner**: Kike
> **Versión**: 1.0
> **Última actualización**: 2026-06-26

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

Un **Servidor MCP en Go con persistencia en SQLite** que se ejecuta en la propia VPS o PC del cliente. El sistema **no tiene UI propia**; expone un conjunto de herramientas (tools) al protocolo MCP. Un agente de IA conversacional (Hermes) consume esas herramientas y actúa como la interfaz para clientes finales y administradores. El sistema es single-tenant (una DB por negocio) pero multi-staff (varios profesionales por instalación). La configuración inicial se realiza mediante prompts interactivos en el script `install.sh` (bash) con validación regex/string; un checkpoint (`setup.json.tmp`) permite resumir si el usuario cancela a mitad. El despliegue en la VPS del cliente se automatiza con un script `curl | bash` que descarga el binario, lo registra como servicio del SO e imprime al final una línea sugerida para schedular `backup.sh`.

---

## 2. Objetivos y Métricas de Éxito

### 2.1 Objetivos (SMART)

- [ ] **O1**: Lanzar un binario MCP server en Go v1.0 que exponga al menos 12 tools funcionales (gestión de identidad, recursos, reservas, alertas, fidelización) antes del 2026-Q4.
- [ ] **O2**: Alcanzar un tiempo de instalación en una VPS Ubuntu limpia (sólo con `curl` y `bash`) inferior a 5 minutos, medido desde `curl | bash` hasta el log "Servidor MCP Activo".
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
- Script `install.sh` que descarga el binario, lo registra como servicio del SO e imprime al final una línea sugerida para schedular `backup.sh`.
- Script `scripts/backup.sh` portable (bash, sin scheduler automático) que produce un backup consistente del `.db` con `sqlite3 .backup` + gzip.
- Templates de service unit para Linux (`mcp-appointments-crm.service`), macOS (`com.mcp.appointments.server.plist`) y Windows (`mcp-appointments-crm.xml` para Task Scheduler).
- Endpoint SSE expuesto **únicamente** en loopback. Bind default `127.0.0.1` (IPv4); `MCP_BIND` may also be set to `::1` (IPv6) u otra dirección loopback (127.0.0.0/8). Validación acepta loopback (127.0.0.0/8 o ::1). Ver [ADR-0007](../architecture/0007-server-config.md). Puerto default `3000`. Configurable vía env vars `MCP_BIND` y `MCP_PORT`. Precedencia (mayor a menor): env vars del sistema > `~/.config/mcp-appointments-crm/.env` (o equivalente platform-native) > defaults. El binario **no hace fallback automático** de puerto. Si `MCP_BIND` no es loopback (127.0.0.0/8 o ::1), falla con error de seguridad antes de bindear. Ver [ADR-0007](../architecture/0007-server-config.md).
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
- **Binario nativo en Go 1.26.4** con `modernc.org/sqlite` (pure Go, sin CGo, sin capas de contenedor). Se distribuye como binario único cross-compiled para 5 plataformas. Binario corre como **user-level service** (sin root, sin `appuser` dedicado) bajo el usuario que invoca `install.sh`.
- **Prompts interactivos en `install.sh`** (bash) con validación regex/string por campo antes de permitir avanzar. Un checkpoint (`setup.json.tmp`) cubre la cancelación del usuario.
- **Trazabilidad de errores** con `fmt.Errorf("...: %w", err)` y mensajes semánticos en español para devolver al LLM.
- **Tradeoff principal**: usar `modernc.org/sqlite` (pure Go) a cambio de un binario ~5 MB más grande que el driver CGo. Se acepta porque simplifica el build cross-platform (no requiere toolchain C en target ni runtime de contenedor).

### 3.5 Affected Areas

- `cmd/mcp-server/` — entry point del servidor MCP.
- `internal/db/` — conexión, pragmas, schema (ya existe `database.go`). Incluye la tabla `schema_version` para tracking de versión del esquema (ver spec `openspec/changes/feat-db-layer/specs/schema-version/spec.md`).
- `internal/repository/` — nuevo: repos por tabla con prepared statements.
- `internal/mcp/` — nuevo: handlers de tools MCP, registro del server.
- `internal/model/` — nuevo: structs de dominio (Client, Service, Booking, etc.).
- `scripts/install.sh` — script de despliegue para VPS del cliente.
- `scripts/backup.sh` — nuevo: script bash portable de backup (usa `sqlite3 .backup` para consistencia).
- `setup/service/` — templates de user-level service unit (systemd `~/.config/systemd/user/`, launchd `~/Library/LaunchAgents/`, Task Scheduler user task) con bind a `127.0.0.1` (default, configurable vía `MCP_BIND`).
- `~/.config/mcp-appointments-crm/.env` (Linux) o equivalente platform-native (§3.5 Install Layout) — archivo de configuración opcional con `MCP_BIND` y `MCP_PORT`; generado por `install.sh` con los valores default; editable por el operador; el service unit (systemd) lo carga con `EnvironmentFile=`. Si no existe, el binario arranca con los defaults sin error.
- `openspec/changes/<fase>/specs/<capability>/` — delta specs por dominio (un spec por capability dentro de la carpeta del change). Convención: `openspec/changes/feat-db-layer/specs/business-profile/spec.md`, `bookings/spec.md`, etc.
- `openspec/changes/<fase>/` — carpetas por fase del SDD workflow.

#### Matriz de cross-compilation

| Plataforma | Binario | Service manager |
|---|---|---|
| `linux/amd64` | `mcp-server-linux-amd64` | systemd |
| `linux/arm64` | `mcp-server-linux-arm64` | systemd |
| `darwin/amd64` | `mcp-server-darwin-amd64` | launchd |
| `darwin/arm64` | `mcp-server-darwin-arm64` | launchd |
| `windows/amd64` | `mcp-server-windows-amd64.exe` | NSSM o Task Scheduler |

Distribución: GitHub Releases + `install.sh` que detecta OS/arquitectura (`uname -s` + `uname -m`) y descarga el binario correspondiente.

#### Install Layout (paths por OS)

Install **user-level** (sin root, sin `appuser` dedicado). El servicio corre bajo el usuario que invoca `install.sh`. La convención de paths sigue el XDG Base Directory spec en Linux y las convenciones nativas en macOS/Windows.

| Componente | Linux (XDG) | macOS | Windows |
|---|---|---|---|
| **Binario** | `~/.local/bin/mcp-server` | `~/.local/bin/mcp-server` | `%LOCALAPPDATA%\Programs\mcp-server\mcp-server.exe` |
| **Data** (SQLite + backups) | `~/.local/share/mcp-appointments-crm/` | `~/Library/Application Support/MCP Appointments CRM/` | `%APPDATA%\MCP Appointments CRM\` |
| **Config** (JSON del setup) | `~/.config/mcp-appointments-crm/setup/` | `~/Library/Application Support/MCP Appointments CRM/setup/` | `%APPDATA%\MCP Appointments CRM\setup\` |
| **Logs** | `~/.local/state/mcp-appointments-crm/mcp-server.log` | `~/Library/Logs/MCP Appointments CRM/mcp-server.log` | `%LOCALAPPDATA%\MCP Appointments CRM\Logs\mcp-server.log` |
| **Service definition** | `~/.config/systemd/user/mcp-appointments-crm.service` | `~/Library/LaunchAgents/com.mcp.appointments.server.plist` | Task Scheduler (carpeta del usuario) |

> **Convenciones XDG**: si `XDG_DATA_HOME`, `XDG_CONFIG_HOME` o `XDG_STATE_HOME` están definidas, se respetan como base de los paths de data/config/logs.
>
> **24/7 en Linux**: para que el servicio user-level de systemd siga corriendo después de logout, `install.sh` ejecuta automáticamente `loginctl enable-linger <user>` (operación one-time, no afecta el login del usuario). En macOS y Windows, los user-level services/agents/tasks persisten tras logout por defecto.
>
> Los ejemplos de paths en este PRD usan los valores de Linux (XDG) como referencia canónica; para macOS y Windows, consultar la tabla anterior.

> **Convenciones de nomenclatura del modelo de datos** (alinear antes de Fase 1 db-layer):
> - La tabla de reservas se llama **`bookings`** (no `appointments`).
> - El campo de duración se llama **`duration_minutes`** (no `duration_mins`).
> - Los campos `messenger_platform` y `messenger_id` van en **`business_profile`** (canal del bot del negocio), no en `clients`.
> - Los repos Go se nombran en plural para colecciones (`BookingsRepo`) y singular para agregados (`Booking`).

### 3.6 Rollback Plan

- **Estrategia**: una vez commiteado, cada fase del SDD es revertible con `git revert <sha>` sobre el branch de feature antes de merge a `main`. Para releases ya desplegados, el servicio se puede detener con `systemctl stop mcp-appointments-crm`, restaurar el binario anterior desde un release previo, y reiniciar con `systemctl start mcp-appointments-crm`.
- **Tiempo estimado de rollback**: < 5 minutos por commit en entorno de desarrollo; < 15 minutos en VPS de cliente con el script `backup.sh` ejecutándose según la estrategia de scheduling elegida por el operador.
- **Riesgo residual si rollback falla**: la base de datos SQLite queda en un estado inconsistente con el binario. Mitigación: el backup ejecutado vía `scripts/backup.sh` (que produce `~/.local/share/mcp-appointments-crm/backups/reservas-YYYYMMDD.db.gz` en Linux, con paths análogos en macOS/Windows según §3.5) permite restaurar el `.db` a un punto anterior y volver a iniciar el servicio contra ese backup. La estrategia de scheduling queda a criterio del operador.

---

### 3.7 Modelo de Datos Relacional

> **Referencia canónica del schema.** Las migraciones y la Fase 1 (db-layer) deben
> implementar exactamente estas tablas. Cambios al schema requieren update de
> esta sección y, si son significativos, un ADR nuevo.
>
> Este modelo se alinea con `docs/SDD.md §B` con las correcciones/adiciones
> detectadas en la revisión de 2026-06-25:
>
> - `business_hours` se almacena como columna JSON dentro de `business_profile` (decisión de diseño del 2026-06-25: no justifica una tabla separada para una sola fila de config; trade-off documentado en §3.7.2).
> - `business_hours_exception` se agrega como tabla nueva para feriados, eventos y vacaciones — fechas específicas con horario diferente al semanal regular. Ver §3.7.3.
> - `business_profile` se documenta con sus campos extendidos (SDD.md lo
>   lista en otra sección).
> - `schedules` se documenta formalmente (estaba ausente del PRD).
> - `bookings` documenta explícitamente `end_datetime` y `payment_method`
>   (estaban en SDD.md pero faltantes en el PRD).
> - FTS5 sync via triggers `AFTER INSERT/UPDATE/DELETE` se documenta como
>   requisito funcional (gap conocido desde la fundación).

#### 3.7.1 `business_profile` (singleton — una sola fila por instalación)

Configuración global del negocio. Relación 1:1 con la instalación.

```sql
CREATE TABLE business_profile (
    id                          TEXT PRIMARY KEY DEFAULT 'singleton',
    name                        TEXT NOT NULL,
    industry                    TEXT,                          -- "veterinaria", "barbería", "peluquería"
    country                     TEXT,                          -- ISO 3166-1 alpha-2, ej "AR"
    address                     TEXT,
    latitude                    REAL,
    longitude                   REAL,
    cover_photo_url             TEXT,
    public_phone                TEXT,
    messenger_platform          TEXT,                          -- "whatsapp" | "telegram" | NULL
    messenger_id                TEXT,                          -- número o handle del bot del negocio
    contact_email               TEXT,
    website_url                 TEXT,
    general_description         TEXT,
    currency_code               TEXT NOT NULL DEFAULT 'ARS',   -- ISO 4217
    currency_symbol             TEXT NOT NULL DEFAULT '$',
    accepted_payment_methods    TEXT,                          -- JSON array, ej ["efectivo","tarjeta","transferencia"]
    timezone                    TEXT NOT NULL DEFAULT 'UTC',   -- IANA, ej "America/Argentina/Buenos_Aires"
    slot_interval_minutes       INTEGER NOT NULL DEFAULT 30,   -- granularidad para "find next available"
    business_hours              TEXT NOT NULL DEFAULT '{}',    -- JSON, ver §3.7.2
    created_at                  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at                  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    CHECK (id = 'singleton')
);
```

#### 3.7.2 Formato del JSON `business_hours` (columna de `business_profile`)

El horario de atención del negocio se almacena como JSON en la columna `business_hours` de `business_profile` (decisión del 2026-06-25: no justifica una tabla separada para una sola fila de config del negocio). La estructura es un objeto con una entrada por día de la semana; un día con valor `null` significa "cerrado".

```json
{
  "monday":    { "open": "09:00", "close": "18:00" },
  "tuesday":   { "open": "09:00", "close": "18:00" },
  "wednesday": { "open": "09:00", "close": "18:00" },
  "thursday":  { "open": "09:00", "close": "18:00" },
  "friday":    { "open": "09:00", "close": "18:00" },
  "saturday":  { "open": "09:00", "close": "13:00" },
  "sunday":    null
}
```

Los horarios se expresan en formato `HH:MM` (24 horas) en la `timezone` del negocio (columna `business_profile.timezone`).

**Query de ejemplo** (¿está abierto el sábado a las 10:00?):

```sql
SELECT
  json_extract(bp.business_hours, '$.saturday.open')  AS sat_open,
  json_extract(bp.business_hours, '$.saturday.close') AS sat_close
FROM business_profile bp
WHERE bp.id = 'singleton';
```

**Trade-off documentado**: este enfoque sacrifica queries SQL directas sobre horarios a cambio de mantener el "perfil" del negocio consolidado en una sola fila (single source of truth). Es aceptable porque:
- Solo hay UNA fila de `business_profile` por instalación
- El acceso a horarios se hace con `json_extract` (que SQLite soporta nativamente)
- Cambiar de un día a otro es raro (lo hace el dueño del negocio, no en hot path)

`business_hours` siempre representa el **horario regular semanal** del negocio. Para fechas específicas con horario diferente (feriados, eventos, vacaciones), ver `business_hours_exception` en §3.7.3. Si en el futuro se necesitan patrones más complejos (ej. "todos los domingos de agosto"), se evaluará una nueva abstracción sin romper estos dos.

#### 3.7.3 `business_hours_exception`

Excepciones por fecha al horario semanal regular. Permite representar feriados, eventos especiales, vacaciones del dueño, o días con horario reducido.

```sql
CREATE TABLE business_hours_exception (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    exception_date  TEXT NOT NULL UNIQUE,        -- "2026-12-25" (ISO date, sin timezone)
    is_closed       BOOLEAN NOT NULL DEFAULT 1, -- 1=cerrado, 0=abierto con horario custom
    open_time       TEXT,                        -- "HH:MM" (sólo si is_closed=0)
    close_time      TEXT,                        -- "HH:MM" (sólo si is_closed=0)
    reason          TEXT,                        -- "Navidad", "Vacaciones del dueño", "Feriado puente"
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_business_hours_exception_date ON business_hours_exception(exception_date);
```

**Semántica de la columna `is_closed`**:
- `is_closed = 1` → el negocio está cerrado ese día. `open_time`/`close_time` son NULL.
- `is_closed = 0` → el negocio está abierto con horario custom. `open_time`/`close_time` son requeridos.

**Regla de precedencia en `check_availability`**: si existe una fila en `business_hours_exception` para la fecha solicitada, se usa esa (con `is_closed` y opcionalmente `open_time`/`close_time`). Si NO existe, se usa el JSON `business_hours` con el día de la semana. Esto se documenta en §3.7.13 Paso 3a (ver abajo).

**Sobre los feriados nacionales**: por simplicidad, esta tabla NO incluye una librería de feriados nacionales por país. El dueño del negocio carga manualmente los feriados que le importan. Si en el futuro se vuelve tedioso, se evaluará agregar una tabla `national_holidays` curada por país o una librería Go de holidays.

#### 3.7.4 `professionals`

Staff que presta servicios. Multi-staff por instalación. La entidad existía en SDD.md §B pero el PRD no documentaba sus campos; los formalizamos acá.

```sql
CREATE TABLE professionals (
    id              TEXT PRIMARY KEY,                       -- UUID v4
    name            TEXT NOT NULL,
    role_specialty  TEXT,                                   -- "Veterinario", "Barbero", "Estilista" (alineado con SDD.md §B)
    status          TEXT NOT NULL DEFAULT 'active',         -- 'active' | 'inactive'
    email           TEXT,
    phone           TEXT,
    specialties     TEXT,                                   -- JSON array de service_ids, ej ["svc-001","svc-003"]
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
```

`role_specialty` (string) viene de SDD.md §B para describir el rol principal; `specialties` (JSON array) es una adición que permite asociar múltiples servicios a un profesional (un veterinario puede atender "consulta" y "cirugía"). El campo `status` controla visibilidad: profesionales inactivos no aparecen en `check_availability` ni en la lista de staff.

#### 3.7.5 `schedules`

Horario semanal de cada profesional. Permite responder "¿el Profesional A trabaja hoy?". Era completamente ausente del PRD antes de esta sección; SDD.md §B la definía.

```sql
CREATE TABLE schedules (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    professional_id     TEXT NOT NULL REFERENCES professionals(id) ON DELETE CASCADE,
    day_of_week         INTEGER NOT NULL,                   -- 0=domingo, ..., 6=sábado
    start_time          TEXT NOT NULL,                      -- "HH:MM" en la timezone del negocio
    end_time            TEXT NOT NULL,                      -- "HH:MM"
    UNIQUE(professional_id, day_of_week)
);

CREATE INDEX idx_schedules_professional_day ON schedules(professional_id, day_of_week);
```

Una fila por (profesional, día). Si un profesional no tiene fila para un día, ese día no trabaja.

#### 3.7.6 `services`

Catálogo de servicios que ofrece el negocio. Cada servicio tiene duración y precio.

```sql
CREATE TABLE services (
    id              TEXT PRIMARY KEY,                       -- UUID v4
    name            TEXT NOT NULL,
    description     TEXT,                                   -- campo que faltaba documentar en el PRD; presente en SDD.md §B
    duration_minutes INTEGER NOT NULL,                      -- canónico, ver ADR-0004
    price           REAL NOT NULL,                          -- en la currency_code de business_profile
    is_active       BOOLEAN NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
```

#### 3.7.7 `clients`

Clientes del negocio. `phone` es único porque se usa como ID del chat en WhatsApp/Telegram (alineado con SDD.md §B).

```sql
CREATE TABLE clients (
    id              TEXT PRIMARY KEY,                       -- UUID v4
    name            TEXT NOT NULL,
    phone           TEXT NOT NULL UNIQUE,                  -- ID del chat (WhatsApp/Telegram)
    email           TEXT,
    preferences     TEXT,                                   -- texto libre, ej "alergia a penicilina"
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);
```

#### 3.7.8 `bookings`

Reservas de servicios. Tabla central del sistema. Renombrada de `appointments` (gap #10, ver ADR-0004).

```sql
CREATE TABLE bookings (
    id                  TEXT PRIMARY KEY,                   -- UUID v4
    client_id           TEXT NOT NULL REFERENCES clients(id) ON DELETE CASCADE,
    professional_id     TEXT NOT NULL REFERENCES professionals(id) ON DELETE RESTRICT,
    service_id          TEXT NOT NULL REFERENCES services(id) ON DELETE RESTRICT,
    start_datetime      TEXT NOT NULL,                      -- ISO 8601 UTC, ej "2026-06-25T17:00:00.000Z"
    end_datetime        TEXT NOT NULL,                      -- ISO 8601 UTC, start + service.duration_minutes
    status              TEXT NOT NULL DEFAULT 'pending',    -- 'pending' | 'confirmed' | 'cancelled'
    notes               TEXT,
    payment_method      TEXT,                               -- método elegido para la cita (alineado con SDD.md §B)
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_bookings_overlap
    ON bookings(professional_id, start_datetime, end_datetime);
```

**Máquina de estados de `status`**: ver §5.1 RF6. Valores permitidos `pending`, `confirmed`, `cancelled`. Transiciones documentadas ahí.

**`end_datetime`**: se almacena para optimizar queries de overlap check en `check_availability()`. Se calcula al insert/update como `start_datetime + duration_minutes` del servicio. Si el servicio cambia de duración en el futuro, las reservas existentes mantienen su `end_datetime` (consistencia histórica). Si la reserva se mueve, ambos campos se recalculan juntos.

**Overlap check** (la consulta clave de `check_availability`):

```sql
-- Devuelve 1 si hay conflicto entre (professional_id, start, end) y otra reserva no cancelada
SELECT 1
FROM bookings
WHERE professional_id = ?
  AND status != 'cancelled'
  AND start_datetime < ?     -- proposed end
  AND end_datetime   > ?     -- proposed start
LIMIT 1;
```

#### 3.7.9 `pending_alerts`

Cola de notificaciones pull-based. Hermes las consume con `get_pending_alerts()` y las marca como enviadas con `mark_alert_as_sent()` (RF7).

```sql
CREATE TABLE pending_alerts (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    type                TEXT NOT NULL,                      -- "confirmation_requested" | "reminder_24h" | "loyalty_alert"
    message             TEXT NOT NULL,                      -- texto en español, listo para enviar
    scheduled_datetime  TEXT NOT NULL,                      -- ISO 8601 UTC, cuándo debe enviarse
    status              TEXT NOT NULL DEFAULT 'pending',    -- 'pending' | 'sent' | 'cancelled'
    related_booking_id  TEXT REFERENCES bookings(id),       -- opcional, link a la reserva que origina la alerta
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
);

CREATE INDEX idx_pending_alerts_scheduled_status
    ON pending_alerts(scheduled_datetime, status);
```

#### 3.7.10 Tablas FTS5 (búsqueda full-text)

```sql
-- Índice full-text sobre clients.name y clients.preferences
CREATE VIRTUAL TABLE clients_fts USING fts5(
    name,
    preferences,
    content='clients',
    content_rowid='rowid'
);

-- Índice full-text sobre services.name y services.description
CREATE VIRTUAL TABLE services_fts USING fts5(
    name,
    description,
    content='services',
    content_rowid='rowid'
);
```

> **⚠️ CRÍTICO — gap conocido desde la fundación**: las tablas FTS5 con
> `content='source'` requieren triggers `AFTER INSERT / UPDATE / DELETE`
> en la tabla fuente para mantener la sincronización. **Sin los triggers,
> las búsquedas devuelven cero resultados aunque haya datos en la tabla
> fuente.** Implementación obligatoria en Fase 1 (db-layer).

```sql
-- Triggers de sync para clients_fts (naming con infix `_fts_` para
-- consistencia con el nombre de la tabla)
CREATE TRIGGER clients_fts_ai AFTER INSERT ON clients BEGIN
    INSERT INTO clients_fts(rowid, name, preferences)
    VALUES (new.rowid, new.name, new.preferences);
END;

CREATE TRIGGER clients_fts_ad AFTER DELETE ON clients BEGIN
    INSERT INTO clients_fts(clients_fts, rowid, name, preferences)
    VALUES ('delete', old.rowid, old.name, old.preferences);
END;

CREATE TRIGGER clients_fts_au AFTER UPDATE ON clients BEGIN
    INSERT INTO clients_fts(clients_fts, rowid, name, preferences)
    VALUES ('delete', old.rowid, old.name, old.preferences);
    INSERT INTO clients_fts(rowid, name, preferences)
    VALUES (new.rowid, new.name, new.preferences);
END;

-- Triggers análogos para services_fts
CREATE TRIGGER services_fts_ai AFTER INSERT ON services BEGIN
    INSERT INTO services_fts(rowid, name, description)
    VALUES (new.rowid, new.name, new.description);
END;

CREATE TRIGGER services_fts_ad AFTER DELETE ON services BEGIN
    INSERT INTO services_fts(services_fts, rowid, name, description)
    VALUES ('delete', old.rowid, old.name, old.description);
END;

CREATE TRIGGER services_fts_au AFTER UPDATE ON services BEGIN
    INSERT INTO services_fts(services_fts, rowid, name, description)
    VALUES ('delete', old.rowid, old.name, old.description);
    INSERT INTO services_fts(rowid, name, description)
    VALUES (new.rowid, new.name, new.description);
END;
```

#### 3.7.11 Resumen de índices secundarios

| Tabla | Índice | Razón |
|---|---|---|
| `schedules` | `(professional_id, day_of_week)` | "¿trabaja el Profesional A hoy?" |
| `bookings` | `(professional_id, start_datetime, end_datetime)` | Overlap check (3d) + agenda del profesional |
| `business_hours_exception` | `(exception_date)` UNIQUE | "¿hay excepción para esta fecha?" (Paso 3a de §3.7.13) |
| `pending_alerts` | `(scheduled_datetime, status)` | "¿qué alertas hay pendientes para enviar?" |
| `clients_fts` | (auto, FTS5) | `search_clients_advanced` |
| `services_fts` | (auto, FTS5) | `search_services_advanced` |

> **Nota sobre índices FK de `bookings`**: los FKs de `bookings` (`client_id`, `professional_id`, `service_id`) no se indexan por decisión de diseño (Decisión 9 del design de `feat-db-layer`). SQLite no requiere índices sobre FKs para integridad referencial, y los FKs de `bookings` no son hot-path de search; el overlap check usa `professional_id` + `start_datetime`/`end_datetime` (cubierto por el índice compuesto listado arriba).

#### 3.7.12 Convenciones de nombrado aplicadas

Per [ADR-0004](../architecture/0004-naming-conventions.md):

- Tabla de reservas: `bookings` (no `appointments`)
- Campo de duración: `duration_minutes` (no `duration_mins`)
- Fechas de inicio/fin: `start_datetime` / `end_datetime` (no `start_time` / `end_time`)
- Messenger fields: `messenger_platform`, `messenger_id` en `business_profile` (no en `clients`)
- Repos Go: plural para colecciones (`BookingsRepo`), singular para agregados (`Booking`)

> **Convención `updated_at`**: Los repos son responsables de incluir
> `updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')` en cada `UPDATE ... SET ...`.
> **No hay triggers** que lo hagan automáticamente; el repositorio debe setearlo
> explícitamente en cada UPDATE.
>
> **Convención de timestamps**:
> - `business_profile`, `professionals`, `services`, `clients`, `bookings`: `created_at` + `updated_at` (ambas en ISO 8601 UTC, default `strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`)
> - `schedules`, `business_hours_exception`: sin timestamps (configuración set-and-forget; la lógica de negocio no los necesita)
> - `pending_alerts`: `created_at` solamente; los cambios de status (`MarkAsSent`, `Cancel`) se trackean por el `status` en sí, no por un `updated_at`

---

#### 3.7.13 Flujo de Reserva End-to-End

Cuando el cliente solicita una reserva (típicamente vía Hermes, ej. "Hermes, quiero un turno con María para hoy a las 16"), el sistema ejecuta una cadena de validaciones. Cada paso tiene un mensaje semántico en español que se devuelve al LLM si falla (per coding standards: "no raw system dumps", "stack traces NEVER sent to LLM").

##### Paso 1 — Resolución de parámetros

```sql
-- service_id, name, duration_minutes
SELECT id, name, duration_minutes
FROM services
WHERE id = ? AND is_active = 1;

-- professional_id, name, status
SELECT id, name, status
FROM professionals
WHERE id = ?;
```

Si el servicio no existe o está inactivo:
`Error: el servicio '{name}' no existe o no está disponible.`

Si el profesional no existe o está inactivo:
`Error: el profesional '{name}' no está disponible.`

##### Paso 2 — Calcular `end_datetime`

```go
end_datetime = start_datetime + service.duration_minutes
```

##### Paso 3 — `check_availability()` (cadena de validaciones)

El sistema ejecuta las siguientes validaciones **en orden**. La primera que falle aborta y retorna el mensaje correspondiente.

**3a. ¿El negocio está abierto ese día?**

Primero se consulta `business_hours_exception` (excepciones por fecha). Si hay una fila para la fecha solicitada, se usa esa. Si no, se cae al JSON `business_hours` con el día de la semana.

```sql
-- 1. ¿Hay excepción para esta fecha? (feriado, evento, vacaciones)
SELECT is_closed, open_time, close_time, reason
FROM business_hours_exception
WHERE exception_date = ?;  -- "2026-12-25" (ISO date, derivado de start_datetime)

-- 2. Si no hay fila, usar el JSON de business_hours
--    (el día se pasa como string: "monday", "tuesday", etc.,
--     derivado de start_datetime en la timezone del negocio)
SELECT
  json_extract(bp.business_hours, '$.' || ? || '.open')  AS open_time,
  json_extract(bp.business_hours, '$.' || ? || '.close') AS close_time
FROM business_profile bp
WHERE bp.id = 'singleton';
```

**Lógica de aplicación**:
- Si la query 1 retorna fila con `is_closed = 1` → `Error: el negocio está cerrado el {fecha} ({reason}).`
- Si la query 1 retorna fila con `is_closed = 0` → usar `open_time`/`close_time` de la exception (horario especial para esa fecha).
- Si la query 1 NO retorna fila → usar la query 2 con el día de la semana:
  - Si `open_time IS NULL` → `Error: el negocio no abre los {día}.`

**3b. ¿El Profesional trabaja ese día?**

```sql
SELECT start_time, end_time
FROM schedules
WHERE professional_id = ? AND day_of_week = ?;
```

Si no hay fila → `Error: el Profesional {name} no trabaja los {día}.`

**3c. ¿El slot cabe dentro del horario de atención y del horario del profesional?**

```sql
-- slot_start >= max(business_open, pro_start)
-- slot_end   <= min(business_close, pro_end)
```

Si el slot empieza antes de la apertura:
`Error: el horario de atención comienza a las {open}.`

Si el slot termina después del cierre:
`Error: el servicio dura {duration} minutos pero solo quedan {remaining} antes del cierre a las {close}.`

**3d. ¿Hay overlap con otra reserva no cancelada?**

```sql
SELECT 1
FROM bookings
WHERE professional_id = ?
  AND status != 'cancelled'
  AND start_datetime < ?     -- proposed end_datetime
  AND end_datetime   > ?     -- proposed start_datetime
LIMIT 1;
```

> La comparación SQL de strings es correcta porque todos los valores `*_datetime` están normalizados como UTC ISO 8601 (orden lexicográfico = orden cronológico).

Si retorna fila → `Error: el Profesional {name} ya tiene una reserva de {existing_start} a {existing_end}.`

**3e. ¿El slot no está en el pasado?**

El repositorio parsea `start_datetime` a `time.Time` en UTC (usando `time.ParseInLocation` con la timezone del negocio) y compara con `time.Now().UTC()`. Si `startTime.Before(now)`, retorna `&SemanticError{Code: ErrCodeSlotInPast, Message: "no se puede reservar en el pasado."}`. La comparación NO se realiza vía SQL string comparison.

##### Paso 4 — `create_booking()` (INSERT atómico)

Si todas las validaciones pasan (o si el LLM decide crear la reserva directamente sin hacer `check_availability` primero), se inserta la reserva con `status='pending'` mediante un **INSERT atómico** que verifica overlap en el mismo statement:

**Asunción explícita**: si el caller (MCP handler) llama `CreateBooking`
directamente sin antes llamar `CheckAvailability`, las validaciones de 3a
(horario del negocio), 3b (profesional trabaja), 3c (slot cabe en horario)
y 3e (no en el pasado) NO se ejecutan. Solo el overlap check (3d) se ejecuta
atómicamente vía el `INSERT ... WHERE NOT EXISTS`. Esto es aceptable en
Fase 1 (loopback, single-writer) pero Fase 2+ debería mover esas
validaciones dentro de `CreateBooking` para que sean obligatorias.

```sql
INSERT INTO bookings (
    id, client_id, professional_id, service_id,
    start_datetime, end_datetime, status, notes,
    payment_method, created_at, updated_at
)
SELECT ?, ?, ?, ?, ?, ?, 'pending', ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'), strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
WHERE NOT EXISTS (
    SELECT 1 FROM bookings
    WHERE professional_id = ?
      AND status != 'cancelled'
      AND start_datetime < ?     -- proposed end_datetime
      AND end_datetime   > ?     -- proposed start_datetime
);
```

`end_datetime` se calcula en Go (Paso 2) como `endTime := startTime.Add(time.Duration(service.DurationMinutes) * time.Minute)` y se almacena como `endTime.UTC().Format("2006-01-02T15:04:05.000Z")` (ISO 8601 UTC con milisegundos). `startTime` es el resultado de cargar la timezone con `loc, err := time.LoadLocation(business_profile.timezone)` (si `err != nil`, retornar `&SemanticError{Code: ErrCodeInternal, Message: "no se pudo cargar la zona horaria 'X': ..."}`), luego parsear con `time.ParseInLocation(time.RFC3339, start_datetime, loc)` y convertir a UTC con `.UTC()`.

Si `RowsAffected() == 0`, el insert no ocurrió porque existe un overlap. El repositorio retorna `&SemanticError{Code: ErrCodeBookingOverlap, Message: "el Profesional {name} ya tiene una reserva de {a} a {b}."}`.

##### Paso 5 — Generar alerta pendiente

```sql
INSERT INTO pending_alerts (type, message, scheduled_datetime, status,
                            related_booking_id, created_at)
VALUES ('confirmation_requested',
        'Confirmar reserva de {client_name} con {pro_name} el {start_datetime}',
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now'),
        'pending',
        ?,
        strftime('%Y-%m-%dT%H:%M:%fZ', 'now'));
```

Hermes consumirá esta alerta con `get_pending_alerts()` y la marcará como enviada con `mark_alert_as_sent()` después de confirmar con el cliente vía WhatsApp/Telegram.

##### Mensajes semánticos — Tabla de referencia

| Validación fallida | Mensaje (español) |
|---|---|
| Negocio cerrado por excepción (feriado/evento) | `Error: el negocio está cerrado el {fecha} ({reason}).` |
| Servicio no existe | `Error: el servicio '{name}' no existe o no está disponible.` |
| Profesional no existe | `Error: el profesional '{name}' no está disponible.` |
| Negocio cerrado semanal (no hay exception pero JSON lo marca cerrado) | `Error: el negocio no abre los {día}.` |
| Profesional no trabaja ese día | `Error: el Profesional {name} no trabaja los {día}.` |
| Slot antes de la apertura | `Error: el horario de atención comienza a las {open}.` |
| Slot antes del horario del profesional (3c) | `Error: el Profesional {name} empieza a las {start}.` |
| Slot después del cierre | `Error: el servicio dura {duration} minutos pero solo quedan {remaining} antes del cierre a las {close}.` |
| Overlap con otra reserva | `Error: el Profesional {name} ya tiene una reserva de {existing_start} a {existing_end}.` |
| Slot en el pasado | `Error: no se puede reservar en el pasado.` |

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
- Como **soporte técnico**, quiero **poder conectarme por SSH a la VPS del cliente y detener/iniciar el servicio**, para **aplicar updates o restaurar backups sin pedirle nada al cliente**.

---

## 5. Requerimientos

### 5.1 Requerimientos Funcionales (RF)

**RF1: Configuración inicial del negocio vía prompts inline en `install.sh`**
- **Descripción**: El sistema debe capturar los datos de `business_profile`, `professionals` iniciales y `services` iniciales a través de prompts interactivos en `install.sh` (bash + `read -p` + regex), con validación por campo antes de permitir avanzar. La salida son archivos JSON en `~/.config/mcp-appointments-crm/setup/` (Linux) o su equivalente platform-native según §3.5. Si el usuario cancela (`Ctrl+C`) a mitad del ingreso, el script guarda un checkpoint `setup.json.tmp` que permite resumir al re-correr `install.sh`.
- **Prioridad**: Must
- **Criterios de Aceptación** (formato Gherkin):
  - [ ] Dado que el usuario ejecuta `install.sh` por primera vez, cuando completa todos los prompts, entonces el sistema genera `setup_business.json`, `setup_staff.json` y `setup_services.json` válidos en `~/.config/mcp-appointments-crm/setup/`, y elimina `setup.json.tmp`.
  - [ ] Dado que el usuario ingresa un email con formato inválido en `contact_email`, cuando intenta avanzar, entonces `install.sh` muestra un error de validación y vuelve a pedir el campo (loop de reintentos) sin avanzar.
  - [ ] Dado que el usuario cancela (`Ctrl+C`) el setup a mitad del ingreso, cuando re-ejecuta `install.sh`, el sistema detecta `setup.json.tmp` y le ofrece: (R)esumir desde el último checkpoint, (S)tart over, o (Q)uit. Si elige R, los campos ya completados no se vuelven a preguntar.

**RF2: Exposición de identidad del negocio vía MCP**
- **Descripción**: El sistema debe exponer los tools `get_business_profile()` y `update_business_profile(fields...)` que leen y modifican la tabla `business_profile` a través del protocolo MCP.
- **Prioridad**: Must
- **Criterios de Aceptación**:
  - [ ] Dado que el primer inicio del servicio terminó exitosamente, cuando Hermes invoca `get_business_profile()`, entonces el sistema retorna un JSON con todos los campos del negocio actual.
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
  > **Nota sobre el flujo de reserva**: per la decisión arquitectónica D1
  > (atomic INSERT, design.md Decisión 11), `create_booking` ejecuta
  > **únicamente** el check de overlap atómico (Paso 4 §3.7.13). Las validaciones
  > de horario del negocio (3a), horario del profesional (3b), slot dentro del
  > horario (3c) y no-en-el-pasado (3e) se ejecutan vía `check_availability`.
  > Esta separación es intencional y documentada en el design; mover las
  > validaciones dentro de `create_booking` queda para Fase 2+. Los criterios
  > de aceptación a continuación reflejan este flujo.
  - [ ] Dado que el profesional X tiene horario Lunes 9-18 y hay una reserva de 10:00 a 11:00, cuando Hermes invoca `check_availability(professional_id=X, start=10:30)`, entonces el sistema retorna `false` y un mensaje `Error: el Profesional {name} ya tiene una reserva de {existing_start} a {existing_end}.`.
  - [ ] Dado que el profesional X no trabaja los domingos, cuando Hermes invoca `check_availability` con `start_datetime` en domingo, entonces el sistema retorna `Error: el Profesional {name} no trabaja los {día}.`. Tras la confirmación, `create_booking` ejecuta el INSERT atómico (Paso 4 §3.7.13) y el registro se crea o falla con overlap.
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
- **Descripción**: El sistema debe proveer un script `install.sh` ejecutable vía `curl | bash` que instala el binario, lo registra como servicio del SO e imprime al final una línea sugerida para schedular el script `backup.sh`.
- **Prioridad**: Must
- **Criterios de Aceptación**:
  - [ ] Dado que el script se ejecuta en una VPS Ubuntu limpia (sólo con `curl` y `bash`), cuando termina exitosamente, entonces el servicio `mcp-appointments-crm` está activo (`systemctl is-active` o equivalente) y el log final imprime `http://127.0.0.1:3000/mcp` y el log final muestra la línea sugerida para schedular `backup.sh` en `crontab` (u otro scheduler nativo según OS).
  - [ ] Dado que el script se ejecuta sin los archivos JSON de `setup/`, cuando el sistema valida los prerrequisitos, entonces imprime `Error: ejecute primero install.sh (que captura los datos del negocio) y vuelva a correrlo` y termina con exit code 1 sin instalar el binario ni registrar el servicio.
  - [ ] Dado que el script terminó exitosamente, cuando el operador revisa la salida, entonces encuentra al final un snippet sugerido para `crontab` con la frecuencia por defecto (1 vez al día, 03:00 hora local) que puede agregar manualmente.
  - [ ] Dado que `sqlite3` CLI no está instalado en el sistema, cuando el script `install.sh` termina exitosamente, entonces el log final incluye un bloque "Recommended additional tools" con el comando de instalación específico para el OS detectado, **sin ejecutar la instalación** (ver [ADR-0005](../architecture/0005-optional-external-tools.md)).

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
| Disponibilidad | El servicio debe reiniciarse automáticamente ante crash | Unit de systemd con `Restart=always` (equivalente launchd `KeepAlive=true`) |
| Mantenibilidad | Cobertura de tests sobre el repository layer | > 80% con `go test -cover` |
| Seguridad | 100% de queries con prepared statements | Linter custom o test de auditoría que falle si hay concatenación |
| Seguridad | Puerto expuesto solo en loopback; bind y puerto configurables vía env vars + `.env` | Doble capa: bind a `127.0.0.1` (IPv4) o `::1` (IPv6) (default, configurable vía `MCP_BIND` con validación de loopback 127.0.0.0/8 o ::1) + `IPAddressAllow=127.0.0.1` en systemd. Puerto default `3000` configurable vía `MCP_PORT`. Precedencia: env vars > `~/.config/mcp-appointments-crm/.env` > defaults. Ver ADR-0007. |
| Configurabilidad | Bind y puerto sin recompilar | `MCP_BIND` (default `127.0.0.1`) y `MCP_PORT` (default `3000`) vía env vars del sistema o archivo `.env` en el config dir |
| Seguridad | Servicio corre sin root | User-level systemd (`~/.config/systemd/user/`), launchd `LaunchAgents/`, Task Scheduler user task. No se crea `appuser` dedicado. |
| Seguridad | Permisos de directorio restrictivos al crear el path del SQLite | `os.MkdirAll(dir, 0750)` en `internal/db/database.go` |
| Observabilidad | Logs estructurados en stdout (JSON) | `slog` de Go stdlib; nivel configurable vía env var |
| Resiliencia | Backup del archivo SQLite | Script `scripts/backup.sh` portable (bash) que el operador puede schedular con la herramienta que prefiera (cron, systemd timer, launchd, Task Scheduler, o solución del proveedor de VPS) |

---

## 6. Restricciones Técnicas

### 6.1 Stack Técnico

- **Backend**: Go 1.26.4 (binario `mcp-server`)
- **Base de Datos**: SQLite vía `modernc.org/sqlite` v1.53+ (pure Go, sin CGo) con FTS5 nativo
- **MCP**: Protocolo MCP sobre SSE en `http://127.0.0.1:3000/mcp` (loopback por default; bind y puerto configurables vía `MCP_BIND` + `MCP_PORT` — ver ADR-0007)
- **Infraestructura**: binarios nativos en la VPS/PC del cliente, gestionados por el service manager del SO
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
- **Controles de seguridad requeridos**: prepared statements (100%), puerto loopback estricto, **bind validado contra loopback al arranque** (rechaza `0.0.0.0` y otras interfaces públicas antes de bindear), servicio user-level sin root, validación regex/string en prompts de install.sh, mensajes semánticos sin stack traces al LLM, HTTPS para descarga del `install.sh` desde GitHub.

---

## 7. Roadmap y Fases

> **Regla**: 1 fase = 1 SDD (openspec/changes/<nombre-de-la-fase>/).

### Fase 1: db-layer (Estimación: M)

**Objetivo**: sentar las bases de persistencia con repository pattern, prepared statements, FTS5 sync via triggers y tests con `go-sqlmock`.

**Entregables**:
- `internal/db/database.go` reescrito: 11 tablas (10 de dominio PRD §3.7 + `schema_version`) + 6 triggers FTS5 + 4 índices secundarios
- `internal/db/database_test.go` (integración FTS con SQLite en memoria)
- `internal/model/` (8 archivos: 1 struct por tabla de dominio)
- `internal/repository/errors.go` (sentinels + `SemanticError{Code, Message, Cause}`)
- `internal/repository/{business_profile,business_hours_exception,clients,services,professionals,schedules,bookings,pending_alerts}.go` (8 `*_Repo.go`)
- Tests con `go-sqlmock` cubriendo >80% del repository
- Índices secundarios en `bookings (professional_id, start_datetime, end_datetime)` y `pending_alerts (scheduled_datetime, status)`

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
- Templates de user-level service unit (systemd `~/.config/systemd/user/`, launchd `~/Library/LaunchAgents/`, Task Scheduler user task) con bind a `127.0.0.1` (default, configurable vía `MCP_BIND`)

**Definition of Done**:
- [ ] 6+ tools MCP registrados y funcionales
- [ ] Endpoint SSE responde en `http://127.0.0.1:3000/mcp` (o en el bind/puerto configurado vía `MCP_BIND`/`MCP_PORT`/`.env`)
- [ ] El servicio corre bajo el usuario que invoca `install.sh` (verificable con `systemctl --user show mcp-appointments-crm -p User` o `ps -o user= -p $(pgrep mcp-server)`)
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

### Fase 4: install.sh con prompts interactivos (Estimación: S)

**Objetivo**: extender `install.sh` con prompts interactivos para configurar `business_profile`, `professionals` iniciales y `services` iniciales, con validación por campo y checkpoint para cancelación. Sin binario TUI separado.

**Entregables**:
- `install.sh` con bloque de prompts (read -p + regex validation)
- Lógica de checkpoint en `~/.config/mcp-appointments-crm/setup.json.tmp` (escritura tras cada respuesta válida; lectura + oferta de resume en re-run)
- Finalización atómica: 3 JSONs en `~/.config/mcp-appointments-crm/setup/` (business, staff, services) + borrado de `setup.json.tmp`
- Cobertura de tests para `install.sh` (bats, shunit2, o equivalente)

**Definition of Done**:
- [ ] `install.sh` guía al usuario paso a paso con prompts y validación
- [ ] Cada campo valida antes de permitir avanzar (regex para email, formato `HH:MM` para horarios, coordenadas geográficas, IANA timezone, etc.)
- [ ] Si el usuario cancela (`Ctrl+C`) a mitad, el último checkpoint queda en `setup.json.tmp`; re-ejecutar `install.sh` detecta el checkpoint y ofrece [R]esume / [S]tart over / [Q]uit
- [ ] Al finalizar, los 3 JSON están en `~/.config/mcp-appointments-crm/setup/` con schema válido, y `setup.json.tmp` se borra
- [ ] Tests de los 3 escenarios: fresh install, cancel + resume, cancel + start-over pasan

### Fase 5: install-and-service (Estimación: S)

**Objetivo**: script de despliegue automatizado para la VPS del cliente + script de backup portable + documentación de instalación.

**Entregables**:
- `scripts/install.sh` ejecutable vía `curl | bash`
- Verificación de prerrequisitos (JSON existen, OS soportado)
- Descarga del binario correcto según OS/arquitectura desde GitHub Releases
- Registro del binario como user-level service: `~/.config/systemd/user/mcp-appointments-crm.service` (Linux), `~/Library/LaunchAgents/com.mcp.appointments.server.plist` (macOS), Task Scheduler user task (Windows)
- Impresión al final de `install.sh` de la línea sugerida para schedular `backup.sh` (sin auto-configurar ningún scheduler)
- Bloque "Recommended additional tools" al final del log: lista cada herramienta opcional (ej. `sqlite3` CLI) con su estado (✓ encontrado / ⚠ no encontrado) y el comando de instalación específico para el OS detectado; **nunca ejecuta la instalación** (ver [ADR-0005](../architecture/0005-optional-external-tools.md))
- `scripts/backup.sh` portable (bash, sin scheduler) disponible en el repo y en el release
- Ejecución de `loginctl enable-linger <user>` (sólo en Linux) para que el servicio user-level siga corriendo tras logout
- Log final con el endpoint `http://127.0.0.1:3000/mcp`
- `docs/installation.md` con el manual de uso del script
- `docs/maintenance.md` con el manual de soporte anual

**Definition of Done**:
- [ ] En una máquina con SO soportado (Linux, macOS 13+ o Windows 10+), el comando `curl -fsSL <url> | bash` (o `iwr -useb <url> | iex` en Windows) deja el sistema corriendo en < 5 minutos
- [ ] El script `backup.sh` está disponible en el repo y en el release, y produce un `.gz` ejecutándose manualmente con `./scripts/backup.sh`
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
| D2 | VPS o PC del cliente con SO soportado (Linux, macOS 13+, Windows 10+) | Bloqueante | Aprovisionar por el cliente | Cliente |
| D3 | Suscripción a un LLM (OpenAI, Anthropic, etc.) | Bloqueante | Aprovisionar por el cliente | Cliente |
| D4 | Cuenta de WhatsApp Business / Telegram Bot | Paralela | Configurar por el cliente vía Hermes | Cliente |
| D5 | Librería MCP para Go (oficial o comunitaria) | Bloqueante para Fase 2 | A evaluar al inicio de Fase 2 | Kike |

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
- **Prompt interactivo**: pregunta que el script `install.sh` hace al usuario vía `read -p` (bash). Se valida con regex antes de avanzar; el valor se persiste en `setup.json.tmp` (checkpoint) tras cada respuesta válida. Equivale funcionalmente a un campo de formulario en una TUI pero sin la complejidad de un framework.
- **Checkpoint de setup**: archivo `~/.config/mcp-appointments-crm/setup.json.tmp` que el `install.sh` escribe tras cada prompt válido. Si el usuario cancela, el checkpoint queda con los datos ingresados hasta el momento; re-ejecutar `install.sh` ofrece [R]esumir / [S]tart over / [Q]uit.
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
- `modernc.org/sqlite`: <https://pkg.go.dev/modernc.org/sqlite>.

---

## 10. Historial de Cambios

| Fecha | Versión | Autor | Cambios |
|-------|---------|-------|---------|
| 2026-06-24 | 1.0 | Kike | Creación inicial del PRD a partir de `docs/SDD.md` y `docs/common/prd-template.md`. Incluye 9 RF (Must + Should), 11 RNF, 5 fases de roadmap y 7 riesgos identificados. |
| 2026-06-26 | 1.1 | Kike | ADR-0008: reemplazo de `config-wizard` TUI (Bubble Tea) por prompts interactivos en `install.sh` con checkpoint. RF1 reformulado, Fase 4 simplificada (M→S), eliminadas referencias a TUI/MVU/Bubble Tea del alcance y glosario. |
