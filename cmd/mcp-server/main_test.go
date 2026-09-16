package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/buildinfo"
	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// buildTestBinary compiles the package into a temporary directory and returns
// the binary path, so CLI behavior is asserted against real process exit codes
// instead of in-process calls.
func buildTestBinary(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "mcp-server")
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", bin, ".") //nolint:gosec // test builds local package, bin path is TempDir-controlled
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}
	return bin
}

func TestWantsVersion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "with --version flag",
			args: []string{"mcp-server", "--version"},
			want: true,
		},
		{
			name: "no args",
			args: []string{"mcp-server"},
			want: false,
		},
		{
			name: "other args",
			args: []string{"mcp-server", "--help"},
			want: false,
		},
		{
			name: "empty args",
			args: []string{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wantsVersion(tt.args); got != tt.want {
				t.Errorf("wantsVersion(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf)

	got := strings.TrimSpace(buf.String())
	want := buildinfo.Version
	if got != want {
		t.Errorf("printVersion() = %q, want %q", got, want)
	}
}

func TestBinaryVersionFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping exec-based test on windows")
	}

	bin := buildTestBinary(t)

	cmd := exec.CommandContext(context.Background(), bin, "--version") //nolint:gosec // test executes TempDir-built binary with fixed args
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary --version failed: %v\n%s", err, out)
	}

	got := strings.TrimSpace(string(out))
	want := buildinfo.Version
	if got != want {
		t.Errorf("binary --version = %q, want %q", got, want)
	}
}

func TestParseCommand(t *testing.T) {
	tests := []struct {
		name  string
		args  []string
		want  commandKind
		wants []string // substrings required in the semantic error message
	}{
		{name: "no args selects serve mode", want: commandServe},
		{name: "admin tui", args: []string{"admin", "tui"}, want: commandAdminTUI},
		{name: "hermes chat", args: []string{"hermes", "chat"}, want: commandHermesChat},
		{
			name:  "admin without sub-command",
			args:  []string{"admin"},
			wants: []string{"sub-comando faltante para", "tui", usageHint},
		},
		{
			name:  "admin with unknown sub-command",
			args:  []string{"admin", "chat"},
			wants: []string{"sub-comando desconocido", "tui", usageHint},
		},
		{
			name:  "admin with extra arguments",
			args:  []string{"admin", "tui", "--now"},
			wants: []string{"argumentos no esperados", usageHint},
		},
		{
			name:  "hermes without sub-command",
			args:  []string{"hermes"},
			wants: []string{"sub-comando faltante para", "chat", usageHint},
		},
		{
			name:  "hermes with unknown sub-command",
			args:  []string{"hermes", "tui"},
			wants: []string{"sub-comando desconocido", "chat", usageHint},
		},
		{
			name:  "unknown flag",
			args:  []string{"--help"},
			wants: []string{"argumento desconocido", "--help", usageHint},
		},
		{
			name:  "unknown positional argument",
			args:  []string{"serve"},
			wants: []string{"argumento desconocido", "serve", usageHint},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCommand(tt.args)

			if len(tt.wants) == 0 {
				if err != nil {
					t.Fatalf("parseCommand(%v) unexpected error: %v", tt.args, err)
				}
				if got != tt.want {
					t.Errorf("parseCommand(%v) = %d, want %d", tt.args, got, tt.want)
				}
				return
			}

			if err == nil {
				t.Fatalf("parseCommand(%v) = %d, want a semantic error", tt.args, got)
			}
			// A rejected invocation must never select a command: serve mode would
			// start the HTTP server and a sub-command would run its runner.
			if got != commandServe {
				t.Errorf("parseCommand(%v) kind = %d, want commandServe when invalid", tt.args, got)
			}

			var semErr *domain.SemanticError
			if !errors.As(err, &semErr) {
				t.Fatalf("parseCommand(%v) error = %T, want *domain.SemanticError", tt.args, err)
			}
			if semErr.Code != domain.ErrCodeInvalidInput {
				t.Errorf("parseCommand(%v) code = %q, want %q", tt.args, semErr.Code, domain.ErrCodeInvalidInput)
			}
			for _, want := range tt.wants {
				if !strings.Contains(semErr.Message, want) {
					t.Errorf("parseCommand(%v) message %q missing %q", tt.args, semErr.Message, want)
				}
			}
		})
	}
}

