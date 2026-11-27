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
- T3 (S3) · inline docs · issue + PR re-punto de comandos vivos · commit: _pendiente_

## Log

- **L1** (user, verbatim): «Saca release nuevo y asegurate de actualizar todos los docs con el nuevo release». Issue #296 creado con labels al nacimiento (`status:approved` + `type:docs`, grant de este ciclo), read-back confirmado.
- **L2** (molde verificado contra v0.14.0): tag anotado apunta al commit del PR de declaración (25494b8 = PR #289); el re-punto de comandos vivos va post-tag (2149746 = PR #291). Rango al momento del tag: 4 commits desde v0.14.0 (2149746 + 88f1f05 + 6d99bc6 + el de este PR).
- **L3** (decisión de versión): v0.15.0 minor. `internal/buildinfo.Version` queda `dev` (ldflags inyecta desde el tag, ADR-0014).
