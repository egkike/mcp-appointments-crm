# ADR-0014: Release and deploy workflow — GitHub Releases + GoReleaser

- **Status**: accepted
- **Date**: 2026-08-27
- **Authors**: Kike

## Context

The project needs a reproducible, auditable release and deployment story that satisfies
several constraints accumulated across previous ADRs and the PRD:

1. **Reproducible artifact for 5 platforms.** The binary is pure Go (`modernc.org/sqlite`,
   no CGo) and cross-compiles cleanly to `linux/amd64`, `linux/arm64`, `darwin/amd64`,
   `darwin/arm64`, `windows/amd64` (see `docs/PRD.md §3.5` cross-compilation matrix). A
   release must produce the same 5 binaries from the same tag, with embedded version
   metadata (`ldflags`) and verifiable checksums. Manual `go build` on a developer machine
   is not reproducible enough.

2. **Keep the VM clean.** The HomeLab VM (`100.95.242.72` via Tailscale, the canonical
   deployment target for this project) must not require a Go toolchain, C toolchain, or
   Docker daemon at runtime. The install path should download a prebuilt binary over HTTPS
   and register it as a user-level service. This aligns with
   [ADR-0001](./0001-no-docker.md) (no Docker) and
   [ADR-0002](./0002-user-level-install.md) (user-level install, no root).

3. **Windows publisher warnings and cost.** Shipping an unsigned `.exe` directly triggers
   Windows SmartScreen / "Unknown publisher" warnings. The mechanism is:
   - Downloaded executables receive the **Mark of the Web (MotW)** alternate data stream
     when fetched via browser or `Invoke-RestMethod`/`curl`.
   - SmartScreen checks the signing publisher against a reputation database. Unsigned
     binaries show "Windows protected your PC" / "Unknown publisher".
   - Obtaining a trusted signing identity costs **OV ~$80–300/yr** or **EV ~$300–700/yr**,
     plus identity validation (organization vetting, HSM for EV). Reputation for a new
     publisher must still be built over time even after paying.
   - For a self-hosted, single-operator tool this cost and process is disproportionate for
     v1.0. A release strategy must avoid forcing every Windows user through an unsigned-EXE
     SmartScreen interstitial when a cheaper, cleaner path exists.

4. **Alignment with existing ADRs.** The release workflow must remain consistent with:
   - [ADR-0001](./0001-no-docker.md) — single binary, no container runtime.
   - [ADR-0002](./0002-user-level-install.md) — user-level service registration
     (`systemd --user` / `launchd` / Task Scheduler) and `loginctl enable-linger` on Linux.
   - [ADR-0007](./0007-server-config.md) — loopback-only bind (`127.0.0.1:3000` by default,
     validated at startup, no automatic port fallback) and env-file precedence.
   - [ADR-0005](./0005-optional-external-tools.md) — scripts never silently install
     system tools; they check and print OS-specific install commands.

5. **Inspiration from `gentle-ai` release pattern.** The `gentle-ai` CLI ships two
   complementary paths: `curl | bash` for Linux/macOS and `go install` for Windows.
   The former keeps the target machine free of a toolchain; the latter builds locally
   so the binary never carries MotW and never triggers SmartScreen for publisher
   reputation. The pattern is proven but must be adapted: our GoReleaser config,
   asset naming, checksum verification, and service-registration steps differ.

6. **PRD obligations.** `docs/PRD.md §3.1` (loopback transport), `§3.5` (install layout
   and cross-compilation matrix), `§8` (roadmap — install-and-service in Phase 5), and
   objective **O2** (install on a clean Ubuntu VPS in < 5 min from `curl | bash` to
   "MCP Server Active") all assume a one-line install story and a versioned release
   artifact that a runbook can reference.

Without a decision, the project ships ad-hoc binaries, manual `scp` deploys, and no
verifiable provenance.

## Decision

**GitHub Releases + GoReleaser is the source of truth for every versioned artifact.**
Releases are triggered by pushing an annotated tag `vX.Y.Z`. The workflow applies to
all 5 platforms and both Unix and Windows install paths.

### Decision 1: GoReleaser builds 5 artifacts + checksums + version ldflags

- **Trigger**: `git tag vX.Y.Z && git push origin vX.Y.Z` triggers
  `.github/workflows/release.yml` (build + publish). No manual asset upload.
