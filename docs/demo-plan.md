# Demo Plan — staging en HomeLab VM (Fase N)

> **Objetivo**: dejar `ubuntu-server-kike` como instancia demo permanente de
> `mcp-appointments-crm` y validar allí cada entrega (empezando por Fase 5).
> **Alcance**: operativo, no es SDD — no diseña ni implementa producto, solo
> define cómo probar lo ya mergeado en un entorno real.
> **Dueño**: Kike. **VM**: `Ubuntu-Server-Kike` (VirtualBox, Ubuntu 26.04.1).

## Precondiciones (verificadas 2026-09-06 vía SSH)

| # | Condición | Estado |
|---|-----------|--------|
| P1 | Acceso SSH `kike@100.95.242.72` (Tailscale) + fallback `192.168.100.192`, clave sin pass, sudo NOPASSWD | ✅ |
| P2 | `loginctl show-user kike -p Linger` → `Linger=yes` | ✅ |
| P3 | Puerto `3000` libre (`ss -tlnp`) | ✅ (re-chequear antes de cada corrida) |
| P4 | `sqlite3` CLI ≥ 3.46 (`sqlite3 --version`) | ✅ 3.46.1 |
| P5 | Espacio en host para snapshot (`df -h ~/VirtualBox\ VMs`) | ✅ 576 GB libres |
| P6 | Único user-service activo: `hermes-gateway.service` (Telegram) — convive, no colisiona | ✅ |

**No-objetivos**: instalar toolchain Go en la VM (se cross-compila en la laptop);
exponer el puerto 3000 fuera de loopback; usar datos reales (demo = datos
ficticios, siempre).

## Paso 0 — Snapshot `pre-demo-fase5`

```bash
# VM prendida: pausa de segundos, incluye RAM. VM apagada: solo disco, más liviano.
VBoxManage snapshot "Ubuntu-Server-Kike" take "pre-demo-fase5" \
  --description "Pre-demo mcp-appointments-crm Fase 5. Ubuntu 26.04.1 + Hermes gateway + hardening al 2026-09-06."
VBoxManage snapshot "Ubuntu-Server-Kike" list   # confirmar
```

Rollback (destructivo hacia adelante — tomar otro snapshot antes si la demo evolucionó):

```bash
VBoxManage snapshot "Ubuntu-Server-Kike" restore "pre-demo-fase5"
```

## Paso 1 — Build + primer release `v0.3.0`

> Nota: `validate_tag` solo acepta `vMAJOR.MINOR.PATCH` (sin `-rc`/ sufijos).
> Sin release publicado, `curl | bash` no tiene de dónde descargar: este paso
> lo desbloquea. GoReleaser completo (.goreleaser.yml) queda para Fase N;
> acá se publica a mano el asset mínimo.
>
> **Decisión 2026-09-06: release estable directo, sin pre-release.** No hay
> testers externos que justifiquen un rc (el único consumidor es esta demo);
> el código ya pasó sdd-verify + 3 receipts RDD; si aparece un bug se corta
> `v0.3.0+n` (patch) y se ejercita el upgrade path (REQ-INS-013). El soporte
> de tags `-rc.N`/`-beta.N` en `validate_tag` queda diferido a Fase N junto
> con el workflow GoReleaser.

En la laptop, desde `main` limpio:

```bash
export TAG=v0.3.0
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
  -ldflags "-X github.com/egkike/mcp-appointments-crm/internal/buildinfo.Version=$TAG" \
  -o /tmp/demo-release/mcp-server ./cmd/mcp-server
./tmp/demo-release/mcp-server --version   # debe imprimir v0.3.0 (REQ-BVER)
/tmp/demo-release/mcp-server --version || true
cd /tmp/demo-release
tar -czf mcp-appointments-crm_Linux_x86_64.tar.gz mcp-server
sha256sum mcp-appointments-crm_Linux_x86_64.tar.gz > checksums.txt
cat checksums.txt
```

Publicar (el contrato que `install.sh` espera es
`.../releases/download/{tag}/{asset}` + `checksums.txt`):

```bash
gh release create v0.3.0 --title "v0.3.0 (demo)" --notes "Primer release demo Fase 5 (manual; GoReleaser pendiente Fase N)." \
  /tmp/demo-release/mcp-appointments-crm_Linux_x86_64.tar.gz \
  /tmp/demo-release/checksums.txt
```

Criterio: `gh release view v0.3.0` lista los 2 archivos.

## Paso 2 — Setup interactivo (negocio demo ficticio)

En la VM, terminal real (el flujo Fase 4 exige TTY):

```bash
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh -o /tmp/install.sh
bash /tmp/install.sh
```

