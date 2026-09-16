// Package tui implements the operator-facing Bubble Tea interface of
// `mcp-server admin tui` (ADR-0016 Decision 1).
//
// It is a presentation and state layer: the package owns the screens, the
// strict Model-View-Update state machine and the input validation, and it
// delegates every read and every mutation to internal/admin — the reviewed core
// that applies the business rules, the semantic Spanish messages and the
// fabricated TUI caller (admin.TUIContext). The package issues no SQL, builds no
// entity for a write (it passes the core input types) and reaches the
// repositories only through the narrow ports declared here.
package tui

import "github.com/egkike/mcp-appointments-crm/internal/admin"

// AccountsPort is the union of the narrow internal/admin ports the TUI drives.
// *repository.AccountsRepo satisfies it unchanged; composing the already-narrow
// core ports keeps this package unaware of the concrete repository type
// (mirrors how newIdentityDeps wires the same repo into the console flows).
//
// The methods are the union of AccountsReader (seed decision), AccountsCreator
// (seed, Add Staff, prepared successor), AccountsActivator (owner reactivation),
// AccountsAdmin (list views and soft delete), AccountsTransfer (ownership
// transfer) and AccountsByIDReader (add-self account validation).
type AccountsPort interface {
	admin.AccountsReader
	admin.AccountsCreator
	admin.AccountsActivator
	admin.AccountsAdmin
	admin.AccountsTransfer
	admin.AccountsByIDReader
}

// ProfessionalsPort is the read slice of repository.ProfessionalsRepo that the
// Add Staff picker needs. *repository.ProfessionalsRepo satisfies it unchanged.
type ProfessionalsPort interface {
	admin.ProfessionalsReader
}

// ClientsPort is the client slice of repository.ClientsRepo that the
// "Agregarme como cliente" flow needs (ADR-0011). *repository.ClientsRepo
// satisfies it unchanged.
type ClientsPort interface {
	admin.ClientsSelfService
}

// Deps bundles the identity repositories the TUI operates on. The composition
// root (cmd/mcp-server) builds it from newIdentityDeps, so the TUI and serve
// mode cannot drift in repository construction.
type Deps struct {
	Accounts      AccountsPort
	Professionals ProfessionalsPort
	Clients       ClientsPort
}
