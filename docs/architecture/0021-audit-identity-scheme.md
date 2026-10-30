# ADR-0021: Esquema de identidad de auditoría — digest keyed (HMAC-SHA-256) del caller id

- **Status**: accepted
- **Date**: 2026-10-30
- **Authors**: Kike
- **Related**: ADR-0002 (rutas XDG / platform-native), ADR-0009 (modelo de autorización, `X-Caller-Id`), ADR-0012 (`~/.config/mcp-appointments-crm/caller-id`), ADR-0013 (composition root único), ADR-0016 (TUI de administración), issue #155

## Context

La tabla append-only `accounts_audit` persiste, por cada mutación de cuenta, una
referencia opaca al actor: la columna `actor_ref`. Desde `feat-tui-ops-residuals` esa
referencia es `auth.HashCallerID(id)`, es decir **SHA-256 sin clave** del caller id,
hex-encodeado (64 caracteres hex minúsculas). El mismo digest aparece en el log
operativo (`caller_hash`) que emite el middleware de autenticación.

El problema es de entropía, no de criptografía: los caller id son teléfonos E.164 o
emails, un espacio de ~10^10–10^11 valores con prefijo de país/negocio conocido. Un
digest sin clave es una función pública del id: cualquiera que obtenga la tabla de
auditoría (backup, export, `scp` del `.db`, una copia en un ticket) puede enumerar ese
espacio offline —GPU o incluso CPU— y recuperar el teléfono de cada actor. El digest
es **pseudonimización, no anonimización**. El propio comentario de `HashCallerID`
reconocía la desviación y la condicionaba a "mientras el log no salga del host"; el
owner cerró la decisión el 2026-10-30: no se espera al primer export, se corrige el
esquema.

Restricciones duras del contexto:

1. **La tabla es append-only por diseño** (triggers de inmutabilidad en
   `internal/db/schema.go`): no se puede reescribir una fila histórica.
2. **El plaintext detrás de un digest histórico no existe** en ningún lado: la fila
   guarda el hash, no el teléfono. Un "re-hash" es imposible por construcción, no por
   falta de tiempo.
3. **El digest es display-only.** No es un identificador de autorización: el RBAC se
   resuelve por `accounts`/`clients` con el caller id en claro en la request. Romper la
   comparabilidad histórica degrada la lectura humana del audit trail, nunca una
   decisión de acceso.
4. La instalación ya tiene un directorio de configuración de operador con material de
   identidad propio (`~/.config/mcp-appointments-crm/caller-id`, 0600, ADR-0012), y el
   servidor es loopback-only (127.0.0.1), single-tenant.

## Decision

### Decisión 1 — El `actor_ref` es HMAC-SHA-256 con una clave de instalación

`actor_ref = hex(HMAC-SHA-256(key_instalación, caller_id))`, 64 caracteres hex
minúsculas. El espacio de claves (2^256) elimina la enumeración offline: sin el archivo
de clave, el digest no es una función computable del id.

La **forma en disco no cambia** (64 hex minúsculas), y eso es deliberado: la columna
`actor_ref` es `TEXT`, el validador del sink (`auditActorRefRe`) y todos los lectores
siguen intactos. El cambio es de esquema criptográfico, no de esquema de datos: **cero
DDL, cero migración, cero backfill**.

### Decisión 2 — La clave vive en un archivo auto-generado del config dir (0600)

- **Ubicación**: `~/.config/mcp-appointments-crm/audit-hash-key` — al lado de
  `caller-id`, porque ambos son material de identidad de la misma instalación. La ruta
  la resuelve `internal/admin.ConfigDir()` (override `MCP_CONFIG_DIR`, default
  platform-native: `~/.config/mcp-appointments-crm` en Linux,
  `~/Library/Application Support/MCP Appointments CRM` en macOS). No se agrega una
  variable de entorno nueva: `MCP_CONFIG_DIR` ya es el seam de test/instalación.
- **Formato**: 32 bytes crudos de `crypto/rand`, hex-encodeados como 64 caracteres
  minúsculas + `\n`. El lector tolera newlines finales (`\n`, `\r\n`, repetidos) y
  **rechaza** cualquier otra cosa (longitud distinta, mayúsculas, no-hex, bytes crudos)
  con un `*domain.SemanticError`.
