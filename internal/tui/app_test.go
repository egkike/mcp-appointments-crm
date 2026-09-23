package tui

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/egkike/mcp-appointments-crm/internal/admin"
	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

// Fixture phones and names. They are valid phones for admin.ValidatePhone (4 to
// 15 digits with an optional leading '+').
const (
	ownerPhone = "+5491100000000"
	staffPhone = "+5491100000001"
	newPhone   = "+5491100000002"
	ownerName  = "Dueño"
	staffName  = "Ana Staff"
)

// ── Fixture ──────────────────────────────────────────────────────────────

// fixture is a throwaway installation: a real SQLite file (WAL rejects
// :memory:), a throwaway config directory and the production repositories the
// model drives. Verification reads through the same connections, so a test
// asserts the committed state and not a cached copy.
type fixture struct {
	dbPath     string
	configDir  string
	hermesPath string

	accounts      *repository.AccountsRepo
	professionals *repository.ProfessionalsRepo
	clients       *repository.ClientsRepo

	deps Deps
}

func newFixture(t *testing.T) fixture {
	t.Helper()

	f := fixture{
		dbPath:     filepath.Join(t.TempDir(), "appointments.db"),
		configDir:  filepath.Join(t.TempDir(), "config"),
		hermesPath: filepath.Join(t.TempDir(), ".hermes", admin.HermesConfigFileName),
	}
	t.Setenv("MCP_DB_PATH", f.dbPath)
	t.Setenv("MCP_CONFIG_DIR", f.configDir)

	database, err := db.NewDatabase(context.Background(), f.dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	t.Cleanup(func() {
		if cerr := database.Close(); cerr != nil {
			t.Errorf("closing the fixture database: %v", cerr)
		}
	})

	endpoint, err := admin.HermesEndpointURL("127.0.0.1", "3000")
	if err != nil {
		t.Fatalf("admin.HermesEndpointURL() failed: %v", err)
	}

	f.accounts = repository.NewAccountsRepo(database.Conn, slog.Default())
	f.professionals = repository.NewProfessionalsRepo(database.Conn)
	f.clients = repository.NewClientsRepo(database.Conn)
	f.deps = Deps{
		Accounts:      f.accounts,
		Professionals: f.professionals,
		Clients:       f.clients,
		Hermes:        HermesConfig{Path: f.hermesPath, EndpointURL: endpoint},
	}
	return f
}

// seedOwner creates the first owner through the reviewed core, exactly like the
// seed screen does.
func (f fixture) seedOwner(t *testing.T, phone, name string) {
	t.Helper()
	if err := admin.Seed(context.Background(), f.accounts, admin.SeedInput{Phone: phone, DisplayName: name}); err != nil {
		t.Fatalf("seed owner fixture: %v", err)
	}
}

// seedProfessional inserts one active professional and returns its id, so the
// Add Staff picker has a row to offer.
func (f fixture) seedProfessional(t *testing.T, name, phone string) string {
	t.Helper()
	professional := &entity.Professional{Name: name, Status: "active", Phone: &phone}
	if err := f.professionals.Save(admin.TUIContext(context.Background()), professional); err != nil {
		t.Fatalf("seed professional fixture: %v", err)
	}
	return professional.ID
}

// createStaff pre-creates a staff account so the pickers and the transfer flow
// have a second row.
func (f fixture) createStaff(t *testing.T, phone, name, professionalID string) {
	t.Helper()
	professional := professionalID
	err := f.accounts.Create(admin.TUIContext(context.Background()), &entity.Account{
		ID:             phone,
		Role:           entity.RoleStaff,
		DisplayName:    name,
		ProfessionalID: &professional,
		Active:         true,
	})
	if err != nil {
		t.Fatalf("seed staff fixture: %v", err)
	}
}

// deactivate soft-deletes one account through the repository, so the list views
// and the recovery screen have an inactive row.
func (f fixture) deactivate(t *testing.T, id string) {
	t.Helper()
	if err := f.accounts.Deactivate(admin.TUIContext(context.Background()), id); err != nil {
		t.Fatalf("deactivate fixture: %v", err)
	}
}

// ownersByRole reads the committed accounts of one role.
func (f fixture) byRole(t *testing.T, role entity.AccountRole) []*entity.Account {
	t.Helper()
	rows, err := f.accounts.GetByRole(admin.TUIContext(context.Background()), role)
	if err != nil {
		t.Fatalf("GetByRole(%s) failed: %v", role, err)
	}
	return rows
}

// account reads one committed account row.
func (f fixture) account(t *testing.T, id string) *entity.Account {
	t.Helper()
	row, err := f.accounts.FindByID(admin.TUIContext(context.Background()), id)
	if err != nil {
		t.Fatalf("FindByID(%s) failed: %v", id, err)
	}
	return row
}

// callerID reads the caller-id file the flows republish.
func (f fixture) callerID(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(f.configDir, admin.CallerIDFileName))
	if err != nil {
		t.Fatalf("read caller-id file: %v", err)
	}
	return strings.TrimSpace(string(content))
}

// hasCallerID reports whether the caller-id file exists.
func (f fixture) hasCallerID(t *testing.T) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(f.configDir, admin.CallerIDFileName))
	return err == nil
}

// hermesFile reads the Hermes config the flow wrote.
func (f fixture) hermesFile(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(f.hermesPath)
	if err != nil {
		t.Fatalf("read hermes config: %v", err)
	}
	return string(content)
}

// withOwnerAndStaff prepares the two-account installation the menu flows operate
// on: an active owner and an active staff account linked to a professional.
func withOwnerAndStaff(t *testing.T, f fixture) {
	t.Helper()
	f.seedOwner(t, ownerPhone, ownerName)
	professionalID := f.seedProfessional(t, "Ana", staffPhone)
	f.createStaff(t, staffPhone, staffName, professionalID)
}

// ── Driver ───────────────────────────────────────────────────────────────

