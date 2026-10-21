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
| T2 | S11 | code | run_deploy: `seed_config_template` + test en install_deploy_test.sh + shellcheck |
| T3 | S13/S14 | code | wizard: secciones invertidas (services → staff) + prompt specialties multi-select + render |
| T4 | S13/S14 | code | seeder Go: inserta services primero, resuelve nombre→id fail-closed, error semántico con nombres válidos |
| T5 | S15 | test | tests bash sintéticos (setup/skill/deploy) + tests Go config (RED→GREEN) |
| T6 | S16 | tests | regresión: JSON legacy "" importa igual; deploy no render skill |
| T7 | S11-S15 | docs | PRD (contrato setup JSON), PRD tool count intacto, changelog v0.8.1 |
| T8 | S11-S15 | release | pipeline + PR + patch release v0.8.1 tras merge |

## Log

- **L1** 2026-10-21 — owner: "Hola capo, veamos el cuadro de pendientes del proyecto para luego ver cómo seguimos" → cuadro presentado (B11, B12 apply, B6 pasivo). Owner elige "B11 + OQ-5 juntos". Exploración: `run_deploy` (install.sh:1584-1698) no llama seed de template; `seed_config_template`/`resolve_skill_template` ya reutilizables (:1044-1106); `render_setup_staff` hardcodea `"specialties": []` (:930); `seed()` inserta staff antes de services (setup_seeder.go:151-184); `prof.Validate()` no inspecciona specialties (professional.go:50-58) → nombres pueden atravessar `validateForSeed` y resolverse dentro de la transacción. Doc-first creado.
