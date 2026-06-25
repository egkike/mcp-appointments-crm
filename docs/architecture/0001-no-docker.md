# ADR-0001: No Docker in deployment

- **Status**: accepted
- **Date**: 2026-06-25
- **Authors**: Kike

## Context

The original idea document (`docs/SDD.md`, section 2) argued for Docker as the
deployment mechanism, citing isolation across Linux distributions, one-command
updates, and easy backups. The project has properties that weaken that argument:

1. **Pure-Go SQLite**: We use `modernc.org/sqlite` (no CGo), so the binary is
   fully self-contained and cross-compiles cleanly to 5 platforms.
2. **Loopback-only transport**: The MCP server binds to `127.0.0.1:3000`. No
   public exposure, no TLS, no authentication. The isolation Docker provides
   (network namespaces, capabilities) does not add value.
3. **SQLite wants direct filesystem access**: SQLite performs best and is
   most reliable with direct access. Docker bind mounts and UID/GID mapping
   add complexity (especially for the WAL files `reservas.db-wal` and
   `reservas.db-shm`).
4. **Self-hosted, lightweight positioning**: The project pitches itself as
   "self-hosted, lightweight" — running a Docker daemon and pulling images
   is neither.
5. **"Minimal dependencies" principle**: A stated project principle
   (AGENTS.md, PRD) is to avoid dependencies that are not strictly necessary.

## Decision

We do not use Docker or Docker Compose. Deployment is a single cross-compiled
Go binary registered as a user-level system service. See
[ADR-0002](./0002-user-level-install.md) for the install model.

## Consequences

**Positive**:
- One binary per platform, no container runtime required
- Faster install, faster cold start, lower memory footprint
- SQLite gets direct filesystem access (no overlay filesystem surprises)
- No CVEs in base images to track
- Docker daemon failure modes are not our problem

**Negative**:
- Cross-distro compatibility must be tested at the binary level
- Service registration is per-OS (systemd / launchd / Task Scheduler) instead
  of a single `docker-compose.yml`
- We lose the "container as a unit of deployment" abstraction

**Rejected alternatives**:
- **Docker + docker-compose**: keeps the isolation story but adds runtime
  overhead and contradicts "lightweight"
- **Podman**: similar to Docker but with rootless mode; doesn't address the
  core "do we need a container?" question

## References

- `docs/SDD.md` §2 — original Docker argument
- `docs/PRD.md` §3 — current architecture description (post-removal)
- `docs/PRD.md` §3.5 — install layout (no Docker)
- `docs/PRD.md` §6 — dependencies (D6 removed)
- Commit `194888d` — "docs(prd): remove Docker, switch to user-level install"
