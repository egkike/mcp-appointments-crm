package tui

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/egkike/mcp-appointments-crm/internal/admin"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// Terminal geometry defaults and bounds. The viewport needs a size before the
// first tea.WindowSizeMsg arrives, and a terminal smaller than the minimum would
// render an unusable screen, so the model clamps instead of trusting it.
const (
	defaultWidth  = 80
	defaultHeight = 24
	minWidth      = 40
	minHeight     = 15
	chromeLines   = 10
	maxPickerRows = 12
)

// screen enumerates the states of the TUI state machine. The zero value is the
// boot screen, so a zero AppModel renders something honest instead of an empty
// frame.
type screen int

const (
	screenBoot screen = iota
	screenSeedForm
	screenSeedRecovery
	screenMenu
	screenStaffPicker
	screenStaffForm
	screenDeactivatePicker
	screenRolePicker
	screenAccountsTable
	screenTransferPicker
	screenTransferForm
	screenSelfForm
	screenConfirm
	screenInfo
	screenHermesForm
	screenHermesSnippet
	screenAuditTrail
)

// Step indexes of the Hermes wizard. The steps themselves are built by
// hermesSteps (view.go), because the label and validator copy is shared with the
// console presentation; the indexes are named here because the skip and removal
// semantics of the optional Telegram group (afterFormAdvance) and runForm reason
// about them. TestHermesStepIndexesMatchTheSharedWizard pins the two against each
// other.
const (
	hermesServerPhoneStep = 0
	hermesTokenStep       = 1
	// hermesRemoveStep is the explicit "sin Telegram" question (S11 unit B). It
	// is only reached when the token answer was empty; afterFormAdvance jumps
	// over it otherwise.
	hermesRemoveStep     = 2
	hermesChatIDStep     = 3
	hermesOwnerPhoneStep = 4
)

// hermesTelegramOptionalNotice tells the operator, before the Telegram question
// is asked, that the channel is optional: the token field cannot carry that word
// (its label is the shared console/wizard copy), and without the hint an empty
// answer would look like a mistake instead of a decision. It also announces the
// removal question, so the next screen is never a surprise (S11 unit B).
const hermesTelegramOptionalNotice = "El bot de Telegram es opcional: dejá el token vacío y Enter para deshabilitar el canal (después elegís si retirar el bloque existente o conservarlo)."

// confirmAction identifies the write the confirmation screen gates. One screen
// with an explicit action keeps every destructive flow behind the same gate
// instead of four copies of the same yes/no handler.
type confirmAction int

const (
	confirmNone confirmAction = iota
	confirmReactivate
	confirmDeactivate
	confirmTransfer
	confirmHermesConfig
	confirmSkillRegen
	confirmPurge
	// confirmPurgeFinal is the second gate of the purge chain: the first
	// screen announced the batch, this one closes it with the permanent
	// consequence spelled out (S3: extra confirmation, both flows).
	confirmPurgeFinal
)

// infoKind distinguishes the outcome of a flow on the shared info screen: a
// plain notice, a success or a business failure. All of them are recoverable to
// the menu.
type infoKind int

const (
	infoNotice infoKind = iota
	infoSuccess
	infoError
)

// transferOption is one numbered successor candidate of the transfer picker: an
// existing account row (inactive owner or staff) or the "new phone" escape
// hatch. The eligibility of every candidate is re-validated by the core before
// anything is written.
type transferOption struct {
	kind  admin.SuccessorKind
	id    string
	label string
}

// confirmState is what the confirmation screen asks and what it will run when
// the operator answers yes.
type confirmState struct {
	action   confirmAction
	title    string
	question string
	warning  string
	// details are the plain facts of a non-destructive confirmation (the
	// Hermes endpoint, phone and target file). Empty for the account flows.
	details []string
	target  admin.AccountView
	// ids carries the batch the purge gates confirm (feat-tui-ops S3): the
	// two-step chain re-shares the exact same ids, never re-deriving them
	// between gates.
	ids []string
}

// busyLine is the answer to Esc, q or Ctrl+C while a core call is in flight: the
// write is already on its way to the database, so the TUI refuses to leave
// before it knows the outcome (ADR-0016 §5 key bindings).
const busyLine = "Hay una operación en curso: esperá a que termine antes de salir."

