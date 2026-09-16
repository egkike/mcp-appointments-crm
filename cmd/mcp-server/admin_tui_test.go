package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

// prepareAdminTUI points MCP_DB_PATH and MCP_CONFIG_DIR at throwaway locations
// so the console flow never touches the operator's real installation.
func prepareAdminTUI(t *testing.T) (dbPath, configDir string) {
	t.Helper()
	dbPath = filepath.Join(t.TempDir(), "appointments.db")
	configDir = filepath.Join(t.TempDir(), "config")
	t.Setenv("MCP_DB_PATH", dbPath)
	t.Setenv("MCP_CONFIG_DIR", configDir)
	return dbPath, configDir
}

// listOwners reads the owner accounts back from dbPath through the production
// repository, using the same fabricated TUI caller the flow uses.
func listOwners(t *testing.T, dbPath string) []*entity.Account {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	accounts := repository.NewAccountsRepo(database.Conn, slog.Default())
	owners, err := accounts.GetByRole(admin.TUIContext(context.Background()), entity.RoleOwner)
	if err != nil {
		t.Fatalf("GetByRole(owner) failed: %v", err)
	}
	return owners
}

// seedOwnerForTest pre-creates an active owner so the already-exists path can
// be exercised without stdin input.
func seedOwnerForTest(t *testing.T, dbPath, phone, name string) {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	accounts := repository.NewAccountsRepo(database.Conn, slog.Default())
	if err := admin.Seed(context.Background(), accounts, admin.SeedInput{Phone: phone, DisplayName: name}); err != nil {
		t.Fatalf("seed owner fixture: %v", err)
	}
}

func TestRunAdminTUIFlow_SeedsOwnerAndWritesCallerID(t *testing.T) {
	dbPath, configDir := prepareAdminTUI(t)

	var out bytes.Buffer
	stdin := strings.NewReader("+5491100000000\nDueño\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	owners := listOwners(t, dbPath)
	if len(owners) != 1 {
		t.Fatalf("owner count = %d, want exactly 1", len(owners))
	}
	owner := owners[0]
	if owner.ID != "+5491100000000" {
		t.Errorf("owner.ID = %q, want the phone", owner.ID)
	}
	if owner.Role != entity.RoleOwner {
		t.Errorf("owner.Role = %q, want %q", owner.Role, entity.RoleOwner)
	}
	if owner.DisplayName != "Dueño" {
		t.Errorf("owner.DisplayName = %q, want %q", owner.DisplayName, "Dueño")
	}
	if !owner.Active {
		t.Error("owner.Active = false, want an active owner")
	}

	// #nosec G304 -- configDir is a throwaway directory under t.TempDir();
	// the filename is a package constant.
	data, err := os.ReadFile(filepath.Join(configDir, admin.CallerIDFileName))
	if err != nil {
		t.Fatalf("read caller-id file: %v", err)
	}
	if string(data) != "+5491100000000" {
		t.Errorf("caller-id content = %q, want the owner phone", string(data))
	}

	if !strings.Contains(out.String(), "Owner creado") {
		t.Errorf("output = %q, want a confirmation message", out.String())
	}
	if !strings.Contains(out.String(), admin.CallerIDFileName) {
		t.Errorf("output = %q, want the caller-id path", out.String())
	}
}

func TestRunAdminTUIFlow_ExistingOwnerExitsCleanlyWithoutPrompting(t *testing.T) {
	dbPath, configDir := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, "+5491100000000", "Dueño")

	// The caller-id file already exists with a sentinel: a repeat run must not
	// rewrite it (a repair would overwrite the sentinel with the owner phone).
	callerIDPath := filepath.Join(configDir, admin.CallerIDFileName)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("MkdirAll(config): %v", err)
	}
	// #nosec G306 -- the fixture writes a sentinel; the file mode is not under test.
	if err := os.WriteFile(callerIDPath, []byte("caller-id-existente"), 0o644); err != nil {
		t.Fatalf("seed caller-id fixture: %v", err)
	}

	var out bytes.Buffer
	// The stream is empty on purpose: an already-seeded install must not ask
	// the operator anything (and must not fail on EOF).
	if err := runAdminTUIFlow(strings.NewReader(""), &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v, want a clean exit", err)
	}

	if !strings.Contains(out.String(), "Ya existe un owner activo") {
		t.Errorf("output = %q, want the semantic already-exists message", out.String())
	}
	if strings.Contains(out.String(), "Se reparó") {
		t.Errorf("output = %q, want no repair message when the caller-id file exists", out.String())
	}

	// #nosec G304 -- callerIDPath is inside a throwaway t.TempDir().
	data, err := os.ReadFile(callerIDPath)
	if err != nil {
		t.Fatalf("read caller-id file: %v", err)
	}
	if string(data) != "caller-id-existente" {
		t.Errorf("caller-id content = %q, want the untouched sentinel", string(data))
	}

	if got := len(listOwners(t, dbPath)); got != 1 {
		t.Errorf("owner count = %d, want exactly 1 (no duplicate)", got)
	}
}

func TestRunAdminTUIFlow_RepairsMissingCallerIDForExistingOwner(t *testing.T) {
	dbPath, configDir := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, "+5491100000000", "Dueño")

	var out bytes.Buffer
	// No stdin on purpose: the repair path must not prompt either.
	if err := runAdminTUIFlow(strings.NewReader(""), &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v, want the repair to succeed", err)
	}

	if !strings.Contains(out.String(), "Ya existe un owner activo") {
		t.Errorf("output = %q, want the semantic already-exists message", out.String())
	}
	if !strings.Contains(out.String(), "Se reparó el archivo caller-id") {
		t.Errorf("output = %q, want the honest repair message", out.String())
	}

	// #nosec G304 -- configDir is a throwaway t.TempDir().
	data, err := os.ReadFile(filepath.Join(configDir, admin.CallerIDFileName))
	if err != nil {
		t.Fatalf("read caller-id file: %v", err)
	}
	if string(data) != "+5491100000000" {
		t.Errorf("caller-id content = %q, want the existing owner phone", string(data))
	}

	if got := len(listOwners(t, dbPath)); got != 1 {
		t.Errorf("owner count = %d, want exactly 1 (no duplicate)", got)
	}
}