Datos demo sugeridos (ficticios, nunca reales): negocio `Peluquería Demo`,
`AR`, `ARS`, `America/Argentina/Buenos_Aires`, 1 profesional + 1 servicio
mínimo. Resultado esperado: `~/.config/mcp-appointments-crm/setup/` con los
3 JSONs y sin checkpoint residual.

## Paso 3 — Deploy real con `curl | bash` (cronometrar, DoD < 5 min)

```bash
time curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh \
  | bash -s -- --version v0.3.0
```

Criterio: exit 0 y tiempo total < 5 minutos (DoD 1).

## Paso 4 — Matriz DoD (una fila por check)

| DoD | Comando (en la VM) | Esperado |
|-----|--------------------|----------|
| 5 | `systemctl --user is-active mcp-appointments-crm` | `active` |
| 6 | `loginctl show-user kike -p Linger` | `Linger=yes` |
| 1 | `curl --fail http://127.0.0.1:3000/mcp` | HTTP 200 / respuesta JSON-RPC |
| 9 | `~/.local/bin/mcp-server --version` | `v0.3.0` |
| 5b | `sudo reboot` → esperar → `systemctl --user is-active mcp-appointments-crm` | `active` (sobrevive reboot) |
| 7 | `bash ~/.local/share/mcp-appointments-crm/scripts/backup.sh ~/.local/share/mcp-appointments-crm/reservas.db` → `gunzip -c backups/reservas-*.db.gz > /tmp/r.db` → `sqlite3 /tmp/r.db "PRAGMA integrity_check;"` | `ok` |
| 4 | `bash install.sh` sin JSONs (mover `setup/` temporalmente) | exit ≠ 0 nombrando los faltantes |

Registrar cada output en la bitácora (§ Bitácora).

## Paso 5 — Seed del owner + wire de Hermes

El TUI admin (Fase 2+) no existe aún: el seed se hace por SQL directo
(workaround documentado solo para demo):

```bash
DB=~/.local/share/mcp-appointments-crm/reservas.db
sqlite3 "$DB" "INSERT INTO accounts (id, role, display_name, is_active) VALUES ('owner-demo', 'owner', 'Owner Demo', 1);"
sqlite3 "$DB" "SELECT id, role, is_active FROM accounts;"
```

Luego configurar Hermes (en la VM) con el endpoint MCP
`http://127.0.0.1:3000/mcp` y header `X-Caller-Id: owner-demo` según su
documentación de MCP clients. Criterio: una llamada de prueba responde sin
403 (403 = revisar `accounts` + header).

## Paso 6 — Smoke tests por chat (vía Hermes)

Secuencia mínima sugerida, con negocio demo:

1. `get_business_profile` → muestra "Peluquería Demo".
2. `check_availability` para mañana → slots coherentes con el horario cargado.
3. `create_booking` → reserva creada; `get_booking` → la devuelve.
4. `get_pending_alerts` → hay alerta por la reserva; `mark_alert_as_sent` → la consume.
5. `reschedule_booking` → mueve la reserva; `cancel_booking` → la cancela.
6. `search_clients_advanced` + `search_services_advanced` → FTS5 responde.
7. `get_loyalty_report` → reporte con datos agregados.

Criterio: las 7 familias de tools responden con mensajes semánticos en
español y sin stack traces. Anotar desvíos en la bitácora.

## Paso 7 — Schedular el backup (valida `maintenance.md`)

La doc deja el scheduling como decisión del operador: acá el operador decide
activarlo (systemd user timer o cron, ver `docs/maintenance.md` §6).
Verificar al día siguiente que apareció `backups/reservas-YYYYMMDD.db.gz`.

## Bitácora de corridas

| Fecha | Versión | Paso(s) | Resultado | Notas |
|-------|---------|---------|-----------|-------|
| 2026-09-06 | — | P1–P6 | ✅ precondiciones verificadas vía SSH | Base para la primera corrida demo |
| 2026-09-06 | — | decisión release | v0.3.0 estable directo, sin rc (ver nota Paso 1) | Habilita Paso 1 |
| | | | | |

## Riesgos y notas

- El reboot interrumpe el bot de Telegram unos minutos (gateway vuelve solo por linger). Avisar antes si alguien lo usa.
- `fail2ban` + UFW no ven nada anómalo: todo es loopback y usuario local.
- Si una corrida deja la demo en estado raro: snapshot intermedio (`demo-v1-funcionando`) antes de seguir experimentando; `pre-demo-fase5` siempre intacto como punto cero.
- Cuando exista GoReleaser (Fase N), repetir el Paso 3 contra el release generado por CI para validar ese camino también.
