package tui

import (
	"context"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// TestRunProgramQuitsCleanly covers the program assembly: the model tests prove
// the state machine, and this test proves that the assembled Bubble Tea program
// starts and exits without a terminal. A headless assembly needs no renderer and
// no input, so the test injects the quit a terminal operator would type.
func TestRunProgramQuitsCleanly(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)

	program := newProgram(
		NewAppModel(context.Background(), f.deps),
		tea.WithInput(nil),
		tea.WithoutRenderer(),
		tea.WithoutSignalHandler(),
	)

	done := make(chan error, 1)
	go func() { done <- runProgram(program) }()

	program.Send(tea.QuitMsg{})

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("runProgram() = %v, want a clean exit", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("the program did not exit after the quit message")
	}
}