// AppModel is the top-level Bubble Tea model: it owns the screen state, the
// flows' drafts and the operator-facing copy. Update only performs state
// transitions and validation; View renders purely from that state, and every
// read and write of the installation goes through internal/admin inside a
// tea.Cmd.
type AppModel struct {
	ctx  context.Context
	deps Deps

	width  int
	height int

	screen  screen
	busy    bool
	spinner spinner.Model

	// errLine is the semantic error of the current screen: a rejected input or a
	// failed core call. Every screen renders it, so a failure never hides.
	errLine string
	// noticeLine is the non-error outcome of the previous action (a cancellation
	// or a repaired caller id).
	noticeLine string

	// info screen
	infoLines []string
	infoKind  infoKind

	// menu
	menuIndex int

	// role picker
	roles []entity.AccountRole

	// the wizard of the active flow (seed, add staff, new successor, add self)
	form formModel

	// cursor of the active picker
	cursor int

	// data loaded from the core
	professionals []admin.PickableProfessional
	accounts      []admin.AccountView
	inactive      []admin.AccountView
	successors    []transferOption
	owner         admin.AccountView

	// read-only table view
	tableFilter admin.ListFilter
	// auditEvents holds the read-only audit page of the audit view screen
	// (feat-tui-ops-residuals S1): the rows the viewport renders, newest first.
	auditEvents []admin.AuditEventView
	tableTitle  string
	viewport    viewport.Model

	// gatePassed is true only once the seed gate confirmed an active owner. It
	// gates the menu: without an owner there is nothing to operate on.
	gatePassed bool

	// flow drafts
	submittedPhone string
	professional   admin.PickableProfessional
	successor      transferOption
	transferDraft  admin.TransferSuccessor
	confirm        confirmState

	// hermesDraft is the phone of the server entry, the Telegram owner-bot field
	// group and the fallback snippet of the "Configurar Hermes" flow (ADR-0017,
	// ADR-0018 Decision 3b). The endpoint and the target file come from the
	// composition root through deps.Hermes and are never derived here.
	hermesPhone    string
	hermesTelegram TelegramFields
	hermesSnippet  string

	// skillRegen holds the resolved inputs (Hermes home, business name, template
	// bytes) of the receptionist-skill regeneration cascade started right after
	// a successful Hermes config write (ADR-0020 §4). It lives only between two
	// screens of the same decision; menuScreen resets it.
	skillRegen skillInputs
}

// NewAppModel builds the initial model. ctx is propagated into every core call
// (cancellation and timeouts), and deps carries the identity repositories built
// by the composition root.
func NewAppModel(ctx context.Context, deps Deps) AppModel {
	return AppModel{
		ctx:      ctx,
		deps:     deps,
		width:    defaultWidth,
		height:   defaultHeight,
		spinner:  spinner.New(spinner.WithSpinner(spinner.Dot)),
		viewport: viewport.New(defaultWidth, tableHeight(defaultHeight)),
	}
}

// Init opens the seed gateway: the first screen is always the boot sequence, so
// a clean install cannot fall through to a menu that has no owner to operate on.
// The boot screen counts as busy (busyNow), so the spinner and the quit guard
// cover the gate without a second state field.
func (m AppModel) Init() tea.Cmd {
	return tea.Batch(loadGateCmd(m.ctx, m.deps), m.spinner.Tick)
}

// Update routes one message to the state transition it belongs to. Framework
// bookkeeping messages (window resizes, spinner ticks, text-field blinks) are
// handled here or forwarded to the focused component; nothing in Update calls
// the database directly.
func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = max(msg.Width, minWidth)
		m.height = max(msg.Height, minHeight)
		m.viewport.Width = m.contentWidth()
		m.viewport.Height = tableHeight(m.height)
		return m, nil

	case spinner.TickMsg:
		if !m.busyNow() {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case gateMsg:
		return m.onGate(msg)
	case professionalsMsg:
		return m.onProfessionals(msg)
	case accountsMsg:
		return m.onAccounts(msg)
	case auditMsg:
		return m.onAudit(msg)
	case purgeDataMsg:
		return m.onPurgeData(msg)
	case transferDataMsg:
		return m.onTransferData(msg)
	case selfOwnerMsg:
		return m.onSelfOwner(msg)
	case hermesDataMsg:
		return m.onHermesData(msg)
	case hermesResultMsg:
		return m.onHermesResult(msg)
	case skillStateMsg:
		return m.onSkillState(msg)
	case skillResultMsg:
		return m.onSkillResult(msg)
	case resultMsg:
		return m.onResult(msg)

	case tea.KeyMsg:
		return m.onKey(msg)

	default:
		return m.dispatchToComponent(msg)
	}
}

// ── Key handling ─────────────────────────────────────────────────────────

// onKey applies the global key bindings before the per-screen routing:
//
//   - q and Ctrl+C quit, except inside a text field where q is a character and
//     while a core call is in flight, where leaving would hide its outcome;
//   - Esc goes back to the menu, never mid-write;
//   - every other key belongs to the active screen.
func (m AppModel) onKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	name := key.String()

	if name == "ctrl+c" || (!m.typing() && name == keyQuit) {
		return m.quit()
	}

	if name == "esc" {
		if m.busyNow() {
			m.errLine = busyLine
			return m, nil
		}
		return m.goBack()
	}

	return m.routeKey(key)
}

// quit implements the safe quit: it refuses to leave while a write is in flight.
func (m AppModel) quit() (tea.Model, tea.Cmd) {
	if m.busyNow() {
		m.errLine = busyLine
		return m, nil
	}
	return m, tea.Quit
}

// typing reports whether the active screen routes keys into a text field, so
// `q` stays a plain character while the operator types a name.
func (m AppModel) typing() bool {
	switch m.screen {
	case screenSeedForm, screenStaffForm, screenTransferForm, screenSelfForm, screenHermesForm:
		return true
	default:
		return false
	}
}

