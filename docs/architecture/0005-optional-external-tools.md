# ADR-0005: Project does not install external system tools; only suggests

- **Status**: accepted
- **Date**: 2026-06-25
- **Authors**: Kike

## Context

The project has a "minimal dependencies" principle that has been applied
consistently in earlier decisions: [ADR-0001](./0001-no-docker.md) (no Docker),
[ADR-0002](./0002-user-level-install.md) (user-level install, no `appuser`), and
[ADR-0003](./0003-portable-backup.md) (no auto-configured scheduler for
backups). The natural extension of this principle is whether our scripts
(`install.sh`, `backup.sh`, future scripts) should install external system
tools that the OS package manager manages.

The candidate case is `sqlite3` CLI. `scripts/backup.sh` uses the native
`sqlite3 .backup` command for crash-consistent snapshots. If the operator
schedules the backup but doesn't have the CLI installed, the backup will
fail. The question is: should our project install `sqlite3` on the
operator's behalf?

Three alternatives were considered:
1. **Auto-install in `install.sh`**: `install.sh` detects the OS and
   package manager, runs the appropriate install command (with `sudo` if
   needed).
2. **Auto-install in `backup.sh`**: `backup.sh` checks and installs on
   every run.
3. **Check and message only** (this ADR): scripts detect the tool's
   presence, and if missing, print a clear OS-specific install command
   for the operator to run manually.

## Decision

Project scripts **never** install external system tools that the OS
package manager manages. They do check if the tool is present, and if
missing, they print a clear OS-specific install command for the operator
to run.

This applies to all current and future optional features that depend on
external tools. Core dependencies (the Go binary, SQLite via
`modernc.org/sqlite`) are bundled in the binary and don't need system
package installation.

The principle is enforced by:
- **`install.sh`**: at the end, prints a "Recommended additional tools"
  block listing each optional tool's status (✓ found / ⚠ not found) and
  the install command for the current OS.
- **`backup.sh`**: at startup, checks for `sqlite3` and fails with a clear
  error message (including install commands) if missing.

## Consequences

**Positive**:
- Zero cross-platform package manager logic in our scripts (no
  detection of apt / dnf / pacman / brew / choco / scoop / winget)
- The operator has full agency over their system — no implicit mutations
- Aligns with the spirit of ADR-0001, ADR-0002, and ADR-0003
- Predictable failure mode: if a tool is missing, the script fails
  immediately with a clear, actionable error message
- Self-documenting: the error message **is** the documentation
- Backup script stays small and pure (no installer logic mixed in)
- Works in unattended contexts (cron, systemd timer, etc.) where `sudo`
  is not available

**Negative**:
- The operator has to install the tool manually if they want the
  optional feature
- If they forget, the optional feature fails when they try to use it
  (no surprise — that's the design)
- Slightly more friction than auto-install

**Rejected alternatives**:
- **Auto-install in `install.sh`**: requires ~30 lines of
  OS/package-manager detection logic; mutates the system implicitly;
  needs `sudo`; not consistent with the "minimal dependencies" principle
- **Auto-install in `backup.sh`**: same issues, plus it runs unattended
  in cron / systemd timer where `sudo` is not available, so the install
  would fail anyway
- **Vendor the tool**: impossible for `sqlite3` (it's a system tool
  tightly coupled to the OS SQLite library)
- **Write our own backup in Go**: would duplicate `sqlite3 .backup`
  logic and risk losing crash-consistency guarantees

## References

- [ADR-0001](./0001-no-docker.md) (no Docker) — same philosophy
- [ADR-0002](./0002-user-level-install.md) (user-level install) — same philosophy
- [ADR-0003](./0003-portable-backup.md) (portable backup, no auto-scheduler) — same philosophy
- `docs/PRD.md` §5.1 RF9 — install.sh criteria reference this ADR
- `docs/PRD.md` Fase 5 — install-and-service deliverables reference this ADR
