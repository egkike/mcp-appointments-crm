# Guía de instalación de MCP Appointments CRM

> Esta guía lleva un VPS limpio (Ubuntu 22.04+ o macOS) hasta tener el servicio `mcp-appointments-crm` activo y respondiendo en `http://127.0.0.1:3000/mcp`.
>
> ⚠️ **Assets publicados**: el release publicado más reciente es **v0.12.2** (2026-11-12: patch — el detector de allowlist ya no alarma en falso; anterior: v0.12.1, patch — el advisory de allowlist visible en la TUI; anterior: v0.12.0, minor — la opción 7 del TUI audita la allowlist de tools del canal y muestra un advisory read-only en tres estados (nunca escribe `config.yaml`), corrección de las docs de seguridad (el boundary por plataforma existe pero está APAGADO por default; los bundles de canal son full-access) con el procedimiento verificado de allowlist mínima, y fix del `pr-gate` de CI (`cancel-in-progress: false`); anterior: v0.11.2, 2026-11-06: patch — hardening interno `internal/db`: fail-closed para `MCP_DB_PATH` con query string (antes: silent-ignore de los pragmas y `foreign_keys` OFF), paridad `:memory:` canónica `file::memory:`; anterior: v0.11.1, 2026-11-06 — patch: fix de producto `internal/db.buildDSN`: DSN `file:` canónica para que el server arranque en mac reales, bug descubierto por el smoke macOS e2e del propio repo; anterior: v0.11.0, 2026-11-04 — minor: uninstall end-to-end (script + Go sibling + docs), pre-flight de puerto del installer, framing público de números; anterior: v0.10.0, 2026-11-01 — Windows hardening ADR-0021, skill v1.1.1, contract de modos WhatsApp; el changelog completo vive en `docs/PRD.md`; anterior: v0.9.1 2026-10-22 — `list_hours_exceptions` en carrier `{exceptions:[…]}` para el contract root-object del cliente MCP de Hermes; v0.9.0 2026-10-22: minor de 25 tools — paquete B12 owner-in-chat: 2 tools de lectura nuevas, skill v1.1.0 dual persona, Telegram opcional/removible, fix de consola B14; v0.8.1 2026-10-21: fix de seed B11 — despliegue vía `--version` que siembra el template pristino de la skill — y OQ-5 — wizard pide specialties por profesional para instalaciones nuevas; v0.8.0 2026-10-19: skill de recepcionista de Hermes, vista TUI "Ver audit log", `Purgar cuentas desactivadas` con doble confirmación; v0.7.0: tools 20→23 hours-exceptions + backend del bot; v0.6.1 y v0.6.0 anteriores); todos los releases desde v0.4.0 salen del pipeline GoReleaser y traen la matriz Unix de 4 plataformas (`Darwin_arm64/x86_64`, `Linux_arm64/x86_64`) + `checksums.txt`; desde el covenant 2026-11-01 el zip de Windows ya no se publica (Windows = WSL2 con el asset Linux). El camino macOS de `install.sh` está implementado y descarga el asset `Darwin` correspondiente. Windows no tiene asset ni path de instalación nativo (covenant 2026-11-01; el instalador imprime el hint de WSL2 en el branch no-soportado) ([ADR-0014](./architecture/0014-release-and-deploy-workflow.md) Decision 3). `v0.3.0` fue el último armado a mano.
>
> El flujo tiene dos partes: (1) configuración inicial interactiva con el wizard (`curl -fsSLO ... && bash install.sh`, requiere TTY) y (2) despliegue pinned del binario con `install.sh --version vX.Y.Z`. Si ya generaste los JSONs de setup en otra máquina, podés saltar directo al despliegue copiando los archivos al directorio de configuración del host destino.

---

## Prerrequisitos

Verificá estos ítems antes de empezar:

- [ ] VPS o máquina propia con Ubuntu 22.04+ o macOS 12+.
- [ ] Usuario normal con shell `bash` y acceso por SSH (o consola local).
- [ ] `curl`, `tar` y una herramienta de SHA256 (`sha256sum` en Linux, `shasum -a 256` en macOS) en el `PATH`.
- [ ] Conexión saliente HTTPS a `github.com` y `raw.githubusercontent.com`.
- [ ] Puerto `3000` libre en `127.0.0.1` (o el puerto que configures en `.env`; el instalador lo verifica con pre-flight y aborta temprano nombrando las dos salidas si está ocupado).

> **No ejecutes la instalación como root.** Todo el pipeline es user-level: el binario,
> la DB, los JSONs de setup y el servicio quedan bajo el home del usuario que
> invoca `install.sh` (ADR-0002).

---

## Paso 0 — El flujo completo (receta recomendada)

> La instalación del **server MCP es autónoma** — no requiere Hermes ni ningún
> agente para correr (service + DB + cuenta owner, Pasos 1–3). Lo que
> recomendamos es fijarse el **orden del flujo productivo completo** de la
> instalación típica a producción, para que el smoke del canal sea end-to-end
> apenas termina el setup:

| # | Paso | Artefacto | Nota |
|---|---|---|---|
| 1 | (Opcional, caso típico: ya lo tenés) **Agente de canal vivo con WhatsApp del negocio configurado** — Hermes gateway instalado y el canal pareado | `~/.hermes/` con config del bot | en el caso greenfield este paso va al final (rama B) |
| 2 | **Instalar el server MCP** — este documento (Pasos 1–3) | service + DB + owner | independiente del paso 1 |
| 3 | **Puente de integración** — `mcp-server admin tui` opción 7 (Configurar Hermes) escribe el server entry y dispara la cascada de la skill | `mcp_servers.mcp-appointments` en el config del gateway | necesita que el server MCP ya esté corriendo (deriva la URL de `MCP_BIND`/`MCP_PORT`) y, si Hermes no existiera todavía, se instala en este punto (guía de Hermes: `docs/gateway-contract.md` y docs del runtime) |
| 4 | **Smoke del canal** — runbook `gateway-contract.md` §7.5 (una fase del orden: Transfer Ownership del owner real **antes** de la opción 7 que persiste el teléfono) | bot respondiendo con persona correcta | los criterios de persona por canal viven en §8.1/§9 |

