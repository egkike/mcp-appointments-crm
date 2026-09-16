package tui

import (
	"strings"
	"unicode/utf8"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

// accountTableHeaders are the columns of the read-only account views. They are
// fixed, so the operator scans the same five fields on every list screen
// (ADR-0016 Decision 1, "List views").
var accountTableHeaders = [5]string{"TELÉFONO", "ROL", "NOMBRE", "PROFESIONAL", "ESTADO"}

// renderAccountTable renders one account view as a fixed-width plain-text table
// so it stays readable and scrollable inside a viewport. Widths are measured in
// runes because display names carry accents, and a cell is never truncated:
// hiding operator data to keep a table tidy would be a silent lie.
func renderAccountTable(views []admin.AccountView) string {
	var widths [5]int
	for i, header := range accountTableHeaders {
		widths[i] = utf8.RuneCountInString(header)
	}

	rows := make([][5]string, 0, len(views))
	for _, view := range views {
		row := [5]string{
			view.ID,
			string(view.Role),
			view.DisplayName,
			professionalColumn(view),
			stateColumn(view),
		}
		for i, cell := range row {
			if width := utf8.RuneCountInString(cell); width > widths[i] {
				widths[i] = width
			}
		}
		rows = append(rows, row)
	}

	var b strings.Builder
	b.WriteString(formatAccountRow(accountTableHeaders, widths))
	for _, row := range rows {
		b.WriteString("\n")
		b.WriteString(formatAccountRow(row, widths))
	}
	return b.String()
}

// formatAccountRow pads every column to its width and joins the cells.
func formatAccountRow(cells [5]string, widths [5]int) string {
	padded := make([]string, 0, len(cells))
	for i, cell := range cells {
		padded = append(padded, padCell(cell, widths[i]))
	}
	return strings.Join(padded, "  ")
}

// padCell right-pads a cell with spaces up to width, measured in runes.
func padCell(cell string, width int) string {
	if shortfall := width - utf8.RuneCountInString(cell); shortfall > 0 {
		return cell + strings.Repeat(" ", shortfall)
	}
	return cell
}

// professionalColumn renders the account's professional reference, or "-" when
// the account has none (owner and admin accounts).
func professionalColumn(view admin.AccountView) string {
	if view.ProfessionalID == "" {
		return "-"
	}
	return view.ProfessionalID
}

// stateColumn renders the soft-delete (is_active) state in the operator's
// language instead of the raw 0/1 of the column.
func stateColumn(view admin.AccountView) string {
	if view.Active {
		return "activa"
	}
	return "inactiva"
}

// pickerWindow returns the visible bounds [start, end) of a cursor-driven
// picker that must never print more than limit rows. The window keeps the
// cursor centred while it can and pins itself to the last page at the bottom,
// so a long professional list stays navigable without a scroll offset the
// operator has to reason about.
func pickerWindow(count, cursor, limit int) (start, end int) {
	if count <= limit {
		return 0, count
	}
	if limit < 1 {
		return cursor, min(cursor+1, count)
	}

	start = cursor - limit/2
	if start < 0 {
		start = 0
	}
	if start+limit > count {
		start = count - limit
	}
	return start, start + limit
}
