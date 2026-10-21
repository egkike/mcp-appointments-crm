# Tasks: fix-v081-skill-seed-oq5

> **Change:** fix-v081-skill-seed-oq5 (B11 + OQ-5, v0.8.1)
> **Spec autoritativa:** `odd/tasks/feat-b11-oq5-seed-fixes.md` (S11–S16, T1–T8)

## Phase 1 — doc-first

- [x] `openspec/changes/fix-v081-skill-seed-oq5/proposal.md`
- [x] `openspec/changes/fix-v081-skill-seed-oq5/design.md`
- [x] `openspec/changes/fix-v081-skill-seed-oq5/specs/seed-fixes/spec.md`
- [x] `openspec/changes/fix-v081-skill-seed-oq5/tasks.md` (este archivo)

## Phase 2 — apply

- [x] **TASK-2.1 (B11)** — `seed_skill_pristine` en run_deploy (no fatal + summary line) — commit `4d53ad1`; 42/42 deploy tests (5 nuevos: byte-copy 0600/0700, template ausente no fatal, CONFIG_DIR no escribible no fatal, sin skill renderizada, línea de summary); harness fix: stub `atomic_write` con save/restore
- [x] **TASK-2.2 (OQ-5 wizard, RED bash)** — `scripts/tests/install_specialties_test.sh` 11 casos, RED observado (31 fallos antes de implementar); render con nombres y vacío; re-prompt inválido
- [x] **TASK-2.3 (OQ-5 wizard, GREEN)** — pasada `run_staff_specialties_pass` después de servicios (diseño §2.1 corregido); prompt índices/nombres/comas, dedup ordenado, Enter vacío con aviso, marcador `staff.N.specset`; render emite nombres escapados; e2e canned +1 línea por profesional (26/26); commit `95cf1ee`
- [x] **TASK-2.4 (OQ-5 seeder, RED Go)** — `internal/config/setup_specialties_seeder_test.go` RED: specialties guardan ids reales / nombre desconocido = error semántico español con nombres válidos / rollback completo
- [x] **TASK-2.5 (OQ-5 seeder, GREEN)** — `seed()` inserta servicios primero + mapa nombre→id + resolución fail-closed dentro de la misma tx; comentario de campo en `setup_types.go` actualizado; commit `17fcd3c` (GGA PASSED r1 tras `gga cache clear`)
- [x] **TASK-2.6 (S16 regresión)** — JSON legacy `specialties: []` importa vacío; pipeline completo verde: `golangci-lint run ./...` 0 issues, `go build ./...`, `go test -race ./...` (go los grep 'ok'), bash suites 7/7 (`run_tests.sh`), shellcheck sin hallazgos nuevos vs main (3 SC2015 preexistentes ensure_dirs)
- [x] **TASK-2.7 (docs)** — PRD changelog 1.24 + header; `docs/installation.md`: pasada de specialties en Paso 1 + seed del template pristino en Paso 2
- [ ] **TASK-2.8 (delivery fecha del merge)** — PR code-lane, review nativo, tras merge: tag `v0.8.1` (GoReleaser), smoke por defensor del cambio en VM upgradeado

## Forecast

- ~300-400 authored lines (bash wizard + Go seeder + tests + docs)
- Un solo PR (no excede el budget de review de 400 en screenings del change; tests son la parte mayor)