// driver drives the model the way the Bubble Tea program does: it feeds key
// messages and executes the dispatched commands synchronously, so a model test
// observes the real message flow (core calls included) without a terminal.
type driver struct {
	t     *testing.T
	model AppModel
	quit  bool
}

func newDriver(t *testing.T, f fixture) *driver {
	t.Helper()
	d := &driver{t: t, model: newModel(f)}
	d.run(d.model.Init())
	return d
}

// newModel builds the model with a wide geometry, so table assertions are not
// affected by the viewport clipping columns.
func newModel(f fixture) AppModel {
	m := NewAppModel(context.Background(), f.deps)
	m.width, m.height = 120, 30
	m.viewport.Width = m.contentWidth()
	m.viewport.Height = tableHeight(m.height)
	return m
}

// send feeds one message to the model and executes everything it dispatches.
func (d *driver) send(msg tea.Msg) {
	d.t.Helper()
	next, cmd := d.model.Update(msg)
	model, ok := next.(AppModel)
	if !ok {
		d.t.Fatalf("Update() returned %T, want AppModel", next)
	}
	d.model = model
	d.run(cmd)
}

// run executes a command tree and feeds the resulting messages back to the
// model. Framework bookkeeping (spinner ticks, cursor blinks) is dropped: it has
// no bearing on the state under test and would loop forever.
func (d *driver) run(cmd tea.Cmd) {
	d.t.Helper()
	if cmd == nil {
		return
	}

	switch msg := cmd().(type) {
	case nil:
		return
	case tea.BatchMsg:
		for _, child := range msg {
			d.run(child)
		}
	case tea.QuitMsg:
		d.quit = true
	case gateMsg, professionalsMsg, accountsMsg, transferDataMsg, selfOwnerMsg, hermesDataMsg, hermesResultMsg, resultMsg:
		d.send(msg)
	default:
		// spinner.TickMsg, cursor blink messages and other framework traffic.
	}
}

// press sends one named key.
func (d *driver) press(name string) {
	d.t.Helper()
	d.send(testKey(name))
}

// typeText sends one key per rune, the way a terminal reports typing.
func (d *driver) typeText(text string) {
	d.t.Helper()
	for _, r := range text {
		d.send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

// view renders the current frame.
func (d *driver) view() string {
	d.t.Helper()
	return d.model.View()
}

// selectRow moves the active picker cursor onto the first row containing want, so
// a test never depends on repository row order.
func (d *driver) selectRow(want string) {
	d.t.Helper()

	for i, label := range d.model.pickerLabels() {
		if strings.Contains(label, want) {
			for j := 0; j < i; j++ {
				d.press("down")
			}
			return
		}
	}
	d.t.Fatalf("no picker row matching %q in %v", want, d.model.pickerLabels())
}

// testKey builds the key message of a named key.
func testKey(name string) tea.KeyMsg {
	switch name {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "esc":
		return tea.KeyMsg{Type: tea.KeyEsc}
	case "up":
		return tea.KeyMsg{Type: tea.KeyUp}
	case "down":
		return tea.KeyMsg{Type: tea.KeyDown}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(name)}
	}
}

// ── Seed gateway ─────────────────────────────────────────────────────────

func TestAppModelCleanInstallSeedsTheOwner(t *testing.T) {
	f := newFixture(t)
	d := newDriver(t, f)

	if d.model.screen != screenSeedForm {
		t.Fatalf("boot screen = %v, want the seed form", d.model.screen)
	}
	if view := d.view(); !strings.Contains(view, "Crear el owner de esta instalación") {
		t.Errorf("seed view = %q, want the seed title", view)
	}

	d.typeText(ownerPhone)
	d.press("enter")
	if d.model.form.index != 1 {
		t.Fatalf("form index = %d, want the display name step after a valid phone", d.model.form.index)
	}

	d.typeText(ownerName)
	d.press("enter")

	if d.model.screen != screenInfo || d.model.infoKind != infoSuccess {
		t.Fatalf("screen = %v kind = %v, want the success info screen", d.model.screen, d.model.infoKind)
	}
	if d.model.busy {
		t.Error("model is still busy after the seed result arrived")
	}
	view := d.view()
	for _, want := range []string{"Owner creado: " + ownerName, "caller-id escrito"} {
		if !strings.Contains(view, want) {
			t.Errorf("seed outcome view missing %q\n%s", want, view)
		}
	}

	owners := f.byRole(t, entity.RoleOwner)
	if len(owners) != 1 || owners[0].ID != ownerPhone || !owners[0].Active {
		t.Fatalf("owners = %+v, want exactly one active owner %q", owners, ownerPhone)
	}
	if got := f.callerID(t); got != ownerPhone {
		t.Errorf("caller-id = %q, want %q", got, ownerPhone)
	}

	// Acknowledging the outcome opens the menu.
	d.press("enter")
	if d.model.screen != screenMenu {
		t.Fatalf("screen after acknowledging the outcome = %v, want the menu", d.model.screen)
	}
}

func TestAppModelInvalidInputBlocksSeedProgress(t *testing.T) {
	f := newFixture(t)
	d := newDriver(t, f)

	d.typeText("abc")
	d.press("enter")

	if d.model.screen != screenSeedForm || d.model.form.index != 0 {
		t.Fatalf("screen = %v index = %d, want the seed form still on the phone step",
			d.model.screen, d.model.form.index)
	}
	if view := d.view(); !strings.Contains(view, "no es válido") {
		t.Errorf("view = %q, want the semantic phone error", view)
	}
	if len(f.byRole(t, entity.RoleOwner)) != 0 {
		t.Error("an invalid phone created an owner")
	}

	// The rejected answer is kept in the field so the operator can fix the typo,
	// and the step did not advance: a valid answer still completes it.
	for range d.model.form.input.Value() {
		d.press("backspace")
	}
	d.typeText(ownerPhone)
	d.press("enter")
	if d.model.form.index != 1 {
		t.Fatalf("form index = %d, want the display name step after a valid phone", d.model.form.index)
	}
}

func TestAppModelBlankDisplayNameIsRejected(t *testing.T) {
	f := newFixture(t)
	d := newDriver(t, f)

	d.typeText(ownerPhone)
	d.press("enter")
	d.press("enter") // empty display name

	if d.model.form.index != 1 {
		t.Fatalf("form index = %d, want the wizard to stay on the display name step", d.model.form.index)
	}
	if view := d.view(); !strings.Contains(view, "no puede estar vacío") {
		t.Errorf("view = %q, want the semantic blank-name error", view)
	}
	if len(f.byRole(t, entity.RoleOwner)) != 0 {
		t.Error("a blank display name created an owner")
	}
}

func TestAppModelActiveOwnerOpensTheMenuAndRepairsTheCallerID(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)
	if f.hasCallerID(t) {
		t.Fatal("precondition: the fixture must start without a caller-id file")
	}

	d := newDriver(t, f)

	if d.model.screen != screenMenu || !d.model.gatePassed {
		t.Fatalf("screen = %v gatePassed = %v, want the menu", d.model.screen, d.model.gatePassed)
	}
	if !strings.Contains(d.view(), "Se reparó el archivo caller-id") {
		t.Errorf("view = %q, want the caller-id repair notice", d.view())
	}
	if !f.hasCallerID(t) {
		t.Error("the boot sequence did not write the caller-id file")
	}
	if got := f.callerID(t); got != ownerPhone {
		t.Errorf("caller-id = %q, want the repaired %q", got, ownerPhone)
	}
	if len(f.byRole(t, entity.RoleOwner)) != 1 {
		t.Error("the boot sequence created a second owner")
	}
}