func TestRunAdminTUIFlow_RepairFailureIsReported(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, "+5491100000000", "Dueño")

	// The config dir cannot be created: a regular file owns the parent path.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocker"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	t.Setenv("MCP_CONFIG_DIR", filepath.Join(blocker, "config"))

	var out bytes.Buffer
	err := runAdminTUIFlow(strings.NewReader(""), &out)
	if err == nil {
		t.Fatal("runAdminTUIFlow() error = nil, want the caller-id repair failure")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("runAdminTUIFlow() error = %T (%v), want *domain.SemanticError", err, err)
	}
	if semErr.Code != domain.ErrCodeInternal {
		t.Errorf("runAdminTUIFlow() code = %q, want %q", semErr.Code, domain.ErrCodeInternal)
	}
}

func TestRunAdminTUIFlow_RepromptsOnInvalidPhone(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)

	var out bytes.Buffer
	// Two invalid answers (non-numeric, then too short) before a valid one.
	stdin := strings.NewReader("no-es-un-telefono\n123\n+5491100000000\nDueño\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if got := strings.Count(out.String(), "Error: el teléfono no es válido"); got != 2 {
		t.Errorf("rejection message count = %d, want 2 re-prompts\noutput: %s", got, out.String())
	}

	owners := listOwners(t, dbPath)
	if len(owners) != 1 {
		t.Fatalf("owner count = %d, want exactly 1", len(owners))
	}
	if owners[0].ID != "+5491100000000" {
		t.Errorf("owner.ID = %q, want the phone from the last valid answer", owners[0].ID)
	}
}

func TestRunAdminTUIFlow_RepromptsOnBlankDisplayName(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)

	var out bytes.Buffer
	stdin := strings.NewReader("+5491100000000\n   \nDueño\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "Error: el nombre para mostrar no puede estar vacío") {
		t.Errorf("output = %q, want the blank-name rejection", out.String())
	}

	owners := listOwners(t, dbPath)
	if len(owners) != 1 {
		t.Fatalf("owner count = %d, want exactly 1", len(owners))
	}
	if owners[0].DisplayName != "Dueño" {
		t.Errorf("owner.DisplayName = %q, want %q", owners[0].DisplayName, "Dueño")
	}
}

func TestRunAdminTUIFlow_AbortedInputCreatesNothing(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)

	err := runAdminTUIFlow(strings.NewReader(""), &bytes.Buffer{})
	if err == nil {
		t.Fatal("runAdminTUIFlow() error = nil, want an aborted-seed error")
	}

	owners := listOwners(t, dbPath)
	if len(owners) != 0 {
		t.Errorf("owner count = %d, want 0 after an aborted seed", len(owners))
	}
}

func TestRunAdminTUIReportsDatabaseFailure(t *testing.T) {
	// A regular file cannot act as a parent directory, so opening SQLite fails
	// before the seed flow says anything.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocker"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	t.Setenv("MCP_DB_PATH", filepath.Join(blocker, "appointments.db"))
	t.Setenv("MCP_CONFIG_DIR", filepath.Join(t.TempDir(), "config"))

	err := runAdminTUI()
	if err == nil {
		t.Fatal("runAdminTUI() error = nil, want an open-database failure")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("runAdminTUI() error = %v, want it to wrap the open-database failure", err)
	}
}

// TestExecuteCLIRoutesAdminTUIRunner locks the dispatch contract end to end
// with the real runner: `admin tui` reaches runAdminTUI and serve mode is never
// started. The throwaway database is pre-seeded so the runner stays
// non-interactive.
func TestExecuteCLIRoutesAdminTUIRunner(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, "+5491100000000", "Dueño")

	var serveCalled, hermesCalled bool
	err := executeCLI([]string{"admin", "tui"}, cliRunners{
		serve:      func() error { serveCalled = true; return nil },
		adminTUI:   runAdminTUI,
		hermesChat: func() error { hermesCalled = true; return nil },
	})

	if serveCalled {
		t.Error("executeCLI(admin tui) started serve mode")
	}
	if hermesCalled {
		t.Error("executeCLI(admin tui) ran the hermes runner")
	}
	if err != nil {
		t.Fatalf("executeCLI(admin tui) error = %v, want a clean exit", err)
	}
}

// TestRunHermesChatWithoutImplementation mirrors the admin flow for the other
// reserved sub-command (ADR-0016 §5).
func TestRunHermesChatWithoutImplementation(t *testing.T) {
	t.Setenv("MCP_DB_PATH", filepath.Join(t.TempDir(), "appointments.db"))

	err := runHermesChat()
	if err == nil {
		t.Fatal("runHermesChat() error = nil, want the not-implemented semantic error")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("runHermesChat() error = %T (%v), want *domain.SemanticError", err, err)
	}
	if semErr.Code != domain.ErrCodeInternal {
		t.Errorf("runHermesChat() code = %q, want %q", semErr.Code, domain.ErrCodeInternal)
	}
	if !strings.Contains(semErr.Message, "no está implementado") {
		t.Errorf("runHermesChat() message = %q, want it to state the chat is not implemented", semErr.Message)
	}
}

// listStaff reads the staff accounts back from dbPath through the production
// repository, using the same fabricated TUI caller the flow uses.
func listStaff(t *testing.T, dbPath string) []*entity.Account {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	accounts := repository.NewAccountsRepo(database.Conn, slog.Default())
	staff, err := accounts.GetByRole(admin.TUIContext(context.Background()), entity.RoleStaff)
	if err != nil {
		t.Fatalf("GetByRole(staff) failed: %v", err)
	}
	return staff
}