// goBack implements Esc. The seed screens exit instead of opening the menu: the
// seed gateway is the mandatory first screen and a menu without an active owner
// could not operate. A failure that happened before the gateway was satisfied
// re-runs the gate instead of open/close the menu.
func (m AppModel) goBack() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenSeedForm:
		// The wizard reached through the recovery picker goes back to that choice:
		// Esc must never end the session when another sanctioned path is one key
		// away. A clean install has no such path, so Esc exits without writing.
		if len(m.inactive) > 0 {
			m.screen = screenSeedRecovery
			m.cursor = 0
			m.form = formModel{}
			return m, nil
		}
		return m, tea.Quit
	case screenBoot, screenSeedRecovery:
		return m, tea.Quit
	case screenMenu:
		m.noticeLine = "Para salir usá q o Ctrl+C."
		return m, nil
	case screenInfo:
		if !m.gatePassed {
			return m.retryGate()
		}
		return m.menuScreen(""), nil
	default:
		return m.menuScreen(""), nil
	}
}

// routeKey dispatches a non-global key to the active screen.
func (m AppModel) routeKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenMenu:
		return m.menuKey(key)
	case screenInfo:
		if key.String() == "enter" {
			return m.goBack()
		}
		return m, nil
	case screenHermesSnippet:
		if key.String() == "enter" {
			return m.goBack()
		}
		return m, nil
	case screenSeedForm, screenStaffForm, screenTransferForm, screenSelfForm, screenHermesForm:
		if key.String() == "enter" {
			return m.submitForm()
		}
		return m, m.form.update(key)
	case screenSeedRecovery, screenStaffPicker, screenDeactivatePicker, screenRolePicker, screenTransferPicker:
		return m.pickerKey(key)
	case screenConfirm:
		return m.confirmKey(key)
	case screenAccountsTable:
		if key.String() == "i" {
			return m.toggleInactive()
		}
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(key)
		return m, cmd
	case screenAuditTrail:
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(key)
		return m, cmd
	default:
		return m, nil
	}
}

// menuKey navigates the menu with the arrows (or j/k) and Enter, and accepts the
// numbered shortcut, so both the vim-inclined and the numeric operator are served.
func (m AppModel) menuKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	count := len(menuItems())
	switch name := key.String(); name {
	case "up", "k":
		m.menuIndex = (m.menuIndex + count - 1) % count
		return m, nil
	case "down", "j":
		m.menuIndex = (m.menuIndex + 1) % count
		return m, nil
	case "enter":
		return m.openMenuItem(m.menuIndex)
	default:
		if index, err := strconv.Atoi(name); err == nil && index >= 1 && index <= count {
			return m.openMenuItem(index - 1)
		}
		return m, nil
	}
}

// pickerKey moves the cursor of the active picker and selects with Enter.
func (m AppModel) pickerKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	count := m.pickerCount()
	if count == 0 {
		return m, nil
	}

	switch key.String() {
	case "up", "k":
		m.cursor = (m.cursor + count - 1) % count
		return m, nil
	case "down", "j":
		m.cursor = (m.cursor + 1) % count
		return m, nil
	case "enter":
		return m.pick()
	default:
		return m, nil
	}
}

// confirmKey gates every destructive action. Only an explicit s/y is consent and
// only n cancels: any other key is ignored with the hint on screen, because a
// typo must never be read as a confirmation.
func (m AppModel) confirmKey(key tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch strings.ToLower(key.String()) {
	case "s", "y":
		return m.runConfirmed()
	case "n":
		return m.menuScreen(cancelNotice(m.confirm.action)), nil
	default:
		m.errLine = confirmHint
		return m, nil
	}
}

// runConfirmed dispatches the write the operator just confirmed.
func (m AppModel) runConfirmed() (tea.Model, tea.Cmd) {
	switch m.confirm.action {
	case confirmReactivate:
		return m.started(reactivateOwnerCmd(m.ctx, m.deps, m.confirm.target.ID))
	case confirmDeactivate:
		return m.started(deactivateCmd(m.ctx, m.deps, m.confirm.target.ID))
	case confirmTransfer:
		return m.started(transferCmd(m.ctx, m.deps, m.owner.ID, m.transferDraft))
	case confirmHermesConfig:
		return m.started(hermesConfigCmd(m.deps.Hermes.HermesConfig, m.hermesPhone, m.hermesTelegram))
	case confirmSkillRegen:
		// Reemplazo confirmado de una skill modificada por el usuario.
		return m.started(regenSkillCmd(m.skillRegen, true))
	case confirmPurge:
		// Primer gate aceptado: el segundo re-confirma con la consecuencia
		// permanente explícita, con los MISMOS ids capturados al inicio.
		return m.askConfirm(confirmState{
			action:   confirmPurgeFinal,
			title:    confirmPurgeTitle,
			question: fmt.Sprintf("ÚLTIMA CONFIRMACIÓN: la eliminación es permanente y sin vuelta atrás. Repito: %s. ¿Confirmar?", trimQuestionTail(m.confirm.question)),
			ids:      m.confirm.ids,
		}), nil
	case confirmPurgeFinal:
		return m.started(purgeFinalCmd(m.ctx, m.deps, m.confirm.ids))
	default:
		return m.menuScreen(""), nil
	}
}

// submitForm validates the current wizard step: an invalid answer keeps the
// operator on the step with the semantic error rendered, an accepted answer that
// completes an optional field group may close the wizard early (see
// afterFormAdvance) and the last valid answer dispatches the core call of the
// flow.
func (m AppModel) submitForm() (tea.Model, tea.Cmd) {
	switch result := m.form.submit(); result {
	case formRejected:
		return m, nil
	case formAdvanced:
		return m.afterFormAdvance(), nil
	case formDone:
		return m.runForm()
	default:
		return m, nil
	}
}