func TestExecuteCLIDispatch(t *testing.T) {
	sentinel := errors.New("hermes runner error")

	var called []string
	recorder := func(name string, err error) commandRunner {
		return func() error {
			called = append(called, name)
			return err
		}
	}
	runners := cliRunners{
		serve:      recorder("serve", nil),
		adminTUI:   recorder("admin tui", nil),
		hermesChat: recorder("hermes chat", sentinel),
	}

	tests := []struct {
		name     string
		args     []string
		wantCall string
		wantErr  error
	}{
		{name: "no args runs serve mode", wantCall: "serve"},
		{name: "admin tui runs the admin runner", args: []string{"admin", "tui"}, wantCall: "admin tui"},
		{name: "hermes chat runs the hermes runner", args: []string{"hermes", "chat"}, wantCall: "hermes chat", wantErr: sentinel},
		{name: "missing sub-command runs nothing", args: []string{"admin"}},
		{name: "unknown argument runs nothing", args: []string{"--help"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			called = nil

			err := executeCLI(tt.args, runners)

			switch {
			case tt.wantErr != nil:
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("executeCLI(%v) error = %v, want %v", tt.args, err, tt.wantErr)
				}
			case tt.wantCall == "":
				var semErr *domain.SemanticError
				if !errors.As(err, &semErr) {
					t.Errorf("executeCLI(%v) error = %v, want *domain.SemanticError", tt.args, err)
				}
			case err != nil:
				t.Errorf("executeCLI(%v) unexpected error: %v", tt.args, err)
			}

			if tt.wantCall == "" {
				if len(called) != 0 {
					t.Fatalf("executeCLI(%v) ran %v, want no runner", tt.args, called)
				}
				return
			}
			if len(called) != 1 || called[0] != tt.wantCall {
				t.Fatalf("executeCLI(%v) ran %v, want exactly [%s]", tt.args, called, tt.wantCall)
			}
		})
	}
}

func TestBinarySubCommandDispatch(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping exec-based test on windows")
	}

	bin := buildTestBinary(t)

	// The sub-command paths open SQLite while validating dependencies, so point
	// MCP_DB_PATH at a throwaway database instead of the production one. Usage
	// errors must fail before that file is ever touched.
	dbPath := filepath.Join(t.TempDir(), "appointments.db")
	t.Setenv("MCP_DB_PATH", dbPath)

	tests := []struct {
		name     string
		args     []string
		wantText []string
	}{
		{
			name:     "admin tui enters the owner seed gateway",
			args:     []string{"admin", "tui"},
			wantText: []string{"No hay ningún owner activo", "El administrador del sistema operativo es el gatekeeper"},
		},
		{
			name:     "hermes chat reaches the not-implemented stub",
			args:     []string{"hermes", "chat"},
			wantText: []string{"chat de Hermes todavía no está implementado"},
		},
		{
			name:     "admin without sub-command lists the valid sub-commands",
			args:     []string{"admin"},
			wantText: []string{"sub-comando faltante para", "sub-comando válido es", "tui", usageHint},
		},
		{
			name:     "hermes without sub-command lists the valid sub-commands",
			args:     []string{"hermes"},
			wantText: []string{"sub-comando faltante para", "sub-comando válido es", "chat", usageHint},
		},
		{
			name:     "unknown argument fails fast with a usage hint",
			args:     []string{"--help"},
			wantText: []string{"argumento desconocido", usageHint},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := exec.CommandContext(context.Background(), bin, tt.args...) //nolint:gosec // test executes TempDir-built binary with fixed args
			out, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatalf("binary %v exited 0, want a non-zero exit\n%s", tt.args, out)
			}

			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("binary %v could not run: %v\n%s", tt.args, err, out)
			}
			if code := exitErr.ExitCode(); code != 1 {
				t.Errorf("binary %v exit code = %d, want 1\n%s", tt.args, code, out)
			}

			for _, want := range tt.wantText {
				if !strings.Contains(string(out), want) {
					t.Errorf("binary %v output missing %q\n%s", tt.args, want, out)
				}
			}
		})
	}
}

func TestNewIdentityDepsWiresIdentityRepos(t *testing.T) {
	database, err := db.NewDatabase(context.Background(), filepath.Join(t.TempDir(), "appointments.db"))
	if err != nil {
		t.Fatalf("db.NewDatabase() failed: %v", err)
	}
	defer closeDatabase(database, slog.Default())

	deps := newIdentityDeps(database, slog.Default())

	if deps.accounts == nil {
		t.Error("newIdentityDeps() accounts repo = nil, want an *AccountsRepo")
	}
	if deps.clients == nil {
		t.Error("newIdentityDeps() clients repo = nil, want a *ClientsRepo")
	}
	if deps.professionals == nil {
		t.Error("newIdentityDeps() professionals repo = nil, want a *ProfessionalsRepo")
	}
}