- **Permisos**: archivo 0600 (chmod explícito tras la escritura), directorio 0700 —
  mismo criterio que `admin.WriteCallerID`. En Windows no hay bits de modo que
  inspeccionar (`os.FileMode` sólo lleva el atributo read-only), así que el mismo
  contrato se **enforcea sobre la DACL efectiva** de la clave y de su config dir
  (`ensureOwnerOnlyWindows`, ver "Gate de ACL en Windows").
- **Creación única**: `O_CREATE|O_EXCL`. Si el server (systemd) y la TUI del operador
  arrancan a la vez, el perdedor de la carrera **lee la clave del ganador** (con
  reintentos acotados) en vez de sobrescribirla: la clave *es* el namespace del digest,
  regenerarla invalidaría todo el audit trail histórico.
- **Fail closed**: si el directorio no se puede crear, el archivo no se puede leer o la
  clave es inválida, el arranque falla (`exit 1`). No hay clave por defecto, ni
  fallback en memoria, ni override por env var. Un proceso que no puede keyear su
  digest no sirve requests ni escribe cuentas.
- **Sin rotación automática.** Ver Decisión 4.

### Decisión 3 — Configuración única en el boot, un solo namespace por proceso

`main()` carga (o crea) la clave e instala el digest en el paquete `auth`
(`auth.ConfigureAuditDigest`) **antes del dispatch de comandos**: serve mode,
`admin tui` y el reservado `hermes chat` heredan el mismo digest. Los dos consumidores
—el middleware de auth (log `caller_hash`) y el emisor del repositorio de cuentas
(`actor_ref`)— leen ese digest; ninguno de los dos puede degradar al SHA-256 sin clave.

**Fail closed en runtime**: si por un bug de wiring un camino llega sin digest
configurado, la referencia se **omite** (el log de auditoría y la fila se emiten
igual) y la degradación se registra como `ERROR` con `scheme=hmac-sha256`. Nunca se
emite el digest sin clave "como si nada", y nunca se hace panic dentro de un request.
En la fila persistida, omitir `actor_ref` es preferible a dropear la fila: una
referencia faltante es un problema de calidad del audit trail, perder la fila es un
problema de completitud.

### Decisión 4 — Ventana de esquemas mixtos y semántica de rotación

- **Filas existentes**: quedan como están. Las filas previas llevan SHA-256 sin clave;
  las nuevas llevan HMAC-SHA-256 keyed. Ambas son 64 hex minúsculas y conviven en la
  misma columna sin distinción estructural (ventana mixta documentada en
  `internal/repository/accounts_audit.go`). No se re-hashea (imposible), no se borra,
  no se migra.
- **Rotación = namespace nuevo.** Cambiar (o perder) la clave hace que los digests
  históricos dejen de ser comparables con los nuevos: el mismo teléfono produce otro
  `actor_ref`. La correlación histórica se preserva **solo si la clave se conserva**;
  por eso la rotación es una **excepción operativa deliberada** (sospecha de
  compromiso de la clave o del host), no una tarea periódica:
  1. detener el servicio,
  2. archivar la clave anterior junto con el backup de la DB (fuera de línea),
  3. borrar/renombrar el archivo, arrancar (se genera una clave nueva),
  4. registrar en el log de operaciones la fecha de corte del namespace.
  No hay re-keying de filas históricas.
- **Backup**: `scripts/backup.sh` respalda la DB, no el config dir. Restaurar un backup
  en una instalación nueva (clave nueva) deja los `actor_ref` históricos
  **no correlacionables** con caller ids actuales. Es una consecuencia aceptada y
  documentada, no un bug; para auditar hacia atrás hay que conservar la clave del
  período junto al backup.

### Decisión 5 — El `actor_ref` sigue siendo display-only

El digest nunca es un identificador de autorización ni una clave de lookup de negocio:
el RBAC resuelve por `accounts`/`clients` con el caller id en claro de la request
(ADR-0009). Los lectores del audit trail muestran `actor_ref` como referencia de
correlación. Esto es lo que hace que la ventana mixta y la rotación sean aceptables:
degradan la lectura, nunca una decisión de acceso.

## Alternatives considered

