# Spec: seed-fixes (B11 + OQ-5)

Requisitos del change `fix-v081-skill-seed-oq5`, definidos GIVEN/WHEN/THEN. Spec autoritativa con los S# de `odd/tasks/feat-b11-oq5-seed-fixes.md`.

## Req B11-1 — run_deploy siembra el template pristino de la skill (S11)

**GIVEN** una instalación/upgrade desplegada con `install.sh --version vX.Y.Z` sin wizard previo
**WHEN** `run_deploy` completa (binario, servicio y summary impresos)
**THEN** `<CONFIG_DIR>/templates/recepcionista-negocio/SKILL.md` existe, es idéntico byte a byte al template pristino del asset/repo, con permisos 0600 en el archivo y 0700 en el directorio
**THEN** un fallo del seed imprime un aviso y el deploy NO falla (no fatal), consistente con las otras llamadas no fatales del wizard
**THEN** la skill INSTALADA (`~/.hermes/skills/…`) NO se crea ni se re-renderiza en deploy

## Req OQ5-1 — el wizard pide specialties por profesional (S13)

**GIVEN** el wizard interactivo con servicios ya ingresados (las specialties se piden en una pasada separada AL FINAL de la configuración, después de la sección de servicios — decisión CORREGIDA el 2026-10-21; la inversión de orden que exige S14 vive en el seeder, ver OQ5-2)
**WHEN** se llega a la pasada de specialties y hay profesionales sin respuesta registrada
**THEN** por cada profesional se pregunta `Servicios que atiende <nombre> (números o nombres separados por espacio o comas, Enter = ninguno):` listando los servicios ingresados con índice
**WHEN** la respuesta referencia números o nombres válidos
**THEN** se guardan como nombres exactos en el checkpoint y `setup_staff.json` emite `"specialties": ["<nombre>", …]` sin duplicados
**WHEN** la respuesta contiene una entrada que no valida (índice fuera de rango o nombre inexistente)
**THEN** se re-pregunta sin perder los datos ya cargados del profesional
**THEN** Enter vacío imprime un aviso explícito y emite `"specialties": []` (permitido, decidido por el operador)
**WHEN** el wizard no registró ningún servicio
**THEN** no se pregunta specialties (comportamiento legacy: `[]`)
**THEN** la respuesta queda marcada en el checkpoint: reanudar no vuelve a preguntar la misma pasada respondida

## Req OQ5-2 — el seed resuelve nombres a ids (S14)

**GIVEN** un setup JSON válido cuyas `specialties` calzan exactamente (case-sensitive) con nombres de `setup_services.json`
**WHEN** el boot importa el setup en una DB fresca
**THEN** `professionals.specialties` queda con los ids reales de los servicios insertados en esa misma transacción (services primero, profesionales después)
**WHEN** un profesional lista una especialidad que no calza ningún servicio
**THEN** la importación aborta con error semántico en español que nombra el profesional, la especialidad no encontrada y los nombres válidos
**THEN** el rollback es completo: ni profesionales ni servicios quedan insertados (la DB permanece fresca)
**GIVEN** un setup JSON legacy con `"specialties": []`
**WHEN** importa
**THEN** el profesional se inserta con specialties vacías, sin error ni cambio de comportamiento

## Req VERIF-1 — verificación sin VM (S15)

**THEN** la conducta del wizard y del deploy se verifica con tests bash sintéticos en `scripts/tests/` y los tests Go de `internal/config` usan DB temporal (o mocks); ningún test del change requiere el VM real ni red.
**THEN** los nombres de profesionistas/servicios del caso de la demo B12 (Romina, Santiago, Joaquín, Lavado de Cabello, etc.) viven SOLO en fixtures de test y nunca en código de producción; los fixtures Go usan nombres genéricos claramente de prueba (`Servicio A`, etc.) para que nadie los confunda con producción (aclaración del owner 2026-10-21).
