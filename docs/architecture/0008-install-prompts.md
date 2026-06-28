# ADR-0008: Inline prompts in `install.sh` (no separate TUI for config-wizard)

- **Status**: accepted
- **Date**: 2026-06-26
- **Authors**: Kike

## Context

The original design (per `docs/SDD.md §B`) included a separate
`config-wizard` binary using Charm Bubble Tea to capture the initial
business configuration interactively. The wizard would run
**before** `install.sh`, producing 3 JSON files that `install.sh`
then validated and used.

The judgment-day review of 2026-06-25 surfaced concerns about
this approach during the "TUI for setup" discussion on 2026-06-26:
- A TUI framework for a one-time setup is over-engineered
  (~300-500 LOC + `teatest` + 3 dependencies).
- A two-step flow (`config-wizard` then `install.sh`) is
  unnecessarily complex for an operator doing one install.
- A TUI library contradicts [ADR-0005](./0005-optional-external-tools.md)
  (no external runtime tools; per-project only).
- Same RF1 coverage (3 JSON outputs, email + HH:MM validation) can
  be achieved in bash with `read -p` + regex.

## Decision

**Replace the `config-wizard` TUI with inline prompts in `install.sh`**.
The single install flow becomes:

```bash
$ curl -fsSL https://.../install.sh | bash
[install.sh] Bienvenido al setup de MCP Server
Business name: ...
Country code (ISO 3166-1): ...
Currency code (ISO 4217): ...
...
[install.sh] Setup JSON escrito en ~/.config/...
[install.sh] Descargando binario...
[install.sh] Registrando servicio systemd...
[install.sh] Listo.
```

A **checkpoint mechanism** (`setup.json.tmp`) handles the case where
the user cancels (`Ctrl+C`) mid-flow. After each successful prompt
the script writes the in-progress data to `setup.json.tmp`. On
re-run, the script detects the checkpoint and offers
`[R]esume / [S]tart over / [Q]uit`. On completion, the script
atomically writes the 3 final JSON files and removes `setup.json.tmp`.

## Consequences

**Positive**:
- Zero new Go code; `install.sh` grows by ~100 LOC of bash
- Single install flow (`curl | bash install.sh` does everything)
- Cancel-safe (checkpoint + resume) without code complexity
- No new dependencies (per ADR-0005 philosophy)
- Same RF1 coverage with much less code

**Negative**:
- Less polished UX than a TUI (no undo, no inline validation
  feedback during typing — only on submission)
- Bash validation logic is less type-safe than Go
- Mixed language (bash for setup, Go for server) — already the
  case with `install.sh`

**Rejected alternatives**:
- (B) CLI Go without TUI framework (`bufio.Scanner` + simple
  prompt loop): more code, same UX as bash
- (C) Hand-edit `setup.json` file: bad UX, validation deferred
  to install time
- (D) Original TUI with Bubble Tea: ~300-500 LOC + 3 dependencies
  for a one-time tool

## Reversibility

If a polished TUI is wanted later (Fase 6+), it can be added on
top of this baseline. The `install.sh` prompts would still work;
the TUI would be a richer alternative. The current RF1 spec is
written so that a future `config-wizard` does not contradict it.

## References

- `docs/PRD.md` §5.1 RF1 (reformulated for this decision)
- `docs/PRD.md` §8.4 Fase 4 (simplified to "install.sh prompts")
- [ADR-0005](./0005-optional-external-tools.md) (no external runtime tools)
- Commit TBD (this change)
