# Design: fix-v081-skill-seed-oq5

## 1. B11 — seed del pristine en run_deploy

- `run_deploy` (install.sh:1584) agrega, después de `ensure_dirs` (cuando `CONFIG_DIR` ya existe) y antes de `verify_installation`:
  ```bash
  seed_skill_pristine   # nueva función, no fatal sobre fallo total
  ```
- `seed_skill_pristine`: llama `resolve_skill_template` (usa `EXTRACT_DIR` del runtime descargado, con fallback al repo) y `seed_config_template "$template"`. Ambas existen y son idempotentes (sobrescriben el pristino). Cualquier fallo imprime un aviso y continúa (el deploy binario no puede fallar por esto), PERO se loggea — lo producible para el smoke es un summary line "Skill template: sembrado / ausente".

- **No** se invoca `install_receptionist_skill` en deploy: ese paso renderiza con `store_get business.name`, que solo el wizard llena; y el diff de una skill instalada ya existe en `~/.hermes/skills/` sea por wizard o TUI cascade.

## 2. OQ-5 — wizard emite specialties reales

### 2.1 Orden del wizard

`run_setup` pasa a `run_services_section` **antes** de `run_staff_section` (misma lógica de checkpoint: `store_has`/continuations por índice funcionan igual en cualquier orden, son espacios de keys disjuntos `services.N.*` vs `staff.N.*`). `revalidate_all` conserva la invbilidad por sección por key-prefix, no por orden de UI.

### 2.2 Prompt specialties (dentro de run_staff_section)

Por profesional, después del horario:
1. Si la tienda no tiene servicios (`store_has services.0.name` falla) → no se pregunta (vacío, como hoy).
2. Se lista numeración: `(1) Lavado de Cabello (2) Corte de Cabello ...` por cada `services.N.name`.
3. Prompt: `Servicios que atiende <nombre> (números o nombres separados por espacio/comas, Enter = ninguno): `
4. Se parsea y normaliza a **nombres exactos** (input números → lookup por índice; irrelevantes/inválidos → re-prompt; duplicados → se filtran).
5. Enter vacío = sin specialties (aviso explícito en el prompt; la relación va legible por operador después con `update_professional` si quedara vacío).
6. Se guarda en `staff.$i.spec.$j` (índice j dentro de la lista de nombres elegidos) y va al checkpoint; `render_setup_staff` lo renderiza como JSON con los **nombres**.

### 2.3 Contrato setup_staff.json

- `specialties` mantiene el tipo `[]string`, pero pasa a colores: **nombres de servicios** (case-sensitive exact match). Los JSONs existentes con `[]` siguen funcionando.
- `internal/config/setup_types.go`: comentario del campo pasa de "wizard always emits []" a "service names, resolved to IDs by the seeder".

### 2.4 Seeder Go (setup_seeder.go)

En `seed()`, reordenar dentro de la MISMA transacción:
1. Insertar todos los servicios primero, recogiendo `name → id` en un mapa.
2. Insertar profesionales; para cada uno, mapear `m.Specialties` (nombres) → ids:
   - nombre no presente en el mapa → `return fmt.Errorf("la especialidad %q del profesional %q no corresponde a ningún servicio de setup_services.json (servicios válidos: %s)", nombre, m.Name, strings.Join(nombres, ", "))` → rollback total.
3. Repetir el `stringSliceToJSON` ya existente sobre los ids resueltos.

### 2.5 Validación

- `validateForSeed`: sin cambios (los nombres pasan; `prof.Validate()` no los inspecciona).
- La validación de existencia pasa al punto único donde el mapa ya existe (dentro de `seed()`), fail-closed y con rollback.

## 3. Tests

| Test | Archivo | Qué cubre |
|---|---|---|
| Deploy siembra template | `install_deploy_test.sh` | después de un deploy con fixture completa, `templates/$SKILL_SLUG/SKILL.md` existe, es copia del pristine y tiene 0600/0700 |
| Deploy no renderiza skill | idem | `~/.hermes/skills/` intacto (o no creado) en deploy |
| Wizard specialties | `install_e2e_test.sh` / `install_validators_test.sh` | multi-select válido, inválido re-prompt, vacío permitido, duplicado filtrado; el JSON emitido lleva los nombres |
| Seeder resuelve nombre→id | nuevo test Go de integración config | especialidades en DB = ids reales de services; desconocido → error semántico, DB intacta (rollback) |
| JSON legacy | setup_loader_test.go | `specialties: []` importa sin especialidades y sin error |

## 4. Riesgos

- El cambio de orden del wizard rompe checkpoints viejos a mitad de tabla (revisar: son efímeros, una instalación incompleta; se documenta el reset R/S/Q).
- Respuestas del LLM (Hermes) no tocan el wizard.
- El nuevo contracto (nombres) solo toma efecto en instalaciones nuevas (seed en DB vacía); hosts existentes no re-seedeameamean (seed es no-op cuando la DB ya está configurada, setup_seeder.go:24-26).

## 5. Forecast de review

- `scripts` + `internal/config` + tests: ~250-400 líneas changed. Code lane → PR → review nativo (cambia contrato de seed de instalaciones nuevas).
