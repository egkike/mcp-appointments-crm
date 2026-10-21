# Tasks: fix-v081-skill-seed-oq5

> **Change:** fix-v081-skill-seed-oq5 (B11 + OQ-5, v0.8.1)
> **Spec autoritativa:** `odd/tasks/feat-b11-oq5-seed-fixes.md` (S11–S16, T1–T8)

## Phase 1 — doc-first

- [x] `openspec/changes/fix-v081-skill-seed-oq5/proposal.md`
- [x] `openspec/changes/fix-v081-skill-seed-oq5/design.md`
- [x] `openspec/changes/fix-v081-skill-seed-oq5/specs/seed-fixes/spec.md`
- [x] `openspec/changes/fix-v081-skill-seed-oq5/tasks.md` (este archivo)

## Phase 2 — apply

- [ ] **TASK-2.1 (B11)** — `seed_skill_pristine` + invocación en `run_deploy` + summary line; RED→GREEN test en `scripts/tests/install_deploy_test.sh` (existe, es copia del pristine, 0600/0700, no render en `~/.hermes/skills`)
- [ ] **TASK-2.2 (OQ-5 wizard, RED bash)** — test sintético de `render_setup_staff` con specialties elegidas emite nombres; test de re-prompt ante inválido
- [ ] **TASK-2.3 (OQ-5 wizard, GREEN)** — invertir secciones en `run_setup`; prompt multi-select en `run_staff_section` (válido/re-prompt/vacío con aviso/duplicados); `render_setup_staff` con nombres actualizados; checkpoint keys `staff.N.spec.J`
- [ ] **TASK-2.4 (OQ-5 seeder, RED Go)** — test de integración: specialties en DB = uuids de services同名; nombre desconocido → error semántico español con lista válida; rollback: DB sin profissionais/ni servicios
- [ ] **TASK-2.5 (OQ-5 seeder, GREEN)** — reordenar `seed()`: services primero + mapa nombre→id + resolver specialties + `stringSliceToJSON`; comentario de campo en `setup_types.go` actualizado
- [ ] **TASK-2.6 (S16 regresión)** — JSON legacy `specialties: []` importa y queda vacío; pipeline Go completo (`go build ./...`, `go test -race ./internal/config/...`, `-race ./...` al final); `bash scripts/tests/run_tests.sh` + shellcheck
- [ ] **TASK-2.7 (docs)** — PRD (contrato wizard/setup JSON), changelog 1.24 v0.8.1, README si mentiona flujo wizard
- [ ] **TASK-2.8 (delivery fecha del merge)** — PR code-lane, review nativo, tras merge: tag `v0.8.1` (GoReleaser), smoke por defensor del cambio en VM upgradeado

## Forecast

- ~300-400 authored lines (bash wizard + Go seeder + tests + docs)
- Un solo PR (no excede el budget de review de 400 en screenings del change; tests son la parte mayor)
