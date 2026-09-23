package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

// Operator-facing copy. It stays in one block so the wording of every flow can
// be reviewed as text instead of hunting through the state machine. All of it is
// Spanish and business-phrased: the operator never reads a driver error.
const (
	keyQuit = "q"

	seedTitle = "Crear el owner de esta instalación"
	seedIntro = "No hay ningún owner activo: el administrador del sistema operativo es el " +
		"gatekeeper de este paso (ADR-0010)."

	seedRecoveryIntro = "No hay ningún owner activo, pero hay cuentas de owner desactivadas.\n" +
		"Podés reactivar una de ellas o crear un owner nuevo con otro teléfono."

	addStaffTitle        = "Crear una cuenta de staff"
	transferTitle        = "Transferir ownership: nuevo owner"
	selfTitle            = "Agregarme como cliente"
	rolePickerTitle      = "Elegí el rol"
	confirmHint          = `respondé "s" para confirmar o "n" para cancelar`
	emptyTableLine       = "No hay cuentas para mostrar."
	deactivatePickerHint = "Elegí la cuenta a desactivar"

	seedPhoneLabel     = "Teléfono del owner (ej. +5491100000000)"
	staffPhoneLabel    = "Teléfono del staff"
	transferPhoneLabel = "Teléfono del nuevo owner"
	displayNameLabel   = "Nombre para mostrar"
	clientNameLabel    = "Nombre para mostrar del cliente"

	confirmReactivateTitle  = "Reactivar owner"
	confirmDeactivateTitle  = "Desactivar cuenta"
	confirmTransferTitle    = "Transferir ownership"
	confirmTransferQuestion = "El owner actual quedará desactivado. ¿Confirmar?"
	busyLabel               = "Operación en curso…"

	hermesTitle           = "Configurar Hermes"
	hermesPhoneLabel      = "Teléfono del owner (X-Caller-Id)"
	confirmHermesTitle    = "Confirmar configuración de Hermes"
	confirmHermesQuestion = "Se escribirá la entrada mcp-appointments en el config de Hermes. ¿Confirmar?"
	hermesSnippetTitle    = "Configuración manual de Hermes"
	hermesSnippetIntro    = "No se pudo escribir el archivo automáticamente. Copiá este bloque en el archivo indicado:"
)

// capability identifies one operator capability of the menu (ADR-0016
// Decision 1). The menu is declarative so the dispatch (openMenuItem) and the
// rendering (viewMenu) read the same list and cannot drift.
type capability int

const (
	capabilityAddStaff capability = iota
	capabilityDeactivate
	capabilityListAll
	capabilityListByRole
	capabilityTransfer
	capabilityAddSelf
	capabilityConfigureHermes
)

// menuItem is one numbered entry of the operator menu, in menu order.
type menuItem struct {
	label      string
	capability capability
}

// menuItems is the single menu table: a new capability is one entry here plus
// its screen and its core call.
func menuItems() []menuItem {
	return []menuItem{
		{label: "Add Staff", capability: capabilityAddStaff},
		{label: "Desactivar cuenta", capability: capabilityDeactivate},
		{label: "Listar cuentas", capability: capabilityListAll},
		{label: "Listar por rol", capability: capabilityListByRole},
		{label: "Transferir ownership", capability: capabilityTransfer},
		{label: "Agregarme como cliente", capability: capabilityAddSelf},
		{label: "Configurar Hermes", capability: capabilityConfigureHermes},
	}
}

// Minimal, consistent styles: a title, a selected-row marker, a faint secondary
// tone and the three outcome tones. No theming layer on purpose.
var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("12"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Reverse(true)
	subtleStyle   = lipgloss.NewStyle().Faint(true)
	errStyle      = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
	noticeStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("11"))
	successStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("10"))
)

// seedSteps is the seed questionnaire: the phone (the account id, fact 7) and
// the display name, validated by the core validators — the only copy of those
// rules in the codebase.
func seedSteps() []formStep {
	return []formStep{
		{label: seedPhoneLabel, validate: admin.ValidatePhone},
		{label: displayNameLabel, validate: admin.ValidateDisplayName},
	}
}

// staffSteps prefills the phone from professionals.phone (verified fact 8) while
// keeping it editable: the staff account id may legitimately differ from the
// professional's phone.
func staffSteps(defaultPhone string) []formStep {
	return []formStep{
		{label: staffPhoneLabel, defaultValue: defaultPhone, validate: admin.ValidatePhone},
		{label: displayNameLabel, validate: admin.ValidateDisplayName},
	}
}

// transferSteps asks for the brand-new successor owner.
func transferSteps() []formStep {
	return []formStep{
		{label: transferPhoneLabel, validate: admin.ValidatePhone},
		{label: displayNameLabel, validate: admin.ValidateDisplayName},
	}
}

