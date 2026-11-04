# Design: feat-install-scope-uninstall

## D1 — Covenant de plataformas (S1)

| Plataforma | Estado después del change | Nota |
|---|---|---|
| Linux x86_64/arm64 | soportado nativo (installer + service systemd --user) | sin cambios |
| macOS (Darwin x86_64/arm64) | soportado nativo (installer + launchd plist) | sin cambios |
| Windows | soportado **sólo vía WSL2 con systemd** (`wsl --install`, `/etc/wsl.conf` con `[boot] systemd=true`) | el usuario corre el one-liner docs dentro de WSL; `uname` = Linux y el installer entra por el path Linux estándar; prerequisito systemd documentado |
| Windows nativo | **no soportado** (el asset `.exe` se puede correr a mano para pruebas, sin service/config sembrados por el installer) | instalador nativo queda explícitamente fuera de alcance |

**Decisión OQ-1 (owner 2026-11-01): RETIRAR el asset Windows de la matriz
GoReleaser.** Para correr el binario en Windows queda sólo WSL (asset Linux)
o un build manual (`go build`) para pruebas puntuales; el covenant queda:
Windows soportado vía WSL2 únicamente.

## D2 — Contrato de `uninstall.sh` (S2)

- **Modo default: `--dry-run` explícito o implícito?** PRD pedía
  "dry-run por defecto". Mantener: **sin flags → dry-run** que imprime el
  plan completo (por plataforma: qué unit se va a vaciar, qué archivos);
  la ejecución exige `--apply`. Nada toca hasta `--apply`.
- **Orden de reversión** (el inverso del install, con dependencias):
  1. `stop` del service (systemd --user o launchd) — nunca borra un
     live-open DB (se respeta la regla "dos procesos sobre la misma DB
     compiten").
  2. disable + remove del service unit (systemd: `~/.config/systemd/user/
     mcp-appointments-crm.service` + `daemon-reload`; launchd: unload
     plist + remove) y `linger` queda **intacto** (lo podría usar otro
     user service del operador; se imprime como nota).
  3. remoción del binario `~/.local/bin/mcp-server`.
  4. logs dir del remove.
  5. config dir: remoción de `templates/pristine` + copia de scripts…
     **preservado por defecto**: `.env`, `caller-id`, `audit-hash-key`
     (ADR-0021 — keyfile cripto; se borra SOLO con `--purge-config`,
     antes de borrarlo imprimir que borrarlo disvalida la auditoría
     histórica si el DB se purga) y setup/. El cleanup del `.env` queda
     mantenido salvo `--purge-config` (reversible: un tar todo el dir).
  6. data dir: tocado solamente con `--purge-data` (S4). Verificación
     de "no live handle": el service ya está stop; si la DB diguere
     `-wal/-shm` andando se avisa y falla cerrado (no se borra por
     debajo de un runtime vivo en otro host/cwd).
  7. Verificación final de ausencia (idempotente, lista de los que
     quedaron con el motivo — data preservada no es restante).
- **Idempotente por construcción**: correr el uninstall dos veces es
  no-op; reporta "nada que hacer".

## D3 — Desacople del gateway Hermes (S3)

- Operación con precedent byte-clean: agregar `RemoveHermesServer` que
  borra la entrada `mcp_servers.mcp-appointments` completa (como
  `RemoveHermesTelegramOwnerBot` pero por servidor entero; golden del mismo
  shape: keys ajenas sobreviven, re-marshale byte-idéntico si ya no existe).
- En uninstall.sh: NO va en el default; es un flag `--purge-hermes-entry`
  (misma regla: reversible y reversible el config tiene backups del
  runtime). Con el flag se elimina la entrada y se avisa.
  **Skill — OQ-3 (owner 2026-11-01, prevalece sobre el postulado inicial que
  esta sección dejaba escrito):** la skill `recepcionista-negocio` se borra
  **por default** (un Hermes vivo no queda con instrucciones huérfanas
  apuntando a un server inexistente); `--keep-skill` la conserva.
- Ambos flags corren *ANTES* de borrar el binario (para evitar que el
  runtime Hermes siga llamando a un server ya borrado: el config se retira
  primero, el binario después).

## D4 — Puerta destructiva (S4)

- Doble confirmation para `--purge-data` (misma semántica del
  purge-inactive del TUI): print del exacto DB path + tamaño + estado-backup
  (si no hay backup reciente en `backups/` aviso con el comando del
  backup previsto por el script), y una segunda
  confirmación «ÚLTIMA CONFIRMACIÓN» más el flag `--yes` explícito, con la
  advertencia literal de que no hay undo.
- `audit-hash-key`: se borra SOLO si `--purge-config` y además
  se CIFRA el aviso (borrarla aislada disvalida la auditoría
  histórica del HMAC).

## D5 — Implementación futura (para el plan): fase de código

- **T1** `install.sh`: disclaimer por plataforma + conditional WSL hint
  (shell puro; docs lane también en `installation.md`).
- **T2** `scripts/uninstall.sh` con `--dry-run` default: ~200-300 líneas
  shell + tests del shell (`scripts/tests/uninstall_*test.sh`, un golden
  del dry-run por plataforma con t.TempDir sandbox `HOME`).
- **T3** `RemoveHermesServer` en `internal/admin/hermes.go` (Go: ~80
  líneas + golden tests idempotence/byte-clean, el molde de S11) EXPOSED
  VIA TUI/consolos n/a — el uninstall es script (no Go service lookup)
  el Go function es la pieza reusable para TUI futuro.
- **T4** docs: `docs/installation.md` (sección de plataforma + sección
  Uninstall nueva al final), README (matriz de plataformas + hint WSL),
  PRD changelog.

## Riesgos

- **Riesgo 1 (mayor: data)** — un código malo borra el DB del negocio:
  mitigado con default keep-data, `--apply` explícito para ejecutar, doble
  gate + `--yes` para `--purge-data`, backup check en el dry-run.
- **Riesgo 2** — remover el config de Hermes runtime-owned es un
  write point nuevo sobre el archivo del runtime: usar la misma garantía
  del precedent (atomic replace + golden).
- **Riesgo 3** — WSL depende del feature systemd del host Windows:
  documentar la versión mínima (WSL2 + systemd enabled), no tocar el
  installer (uname di Linux ahí de todos modos).

## Open questions del owner (OQ)

| # | Pregunta | Bloquea |
|---|---|---|
| OQ-1 | Asset zip Windows: ¿se mantiene publicado (con nota "WSL usa el asset Linux") o se retira de la matrix GoReleaser? | T1/T4 docs; nada de código |
| OQ-2 | ¿El uninstall vive como script `scripts/uninstall.sh` o como sub-comando `mcp-server uninstall` (Go)? PRD propuso script; Go sue lé no si el server NO está instalado (el script consulta métodos del service directamente sin binario). Abogamos mantener script | T2 (opción) |
| OQ-3 | Después del uninstall, ¿queda contenido del dir `~/.hermes/skills/`? (ours no: `--purge-skill` default NO) | T3 flags |
| OQ-4 | `audit-hash-key`: ¿dejarla en el purge-config con warn, o siempre preservarla? | T2 flags |
