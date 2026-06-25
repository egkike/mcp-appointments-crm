# PRD: <Nombre del Producto / Feature>

> **Estado**: [Borrador | En Revisión | Aprobado | En Implementación | Completado]
> **Owner**: <rol o nombre>
> **Versión**: 1.0
> **Última actualización**: <YYYY-MM-DD>

---

## 1. Contexto y Problema

### 1.1 Contexto

<2-4 párrafos: qué está pasando en el producto/mercado que motiva este feature. Incluye señales de usuarios, tendencias, datos de uso, cambios en el entorno competitivo o técnico.>

### 1.2 Problema

<1-2 párrafos: qué dolor o necesidad NO estamos resolviendo hoy. Cuantificá el impacto si es posible (usuarios afectados, tiempo perdido, revenue perdido, etc.).>

### 1.3 Solución Propuesta

<1 párrafo de alto nivel: qué vamos a hacer para resolver el problema. Esta sección es un resumen ejecutivo — el detalle va en §5.>

---

## 2. Objetivos y Métricas de Éxito

### 2.1 Objetivos (SMART)

- [ ] **O1**: <objetivo específico, medible, alcanzable, relevante y con plazo>
- [ ] **O2**: <objetivo>
- [ ] **O3**: <objetivo>

### 2.2 KPIs

| Métrica | Baseline | Target | Plazo |
|---------|----------|--------|-------|
| <nombre de la métrica> | <valor actual> | <valor objetivo> | <fecha> |
| <nombre de la métrica> | <valor actual> | <valor objetivo> | <fecha> |

---

## 3. Alcance

### 3.1 In Scope

- <qué SÍ vamos a hacer — bullets concretos y verificables>
- <...>

### 3.2 Out of Scope

- <qué NO vamos a hacer — explícitamente. Importante para evitar scope creep.>
- <...>

### 3.3 Asunciones

- <suposiciones que estamos haciendo (sobre usuarios, tech, negocio, equipo). Si alguna cae, hay que replanificar.>
- <...>

### 3.4 Approach Técnico (alto nivel)

- <patrón o estrategia principal, p.ej. "Repository pattern con prepared statements">
- <decisiones arquitectónicas clave, p.ej. "WAL mode + busy_timeout=5000 para alta concurrencia">
- <tradeoffs principales, p.ej. "modernc.org/sqlite (pure Go, sin CGo) a cambio de binario más grande">
- <dependencias externas o librerías candidatas con justificación>

### 3.5 Affected Areas

- `<path/area>` — <qué cambia en este módulo o paquete>
- `<path/area>` — <qué cambia en este módulo o paquete>
- `openspec/specs/<domain>/` — <delta specs que se crean/modifican>
- `openspec/changes/<fase>/` — <carpeta de la fase en el SDD workflow>

### 3.6 Rollback Plan

- **Estrategia**: <revert commit / feature flag / migration reversa / branch de respaldo>
- **Tiempo estimado de rollback**: <minutos/horas, p.ej. "<5 min con revert commit">
- **Riesgo residual si rollback falla**: <qué queda en estado inconsistente>

---

## 4. Usuarios y Personas

### 4.1 Personas

| Persona | Necesidad principal | Frecuencia de uso |
|---------|---------------------|-------------------|
| <nombre del arquetipo> | <qué busca / qué dolor tiene> | [Diaria / Semanal / Mensual / Esporádica] |
| <...> | <...> | <...> |

### 4.2 User Stories de Alto Nivel

- Como **<persona>**, quiero **<acción>**, para **<beneficio>**
- Como **<persona>**, quiero **<acción>**, para **<beneficio>**
- Como **<persona>**, quiero **<acción>**, para **<beneficio>**

---

## 5. Requerimientos

### 5.1 Requerimientos Funcionales (RF)