func TestAppModelExistingCallerIDIsNotRewritten(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)
	if _, err := admin.WriteCallerID(ownerPhone); err != nil {
		t.Fatalf("WriteCallerID() failed: %v", err)
	}

	d := newDriver(t, f)

	if strings.Contains(d.view(), "Se reparó") {
		t.Error("an existing caller-id file was repaired")
	}
}

func TestAppModelSeedConflictReOffersTheReactivation(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)
	f.deactivate(t, ownerPhone)

	d := newDriver(t, f)
	if d.model.screen != screenSeedRecovery {
		t.Fatalf("screen = %v, want the deactivated-owner recovery picker", d.model.screen)
	}

	// Creating a new owner with the deactivated phone collides with the row.
	d.selectRow("Crear un owner nuevo")
	d.press("enter")
	d.typeText(ownerPhone)
	d.press("enter")
	d.typeText("Otro Nombre")
	d.press("enter")

	if d.model.screen != screenSeedRecovery {
		t.Fatalf("screen after the conflict = %v, want the recovery picker again", d.model.screen)
	}
	view := d.view()
	if !strings.Contains(view, "no se creó ninguna cuenta nueva") {
		t.Errorf("view = %q, want the semantic seed conflict", view)
	}
	if !strings.Contains(view, "reactivala") {
		t.Errorf("view = %q, want the reactivation guidance", view)
	}
	if len(f.byRole(t, entity.RoleOwner)) != 1 {
		t.Error("the conflicted seed created a second owner row")
	}

	// The recovery wizard goes back to the choice instead of ending the session.
	d.selectRow("Crear un owner nuevo")
	d.press("enter")
	if d.model.screen != screenSeedForm {
		t.Fatalf("screen = %v, want the seed wizard of the recovery flow", d.model.screen)
	}
	d.press("esc")
	if d.model.screen != screenSeedRecovery {
		t.Fatalf("screen after Esc = %v, want the recovery picker", d.model.screen)
	}
	if d.quit {
		t.Error("Esc in the recovery wizard ended the session")
	}

	// The recovery path reactivates the existing row behind a confirmation.
	d.selectRow("Reactivar")
	d.press("enter")
	if d.model.screen != screenConfirm || d.model.confirm.action != confirmReactivate {
		t.Fatalf("screen = %v action = %v, want the reactivation confirmation",
			d.model.screen, d.model.confirm.action)
	}
	d.press("s")

	if d.model.infoKind != infoSuccess {
		t.Fatalf("kind = %v, want the success info screen", d.model.infoKind)
	}
	owner := f.account(t, ownerPhone)
	if !owner.Active {
		t.Error("the reactivation did not activate the owner row")
	}
	if got := f.callerID(t); got != ownerPhone {
		t.Errorf("caller-id = %q, want %q", got, ownerPhone)
	}
}

// ── Menu and navigation ──────────────────────────────────────────────────

