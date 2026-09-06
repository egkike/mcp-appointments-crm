# Registro manual del servicio en Windows (NSSM)

> El instalador `install.sh` **no automatiza Windows en la Fase 5**. Este documento describe el registro manual con [NSSM](https://nssm.cc/) para quien necesite correr el servidor en ese sistema operativo.

## Layout recomendado

- Configuración y JSONs de setup: `%APPDATA%\MCP Appointments CRM\`
- Archivo `.env`: `%APPDATA%\MCP Appointments CRM\.env`
- Base de datos: `%APPDATA%\MCP Appointments CRM\reservas.db`
- Binario: `%LOCALAPPDATA%\Programs\mcp-server.exe`
- Logs (opcional): `%LOCALAPPDATA%\MCP Appointments CRM\Logs\`

> El binario lee `MCP_BIND` y `MCP_PORT` desde el `.env` (precedencia `env vars > .env > defaults`).

## Ejemplo de registro con NSSM

Desde una terminal con privilegios administrativos (requerido por NSSM para crear el servicio):

```powershell
nssm install MCPAppointmentsCRM "C:\Users\<usuario>\AppData\Local\Programs\mcp-server.exe"
nssm set MCPAppointmentsCRM AppEnvironmentExtra "MCP_DB_PATH=C:\Users\<usuario>\AppData\Roaming\MCP Appointments CRM\reservas.db"
nssm set MCPAppointmentsCRM AppStdout "C:\Users\<usuario>\AppData\Local\MCP Appointments CRM\Logs\mcp-server.out.log"
nssm set MCPAppointmentsCRM AppStderr "C:\Users\<usuario>\AppData\Local\MCP Appointments CRM\Logs\mcp-server.err.log"
nssm start MCPAppointmentsCRM
```

Reemplazá `<usuario>` por el nombre de usuario real y ajustá las rutas si usaste ubicaciones diferentes.

## Notas

- El servicio queda registrado a nivel del sistema porque NSSM requiere elevación; la ejecución del binario puede correr bajo una cuenta de usuario específica configurada en `nssm edit`.
- Backup, upgrade y troubleshooting en Windows no están automatizados en esta fase; seguí el procedimiento equivalente descrito en `docs/maintenance.md` adaptando las rutas a `%APPDATA%`.