// afterFormAdvance applies the skip and removal semantics of the optional field
// group (S11). The Hermes wizard collects the Telegram owner-bot group in the
// steps that follow the server phone; the token answer decides which shape the
// group takes.
//
//   - A TOKEN answer means the operator configures the channel: the explicit
//     removal question does not apply, so the wizard jumps over it and keeps
//     collecting the required pair (chat id + owner phone).
//   - An EMPTY token closes the group, so the chat id and the owner phone are
//     never asked and the removal question decides between skipping (keep what
//     the config has) and removing (delete the block). Reaching the chat-id step
//     from the removal question is what proves the group was closed.
//
// Every other advance keeps the wizard on the next step, so the required pair is
// still collected and validated whole.
func (m AppModel) afterFormAdvance() tea.Model {
	if m.screen != screenHermesForm {
		return m
	}

	// One name for the polarity both guards below share: the group is OPEN when
	// the token step holds a value, and CLOSED when it was answered empty (B15
	// C2 — the two guards used to re-derive it inline with opposite negations).
	telegramTokenAnswered := m.form.value(hermesTokenStep) != ""

	// The token step was just answered: a token means the channel is being
	// configured, so the removal question does not apply and the wizard jumps to
	// the chat id.
	if m.form.index == hermesRemoveStep && telegramTokenAnswered {
		return m.formJump(hermesChatIDStep)
	}
	// Only a closed group (empty token) that just left the removal question
	// reaches this point with the chat-id index focused; every other advance
	// keeps walking the wizard.
	if m.form.index != hermesChatIDStep || telegramTokenAnswered {
		return m
	}

	m.hermesPhone = m.form.value(hermesServerPhoneStep)
	if telegramRemovalRequested(m.form.value(hermesRemoveStep)) {
		m.hermesTelegram = TelegramRemoval()
	} else {
		// The group stays empty on purpose: no token, no chat id and no owner
		// phone are written, so a configured block is preserved untouched.
		m.hermesTelegram = TelegramFields{}
	}
	return m.askConfirm(m.hermesConfirmState())
}

// formJump moves the wizard to a later step, reusing the form's own focus so the
// echoed value, the sensitive mask and the step error are reset exactly like a
// normal advance. It exists for the one conditional question of the Hermes
// wizard (the removal path), which must not be asked when a token was typed.
func (m AppModel) formJump(index int) AppModel {
	m.form.index = index
	m.form.focus()
	return m
}

// hermesConfirmState builds the confirmation the operator reads before the
// Hermes write from the drafts collected so far. It is shared by the complete
// wizard path and the skipped-Telegram path, so the two can never render
// different facts for the same write.
func (m AppModel) hermesConfirmState() confirmState {
	return confirmState{
		action:   confirmHermesConfig,
		title:    confirmHermesTitle,
		question: confirmHermesQuestion,
		details: []string{
			"Endpoint: " + m.deps.Hermes.EndpointURL,
			"Teléfono (X-Caller-Id): " + m.hermesPhone,
			"Telegram: " + m.hermesTelegram.Summary(),
			"Archivo: " + m.deps.Hermes.Path,
		},
	}
}

// runForm maps the collected answers onto the core input type of the active
// flow. This is the only place where a draft becomes a write request, and it
// only ever passes the core's own input types.
func (m AppModel) runForm() (tea.Model, tea.Cmd) {
	switch m.screen {
	case screenSeedForm:
		input := admin.SeedInput{Phone: m.form.value(0), DisplayName: m.form.value(1)}
		m.submittedPhone = input.Phone
		return m.started(seedCmd(m.ctx, m.deps, input))

	case screenStaffForm:
		input := admin.StaffInput{
			ProfessionalID: m.professional.ID,
			Phone:          m.form.value(0),
			DisplayName:    m.form.value(1),
		}
		return m.started(addStaffCmd(m.ctx, m.deps, m.professional, input))

	case screenTransferForm:
		m.transferDraft = admin.TransferSuccessor{
			Kind:        admin.SuccessorNewPhone,
			Phone:       m.form.value(0),
			DisplayName: m.form.value(1),
		}
		return m.askConfirm(confirmState{
			action:   confirmTransfer,
			title:    confirmTransferTitle,
			question: confirmTransferQuestion,
			warning:  fmt.Sprintf("El nuevo owner será %s (%s).", m.form.value(1), m.form.value(0)),
		}), nil

	case screenSelfForm:
		return m.started(addSelfCmd(m.ctx, m.deps, m.owner.ID, m.form.value(0)))

	case screenHermesForm:
		m.hermesPhone = m.form.value(hermesServerPhoneStep)
		m.hermesTelegram = NewTelegramFields(
			m.form.value(hermesTokenStep),
			m.form.value(hermesChatIDStep),
			m.form.value(hermesOwnerPhoneStep),
		)
		return m.askConfirm(m.hermesConfirmState()), nil

	default:
		return m, nil
	}
}