func TestAppModelMenuReachesEveryFlow(t *testing.T) {
	tests := []struct {
		name       string
		install    func(t *testing.T, f fixture)
		keys       []string
		wantScreen screen
		wantCopy   string
	}{
		{
			name:       "add staff opens the professional picker",
			install:    withOwnerAndStaff,
			keys:       []string{"1"},
			wantScreen: screenStaffPicker,
			wantCopy:   "Profesionales activos",
		},
		{
			name:       "add staff without active professionals is a notice",
			install:    func(t *testing.T, f fixture) { t.Helper(); f.seedOwner(t, ownerPhone, ownerName) },
			keys:       []string{"1"},
			wantScreen: screenInfo,
			wantCopy:   "No hay profesionales activos",
		},
		{
			name:       "deactivate opens the active-account picker",
			install:    withOwnerAndStaff,
			keys:       []string{"2"},
			wantScreen: screenDeactivatePicker,
			wantCopy:   "Elegí la cuenta a desactivar",
		},
		{
			name:       "list all accounts opens the table",
			install:    func(t *testing.T, f fixture) { t.Helper(); f.seedOwner(t, ownerPhone, ownerName) },
			keys:       []string{"3"},
			wantScreen: screenAccountsTable,
			wantCopy:   "Todas las cuentas",
		},
		{
			name:       "list by role opens the role picker",
			install:    func(t *testing.T, f fixture) { t.Helper(); f.seedOwner(t, ownerPhone, ownerName) },
			keys:       []string{"4"},
			wantScreen: screenRolePicker,
			wantCopy:   "Elegí el rol",
		},
		{
			name:       "transfer opens the successor picker",
			install:    withOwnerAndStaff,
			keys:       []string{"5"},
			wantScreen: screenTransferPicker,
			wantCopy:   "Paso 1 de 2: elegir el sucesor",
		},
		{
			name:       "add self opens the client name wizard",
			install:    func(t *testing.T, f fixture) { t.Helper(); f.seedOwner(t, ownerPhone, ownerName) },
			keys:       []string{"6"},
			wantScreen: screenSelfForm,
			wantCopy:   "Agregarme como cliente",
		},
		{
			name:       "arrow navigation selects a menu entry",
			install:    func(t *testing.T, f fixture) { t.Helper(); f.seedOwner(t, ownerPhone, ownerName) },
			keys:       []string{"down", "enter"},
			wantScreen: screenDeactivatePicker,
			wantCopy:   "Elegí la cuenta a desactivar",
		},
		{
			name:       "an unknown numbered option is ignored",
			install:    func(t *testing.T, f fixture) { t.Helper(); f.seedOwner(t, ownerPhone, ownerName) },
			keys:       []string{"9"},
			wantScreen: screenMenu,
			wantCopy:   "¿Qué querés hacer?",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			tt.install(t, f)

			d := newDriver(t, f)
			for _, key := range tt.keys {
				d.press(key)
			}

			if d.model.screen != tt.wantScreen {
				t.Fatalf("screen = %v, want %v", d.model.screen, tt.wantScreen)
			}
			if view := d.view(); !strings.Contains(view, tt.wantCopy) {
				t.Errorf("view missing %q\n%s", tt.wantCopy, view)
			}
		})
	}
}

func TestAppModelEscReturnsToTheMenu(t *testing.T) {
	tests := []struct {
		name    string
		install func(t *testing.T, f fixture)
		open    func(d *driver)
	}{
		{
			name:    "from the staff picker",
			install: withOwnerAndStaff,
			open:    func(d *driver) { d.press("1") },
		},
		{
			name:    "from the staff wizard",
			install: withOwnerAndStaff,
			open: func(d *driver) {
				d.press("1")
				d.selectRow("Ana")
				d.press("enter")
			},
		},
		{
			name:    "from the deactivate picker",
			install: withOwnerAndStaff,
			open:    func(d *driver) { d.press("2") },
		},
		{
			name:    "from the confirmation screen",
			install: withOwnerAndStaff,
			open: func(d *driver) {
				d.press("2")
				d.selectRow(staffName)
				d.press("enter")
			},
		},
		{
			name:    "from the table view",
			install: func(t *testing.T, f fixture) { t.Helper(); f.seedOwner(t, ownerPhone, ownerName) },
			open:    func(d *driver) { d.press("3") },
		},
		{
			name:    "from the role picker",
			install: func(t *testing.T, f fixture) { t.Helper(); f.seedOwner(t, ownerPhone, ownerName) },
			open:    func(d *driver) { d.press("4") },
		},
		{
			name:    "from the transfer picker",
			install: withOwnerAndStaff,
			open:    func(d *driver) { d.press("5") },
		},
		{
			name:    "from the add-self wizard",
			install: func(t *testing.T, f fixture) { t.Helper(); f.seedOwner(t, ownerPhone, ownerName) },
			open:    func(d *driver) { d.press("6") },
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t)
			tt.install(t, f)

			d := newDriver(t, f)
			tt.open(d)
			d.press("esc")

			if d.model.screen != screenMenu {
				t.Fatalf("screen after Esc = %v, want the menu", d.model.screen)
			}
		})
	}
}

func TestAppModelQuitBindings(t *testing.T) {
	t.Run("q quits from the menu", func(t *testing.T) {
		f := newFixture(t)
		f.seedOwner(t, ownerPhone, ownerName)

		d := newDriver(t, f)
		d.press("q")

		if !d.quit {
			t.Error("q did not quit the program")
		}
	})

	t.Run("ctrl+c quits from the menu", func(t *testing.T) {
		f := newFixture(t)
		f.seedOwner(t, ownerPhone, ownerName)

		d := newDriver(t, f)
		d.press("ctrl+c")

		if !d.quit {
			t.Error("ctrl+c did not quit the program")
		}
	})

	t.Run("q is a character inside a text field", func(t *testing.T) {
		f := newFixture(t)
		f.seedOwner(t, ownerPhone, ownerName)

		d := newDriver(t, f)
		d.press("6")
		d.typeText("Quique")

		if d.quit {
			t.Fatal("q inside a text field quit the program")
		}
		if got := d.model.form.input.Value(); got != "Quique" {
			t.Errorf("field value = %q, want %q", got, "Quique")
		}
	})

	t.Run("esc on the seed screen exits without writing", func(t *testing.T) {
		f := newFixture(t)

		d := newDriver(t, f)
		d.press("esc")

		if !d.quit {
			t.Error("Esc on the seed screen did not quit")
		}
		if len(f.byRole(t, entity.RoleOwner)) != 0 {
			t.Error("Esc on the seed screen created an owner")
		}
	})

	t.Run("esc on the menu does not quit", func(t *testing.T) {
		f := newFixture(t)
		f.seedOwner(t, ownerPhone, ownerName)

		d := newDriver(t, f)
		d.press("esc")

		if d.quit {
			t.Fatal("Esc on the menu quit the program")
		}
		if view := d.view(); !strings.Contains(view, "Para salir usá q o Ctrl+C") {
			t.Errorf("view = %q, want the quit hint", view)
		}
	})
}

