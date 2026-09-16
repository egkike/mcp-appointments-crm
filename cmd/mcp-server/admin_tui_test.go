package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
