# Feature: GoReleaser + releases por CI (release v0.4.0)

- **Created**: 2026-09-13
- **Status**: in progress
- **Backlog refs**: PRD §7 Fase N, ADR-0014, docs/deployment.md (Release Process pending target)
- **Mirror**: `odd/goreleaser-release-ci/tasks` (Engram) — ENGRAM DOWN at creation
  (server ownership mismatch, pid 2664 at http://127.0.0.1:7437); context recovered
  from `odd/tasks/*.md`. Mirror save pending until Engram is restored.

## Goal

Replace the manual release procedure (v0.3.0 was built and uploaded by hand) with a
tag-triggered GoReleaser pipeline: `.goreleaser.yaml` + `.github/workflows/release.yml`
producing all 5 platform archives + `checksums.txt` on GitHub Releases per tag, per
ADR-0014. Then tag `v0.4.0` (content already merged: admin-tui T1-T8 + Hermes
maintenance #85).

## Contracts to respect (from ADR-0014 + install.sh)

- Asset names: `mcp-appointments-crm_{Linux,Darwin,Windows}_{x86_64,arm64}.tar.gz/.zip`
  (Windows: `.zip`). GoReleaser `{{ .ProjectName }}_{{ .Os }}_{{ .Arch }}` templating.
- Archive contents: `mcp-server` (binary, exact name — install.sh hard-codes
  `EXTRACT_DIR/mcp-server`), `scripts/backup.sh`,
  `setup/service/mcp-appointments-crm.service`,
  `setup/service/com.mcp.appointments.server.plist`.
- Windows zip: `mcp-server_windows_amd64.exe` (from `cmd/mcp-server`).
- ldflags: `-s -w -X github.com/egkike/mcp-appointments-crm/internal/buildinfo.Version={{.Version}}`
  (`internal/buildinfo.Version` exists, default "dev").
- `CGO_ENABLED=0` everywhere (pure Go, `modernc.org/sqlite`).
- `checksums.txt` SHA256 over all archives.
- Trigger: annotated tag `vX.Y.Z` push → release.yml; no manual upload.
- install.sh already resolves the Unix asset names (compose_asset_name, lines 1087-1110);
  no Windows consumption path (fails explicitly on Windows).
- Local dry-run gate: `goreleaser check` + `goreleaser build --snapshot --clean`.

## Tasks

- [x] **T1 — `.goreleaser.yaml`** ✅ (verified locally with goreleaser 2.18.2:
      `goreleaser check` exit 0; full snapshot release without GITHUB_TOKEN produced the
      5 contract archives + checksums.txt with exact names; archive payload = binary at
      root + scripts/backup.sh + 2 service templates; ldflags -> 0.3.0-SNAPSHOT-8eae404;
      statically linked, CGO off). NOTE: GoReleaser 2.18.2 renders raw .Os/.Arch lowercase
      in an explicit name_template, so the uname-style mapping is explicit in the template
      (title .Os / amd64->x86_64). Changelog groups/filters are skipped in snapshot mode;
      the first real tag run is their first true exercise.
- [x] **T2 — `.github/workflows/release.yml`** ✅ (tag trigger `v*`, permissions
      `contents: write`, single job, checkout@v7 fetch-depth 0, setup-go@v7,
      goreleaser-action@v6 pinned v2.18.2, args: release --clean; no concurrency block
      and no paths-filter by design).
- [x] **T3 — install.sh / docs cross-check** ✅ — compose_asset_name() output matches
      the pipeline's final asset names exactly (independently verified in dist/);
      Windows asset has no install.sh consumption path by design (explicit fail,
      install.ps1 unimplemented per ADR-0014 Decision 3).
- [x] **T4 — Docs** ✅ — deployment.md (Release Process step 5 -> CI flow, dry-run
      reframe, Artifacts note, plus one-token fix `mcp-server.exe` ->
      `mcp-server_windows_amd64.exe` at :122), ADR-0014 (Decision 1 status note ->
      implemented 2026-09-18; Decision 3 Windows-asset clause), PRD §7 Fase N item
      marked ENTREGADO + §2.5 note, README.md (scope-note GoReleaser item + macOS
      caveat + Windows install + runbook pointer). Known stale-but-true deferred notes:
      deployment.md ~243/~287 and PRD ~1284 frame Darwin/Windows as unavailable until
      the first tag runs through the pipeline.
- [x] **T5 — Gate + PR** ✅ — pre-flight pipeline PASSED (8/8). Native review
      review-d76fbe5576d59627 approved (4 lenses, high tier, 11 informational findings,
      0 blocking); acknowledgement burned, delivery per ordinary policy. Commits:
      `70d165c` (feat, code work unit) + `6f82a9a` (docs work unit incl. this doc).
      Pending user decision: push + PR + merge.

## Post-merge (user decisions, not part of this feature's code)

- Combined VM smoke: TUI seed + Hermes maintenance + booking flows (obs 910/914).
- Tag `v0.4.0` on main and push → CI builds and publishes the release.
- Optionally ride informational follow-ups from hermes-maintenance as tiny PRs before tagging.

## Verification evidence (2026-09-18)

- Pre-flight pipeline: PASS, 8/8 steps exit 0 (go fmt clean, go vet clean,
  golangci-lint 0 issues, build ok, go test -race all packages ok, goreleaser check
  ok, pin v2.18.2 == installed 2.18.2, YAML parse ok).
- Parent spot-checks: goreleaser check; dist/ artifact names (5 archives +
  checksums.txt exact contract names); archive payloads via tar -tzf / unzip -l
  (binary at root + scripts/backup.sh + 2 service templates in every archive);
  compose_asset_name() match.
- Working tree at gate: M docs/PRD.md, M docs/architecture/0014-release-and-deploy-workflow.md,
  M docs/deployment.md, M README.md, ?? .goreleaser.yaml, ?? .github/workflows/release.yml,
  ?? odd/tasks/goreleaser-release-ci.md.

## Evidence

- `70d165c` — feat(release): tag-triggered GoReleaser CI pipeline (ADR-0014):
  .goreleaser.yaml + .github/workflows/release.yml (158 insertions). Native review
  review-d76fbe5576d59627 approved (4 lenses, high tier, 11 informational findings,
  0 blocking; receipt burned).

(committed per task: branch `feat/goreleaser-release-ci`)
