# ADR-0003: Portable backup.sh, no auto-configured scheduler

- **Status**: accepted
- **Date**: 2026-06-25
- **Authors**: Kike

## Context

Backups are essential for a business system — losing the client database is
catastrophic. The original PRD had `install.sh` configure a daily cron job to
run a backup script. The script itself was not specified.

This created three problems:
1. **OS-specific logic in `install.sh`**: cron on Linux, launchd on macOS,
   Task Scheduler on Windows. Three different code paths, three sets of tests.
2. **"Minimal dependencies" violation**: cron is a system service the
   `install.sh` mutates. This conflicts with the project's principle.
3. **Inflexibility**: every operator is forced into the same scheduling
   strategy (cron, daily at 03:00). Some operators prefer systemd timers,
   others use the VPS provider's snapshot service, others rely on Time
   Machine or Windows Backup. The right answer is "operator's choice".

## Decision

The backup mechanism is split into two parts:
1. **`scripts/backup.sh`** (portable bash): uses `sqlite3 .backup` for
   consistency (respects WAL), gzips the output, and writes to
   `<data_dir>/backups/reservas-YYYYMMDD.db.gz`. The script is the same on
   all platforms.
2. **`install.sh` prints a suggested crontab line** at the end of execution
   as guidance. It does NOT install, configure, or enable any scheduler.

The operator chooses how (or whether) to schedule the script: cron, systemd
timer, launchd, Task Scheduler, VPS snapshot, Time Machine, Windows Backup,
or their own strategy. The script is always available; the scheduling
decision is theirs.

## Consequences

**Positive**:
- `install.sh` is OS-agnostic for the backup step (one less platform branch
  to maintain and test)
- Scheduling is a sysadmin concern, not a product concern
- Operators can adopt the strategy that matches their environment (VPS
  snapshots, enterprise backup tools, etc.) without fighting the installer
- The script itself is reusable for ad-hoc backups (manual `bash
  scripts/backup.sh` works on demand)

**Negative**:
- Some operators will not configure a scheduler, leading to silent data loss
  if they expect automatic backups. Mitigated by the suggestion printed at
  the end of `install.sh` and by the rollback plan in the PRD pointing to
  the script.
- The `install.sh` log may be ignored; documentation must make the
  suggestion prominent.

**Rejected alternatives**:
- **Auto-configure cron in `install.sh`**: violates "minimal dependencies"
  and the user-level install model
- **Expose `create_backup` as an MCP tool**: the binary becomes responsible
  for backup logic; the script approach is simpler and the operator can use
  it from any scheduler
- **No backup mechanism at all**: unacceptable for a business system

## References

- `docs/PRD.md` §1.4 — entregables (backup script listed)
- `docs/PRD.md` §3.5 — Affected Areas (includes `scripts/backup.sh`)
- `docs/PRD.md` §3.6 — Rollback plan (uses the script)
- `docs/PRD.md` §5.1 RF9 — install.sh criterion (prints suggestion)
- `docs/PRD.md` §5.2 RNF Resiliencia — backup row reflects the script
- `docs/PRD.md` Fase 5 — install-and-service (script as deliverable)
- Commit `194888d` — Option A applied