// seedProfessionalForTest inserts one active professional and returns its
// generated id, so the Add Staff picker has a row to offer.
func seedProfessionalForTest(t *testing.T, dbPath, name, phone string) string {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	professionals := repository.NewProfessionalsRepo(database.Conn)
	professional := &entity.Professional{Name: name, Status: "active", Phone: &phone}
	if err := professionals.Save(admin.TUIContext(context.Background()), professional); err != nil {
		t.Fatalf("seed professional fixture: %v", err)
	}
	return professional.ID
}

// createStaffForTest pre-creates a staff account so the duplicate-phone path
// can be exercised through the real repository.
func createStaffForTest(t *testing.T, dbPath, phone, name, professionalID string) {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	accounts := repository.NewAccountsRepo(database.Conn, slog.Default())
	profID := professionalID
	err = accounts.Create(admin.TUIContext(context.Background()), &entity.Account{
		ID:             phone,
		Role:           entity.RoleStaff,
		DisplayName:    name,
		ProfessionalID: &profID,
		Active:         true,
	})
	if err != nil {
		t.Fatalf("seed staff fixture: %v", err)
	}
}

func TestRunAdminTUIFlow_AddStaffFromMenu(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, "+5491100000000", "Dueño")
	profID := seedProfessionalForTest(t, dbPath, "Ana", "+5491100000001")

	var out bytes.Buffer
	// Menu -> Add Staff -> pick #1 -> accept the professional-phone prefill
	// (blank line) -> display name -> quit.
	stdin := strings.NewReader("1\n1\n\nAna Staff\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "Add Staff") {
		t.Errorf("output = %q, want the menu entry", out.String())
	}
	if !strings.Contains(out.String(), "Cuenta de staff creada") {
		t.Errorf("output = %q, want the creation confirmation", out.String())
	}

	staff := listStaff(t, dbPath)
	if len(staff) != 1 {
		t.Fatalf("staff count = %d, want exactly 1", len(staff))
	}
	if staff[0].ProfessionalID == nil || *staff[0].ProfessionalID != profID {
		t.Errorf("staff professional_id = %v, want %q", staff[0].ProfessionalID, profID)
	}
	if staff[0].ID != "+5491100000001" {
		t.Errorf("staff.ID = %q, want the prefilled professional phone", staff[0].ID)
	}
	if staff[0].Role != entity.RoleStaff {
		t.Errorf("staff.Role = %q, want %q", staff[0].Role, entity.RoleStaff)
	}
	if !staff[0].Active {
		t.Error("staff.Active = false, want an active staff account")
	}
}

func TestRunAdminTUIFlow_RepromptsOnInvalidProfessionalSelection(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, "+5491100000000", "Dueño")
	seedProfessionalForTest(t, dbPath, "Ana", "+5491100000001")

	var out bytes.Buffer
	// Out of range (9), out of range (0) and non-numeric (abc) before the valid
	// selection.
	stdin := strings.NewReader("1\n9\n0\nabc\n1\n\nAna Staff\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if got := strings.Count(out.String(), "Error: opción inválida"); got != 3 {
		t.Errorf("rejection message count = %d, want 3 re-prompts\noutput: %s", got, out.String())
	}
	if staff := listStaff(t, dbPath); len(staff) != 1 {
		t.Fatalf("staff count = %d, want exactly 1 after the valid selection", len(staff))
	}
}

func TestRunAdminTUIFlow_RepromptsOnInvalidStaffPhone(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, "+5491100000000", "Dueño")
	seedProfessionalForTest(t, dbPath, "Ana", "+5491100000001")

	var out bytes.Buffer
	// The default is overridden by two invalid answers, then a valid one.
	stdin := strings.NewReader("1\n1\nno-es-un-telefono\n123\n+5491100000002\nAna Staff\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if got := strings.Count(out.String(), "Error: el teléfono no es válido"); got != 2 {
		t.Errorf("rejection message count = %d, want 2 re-prompts\noutput: %s", got, out.String())
	}

	staff := listStaff(t, dbPath)
	if len(staff) != 1 {
		t.Fatalf("staff count = %d, want exactly 1", len(staff))
	}
	if staff[0].ID != "+5491100000002" {
		t.Errorf("staff.ID = %q, want the last valid answer", staff[0].ID)
	}
}

// TestRunAdminTUIFlow_AddStaffConflictReturnsToMenu locks the console
// contract of a rejected core write: the error is rendered and the menu
// reopens instead of aborting the session.
//
// The exact semantic wording of the duplicate-phone conflict is asserted in
// internal/admin/staff_test.go with go-sqlmock, where the repo returns
// domain.ErrConflict. On a real SQLite database `accounts.id` is a PRIMARY KEY,
// so the duplicate INSERT raises SQLITE_CONSTRAINT_PRIMARYKEY (1555), which
// repository.isUniqueViolation now classifies alongside SQLITE_CONSTRAINT_UNIQUE
// (2067) — see the repo-level test
// TestAccountsRepo_Create_DuplicateID_PrimaryKey_ErrConflict_RealSQLite. This
// test therefore asserts the end-to-end flow behavior: the semantic conflict
// renders and the menu reopens.
func TestRunAdminTUIFlow_AddStaffConflictReturnsToMenu(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, "+5491100000000", "Dueño")
	profID := seedProfessionalForTest(t, dbPath, "Ana", "+5491100000001")
	createStaffForTest(t, dbPath, "+5491100000001", "Ana Dup", profID)

	var out bytes.Buffer
	// The second attempt reuses the same phone: the core rejects the write, the
	// menu reopens and `q` still exits cleanly.
	stdin := strings.NewReader("1\n1\n+5491100000001\nAna Staff\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v, want the error to return to the menu", err)
	}

	if !strings.Contains(out.String(), "Error: ") {
		t.Errorf("output = %q, want the rejected write rendered as an error", out.String())
	}
	if !strings.Contains(out.String(), "¿Qué querés hacer?") {
		t.Errorf("output = %q, want the menu to reopen after the rejected write", out.String())
	}
	if staff := listStaff(t, dbPath); len(staff) != 1 {
		t.Errorf("staff count = %d, want the pre-existing row only", len(staff))
	}
}

