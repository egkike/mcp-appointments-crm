# Fix: deuda del review del PR #293 (fallback verificado, chmod condicional, cobertura cp/mv)

Issue: [#294](https://github.com/egkike/mcp-appointments-crm/issues/294) (owner-approved, `status:approved` + `type:bug`, read-back confirmed 2026-11-27).

**Status**: implementación en curso. Origen: advisory findings no-bloqueantes del review nativo del PR #293 (lineage `review-80ed21f438861cd4`, lentes risk+reliability, registrados en `feat-uninstall-shipped-in-asset.md` L6).

## Specs

- **S1 (R1-dir-chmode/R3-perms)**: `install_backup_script()` e `install_uninstall_script()` aplican `chmod 0700` sobre `$DATA_DIR/scripts` **solo cuando el directorio se crea**; un reinstall nunca impone cambio de permisos sobre estado existente válido. Primera instalación: comportamiento observable sin cambios (dir 700).
- **S2 (R3-mvfail-coverage)**: tests cubren las branches cp-fail y mv-fail de `install_uninstall_script` (warn + cleanup + return 0, sin archivo parcial) y el re-seed idempotente (overwrite limpio sobre copia ya sembrada, sin temporales remanentes).
- **S3 (R1-curl-no-checksum)**: docs — el fallback documentado para máquinas sin el archivo pasa a ser **verificado** para tags que shippean el uninstaller en el asset (asset + `checksums.txt` + extract); caveat explícito para tags legacy (raw curl, TLS-only, sin checksum per-file posible). Nota `+x` (R3-docs-exec) alineada con el `chmod 0755` del installer.
- **Invariante**: la copia sembrada por `install.sh` ya queda verificada de fábrica (el installer valida el tarball completo contra `checksums.txt` antes de extraer) — S3 cierra solo el gap del path legacy de docs.

## Tasks

- T1 (S1) · inline code(shell) · chmod condicional a creación en ambos seeders · commit: _pendiente_
- T2 (S2) · inline tests(shell) · RED (chmod-preserve contra base) → GREEN: cp/mv-fail shims, re-seed, first-install chmod · commit: _pendiente_
- T3 (S3) · inline docs · README § Uninstall + installation.md § Desinstalación · commit: _pendiente_
- T4 · pipeline + gate (assess) + PR `Closes #294`

## Log

- **L1** (user, verbatim): «Dale con la deuda del review» — tras el cierre del PR #293 (merge `88f1f05`, CI main verde). Pre-aprobación de scope: los tres WARN + un SUGGESTION listados en el mensaje previo.
- **L2** (decisión técnica): S1 se aplica a **ambos** seeders (backup + uninstall) — el advisory nombraba el molde preexistente de backup.sh como el defecto real, no solo la copia nueva; consistencia de molde exige tocar los dos. Differential: los tests existentes de backup (dir 700 en primera instalación) deben seguir pasando sin cambios.
- **L3** (decisión técnica): las branches cp/mv-fail se testean con shims de PATH (patrón `nobin` de `uninstall_test.sh`), no con chmod de solo-lectura (evita que tearDown no pueda limpiar).
- **L4** (evidencia, 2026-11-27): T1 aplicado (`bash -n` OK). T2: 6 tests nuevos — RED contra base `88f1f05` exacto (`pre-existing dir perms preserved expected:<755> but was:<700>` en ambos seeders; las demás ASSERT del scratch son artefactos de `SCRIPT_DIR` en `/tmp`) → GREEN **65/65** en `install_deploy_test.sh` (cp-fail, mv-fail, re-seed y first-install-chmod cubiertos; los 59 preexistentes sin cambios). T3: `installation.md` § Desinstalación reestructura el fallback en dos caminos (asset+checksums verificado / legacy raw con caveat) + nota `+x` alineada; `README.md` § Uninstall espejo breve.