// pick resolves the Enter of the active picker into the next screen or core call.
func (m AppModel) pick() (tea.Model, tea.Cmd) {
	count := m.pickerCount()
	if count == 0 {
		return m, nil
	}
	// The rows can shrink between two renders (a re-derived gate reloads the
	// deactivated owners, for instance), so a stale cursor is normalized before it
	// can index past the list.
	m.cursor %= count

	switch m.screen {
	case screenSeedRecovery:
		return m.pickSeedRecovery()
	case screenStaffPicker:
		m.professional = m.professionals[m.cursor]
		m = m.withScreen(screenStaffForm)
		m.form = newForm(addStaffTitle, staffSteps(m.professional.Phone))
		return m, nil
	case screenDeactivatePicker:
		return m.askConfirm(confirmState{
			action:   confirmDeactivate,
			title:    confirmDeactivateTitle,
			question: fmt.Sprintf("Esta acción desactiva la cuenta %s. ¿Confirmar?", admin.AccountLabel(m.accounts[m.cursor])),
			target:   m.accounts[m.cursor],
		}), nil
	case screenRolePicker:
		filter := admin.ListFilter{
			Role:            m.roles[m.cursor],
			IncludeInactive: m.tableFilter.IncludeInactive,
		}
		return m.started(accountsCmd(m.ctx, m.deps, accountTable, filter))
	case screenTransferPicker:
		return m.pickSuccessor()
	default:
		return m, nil
	}
}

// pickSeedRecovery handles the deadend recovery picker: reactivate the chosen
// deactivated owner row behind a confirmation, or open the seed wizard for a
// different phone.
func (m AppModel) pickSeedRecovery() (tea.Model, tea.Cmd) {
	if m.cursor >= len(m.inactive) {
		m = m.withScreen(screenSeedForm)
		m.form = newForm(seedTitle, seedSteps())
		return m, nil
	}

	candidate := m.inactive[m.cursor]
	return m.askConfirm(confirmState{
		action:   confirmReactivate,
		title:    confirmReactivateTitle,
		question: fmt.Sprintf("Se reactivará %s como owner activo. ¿Confirmar?", admin.AccountLabel(candidate)),
		target:   candidate,
	}), nil
}

// pickSuccessor resolves the successor picker. A new phone needs the seed-like
// wizard first; the two existing-account kinds go straight to the confirmation,
// and a staff promotion warns about the professional link it drops.
func (m AppModel) pickSuccessor() (tea.Model, tea.Cmd) {
	option := m.successors[m.cursor]
	m.successor = option

	if option.kind == admin.SuccessorNewPhone {
		m = m.withScreen(screenTransferForm)
		m.form = newForm(transferTitle, transferSteps())
		return m, nil
	}

	state := confirmState{
		action:   confirmTransfer,
		title:    confirmTransferTitle,
		question: confirmTransferQuestion,
	}
	if option.kind == admin.SuccessorStaff {
		state.warning = "Al promover una cuenta de staff, la cuenta pierde su vínculo con el profesional."
	}
	m.transferDraft = admin.TransferSuccessor{Kind: option.kind, ID: option.id}
	return m.askConfirm(state), nil
}

// toggleInactive re-runs the table query with the soft-deleted rows opted in or
// out. The list views stay read-only: this is the only key that changes them.
func (m AppModel) toggleInactive() (tea.Model, tea.Cmd) {
	filter := m.tableFilter
	filter.IncludeInactive = !filter.IncludeInactive
	return m.started(accountsCmd(m.ctx, m.deps, accountTable, filter))
}

// openMenuItem dispatches a menu entry. The menu is a declaration table
// (menuItems), so a new capability is one entry plus one screen.
func (m AppModel) openMenuItem(index int) (tea.Model, tea.Cmd) {
	items := menuItems()
	if index < 0 || index >= len(items) {
		return m, nil
	}

	switch items[index].capability {
	case capabilityAddStaff:
		return m.started(professionalsCmd(m.ctx, m.deps))
	case capabilityDeactivate:
		return m.started(accountsCmd(m.ctx, m.deps, accountPicker, admin.ListFilter{}))
	case capabilityListAll:
		return m.started(accountsCmd(m.ctx, m.deps, accountTable, admin.ListFilter{}))
	case capabilityListByRole:
		m = m.withScreen(screenRolePicker)
		m.roles = admin.ListableRoles()
		m.cursor = 0
		return m, nil
	case capabilityTransfer:
		return m.started(transferDataCmd(m.ctx, m.deps))
	case capabilityAddSelf:
		return m.started(selfOwnerCmd(m.ctx, m.deps))
	case capabilityConfigureHermes:
		return m.started(hermesDataCmd(m.ctx, m.deps))
	case capabilityAuditLog:
		return m.started(auditCmd(m.ctx, m.deps))
	case capabilityPurgeInactive:
		return m.started(purgeDataCmd(m.ctx, m.deps))
	default:
		return m, nil
	}
}

// dispatchToComponent forwards a message to the component that owns the screen:
// the focused text field of a wizard step, or the viewport of a table view.
func (m AppModel) dispatchToComponent(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.typing() {
		return m, m.form.update(msg)
	}
	if m.screen == screenAccountsTable {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
		return m, cmd
	}
	return m, nil
}

// ── Async results ────────────────────────────────────────────────────────