func TestRunAdminTUIFlow_NoActiveProfessionalsReturnsToMenu(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, "+5491100000000", "Dueño")

	var out bytes.Buffer
	stdin := strings.NewReader("1\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "No hay profesionales activos") {
		t.Errorf("output = %q, want the empty-professional semantic message", out.String())
	}
	if staff := listStaff(t, dbPath); len(staff) != 0 {
		t.Errorf("staff count = %d, want 0 with no active professional", len(staff))
	}
}

func TestRunAdminTUIFlow_UnknownMenuOptionReprompts(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, "+5491100000000", "Dueño")

	var out bytes.Buffer
	stdin := strings.NewReader("x\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v, want a clean exit", err)
	}

	if !strings.Contains(out.String(), "Error: opción desconocida") {
		t.Errorf("output = %q, want the unknown-option rejection", out.String())
	}
}

// TestRunAdminTUIFlow_SeedThenAddStaffInSameSession locks the single-scanner
// contract: the seed questionnaire and the menu read the same stream, so the
// menu answers typed after the seed lines are not swallowed by a buffered
// scanner.
func TestRunAdminTUIFlow_SeedThenAddStaffInSameSession(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedProfessionalForTest(t, dbPath, "Ana", "+5491100000001")

	var out bytes.Buffer
	stdin := strings.NewReader("+5491100000000\nDueño\n1\n1\n\nAna Staff\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "Owner creado") {
		t.Errorf("output = %q, want the owner confirmation", out.String())
	}

	if owners := listOwners(t, dbPath); len(owners) != 1 {
		t.Fatalf("owner count = %d, want exactly 1", len(owners))
	}
	staff := listStaff(t, dbPath)
	if len(staff) != 1 {
		t.Fatalf("staff count = %d, want exactly 1 created in the same session", len(staff))
	}
	if staff[0].ProfessionalID == nil {
		t.Error("staff professional_id = nil, want the picked professional")
	}
}

// ── T4: Deactivate + List views ────────────────────────────────────────────

// TestWriteAccountTable_AlignsOnRunesAndNeverTruncates locks the rendering
// contract of the list views: column widths are measured in runes (display
// names carry accents), a row longer than its column is never truncated, and
// missing professional references render as "-" instead of an empty cell.
func TestWriteAccountTable_AlignsOnRunesAndNeverTruncates(t *testing.T) {
	views := []admin.AccountView{
		{ID: "+5491100000000", Role: entity.RoleOwner, DisplayName: "Dueño", Active: true},
		{ID: "+5491100000001", Role: entity.RoleStaff, DisplayName: "Ana Staff", ProfessionalID: "prof-1", Active: false},
	}

	var out bytes.Buffer
	if err := writeAccountTable(&out, views); err != nil {
		t.Fatalf("writeAccountTable() error = %v", err)
	}

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("line count = %d, want the header plus two rows\noutput: %s", len(lines), out.String())
	}

	width := utf8.RuneCountInString(lines[0])
	for i, line := range lines {
		if got := utf8.RuneCountInString(line); got != width {
			t.Errorf("line %d width = %d runes, want %d (columns aligned)\noutput: %s", i, got, width, out.String())
		}
	}

	if !strings.Contains(out.String(), "Dueño") || !strings.Contains(out.String(), "Ana Staff") {
		t.Errorf("output = %q, want every display name rendered in full", out.String())
	}
	if !strings.Contains(out.String(), "PROFESIONAL") || !strings.Contains(lines[1], "-") {
		t.Errorf("output = %q, want the missing professional rendered as -\nline: %q", out.String(), lines[1])
	}
	if !strings.Contains(lines[1], "activa") || !strings.Contains(lines[2], "inactiva") {
		t.Errorf("lines = %q/%q, want the soft-delete state rendered per row", lines[1], lines[2])
	}
}

func TestWriteAccountTable_EmptyViewRendersTheHeaderOnly(t *testing.T) {
	var out bytes.Buffer
	if err := writeAccountTable(&out, nil); err != nil {
		t.Fatalf("writeAccountTable() error = %v", err)
	}

	if got := strings.Count(out.String(), "\n"); got != 1 {
		t.Errorf("line count = %d, want the header only\noutput: %s", got, out.String())
	}
	if !strings.Contains(out.String(), "TELÉFONO") {
		t.Errorf("output = %q, want the header row", out.String())
	}
}

const (
	ownerPhone = "+5491100000000"
	staffPhone = "+5491100000001"
)

// activeAccountIndex returns the 1-based position of id in the deactivate
// picker — every ACTIVE account, in repository order (created_at ASC) — so the
// flow tests type the right number without depending on insertion timing.
func activeAccountIndex(t *testing.T, dbPath, id string) int {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	accounts := repository.NewAccountsRepo(database.Conn, slog.Default())
	rows, err := accounts.List(admin.TUIContext(context.Background()))
	if err != nil {
		t.Fatalf("List() failed: %v", err)
	}

	index := 0
	for _, account := range rows {
		if !account.Active {
			continue
		}
		index++
		if account.ID == id {
			return index
		}
	}
	t.Fatalf("account %q is not among the active accounts", id)
	return 0
}

// deactivateAccountForTest soft-deletes an account through the production
// repository, so the list views have an inactive row to hide or show.
func deactivateAccountForTest(t *testing.T, dbPath, id string) {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	accounts := repository.NewAccountsRepo(database.Conn, slog.Default())
	if err := accounts.Deactivate(admin.TUIContext(context.Background()), id); err != nil {
		t.Fatalf("deactivate fixture: %v", err)
	}
}

// seedOwnerAndStaffForTest prepares the two-account installation the T4 flows
// operate on and returns the staff phone index in the deactivate picker.
func seedOwnerAndStaffForTest(t *testing.T, dbPath string) int {
	t.Helper()
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")
	profID := seedProfessionalForTest(t, dbPath, "Ana", staffPhone)
	createStaffForTest(t, dbPath, staffPhone, "Ana Staff", profID)
	return activeAccountIndex(t, dbPath, staffPhone)
}