func TestAppModelRefusesToLeaveWhileACoreCallIsInFlight(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)

	// The boot screen is busy by definition: the gate reads the installation and
	// may write the caller-id file.
	model := newModel(f)
	d := &driver{t: t, model: model}

	d.press("esc")
	if d.quit {
		t.Error("Esc quit while the seed gate was running")
	}
	d.press("ctrl+c")
	if d.quit {
		t.Error("ctrl+c quit while the seed gate was running")
	}
	if !strings.Contains(d.view(), "operación en curso") {
		t.Errorf("view = %q, want the busy hint", d.view())
	}
	if !strings.Contains(d.view(), busyLabel) {
		t.Errorf("view = %q, want the spinner while the core call runs", d.view())
	}

	// The same guard applies to an explicit write.
	busy := newModel(f)
	busy.busy = true
	busy.screen = screenMenu
	d = &driver{t: t, model: busy}

	d.press("q")
	if d.quit {
		t.Error("q quit while a write was in flight")
	}
	if !strings.Contains(d.view(), busyLabel) {
		t.Errorf("view = %q, want the spinner while the write is in flight", d.view())
	}
}

// ── Destructive flows ────────────────────────────────────────────────────

// driveToDeactivateConfirm opens the deactivate flow and stops on the
// confirmation of the staff account.
func driveToDeactivateConfirm(t *testing.T, f fixture) *driver {
	t.Helper()

	d := newDriver(t, f)
	d.press("2")
	d.selectRow(staffName)
	d.press("enter")

	if d.model.screen != screenConfirm || d.model.confirm.action != confirmDeactivate {
		t.Fatalf("screen = %v action = %v, want the deactivate confirmation",
			d.model.screen, d.model.confirm.action)
	}
	return d
}

func TestAppModelDeactivateRequiresAnExplicitConfirmation(t *testing.T) {
	t.Run("a stray key is not consent", func(t *testing.T) {
		f := newFixture(t)
		withOwnerAndStaff(t, f)
		d := driveToDeactivateConfirm(t, f)

		d.press("x")

		if d.model.screen != screenConfirm {
			t.Fatalf("screen = %v, want to stay on the confirmation", d.model.screen)
		}
		if view := d.view(); !strings.Contains(view, "confirmar o") {
			t.Errorf("view = %q, want the confirmation hint", view)
		}
		if !f.account(t, staffPhone).Active {
			t.Error("a stray key deactivated the account")
		}
	})

	t.Run("n cancels and writes nothing", func(t *testing.T) {
		f := newFixture(t)
		withOwnerAndStaff(t, f)
		d := driveToDeactivateConfirm(t, f)

		d.press("n")

		if d.model.screen != screenMenu {
			t.Fatalf("screen = %v, want the menu after cancelling", d.model.screen)
		}
		if !strings.Contains(d.view(), "no se desactivó ninguna cuenta") {
			t.Errorf("view = %q, want the cancellation notice", d.view())
		}
		if !f.account(t, staffPhone).Active {
			t.Error("the cancellation deactivated the account")
		}
	})

	t.Run("s deactivates the account", func(t *testing.T) {
		f := newFixture(t)
		withOwnerAndStaff(t, f)
		d := driveToDeactivateConfirm(t, f)

		d.press("s")

		if d.model.screen != screenInfo || d.model.infoKind != infoSuccess {
			t.Fatalf("screen = %v kind = %v, want the success info screen", d.model.screen, d.model.infoKind)
		}
		if !strings.Contains(d.view(), "Cuenta desactivada: "+staffName) {
			t.Errorf("view = %q, want the deactivation outcome", d.view())
		}
		if f.account(t, staffPhone).Active {
			t.Error("the account is still active after confirming")
		}
	})
}

func TestAppModelBusinessFailureIsRecoverableFromTheInfoScreen(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)

	d := newDriver(t, f)
	d.press("2")     // the only active account is the owner
	d.press("enter") // confirmation
	d.press("s")     // the core refuses the last active owner

	if d.model.screen != screenInfo || d.model.infoKind != infoError {
		t.Fatalf("screen = %v kind = %v, want the error info screen", d.model.screen, d.model.infoKind)
	}
	if !strings.Contains(d.view(), "el sistema quedaría sin owner") {
		t.Errorf("view = %q, want the semantic last-owner refusal", d.view())
	}
	if !f.account(t, ownerPhone).Active {
		t.Error("the refused deactivation changed the account")
	}

	d.press("enter")
	if d.model.screen != screenMenu {
		t.Fatalf("screen after acknowledging the failure = %v, want the menu", d.model.screen)
	}
	if d.model.errLine != "" {
		t.Errorf("errLine = %q, want the menu free of stale errors", d.model.errLine)
	}
}

// ── Add Staff ────────────────────────────────────────────────────────────

func TestAppModelAddStaffCreatesTheAccount(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)
	professionalID := f.seedProfessional(t, "Ana", staffPhone)

	d := newDriver(t, f)
	d.press("1")
	d.selectRow("Ana")
	d.press("enter")

	if d.model.screen != screenStaffForm {
		t.Fatalf("screen = %v, want the staff wizard", d.model.screen)
	}
	if got := d.model.form.input.Value(); got != staffPhone {
		t.Errorf("prefilled phone = %q, want the professional phone %q", got, staffPhone)
	}

	d.press("enter") // accept the prefilled phone
	d.typeText(staffName)
	d.press("enter")

	if d.model.infoKind != infoSuccess {
		t.Fatalf("kind = %v, want the success info screen (view: %s)", d.model.infoKind, d.view())
	}

	staff := f.byRole(t, entity.RoleStaff)
	if len(staff) != 1 || staff[0].ID != staffPhone || staff[0].ProfessionalID == nil {
		t.Fatalf("staff accounts = %+v, want one account linked to a professional", staff)
	}
	if *staff[0].ProfessionalID != professionalID {
		t.Errorf("professional_id = %q, want %q", *staff[0].ProfessionalID, professionalID)
	}
}

