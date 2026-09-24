package tui

import (
	"context"
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

// ── Asynchronous core-call results ───────────────────────────────────────
//
// Every port call runs inside a tea.Cmd, so the database never blocks the
// Update loop and the operator sees the spinner while it runs. The results come
// back as the messages below; the model turns them into screens.

// gateMsg is the result of the boot sequence: the seed decision, the caller-id
// repair outcome and the deactivated owner rows the recovery screen offers.
type gateMsg struct {
	needsSeed bool
	repaired  bool
	inactive  []admin.AccountView
	err       error
}

// professionalsMsg carries the Add Staff picker rows.
type professionalsMsg struct {
	items []admin.PickableProfessional
	err   error
}

// accountsTarget tells the model which screen an accountsMsg fills: the
// deactivate picker or a read-only table view.
type accountsTarget int

const (
	accountPicker accountsTarget = iota
	accountTable
)

// accountsMsg carries the rows of one account query plus the filter that
// produced them, so the table can toggle "include inactive" without re-deriving
// the query.
type accountsMsg struct {
	target accountsTarget
	filter admin.ListFilter
	views  []admin.AccountView
	err    error
}

// transferDataMsg carries the current owner and the successor candidates of the
// transfer picker.
type transferDataMsg struct {
	owner   admin.AccountView
	options []transferOption
	err     error
}

// selfOwnerMsg carries the ACTIVE owner the add-self flow registers as a client
// (the account id is the phone Hermes sends as caller id, ADR-0011).
type selfOwnerMsg struct {
	owner admin.AccountView
	err   error
}

// hermesDataMsg carries the ACTIVE owner whose phone prefills the "Configurar
// Hermes" wizard plus the lazily-resolved bootstrap facts. Without an active
// owner the flow cannot proceed (ADR-0017 Decision 3); resolveFailed is true
// when err came from resolving the bootstrap rather than from the missing owner.
type hermesDataMsg struct {
	owner         admin.AccountView
	hermes        HermesConfig
	resolveFailed bool
	err           error
}

// hermesResultMsg is the outcome of the Hermes bootstrap. written is true when
// the merged config landed on disk; otherwise err carries the semantic failure
// and snippet the exact YAML block the operator copies by hand (ADR-0017
// Decision 1).
type hermesResultMsg struct {
	written bool
	snippet string
	err     error
}

// resultMsg is the outcome of a write through the core. lines are the semantic
// success lines the info screen renders; err is a business failure, already
// phrased for the operator by the core. reloadGate asks the model to re-derive
// the seed gate: every seed/recovery failure must be re-evaluated against the
// database instead of assuming the installation state the flow started from.
type resultMsg struct {
	lines      []string
	err        error
	reloadGate bool
}

// ── Commands ─────────────────────────────────────────────────────────────

// loadGateCmd runs the boot sequence against the reviewed core:
//
//  1. NeedsSeed decides whether the first-owner gateway must run;
//  2. with an active owner, EnsureCallerID repairs the partial-seed wedge
//     (R4-partial-seed-wedge) before the menu opens;
//  3. without one, InactiveOwners tells a clean install apart from an
//     installation whose owner row still exists (R4-deactivated-owner-seed-deadend).
func loadGateCmd(ctx context.Context, deps Deps) tea.Cmd {
	return func() tea.Msg {
		needsSeed, err := admin.NeedsSeed(ctx, deps.Accounts)
		if err != nil {
			return gateMsg{err: err}
		}
		if !needsSeed {
			repaired, repairErr := admin.EnsureCallerID(ctx, deps.Accounts)
			if repairErr != nil {
				return gateMsg{err: repairErr}
			}
			return gateMsg{repaired: repaired}
		}

		inactive, err := admin.InactiveOwners(ctx, deps.Accounts)
		if err != nil {
			return gateMsg{err: err}
		}
		return gateMsg{needsSeed: true, inactive: inactive}
	}
}

// seedCmd creates the first owner and writes the caller-id file. The caller-id
// write is part of the same command because the file must never advertise an id
// for an owner that was not created, and a created owner without the file is the
// wedge EnsureCallerID repairs.
func seedCmd(ctx context.Context, deps Deps, input admin.SeedInput) tea.Cmd {
	return func() tea.Msg {
		if err := admin.Seed(ctx, deps.Accounts, input); err != nil {
			return resultMsg{err: err, reloadGate: true}
		}

		path, err := admin.WriteCallerID(input.Phone)
		if err != nil {
			return resultMsg{
				err: fmt.Errorf("se creó el owner %s (%s) pero no se pudo escribir el caller-id: %w",
					input.DisplayName, input.Phone, err),
				reloadGate: true,
			}
		}
		return resultMsg{lines: []string{
			fmt.Sprintf("Owner creado: %s (%s)", input.DisplayName, input.Phone),
			fmt.Sprintf("caller-id escrito en %s", path),
			"Ya podés usar `mcp-server hermes chat` con ese caller id.",
		}}
	}
}

// reactivateOwnerCmd flips one deactivated owner row back to active and
// republishes the caller id, because Hermes must send the id of the ACCOUNT THAT
// IS ACTIVE (ADR-0012): the previous id was the deactivated row.
func reactivateOwnerCmd(ctx context.Context, deps Deps, id string) tea.Cmd {
	return func() tea.Msg {
		reactivated, err := admin.ReactivateOwner(ctx, deps.Accounts, id)
		if err != nil {
			return resultMsg{err: err, reloadGate: true}
		}

		path, err := admin.WriteCallerID(reactivated.ID)
		if err != nil {
			return resultMsg{
				err: fmt.Errorf("se reactivó %s pero no se pudo escribir el caller-id: %w",
					admin.AccountLabel(reactivated), err),
				reloadGate: true,
			}
		}
		return resultMsg{lines: []string{
			fmt.Sprintf("Owner reactivado: %s", admin.AccountLabel(reactivated)),
			fmt.Sprintf("caller-id escrito en %s", path),
			"Ya podés usar `mcp-server hermes chat` con ese caller id.",
		}}
	}
}

// professionalsCmd reads the ACTIVE professionals the Add Staff picker offers.
// The picker is the only sanctioned source of professional ids: the account's
// professional_id has no FK, so a typed id could create a dangling reference.
func professionalsCmd(ctx context.Context, deps Deps) tea.Cmd {
	return func() tea.Msg {
		items, err := admin.ListPickableProfessionals(ctx, deps.Professionals)
		return professionalsMsg{items: items, err: err}
	}
}

// accountsCmd reads one account view through the core under the given filter.
func accountsCmd(ctx context.Context, deps Deps, target accountsTarget, filter admin.ListFilter) tea.Cmd {
	return func() tea.Msg {
		views, err := admin.ListAccounts(ctx, deps.Accounts, filter)
		return accountsMsg{target: target, filter: filter, views: views, err: err}
	}
}

// addStaffCmd creates a staff account linked to the professional the operator
// picked.
func addStaffCmd(ctx context.Context, deps Deps, professional admin.PickableProfessional, input admin.StaffInput) tea.Cmd {
	return func() tea.Msg {
		if err := admin.AddStaff(ctx, deps.Professionals, deps.Accounts, input); err != nil {
			return resultMsg{err: err}
		}
		return resultMsg{lines: []string{fmt.Sprintf(
			"Cuenta de staff creada: %s (%s) para el profesional %s",
			input.DisplayName, input.Phone, professional.Name)}}
	}
}

// deactivateCmd soft-deletes one account. An already inactive account is an
// outcome the core reports, not an error, and it is phrased as such here.
func deactivateCmd(ctx context.Context, deps Deps, id string) tea.Cmd {
	return func() tea.Msg {
		outcome, err := admin.DeactivateAccount(ctx, deps.Accounts, id)
		if err != nil {
			return resultMsg{err: err}
		}
		if outcome.AlreadyInactive {
			return resultMsg{lines: []string{
				"La cuenta ya estaba inactiva: " + admin.AccountLabel(outcome.Account)}}
		}
		return resultMsg{lines: []string{"Cuenta desactivada: " + admin.AccountLabel(outcome.Account)}}
	}
}

// transferDataCmd resolves the current owner plus the successor candidates: the
// INACTIVE owner rows, the ACTIVE staff accounts that can be promoted, and the
// "new phone" escape hatch. The active owner is never offered — the swap
// deactivates it.
func transferDataCmd(ctx context.Context, deps Deps) tea.Cmd {
	return func() tea.Msg {
		owner, err := admin.ActiveOwner(ctx, deps.Accounts)
		if err != nil {
			return transferDataMsg{err: err}
		}

		options, err := successorOptions(ctx, deps)
		if err != nil {
			return transferDataMsg{err: err}
		}
		return transferDataMsg{owner: owner, options: options}
	}
}

// successorOptions maps the shared core successor list (internal/admin) onto the
// TUI's own option struct. The ordering, the eligibility and the labels are
// single-sourced in the core, which performs no write and no business decision:
// the eligibility of every candidate is re-validated by PrepareSuccessor and
// TransferOwnership before anything is written.
func successorOptions(ctx context.Context, deps Deps) ([]transferOption, error) {
	candidates, err := admin.SuccessorCandidates(ctx, deps.Accounts)
	if err != nil {
		return nil, err
	}

	options := make([]transferOption, 0, len(candidates))
	for _, candidate := range candidates {
		options = append(options, transferOption{
			kind:  candidate.Kind,
			id:    candidate.ID,
			label: candidate.Label,
		})
	}
	return options, nil
}

// transferCmd runs the two steps of the ownership transfer in one command:
// PrepareSuccessor guarantees an INACTIVE owner row for the successor (the
// single-owner invariant holds throughout), TransferOwnership performs the
// transactional swap, and the caller-id file is republished because the previous
// owner is now inactive and the resolver rejects inactive accounts (ADR-0012).
//
// A failure of the caller-id write AFTER the swap is reported as an error line
// next to the successful swap line: the installation has exactly one active
// owner, but Hermes may still be holding the previous caller id, so the operator
// has to know instead of reading a clean success.
func transferCmd(ctx context.Context, deps Deps, fromID string, successor admin.TransferSuccessor) tea.Cmd {
	return func() tea.Msg {
		prepared, err := admin.PrepareSuccessor(ctx, deps.Accounts, successor)
		if err != nil {
			return resultMsg{err: err}
		}

		outcome, err := admin.TransferOwnership(ctx, deps.Accounts, fromID, prepared.ID)
		if err != nil {
			return resultMsg{err: err}
		}

		swapLine := fmt.Sprintf("Ownership transferido: %s → %s",
			admin.AccountLabel(outcome.From), admin.AccountLabel(outcome.To))

		path, err := admin.WriteCallerID(outcome.To.ID)
		if err != nil {
			return resultMsg{
				lines: []string{swapLine},
				err: fmt.Errorf("no se pudo actualizar el archivo caller-id para el nuevo owner (%s); "+
					"borralo para que el próximo arranque lo regenere: %w", callerIDLocation(), err),
			}
		}
		return resultMsg{lines: []string{
			swapLine,
			fmt.Sprintf("caller-id actualizado en %s", path),
		}}
	}
}

// callerIDLocation resolves the caller-id path for the operator-facing repair
// hint. It returns a non-committal phrase when the path cannot be resolved: the
// hint must never turn a successful swap into a confusing failure.
func callerIDLocation() string {
	path, err := admin.CallerIDPath()
	if err != nil {
		return "el archivo caller-id"
	}
	return path
}

// selfOwnerCmd reads the ACTIVE owner whose account id is registered as a client
// by the add-self flow.
func selfOwnerCmd(ctx context.Context, deps Deps) tea.Cmd {
	return func() tea.Msg {
		owner, err := admin.ActiveOwner(ctx, deps.Accounts)
		return selfOwnerMsg{owner: owner, err: err}
	}
}

// addSelfCmd registers the operator's own account as a client of the business
// (ADR-0011 double role). An already registered phone is an outcome, not an
// error, so the operator reads the honest state.
func addSelfCmd(ctx context.Context, deps Deps, callerID, name string) tea.Cmd {
	return func() tea.Msg {
		outcome, err := admin.AddSelfAsClient(ctx, deps.Accounts, deps.Clients, callerID, name)
		if err != nil {
			return resultMsg{err: err}
		}
		if outcome.AlreadyRegistered {
			return resultMsg{lines: []string{"Ya estabas registrado como cliente con este teléfono."}}
		}
		return resultMsg{lines: []string{"Ya podés operar como cliente con este teléfono."}}
	}
}

// hermesDataCmd resolves the Hermes bootstrap facts and reads the ACTIVE owner
// whose phone prefills the wizard. Resolution happens HERE, inside the command,
// never at tui startup: an invalid MCP_BIND or an unresolvable home surfaces as
// the wizard's error line instead of blocking the whole admin TUI. The account
// id is the X-Caller-Id Hermes sends, so the wizard starts from the sanctioned
// value and the operator only edits it when needed.
func hermesDataCmd(ctx context.Context, deps Deps) tea.Cmd {
	return func() tea.Msg {
		hermes, err := deps.Hermes.Resolve()
		if err != nil {
			return hermesDataMsg{resolveFailed: true, err: err}
		}

		owner, err := admin.ActiveOwner(ctx, deps.Accounts)
		return hermesDataMsg{owner: owner, hermes: hermes, err: err}
	}
}

// hermesConfigCmd runs the shared Hermes chain and maps its outcome onto the
// wizard's result message: the written config on success, or the manual snippet
// plus the semantic failure that forced the fallback. The resolved HermesConfig
// and the operator's phone arrive as explicit arguments, so the command depends
// on no model state another handler mutated (R2-03): the caller passes the facts
// it resolved. The chain itself lives in admin.ApplyHermesConfig (R2-01), the
// same helper the line-based console runs.
func hermesConfigCmd(hermes HermesConfig, phone string) tea.Cmd {
	return func() tea.Msg {
		snippet, err := admin.ApplyHermesConfig(hermes.Path, hermes.EndpointURL, phone)
		if err == nil {
			return hermesResultMsg{written: true}
		}
		return hermesResultMsg{snippet: snippet, err: err}
	}
}