// hermesSteps is the single-field Hermes questionnaire: the owner phone that
// Hermes sends as X-Caller-Id, prefilled from the active owner and validated by
// admin.ValidatePhone — the same single validation point the core re-applies
// before writing.
func hermesSteps(defaultPhone string) []formStep {
	return []formStep{
		{label: hermesPhoneLabel, defaultValue: defaultPhone, validate: admin.ValidatePhone},
	}
}

// View renders the current frame purely from the model state. It performs no
// side effect and no port call.
func (m AppModel) View() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("mcp-server admin tui") + "\n\n")

	if m.busyNow() {
		b.WriteString(m.spinner.View() + " " + subtleStyle.Render(busyLabel) + "\n")
	}
	if m.errLine != "" {
		b.WriteString(errStyle.Render("Error: "+m.errLine) + "\n")
	}
	if m.noticeLine != "" {
		b.WriteString(noticeStyle.Render(m.noticeLine) + "\n")
	}

	switch m.screen {
	case screenBoot:
		b.WriteString("Verificando el estado de la instalación…\n")
	case screenMenu:
		b.WriteString(m.viewMenu())
	case screenSeedForm, screenStaffForm, screenTransferForm, screenSelfForm, screenHermesForm:
		b.WriteString(m.viewForm())
	case screenSeedRecovery, screenStaffPicker, screenDeactivatePicker, screenRolePicker, screenTransferPicker:
		b.WriteString(m.viewPicker())
	case screenAccountsTable:
		b.WriteString(m.viewTable())
	case screenConfirm:
		b.WriteString(m.viewConfirm())
	case screenInfo:
		b.WriteString(m.viewInfo())
	case screenHermesSnippet:
		b.WriteString(m.viewHermesSnippet())
	}

	b.WriteString("\n" + subtleStyle.Render(m.hintLine()) + "\n")
	return b.String()
}

// viewMenu renders the numbered capabilities plus the quit option.
func (m AppModel) viewMenu() string {
	items := menuItems()
	var b strings.Builder
	b.WriteString("¿Qué querés hacer?\n")
	for i, item := range items {
		b.WriteString(rowMarker(i == m.menuIndex))
		b.WriteString(strconv.Itoa(i+1) + ") " + item.label + "\n")
	}
	b.WriteString(subtleStyle.Render("  q) Salir") + "\n")
	return b.String()
}

// viewForm renders the current wizard step with its validation error.
func (m AppModel) viewForm() string {
	if len(m.form.steps) == 0 || m.form.index >= len(m.form.steps) {
		return ""
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(m.form.title) + "\n")
	if m.screen == screenSeedForm {
		b.WriteString(subtleStyle.Render(seedIntro) + "\n")
	}
	if m.screen == screenHermesForm {
		b.WriteString(subtleStyle.Render("Endpoint: "+m.deps.Hermes.EndpointURL) + "\n")
	}
	step := m.form.steps[m.form.index]
	b.WriteString(step.label + m.form.stepCounter() + "\n")
	b.WriteString(m.form.input.View() + "\n")
	if m.form.err != "" {
		b.WriteString(errStyle.Render("Error: "+m.form.err) + "\n")
	}
	return b.String()
}

// viewPicker renders the cursor-driven list of the active selection screen. The
// window is bounded (pickerWindow) so a long professional list stays navigable.
func (m AppModel) viewPicker() string {
	labels := m.pickerLabels()

	var b strings.Builder
	b.WriteString(titleStyle.Render(m.pickerTitle()) + "\n")
	switch m.screen {
	case screenSeedRecovery:
		b.WriteString(subtleStyle.Render(seedRecoveryIntro) + "\n")
	case screenTransferPicker:
		b.WriteString("Owner actual: " + admin.AccountLabel(m.owner) + "\n")
	}

	start, end := pickerWindow(len(labels), m.cursor, maxPickerRows)
	if start > 0 {
		b.WriteString(subtleStyle.Render("↑ hay más opciones") + "\n")
	}
	for i := start; i < end; i++ {
		b.WriteString(rowMarker(i == m.cursor) + labels[i] + "\n")
	}
	if end < len(labels) {
		b.WriteString(subtleStyle.Render("↓ hay más opciones") + "\n")
	}
	return b.String()
}

// viewTable renders the read-only account table inside its viewport.
func (m AppModel) viewTable() string {
	scope := "activas"
	if m.tableFilter.IncludeInactive {
		scope = "activas e inactivas"
	}

	var b strings.Builder
	b.WriteString(titleStyle.Render(m.tableTitle) + "\n")
	if col := scopeColumn(m.tableFilter); col != "" {
		b.WriteString(subtleStyle.Render("Rol: "+col) + "\n")
	}
	b.WriteString(subtleStyle.Render("Mostrando cuentas "+scope) + "\n")
	b.WriteString(m.viewport.View() + "\n")
	return b.String()
}