func TestRunAdminTUIFlow_DeactivateFromMenu(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	staffIndex := seedOwnerAndStaffForTest(t, dbPath)

	var out bytes.Buffer
	// Menu -> Deactivate -> pick the staff account -> confirm -> quit.
	stdin := strings.NewReader(fmt.Sprintf("2\n%d\ns\nq\n", staffIndex))

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "Cuentas activas:") {
		t.Errorf("output = %q, want the active-account picker", out.String())
	}
	if !strings.Contains(out.String(), "Esta acción desactiva la cuenta Ana Staff") {
		t.Errorf("output = %q, want the explicit confirmation prompt naming the account", out.String())
	}
	if !strings.Contains(out.String(), "Cuenta desactivada: Ana Staff (staff, "+staffPhone+")") {
		t.Errorf("output = %q, want the deactivation confirmation", out.String())
	}

	staff := listStaff(t, dbPath)
	if len(staff) != 1 {
		t.Fatalf("staff count = %d, want exactly 1", len(staff))
	}
	if staff[0].Active {
		t.Error("staff account is still active after a confirmed deactivation")
	}
	owners := listOwners(t, dbPath)
	if len(owners) != 1 || !owners[0].Active {
		t.Errorf("owners = %+v, want the owner untouched and active", owners)
	}
}

func TestRunAdminTUIFlow_DeactivateCancelledWritesNothing(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	staffIndex := seedOwnerAndStaffForTest(t, dbPath)

	var out bytes.Buffer
	stdin := strings.NewReader(fmt.Sprintf("2\n%d\nn\nq\n", staffIndex))

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "Operación cancelada: no se desactivó ninguna cuenta.") {
		t.Errorf("output = %q, want the cancellation message", out.String())
	}

	staff := listStaff(t, dbPath)
	if len(staff) != 1 {
		t.Fatalf("staff count = %d, want exactly 1", len(staff))
	}
	if !staff[0].Active {
		t.Error("staff account was deactivated after the operator answered no")
	}
}

func TestRunAdminTUIFlow_DeactivateRepromptsOnInvalidConfirmation(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	staffIndex := seedOwnerAndStaffForTest(t, dbPath)

	var out bytes.Buffer
	// An unrecognised answer must never be read as consent: the flow re-prompts
	// and only the explicit "s" deactivates.
	stdin := strings.NewReader(fmt.Sprintf("2\n%d\nquizás\ns\nq\n", staffIndex))

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if got := strings.Count(out.String(), "Error: respondé"); got != 1 {
		t.Errorf("re-prompt count = %d, want 1\noutput: %s", got, out.String())
	}

	staff := listStaff(t, dbPath)
	if len(staff) != 1 {
		t.Fatalf("staff count = %d, want exactly 1", len(staff))
	}
	if staff[0].Active {
		t.Error("staff account is still active, want the re-prompt followed by the confirmed write")
	}
}

func TestRunAdminTUIFlow_RefusesToDeactivateTheLastActiveOwner(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")
	ownerIndex := activeAccountIndex(t, dbPath, ownerPhone)

	var out bytes.Buffer
	// Menu -> Deactivate -> the only active owner -> confirm: the core refuses
	// and the menu reopens instead of aborting the session.
	stdin := strings.NewReader(fmt.Sprintf("2\n%d\ns\nq\n", ownerIndex))

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v, want the refusal to return to the menu", err)
	}

	if !strings.Contains(out.String(), "Error: no se puede desactivar al único owner activo") {
		t.Errorf("output = %q, want the semantic single-owner refusal", out.String())
	}
	if got := strings.Count(out.String(), "¿Qué querés hacer?"); got < 2 {
		t.Errorf("menu render count = %d, want the menu reopened after the refusal\noutput: %s", got, out.String())
	}

	owners := listOwners(t, dbPath)
	if len(owners) != 1 {
		t.Fatalf("owner count = %d, want exactly 1", len(owners))
	}
	if !owners[0].Active {
		t.Error("the last active owner was deactivated, want the invariant preserved")
	}
}

func TestRunAdminTUIFlow_ListsAllAccounts(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerAndStaffForTest(t, dbPath)

	var out bytes.Buffer
	// Menu -> List accounts -> do not include inactive -> quit.
	stdin := strings.NewReader("3\nn\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	for _, want := range []string{"TELÉFONO", "ROL", "NOMBRE", "PROFESIONAL", "ESTADO", ownerPhone, staffPhone, "owner", "staff"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output = %q, want it to render %q", out.String(), want)
		}
	}
	if strings.Contains(out.String(), "inactiva\n") {
		t.Errorf("output = %q, want no inactive state without opting in", out.String())
	}
}

func TestRunAdminTUIFlow_ListHidesInactiveUntilOptedIn(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerAndStaffForTest(t, dbPath)
	deactivateAccountForTest(t, dbPath, staffPhone)

	var hidden bytes.Buffer
	if err := runAdminTUIFlow(strings.NewReader("3\nn\nq\n"), &hidden); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}
	if strings.Contains(hidden.String(), staffPhone) {
		t.Errorf("output = %q, want the inactive account hidden by default", hidden.String())
	}
	// The state cell is the last column of a row, so a trailing newline
	// distinguishes it from the opt-in question, which also contains the word.
	if strings.Contains(hidden.String(), "inactiva\n") {
		t.Errorf("output = %q, want no inactive row without opting in", hidden.String())
	}

	var shown bytes.Buffer
	if err := runAdminTUIFlow(strings.NewReader("3\ns\nq\n"), &shown); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}
	if !strings.Contains(shown.String(), staffPhone) {
		t.Errorf("output = %q, want the inactive account listed after opting in", shown.String())
	}
	if !strings.Contains(shown.String(), "inactiva\n") {
		t.Errorf("output = %q, want the inactive state rendered in a row", shown.String())
	}
}

