# Release: v0.15.0 (uninstaller en el asset + hardening del install)

Issues: [#296](https://github.com/egkike/mcp-appointments-crm/issues/296) (declaración, dentro del tag) + issue de re-punto de comandos vivos (post-tag, molde PR #291).

**Status**: en curso. Owner: «Saca release nuevo y asegurate de actualizar todos los docs con el nuevo release» (2026-11-27).

## Specs

- **S1 (declaración, issue #296)**: PRD changelog 1.50 + headers (PRD/README/installation) declaran v0.15.0 **antes** del tag — el commit de docs queda dentro del tag (molde PR #289).
- **S2 (tag + release)**: tag anotado `v0.15.0` → `release.yml` publica assets + `checksums.txt` → verificación: el tarball **incluye `scripts/uninstall.sh`** (el punto del release), `gh release view` OK.
- **S3 (comandos vivos, post-tag)**: todos los comandos de install/upgrade/verify de `README.md`, `docs/installation.md`, `docs/deployment.md`, `docs/demo-plan.md` apuntan a `v0.15.0` (molde PR #291).
- **Versión**: **minor** — `feat` desde v0.14.0 (issue #292); el changelog de GoReleaser agrupa Features.

## Tasks

- T1 (S1) · inline docs · PRD 1.50 + headers + README block · commit: _pendiente_
- T2 (S2) · release · tag v0.15.0 → release CI → verificación de assets · tag: _pendiente_
- T3 (S3) · inline docs · issue + PR re-punto de comandos vivos · commit: 2479972

## Log

- **L1** (user, verbatim): «Saca release nuevo y asegurate de actualizar todos los docs con el nuevo release». Issue #296 creado con labels al nacimiento (`status:approved` + `type:docs`, grant de este ciclo), read-back confirmado.
- **L2** (molde verificado contra v0.14.0): tag anotado apunta al commit del PR de declaración (25494b8 = PR #289); el re-punto de comandos vivos va post-tag (2149746 = PR #291). Rango al momento del tag: 4 commits desde v0.14.0 (2149746 + 88f1f05 + 6d99bc6 + el de este PR).
- **L3** (decisión de versión): v0.15.0 minor. `internal/buildinfo.Version` queda `dev` (ldflags inyecta desde el tag, ADR-0014).
- **L4** (S3, 2026-11-27): re-punto de comandos vivos → commit **`2479972`** (`docs(live): point every live install, upgrade and verify command at v0.15.0`, 4 docs, +28/−28; la entrada de T3 en este doc se registra en el commit inmediatamente siguiente porque un commit no puede contener su propio hash). Sobreviven a propósito 6 ocurrencias de `v0.14.0`, todas históricas o de release-anterior: README `Previous:` (cadena histórica), header de `installation.md` (`anterior:`), `installation.md` § Desinstalación «Tags legacy … p. ej. v0.14.0» (sigue siendo el último tag cuyo asset no trae `uninstall.sh`) y el bloque Rollback de `deployment.md` (release anterior, comentario alineado con sus comandos, molde PR #291). Los outputs esperados de versión (`{"status":"ok","version":"0.15.0"}` y el `--version` pelado) se movieron también: dejarlos en `0.14.0` contradecía el pin nuevo (mismo criterio que PR #291). Pipeline completo verde: `gofmt -l .` vacío, `go vet ./...`, `golangci-lint run ./...` 0 issues, `go build -o /dev/null ./...`, `go test -race ./...` OK.
