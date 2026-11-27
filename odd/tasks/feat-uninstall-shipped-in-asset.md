# Feature: Ship uninstall.sh in the release asset + docs for release-only installs

Issue: [#292](https://github.com/egkike/mcp-appointments-crm/issues/292) (owner-approved, `status:approved` + `type:feature`, read-back confirmed 2026-11-25).

**Status**: implementación en curso (scope B aprobado por owner: docs + packaging).

## Specs

- **S1 (packaging)**: el asset del release (`.goreleaser.yaml` → `archives.files`) incluye `scripts/uninstall.sh` junto a `scripts/backup.sh`, para que toda instalación desde release tenga el uninstaller en la máquina.
- **S2 (installer)**: `install.sh` siembra `$DATA_DIR/scripts/uninstall.sh` con el mismo molde que `install_backup_script()` (source chain: `$EXTRACT_DIR/scripts/uninstall.sh` → `$SCRIPT_DIR/uninstall.sh`). **Best-effort**: si la fuente no existe (asset viejo pineado), advierte y NO aborta el install — la presencia del uninstaller es una mejora de docs, no un requisito de arranque del server.
- **S3 (docs)**: `docs/installation.md` § Desinstalación y `README.md` § Uninstall documentan (a) el path instalado por el installer en installs nuevos, y (b) el fallback `curl`-desde-tag para instalaciones ya existentes sin el archivo.
- **Invariante**: el comportamiento, flags y orden de reversión de `scripts/uninstall.sh` NO cambian (S2/S4 de `feat-install-scope-uninstall` quedan intactos).

## Tasks

- T1 (S1) · inline code(yaml) · .goreleaser.yaml `archives.files` += `scripts/uninstall.sh` + `goreleaser check` · commit: _pendiente_
- T2 (S2) · inline code(shell) · `install_uninstall_script()` en `scripts/install.sh` (best-effort, molde backup) + invocación junto a `install_backup_script` · commit: _pendiente_
- T3 (S2) · inline tests(shell) · RED→GREEN: install siembra uninstall.sh ejecutable; asset sin uninstall.sh no aborta (warn path) · commit: _pendiente_
- T4 (S3) · inline docs · `installation.md` + `README.md` · commit: _pendiente_
- T5 · gate: pipeline completo + routing del Verification & Review Protocol (assess) · PR `Closes #292`

## Log

- **L1** (user, verbatim): «Hola capo, estoy en la VM server y quiero ejecutar el uninstall pero no lo encuentra...puedes ver que pasa y decirme?» — Repro en VM server: install desde release v0.14.0 vía `curl | bash`; `./scripts/uninstall.sh` → `No such file or directory`; el asset no lo contiene; workaround: `curl -fsSL .../v0.14.0/scripts/uninstall.sh` + `bash uninstall.sh` (dry-run + `--apply` OK, `Estado final: OK — desinstalación consistente`).
- **L2** (user, verbatim): «Pero yo quise guiarme de la doc del proyecto para hacer esto pero nada... no dice nada de esto» — confirma el gap de docs: `installation.md` § Desinstalación y `README.md` § Uninstall asumen un clone del repo; el camino documentado de instalación es release `curl | bash` sin repo local.
- **L3** (owner, 2026-11-25): elige **opción B: docs + packaging** (vía selección explícita sobre el draft del issue). Issue #292 creado con labels al nacimiento (`status:approved` + `type:feature`, grant de este ciclo); host read-back verificado (author egkike, OPEN, labels OK) → clasificación `confirmed`.
- **L4** (decisión técnica): T2 best-effort (warn, no abort) en vez de hard-fail como backup.sh — un `install.sh` nuevo pineando un tag viejo no debe romper un install que antes funcionaba; la doc cubre el fallback curl para ese caso.
- **L5** (evidencia, 2026-11-25): diff +141/−5 en 6 archivos (un solo work unit: goreleaser + installer + tests + docs). RED verificado contra base (`127`, 3 asserts en rojo) → GREEN **59/59** en `install_deploy_test.sh`. Pipeline completo verde: suite shell **9/9** (`run_tests.sh`), `gofmt`/`go vet` limpios, `golangci-lint` 0 issues, `go build` OK, `go test -race ./...` OK. `goreleaser check` OK. Assess: riesgo **medium** (lane default), native unassessable por untracked sin declarar → gate native sobre el rango cometido.