func TestAppModelAddStaffRejectsAnInvalidPhone(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)
	f.seedProfessional(t, "Ana", staffPhone)

	d := newDriver(t, f)
	d.press("1")
	d.selectRow("Ana")
	d.press("enter")

	// Clear the prefill and type something invalid.
	for range d.model.form.input.Value() {
		d.press("backspace")
	}
	d.typeText("nope")
	d.press("enter")

	if d.model.screen != screenStaffForm || d.model.form.index != 0 {
		t.Fatalf("screen = %v index = %d, want the wizard still on the phone step",
			d.model.screen, d.model.form.index)
	}
	if !strings.Contains(d.view(), "no es válido") {
		t.Errorf("view = %q, want the semantic phone error", d.view())
	}
	if len(f.byRole(t, entity.RoleStaff)) != 0 {
		t.Error("an invalid phone created a staff account")
	}
}

// ── List views ───────────────────────────────────────────────────────────

func TestAppModelListViewHidesInactiveUntilToggled(t *testing.T) {
	f := newFixture(t)
	withOwnerAndStaff(t, f)
	f.deactivate(t, staffPhone)

	d := newDriver(t, f)
	d.press("3")

	if view := d.view(); strings.Contains(view, staffName) {
		t.Errorf("view = %q, want the inactive account hidden", view)
	}
	if view := d.view(); !strings.Contains(view, ownerName) {
		t.Errorf("view = %q, want the active owner shown", view)
	}

	d.press("i")

	view := d.view()
	if !strings.Contains(view, "incluye inactivas") {
		t.Errorf("view = %q, want the inactive opt-in reflected in the title", view)
	}
	if !strings.Contains(view, staffName) {
		t.Errorf("view = %q, want the inactive account listed after opting in", view)
	}
}

// TestAppModelTableViewScrollsWithTheKeyboard proves the table view is really
// scrollable: the arrow keys are forwarded to the viewport that owns the screen
// instead of being swallowed by the state machine.
func TestAppModelTableViewScrollsWithTheKeyboard(t *testing.T) {
	f := newFixture(t)

	model := newModel(f)
	model.screen = screenAccountsTable
	model.tableFilter = admin.ListFilter{}
	model.tableTitle = tableTitle(model.tableFilter)
	model.viewport.Height = 3
	model.accounts = make([]admin.AccountView, 0, 30)
	for i := range 30 {
		model.accounts = append(model.accounts, admin.AccountView{
			ID:          fmt.Sprintf("+54911000%05d", i),
			Role:        entity.RoleStaff,
			DisplayName: fmt.Sprintf("Staff %02d", i),
			Active:      true,
		})
	}
	model.viewport.SetContent(model.tableContent())

	d := &driver{t: t, model: model}
	d.press("down")
	d.press("down")

	if d.model.viewport.YOffset == 0 {
		t.Errorf("viewport offset = %d, want the table scrolled by the arrow keys", d.model.viewport.YOffset)
	}
}

func TestAppModelListByRoleFiltersTheTable(t *testing.T) {
	f := newFixture(t)
	withOwnerAndStaff(t, f)

	d := newDriver(t, f)
	d.press("4")
	d.selectRow("staff")
	d.press("enter")

	if d.model.screen != screenAccountsTable {
		t.Fatalf("screen = %v, want the table view", d.model.screen)
	}
	view := d.view()
	if !strings.Contains(view, "Cuentas con rol staff") {
		t.Errorf("view = %q, want the role-filtered title", view)
	}
	if strings.Contains(view, ownerName) {
		t.Errorf("view = %q, want the owner filtered out", view)
	}
}

// ── Transfer ownership ───────────────────────────────────────────────────

func TestAppModelTransferPromotesTheStaffSuccessor(t *testing.T) {
	f := newFixture(t)
	withOwnerAndStaff(t, f)

	d := newDriver(t, f)
	d.press("5")
	d.selectRow(staffName)
	d.press("enter")

	if d.model.screen != screenConfirm {
		t.Fatalf("screen = %v, want the transfer confirmation", d.model.screen)
	}
	if view := d.view(); !strings.Contains(view, "pierde su vínculo con el profesional") {
		t.Errorf("view = %q, want the staff promotion warning", view)
	}

	d.press("n")
	if d.model.screen != screenMenu {
		t.Fatalf("screen after cancelling = %v, want the menu", d.model.screen)
	}
	if !f.account(t, ownerPhone).Active {
		t.Error("the cancelled transfer deactivated the owner")
	}

	// Retrying the flow performs the swap and republishes the caller id.
	d.press("5")
	d.selectRow(staffName)
	d.press("enter")
	d.press("s")

	if d.model.infoKind != infoSuccess {
		t.Fatalf("kind = %v, want the success info screen (view: %s)", d.model.infoKind, d.view())
	}
	if !strings.Contains(d.view(), "Ownership transferido") {
		t.Errorf("view = %q, want the transfer outcome", d.view())
	}

	if f.account(t, ownerPhone).Active {
		t.Error("the previous owner is still active")
	}
	newOwner := f.account(t, staffPhone)
	if !newOwner.Active || newOwner.Role != entity.RoleOwner {
		t.Errorf("successor = %+v, want an active owner", newOwner)
	}
	if got := f.callerID(t); got != staffPhone {
		t.Errorf("caller-id = %q, want %q after the transfer", got, staffPhone)
	}
}

func TestAppModelTransferToANewPhoneCreatesTheSuccessorAndSwaps(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)

	d := newDriver(t, f)
	d.press("5")
	d.selectRow("Otro teléfono")
	d.press("enter")

	d.typeText(newPhone)
	d.press("enter")
	d.typeText("Nuevo Dueño")
	d.press("enter")

	if d.model.screen != screenConfirm {
		t.Fatalf("screen = %v, want the transfer confirmation", d.model.screen)
	}
	if view := d.view(); !strings.Contains(view, "El nuevo owner será Nuevo Dueño") {
		t.Errorf("view = %q, want the new-owner summary", view)
	}

	d.press("s")

	if d.model.infoKind != infoSuccess {
		t.Fatalf("kind = %v, want the success info screen (view: %s)", d.model.infoKind, d.view())
	}

	successor := f.account(t, newPhone)
	if !successor.Active || successor.Role != entity.RoleOwner {
		t.Errorf("successor = %+v, want an active owner row", successor)
	}
	if f.account(t, ownerPhone).Active {
		t.Error("the previous owner is still active")
	}
	if got := f.callerID(t); got != newPhone {
		t.Errorf("caller-id = %q, want %q", got, newPhone)
	}
}