| Alternativa | Por qué se rechaza |
|---|---|
| **Env var (`MCP_AUDIT_HASH_KEY`)** | Un secreto en el entorno se filtra por `Environment=` del unit, `ps e`, `/proc/<pid>/environ`, dumps de systemd y el historial del shell del operador. El archivo 0600 bajo el config dir tiene un dueño y un modo verificables. Además el owner decidió explícitamente "no env override por ahora". |
| **SHA-256 + pepper guardado en la DB** | El pepper en la misma DB que las filas no agrega barrera: quien lee la tabla lee el pepper. La clave debe vivir en un almacén distinto del dato que protege (defense in depth). |
| **SHA-256 sin clave (status quo)** | Es exactamente el problema: pseudonimización enumerable offline sobre un espacio de ~10^11 teléfonos. |
| **Re-hash / backfill de filas históricas** | Imposible: el plaintext no se guardó. Sólo existe migración hacia adelante, que es la ventana mixta ya decidida. |
| **Cifrado del `actor_ref` (AES-GCM) en lugar de HMAC** | El audit trail sólo necesita igualdad/correlación, no reversibilidad. Un MAC da integridad de referencia sin superficie de descifrado; además el formato de 64 hex se conserva sin tocar la columna. |
| **Rotación automática / multi-clave por período** | Multiplica el estado (qué clave usó qué fila) sin un lector que lo necesite hoy; el `actor_ref` es display-only. Queda como follow-up si aparece la necesidad real de correlación cross-período. |

## Consequences

**Positive**:

- La enumeración offline del espacio de caller ids deja de ser posible con la tabla (o
  los logs) en la mano: hace falta la clave, que vive 0600 en el host.
- **Cero migración**: mismo tipo de columna, misma validación del sink, mismos lectores,
  misma TUI. El cambio es de esquema criptográfico, no de datos.
- Un solo punto de configuración en el boot (composition root, ADR-0013) cubre serve
  mode y todos los sub-comandos: no hay comandos que puedan escribir auditoría con un
  digest distinto ni sin digest.
- Fail closed explícito en dos capas: arranque fatal si no hay clave, degradación
  visible (referencia omitida + `ERROR`) si un camino llega sin digest.

**Negative**:

- **Nueva pieza de estado operativo**: perder `audit-hash-key` no rompe el server (se
  genera otra), pero corta la correlación histórica. Backups de la DB sin la clave del
  período son auditables sólo como "filas de un namespace previo".
- **Ventana mixta permanente**: mientras existan filas previas al corte, la tabla
  contiene dos esquemas indistinguibles a simple vista. No hay columna de esquema
  (agregarla habría exigido migración); el corte se documenta por fecha en el log de
  operaciones.
- **Rotación manual y disruptiva**: es una excepción, no un proceso periódico, y
  requiere archivar la clave anterior para poder auditar el pasado.
- La clave es un secreto más del host: si el atacante ya puede leer el config dir del
  operador, el digest keyed no aporta defensa adicional (sí la aporta frente al
  escenario realista: export/backup/copia de la DB o del log).
- El comentario del campo `ActorRef` en `internal/domain/entity/account_audit.go` sigue
  nombrando `auth.HashCallerID` como "hex(sha256)": la función mantiene el nombre y la
  forma, pero el esquema real ahora es keyed (deuda de comentario, fuera del alcance de
  este change).

## References

- Issue: #155 "Keyed digest (HMAC) for caller-id hashes in the audit log";
  #158 "Windows ACL enforcement for the audit-hash-key file and its config dir".
- Código: `internal/auth/audit_digest.go` (clave + digest), `internal/auth/middleware.go`
  (`caller_hash` fail-closed), `internal/repository/accounts.go` (`actor_ref`),
  `internal/repository/accounts_audit.go` (ventana mixta), `cmd/mcp-server/main.go`
  (`configureAuditDigest`, boot).
- Tests: `internal/auth/audit_digest_test.go` (vector pineado, carga/creación de clave),
  `internal/auth/middleware_test.go`, `internal/repository/accounts_audit_test.go`.
- ADR-0002 (rutas XDG), ADR-0009 (autorización), ADR-0012 (`caller-id`), ADR-0013
  (composition root), ADR-0016 (TUI admin).

### Decisión 6 — el journal operativo comparte la regla de máscara (2026-10-30, GGA round 1)