func TestRunAdminTUIFlow_ListsByRole(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerAndStaffForTest(t, dbPath)

	var out bytes.Buffer
	// Menu -> List by role -> invalid role index -> staff -> no inactive -> quit.
	stdin := strings.NewReader("4\n9\n3\nn\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "Error: opción inválida") {
		t.Errorf("output = %q, want the invalid role index rejected", out.String())
	}
	if !strings.Contains(out.String(), staffPhone) {
		t.Errorf("output = %q, want the staff account listed", out.String())
	}
	if strings.Contains(out.String(), ownerPhone) {
		t.Errorf("output = %q, want the owner excluded by the staff role filter", out.String())
	}
}

func TestRunAdminTUIFlow_EmptyRoleListShowsSemanticMessage(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")

	var out bytes.Buffer
	// There is no admin account: the empty view is an answer, not an error.
	stdin := strings.NewReader("4\n2\nn\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "No hay cuentas para mostrar.") {
		t.Errorf("output = %q, want the empty-view semantic message", out.String())
	}
	if strings.Contains(out.String(), "Error: ") {
		t.Errorf("output = %q, want no error for an empty list", out.String())
	}
}

// ── T5: Transfer Ownership ─────────────────────────────────────────────────

const (
	successorPhone = "+5491100000022"
)

// activeOwnerAccounts returns the ACTIVE owner rows of dbPath, so a test can
// assert the single-owner invariant without counting inactive history rows.
func activeOwnerAccounts(t *testing.T, dbPath string) []*entity.Account {
	t.Helper()
	active := []*entity.Account{}
	for _, owner := range listOwners(t, dbPath) {
		if owner.Active {
			active = append(active, owner)
		}
	}
	return active
}

// readCallerID reads the caller-id file written by the console flows.
func readCallerID(t *testing.T, configDir string) string {
	t.Helper()
	// #nosec G304 -- configDir is a throwaway directory under t.TempDir();
	// the filename is a package constant.
	data, err := os.ReadFile(filepath.Join(configDir, admin.CallerIDFileName))
	if err != nil {
		t.Fatalf("read caller-id file: %v", err)
	}
	return string(data)
}

// TestRunAdminTUIFlow_TransferOwnershipPromotesStaffFromMenu covers the whole
// T5 flow end to end on a real database: menu -> promote the staff account ->
// confirm -> swap -> caller-id rewritten to the new owner.
func TestRunAdminTUIFlow_TransferOwnershipPromotesStaffFromMenu(t *testing.T) {
	dbPath, configDir := prepareAdminTUI(t)
	seedOwnerAndStaffForTest(t, dbPath)

	var out bytes.Buffer
	// Menu -> Transfer -> option 1 (the only staff account) -> confirm -> quit.
	stdin := strings.NewReader("5\n1\ns\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	for _, want := range []string{
		"Paso 1 de 2: elegir el sucesor.",
		"Owner actual:",
		"Ana Staff (staff, " + staffPhone + ")",
		"Paso 2 de 2: confirmar la transferencia.",
		"El owner actual quedará desactivado. ¿Confirmar?",
		"Ownership transferido:",
		"caller-id actualizado en",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output = %q, want it to contain %q", out.String(), want)
		}
	}

	active := activeOwnerAccounts(t, dbPath)
	if len(active) != 1 {
		t.Fatalf("active owner count = %d, want exactly 1\nowners: %+v", len(active), listOwners(t, dbPath))
	}
	if active[0].ID != staffPhone {
		t.Errorf("active owner = %q, want the promoted staff account %q", active[0].ID, staffPhone)
	}
	if active[0].Role != entity.RoleOwner {
		t.Errorf("active owner role = %q, want %q", active[0].Role, entity.RoleOwner)
	}

	owners := listOwners(t, dbPath)
	if len(owners) != 2 {
		t.Fatalf("owner row count = %d, want the previous owner soft-deleted plus the new one", len(owners))
	}
	for _, owner := range owners {
		if owner.ID == ownerPhone && owner.Active {
			t.Error("the previous owner is still active after the transfer")
		}
	}
	if staff := listStaff(t, dbPath); len(staff) != 0 {
		t.Errorf("staff rows = %d, want 0: the account was promoted to owner", len(staff))
	}
	if got := readCallerID(t, configDir); got != staffPhone {
		t.Errorf("caller-id = %q, want the new owner %q", got, staffPhone)
	}
}

// TestRunAdminTUIFlow_TransferCancelledWritesNothing locks the consent gate:
// anything but "s" leaves the installation untouched, caller-id file included.
func TestRunAdminTUIFlow_TransferCancelledWritesNothing(t *testing.T) {
	dbPath, configDir := prepareAdminTUI(t)
	seedOwnerAndStaffForTest(t, dbPath)

	var out bytes.Buffer
	stdin := strings.NewReader("5\n1\nn\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "Operación cancelada: no se transfirió la propiedad.") {
		t.Errorf("output = %q, want the cancellation message", out.String())
	}

	active := activeOwnerAccounts(t, dbPath)
	if len(active) != 1 || active[0].ID != ownerPhone {
		t.Fatalf("active owners = %+v, want the untouched original owner", active)
	}
	if staff := listStaff(t, dbPath); len(staff) != 1 || !staff[0].Active {
		t.Errorf("staff = %+v, want the untouched active staff account", staff)
	}
	// A cancelled transfer must not repoint the caller-id file at another owner.
	// The file may already exist: the flow's EnsureCallerID repair path writes it
	// for the current owner before opening the menu.
	if got := readCallerID(t, configDir); got != ownerPhone {
		t.Errorf("caller-id = %q, want it still pointing at the current owner %q", got, ownerPhone)
	}
}

