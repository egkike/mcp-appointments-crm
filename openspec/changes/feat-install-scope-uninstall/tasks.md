# Tasks: feat-install-scope-uninstall

> **Change:** feat-install-scope-uninstall
> **Status:** doc-first landed; el código (T1-T4) es entrega posterior y NO
> arranca sin las decisiones OQ del owner.
> **Inputs:** `proposal.md`, `design.md`,
> `specs/install-scope-uninstall/spec.md`, `odd/tasks/feat-install-scope-uninstall.md`

## Review Workload Forecast

| Field | Value |
|-------|-------|
| Estimated (docs de este change) | ~250 (prop 90 + design ~115 + spec ~75 + tasks ~40) |
| Estimated (código futuro) | T2 shell ~250-350 + tests ~200; T3 Go ~120-180 con darkens |
| 400-line budget risk (código) | Medium — T2 (uninstall.sh + tests) probable chained con T3 |
| Chained PRs recommended | Sí si T2+T3 viajan juntos: PR-1 docs+installer scope, PR-2 uninstall.sh+tests, PR-3 RemoveHermesServer Go |

## OQ del owner (RESUELTAS 2026-11-01)

- [x] **OQ-1 — RETIRAR** el zip de Windows de la matriz GoReleaser (covenant Windows = WSL con el asset Linux; `install.sh` hoy no tenía path Windows: la única superficie es la matriz de `.goreleaser.yaml`)
- [x] **OQ-2 — script shell** `scripts/uninstall.sh` (corre aunque el binario no exista)
- [x] **OQ-3 — default PURGA**: el uninstall borra `~/.hermes/skills/recepcionista-negocio/` por default (un Hermes vivo no queda con instrucciones huérfanas apuntando a un server inexistente); flag `--keep-skill` para conservarla — rotación semántica frente al postulado (era default-keep, el owner eligió default-purga)
- [x] **OQ-4 — purgable con warn**: `audit-hash-key` se borra con `--purge-config` + advertencia explícita sobre la auditoría histórica del HMAC

## Phase 2 — entrega (después de OQ)

- [ ] **T1 (S1)** — `install.sh`: mensajes por plataforma + hint WSL en el
  branch de plataformas no soportadas (hoy el error genérico); docs
  `installation.md` nueva sección de plataformas (matriz) + README + hint en Prerrequisitos
  1 de Prerequisitos
- [ ] **T2 (S2/S4)** — `scripts/uninstall.sh` con el contrato D2/D4 completo
  (dry-run default / --apply / --purge-data doble gate / verificación final
  tabla) + shell tests: `scripts/tests/uninstall_test.sh` (doble run idempotente,
  keep-data default, live-DB fail-closed, dry-run no-op byte-evidence) + golden
  del dry-run con HOME sandbox
- [ ] **T3 (S3)** — `RemoveHermesServer` en `internal/admin/hermes.go` (molde
  RemoveHermesTelegramOwnerBot: golden byte-clean, idempotente, entradas
  hermanas sobreviven) + tests admin + el modo de uso del script: decisión de apply — o el script
  reescribe el config yaml por sí mismo (misma garantía atomic replace del
  runtime), o se apoya en el binario si está en PATH; postulado: script
  independiente + el mismo golden test aplicado al pasaje del script
- [ ] **T4** — docs del uninstall (installation.md sección nueva + README
  repos_github) y PRD changelog del release que lo traiga
- [ ] **T5 (gates)** — pipeline completo por cara Go (si T3 Go), shellcheck +
  tests de shell por T2, native review por routing (uninstall es
  sensible: riesgo de data), GGA por unit

## Coverage check

| Spec requirement | Tasks |
|---|---|
| Plataformas declaradas (Linux/macOS/WSL) | T1 |
| dry-run default + --apply + idempotente | T2 |
| keep-data default + purge explícito con doble gate | T2 |
| Herm desacoplado con flags byte-clean | T2+T3 |
| verificación de ausencia final | T2 |
