package tui

import (
	"context"
	"errors"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// Run assembles and runs the single Bubble Tea program of `mcp-server admin tui`
// over the same reviewed core the line-based flow calls.
//
// A user quit (q or Ctrl+C) is a clean exit, including the Ctrl+C that reaches
// the process as a signal instead of a key. A terminal that cannot be
// initialized is reported as a semantic error, so the operator reads why the
// interface did not open instead of a raw termios failure.
func Run(ctx context.Context, deps Deps) error {
	program := newProgram(
		NewAppModel(ctx, deps),
		tea.WithAltScreen(),
		tea.WithInput(os.Stdin),
		tea.WithOutput(os.Stdout),
	)
	return runProgram(program)
}

// newProgram assembles the program from the model and the terminal options. It is
// split from Run so a headless test can assemble the same program without a
// terminal (no renderer, no input) and drive the shutdown that a terminal
// operator would trigger with q.
func newProgram(model tea.Model, options ...tea.ProgramOption) *tea.Program {
	return tea.NewProgram(model, options...)
}

// runProgram blocks until the operator quits or the process is killed.
func runProgram(program *tea.Program) error {
	if _, err := program.Run(); err != nil {
		if errors.Is(err, tea.ErrProgramKilled) {
			return nil
		}
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo iniciar la interfaz del operador",
			Cause:   err,
		}
	}
	return nil
}
