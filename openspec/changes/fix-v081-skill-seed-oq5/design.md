# Design: fix-v081-skill-seed-oq5

## 1. B11 — seed del pristine en run_deploy

- `run_deploy` (install.sh:1584) agrega, después de `ensure_dirs` (cuando `CONFIG_DIR` ya existe) y antes de `verify_installation`:
  ```bash
  seed_skill_pristine   # nueva función, no fatal sobre fallo total
  ```
- `seed_skill_pristine`: llama `resolve_skill_template` (usa `EXTRACT_DIR` del runtime descargado, con fallback al repo) y `seed_config_template "$template"`. Ambas existen y son idempotentes (sobrescriben el pristino). Cualquier fallo imprime un aviso y continúa (el deploy binario no puede fallar por esto), PERO se loggea — lo producible para el smoke es un summary line "Skill template: sembrado / ausente".

- **No** se invoca `install_receptionist_skill` en deploy: ese paso renderiza con `store_get business.name`, que solo el wizard llena; y el diff de una skill instalada ya existe en `~/.hermes/skills/` sea por wizard o TUI cascade.

## 2. OQ-5 — wizard emite specialties reales

### 2.1 Pasada de specialties al final del wizard (CORREGIDO 2026-10-21)

> La primera redacción invertía el orden de las secciones del wizard (services antes que staff). Se mantiene el orden canónico `business → hours → staff → services` y las specialties se piden en una **pasada separada después de la sección de servicios**, porque: (a) cumple S13 (el wizard pregunta specialties por profesional), (b) minimiza la rotura de los ~15 flujos canned de `scripts/tests/install_e2e_test.sh` y del contrato de checkpoint, y (c) el orden del SEED (services antes de staff) que exige S14 se resuelve en el seeder Go, donde vive el requisito real (resolución nombre→id).

`run_setup` agrega, después de `run_services_section`:

```bash
run_staff_specialties_pass   # pregunta specialties a cada profesional sin responder
```

La pasada itera `store_has "staff.N.name"` y por cada profesional pide (idempotente vía marcador `staff.N.specset`, verificado en checkpoint/reanudación):
1. Si no hay servicios (`store_has "services.0.name"` falla) → no pregunta (marcador automático, legacy `[]`).
2. Lista numerada `(N) nombre` y prompt: `Servicios que atiende <nombre> (números o nombres separados por espacio o comas, Enter = ninguno): `.
3. Parseo por coma y por espacio: números → nombre por índice (base `10#`), nombres exactos case-sensitive los fragmentos no numéricos; entrada inválida → re-pregunta sin perder datos; duplicados (por nombre ya visto) → ignorados preservando orden.
4. Enter vacío → aviso explícito ("podés cargarlas después con update_professional") y relación vacía (permitido para el operador, no es barrera).
5. Nombres en `staff.N.spec.J` consecutivos + marcador + `checkpoint_save` al confirmar.

Sin validador para `staff.*.spec.*` en `validator_for_key`: los nombres ya se validan al pedirlos; `revalidate_all` los deja intactos.

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

- La pasada de specialties a mitad de un checkpoint viejo re-pregunta a los profesionales sin marcador `specset` (efímero: es una instalación incompleta; el reset R/S/Q igual la borra).
- Respuestas del LLM (Hermes) no tocan el wizard.
- El nuevo contracto (nombres) solo toma efecto en instalaciones nuevas (seed en DB vacía); hosts existentes no re-seedeameamean (seed es no-op cuando la DB ya está configurada, setup_seeder.go:24-26).

## 5. Forecast de review

- `scripts` + `internal/config` + tests: ~250-400 líneas changed. Code lane → PR → review nativo (cambia contrato de seed de instalaciones nuevas).
