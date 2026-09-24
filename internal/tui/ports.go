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

import (
	"sync"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

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

// HermesConfig is the Hermes bootstrap data the composition root resolves once
// from the running server configuration (ADR-0017): the config file the flow
// reads, merges and writes, and the MCP endpoint URL Hermes must call. The TUI
// derives neither value itself — the endpoint comes from MCP_BIND/MCP_PORT and
// the path from the standard ~/.hermes layout — and the path stays injectable so
// a test points it at t.TempDir() instead of a real home.
type HermesConfig struct {
	Path        string
	EndpointURL string
}

// HermesResolver resolves the Hermes bootstrap facts on demand.
type HermesResolver func() (HermesConfig, error)

// HermesBootstrap is the lazily-resolved Hermes bootstrap of the TUI. It embeds
// the resolved HermesConfig, so the Hermes screens read Path/EndpointURL
// directly, and carries the resolver that fills them. Nothing resolves at tui
// startup: the composition root builds it with NewHermesBootstrap and the
// "Configurar Hermes" command calls Resolve, so an unresolvable home or an
// invalid MCP_BIND only surfaces when the operator opens that capability.
type HermesBootstrap struct {
	HermesConfig
	resolve HermesResolver
}

// NewHermesBootstrap returns a bootstrap that resolves its facts on demand
// through resolve. The composition root wraps resolve with
// NewCachedHermesResolver, so a success is paid once and a failure can be
// retried on the next open.
func NewHermesBootstrap(resolve HermesResolver) HermesBootstrap {
	return HermesBootstrap{resolve: resolve}
}

// NewCachedHermesResolver wraps resolve so the first SUCCESSFUL resolution is
// cached and a failed one is left uncached. A transient failure — an unmounted
// home, a MCP_BIND the operator is about to correct — therefore surfaces on the
// open that hit it and is retried on the next one, while a resolved bootstrap
// pays its cost once. This replaces the sync.OnceValues the composition root
// used to install, which cached the error forever and left a first failure
// stuck for the whole session with no retry path (R4-001). Both presentations —
// the Bubble Tea program and the line-based console — share this contract.
//
// A nil resolve returns nil: the zero HermesBootstrap already models "nothing to
// resolve".
func NewCachedHermesResolver(resolve HermesResolver) HermesResolver {
	if resolve == nil {
		return nil
	}
	var (
		mu     sync.Mutex
		cached *HermesConfig
	)
	return func() (HermesConfig, error) {
		mu.Lock()
		defer mu.Unlock()
		if cached != nil {
			return *cached, nil
		}
		config, err := resolve()
		if err != nil {
			return HermesConfig{}, err
		}
		cached = &config
		return config, nil
	}
}

// Resolve returns the resolved bootstrap facts. A bootstrap with no resolver
// (already resolved, e.g. a test fixture) returns its embedded facts unchanged.
func (h HermesBootstrap) Resolve() (HermesConfig, error) {
	if h.resolve == nil {
		return h.HermesConfig, nil
	}
	return h.resolve()
}

// Deps bundles the identity repositories the TUI operates on. The composition
// root (cmd/mcp-server) builds it from newIdentityDeps, so the TUI and serve
// mode cannot drift in repository construction. Hermes carries the lazily
// resolved Hermes bootstrap of the "Configurar Hermes" capability.
type Deps struct {
	Accounts      AccountsPort
	Professionals ProfessionalsPort
	Clients       ClientsPort
	Hermes        HermesBootstrap
}
