# Guía de instalación de MCP Appointments CRM

> Esta guía lleva un VPS limpio (Ubuntu 22.04+ o macOS) hasta tener el servicio `mcp-appointments-crm` activo y respondiendo en `http://127.0.0.1:3000/mcp`.
>
> El flujo tiene dos partes: (1) configuración inicial interactiva con el wizard (`curl -fsSLO ... && bash install.sh`, requiere TTY) y (2) despliegue pinned del binario con `install.sh --version vX.Y.Z`. Si ya generaste los JSONs de setup en otra máquina, podés saltar directo al despliegue copiando los archivos al directorio de configuración del host destino.

---

## Prerrequisitos

Verificá estos ítems antes de empezar:

- [ ] VPS o máquina propia con Ubuntu 22.04+ o macOS 12+.
- [ ] Usuario normal con shell `bash` y acceso por SSH (o consola local).
- [ ] `curl`, `tar` y una herramienta de SHA256 (`sha256sum` en Linux, `shasum -a 256` en macOS) en el `PATH`.
- [ ] Conexión saliente HTTPS a `github.com` y `raw.githubusercontent.com`.
- [ ] Puerto `3000` libre en `127.0.0.1` (o el puerto que configures en `.env`).

> **No ejecutes la instalación como root.** Todo el pipeline es user-level: el binario, la DB, los JSONs de setup y el servicio quedan bajo el home del usuario que invoca `install.sh` (ADR-0002).

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

---

## Paso 2 — Despliegue pinned del binario

Una vez que los tres JSONs de setup existen, desplegá la versión que quieras. El flag `--version` del instalador **selecciona el tag del release** a instalar; es distinto de `mcp-server --version`, que reporta la versión del binario ya instalado.

### Opción A — One-line pipe (no interactiva)

```bash
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh \
  | bash -s -- --version v0.3.0
```

### Opción B — Descarga previa del script (recomendada para auditar)

```bash
curl -fsSLO https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh
bash install.sh --version v0.3.0
```

Reemplazá `v0.3.0` por el tag exacto que querés instalar. El formato obligatorio es `vMAJOR.MINOR.PATCH` (por ejemplo `v0.3.0`). No se acepta `latest` ni pre-releases desde el instalador.

### Qué hace el despliegue

1. Resuelve los paths XDG por OS (`DATA_DIR`, `BIN_DIR`, `LOG_DIR`, `ENV_FILE`).
2. Rechaza ejecución como root.
3. Valida que existan `curl`, `tar` y la herramienta SHA256.
4. Requiere los tres JSONs de setup; si falta alguno, aborta nombrándolo.
5. Detecta OS y arquitectura (`uname -s` / `uname -m`) y descarga el asset correcto:
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

Verificación opcional del wire MCP (handshake POST `initialize`, sin sesión —
el server es stateless y no exige `Mcp-Session-Id`):

```bash
curl --fail -sS http://127.0.0.1:3000/mcp \
  -H 'Content-Type: application/json' \
  -H 'X-Caller-Id: owner-demo' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"curl-smoke","version":"0.0.1"}}}'
```

Debe devolver una respuesta JSON-RPC `initialize` con las capacidades del
server. Si el server espera otra `protocolVersion`, la respuesta lo indica.

### 3.4 Versión del binario instalado

```bash
~/.local/bin/mcp-server --version
```

Resultado esperado: el tag que instalaste, por ejemplo:

```text
v0.3.0
```

> **Diferencia clave:** `install.sh --version vX.Y.Z` le dice al instalador qué release descargar; `mcp-server --version` le pregunta al binario instalado qué versión es. Ambos comandos coexisten.

### 3.5 Ubicación de la base de datos de producción

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
| Logs | `~/.local/state/mcp-appointments-crm/` |
| Service unit | `~/.config/systemd/user/mcp-appointments-crm.service` |

En macOS los paths de datos/logs/config usan `~/Library/Application Support/...` y `~/Library/Logs/...` según la tabla del PRD §3.5.

---

## Caveat importante: no corras `./mcp-server` a mano como servicio

El binario, cuando se ejecuta directamente sin el service unit, usa como default `./data/appointments.db` en el directorio de trabajo actual. Eso es **solo para desarrollo**.

El camino soportado en producción es el service unit, que inyecta:

```systemd
Environment=MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db
```

Si corré el binario a mano mientras el servicio está activo, terminás con **dos bases de datos distintas** (split-brain). Para operar el sistema siempre usá el servicio registrado.

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

### `install.sh --version` reporta "Faltan: setup_business.json, setup_staff.json, setup_services.json"

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

Editá `~/.config/mcp-appointments-crm/.env` para usar otro puerto, por ejemplo `MCP_PORT=3001`, y reiniciá:

```bash
systemctl --user restart mcp-appointments-crm
curl --fail http://127.0.0.1:3001/mcp
```

### Error de symlink en `DATA_DIR`, `BIN_DIR` o `LOG_DIR`

El instalador rechaza symlinks en esos directorios por seguridad. Si tenés un symlink intencional, eliminálo o cambiá el valor de `XDG_DATA_HOME`/`XDG_BIN_HOME`/`XDG_STATE_HOME` a una ruta real antes de correr el instalador.

---

## Próximos pasos

- Operación diaria/anual: ver [`docs/maintenance.md`](./maintenance.md).
- Referencia técnica de releases y artefactos: ver [`docs/deployment.md`](./deployment.md).
