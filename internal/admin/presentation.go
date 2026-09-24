package admin

import (
	"context"
	"fmt"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// This file holds the presentation-agnostic helpers shared by the two operator
// entry points: the Bubble Tea TUI (internal/tui) and the line-based console
// (cmd/mcp-server). Keeping the label, the role menu, the successor list and the
// recovery hint here, next to the flows they describe, is what makes the
// "cannot drift" claim in the console header true for the rendered facts as
// well as for the repository wiring: a label, a role, a candidate order or a
// hint changes once, for both presentations.
//
// The helpers stay free of any terminal concern (no io.Writer, no lipgloss):
// each presentation keeps its own option structs and renders the core values in
// its own way, mapping them at the boundary.

// ApplyHermesConfig runs the full Hermes bootstrap chain against the reviewed
// core: load the existing config, merge the single mcp_servers.mcp-appointments
// entry (preserving every other key and server) and write it atomically. On
// success it returns an empty snippet and a nil error. On failure it returns the
// semantic error that broke the chain plus the exact YAML snippet the writer
// would have emitted — empty when the snippet itself cannot be rendered — so the
// caller degrades to the same manual fallback (ADR-0017 Decision 1).
//
// It is the single chain both presentations run (R2-01): the Bubble Tea
// hermesConfigCmd and the line-based console runConfigureHermesFlow, so the
// merge, the atomic write and the fallback snippet cannot drift between them.
// The values arrive as plain data (a path and two strings), so neither
// presentation re-derives the path or the endpoint.
//
// presentation.go is the deliberate home: this composition is the seam the two
// operator entry points share, so keeping it beside the label/role/hint helpers
// extends the "cannot drift" claim to the chain itself. The steps it composes
// (LoadHermesConfig, SetHermesServer, WriteHermesConfig, RenderHermesSnippet)
// stay in hermes.go as the framework-free Hermes core; this file owns only the
// presentation-agnostic composition and the ADR-0017 Decision 1 fallback.
func ApplyHermesConfig(path, endpointURL, phone string) (snippet string, err error) {
	doc, err := LoadHermesConfig(path)
	if err == nil {
		err = doc.SetHermesServer(endpointURL, phone)
	}
	if err == nil {
		err = WriteHermesConfig(path, doc)
	}
	if err == nil {
		return "", nil
	}

	snippet, snippetErr := RenderHermesSnippet(endpointURL, phone)
	if snippetErr != nil {
		snippet = ""
	}
	return snippet, err
}

// AccountLabel renders one account for the operator: the display name the human
// recognises, plus the role and the phone, the two fields that cannot be guessed
// from the name.
func AccountLabel(view AccountView) string {
	return fmt.Sprintf("%s (%s, %s)", view.DisplayName, view.Role, view.ID)
}

// ListableRoles is the role sub-menu of the "Listar por rol" flow. It mirrors
// the domain enumeration (ADR-0009) in menu order; the core validates the
// selected role again before any port call.
func ListableRoles() []entity.AccountRole {
	return []entity.AccountRole{entity.RoleOwner, entity.RoleAdmin, entity.RoleStaff}
}

// ReactivationHint names the deactivated owner whose phone the operator just
// tried to reuse, so the semantic conflict carries the exact next step. It
// returns an empty string when the phone belongs to something else.
func ReactivationHint(inactiveOwners []AccountView, phone string) string {
	for _, view := range inactiveOwners {
		if view.ID == phone {
			return fmt.Sprintf(
				"Sugerencia: el teléfono %s pertenece a la cuenta de owner desactivada %q; reactivala con la opción de reactivación en lugar de crear una cuenta nueva.",
				phone, view.DisplayName)
		}
	}
	return ""
}

// SuccessorCandidate is one numbered entry of the ownership-transfer picker: an
// existing account row (inactive owner or active staff) or the "new phone"
// escape hatch. The eligibility of every candidate is re-validated by
// PrepareSuccessor and TransferOwnership before anything is written.
type SuccessorCandidate struct {
	Kind  SuccessorKind
	ID    string
	Label string
}

// SuccessorCandidates builds the numbered successor list shared by both
// presentations: every INACTIVE owner row, then every ACTIVE staff account, then
// the "new phone" escape hatch. The active owner is never offered — the swap
// deactivates it. Labels carry name, role and phone so the operator recognises
// the row they are about to promote.
//
// It performs no write and no business decision: the ordering and the
// eligibility of every candidate are re-validated by PrepareSuccessor and
// TransferOwnership before anything is written.
func SuccessorCandidates(ctx context.Context, accounts AccountsAdmin) ([]SuccessorCandidate, error) {
	owners, err := ListAccounts(ctx, accounts, ListFilter{
		Role:            entity.RoleOwner,
		IncludeInactive: true,
	})
	if err != nil {
		return nil, err
	}

	candidates := make([]SuccessorCandidate, 0, len(owners)+1)
	for _, view := range owners {
		if view.Active {
			continue
		}
		candidates = append(candidates, SuccessorCandidate{
			Kind:  SuccessorInactiveOwner,
			ID:    view.ID,
			Label: AccountLabel(view),
		})
	}

	staff, err := ListAccounts(ctx, accounts, ListFilter{Role: entity.RoleStaff})
	if err != nil {
		return nil, err
	}
	for _, view := range staff {
		candidates = append(candidates, SuccessorCandidate{
			Kind:  SuccessorStaff,
			ID:    view.ID,
			Label: AccountLabel(view) + " — promover a owner",
		})
	}

	return append(candidates, SuccessorCandidate{
		Kind:  SuccessorNewPhone,
		Label: "Otro teléfono (crear una cuenta de owner nueva)",
	}), nil
}
