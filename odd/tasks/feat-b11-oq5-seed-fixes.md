# feat-b11-oq5-seed-fixes

Fix de seed antes de v0.8.1: B11 (run_deploy siembra el template de la skill) + OQ-5 (wizard/seeder emiten specialties reales). Owner: eugeniokike (2026-10-21, routing elegido en conversación).

## Specs

- **S11 (B11, verbatim backlog L18)**: "**install.sh run_deploy no siembra el template de la skill** (defecto descubierto por el smoke B2 en v0.8.0): `install_receptionist_skill` solo corre en el wizard interactivo; upgrades via `--version` (run_deploy, línea 1584) dejan `<CONFIG_DIR>/templates/recepcionista-negocio/SKILL.md` inexistente → el cascade de la TUI falla fail-secure ("no se encontró el template"). Desbloqueo manual aplicado al VM (seed sha `b9d94d84…`, perms 0600/0700) — el fix real es code lane: seed del pristine en run_deploy (~15 líneas reusando `seed_config_template` + resolve_skill_template del EXTRACT_DIR), shellcheck + PR, luego patch release v0.8.1"
- **S12 (OQ-5, verbatim Log L25 backlog)**: "**OQ-5** seed-wizard specialties **junto a B11** antes de v0.8.1"
- **S13 (OQ-5, verbatim feat-owner-chat L8)**: "**OQ-5 junto a B11** — el wizard de install.sh pide specialties por profesional (setup_staff.json) y ese fix de seed va al mismo PR del seed template antes de v0.8.1"
- **S14 (diseño OQ-5, verbatim design feat-owner-chat :363)**: "exige invertir el orden del seed (services antes de staff, E60) y un contrato nuevo tipo `specialties_by_service_name` en `setup_staff.json`"
- **S15 (verificación OQ-5, verbatim feat-owner-chat L9)**: "el wizard specialties (OQ-5, junto a B11/v0.8.1) cubre instalaciones NUEVAS y se verifica con tests bash sintéticos en scripts/tests (sin VM)"
- **S16 (preservación de conducta)**: JSONs de setup existentes con `"specialties": []` siguen importando sin cambios; el deploy `--version` no intenta instalar/reescribir la skill renderizada (la TUI cascade es el path de render, queda intacto); el wizard pide specialties como multi-select numerado de servicios ya ingresados (Enter vacío permitido con aviso, nombres invalidados fail-closed en el seeder).

## Tasks

| ID | Spec | Route | Nota |
|---|---|---|---|
| T1 | S14 | doc-first | abre `openspec/changes/fix-v081-skill-seed-oq5/` |
| T2 | S11 | code | run_deploy: `seed_skill_pristine` + test en install_deploy_test.sh + shellcheck — **DONE 2026-10-21, commit `4d53ad1`** (92 lines: seed nonfatal + summary line + 5 tests deploy + harness save/restore stub atomic_write) |
| T3 | S13/S14 | code | wizard: secciones invertidas (services → staff) + prompt specialties multi-select + render |
| T4 | S13/S14 | code | seeder Go: inserta services primero, resuelve nombre→id fail-closed, error semántico con nombres válidos |
| T5 | S15 | test | tests bash sintéticos (setup/skill/deploy) + tests Go config (RED→GREEN) |
| T6 | S16 | tests | regresión: JSON legacy "" importa igual; deploy no render skill |
| T7 | S11-S15 | docs | PRD (contrato setup JSON), PRD tool count intacto, changelog v0.8.1 |
| T8 | S11-S15 | release | pipeline + PR + patch release v0.8.1 tras merge |

## Log

- **L1** 2026-10-21 — owner: "Hola capo, veamos el cuadro de pendientes del proyecto para luego ver cómo seguimos" → cuadro presentado (B11, B12 apply, B6 pasivo). Owner elige "B11 + OQ-5 juntos". Exploración: `run_deploy` (install.sh:1584-1698) no llama seed de template; `seed_config_template`/`resolve_skill_template` ya reutilizables (:1044-1106); `render_setup_staff` hardcodea `"specialties": []` (:930); `seed()` inserta staff antes de services (setup_seeder.go:151-184); `prof.Validate()` no inspecciona specialties (professional.go:50-58) → nombres pueden atravessar `validateForSeed` y resolverse dentro de la transacción. Doc-first creado.
- **L2** 2026-10-21 — doc-first commiteado y mergeado a main ff-only (`552954b`, docs lane; owner aprobó vía cuestionario). B11 (T2) aplicado inline: RED (tests fallos) → función `seed_skill_pristine` + call en run_deploy tras ensure_dirs + summary line → GREEN 42/42 deploy tests, 6/6 suites bash, shellcheck findings idénticos a main. Hallazgos de harness: stub de atomic_write sin restaurar (preexistente), subshell `$(fn)` pierde asignaciones. Commit `4d53ad1` en `feat/b11-oq5-seed-fixes` (GGA: bash no evaluable). OQ-5 (T3-T7) delegado a worker en background con commit pre-autorizado.
- **L3** 2026-10-21 — CORRECCIÓN de diseño del change (propia, previa al apply): la primera redacción de OQ5-1 invertía las secciones del wizard (services antes de staff). Se mantiene el orden canónico y las specialties se piden en una **pasada separada al final** (run_staff_specialties_pass después de run_services_section), porque minimiza la rotura de ~15 flujos canned del e2e y del checkpoint; la inversión de orden que exige S14 vive exclusivamente en el seeder Go (services antes de staff, resolución nombre→id). design.md §2.1/§2.2, spec.md OQ5-1 y proposal Scope reescritos; S11–S16 del owner intactas.
- **L4** 2026-10-21 — OQ-5 completo inline (2 commits): wizard `95cf1ee` (pasada + render + tests, 318 líneas) y seeder `17fcd3c` (services antes de staff + resolución nombre→id fail-closed + tests, 190 líneas). Pipeline: fmt/vet/lint 0/build/-race all, bash suites 7/7, e2e 26/26, shellcheck sin hallazgos nuevos. GGA del seeder falló r1 por parser STRICT (bug #113) — cache clear + re-commit → PASSED r1 (`17fcd3c`). Docs: PRD 1.24 + installation.md (Pasos 1 y 2). Total rama: 3 commits, ~600 authored lines (excede el slice de 400 → PR encadenado o squash por unidad; decisión del owner en el gate). TASK-2.8 (PR + v0.8.1) pendiente del owner.
- **L5** 2026-10-21 — review nativo cerrado end-to-end: lineage `review-5156208c02a17a69`, tier high, 4 lentes (risk/resilience/readability/reliability) + refuter + validator. Refuter corroboró R3-001 CRITICAL (dedup por pattern en los metacaracteres glob del nombre de servicio — introducido) → correction plan 22 diff lines → fix commit `c227b6a` (helper `picks_seen_has` con igualdad exacta + test regresión glob) → validator targeted → **APPROVED** + authority burned (`gentle-ai.review-acknowledged/v1`; delivery ordinary repository policy). Advisories no-bloqueantes para luego: R2-001 WARNING readability (`scripts/install.sh:934`), R2-002 SUGGESTION (`docs/PRD.md:1470`). Push + PR ejecutados con especial consent del owner (envase 1 PR, 5 commits, ~705 authored lines).