// ── Add self as client ───────────────────────────────────────────────────

func TestAppModelAddSelfAsClientIsIdempotent(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)

	d := newDriver(t, f)
	d.press("6")
	if view := d.view(); !strings.Contains(view, ownerPhone) {
		t.Errorf("view = %q, want the account phone shown before asking", view)
	}

	d.typeText("Dueño Cliente")
	d.press("enter")

	if d.model.infoKind != infoSuccess {
		t.Fatalf("kind = %v, want the success info screen (view: %s)", d.model.infoKind, d.view())
	}

	client, err := f.clients.FindByID(admin.TUIContext(context.Background()), ownerPhone)
	if err != nil {
		t.Fatalf("FindByID(%s) failed: %v", ownerPhone, err)
	}
	if client.Phone != ownerPhone {
		t.Errorf("client phone = %q, want %q (the caller id the resolver matches)", client.Phone, ownerPhone)
	}

	// Running the flow again is an outcome, not an error, and writes nothing.
	d.press("enter")
	d.press("6")
	d.typeText("Dueño Cliente")
	d.press("enter")

	if !strings.Contains(d.view(), "Ya estabas registrado") {
		t.Errorf("view = %q, want the idempotent outcome", d.view())
	}
}

// ── Configurar Hermes (ADR-0017) ────────────────────────────────────────

