# Guía de mantenimiento de MCP Appointments CRM

> Manual de operación anual para una instalación existente de MCP Appointments CRM. Cubre backups, restores, upgrades, inspección de logs, control del servicio y troubleshooting en Linux y macOS.
>
> Esta guía asume que el sistema fue instalado con el pipeline de `install.sh --version vX.Y.Z` y que la base de datos de producción es `reservas.db` bajo el `DATA_DIR` del usuario.

---

## 1. Backups y restauración

### 1.1 Ejecutar un backup manual

El script `scripts/backup.sh` produce un backup consistente de la DB aunque el servicio esté corriendo, usando `sqlite3 .backup` y comprimiendo con `gzip`.

```bash
bash ~/.local/share/mcp-appointments-crm/scripts/backup.sh \
  ~/.local/share/mcp-appointments-crm/reservas.db
```

Salida esperada:

```text
Backup: /home/tu-usuario/.local/share/mcp-appointments-crm/backups/reservas-YYYYMMDD.db.gz (N bytes)
```

Re-ejecutar el mismo día sobrescribe el archivo del día con el snapshot más reciente.

> **Nota:** el pipeline de `install.sh` muestra al final una línea `backup.sh` copy-pasteable con el `MCP_DB_PATH` exacto de tu instalación. Usá esa línea si no estás seguro de la ruta.

### 1.2 Restaurar un backup

1. Detené el servicio para evitar escrituras concurrentes.
2. Descomprimí el backup sobre una ruta nueva o sobre la DB existente (sobrescribe).
3. Verificá integridad.
4. Arrancá el servicio.

```bash
# Linux
systemctl --user stop mcp-appointments-crm

# Restaurar a una ruta nueva (recomendado para validar)
gunzip -c ~/.local/share/mcp-appointments-crm/backups/reservas-20260827.db.gz \
  > /tmp/reservas-restaurada.db

# Integrity check
sqlite3 /tmp/reservas-restaurada.db "PRAGMA integrity_check;"
```

Resultado esperado:

```text
ok
```

Verificá que los datos estén presentes:

```bash
sqlite3 /tmp/reservas-restaurada.db \
  "SELECT count(*) FROM bookings;"
```

Si la verificación es correcta, mové el archivo restaurado al path de producción:

```bash
mv /tmp/reservas-restaurada.db ~/.local/share/mcp-appointments-crm/reservas.db
systemctl --user start mcp-appointments-crm
systemctl --user is-active mcp-appointments-crm
```

> **ADVERTENCIA:** no restaures sobre `reservas.db` mientras el servicio esté activo. Siempre detenelo primero.

---

## 2. Upgrade (y downgrade) de versión

### 2.1 Upgrade a un tag más nuevo

El pipeline de despliegue es idempotente: re-ejecutalo con el tag nuevo.

```bash
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh \
  | bash -s -- --version v0.3.1
```

Lo que se preserva automáticamente:

- `~/.config/mcp-appointments-crm/.env` (incluyendo puerto personalizado).
- Los tres JSONs de setup en `~/.config/mcp-appointments-crm/setup/`.
- `~/.local/share/mcp-appointments-crm/reservas.db` y sus backups.

Lo que se reemplaza:

- El binario en `~/.local/bin/mcp-server` (atómicamente).
- La service unit renderizada.

Verificación post-upgrade:

```bash
~/.local/bin/mcp-server --version   # v0.3.1
systemctl --user is-active mcp-appointments-crm
curl --fail http://127.0.0.1:3000/mcp
```

### 2.2 Downgrade

Reinstalá el tag anterior con el mismo comando. Los datos y la configuración no se tocan:

```bash
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh \
  | bash -s -- --version v0.3.0
```

---

## 3. Inspección de logs

### 3.1 Linux — journalctl

```bash
# Últimas 50 líneas
journalctl --user -u mcp-appointments-crm -n 50 --no-pager

# Seguir en vivo
journalctl --user -u mcp-appointments-crm -f

# Desde el último boot
journalctl --user -u mcp-appointments-crm --since today
```