**Rama A — agente ya andando (flujo típico de producción, recomendado):** pasos
1 → 2 → 3 → 4. Con el canal ya funcionando, después de la opción 7 el smoke sale
end-to-end al toque (canal vivo + tools + skill recién renovada por la cascada).

> **Host objetivo (24/7):** una VPS o una máquina local **siempre-encendida**
> que no suspende — el servicio MCP es user-level (`systemd --user` + linger
> en Linux; launchd user-agent en macOS, que requiere que la sesión del
> usuario esté activa). El upgrade del host apaga el server: una laptop que
> suspende al cerrar la tapa mata la disponibilidad; en Windows, WSL2 hereda
> el arranque del host Windows (ver nota Windows de arriba).

> ⚠️ **Puerto (rama A — se fija ANTES del paso 2):** si el agente del canal ya
> corre en esta máquina y su WhatsApp/config ya está activo, el puente de
> WhatsApp (`bridge.js`) ya tiene — o tomará — `127.0.0.1:3000`. Cuando el
> Paso 2 deja el server con el default `3000` contra un puerto ocupado, el
> binario no arranca (`bind: address already in use`) y el unit entra en un
> crash-loop de auto-restart cada 5s (verificado 2026-11-02 en vivo). Fijá
> desde ya **`MCP_PORT=3001`** en el paso 2 (higiene de puertos:
> `gateway-contract.md` §6.1; la otra palanca equivalente es fijar el
> `bridge_port` del bridge en el config del gateway). La variante trampa
> silenciosa (agente configurado pero sin parear aún) también cae acá: el
> MCP arrancaría bien con 3000, y la pareja posterior del WhatsApp chocaría
> contra el server ya corriendo — el canal no conectará sin una de las dos
> palancas aplicadas. Nunca queda a ojo: el instalador hace **pre-flight del
> puerto** justo después de escribir el `.env` y antes de instalar el binario
> y el servicio — si el puerto está ocupado por otro proceso, aborta temprano
> con las mismas dos palancas (un listener del propio unit, como durante un
> upgrade en caliente, no cuenta como conflicto; sin `ss` ni `lsof`, advierte
> que el chequeo se salteó y continúa).

**Rama B — máquina nueva (greenfield):** 2 → (instalar Hermes + WhatsApp del
negocio) → 3 → 4. El resultado final es idéntico: ambos caminos convergen en la
opción 7.

