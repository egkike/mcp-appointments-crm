package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

func TestRunAdminTUIWithoutImplementation(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "appointments.db")
	t.Setenv("MCP_DB_PATH", dbPath)

	err := runAdminTUI()
	if err == nil {
		t.Fatal("runAdminTUI() error = nil, want the not-implemented semantic error")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("runAdminTUI() error = %T (%v), want *domain.SemanticError", err, err)
	}
	if semErr.Code != domain.ErrCodeInternal {
		t.Errorf("runAdminTUI() code = %q, want %q", semErr.Code, domain.ErrCodeInternal)
	}
	if !strings.Contains(semErr.Message, "todavía no está implementada") {
		t.Errorf("runAdminTUI() message = %q, want it to state the TUI is not implemented", semErr.Message)
	}

	// Dependency validation must really run: the stub opens the SQLite file
	// before failing, so a broken install is reported as such.
	if _, statErr := os.Stat(dbPath); statErr != nil {
		t.Errorf("runAdminTUI() did not open the database at MCP_DB_PATH: %v", statErr)
	}
}

func TestRunAdminTUIReportsDatabaseFailure(t *testing.T) {
	// A regular file cannot act as a parent directory, so opening SQLite fails
	// before the stub says anything about the TUI.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocker"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	t.Setenv("MCP_DB_PATH", filepath.Join(blocker, "appointments.db"))

	err := runAdminTUI()
	if err == nil {
		t.Fatal("runAdminTUI() error = nil, want an open-database failure")
	}
	if !strings.Contains(err.Error(), "open database") {
		t.Errorf("runAdminTUI() error = %v, want it to wrap the open-database failure", err)
	}

	var semErr *domain.SemanticError
	if errors.As(err, &semErr) {
		t.Errorf("runAdminTUI() error = %v, want a startup failure instead of the not-implemented error", err)
	}
}

// TestExecuteCLIRoutesAdminTUIRunner locks the dispatch contract end to end
// with the real runner: `admin tui` reaches runAdminTUI and serve mode is never
// started. Only a throwaway SQLite file is required, not the production DB.
func TestExecuteCLIRoutesAdminTUIRunner(t *testing.T) {
	t.Setenv("MCP_DB_PATH", filepath.Join(t.TempDir(), "appointments.db"))

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

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("executeCLI(admin tui) error = %T (%v), want *domain.SemanticError", err, err)
	}
	if !strings.Contains(semErr.Message, "TUI de administración") {
		t.Errorf("executeCLI(admin tui) message = %q, want the admin TUI stub message", semErr.Message)
	}
}

// TestRunHermesChatWithoutImplementation mirrors the admin stub for the other
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