// viewConfirm renders the confirmation of a destructive action: what will
// happen, the extra consequence when there is one, and the explicit answer.
func (m AppModel) viewConfirm() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(m.confirm.title) + "\n")
	if m.confirm.warning != "" {
		b.WriteString(noticeStyle.Render(m.confirm.warning) + "\n")
	}
	for _, detail := range m.confirm.details {
		b.WriteString(subtleStyle.Render(detail) + "\n")
	}
	b.WriteString(m.confirm.question + "\n")
	b.WriteString(subtleStyle.Render(confirmHint) + "\n")
	return b.String()
}

// viewHermesSnippet renders the manual fallback of the Hermes bootstrap: the
// semantic failure is already on screen (errLine), and this adds the exact YAML
// block and the file it belongs in, so the operator copies a working value
// instead of reconstructing it.
func (m AppModel) viewHermesSnippet() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render(hermesSnippetTitle) + "\n")
	b.WriteString(subtleStyle.Render(hermesSnippetIntro) + "\n")
	b.WriteString(subtleStyle.Render(m.deps.Hermes.Path) + "\n\n")
	b.WriteString(m.hermesSnippet)
	if !strings.HasSuffix(m.hermesSnippet, "\n") {
		b.WriteString("\n")
	}
	return b.String()
}

// viewInfo renders the outcome of a flow. Every kind is recoverable to the menu.
func (m AppModel) viewInfo() string {
	if len(m.infoLines) == 0 {
		return ""
	}

	style := successStyle
	switch m.infoKind {
	case infoError:
		style = errStyle
	case infoNotice:
		style = noticeStyle
	}

	lines := make([]string, 0, len(m.infoLines))
	for _, line := range m.infoLines {
		lines = append(lines, style.Render(line))
	}
	return strings.Join(lines, "\n") + "\n"
}

// pickerTitle names the active selection screen.
func (m AppModel) pickerTitle() string {
	switch m.screen {
	case screenSeedRecovery:
		return "Recuperar el acceso de owner"
	case screenStaffPicker:
		return "Profesionales activos"
	case screenDeactivatePicker:
		return deactivatePickerHint
	case screenRolePicker:
		return rolePickerTitle
	case screenTransferPicker:
		return "Paso 1 de 2: elegir el sucesor"
	default:
		return ""
	}
}

// hintLine is the key hint of the active screen.
func (m AppModel) hintLine() string {
	switch m.screen {
	case screenBoot:
		return "Ctrl+C: salir"
	case screenMenu:
		return "↑/↓ y Enter, o el número de la opción · q: salir"
	case screenSeedForm:
		if len(m.inactive) > 0 {
			return "Enter: confirmar el campo · Esc: volver a las opciones de recuperación"
		}
		return "Enter: confirmar el campo · Esc: salir sin crear nada"
	case screenSeedRecovery:
		return "↑/↓ y Enter: elegir · Esc: salir sin crear nada"
	case screenStaffForm, screenTransferForm, screenSelfForm, screenHermesForm:
		return "Enter: confirmar el campo · Esc: volver al menú"
	case screenConfirm:
		return confirmHint + " · Esc: volver al menú"
	case screenInfo:
		return "Enter o Esc: volver al menú"
	case screenHermesSnippet:
		return "Enter o Esc: volver al menú"
	case screenAccountsTable:
		return "↑/↓: desplazar · i: incluir inactivas · Esc: volver al menú · q: salir"
	default:
		return "↑/↓ y Enter: elegir · Esc: volver al menú · q: salir"
	}
}

// rowMarker renders the cursor marker of a list row.
func rowMarker(selected bool) string {
	if selected {
		return selectedStyle.Render("› ")
	}
	return "  "
}

// tableTitle names a list view from its filter.
func tableTitle(filter admin.ListFilter) string {
	title := "Todas las cuentas"
	if filter.Role != "" {
		title = "Cuentas con rol " + string(filter.Role)
	}
	if filter.IncludeInactive {
		return title + " (incluye inactivas)"
	}
	return title
}

// scopeColumn names the role filter of a table view, or the empty string when
// the view covers every role.
func scopeColumn(filter admin.ListFilter) string {
	return string(filter.Role)
}

// busyNow reports whether a core call is in flight. The boot screen counts as
// busy from the first frame: the gate reads the installation and may write the
// caller-id file, so leaving during it is refused exactly like a mid-write exit.
func (m AppModel) busyNow() bool {
	return m.busy || m.screen == screenBoot
}