- **Tool**: [GoReleaser](https://goreleaser.com/) (declarative `.goreleaser.yaml`).
  Responsible for cross-compilation, archive naming, checksum generation, and GitHub
  Release creation.
- **Artifacts per release** (5 binaries + 1 checksum file):

  | Asset | GOOS | GOARCH | Notes |
  |---|---|---|---|
  | `mcp-server_linux_amd64` | linux | amd64 | archived as `mcp-appointments-crm_Linux_x86_64.tar.gz` |
  | `mcp-server_linux_arm64` | linux | arm64 | `mcp-appointments-crm_Linux_arm64.tar.gz` |
  | `mcp-server_darwin_amd64` | darwin | amd64 | `mcp-appointments-crm_Darwin_x86_64.tar.gz` |
  | `mcp-server_darwin_arm64` | darwin | arm64 | `mcp-appointments-crm_Darwin_arm64.tar.gz` |
  | `mcp-server_windows_amd64.exe` | windows | amd64 | `mcp-appointments-crm_Windows_x86_64.zip` |
  | `checksums.txt` | — | — | SHA256 for all 5 archives |

  Exact archive names follow GoReleaser `{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}` templating.
  The table above shows the canonical mapping; `install.sh` / `install.ps1` resolve the
  correct asset at runtime (see Decision 2/3).
- **Version injection**: `ldflags` set at build time:

  ```yaml
  ldflags:
    - -s -w -X github.com/egkike/mcp-appointments-crm/internal/version.Version={{.Version}}
            -X github.com/egkike/mcp-appointments-crm/internal/version.Commit={{.Commit}}
            -X github.com/egkike/mcp-appointments-crm/internal/version.Date={{.Date}}
  ```

  `mcp-server --version` and `GET /mcp` health metadata expose the same values.
- **CGO disabled**: `CGO_ENABLED=0` for all targets (pure Go via `modernc.org/sqlite`).
  No cross-toolchain required on the builder.
- **Provenance**: every release publishes `checksums.txt` (SHA256). Install scripts
  verify the downloaded archive against this file before extraction. Artifacts are
  fetched exclusively over HTTPS from `github.com`.

### Decision 2: Linux and macOS — `curl | bash` via `scripts/install.sh`

Primary install path for Unix. Keeps the target VM free of a Go toolchain.

```bash
# latest
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash
# pinned version
curl -fsSL https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh | bash -s -- --version v0.3.0
```

What `install.sh` does (in order):

1. **Detect OS/arch** via `uname -s` / `uname -m` and map to GoReleaser asset names
   (`Linux`/`Darwin` × `x86_64`/`arm64`). Fails with a clear message on unsupported
   platforms.
2. **Resolve version**: `--version vX.Y.Z` uses that tag; otherwise queries the
   GitHub Releases API for the latest `v*` tag. No `latest` symlink assumption.
3. **Download** the matching archive from
   `https://github.com/egkike/mcp-appointments-crm/releases/download/<version>/…`
   over HTTPS to a temp dir. Respects `TMPDIR`.
4. **Verify SHA256** against `checksums.txt` from the same release
   (`sha256sum -c` / `shasum -a 256 -c` fallback). Abort on mismatch.
5. **Install binary** to `~/.local/bin/mcp-server` (or `/usr/local/bin/mcp-server`
   if `--system` is passed and the user has write permission; default is user-level
   per ADR-0002). Ensures `~/.local/bin` is on `PATH` and prints a hint if not.
6. **Write config** `~/.config/mcp-appointments-crm/.env` if absent (defaults
   `MCP_BIND=127.0.0.1`, `MCP_PORT=3000`). Never overwrites an existing `.env`.
   Respects `XDG_CONFIG_HOME` / `XDG_DATA_HOME` / `XDG_STATE_HOME` when set.
7. **Register user-level service**:
   - Linux: `~/.config/systemd/user/mcp-appointments-crm.service` with
     `EnvironmentFile=%h/.config/mcp-appointments-crm/.env`, then
     `systemctl --user daemon-reload && systemctl --user enable --now mcp-appointments-crm`.
   - macOS: `~/Library/LaunchAgents/com.mcp.appointments.server.plist` then
     `launchctl bootstrap gui/$UID … && launchctl kickstart …`.
8. **Enable linger on Linux**: `loginctl enable-linger $USER` (one-time, idempotent;
   required so the user service survives logout on a VPS — ADR-0002).
9. **Health verification**: `curl --fail --max-time 5 http://127.0.0.1:3000/mcp`
   (or the configured `MCP_BIND`/`MCP_PORT` from `.env`). Prints next steps on
   success; prints `journalctl --user -u mcp-appointments-crm -n 50 --no-pager`
   hint on failure.
10. **Loopback validation** is not in the script — it is enforced by the binary
    itself at startup per ADR-0007 (rejects non-loopback `MCP_BIND` before bind).

Manual alternative (no pipe) is documented in `docs/deployment.md`:

```bash
curl -fsSLO https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh
bash install.sh --version v0.3.0
```

### Decision 3: Windows — `go install` as primary, `irm install.ps1` as fallback

Windows has two supported paths. The split directly addresses the SmartScreen/MotW
cost problem.

#### Primary (recommended): `go install` — no publisher warning, no cert cost

```powershell
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@latest
# pinned
go install github.com/egkike/mcp-appointments-crm/cmd/mcp-server@v0.3.0
# register as user-level service (Task Scheduler / NSSM) after install
mcp-server --register-service
```

- The binary is **built locally** by the Go toolchain, so it is never downloaded
  as an EXE over HTTPS and never receives a Mark of the Web ADS. SmartScreen's
  publisher-reputation check for downloaded binaries does not apply (there is no
  downloaded binary — only `go` fetched module sources over HTTPS and compiled
  them).
- No code-signing certificate is required. No "Unknown publisher" interstitial.
- The Go toolchain is the only prerequisite (single `winget install Go.Go` or
  `scoop install go`). This is acceptable because the Windows audience for this
  project is small and the toolchain is a one-time install.
- After `go install`, the operator runs `mcp-server --register-service` (or
  `mcp-server install-service`) which creates a user-level Task Scheduler entry
  (or NSSM service if available) pointing at `%USERPROFILE%\go\bin\mcp-server.exe`
  with the same `.env` / loopback semantics as Unix.

#### Fallback: `irm install.ps1` — prebuilt EXE with explicit SmartScreen note

```powershell
irm https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.ps1 | iex
# pinned
& ([scriptblock]::Create((iwr https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.ps1).Content)) -Version v0.3.0
```

- Downloads `mcp-server_windows_amd64.exe` from the GitHub Release over HTTPS,
  verifies SHA256 against `checksums.txt`, installs to
  `%LOCALAPPDATA%\Programs\mcp-server\mcp-server.exe`, and registers NSSM or
  Task Scheduler (user-level).
- **SmartScreen warning for unsigned EXE**: because the EXE is downloaded from
  the internet, it carries MotW and SmartScreen will show **"Windows protected
  your PC" / "Unknown publisher"** for an unsigned binary. The script prints a
  warning before download explaining the interstitial and how to proceed
  ("More info" → "Run anyway") and that the warning disappears only if the
  project later ships a signed binary from a publisher with established
  reputation. No cert is purchased for v1.0 — the `go install` path is the
  recommended way to avoid the dialog entirely.

Both Windows paths validate `MCP_BIND` as loopback at startup (ADR-0007) and read
`%APPDATA%\MCP Appointments CRM\.env` when present.

### Decision 4: Verification, rollback, and security

- **Verification after every install** (script + manual):
  ```bash
  curl --fail http://127.0.0.1:3000/mcp
  systemctl --user is-active mcp-appointments-crm   # Linux
  launchctl list | grep com.mcp.appointments        # macOS
  journalctl --user -u mcp-appointments-crm -n 50   # Linux logs
  sqlite3 ~/.local/share/mcp-appointments-crm/reservas.db "SELECT count(*) FROM bookings;"
  ```
  `scripts/install.sh` runs the HTTP health check automatically; `install.ps1`
  runs an equivalent `Invoke-RestMethod` check.

- **Rollback**: re-run the install script pinned to the previous tag, or
  `go install …@v<previous>` on Windows. Data (SQLite + backups) lives outside
  the binary path (`~/.local/share/mcp-appointments-crm/` per ADR-0002) so a
  binary rollback never touches data. For data rollback, restore the latest
  `reservas-YYYYMMDD.db.gz` produced by `scripts/backup.sh` (see PRD §3.6).

- **Security**:
  - All downloads over **HTTPS** from `github.com` / `raw.githubusercontent.com`.
  - **SHA256 verification** against `checksums.txt` before extraction/execution.
  - **Loopback bind validation** at startup (`127.0.0.0/8` or `::1` only — ADR-0007);
    any non-loopback `MCP_BIND` fails before the socket is opened.
  - No `sudo` / no root at any point (ADR-0002). No Docker daemon (ADR-0001).
  - Version is auditable via `mcp-server --version` (ldflags) and GitHub Release
    tag + commit SHA.

## Consequences

### Positive

- **Reproducible and auditable**. Every `vX.Y.Z` tag maps to one GitHub Release
  with 5 deterministic binaries + `checksums.txt` + commit-anchored ldflags.
- **VM stays clean**. Linux/macOS install needs only `curl` + `bash` on the target
  (both present on a clean Ubuntu image). No Go toolchain, no C toolchain, no
  Docker daemon.
- **No certificate cost for v1.0**. Windows primary path (`go install`) builds
  locally, never carries MotW, and never triggers the "Unknown publisher"
  SmartScreen interstitial. The project avoids **OV ~$80–300/yr** and
  **EV ~$300–700/yr** plus identity vetting for the initial releases.
- **Fast install**. Satisfies PRD O2 (< 5 min from `curl | bash` to health check)
  on the HomeLab VM and on a clean VPS. No compilation on Unix; one-time
  compilation on Windows primary path.
- **Consistent with existing ADRs**. User-level service model, loopback-only
  transport, no silent system-tool installs — all preserved.
- **Adapted gentle-ai pattern**. Reuses the proven `curl | bash` + `go install`
  dual-path idea without copying its config verbatim; asset naming, service
  registration, linger, and health check are tailored to this project's layout
  (XDG / LaunchAgents / Task Scheduler).

### Negative

- **Windows primary path requires Go**. Operators who choose `go install` must
  install the Go toolchain once (the fallback `install.ps1` avoids this at the
  cost of the SmartScreen interstitial).
- **GoReleaser maintenance**. `.goreleaser.yaml` and `.github/workflows/release.yml`
  are additional config to maintain and test (dry-run via `goreleaser check` /
  `goreleaser build --snapshot --clean`).
- **SmartScreen reputation still needed if we later sign**. If the project ever
  purchases an OV/EV certificate and ships signed EXEs, SmartScreen reputation
  must still be built over time for the new publisher; existing unsigned installs
  do not transfer reputation. The `go install` path remains the escape hatch.
- **Two Windows docs to maintain**. Both install paths must be documented and
  tested in CI (or manually on a Windows runner).

### Rejected alternatives

| Alternative | Why rejected |
|---|---|
| **Only `go install` everywhere** | Forces a Go toolchain on every VM, including the HomeLab Linux VM (`100.95.242.72`) and every clean Ubuntu VPS. Contradicts "VM clean" and PRD O2 (< 5 min). Unix users expect a prebuilt binary. |
| **Only prebuilt EXE + code-signing cert** | Requires purchasing and renewing an OV/EV cert (cost + identity vetting + HSM for EV), still needs reputation warm-up, and does not remove MotW — it only changes the SmartScreen dialog from "Unknown" to the org name. Overkill for a single-operator self-hosted tool at v1.0. |
| **Docker / Docker Compose** | Rejected globally by ADR-0001 (pure-Go binary, loopback transport, direct SQLite filesystem access, no isolation benefit, contradicts "lightweight"). Adds a daemon and image CVE surface for no gain. |
| **Manual `scp` / `sftp` deploy** | Not reproducible, no checksums, no version ldflags, no service registration, no health check, no rollback story. |

## References

- `docs/PRD.md §3.1` — loopback transport (`127.0.0.1:3000` + Streamable HTTP).
- `docs/PRD.md §3.5` — install layout (XDG / platform-native paths), cross-compilation matrix, `.env` precedence.
- `docs/PRD.md §3.6` — rollback plan (binary restore + SQLite backup).
- `docs/PRD.md §8` — roadmap (Phase 5 install-and-service, Phase 4 config-wizard / install.sh prompts).
- `docs/PRD.md §2.1 O2` — < 5 min install on a clean VPS with `curl` + `bash`.
- [ADR-0001](./0001-no-docker.md) — no Docker in deployment.
- [ADR-0002](./0002-user-level-install.md) — user-level install with XDG / platform-native paths, `loginctl enable-linger`.
- [ADR-0005](./0005-optional-external-tools.md) — project does not install external system tools; only suggests.
- [ADR-0007](./0007-server-config.md) — server bind and port configuration (loopback validation, env precedence, no port fallback).
- [ADR-0008](./0008-install-prompts.md) — inline prompts in `install.sh` (no separate TUI).
- Repository: <https://github.com/egkike/mcp-appointments-crm>
- Raw install scripts:
  - `https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.sh`
  - `https://raw.githubusercontent.com/egkike/mcp-appointments-crm/main/scripts/install.ps1`
- GoReleaser: <https://goreleaser.com/>
- gentle-ai release pattern (inspiration, adapted): `curl` for Linux/macOS + `go install` for Windows.
