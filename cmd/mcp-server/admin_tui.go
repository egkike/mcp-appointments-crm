package main

import "github.com/egkike/mcp-appointments-crm/internal/domain"

// runAdminTUI is the entry point of `mcp-server admin tui`, the operator-facing
// identity and accounts TUI (ADR-0016 §5, ADR-0010).
//
// T1 wires the sub-command only. Until T2-T7 land the screens, the stub:
//
//  1. validates the dependencies serve mode also validates at startup
//     (configuration + SQLite), so a broken install fails here instead of
//     half-opening a TUI;
//  2. constructs the identity repositories through the shared construction
//     site, so the flows inherit production wiring with no extra plumbing;
//  3. fails fast with a semantic error → exit code 1.
//
// The HTTP transport is deliberately not validated: ADR-0016 Decision 3.5 keeps
// the TUI independent from it (shared *sql.DB, logger and repos only).
func runAdminTUI() error {
	deps, err := openCommandDependencies()
	if err != nil {
		return err
	}
	defer deps.close()

	// Accounts + clients repos are ready for T2-T7; this task intentionally does
	// not consume them yet. Building them now keeps one construction site with
	// serve mode (newIdentityDeps) and proves the wiring against a real DB.
	_ = newIdentityDeps(deps.database, deps.logger)

	deps.logger.Info("admin tui dependencies ready", "version", deps.config.Version)

	return &domain.SemanticError{
		Code: domain.ErrCodeInternal,
		Message: "la TUI de administración todavía no está implementada: " +
			"la gestión de identidad y cuentas llega en las tareas T2-T7 de la feature admin-tui",
	}
}