// onGate lands the boot sequence: menu when an active owner exists (repairing a
// missing caller-id file first), the recovery picker when only deactivated owner
// rows exist, and the seed wizard on a clean install.
func (m AppModel) onGate(msg gateMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		return m.withInfo(infoError, msg.err.Error()), nil
	}

	if !msg.needsSeed {
		m.gatePassed = true
		menu := m.menuScreen("")
		if msg.repaired {
			menu.noticeLine = "Se reparó el archivo caller-id para el owner existente."
		}
		return menu, nil
	}

	m.inactive = msg.inactive
	if len(msg.inactive) > 0 {
		// Keep any error line from the previous attempt: it explains why the
		// operator is looking at the recovery picker again.
		m.screen = screenSeedRecovery
		m.cursor = 0
		return m, nil
	}
	m.screen = screenSeedForm
	m.form = newForm(seedTitle, seedSteps())
	return m, nil
}

// onProfessionals fills the Add Staff picker. No active professional is a
// legitimate answer, not an error: there is nothing to link an account to.
func (m AppModel) onProfessionals(msg professionalsMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		return m.withInfo(infoError, msg.err.Error()), nil
	}
	if len(msg.items) == 0 {
		return m.withInfo(infoNotice, "No hay profesionales activos: no se puede crear una cuenta de staff."), nil
	}

	m = m.withScreen(screenStaffPicker)
	m.professionals = msg.items
	m.cursor = 0
	return m, nil
}

// onAccounts fills either the deactivate picker or the read-only table.
func (m AppModel) onAccounts(msg accountsMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		return m.withInfo(infoError, msg.err.Error()), nil
	}

	if msg.target == accountPicker {
		if len(msg.views) == 0 {
			return m.withInfo(infoNotice, "No hay cuentas activas para desactivar."), nil
		}
		m = m.withScreen(screenDeactivatePicker)
		m.accounts = msg.views
		m.cursor = 0
		return m, nil
	}

	m = m.withScreen(screenAccountsTable)
	m.accounts = msg.views
	m.tableFilter = msg.filter
	m.tableTitle = tableTitle(msg.filter)
	m.viewport.SetContent(m.tableContent())
	return m, nil
}

// onTransferData fills the transfer picker with the current owner and the
// successor candidates.
func (m AppModel) onTransferData(msg transferDataMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		// The flow needs an ACTIVE owner: re-deriving the gate either opens the
		// menu again or takes the operator to the seed gateway, instead of a menu
		// that could not operate.
		m.errLine = msg.err.Error()
		return m.retryGate()
	}

	m = m.withScreen(screenTransferPicker)
	m.owner = msg.owner
	m.successors = msg.options
	m.cursor = 0
	return m, nil
}

// onSelfOwner opens the add-self wizard once the caller id to register is known.
func (m AppModel) onSelfOwner(msg selfOwnerMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		// Same reasoning as the transfer picker: without an ACTIVE owner the
		// add-self flow has no caller id to register.
		m.errLine = msg.err.Error()
		return m.retryGate()
	}

	m.owner = msg.owner
	m = m.withScreen(screenSelfForm)
	m.form = newForm(selfTitle, []formStep{{label: clientNameLabel, validate: admin.ValidateDisplayName}})
	m.noticeLine = fmt.Sprintf("Se registrará tu cuenta (%s) como cliente del negocio.", m.owner.ID)
	return m, nil
}

// onHermesData opens the Hermes phone wizard once the active owner is known. A
// missing ACTIVE owner (the menu could only be open if the owner was deactivated
// after the gate ran) is a business failure: the flow needs an owner phone to
// prefill, so it re-derives the gate instead of continuing with an empty field.
// A bootstrap resolution failure (invalid MCP_BIND, unresolvable home) is
// action-scoped instead: the menu reopens with the error visible.
func (m AppModel) onHermesData(msg hermesDataMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		m.errLine = msg.err.Error()
		if msg.resolveFailed {
			return m.reopenMenu(), nil
		}
		return m.retryGate()
	}

	// The resolved facts live in the model's Hermes bootstrap state: the Hermes
	// screens render the promoted EndpointURL/Path fields from it, and the
	// confirmation hands them to the write command as explicit arguments instead
	// of the command reaching back into deps (R2-03). The resolver is kept so the
	// cached provider is reused on a later open.
	m.deps.Hermes.HermesConfig = msg.hermes
	m.owner = msg.owner
	m = m.withScreen(screenHermesForm)
	steps := hermesSteps(msg.owner.ID)
	// S11: the Telegram owner-bot token step accepts an empty answer (the
	// operator disabling the channel). The shared label stays untouched, so the
	// console prompt and the wizard keep reading the same field name; the hint
	// rides on the notice line, which this screen renders above the question.
	steps[hermesTokenStep].validate = optionalTelegramToken
	m.form = newForm(hermesTitle, steps)
	m.noticeLine = hermesTelegramOptionalNotice
	return m, nil
}