// TestRunAdminTUIFlow_TransferToNewPhoneCreatesInactiveOwnerThenSwaps covers
// the "brand-new phone" successor: step 1 creates a role=owner, is_active=0 row
// (allowed while the current owner is active because the triggers count only
// ACTIVE owners), step 2 swaps it in.
func TestRunAdminTUIFlow_TransferToNewPhoneCreatesInactiveOwnerThenSwaps(t *testing.T) {
	dbPath, configDir := prepareAdminTUI(t)
	seedOwnerAndStaffForTest(t, dbPath)

	var out bytes.Buffer
	// Menu -> Transfer -> option 2 (new phone) -> phone -> name -> confirm -> quit.
	stdin := strings.NewReader("5\n2\n" + successorPhone + "\nDueño Nuevo\ns\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "Otro teléfono (crear una cuenta de owner nueva)") {
		t.Errorf("output = %q, want the new-phone successor option", out.String())
	}

	active := activeOwnerAccounts(t, dbPath)
	if len(active) != 1 {
		t.Fatalf("active owner count = %d, want exactly 1\nowners: %+v", len(active), listOwners(t, dbPath))
	}
	if active[0].ID != successorPhone {
		t.Errorf("active owner = %q, want the new phone %q", active[0].ID, successorPhone)
	}
	if active[0].DisplayName != "Dueño Nuevo" {
		t.Errorf("active owner name = %q, want the operator-provided name", active[0].DisplayName)
	}
	if staff := listStaff(t, dbPath); len(staff) != 1 || !staff[0].Active {
		t.Errorf("staff = %+v, want the staff account untouched", staff)
	}
	if got := readCallerID(t, configDir); got != successorPhone {
		t.Errorf("caller-id = %q, want the new owner %q", got, successorPhone)
	}
}

// TestRunAdminTUIFlow_DeadendOffersReactivation closes
// R4-deactivated-owner-seed-deadend: with zero ACTIVE owners and a deactivated
// owner row, the console must reactivate that row instead of walking into the
// accounts.id PRIMARY KEY.
func TestRunAdminTUIFlow_DeadendOffersReactivation(t *testing.T) {
	dbPath, configDir := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")
	deactivateAccountForTest(t, dbPath, ownerPhone)

	var out bytes.Buffer
	// Recovery menu -> reactivate option 1 -> confirm -> quit the operator menu.
	stdin := strings.NewReader("1\ns\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	for _, want := range []string{
		"No hay ningún owner activo, pero hay 1 cuenta(s) de owner desactivada(s).",
		"Reactivar Dueño (owner, " + ownerPhone + ")",
		"Se reactivará",
		"Owner reactivado: Dueño (" + ownerPhone + ")",
		"caller-id escrito en",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output = %q, want it to contain %q", out.String(), want)
		}
	}

	owners := listOwners(t, dbPath)
	if len(owners) != 1 {
		t.Fatalf("owner row count = %d, want the reactivated row only", len(owners))
	}
	if !owners[0].Active {
		t.Error("the owner row is still inactive after the reactivation")
	}
	if got := readCallerID(t, configDir); got != ownerPhone {
		t.Errorf("caller-id = %q, want the reactivated owner %q", got, ownerPhone)
	}
}

// TestRunAdminTUIFlow_DeadendReactivationCancelledKeepsTheSystemOwnerless locks
// the consent gate of the recovery: a "no" writes nothing and re-renders the
// choice, so the operator can still create a fresh owner in the same session.
func TestRunAdminTUIFlow_DeadendReactivationCancelledKeepsTheSystemOwnerless(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")
	deactivateAccountForTest(t, dbPath, ownerPhone)

	var out bytes.Buffer
	// Recovery -> reactivate -> decline -> create a new owner with a fresh phone -> quit.
	stdin := strings.NewReader("1\nn\n2\n" + successorPhone + "\nDueño Nuevo\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "Operación cancelada: no se reactivó ninguna cuenta.") {
		t.Errorf("output = %q, want the cancellation message", out.String())
	}
	if !strings.Contains(out.String(), "Owner creado: Dueño Nuevo ("+successorPhone+")") {
		t.Errorf("output = %q, want the fresh owner creation", out.String())
	}

	active := activeOwnerAccounts(t, dbPath)
	if len(active) != 1 || active[0].ID != successorPhone {
		t.Fatalf("active owners = %+v, want exactly the new owner %q", active, successorPhone)
	}
	if owners := listOwners(t, dbPath); len(owners) != 2 {
		t.Errorf("owner row count = %d, want the deactivated row plus the new one", len(owners))
	}
}

// TestRunAdminTUIFlow_DeadendTakenPhoneGivesReactivationGuidance locks the
// conflict path of the deadend: insisting on the deactivated owner's phone
// fails semantically (accounts.id PRIMARY KEY) and the message points at the
// reactivation option instead of dumping driver text.
func TestRunAdminTUIFlow_DeadendTakenPhoneGivesReactivationGuidance(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")
	deactivateAccountForTest(t, dbPath, ownerPhone)

	var out bytes.Buffer
	// Recovery -> create new -> reuse the deactivated phone -> guidance and
	// re-render -> reactivate option 1 -> confirm -> quit.
	stdin := strings.NewReader("2\n" + ownerPhone + "\nOtro Nombre\n1\ns\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "No se creó ninguna cuenta") {
		t.Errorf("output = %q, want the semantic conflict of the reused phone", out.String())
	}
	if !strings.Contains(out.String(), "Sugerencia: el teléfono "+ownerPhone+" pertenece a la cuenta de owner desactivada") {
		t.Errorf("output = %q, want the reactivation guidance", out.String())
	}
	if strings.Contains(out.String(), "PRIMARY KEY") || strings.Contains(out.String(), "constraint") {
		t.Errorf("output = %q, want no driver detail", out.String())
	}
	if !strings.Contains(out.String(), "Owner reactivado") {
		t.Errorf("output = %q, want the reactivation that followed the guidance", out.String())
	}

	owners := listOwners(t, dbPath)
	if len(owners) != 1 {
		t.Fatalf("owner row count = %d, want a single reactivated row", len(owners))
	}
	if !owners[0].Active {
		t.Error("the owner row is inactive, want the reactivation applied")
	}
	if owners[0].DisplayName != "Dueño" {
		t.Errorf("owner name = %q, want the stored name (reactivation is not an edit)", owners[0].DisplayName)
	}
}

// ── T6: Agregarme como cliente ─────────────────────────────────────────────