Las líneas de `slog` que el server escribe al journal (`actor_id`, `target_id`, `previous_owner_id`) llevan la misma forma `MaskPhone` (`***<últimos 4>`) que las columnas persistidas: el journal es un log durable con lectores no acotados a la sesión TUI, así que la regla "no PII cruda" aplica a toda la salida de auditoría, no solo a la tabla. La relación nace de `entity.MaskPhone` (entidad), se aplica en `AccountsRepo.auditAttrs` + la línea de transfer, y los pines de slog del repo afirman la forma enmascarada.

Consecuencia: un operador que necesite correlacionar una línea del journal con una cuenta hace la consulta vía DB (que guarda la fila con `actor_ref` keyed o `target_id` enmascarada + el detail estructurado), no leyendo el journal a ojo.

### Detección de permisos en el keyfile (2026-10-30, GGA round 1)

La lectura también valida permisos (GGA round 1-2): un `audit-hash-key` no-0600 (ej. 0644 por umask suelta) o un dir de config **group/world-writable** hacen boot con error semántico que nombra el chmod. El dir gate rechaza group-write también porque un group member puede **reemplazar** el key file (delete + rewrite por el directorio) pese al 0600 del archivo — un takeover del namespace, no una conveniencia. Los homes reales tienen `~/.config` owner-write-only (0755), así que el gate no cuesta nada en producción; las fixtures de test explicitan chmod 0700 porque `t.TempDir()` de este ambiente es 0775 (umask 0002).

Esos dos chequeos viven desde #158 en `audit_digest_other.go`, detrás de `auditHashKeyPermissionGate`; su semántica y sus mensajes no cambiaron un byte.

### Gate de ACL en Windows (2026-10-31, issue #158 — retira el residual de `review-5f5aafb87aa13e6f`)

El residual que el validador TV-R1 rechazó era exacto: en Windows no se evaluaba **ningún** permiso de la clave, mientras un comentario asumía una "ACL default de user-profile" que nadie verificaba. Un key file en un path cuya ACL permite a otros usuarios leerlo o reemplazarlo se aceptaba sin chequeo. Este apartado lo remueve.

La lectura valida acceso en **todas** las plataformas: el verdicto vive detrás de `auditHashKeyPermissionGate`, que en POSIX son los chequeos de modo ya descritos y en Windows es `ensureAuditHashKeyOwnerOnly` (`audit_digest_windows.go`).

`ensureAuditHashKeyOwnerOnly` lee el security descriptor efectivo (`GetNamedSecurityInfo` sobre `SE_FILE_OBJECT` con `DACL|OWNER`) de la clave y de su config dir, toma el owner SID (`GetSecurityDescriptorOwner`) y falla cerrado cuando algún ACE `ACCESS_ALLOWED` cuyo principal **no** es el owner, ni `SYSTEM` (`S-1-5-18`), ni `BUILTIN\Administrators` (`S-1-5-32-544`) concede lectura o escritura. Máscaras: lectura = `FILE_GENERIC_READ|FILE_READ_DATA|READ_CONTROL`; escritura = `FILE_GENERIC_WRITE|GENERIC_WRITE|GENERIC_ALL|WRITE_DAC|WRITE_OWNER|DELETE|FILE_DELETE_CHILD`. `DELETE`/`FILE_DELETE_CHILD` cuentan como escritura por el mismo razonamiento que el gate POSIX de group-write: borrar + recrear por el directorio es un takeover del namespace. Ojo con `0x20000`: en Windows `STANDARD_RIGHTS_READ == STANDARD_RIGHTS_WRITE == READ_CONTROL`, así que ese bit solo dispara **ambos** gates; es la dirección fail-closed y se acepta documentadamente en vez de inventar un desempate.

Los dos targets (dir, archivo) se evalúan en la **misma** llamada y los problemas se reportan juntos: el mensaje nombra objeto (archivo vs dir), permiso (lectura / escritura / ambas), tipo de ACE (`ACCESS_ALLOWED`, o `DACL ausente` cuando no hay lista que evaluar) y principal (SID en forma SDDL, sin lookup de dominio), y da el fix `icacls "<path>" /inheritance:r /grant:r "%USERNAME%:F"` — el equivalente Windows del `chmod 0600` (el binario nunca reescribe ACLs por su cuenta). Cualquier falla de acceso o de evaluación también falla cerrado: "no se pudo evaluar" nunca significa "es seguro".

Simplificaciones declaradas frente al outline del issue (no silenciosas):