// onHermesResult lands the Hermes write. Success shows the written facts; a
// failure (the config could not be merged or written) opens the snippet screen
// with the exact YAML the operator can paste by hand (ADR-0017 Decision 1).
func (m AppModel) onHermesResult(msg hermesResultMsg) (tea.Model, tea.Cmd) {
	m.busy = false

	if msg.written {
		// Cascada (ADR-0020 §4): el write del config dispara la detección del
		// estado de la skill; el resultado llega por skillStateMsg y decide
		// auto-regeneración, confirmación o nada.
		return m.started(detectSkillCmd(m.ctx, m.deps))
	}

	m = m.withScreen(screenHermesSnippet)
	if msg.err != nil {
		m.errLine = msg.err.Error()
	}
	m.hermesSnippet = msg.snippet
	return m, nil
}

// onSkillState lands the detection of the installed skill against a fresh
// rendering. missing and up-to-date walk straight into the regeneration with no
// confirmation (there is no user content to protect); modified opens the
// explicit confirm screen: the installed skill may hold owner edits that must
// not be replaced without consent (ADR-0020 §4, spec "Modified skill").
// Detection or rendering failures surface on the info screen and end the
// cascade: the Hermes config write already succeeded.
func (m AppModel) onSkillState(msg skillStateMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	if msg.err != nil {
		return m.withInfo(infoError, "Configuración de Hermes actualizada.", skillOutcomeErrorLine(msg.err)), nil
	}
	m.skillRegen = msg.inputs
	switch msg.state {
	case admin.SkillStateMissing, admin.SkillStateUpToDate:
		return m.started(regenSkillCmd(msg.inputs, false))
	case admin.SkillStateModified:
		return m.askConfirm(confirmState{
			action:   confirmSkillRegen,
			title:    confirmSkillRegenTitle,
			question: confirmSkillRegenQuestion,
			warning:  confirmSkillRegenWarning,
		}), nil
	default:
		return m.withInfo(infoError, "Configuración de Hermes actualizada.", "El estado de la skill instalada es desconocido."), nil
	}
}

// onSkillResult lands the regeneration outcome and closes the option 7 cascade
// with one screen: the config facts, the skill summary token (the exact
// vocabulary install.sh reports) and the Hermes loader note.
func (m AppModel) onSkillResult(msg skillResultMsg) (tea.Model, tea.Cmd) {
	m.busy = false
	lines := []string{
		"Configuración de Hermes actualizada.",
		"Endpoint: " + m.deps.Hermes.EndpointURL,
		"Teléfono (X-Caller-Id): " + m.hermesPhone,
		"Telegram: " + m.hermesTelegram.Summary(),
		"Archivo: " + m.deps.Hermes.Path,
	}
	if msg.err != nil {
		return m.withInfo(infoError, append(lines, skillOutcomeErrorLine(msg.err))...), nil
	}
	return m.withInfo(infoSuccess, append(lines, resultLines(msg.outcome)...)...), nil
}

// onResult lands a write. A seed or recovery failure re-derives the gate instead
// of trusting the state the flow started from, which is what keeps the seed
// gateway loop honest: the operator cannot end up in the menu without an owner.
func (m AppModel) onResult(msg resultMsg) (tea.Model, tea.Cmd) {
	m.busy = false

	if msg.reloadGate {
		if msg.err != nil {
			m.errLine = m.seedErrorLine(msg.err)
		}
		return m.retryGate()
	}

	if msg.err != nil {
		lines := make([]string, 0, len(msg.lines)+1)
		lines = append(lines, msg.lines...)
		lines = append(lines, msg.err.Error())
		return m.withInfo(infoError, lines...), nil
	}
	return m.withInfo(infoSuccess, msg.lines...), nil
}

// seedErrorLine adds the sanctioned recovery to a seed collision: the phone
// belongs to a deactivated owner row, so the operator reads which row to
// reactivate instead of a PRIMARY KEY conflict. Only the seed wizard can
// produce that collision, so the hint is scoped to it.
func (m AppModel) seedErrorLine(err error) string {
	line := err.Error()
	if m.screen != screenSeedForm || !errors.Is(err, admin.ErrOwnerAlreadyExists) {
		return line
	}
	if hint := admin.ReactivationHint(m.inactive, m.submittedPhone); hint != "" {
		return line + "\n" + hint
	}
	return line
}

// ── State helpers ────────────────────────────────────────────────────────

// started marks the model busy, clears the previous outcome and dispatches an
// asynchronous core call together with the spinner tick. The model stays busy
// until the matching result message arrives, so Esc, q and Ctrl+C are refused
// for exactly as long as the write is in flight.
func (m AppModel) started(cmd tea.Cmd) (tea.Model, tea.Cmd) {
	m.busy = true
	m.errLine = ""
	m.noticeLine = ""
	return m, tea.Batch(cmd, m.spinner.Tick)
}

// retryGate re-runs the boot sequence keeping the error line that explains why.
func (m AppModel) retryGate() (tea.Model, tea.Cmd) {
	m.screen = screenBoot
	m.busy = true
	return m, tea.Batch(loadGateCmd(m.ctx, m.deps), m.spinner.Tick)
}

// reopenMenu returns to the operator menu keeping the error line that explains
// why the action could not open. Unlike retryGate it does not re-run the boot
// sequence: the installation state is unchanged, only the action's own
// dependency failed to resolve, so the error must stay visible on the menu.
func (m AppModel) reopenMenu() AppModel {
	errLine := m.errLine
	m = m.menuScreen("")
	m.errLine = errLine
	return m
}

