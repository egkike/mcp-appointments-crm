package tui

// audit_screen.go hosts the Bubble Tea plumbing of the read-only audit view
// (change feat-tui-ops-residuals S1): screen constant, model state and the
// view body. The dispatch (openMenuItem/menuItems) lives with the other menu
// capabilities; the keys are mirrowed from the accounts table screen so the
// operator navigates both read-only views the same way.

// viewAuditTrail renders the audit screen: title (the row count already
// speaks: "últimos N"), a read-only hint line and the viewport body.
func (m AppModel) viewAuditTrail() string {
	return m.tableTitle + "\n" + m.viewport.View() + "\n"
}
