# ADR-0019: Estrategia de build y pruebas para targets iOS — GitHub Actions runners macOS

- **Status**: accepted (ejecución aplazada)
- **Date**: 2026-10-13
- **Authors**: Kike
- **Related**: ADR-0018 (canales congelados — no hay superficie iOS en el producto), ADR-0014 (release/deploy), ADR-0013 (arquitectura en capas), PRD §7

## Context

**No hay superficie iOS en el producto.** ADR-0018 congeló los canales: los clientes del negocio
escriben por WhatsApp (primario) o Telegram al número del negocio; el owner opera por SSH con
`mcp-server hermes chat` y un bot privado de Telegram; el server corre en la VPS con bind estricto
a `127.0.0.1:3000`. Un usuario con iPhone es un cliente de mensajería o un terminal SSH — el server
no distingue el OS del cliente. La pregunta "¿cómo probamos la instalación en iOS?" nació de una
premisa que el diseño ya descartó, y motivó este registro para cuando el escenario cambie.

**No hay hardware Apple disponible** (ni iPhone ni Mac). Se evaluó
[xtool](https://github.com/xtool-org/xtool) como herramienta de deploy y no aplica: compila
Swift/SwiftPM (no Go), iOS no permite binarios standalone ni servicios de fondo (no existe
equivalente a systemd/launchd), y una app en background ve suspendido su listener SSE. Además
requiere dispositivo físico para todo su flujo.

**Evidencia empírica** (2026-10-13, Go 1.26.9, linux/amd64, módulo en HEAD):

- `GOOS=darwin GOARCH=arm64|amd64 CGO_ENABLED=0 go build ./...` → **OK**: la historia
  macOS/launchd (AGENTS.md, ADR-0014) se verifica desde Linux sin cgo.
- `GOOS=ios GOARCH=arm64|amd64 CGO_ENABLED=0 go build ./...` → **falla** con
  `ios/arm64 requires external (cgo) linking` / `default PIE binary requires external (cgo)
  linking`: el toolchain Go exige cgo + clang con SDK de Apple. Sin sysroot no hay cross-compile
  desde Linux pelado (instalar clang solo no alcanza).
- Con Xcode + iOS SDK, `GOOS=ios GOARCH=arm64 CGO_ENABLED=1 go build` es un patrón establecido de
  la comunidad sin gomobile (gomobile solo se necesita para envolver Go como librería dentro de
  una app).

**Costo cero disponible**: el repo es público y la facturación de GitHub lo confirma —
*"The use of standard GitHub-hosted runners is free: In public repositories"*
(docs.github.com/en/billing/concepts/product-billing/github-actions, verificado 2026-10-13;
los "larger runners" siempre se cobran, incluso en repos públicos). Los runners macOS estándar
traen Xcode + iOS SDK preinstalados.

## Decision

Cuando exista un requisito real de verificar build/instalación para iOS, la estrategia elegida es
**GitHub Actions con runners macOS estándar, sin hardware Apple propio**:

1. **Compilación iOS**: job con `runs-on: macos-latest` (checkout + setup-go) corriendo
   `GOOS=ios GOARCH=arm64 CGO_ENABLED=1 go build ./...` (device) y opcionalmente
   `GOOS=ios GOARCH=amd64` (simulador). Xcode/iOS SDK ya está presente en el runner; no se
   requiere gomobile para el binario puro.
2. **Simulador** (solo si algún día existe una app iOS nativa): `xcodebuild test` en el mismo
   runner.
3. **Runtime sobre device real**: fuera de alcance. Re-evaluar solo si el producto incorpora una
   app iOS (farms pagas tipo BrowserStack / Sauce Labs / Firebase Test Lab, o compra de device).

La ejecución es **aplazada**: este ADR registra la decisión pero **no agrega el job al CI hoy**.
Mientras no haya superficie iOS (ADR-0018), el CI actual no lo necesita. Al momento de ejecutar:
re-verificar precios/políticas de runners y la versión de Xcode disponible en el runner.

Nota colateral registrada: proteger el build `darwin/arm64` + `darwin/amd64` en el CI Linux
(sin cgo, ya verificado que pasa) es una brecha independiente de esta decisión, pendiente de
decisión propia.

## Consecuencias

- **Positivas**: verificación de compilación iOS reproducible y gratuita (mientras el repo siga
  público y se usen runners estándar), sin hardware propio ni VMs en zona gris legal; patrón sin
  gomobile para binario puro; la verificación queda documentada y es accionable en el momento en
  que el scope lo pida.
- **Limitaciones aceptadas**: es verificación de compilación (y simulador si hubiera app), no de
  runtime en device — background/sandbox de iOS no se pueden probar sin hardware. Precios y
  políticas de GitHub pueden cambiar: re-verificar en la fecha de ejecución.
- **Alternativas rechazadas**:
  - *Docker-OSX / OSX-KVM* (macOS en KVM sobre Linux): EULA de Apple solo autoriza macOS sobre
    hardware Apple (zona gris), pesado, y sin device físico solo llega al simulador.
  - *Farms de dispositivos reales* (BrowserStack, Sauce Labs, Firebase Test Lab iOS): costo por
    minutos; hoy no hay app que probar.
  - *Corellium*: iOS virtualizado enterprise, overkill para este proyecto.
  - *iSH / a-Shell* (terminal Linux emulada dentro del iPhone del usuario final): lento, no
    verificable sin device, no es camino de producto.
  - *xtool* (xtool-org/xtool): compila Swift/SwiftPM, no Go; exige device físico.

## References

- [xtool](https://github.com/xtool-org/xtool) — evaluado y descartado (2026-10-13)
- [GitHub Actions billing](https://docs.github.com/en/billing/concepts/product-billing/github-actions) — runners estándar gratis en repos públicos (verificado 2026-10-13)
- Patrón de build iOS sin gomobile: discusiones de la comunidad Go (p. ej. golang/go#57397 y scripts de build tipo xcframework con `GOOS=ios` + cgo)
- ADR-0018 (modelo de canales), ADR-0014 (release/deploy), PRD §7