// readClient reads one clients row through the production repository, using the
// same fabricated TUI caller the console flows use. ok is false when the row
// does not exist.
func readClient(t *testing.T, dbPath, id string) (*entity.Client, bool) {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	clients := repository.NewClientsRepo(database.Conn)
	client, err := clients.FindByID(admin.TUIContext(context.Background()), id)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			return nil, false
		}
		t.Fatalf("FindByID(%q) failed: %v", id, err)
	}
	return client, true
}

// countClients counts the clients rows of the installation, so a test can lock
// the idempotent path without relying on the flow output alone.
func countClients(t *testing.T, dbPath string) int {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	var count int
	if err := database.Conn.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM clients`).Scan(&count); err != nil {
		t.Fatalf("count clients rows: %v", err)
	}
	return count
}

// createClientForTest inserts a clients row through the production repository
// under the fabricated TUI caller, so the conflict paths run against the real
// UNIQUE(phone) constraint.
func createClientForTest(t *testing.T, dbPath, id, name, phone string) {
	t.Helper()
	database, err := db.NewDatabase(context.Background(), dbPath)
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	clients := repository.NewClientsRepo(database.Conn)
	if err := clients.Create(admin.TUIContext(context.Background()), &entity.Client{
		ID:    id,
		Name:  name,
		Phone: phone,
	}); err != nil {
		t.Fatalf("create client fixture: %v", err)
	}
}

// TestRunAdminTUIFlow_AddSelfAsClientFromMenu covers the whole operator path:
// menu -> shown account id -> display name -> registered row whose id is the
// operator phone (what makes the resolver discover the client role).
func TestRunAdminTUIFlow_AddSelfAsClientFromMenu(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")

	var out bytes.Buffer
	stdin := strings.NewReader("6\nDueño Cliente\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	for _, want := range []string{
		"Agregarme como cliente",
		"Tu teléfono de cuenta es: " + ownerPhone,
		"Ya podés operar como cliente con este teléfono.",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output = %q, want it to contain %q", out.String(), want)
		}
	}

	client, ok := readClient(t, dbPath, ownerPhone)
	if !ok {
		t.Fatalf("no clients row for %q after the flow", ownerPhone)
	}
	if client.ID != ownerPhone {
		t.Errorf("client.ID = %q, want the operator phone %q", client.ID, ownerPhone)
	}
	if client.Phone != ownerPhone {
		t.Errorf("client.Phone = %q, want %q", client.Phone, ownerPhone)
	}
	if client.Name != "Dueño Cliente" {
		t.Errorf("client.Name = %q, want %q", client.Name, "Dueño Cliente")
	}
}

// TestRunAdminTUIFlow_AddSelfAsClientIsIdempotent locks the outcome-not-error
// contract: a second attempt in the same session reports the existing row and
// returns to the menu without writing again.
func TestRunAdminTUIFlow_AddSelfAsClientIsIdempotent(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")

	var out bytes.Buffer
	stdin := strings.NewReader("6\nDueño Cliente\n6\nDueño Cliente\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if got := strings.Count(out.String(), "Ya podés operar como cliente con este teléfono."); got != 1 {
		t.Errorf("fresh-registration message count = %d, want 1\noutput: %s", got, out.String())
	}
	if !strings.Contains(out.String(), "Ya estabas registrado como cliente con este teléfono.") {
		t.Errorf("output = %q, want the idempotent outcome message", out.String())
	}
	if strings.Contains(out.String(), "Error: ") {
		t.Errorf("output = %q, want no error for an already-registered operator", out.String())
	}
	if got := countClients(t, dbPath); got != 1 {
		t.Errorf("clients row count = %d, want exactly 1", got)
	}
}

// TestRunAdminTUIFlow_AddSelfAsClientConflictReturnsToMenu locks the console
// contract of a rejected core write: the phone already belongs to another
// client row, the semantic message is rendered (never driver detail) and the
// menu reopens instead of aborting the session.
func TestRunAdminTUIFlow_AddSelfAsClientConflictReturnsToMenu(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")
	// Same phone, different id: the UUID bootstrap path the add-self flow must
	// not reuse.
	createClientForTest(t, dbPath, "uuid-cliente-existente", "Otra Ficha", ownerPhone)

	var out bytes.Buffer
	stdin := strings.NewReader("6\nDueño Cliente\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v, want the error to return to the menu", err)
	}

	if !strings.Contains(out.String(), "Error: ") {
		t.Errorf("output = %q, want the rejected write rendered as an error", out.String())
	}
	if !strings.Contains(out.String(), "teléfono ya está registrado como cliente") {
		t.Errorf("output = %q, want the semantic duplicate-phone message", out.String())
	}
	if strings.Contains(out.String(), "constraint") || strings.Contains(out.String(), "UNIQUE") {
		t.Errorf("output = %q, want no driver detail", out.String())
	}
	if !strings.Contains(out.String(), "¿Qué querés hacer?") {
		t.Errorf("output = %q, want the menu to reopen after the rejected write", out.String())
	}
	if got := countClients(t, dbPath); got != 1 {
		t.Errorf("clients row count = %d, want the pre-existing row only", got)
	}
	if _, ok := readClient(t, dbPath, ownerPhone); ok {
		t.Error("a clients row with the operator id was created despite the phone conflict")
	}
}

// TestRunAdminTUIFlow_AddSelfAsClientRepromptsOnBlankName locks the input
// validation of the flow: a blank display name never reaches the core.
func TestRunAdminTUIFlow_AddSelfAsClientRepromptsOnBlankName(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")

	var out bytes.Buffer
	stdin := strings.NewReader("6\n   \nDueño Cliente\nq\n")

	if err := runAdminTUIFlow(stdin, &out); err != nil {
		t.Fatalf("runAdminTUIFlow() error = %v", err)
	}

	if !strings.Contains(out.String(), "Error: el nombre para mostrar no puede estar vacío") {
		t.Errorf("output = %q, want the blank-name rejection", out.String())
	}
	if got := countClients(t, dbPath); got != 1 {
		t.Errorf("clients row count = %d, want exactly 1 after the valid answer", got)
	}
}
