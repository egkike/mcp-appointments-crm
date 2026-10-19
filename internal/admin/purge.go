// Package admin — purge.go is the shared core of the batch purge of inactive
// accounts (change feat-tui-ops-residuals S3, owner decisions OQ-1 menu entry
// + OQ-3 accounts-only). Both presentations (Bubble Tea and console) call the
// same functions, so the FK-safety checks, the target resolution and the
// outcome shaping cannot drift between them.
package admin

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// AccountsPurger is the destructive slice of repository.AccountsRepo the
// purge flow needs: the full list (to resolve the current inactive set) and
// the per-row guarded hard delete. Kept narrow so a mock survives without a
// database (mirrors AccountsAdmin).
type AccountsPurger interface {
	AccountsAdmin
	PurgeInactive(ctx context.Context, id string) (bool, error)
}

// InactiveAccounts returns the soft-deleted accounts the purge would remove,
// oldest first: the operator confirms against THIS list, and the write runs
// against the current state anyway (the repo re-checks is_active at delete
// time), so a re-activated account after the pick is never lost.
func InactiveAccounts(ctx context.Context, reader AccountsAdmin) ([]AccountView, error) {
	views, err := ListAccounts(TUIContext(ctx), reader, ListFilter{IncludeInactive: true})
	if err != nil {
		return nil, err
	}

	inactive := make([]AccountView, 0, len(views))
	for _, view := range views {
		if !view.Active {
			inactive = append(inactive, view)
		}
	}
	return inactive, nil
}

// PurgeInactiveAccounts hard-deletes the inactive accounts whose ids are
// passed in. The requirement is an owner caller (the port enforces it per
// row) and ids that were inactive WHEN the operator confirmed: the port
// re-checks at delete time and reports per-row outcomes, so one stale id
// never aborts the batch. A purge stays accounts-only by owner decision
// (OQ-3): orphaned professionals are a documented follow-up, not this batch.
type PurgeOutcome struct {
	Account AccountView // the slice projection of the tried id
	Purged  bool        // false when the row disappeared or was re-activated meanwhile
}

func PurgeInactiveAccounts(ctx context.Context, purger AccountsPurger, ids []string) ([]PurgeOutcome, error) {
	if len(ids) == 0 {
		return nil, fmt.Errorf("purgar: %w: no hay cuentas desactivadas para purgar", domain.ErrInvalidInput)
	}
	ctx = TUIContext(ctx)

	outcomes := make([]PurgeOutcome, 0, len(ids))
	for _, id := range ids {
		purged, err := purger.PurgeInactive(ctx, id)
		if err != nil {
			// B8 f9 (GGA round 1, owner: fix + recommit): the operator-facing
			// partial message points to the service log, so the raw cause must
			// actually land there. slog.Default() IS the designated main logger
			// (main.go D2) for both the TUI and console flows, and masking stays
			// consistent with the package's no-raw-id display contract (this
			// path never carried a raw-id emission, so f5's disposition is
			// untouched). The masked wrap below stays the client-facing shape.
			slog.Error("purge-inactive row failed", "account", entity.MaskPhone(id), "error", err)
			return outcomes, fmt.Errorf("purgar cuenta %s: %w", entity.MaskPhone(id), err)
		}
		// The outcome view carries the MASKED id only: the per-row report is
		// operator-facing, and the id IS the phone (no-PII display contract,
		// untouched elsewhere: the raw id stays in the DELETE parameter only).
		outcomes = append(outcomes, PurgeOutcome{
			Account: AccountView{ID: entity.MaskPhone(id)},
			Purged:  purged,
		})
	}
	return outcomes, nil
}

// PurgeQuestion renders the batch summary the confirm gates repeat. The ids
// are already masked (AccountView ids here carry the masked shape only).
// PurgeQuestion builds the operator-facing summary both confirm gates show:
// one masked label per account.
func PurgeQuestion(views []AccountView) string {
	summary := ""
	for i, view := range views {
		if i > 0 {
			summary += "; "
		}
		summary += AccountLabel(view)
	}
	return summary
}