func TestAppModelHermesConfigWritesTheMergedConfig(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)

	// A pre-existing Hermes config with a foreign top-level key and a foreign
	// server: the merge must preserve both and only replace our entry.
	if err := os.MkdirAll(filepath.Dir(f.hermesPath), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	seed := []byte("model: hermes-default\nmcp_servers:\n  openai:\n    url: https://api.example/mcp\n")
	if err := os.WriteFile(f.hermesPath, seed, 0o600); err != nil {
		t.Fatalf("seed hermes config: %v", err)
	}

	d := newDriver(t, f)
	d.press("7")

	if d.model.screen != screenHermesForm {
		t.Fatalf("screen = %v, want the Hermes phone wizard", d.model.screen)
	}
	if got := d.model.form.input.Value(); got != ownerPhone {
		t.Errorf("prefilled phone = %q, want the active owner phone %q", got, ownerPhone)
	}

	d.press("enter") // accept the prefilled phone
	if d.model.screen != screenConfirm || d.model.confirm.action != confirmHermesConfig {
		t.Fatalf("screen = %v action = %v, want the Hermes confirmation",
			d.model.screen, d.model.confirm.action)
	}

	confirmView := d.view()
	for _, want := range []string{f.deps.Hermes.EndpointURL, ownerPhone, f.hermesPath} {
		if !strings.Contains(confirmView, want) {
			t.Errorf("confirmation view missing %q\n%s", want, confirmView)
		}
	}

	d.press("s")

	if d.model.screen != screenInfo || d.model.infoKind != infoSuccess {
		t.Fatalf("screen = %v kind = %v, want the success info screen", d.model.screen, d.model.infoKind)
	}
	if view := d.view(); !strings.Contains(view, "Configuración de Hermes actualizada") {
		t.Errorf("view = %q, want the success summary", view)
	}

	content := f.hermesFile(t)
	for _, want := range []string{
		"model: hermes-default",
		"openai",
		admin.HermesServerName,
		f.deps.Hermes.EndpointURL,
		`X-Caller-Id: "` + ownerPhone + `"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("written Hermes config missing %q\n%s", want, content)
		}
	}
}

func TestAppModelHermesConfigRejectsAnInvalidPhone(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)

	d := newDriver(t, f)
	d.press("7")

	// Clear the prefill and type something invalid.
	for range d.model.form.input.Value() {
		d.press("backspace")
	}
	d.typeText("nope")
	d.press("enter")

	if d.model.screen != screenHermesForm || d.model.form.index != 0 {
		t.Fatalf("screen = %v index = %d, want the wizard still on the phone step",
			d.model.screen, d.model.form.index)
	}
	if view := d.view(); !strings.Contains(view, "no es válido") {
		t.Errorf("view = %q, want the semantic phone error", view)
	}
	if _, err := os.Stat(f.hermesPath); err == nil {
		t.Error("an invalid phone wrote the Hermes config")
	}
}

func TestAppModelHermesConfigFallsBackToTheSnippetWhenTheWriteIsImpossible(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)

	// An empty path is the degenerate "there is nowhere to write" case: the load
	// treats the missing file as empty, and the write fails with a semantic error
	// instead of a raw driver failure. The flow must degrade to the manual
	// snippet (ADR-0017 Decision 1).
	f.deps.Hermes.Path = ""

	d := newDriver(t, f)
	d.press("7")
	d.press("enter") // accept the prefilled phone
	d.press("s")

	if d.model.screen != screenHermesSnippet {
		t.Fatalf("screen = %v, want the snippet fallback screen", d.model.screen)
	}
	view := d.view()
	for _, want := range []string{
		"Configuración manual de Hermes",
		"no se indicó dónde escribir",
		admin.HermesServerName,
		`X-Caller-Id: "` + ownerPhone + `"`,
	} {
		if !strings.Contains(view, want) {
			t.Errorf("snippet view missing %q\n%s", want, view)
		}
	}
}

func TestAppModelHermesConfigRequiresAnActiveOwner(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)

	d := newDriver(t, f)
	if d.model.screen != screenMenu {
		t.Fatalf("precondition: screen = %v, want the menu", d.model.screen)
	}

	// The owner disappears after the gate opened: the flow must refuse and
	// re-derive the seed gate instead of continuing with an empty prefill.
	f.deactivate(t, ownerPhone)
	d.press("7")

	if d.model.screen != screenSeedRecovery {
		t.Fatalf("screen = %v, want the deactivated-owner recovery picker", d.model.screen)
	}
	if d.model.errLine == "" {
		t.Error("errLine is empty, want the semantic no-active-owner error")
	}
	if view := d.view(); !strings.Contains(view, "cuentas de owner desactivadas") {
		t.Errorf("view = %q, want the recovery gateway", view)
	}
}

// ── Pure helpers ─────────────────────────────────────────────────────────

func TestPickerWindowKeepsTheCursorVisible(t *testing.T) {
	tests := []struct {
		name      string
		count     int
		cursor    int
		limit     int
		wantStart int
		wantEnd   int
	}{
		{name: "fits in one page", count: 3, cursor: 2, limit: 5, wantStart: 0, wantEnd: 3},
		{name: "cursor at the top", count: 10, cursor: 0, limit: 4, wantStart: 0, wantEnd: 4},
		{name: "cursor in the middle", count: 10, cursor: 5, limit: 4, wantStart: 3, wantEnd: 7},
		{name: "cursor at the bottom pins the last page", count: 10, cursor: 9, limit: 4, wantStart: 6, wantEnd: 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, end := pickerWindow(tt.count, tt.cursor, tt.limit)

			if start != tt.wantStart || end != tt.wantEnd {
				t.Errorf("pickerWindow(%d, %d, %d) = (%d, %d), want (%d, %d)",
					tt.count, tt.cursor, tt.limit, start, end, tt.wantStart, tt.wantEnd)
			}
			if tt.cursor < start || tt.cursor >= end {
				t.Errorf("cursor %d outside the visible window [%d, %d)", tt.cursor, start, end)
			}
		})
	}
}

func TestRenderAccountTableAlignsOnRunesAndNeverTruncates(t *testing.T) {
	views := []admin.AccountView{
		{ID: "+5491100000000", Role: entity.RoleOwner, DisplayName: "Dueño Ñandú", Active: true},
		{ID: staffPhone, Role: entity.RoleStaff, DisplayName: staffName, ProfessionalID: "prof-1", Active: false},
	}

	table := renderAccountTable(views)
	lines := strings.Split(table, "\n")
	if len(lines) != len(views)+1 {
		t.Fatalf("table has %d lines, want %d (header plus one per row)", len(lines), len(views)+1)
	}

	width := utf8.RuneCountInString(lines[0])
	for _, line := range lines {
		if got := utf8.RuneCountInString(line); got != width {
			t.Errorf("line %q has %d runes, want the aligned width %d", line, got, width)
		}
	}

	for _, want := range []string{"TELÉFONO", "ESTADO", "Dueño Ñandú", "inactiva", "prof-1"} {
		if !strings.Contains(table, want) {
			t.Errorf("table missing %q\n%s", want, table)
		}
	}
}

func TestRenderAccountTableEmptyViewRendersTheHeaderOnly(t *testing.T) {
	table := renderAccountTable(nil)
	lines := strings.Split(table, "\n")

	if len(lines) != 1 || !strings.Contains(lines[0], "TELÉFONO") {
		t.Fatalf("table = %q, want the header only", table)
	}
}

func TestFormModelRejectsInvalidAnswersAndAcceptsDefaults(t *testing.T) {
	t.Run("invalid answer keeps the step", func(t *testing.T) {
		form := newForm("prueba", seedSteps())
		form.input.SetValue("nope")

		if got := form.submit(); got != formRejected {
			t.Fatalf("submit() = %v, want formRejected", got)
		}
		if form.index != 0 {
			t.Errorf("index = %d, want to stay on the rejected step", form.index)
		}
		if !strings.Contains(form.err, "no es válido") {
			t.Errorf("err = %q, want the semantic validation error", form.err)
		}
		if form.value(0) != "" {
			t.Errorf("value(0) = %q, want no accepted answer", form.value(0))
		}
	})

	t.Run("empty answer falls back to the step default", func(t *testing.T) {
		form := newForm("prueba", staffSteps(staffPhone))

		if got := form.submit(); got != formAdvanced {
			t.Fatalf("submit() = %v, want formAdvanced", got)
		}
		if got := form.value(0); got != staffPhone {
			t.Errorf("value(0) = %q, want the default %q", got, staffPhone)
		}
	})

	t.Run("the last step reports the wizard as done", func(t *testing.T) {
		form := newForm("prueba", []formStep{{label: displayNameLabel, validate: admin.ValidateDisplayName}})
		form.input.SetValue(ownerName)

		if got := form.submit(); got != formDone {
			t.Fatalf("submit() = %v, want formDone", got)
		}
		if got := form.value(0); got != ownerName {
			t.Errorf("value(0) = %q, want %q", got, ownerName)
		}
	})
}

func TestMenuItemsMirrorTheFrozenScope(t *testing.T) {
	items := menuItems()
	want := []string{
		"Add Staff",
		"Desactivar cuenta",
		"Listar cuentas",
		"Listar por rol",
		"Transferir ownership",
		"Agregarme como cliente",
		"Configurar Hermes",
	}

	if len(items) != len(want) {
		t.Fatalf("menu has %d entries, want %d (ADR-0016 Decision 1)", len(items), len(want))
	}
	for i, label := range want {
		if items[i].label != label {
			t.Errorf("menu[%d] = %q, want %q", i, items[i].label, label)
		}
	}
}

func TestTableTitleNamesTheView(t *testing.T) {
	tests := []struct {
		name   string
		filter admin.ListFilter
		want   string
	}{
		{name: "every active account", filter: admin.ListFilter{}, want: "Todas las cuentas"},
		{
			name:   "every account including inactive",
			filter: admin.ListFilter{IncludeInactive: true},
			want:   "Todas las cuentas (incluye inactivas)",
		},
		{
			name:   "role filtered",
			filter: admin.ListFilter{Role: entity.RoleStaff, IncludeInactive: true},
			want:   "Cuentas con rol staff (incluye inactivas)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tableTitle(tt.filter); got != tt.want {
				t.Errorf("tableTitle(%+v) = %q, want %q", tt.filter, got, tt.want)
			}
		})
	}
}
