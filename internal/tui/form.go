package tui

import (
	"strconv"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

// maxInputRunes caps every operator text field. The longest legitimate value is
// a display name or an E.164 phone; anything longer is a paste accident, and a
// bounded field keeps the rendering and the validation predictable.
const maxInputRunes = 80

// formStep is one question of a wizard: the label the operator reads, the
// optional default that Enter accepts, and the validator that must accept the
// answer before the wizard advances. The validators are the core ones
// (admin.ValidatePhone / admin.ValidateDisplayName): the TUI owns no second
// copy of the validation rule (ADR-0016 Decision 3.2, AGENTS.md TUI input
// validation).
type formStep struct {
	label        string
	defaultValue string
	validate     func(string) error
}

// formResult is the outcome of submitting one step of a wizard.
type formResult int

const (
	// formRejected means the answer is invalid: the wizard stays on the step and
	// renders the semantic error. Enter is never able to advance past an
	// invalid answer.
	formRejected formResult = iota
	// formAdvanced means the answer was accepted and the next step is focused.
	formAdvanced
	// formDone means the answer was accepted and every step is collected.
	formDone
)

// formModel is a wizard of one or more validated text steps backed by a single
// bubbles/textinput: only one step is visible and focused at a time, so the
// operator keeps real cursor editing (arrows, backspace, paste) without the TUI
// reimplementing line editing.
//
// The form is pure input: it collects validated strings and never calls the
// core. The AppModel maps the collected answers onto the core input type of the
// flow (admin.SeedInput, admin.StaffInput, admin.TransferSuccessor).
type formModel struct {
	title  string
	steps  []formStep
	index  int
	input  textinput.Model
	values []string
	err    string
}

// newForm builds a wizard for the given steps and focuses the first one.
func newForm(title string, steps []formStep) formModel {
	input := textinput.New()
	input.Prompt = "› "
	input.CharLimit = maxInputRunes
	// A steady block cursor. The operator fills one field at a time, so the
	// blinking mode would only add a framework timer command that a headless
	// model test would have to execute and wait out; steady is also calmer to
	// read.
	input.Cursor.SetMode(cursor.CursorStatic)

	form := formModel{
		title:  title,
		steps:  steps,
		values: make([]string, len(steps)),
		input:  input,
	}
	form.focus()
	return form
}

// focus loads the current step into the input component and clears the step
// error, so a rejected answer is never rendered against the next question.
func (f *formModel) focus() {
	f.input.SetValue(f.steps[f.index].defaultValue)
	f.input.CursorEnd()
	f.input.Focus()
	f.err = ""
}

// value returns the accepted answer of step i. It is empty until that step was
// accepted.
func (f formModel) value(i int) string {
	if i < 0 || i >= len(f.values) {
		return ""
	}
	return f.values[i]
}

// submit validates the current answer and advances. An empty answer falls back
// to the step default when it has one, exactly like the console prompt did: the
// value handed to the validator is the value that would be written.
func (f *formModel) submit() formResult {
	step := f.steps[f.index]
	value := strings.TrimSpace(f.input.Value())
	if value == "" && step.defaultValue != "" {
		value = step.defaultValue
	}

	if err := step.validate(value); err != nil {
		f.err = err.Error()
		return formRejected
	}

	f.values[f.index] = value
	f.err = ""
	if f.index == len(f.steps)-1 {
		return formDone
	}

	f.index++
	f.focus()
	return formAdvanced
}

// update forwards a message to the focused input component. The AppModel
// intercepts Enter, Esc and the quit keys before calling this, so only editing
// keys reach the text field.
func (f *formModel) update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	f.input, cmd = f.input.Update(msg)
	return cmd
}

// stepCounter renders the "campo i de n" progress label. The operator always
// knows how much of the wizard is left, which is what makes an interrupted form
// safe to leave with Esc.
func (f formModel) stepCounter() string {
	if len(f.steps) <= 1 {
		return ""
	}
	return " (campo " + strconv.Itoa(f.index+1) + " de " + strconv.Itoa(len(f.steps)) + ")"
}
