package tui

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

// auditMsg carries the read-only audit page to the model: the exact projection
// the audit view screen renders (change feat-tui-ops-residuals S1, owner
// decision OQ-6: últimas 200, sin paginación).
type auditMsg struct {
	events []admin.AuditEventView
	err    error
}

// auditCmd reads the latest audit events through the core. The page is the
// OQ-6 default (admin.AuditTrailViewLimit: 200, read-only); the reader clamps
// defensively, so a wiring bug cannot turn the read into a full-table scan.
func auditCmd(ctx context.Context, deps Deps) tea.Cmd {
	return func() tea.Msg {
		events, err := admin.ListAuditTrail(ctx, deps.Audit, admin.AuditTrailViewLimit)
		return auditMsg{events: events, err: err}
	}
}

// onAudit fills the audit view screen, or degrades to an info screen: an
// empty trail is the honest notice for a fresh host, and a read error is an
// operator message, never a panic.
func (m AppModel) onAudit(msg auditMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		return m.withInfo(infoError, msg.err.Error()), nil
	}
	if len(msg.events) == 0 {
		return m.withInfo(infoNotice, "El trail de auditoría está vacío: aún no hay mutaciones de cuentas."), nil
	}
	m = m.withScreen(screenAuditTrail)
	m.auditEvents = msg.events
	m.tableTitle = fmt.Sprintf("Audit log (últimos %d, read-only)", len(msg.events))
	m.viewport.SetContent(renderAuditTrailTable(msg.events))
	return m, nil
}

// renderAuditTrailTable renders the read-only audit page as a fixed-width
// plain-text table with the same runes-measured padding as the accounts table
// (cell never truncated: hiding operator data would be a silent lie). The
// actor reference is hashed at the persistence layer; the view shows a prefix
// that stays scannable inside the viewport.
func renderAuditTrailTable(events []admin.AuditEventView) string {
	const cols = 6
	headers := [cols]string{"TS", "EVENTO", "ACCIÓN", "CUENTA", "TELÉFONO", "ACTOR"}
	widths, rows := [cols]int{}, make([][cols]string, 0, len(events))
	for i, header := range headers {
		widths[i] = utf8.RuneCountInString(header)
	}
	for _, e := range events {
		rows = append(rows, [cols]string{
			e.Ts,
			e.Event,
			e.Action,
			auditTargetCell(e),
			e.PhoneMasked,
			auditActorCell(e),
		})
		for i := range rows[len(rows)-1] {
			if w := utf8.RuneCountInString(rows[len(rows)-1][i]); w > widths[i] {
				widths[i] = w
			}
		}
	}

	var b strings.Builder
	b.WriteString(formatRow(headers, widths))
	for _, row := range rows {
		b.WriteString("\n")
		b.WriteString(formatRow(row, widths))
	}
	return b.String()
}

// auditTargetCell renders the affected account id (masked phone) with the
// role, so the row is self-explanatory without a second scan.
func auditTargetCell(e admin.AuditEventView) string {
	if e.TargetRole == "" {
		return e.TargetID
	}
	return e.TargetID + " (" + e.TargetRole + ")"
}

// auditActorCell renders the hashed actor reference, or "-" when the flow ran
// without a Caller (seed/bootstrap paths). The hash is 64 hex chars; the
// 8-rune prefix keeps the correlation key visible without widening every row.
func auditActorCell(e admin.AuditEventView) string {
	if e.ActorRef == "" {
		return "-"
	}
	// Hashes are 64 hex chars, but the row reader must never trust them: burn
	// a short (corrupt) reference whole instead of panicking on the slice.
	if len(e.ActorRef) <= 8 {
		return e.ActorRef
	}
	return e.ActorRef[:8] + "…"
}

// formatRow pads every column to its width and joins the cells. It is the
// exact same shape formatAccountRow serves for the accounts table; the audit
// columns reuse it so both read-only views keep one padding implementation.
func formatRow(cells [6]string, widths [6]int) string {
	padded := make([]string, 0, len(cells))
	for i, cell := range cells {
		padded = append(padded, padCell(cell, widths[i]))
	}
	return strings.Join(padded, "  ")
}