> **El agente recomendado (soporte de primera clase):** [Hermes](https://hermes-agent.nousresearch.com/)
> (NousResearch) es el único agente con integración de primera clase del
> producto — canal WhatsApp/Telegram + la skill de recepción instalada como
> skill oficial (`~/.hermes/skills/`, estándar verificado en la doc del
> producto); cualquier otro agente MCP se conecta con la guía de abajo.
> **Instalalo siguiendo la doc oficial del site** — el runtime es de Nous y se
> actualiza con su propio updater (`hermes update`); este repo no lo instala ni
> lo versiona: solo lo configura para hablar con nuestro MCP server. Verificación
> mínima antes de seguir: `hermes --version` responde y el gateway del canal
> está vivo. El contrato del pairing (canales, skill, security boundary) está
> en [gateway-contract.md](gateway-contract.md). Referencia operativa
> (2026-11-06): los smokes de este repo corrieron contra Hermes **v0.22.7**; el VM de producción corre **v0.23.0** (`2bd378ed1b8d`, Hearth Release) verificado con server **v0.11.2** (healthz + gateway activo), y la upgraded-notes de v0.23.0 no toca nuestra instalación (el gateway corre `gateway run`, no el temporal `hermes mcp serve`).

**Otros agentes (no Hermes):** no hay installer — la guía documentada es
[Conectar tu agente MCP](#conectar-tu-agente-mcp-hermes-u-otros) (el step 3
equivale al snippet de config de tu agente + smoke con `curl`; el `caller-id`
del Paso 1 de este doc es la identidad que inyectan).

---

## Paso 1 — Configuración inicial (interactiva, requiere TTY)

El instalador genera tres archivos JSON con los datos del negocio, staff y servicios. Esta parte **solo funciona en una terminal real** (`[ -t 0 ]`); no la corras por pipe.

```bash
# Descargá y ejecutá el wizard en una terminal (requiere TTY)
curl -fsSLO https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh && bash install.sh
```

> Nota: no hace falta `chmod +x` porque el script se invoca con `bash install.sh`. Evitá `curl ... | bash` en este paso: el instalador rechaza a propósito la ejecución por pipe sin TTY (ver [troubleshooting](#el-pipe-curl--bash-se-cuelga-o-muestra-errores-de-read)).

Completá los prompts. Al finalizar se guardarán:

```text
~/.config/mcp-appointments-crm/setup/setup_business.json
~/.config/mcp-appointments-crm/setup/setup_staff.json
~/.config/mcp-appointments-crm/setup/setup_services.json
```

Si cancelás a mitad de camino, el script deja un checkpoint `setup.json.tmp` para reanudar la próxima vez que corras `bash install.sh`.

Al final de la configuración (después de los servicios) el wizard pregunta **por cada profesional** qué servicios atiende:

```text
Servicios del negocio:   (1) Corte de Cabello   (2) Lavado de Cabello   (3) ...
Servicios que atiende Romina (números o nombres separados por espacio o comas, Enter = ninguno):
```

La respuesta acepta índices (`1 2`), nombres exactos (`Corte de Cabello`) o una mezcla separada por comas (`Corte de Cabello, 2`); duplicados se ignoran. `Enter` vacío deja la relación vacía (podés cargarla después con `update_professional`). El wizard vuelca esos nombres a `"specialties"` en `setup_staff.json` y el primer arranque del servidor los registra con los ids reales de `setup_services.json`, así que una instalación nueva nace con la relación profesional↔servicio ya cargada (hoy se consulta con `update_professional`/SQL y la lectura por servicio la trae un change posterior).

> **Nota sobre el primer arranque:** el servidor siembra la base de datos automáticamente en el primer arranque si el perfil del negocio está vacío, leyendo los tres archivos JSON generados arriba. En los arranques siguientes la importación se omite gracias al guarda que detecta que el perfil ya existe, por lo que las modificaciones que hagas en tiempo de ejecución (desde las herramientas MCP o directamente en la base de datos) no se sobrescriben. Conservá los JSON como respaldo o para reprovisionar el sistema; ya no son solo artefactos finales sin consumidor.

---

## Paso 2 — Despliegue pinned del binario

Una vez que los tres JSONs de setup existen, desplegá la versión que quieras. El flag `--version` del instalador **selecciona el tag del release** a instalar; es distinto de `mcp-server --version`, que reporta la versión del binario ya instalado.

### Opción A — One-line pipe (no interactiva)

```bash
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh \
  | bash -s -- --version v0.12.2
```

> El despliegue por `--version` siembra el **template pristino** de la skill de recepcionista en `<CONFIG_DIR>/templates/recepcionista-negocio/SKILL.md` (0600/0700). El wizard interactivo sí instala la skill renderizada; el deploy nunca la renderiza ni la reescribe — el cascade de la TUI (opción 7) es quien regenera la skill instalada a partir de ese template.

### Opción B — Descarga previa del script (recomendada para auditar)

```bash
curl -fsSLO https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh
bash install.sh --version v0.12.2
```

Reemplazá `v0.12.2` por el tag exacto que querés instalar. El formato obligatorio es `vMAJOR.MINOR.PATCH` (por ejemplo `v0.9.1`). No se acepta `latest` ni pre-releases desde el instalador.

### Qué hace el despliegue

1. Resuelve los paths XDG por OS (`DATA_DIR`, `BIN_DIR`, `LOG_DIR`, `ENV_FILE`).
2. Rechaza ejecución como root.
3. Valida que existan `curl`, `tar` y la herramienta SHA256.
4. Requiere los tres JSONs de setup; si falta alguno, aborta nombrándolo.
5. Detecta OS y arquitectura (`uname -s` / `uname -m`) y descarga el asset correcto (el mapa soporta `Linux`/`Darwin` × `x86_64`/`arm64`; hoy solo está publicado el de Linux x86_64):
   - `mcp-appointments-crm_Linux_x86_64.tar.gz`
   - `mcp-appointments-crm_Linux_arm64.tar.gz`
   - `mcp-appointments-crm_Darwin_x86_64.tar.gz`
   - `mcp-appointments-crm_Darwin_arm64.tar.gz`
6. Verifica SHA256 del archive contra `checksums.txt` **antes de extraer**.
7. Extrae el binario y los templates de servicio.
8. Crea los directorios de datos/logs con permisos owner-only.
9. Crea `~/.config/mcp-appointments-crm/.env` si no existe con:
   ```bash
   MCP_BIND=127.0.0.1
   MCP_PORT=3000
   ```
   Si ya existe `.env`, se preserva byte-idéntico (upgrade).

   > **Recomendación si vas a correr el canal WhatsApp del negocio en esta misma
   > máquina** (installación típica del producto con el gateway de Hermes):
   > antes del primer arranque, editá el `.env` y fijá **`MCP_PORT=3001`** (o
   > cualquier otro puerto libre). El default `3000` es el puerto contratado
   > del MCP, pero **el puente de WhatsApp de Hermes (bridge.js/Baileys)
   > también usa 3000 como puerto local por default** y choques con él dejarían
   > el canal WhatsApp sin arrancar (`standalone_bridge_port` del runtime
   > "never allocates", no busca un puerto libre). Con el MCP en 3001 ambos
   > servicios conviven sin configuración extra. Si preferís mantener el MCP en
   > 3000, la alternativa equivalente es fijar el `bridge_port` del bridge
   > (config `platforms.whatsapp.extra.bridge_port` del runtime; ver
   > [gateway-contract §6.1](gateway-contract.md)) — una de las dos palancas es
   > necesaria; ninguna es opcional WhatsApp-on-same-host. Verificación:
   > `curl --fail http://127.0.0.1:3001/healthz` y re-coré la opción 7 de la
   > TUI para que la entrada `mcp_servers` de Hermes derive la URL del puerto
   > nuevo.
10. Instala el binario en `~/.local/bin/mcp-server` de forma atómica.
11. Renderiza e instala la service unit:
    - Linux: `~/.config/systemd/user/mcp-appointments-crm.service`
    - macOS: `~/Library/LaunchAgents/com.mcp.appointments.server.plist`
12. En Linux ejecuta `loginctl enable-linger "$USER"` para que el servicio sobreviva logout/reboot.
13. Verifica que `$BIN_DIR/mcp-server --version` imprima el tag solicitado y que el servicio esté activo.
14. Imprime un resumen con la línea sugerida para backup, herramientas recomendadas, URL del endpoint y `MCP_DB_PATH` real.

---

## Paso 3 — Verificación post-instalación

Ejecutá estos comandos en el mismo host. Deben dar los resultados indicados.

### 3.1 Servicio activo (Linux)

```bash
systemctl --user is-active mcp-appointments-crm
```

Resultado esperado:

```text
active
```

Si recién terminó el instalador, podés esperar unos segundos o consultar estado:

```bash
systemctl --user status mcp-appointments-crm --no-pager
```

### 3.2 Linger habilitado (Linux)

```bash
loginctl show-user "$USER" -p Linger
```

Resultado esperado:

```text
Linger=yes
```

### 3.3 Endpoint MCP respondiendo

```bash
curl --fail http://127.0.0.1:3000/healthz
```

Resultado esperado: HTTP 200 con:

```json
{"status":"ok","version":"v0.3.0"}
```

> **Nota:** un GET pelado a `http://127.0.0.1:3000/mcp` responde **405 por
> diseño** (REQ-MT-002): el endpoint MCP solo acepta POST JSON-RPC. Ese 405 es
> buena señal — prueba que el server vive y rutea — pero la verificación de
> liveness es `/healthz`.

### 3.4 Cuenta owner (obligatoria antes de usar `/mcp`)

Una instalación limpia **no tiene ninguna cuenta owner**. El seeder de arranque
(`config.SeedOnBoot`) crea `business_profile`, `professionals`, `schedules` y
`services`, pero **nunca** inserta una fila en `accounts`. Con `accounts` y `clients`
vacías, todo caller resuelve a `ErrUnauthenticated`: mientras no exista una cuenta,
**toda** petición a `/mcp` es rechazada, aunque `/healthz` siga respondiendo 200.

Ojo con **cómo** se rechaza, porque cambia lo que hay que mirar: el middleware de
auth responde **401**, pero en `/mcp` el traductor JSON-RPC lo convierte en un
envelope de error **con HTTP 200** (`code -32000`, mensaje `no te reconozco. Por
favor regístrate primero.`) para que el cliente MCP lo vea como error de protocolo
y no de transporte (`internal/mcp/auth_translator.go`). Consecuencia práctica: un
`curl --fail` contra `/mcp` **no** falla por status; hay que leer el cuerpo.

La cuenta se crea con el sub-comando de administración del binario (ADR-0010,
alcance en [ADR-0016](./architecture/0016-admin-tui-scope.md)):

```bash
~/.local/bin/mcp-server admin tui
```

En una instalación limpia (ningún owner activo) el comando abre el **wizard de
seed**: pide teléfono y nombre para mostrar, crea la fila del owner
(`role='owner'`, `is_active=1`) y escribe el archivo `caller-id` con el teléfono
elegido. No hay ningún `INSERT` manual en el flujo.

El wizard crea exactamente un owner: el single-owner invariant lo sostienen los
triggers de `accounts` más las validaciones previas del repositorio. Si el
comando vuelve a correr con un owner activo, no duplica la cuenta: repara el
archivo `caller-id` si falta y abre el menú de cuentas (Add Staff con picker de
profesional, desactivación soft delete, listados read-only, transferencia de
ownership, alta del owner como cliente, y **Configurar Hermes**, que bootstrap-ea
`~/.hermes/config.yaml` sin YAML a mano — ver el paso siguiente). Si existen filas de owner
**desactivadas**, el arranque ofrece reactivar una primero.

**Terminal interactiva:** con TTY en `stdin` y `stdout` el sub-comando abre la
TUI Bubble Tea; sin TTY (invocación por pipe, CI o script) cae al flujo de
consola por líneas, que cubre el mismo wizard de seed y el mismo menú. Ahí la
corrida no interactiva no queda a medias: es la misma lógica con otra
presentación.

Usá el teléfono que cargaste en el wizard como `X-Caller-Id`; con ese valor el
handshake del paso siguiente se autentica.

**Configurar Hermes:** con el owner creado, la opción **7 — Configurar Hermes**
del menú (consola o TUI) bootstrap-ea el config de Hermes del host: lee y
combina `~/.hermes/config.yaml` (crea la entrada `mcp_servers.mcp-appointments`
con el endpoint `http://<bind>:<puerto>/mcp` derivado de `MCP_BIND`/`MCP_PORT` y
el header `X-Caller-Id: "<teléfono>"`, siempre con el valor entre comillas),
preserva toda clave ajena y escribe el archivo atómicamente. El teléfono va
prefillado con el del owner activo (editable). Si el archivo no se puede
escribir, la opción muestra el bloque exacto para copiar a mano
([ADR-0017](./architecture/0017-hermes-config-tui.md)).

El grupo del **bot privado de Telegram** (token, chat id de la allowlist y
teléfono del owner) es **opcional**: los canales oficiales son el número de
WhatsApp del negocio y el chat local de Hermes, y Telegram es un canal aditivo
owner-only. Dejá el token vacío (Enter) y la opción escribe la entrada
`mcp_servers.mcp-appointments` **deja intacto** el bloque `MCP_TELEGRAM_*` ya existente, si lo
había, informando `Telegram: omitido (sin cambios en el bloque de Telegram)`; el server arranca
gual. Si
cargás el token, el chat id y el teléfono siguen siendo obligatorios (par
completo), y re-correr la opción 7 sin tocar el grupo preserva el bot que ya
estaba configurado.

Con el token vacío el flujo pregunta además **`¿Retirar el canal Telegram del
bot?`**, porque un grupo vacío tiene dos formas distintas y la decisión es tuya:
responder que no (o Enter) **conserva** el bloque `MCP_TELEGRAM_*` que el config
ya tuviera; responder que sí lo **elimina** y la opción informa
`Telegram: canal deshabilitado (bloque MCP_TELEGRAM eliminado)`. El retiro es
byte-limpio e idempotente: borra sólo las tres claves `MCP_TELEGRAM_*` del
`env` de `mcp_servers.mcp-appointments` (el bloque `env` desaparece si queda
vacío) y preserva el `url`, los `headers` con el `X-Caller-Id` entre comillas y
cualquier otra clave del archivo; si el canal nunca estuvo configurado, la
opción no cambia un solo byte.

**Archivo `caller-id`:** el teléfono del owner queda en
`~/.config/mcp-appointments-crm/caller-id` con permisos `0600` (el directorio se
crea `0700`), y `MCP_CONFIG_DIR` reemplaza ese directorio cuando la definís. El
archivo guarda el identificador tal cual — es lo que hay que mandar como
`X-Caller-Id` y lo que va a leer el chat local de Hermes cuando ese sub-comando
deje de ser un nombre reservado ([ADR-0012](./architecture/0012-hermes-chat-local.md)).

> La siembra de perfil, profesionales, horarios y servicios sigue siendo
> trabajo de `config.SeedOnBoot` desde los JSONs del wizard; después del primer
> arranque, esos datos operativos se editan con los tools Hermes de
> mantenimiento owner-only (`update_business_profile`,
> `create/update/delete_service`, `create/update_professional`,
> `upsert/delete_schedule`), sin SQL manual. El resto de los datos demo del
> [`demo-plan.md`](./demo-plan.md) (cliente demo, reserva histórica) se carga
> por SQL: ADR-0016 limita este sub-comando a identidad y cuentas.

### 3.5 Handshake MCP (requiere la cuenta owner de 3.4)

Verificación opcional del wire MCP (handshake POST `initialize`, sin sesión —
el server es stateless y no exige `Mcp-Session-Id`):

```bash
curl --fail -sS http://127.0.0.1:3000/mcp \
  -H 'Content-Type: application/json' \
  -H 'X-Caller-Id: <teléfono-del-owner>' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl-smoke","version":"0.0.1"}}}'
```

Debe devolver una respuesta JSON-RPC `initialize` con las capacidades del
server. Si el server espera otra `protocolVersion`, la respuesta lo indica. Si
el teléfono del owner no existe en `accounts`, el comando **no** devuelve el
envelope `initialize`: devuelve un envelope de error JSON-RPC (`code -32000`) con HTTP 200 —
por eso `curl --fail` igual termina en exit 0 y hay que inspeccionar el cuerpo.

### 3.6 Versión del binario instalado

```bash
~/.local/bin/mcp-server --version
```

Resultado esperado: el tag que instalaste, por ejemplo:

```text
v0.3.0
```

> **Diferencia clave:** `install.sh --version vX.Y.Z` le dice al instalador qué release descargar; `mcp-server --version` le pregunta al binario instalado qué versión es. Ambos comandos coexisten.

### 3.7 Ubicación de la base de datos de producción

```bash
sqlite3 ~/.local/share/mcp-appointments-crm/reservas.db \
  "SELECT name FROM sqlite_master WHERE type='table';"
```

Deberías ver las tablas del sistema (`business_profile`, `bookings`, etc.).

---

## Layout resultante (Linux XDG default)

| Componente | Ruta |
|---|---|
| Binario | `~/.local/bin/mcp-server` |
| Base de datos de producción | `~/.local/share/mcp-appointments-crm/reservas.db` |
| Backups | `~/.local/share/mcp-appointments-crm/backups/reservas-YYYYMMDD.db.gz` |
| Config (JSON + `.env`) | `~/.config/mcp-appointments-crm/` |
| Logs | Journal del usuario: `journalctl --user -u mcp-appointments-crm` (el unit no escribe archivo de log) |
| Service unit | `~/.config/systemd/user/mcp-appointments-crm.service` |

En macOS los paths de datos/logs/config usan `~/Library/Application Support/...` y `~/Library/Logs/...` según la tabla del PRD §3.5; la única excepción es el `.env` que lee el binario, que en todas las plataformas vive en `~/.config/mcp-appointments-crm/.env` (`internal/mcp/config.go`).

---

## Caveat importante: no corras `./mcp-server` a mano como servicio

El binario, cuando se ejecuta directamente sin el service unit, usa como default `~/.local/share/mcp-appointments-crm/reservas.db` (layout XDG, ADR-0002), la misma ruta que apunta el service unit. `MCP_DB_PATH` sigue siendo el override explícito.

El camino soportado en producción es el service unit, que inyecta:

```systemd
Environment=MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db
```

Como el default del binario ya es esa ruta, una ejecución manual abre la **misma base de datos** que el servicio: no hay split-brain por CWD. Aun así, no corras un segundo proceso contra la misma DB mientras el servicio está activo — dos procesos compiten por el archivo SQLite y por el puerto. Para operar el sistema siempre usá el servicio registrado.

---

## Troubleshooting

### El pipe `curl | bash` se cuelga o muestra errores de "read"

El flujo interactivo de setup **no funciona por pipe**. Si todavía no tenés los JSONs de setup, el instalador debe abortar con un mensaje claro. Solución:

```bash
# 1. En una terminal real, generá los JSONs
bash install.sh

# 2. Luego volvé a correr el deploy por pipe
```

El **deploy** (`bash install.sh --version vX.Y.Z`) sí funciona por pipe: está
corregido desde PR #69 (antes fallaba con `BASH_SOURCE: unbound variable`).
Solo el paso interactivo requiere terminal real.

### `systemctl --user` falla con "Failed to connect to bus"

Sobre SSH plano puede faltar `XDG_RUNTIME_DIR`. Antes de correr `systemctl --user`:

```bash
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id - u)}"
```

O usá `machinectl shell "$USER"@` si está disponible.

### `install.sh --version` reporta "Error: faltan archivos de setup: setup_business.json setup_staff.json setup_services.json"

Significa que no completaste el Paso 1. Corré `bash install.sh` en una terminal, o copiá los tres JSONs desde otra máquina al `SETUP_DIR` correspondiente.

### SHA256 mismatch

No bypasses la verificación. El archive está corrupto, fue modificado o el release no coincide. Reintentá una vez; si persiste:

```bash
curl -fsSLO https://github.com/egkike/mcp-appointments-crm/releases/download/v0.3.0/checksums.txt
curl -fsSLO https://github.com/egkike/mcp-appointments-crm/releases/download/v0.3.0/mcp-appointments-crm_Linux_x86_64.tar.gz
sha256sum -c checksums.txt --ignore-missing
```

Si el checksum oficial falla, no instales y abrí un issue en el repo.

### El servicio no arranca y el log dice "puerto 3000 en uso"

Causas típicas: (a) **el puente de WhatsApp de Hermes** (`bridge.js`, que por
default usa 3000 — añadido el 2026-11-01 con la prueba de onboarding), o (b)
cualquier otro proceso en el 3000. Dos salidas equivalentes, elegí UNA:

1. **Mover el MCP**: editá `~/.config/mcp-appointments-crm/.env` para usar otro
   puerto — la recomendación del producto para hosts con WhatsApp es
   `MCP_PORT=3001` (`docs/installation.md` §Paso 2) — y reiniciá:

```bash
systemctl --user restart mcp-appointments-crm
curl --fail http://127.0.0.1:3001/healthz
# después re-coré la opción 7 de la TUI (o editá la entrada mcp_servers) para
# que la URL de Hermes apunte al puerto nuevo
```

2. **Mover el puente**: fijá `platforms.whatsapp.extra.bridge_port` en el config
   del gateway (por ej. `3005`) — ver [gateway-contract §6.1](gateway-contract.md).

### Error de symlink en `DATA_DIR`, `BIN_DIR` o `LOG_DIR`

El instalador rechaza symlinks en esos directorios por seguridad. Si tenés un symlink intencional, eliminálo o cambiá el valor de `XDG_DATA_HOME`/`XDG_BIN_HOME`/`XDG_STATE_HOME` a una ruta real antes de correr el instalador.

---

## Conectar tu agente MCP (Hermes u otros)

> **La instalación limpia y estándar del producto es con Hermes**: el flujo
documentado (Paso 1–3 acá + `Configurar Hermes` en la TUI, `docs/gateway-
contract.md`) deja el server integrado con la cascada de la skill de
recepcionista y los canales WhatsApp/Telegram. **Si usás otro agente MCP**
(Claude Code, OpenClaw, OpenCode, Codex, un IDE…), el server es un **servidor
MCP estándar** y funciona con cualquiera que hable Streamable HTTP e inyecte
el header de identidad — esta sección es la guía universal de conexión.

### Contrato universal para cualquier cliente MCP

| Aspecto | Valor |
|---|---|
| Endpoint MCP | `http://127.0.0.1:3000/mcp` (POST JSON-RPC solo; `GET` responde 405) |
| Liveness | `GET http://127.0.0.1:3000/healthz` |
| Auth | **header `X-Caller-Id` con el teléfono del owner por request** (no hay OAuth/keys: el trust model es loopback-only, ADR-002; el header ES la identidad — roles por `accounts`/`clients`) |
| Identidad del owner | `~/.config/mcp-appointments-crm/caller-id` (creado por la TUI Paso 1 — esa pieza es agnóstica del agente) |
| Puerto no-3000 | el default es 3000 documentado; en hosts que corren además el canal WhatsApp leé la higiene de puertos en el Paso 2 de este doc + `gateway-contract.md` §6.1 |

> **Límite honesto de los headers estáticos**: los clientes MCP configuran
> headers **por server**, no per-sender. Con un agente distinto de Hermes
> tienes una única identidad caller por config del run; la resolución
> multi-emisor por canal (cliente/owner/staff que resuelven por el `from`
> del mensaje) es lo que hoy entrega el gateway de Hermes. Para un uso de
> owner self-host esto cubre todo el flujo con el owner; para un canal
> comercial multi-emisor, el agente necesita su implementación de gateway
> per-sender equivalente.

### Smoke de conexión con cualquier cliente (con `curl`)

```bash
# 1. liveness
curl --fail http://127.0.0.1:3000/healthz

# 2. catálogo de tools (25)
curl -s -X POST http://127.0.0.1:3000/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'X-Caller-Id: +5491100000000' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | head -c 400

# 3. lectura real ejemplo (caller con rol resuelto)
curl -s -X POST http://127.0.0.1:3000/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -H 'X-Caller-Id: <tu-teléfono-registrado>' \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_business_profile","arguments":{}}}'
```

### OpenClaw (verificado 2026-11-01, docs.openclaw.ai/gateway/config-extensions)

OpenClaw tiene cliente MCP remoto (transport `streamable-http` o `sse`) con
map de headers por server: bajo `mcp.servers` de su config:

```json5
mcp: {
  servers: {
    "mcp-appointments": {
      url: "http://127.0.0.1:3000/mcp",
      transport: "streamable-http",
      headers: { "X-Caller-Id": "<teléfono del owner>" },
      requestTimeoutMs: 20000,
    }
  }
}
```

Después: `openclaw mcp list` / `openclaw mcp doctor` para ver la conexión;
si OpenClaw corre en otra máquina, recordar que el server es loopback-only
(ADR-002: un túnel/SSH local-port-forward, evalufado como decision del
operador; nunca exponer el puerto más allá del loopback.

### Claude Code / Codex (`.mcp.json` de project scope)

```json
{
  "mcpServers": {
    "mcp-appointments": {
      "type": "http",
      "url": "http://127.0.0.1:3000/mcp",
      "headers": { "X-Caller-Id": "<teléfono del owner>" }
    }
  }
}
```

*(Sintaxis vigente a la fecha de publicación del doc: validála contra la docs
> de tu client si su formato cambió.*)

### OpenCode (`opencode.json` del workspace)

```json
{
  "mcp": {
    "mcp-appointments": {
      "type": "remote",
      "url": "http://127.0.0.1:3000/mcp",
      "headers": { "X-Caller-Id": "<teléfono del owner>" },
      "enabled": true
    }
  }
}
```

## Configurar el canal WhatsApp (modo A/B)

El canal oficial del producto se configura **en el wizard del runtime Hermes**
(`hermes whatsapp`) — no en la TUI de este repo. La única decisión del operador
con efecto observable es **el modo de instalación**, y se toma **antes del paso
2** del runbook ([gateway-contract §7.5](gateway-contract.md), regla
"choose BEFORE step 2"); los pasos 2–5 son idénticos en ambos modos (persona,
gates y funnel son mode-invariant por diseño — [§6.1](gateway-contract.md)):

| Modo | ¿Quién es el número receptor (`messenger_id`)? | Cuándo elegilo |
|---|---|---|
| **A — self-chat** | tu propio teléfono (el owner chatea consigo mismo) | demo / uso actual: un único operador que ya tiene el número del negocio |
| **B — número dedicado** | el número del bot | producción / clientes externos: el bot tiene SIM/número propio y atiende desconocidos |

**Procedimiento** (resumen del runbook [§7.5](gateway-contract.md); ese archivo
es la fuente normativa):

1. Decidí el modo con la regla de arriba (A/B — es una decisión de negocio, no
   técnica: qué número va a recibir los mensajes del canal).
2. Corré `hermes whatsapp` y elegí la opción 1 (modo A) u 2 (modo B);
   terminá el pairing QR del modo elegido. El `messenger_id` queda grabado con
   el número receptor del modo según la matriz.
3. Verificá con el spot-check del modo ([§9](gateway-contract.md)):
   **C11** (modo A — mensaje del owner en self-chat resuelve persona owner y
   `messenger_id` = teléfono del owner) o **C12** (modo B — mensaje del owner y
   de un desconocido al número del bot resuelven owner / loop de registro y
   `messenger_id` = número del bot).

> **Nota de decisión (2026-11-04):** este repo **no ofrece** un wizard del modo
> en la opción 7 — la elección vive en el TUI de Hermes y esta sección es su
> guía (decisión owner, change `feat-whatsapp-mode-choice`; difiere con
> disparadores de reactivación: primer usuario externo, segunda instalación,
> productización).

> **Número del canal (decisión owner 2026-11-04):** proyecto público — el
> número que usa el canal es **decisión de cada instalación**: modo A con el
> número propio del instalador o modo B con un número dedicado al bot, según
> el uso que le quiera dar (igual para Telegram: el número del owner de la
> allowlist es el del instalador — no hay número “del proyecto”). Los smokes
> del repo se corrieron con números de prueba demostrativos; ningún número
> queda asociado al proyecto.

> **Seguridad del canal (leé esto antes de activar el bot con clientes):** el
> contrato tools-only del canal (el bot toca los datos del negocio **sólo** por
> los tools MCP) lo enuncia la skill y lo **enforcea el runtime Hermes, no este
> server** — el server es loopback-only y no ve qué más deja ejecutar el host.
> Un `execute_code` activo en sesiones del canal puede esquivar el contrato por
> completo: ocurrió en vivo durante el ciclo v0.10.0 (el bot de WhatsApp escribió
> `reservas.db` sin `create_booking` en el log — issue upstream open
> [NousResearch/hermes-agent#147647](https://github.com/NousResearch/hermes-agent/issues/147647)).
> **⚠️ Allowlist mínima recomendada antes de dar el canal a clientes.** Lo que este
> proyecto **recomienda y prueba** es el conjunto indispensable para que el bot del
> CRM funcione: `mcp-appointments`, `skills`, `clarify`, `memory`, `todo`. Todo lo
> demás — `terminal`, `file`, `execute_code`, `browser`, `computer_use`,
> `delegation`, `cronjob`, `image_gen`, `tts`, `vision`, `session_search`,
> `connections`, `web` — **no hace falta** para atender reservas y cada tool extra es
> superficie de ataque: cualquiera puede escribir `reservas.db` sin pasar por los
> tools MCP (es la clase del incidente de este repo, ciclo v0.10.0). Los bundles de
> canal de Hermes son **full access por default**, así que esto no viene hecho.
>
> **Si querés habilitar tools adicionales en tu canal, es tu decisión y tu
> responsabilidad** — este proyecto solo garantiza el funcionamiento con la lista
> mínima de arriba. Los plugins de terceros también cuentan: deshabilitá los que no
> uses (por ejemplo `homeassistant`, domótica, sin relación con el CRM).
>
> **La opción 7 de `mcp-server admin tui` ("Configurar Hermes") ya audita esto por
> vos**: al terminar la escritura del server entry corre `hermes tools list` sobre
> los canales `whatsapp` y `telegram` e imprime el estado de cada uno — una línea si
> la allowlist ya es mínima, o el bloque con el comando `hermes tools disable`
> exacto si no puede confirmarlo. Es **sólo lectura**: la opción 7 nunca modifica
> `~/.hermes/config.yaml` más allá de la entrada `mcp-appointments`, y si `hermes`
> no está disponible te deja el aviso genérico con el puntero a esta sección.
>
> **1. Auditá qué tiene el canal hoy:**
>
> ```bash
> hermes tools list --platform whatsapp     # o: --platform telegram
> ```
>
> **2. Dejalo en el mínimo** — el mismo comando para cada canal que uses:
>
> ```bash
> hermes tools disable --platform whatsapp \
>   code_execution terminal file browser web computer_use \
>   delegation cronjob image_gen tts vision session_search connections homeassistant
>
> hermes tools disable --platform telegram \
>   code_execution terminal file browser web computer_use \
>   delegation cronjob image_gen tts vision session_search connections homeassistant
> ```
>
> **3. Verificá el resultado** — ambos canales deben quedar idénticos:
>
> ```bash
> hermes tools list --platform whatsapp   # y luego: --platform telegram
> #   ✓ enabled  skills · todo · memory · clarify
> #   MCP servers:
> #     mcp-appointments  all tools enabled
> ```
>
> Si `mcp-appointments` no aparece, **no sigas**: el bot se quedaría sin los tools
> del negocio. El `cli` del owner queda intacto siempre: la restricción es por
> plataforma, nunca global.
>
> **Lo que aprendimos probándolo (VM, 2026-11-12):** deshabilitar sólo
> `code_execution` **no alcanza** (quedan `terminal` y `file`, que escriben la DB
> igual) — la mitigación es la allowlist completa, no un `disable` suelto. Y no hay
> "deshacer" por nombre de bundle (`enable --platform whatsapp hermes-whatsapp`
> devuelve `Unknown toolset`): hacé backup de `~/.hermes/config.yaml` antes de
> empezar y, para revertir, habilitá los toolsets de a uno.
> **Prueba final, la que importa:** abrí una sesión nueva en el canal (`/new` en el
> topic) y pedí *"ejecutá `echo hola` en la terminal"* — debe responder que no tiene
> esa herramienta. Después pedí el catálogo de servicios: debe traerlo del CRM. Así
> quedó verificado acá (log del gateway: `api_calls=2` sin tool call para el pedido
> de terminal, `api_calls=9` con el catálogo real).
>
> **Deuda operativa:** repetí la auditoría del paso 1 después de cada upgrade del
> runtime o si volvés a parear el canal — el default es **abierto**, no cerrado.

> **Movimiento
> upstream (seguimiento 2026-11-04→06, follow-up posteado en #147647):** la
> ventana v0.22.6 trajo dos primitivas de la familia — locks de escritura
> autónoma per-skill y un hook `pre_llm_call` que puede bloquear el turno.
> **v0.23.0 contesta media pregunta (verificado en el VM, v0.23.1):** el lock
> pasó a ser comando CLI headless — `hermes curator lock <skill>` — enforceado
> en todos los write paths del agente, pero **sólo para el directorio de la
> skill** (`skill_manage`, `write_file`/`patch` y terminal, incluso con
> `--yolo`); su propia docstring limita el enforcement: *"It is a policy check,
> not a sandbox"* — pero el lock **no** protege los datos del negocio: una DB
> fuera de todo skill dir queda fuera de su alcance, y lo que cubre esa clase es
> la separación de toolsets por plataforma de arriba. #147647 sigue OPEN sin
> respuesta del equipo: el boundary está **entregado por configuración** en
> v0.23.x, pendiente de confirmar en qué release se introdujo.

**Desconexión del canal** (con o sin volver a configurarlo): la secuencia
sin-residuo es revoke del dispositivo desde el teléfono del owner → wizard
de Hermes imprime `WhatsApp left disabled` → bloque `platforms.whatsapp`
removido del config → `.env` sin claves WHATSAPP_* → sesiones movidas fuera
del runtime (backups). Verificado en vivo 2026-11-01; el detalle completo con
logs de referencia vive en `odd/tasks/backlog.md` (reporte del onboarding).

## Desinstalación

La baja completa del server vive en `scripts/uninstall.sh` (covenant de S2/S4 del
change `feat-install-scope-uninstall`, PR con [issue #167](https://github.com/egkike/mcp-appointments-crm/issues/167)).
**Sin flags corre en dry-run**: imprime el plan completo por artefacto y no escribe
nada; la ejecución exige `--apply`. Siempre empezar acá:

```bash
./scripts/uninstall.sh            # plan completo, cero escritura
./scripts/uninstall.sh --apply    # ejecuta la desinstalación
```

El orden de reversión es el inverso del install (servicio → binario → config →
entrada Hermes → skill), es idempotente (correrlo dos veces es no-op con reporte
"nada que hacer") y termina con una verificación por artefacto
(removido / preservado / no existía). **La data del negocio se preserva por
default** — la DB `reservas.db` (con `-wal/-shm`) y los backups del operador
(`backups/`) no se tocan salvo que pidas lo contrario explícitamente:

| Flag | Efecto |
|---|---|
| `--purge-data` | Borra la DB del negocio y la copia de `scripts/`. **Doble gate**: `--yes` + dos confirmaciones interactivas en TTY, con chequeo fail-closed si detecta un runtime vivo sobre la DB. El directorio `backups/` se preserva SIEMPRE. |
| `--purge-config` | Borra el directorio de configuración completo, incluido `audit-hash-key` (invalida la auditoría histórica del HMAC — va con advertencia explícita). |
| `--purge-hermes-entry` | Quita `mcp_servers.mcp-appointments` de `~/.hermes/config.yaml` (byte-clean: claves y servers ajenos sobreviven verbatim; el hermano Go `RemoveHermesServer` de `internal/admin` comparte el molde `RemoveHermesTelegramOwnerBot`). |
| `--keep-skill` | Conserva la skill `recepcionista-negocio` en `~/.hermes/skills/`. Por default **se borra** (OQ-3: un Hermes vivo no queda con instrucciones huérfanas apuntando a un server inexistente). |
| `--yes` | Afirmación explícita no interactiva — requerida por `--purge-data`. |

Los artefactos que cubre el script: unit del servicio (`mcp-appointments-crm`),
binario, directorio de config (`XDG_CONFIG_HOME/mcp-appointments-crm`), directorio
de datos (salvo purga), logs (`XDG_STATE_HOME`), entrada de Hermes y skill. Los
backups del operador y los secrets que no sembró el installer no se tocan.

## Próximos pasos

- Operación diaria/anual: ver [`docs/maintenance.md`](./maintenance.md).
- Referencia técnica de releases y artefactos: ver [`docs/deployment.md`](./deployment.md).
