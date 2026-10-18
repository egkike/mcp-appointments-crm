# Spec: skill-inode-guard

## ADDED Requirements

### Requirement: La regeneración con consentimiento detecta un reemplazo externo del archivo

`admin.WriteSkillFileConsent` MUST, bajo el advisory lock y antes del temp+rename, comparar el inode del PATH con el inode del fd abierto (`os.Stat(path)` vs `f.Stat()`). Si difieren (ambos no-cero), MUST abortar con un semántico en español equivalente al "cambió después del consentimiento" (el caller re-entra el ciclo detect-confirm con bytes frescos) y MUST NOT reemplazar el archivo. Con inodes cero (filesystems sin semántica de inode) el guard NO aplica y el flujo continúa igual que hoy.

#### Scenario: rename externo entre open y rename es reagendado, no perdido

- GIVEN una skill instalada A y un consent snapshot de A capturado por el fd
- WHEN un escritor externo reemplaza la ruta con B vía rename (swap de inode) mientras el flujo sostiene el fd de A
- THEN el guard detecta el mismatch de inode, aborta SIN reemplazar, y los bytes B persisten intactos en la ruta

#### Scenario: no-cooperante write-in-place sigue siendo detectado por el compare

- GIVEN un escritor externo que escribe IN-PLACE sobre la ruta después del open
- THEN el compare por contenido bajo lock ya lo detecta (comportamiento existente r20-r22) — el guard no lo duplica

### Requirement: la atribución de la modificación nombra a Hermes

La pantalla de confirmación del regenerado (TUI y consola) MUST atribuir la modificación detectada: "Puede tratarse de una modificación de Hermes (procedural memory) o manual." — misma constante compartida por ambas presentaciones.