**RF1: <Nombre descriptivo>**
- **Descripción**: <qué debe hacer el sistema>
- **Prioridad**: [Must | Should | Could | Won't]
- **Criterios de Aceptación** (formato Gherkin):
  - [ ] Dado <contexto>, cuando <acción>, entonces <resultado>
  - [ ] Dado <contexto>, cuando <acción>, entonces <resultado>

**RF2: <Nombre descriptivo>**
- **Descripción**: <...>
- **Prioridad**: <...>
- **Criterios de Aceptación**:
  - [ ] <...>
  - [ ] <...>

**RF3: <Nombre descriptivo>**
- **Descripción**: <...>
- **Prioridad**: <...>
- **Criterios de Aceptación**:
  - [ ] <...>
  - [ ] <...>

> **Nota**: los criterios Gherkin de §5.1 se traducen a `scenarios` en el delta spec
> (`openspec/changes/<fase>/specs/<domain>/spec.md`) usando el formato
> `### Requirement: <nombre>` + `#### Scenario: <nombre>`. La prioridad
> `[Must | Should | Could | Won't]` se mapea a palabras clave RFC 2119
> (MUST / SHOULD / MAY) en el spec.

### 5.2 Requerimientos No Funcionales (RNF)

| Categoría | Requerimiento | Métrica / Target |
|-----------|---------------|------------------|
| Performance | <descripción> | <p95 / p99 / throughput> |
| Seguridad | <descripción> | <estándar o control aplicable> |
| Escalabilidad | <descripción> | <N usuarios concurrentes / N req/s> |
| Disponibilidad | <descripción> | <% uptime / SLA> |
| Usabilidad | <descripción> | <tiempo de onboarding / learning curve> |
| Mantenibilidad | <descripción> | <% cobertura de tests / deuda técnica> |
| Observabilidad | <descripción> | <logs / métricas / traces> |
| Compliance | <descripción> | <regulación aplicable> |

---

## 6. Restricciones Técnicas

### 6.1 Stack Técnico

- **Backend**: <tecnología + versión, p.ej. Node.js 22 + Express + TypeScript>
- **Frontend**: <tecnología + versión, p.ej. React 19 + Astro + Tailwind>
- **Base de Datos**: <motor, p.ej. PostgreSQL 16 + pgvector>
- **Infraestructura**: <Docker, Redis, BullMQ, etc.>
- **Otros**: <auth provider, monitoring, CDN, etc.>

### 6.2 Integraciones Externas

| Sistema | Tipo | Propósito | Criticidad |
|---------|------|-----------|------------|
| <nombre del sistema> | [API / DB / Queue / Webhook] | <para qué se usa> | [Bloqueante / Importante / Opcional] |
| <...> | <...> | <...> | <...> |

### 6.3 Compliance y Seguridad

- **Regulaciones aplicables**: <GDPR, PCI-DSS, LEC, etc.>
- **Datos sensibles manejados**: <PII, financieros, contenido, etc.>
- **Controles de seguridad requeridos**: <rate limiting, RBAC, encryption at rest/transit, audit log, etc.>

---

## 7. Roadmap y Fases

> **Regla**: 1 fase = 1 SDD (openspec/changes/<nombre-de-la-fase>/).

### Fase 1: <Nombre de la fase> (Estimación: [S / M / L])

**Objetivo**: <qué se logra cuando esta fase está cerrada>

**Entregables**:
- <componente / servicio / UI / endpoint>
- <componente / servicio / UI / endpoint>

**Definition of Done**:
- [ ] <criterio verificable, p.ej. "Endpoints documentados en Swagger">
- [ ] <criterio verificable, p.ej. "Cobertura de tests > 80%">
- [ ] <criterio verificable, p.ej. "Aprobado en code review + pasa CI">

### Fase 2: <Nombre de la fase> (Estimación: [S / M / L])

**Objetivo**: <...>

**Entregables**: <...>

**Definition of Done**:
- [ ] <...>
- [ ] <...>

### Fase 3: <Nombre de la fase> (Estimación: [S / M / L])

**Objetivo**: <...>

**Entregables**: <...>

**Definition of Done**:
- [ ] <...>

### Fase N: <Nombre de la fase> (Estimación: [S / M / L])

**Objetivo**: <...>

**Entregables**: <...>

**Definition of Done**:
- [ ] <...>

---

## 8. Riesgos y Dependencias

### 8.1 Riesgos

| # | Riesgo | Probabilidad | Impacto | Mitigación |
|---|--------|--------------|---------|------------|
| R1 | <descripción del riesgo> | [Baja / Media / Alta] | [Bajo / Medio / Alto] | <acción preventiva o de contingencia> |
| R2 | <...> | <...> | <...> | <...> |

### 8.2 Dependencias

| # | Dependencia | Tipo | Estado | Owner |
|---|-------------|------|--------|-------|
| D1 | <sistema / equipo / recurso externo> | [Bloqueante / Paralela] | [Resuelta / Pendiente] | <quién la gestiona> |
| D2 | <...> | <...> | <...> | <...> |

---

## 9. Glosario y Referencias

### 9.1 Glosario

- **<término>**: <definición clara y concisa>
- **<término>**: <definición>
- **<término>**: <definición>

### 9.2 Referencias

- <link o path a doc relacionado, p.ej. SDDs previos, análisis técnicos, dashboards, research>
- <link o path a doc relacionado>
- <link o path a doc relacionado>

---

## 10. Historial de Cambios

| Fecha | Versión | Autor | Cambios |
|-------|---------|-------|---------|
| <YYYY-MM-DD> | 1.0 | <nombre> | Creación inicial |
| <YYYY-MM-DD> | 1.1 | <nombre> | <qué cambió y por qué> |
| <YYYY-MM-DD> | 2.0 | <nombre> | <cambio mayor — refactor de alcance / adición de fase> |
