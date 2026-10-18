# Proposal: feat-skill-inode-guard — close the external-writer lost-update blind spot

**Backlog**: B9 (issue #137, ownerConfirmed real 2026-10-18) · **ADR fence**: ADR-0017 (Enmienda 2026-10-16: consent write con flock + snapshot), ADR-0020 (single-writer disposition) · **Scope estimate**: ~80 lines (code + tests), one slice

## Summary

`admin.WriteSkillFileConsent` verifies the consent snapshot against the OPEN FD under the advisory lock. A non-cooperating writer (the Hermes agent itself, confirmed real in production by the owner) that REPLACES the file via rename carries new bytes in a different inode: the fd reads stale content, the compare passes, and the final rename silently discards the external edit (lost update).

## Outcome & scope

- **(a) inode guard**: inside `WriteSkillFileConsent`, after the lock and before the temp+rename, re-stat the PATH and compare its inode against the held fd's inode. Divergence (only meaningful when both are non-zero; zero inodes skip the guard instead of failing closed on filesystems without inode semantics) returns the SAME "run regeneration again" semantic the changed-after-consent path already provides — the caller re-enters the detect-confirm cycle with fresh bytes.
- **(b) attribution copy**: the TUI regen confirm warning attributes the modification: "Puede tratarse de una modificación de Hermes (procedural memory) o manual." Console parity via the same constant.
- **Residual micro-window, documented**: between the path stat and the rename there remains a structurally unavoidable non-lockable gap (external writers don't take our lock). This is narrower than the current "whole consent flow" window and matches the defense-in-depth posture; absolute exclusion needs an FS-level construct out of scope.

## Non-goals

- No lockfile companion for external writers (rejected in issue #137 analysis — effort/вентиля unclear).
- No behavior change for lock-cooperating writers or Windows (lock no-op; the guard is POSIX-stat only, Windows `os.Stat` works but semantics differ — guard stays active but comparison is on the same stat fields).
- No hermes-side config change (`skills.write_approval` lever remains operator-optional, documented).

## Owner decision embedded

Owner 2026-10-18: "Es un caso real que puede pasar así que ponlo en la lista de pendientes para resolverlo" — case REAL; hardening applies.