// menuScreen returns to the menu, dropping the flow drafts: a cancelled or
// finished flow leaves nothing behind that the next flow could reuse by mistake.
func (m AppModel) menuScreen(notice string) AppModel {
	m = m.withScreen(screenMenu)
	m.noticeLine = notice
	m.busy = false
	m.confirm = confirmState{}
	m.cursor = 0
	m.menuIndex = 0
	m.form = formModel{}
	m.successor = transferOption{}
	m.transferDraft = admin.TransferSuccessor{}
	m.hermesPhone = ""
	m.hermesTelegram = TelegramFields{}
	m.hermesSnippet = ""
	m.skillRegen = skillInputs{}
	return m
}

// withScreen switches screens and drops the previous outcome lines: the operator
// is looking at something new. Flow drafts are kept, because the confirmation
// screen must be able to show what it is confirming.
func (m AppModel) withScreen(next screen) AppModel {
	m.screen = next
	m.errLine = ""
	m.noticeLine = ""
	return m
}

// withInfo switches to the shared info screen, which renders a notice, a success
// or a business failure. Enter and Esc always return to the menu (or re-run the
// gate when the gateway never completed), so no failure traps the operator.
func (m AppModel) withInfo(kind infoKind, lines ...string) AppModel {
	m = m.withScreen(screenInfo)
	m.infoKind = kind
	m.infoLines = lines
	return m
}

// askConfirm opens the confirmation screen of a destructive flow.
func (m AppModel) askConfirm(state confirmState) AppModel {
	m = m.withScreen(screenConfirm)
	m.confirm = state
	return m
}

// pickerCount returns the number of rows of the active picker.
func (m AppModel) pickerCount() int {
	switch m.screen {
	case screenSeedRecovery:
		return len(m.inactive) + 1
	case screenStaffPicker:
		return len(m.professionals)
	case screenDeactivatePicker:
		return len(m.accounts)
	case screenRolePicker:
		return len(m.roles)
	case screenTransferPicker:
		return len(m.successors)
	default:
		return 0
	}
}

// pickerLabels renders the rows of the active picker.
func (m AppModel) pickerLabels() []string {
	switch m.screen {
	case screenSeedRecovery:
		labels := make([]string, 0, len(m.inactive)+1)
		for _, view := range m.inactive {
			labels = append(labels, "Reactivar "+admin.AccountLabel(view))
		}
		return append(labels, "Crear un owner nuevo con otro teléfono")
	case screenStaffPicker:
		labels := make([]string, 0, len(m.professionals))
		for _, item := range m.professionals {
			labels = append(labels, fmt.Sprintf("%s (%s)", item.Name, item.ID))
		}
		return labels
	case screenDeactivatePicker:
		labels := make([]string, 0, len(m.accounts))
		for _, view := range m.accounts {
			labels = append(labels, admin.AccountLabel(view))
		}
		return labels
	case screenRolePicker:
		labels := make([]string, 0, len(m.roles))
		for _, role := range m.roles {
			labels = append(labels, string(role))
		}
		return labels
	case screenTransferPicker:
		labels := make([]string, 0, len(m.successors))
		for _, option := range m.successors {
			labels = append(labels, option.label)
		}
		return labels
	default:
		return nil
	}
}

// tableContent renders the rows of the read-only table view, or the semantic
// message of an empty result (an empty view is a legitimate answer, not an
// error).
func (m AppModel) tableContent() string {
	if len(m.accounts) == 0 {
		return emptyTableLine
	}
	return renderAccountTable(m.accounts)
}

// contentWidth and tableHeight size the viewport from the terminal geometry.
func (m AppModel) contentWidth() int {
	return max(m.width-2, minWidth)
}

// tableHeight reserves the header, the outcome lines and the key hints, keeping
// at least three rows of content visible.
func tableHeight(height int) int {
	return max(height-chromeLines, 3)
}

// trimQuestionTail removes the confirm tail (". ¿Confirmar?") the first gate
// appends, so the second gate can re-state the batch without a nested
// "¿Confirmar? ¿Confirmar?" echo.
func trimQuestionTail(question string) string {
	suffix := ". ¿Confirmar?"
	if strings.HasSuffix(question, suffix) {
		return strings.TrimSuffix(question, suffix)
	}
	return question
}

// purgeFinalCmd runs the batch behind the second gate (same ids, no
// re-derivation) and reports every row outcome.
func purgeFinalCmd(ctx context.Context, deps Deps, ids []string) tea.Cmd {
	return purgeCmd(ctx, deps, ids)
}

// cancelNotice phrases the cancellation of the confirmation the operator just
// declined. A cancellation is a decision, not an error.
func cancelNotice(action confirmAction) string {
	switch action {
	case confirmReactivate:
		return "Operación cancelada: no se reactivó ninguna cuenta."
	case confirmDeactivate:
		return "Operación cancelada: no se desactivó ninguna cuenta."
	case confirmTransfer:
		return "Operación cancelada: no se transfirió la propiedad."
	case confirmHermesConfig:
		return "Operación cancelada: no se cambió la configuración de Hermes."
	case confirmSkillRegen:
		return "Operación cancelada: no se tocó la skill instalada."
	case confirmPurge, confirmPurgeFinal:
		return "Operación cancelada: no se purgó ninguna cuenta."
	default:
		return "Operación cancelada."
	}
}
