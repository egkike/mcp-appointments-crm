package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

// Purge flow (change feat-tui-ops-residuals S3, owner decisions OQ-1 menu
// entry + OQ-3 accounts-only): resolve the current inactive set, then gate the
// write behind TWO confirmations (the ordinary one and the permanent one) on
// the shared confirm screen. The ids travel inside confirmState, so the
// gates cannot drift from the actual batch.

type purgeDataMsg struct {
	views []admin.AccountView
	err   error
}

func purgeDataCmd(ctx context.Context, deps Deps) tea.Cmd {
	return func() tea.Msg {
		views, err := admin.InactiveAccounts(ctx, deps.Accounts)
		return purgeDataMsg{views: views, err: err}
	}
}

// onPurgeData opens the first confirm gate, or reports the honest emptiness:
// a host without soft-deleted accounts has nothing to purge and that is a
// notice, not an error.
func (m AppModel) onPurgeData(msg purgeDataMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		return m.withInfo(infoError, msg.err.Error()), nil
	}
	if len(msg.views) == 0 {
		return m.withInfo(infoNotice, "No hay cuentas desactivadas para purgar."), nil
	}

	question := fmt.Sprintf(
		"Esta acción ELIMINA PERMANENTEMENTE %d cuenta(s) desactivada(s): %s. ¿Confirmar?",
		len(msg.views), admin.PurgeQuestion(msg.views))
	m = m.askConfirm(confirmState{
		action:   confirmPurge,
		title:    confirmPurgeTitle,
		question: question,
		ids:      accountIDs(msg.views),
	})
	return m, nil
}

// accountIDs extracts the batch ids the purge would delete (masked at the
// persistence layer by the audit; the DELETE matches the raw account id).
func accountIDs(views []admin.AccountView) []string {
	ids := make([]string, 0, len(views))
	for _, view := range views {
		ids = append(ids, view.ID)
	}
	return ids
}

// purgeCmd runs the batch once both gates accepted. Every row outcome is
// reported individually: a stale id (re-activated meanwhile) is a listed
// outcome, not a failure, and the summary counts what was really purged.
func purgeCmd(ctx context.Context, deps Deps, ids []string) tea.Cmd {
	return func() tea.Msg {
		outcomes, err := admin.PurgeInactiveAccounts(ctx, deps.Accounts, ids)
		// A mid-batch failure must not swallow the completed outcomes: the
		// operator reads WHICH accounts were already deleted, then the error.
		raw := purgeSummary(outcomes, len(ids))
		// B8 f9b (GGA round 2): the count headline claims completeness ONLY
		// when the batch finished. A mid-batch abort is a PARTIAL purge, so the
		// row must not read "Purga completa" next to the failure hint.
		state := "completa"
		if err != nil {
			state = "parcial"
		}
		lines := []string{fmt.Sprintf("Purga %s: %s.", state, raw)}
		if err != nil {
			lines = append(lines, "Resultado parcial: "+purgeDoesNotComplete)
		}
		return resultMsg{lines: lines}
	}
}

// purgeSummary renders the count AND the per-account report the flows share:
// every completed outcome is listed with its masked identifier, so a partial
// batch never leaves the operator guessing what was already deleted.
func purgeSummary(outcomes []admin.PurgeOutcome, requested int) string {
	purged := 0
	rows := make([]string, 0, len(outcomes))
	for _, o := range outcomes {
		state := "no purgada"
		if o.Purged {
			purged++
			state = "eliminada permanente"
		}
		rows = append(rows, fmt.Sprintf("  %s: %s", o.Account.ID, state))
	}
	summary := fmt.Sprintf("%d cuenta(s) eliminada(s) permanentemente de %d pedidas", purged, requested)
	if len(rows) > 0 {
		summary += "\n" + strings.Join(rows, "\n")
	}
	return summary
}

// purgeDoesNotComplete is the ONLY operator error text of the flow: the raw
// cause stays in the service log (Fed/GGA standard: semantic messages toward
// operators, no driver dumps in UI paths).
const purgeDoesNotComplete = "la purga no se completó; revisá el log del servicio"