- se evalúan **sólo** la clave y su config dir inmediato; no se camina el árbol de hijos (ningún otro archivo se lee en este camino);
- **no** se expanden las membresías de grupo del owner: se inspecciona la DACL, no se corre un access check contra el token del owner, así que un ACE a un grupo al que el owner pertenece cuenta como "otro principal";
- un DACL ausente (`ERROR_OBJECT_NOT_FOUND`) se reporta como exposición total; `x/sys` no distingue de forma confiable un DACL ausente de uno vacío (`nil`), así que ambos fallan cerrado;
- ACEs de tipo distinto de `ACCESS_ALLOWED`/`ACCESS_DENIED` (object/callback ACEs) abortan la evaluación como "no soportado" en lugar de reinterpretarse con el layout equivocado;
- el camino de **creación** no se gatea (el archivo todavía no existe y `os.Chmod` en Windows no toca ACLs): la lectura del arranque siguiente aplica el verdicto.

Evidencia: `internal/auth/audit_digest_windows_test.go` fija la decisión ACE con security descriptors canned en SDDL — `(A;;FR;;;WD)` → lectura de `S-1-1-0`, `FA`/`FW`/`WD`/`SD` → escritura, `(A;OICI;FA;;;WD)` → lectura+escritura sobre el dir, ACEs `deny` ignorados, DACL ausente → exposición, owner/SYSTEM/Administrators tolerados — y la forma del mensaje. Límite de evidencia declarado: ese archivo **compila** para `GOOS=windows` en amd64 y arm64 y corre sólo en Windows; el job de CI Linux sobre el que se validó esta unidad **no ejecuta** `GetNamedSecurityInfo` ni ningún test de ese archivo. El gap de runtime es real y no se reclama como cobertura.

El pin ejecutable multiplataforma es el lado POSIX: `TestReadAuditHashKeyFile_RoutesEveryReadThroughThePermissionGate` prueba que la lectura consulta el gate de la plataforma (con un gate permisivo, un keyfile 0644 se acepta), y `TestReadAuditHashKeyFile_PropagatesGateRejection` que el verdicto se propaga tal cual sin entregar la clave. Eso es exactamente la regresión que el validador marcó: un gate que no produce verdicto.

### Atributos tipados del journal (2026-10-30, GGA round 3)

Los attrs de auditoría (`auditAttrs` en auth y en repository) devuelven `[]slog.Attr` — la forma del std-library que le deja al compilador el par clave/valor, sin la convención posicional previa; los call-sites de `slog` elevan con un lift explícito. El wire del journal NO cambia un byte (los pines del journal lo afirman).

### Redondeos GGA del gate de ACL (rounds 1-2 del into #158, 2026-10-30)

- Verbatim fix del round 1: `GENERIC_READ` entra en la máscara de lectura (un ACE genérico-grant no escapa más), el **create path ya está gated** llama al gate de plataforma tras el chmod (un DACL heredado permisos-productores de Windows o una POSIX mode loose falla el First boot), y los mensajes de remediación son **path-free** (el path viaja solo en `Cause` al log del server).
- Layer 2 (el round 2): ancla del **usuario del proceso** — el owner SID del objeto ya NO es la ancla de tolerancia (un owner extranjero podía esconder sus propios grants); el anchor es el SID del token actual (`OpenCurrentProcessToken().GetTokenUser()`), válidos siempre SYSTEM/Administrators. POSIX agrega **Lstat** (los symlinks del key/dir se rechazan, nunca se sigue el salto) y **ownership** (`stat_t.Uid == euid`).
- Pendiente verificado: la mask 0x20000 sigue tripando ambos gates (fail-closed).

### Boundary del gate de ACLs y ancestors (2026-10-30, cierre del bullet #158)

El gate cubre **el key file + su config dir inmediata** (declarado como desviación del outline original, en el code comment de `audit_digest_windows.go` y en el body del change). Los ancestros ARRIBA del config dir (`~/.config`, `$HOME`, o el path base que el operador setee por `MCP_CONFIG_DIR`) quedan FUERA del verdict del boot: validar la cadena completa infla el scope del issue hacia un problema de despliegue (un ancestro user-writable es una misconfiguration del operador, no un defecto del gate — el gate falla cerrado ante lo que DINÁMICamente puede evaluar). Si una revisión nativa eleva el ancestor-chain como blocker: el rebalance pertenece a un candidate SEPARADO con AMBOS los lados documentando el depósito del operador y los helpers de resolución del config path.