Si sobre SSH no funciona `systemctl --user`, fijate la sección [Troubleshooting](#troubleshooting).

### 3.2 macOS — archivos de log

El LaunchAgent escribe a:

```text
~/Library/Logs/MCP Appointments CRM/mcp-server.out.log
~/Library/Logs/MCP Appointments CRM/mcp-server.err.log
```

```bash
tail -f ~/Library/Logs/MCP\ Appointments\ CRM/mcp-server.err.log
```

### 3.3 Linux — archivo de estado (opcional)

Si existe, también podés consultar:

```bash
tail -f ~/.local/state/mcp-appointments-crm/mcp-server.log
```

---

## 4. Control del servicio por sistema operativo

### 4.1 Linux — systemd user

```bash
# Estado
systemctl --user is-active mcp-appointments-crm
systemctl --user status mcp-appointments-crm --no-pager

# Iniciar / detener / reiniciar
systemctl --user start mcp-appointments-crm
systemctl --user stop mcp-appointments-crm
systemctl --user restart mcp-appointments-crm

# Habilitar/deshabilitar arranque automático
systemctl --user enable mcp-appointments-crm
systemctl --user disable mcp-appointments-crm

# Recargar configuración luego de editar la unit
systemctl --user daemon-reload
systemctl --user restart mcp-appointments-crm
```

### 4.2 macOS — launchctl

```bash
# Estado / listado
launchctl list | grep com.mcp.appointments
launchctl print gui/$UID/com.mcp.appointments.server

# Detener (idempotente)
launchctl bootout gui/$UID/com.mcp.appointments.server 2>/dev/null || true

# Iniciar
launchctl bootstrap gui/$UID \
  ~/Library/LaunchAgents/com.mcp.appointments.server.plist
```

---

## 5. Caveat: nunca corras el binario a mano junto al servicio

El binario `mcp-server`, cuando se ejecuta directamente sin el service unit, usa como default `./data/appointments.db` en el directorio de trabajo actual. Ese path es **solo para desarrollo**.

El servicio inyecta:

```systemd
Environment=MCP_DB_PATH=%h/.local/share/mcp-appointments-crm/reservas.db
```

Si ejecutás `./mcp-server` o `~/.local/bin/mcp-server` a mano mientras el servicio está activo, el segundo proceso abrirá una DB distinta (`./data/appointments.db`) y tendrás datos divergentes. El camino soportado es:

- Siempre operar a través del servicio (`systemctl --user` o `launchctl`).
- Si necesitás correr el binario manualmente para debug, detené primero el servicio y exportá `MCP_DB_PATH`:

```bash
systemctl --user stop mcp-appointments-crm
MCP_DB_PATH="$HOME/.local/share/mcp-appointments-crm/reservas.db" \
  ~/.local/bin/mcp-server
```

---

## 6. Scheduling automático de backups (opcional del cliente)

`backup.sh` no registra ningún scheduler por sí mismo (ADR-0005). Si querés backups automáticos, configurá una tarea periódica a tu criterio.

### Opción A — cron diario

```bash
# Editar crontab del usuario
crontab -e
```

Agregá una línea similar:

```cron
0 2 * * * bash /home/tu-usuario/.local/share/mcp-appointments-crm/scripts/backup.sh /home/tu-usuario/.local/share/mcp-appointments-crm/reservas.db >> /home/tu-usuario/.local/state/mcp-appointments-crm/backup.log 2>&1
```

Ajustá el path de `backup.sh` y de `reservas.db` a tu usuario real.

### Opción B — systemd timer (Linux)

Creá un timer user-level, por ejemplo `~/.config/systemd/user/mcp-appointments-crm-backup.timer`:

```ini
[Unit]
Description=Daily backup for MCP Appointments CRM

[Timer]
OnCalendar=daily
Persistent=true

[Install]
WantedBy=timers.target
```

Y su service asociado `~/.config/systemd/user/mcp-appointments-crm-backup.service`:

```ini
[Unit]
Description=Backup MCP Appointments CRM DB

[Service]
Type=oneshot
ExecStart=/bin/bash /home/tu-usuario/.local/share/mcp-appointments-crm/scripts/backup.sh /home/tu-usuario/.local/share/mcp-appointments-crm/reservas.db
```

Habilitalo:

```bash
systemctl --user daemon-reload
systemctl --user enable --now mcp-appointments-crm-backup.timer
systemctl --user list-timers mcp-appointments-crm-backup.timer
```

> **Esta automatización es decisión tuya como operador; no es una función soportada del producto.**

---

## 7. Troubleshooting

### `systemctl --user` falla con "Failed to connect to bus" sobre SSH

El bus de usuario de systemd no siempre está disponible en sesiones SSH mínimas. Soluciones:

```bash
# Opción 1: exportar XDG_RUNTIME_DIR
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/run/user/$(id - u)}"
systemctl --user is-active mcp-appointments-crm

# Opción 2: usar machinectl shell si está disponible
machinectl shell "$USER"@
systemctl --user is-active mcp-appointments-crm
```

### El servicio se detiene después de logout (Linux)

`loginctl enable-linger` no quedó habilitado. Reinstalalo:

```bash
loginctl enable-linger "$USER"
loginctl show-user "$USER" -p Linger  # Linger=yes
systemctl --user enable --now mcp-appointments-crm
```

### Puerto ocupado

```bash
ss -tlnp | grep 3000
# o
lsof -i :3000
```

Cambiale el puerto en `~/.config/mcp-appointments-crm/.env` y reiniciá:

```bash
echo "MCP_PORT=3001" >> ~/.config/mcp-appointments-crm/.env
systemctl --user restart mcp-appointments-crm
curl --fail http://127.0.0.1:3001/mcp
```

### SHA256 mismatch persistente en upgrade

No bypasses la verificación. Si un upgrade específico falla con SHA256:

```bash
curl -fsSLO https://github.com/egkike/mcp-appointments-crm/releases/download/v0.3.1/checksums.txt
curl -fsSLO https://github.com/egkike/mcp-appointments-crm/releases/download/v0.3.1/mcp-appointments-crm_Linux_x86_64.tar.gz
sha256sum -c checksums.txt --ignore-missing
```

Si el checksum oficial falla, no instales esa versión y abrí un issue.

### No hay backups nuevos

Verificá que el script exista y sea ejecutable:

```bash
ls -l ~/.local/share/mcp-appointments-crm/scripts/backup.sh
bash ~/.local/share/mcp-appointments-crm/scripts/backup.sh \
  ~/.local/share/mcp-appointments-crm/reservas.db
```

Si falta `sqlite3` o `gzip`, el script aborta nombrando la herramienta faltante.

### Verificar integridad de la DB activa sin detener el servicio

Podés correr un integrity check online leyendo el archivo activo (SQLite tolera lectura concurrente), aunque el backup es el método recomendado:

```bash
sqlite3 ~/.local/share/mcp-appointments-crm/reservas.db "PRAGMA integrity_check;"
```

---

## Referencias rápidas

| Acción | Linux | macOS |
|---|---|---|
| Estado del servicio | `systemctl --user is-active mcp-appointments-crm` | `launchctl list \| grep com.mcp.appointments` |
| Logs | `journalctl --user -u mcp-appointments-crm -f` | `tail -f ~/Library/Logs/MCP\ Appointments\ CRM/mcp-server.err.log` |
| Backup | `bash ~/.local/share/.../scripts/backup.sh ~/.local/share/.../reservas.db` | mismo comando |
| Upgrade | `curl ... \| bash -s -- --version vX.Y.Z` | mismo comando |
| DB de producción | `~/.local/share/mcp-appointments-crm/reservas.db` | `~/Library/Application Support/MCP Appointments CRM/reservas.db` |
