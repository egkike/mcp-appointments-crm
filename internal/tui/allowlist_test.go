package tui

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The advisory is read-only operator copy: these tests pin the exact strings the
// console prints, so a wording change is a deliberate diff and never an
// accident. No test executes a real binary — every case goes through the
// ToolsListRunner seam with a fake.

// fakeToolsList builds a ToolsListRunner that answers every platform with the
// same output/error and records the platforms it was asked about.
func fakeToolsList(output string, err error, asked *[]string) ToolsListRunner {
	return func(_ context.Context, platform string) (string, error) {
		if asked != nil {
			*asked = append(*asked, platform)
		}
		return output, err
	}
}

// minimalHermesToolsList is the documented shape of a channel already reduced to
// the recommended allowlist (`hermes tools list --platform <p>`).
const minimalHermesToolsList = `  ✓ enabled  skills · todo · memory · clarify
  MCP servers:
    mcp-appointments  all tools enabled
`

// dangerousHermesToolsList is the default messaging bundle of the runtime:
// full access, terminal and file included.
const dangerousHermesToolsList = `  ✓ enabled  skills · todo · memory · clarify · terminal · file · code_execution
  MCP servers:
    mcp-appointments  all tools enabled
`

// expectedMinimalWhatsAppLine is the single line of state 1, verbatim.
const expectedMinimalWhatsAppLine = "Allowlist del canal whatsapp: mínima ✓ (skills, todo, memory, clarify + mcp-appointments)\n"

// expectedMinimalTelegramLine is the same state for the second documented channel.
const expectedMinimalTelegramLine = "Allowlist del canal telegram: mínima ✓ (skills, todo, memory, clarify + mcp-appointments)\n"

// expectedWhatsAppDisableCommand is the exact remediation the operator must run
// for a channel whose bundle is still full access.
const expectedWhatsAppDisableCommand = "hermes tools disable --platform whatsapp code_execution terminal file browser web computer_use delegation cronjob image_gen tts vision session_search connections homeassistant"

// TestChannelAllowlistAdvice covers the three states of one channel: the
// minimal one-liner, the blocked block with the remediation command, and the
// undecidable cases (runner failure, empty output) that must NOT claim safety.
func TestChannelAllowlistAdvice(t *testing.T) {
	tests := []struct {
		name        string
		channel     string
		output      string
		err         error
		wantExact   string
		wantContain []string
		wantOmit    []string
	}{
		{
			name:      "minimal allowlist prints one line",
			channel:   "whatsapp",
			output:    minimalHermesToolsList,
			wantExact: expectedMinimalWhatsAppLine,
			wantOmit:  []string{"hermes tools disable"},
		},
		{
			name:    "dangerous toolsets print the block with the command",
			channel: "whatsapp",
			output:  dangerousHermesToolsList,
			wantContain: []string{
				"Allowlist del canal whatsapp:",
				strings.TrimRight(dangerousHermesToolsList, "\n"),
				"mcp-appointments, skills, clarify, memory, todo",
				expectedWhatsAppDisableCommand,
				`docs/installation.md → "Allowlist mínima recomendada"`,
			},
			wantOmit: []string{"mínima ✓"},
		},
		{
			name:    "hermes absent is not evidence of a minimal allowlist",
			channel: "whatsapp",
			err:     errors.New("exec: \"hermes\": executable file not found in $PATH"),
			wantOmit: []string{
				"mínima ✓",
				"hermes tools disable",
				"executable file not found",
			},
		},
		{
			name:     "a failed command is not evidence of a minimal allowlist",
			channel:  "telegram",
			output:   "Traceback (most recent call last):",
			err:      errors.New("exit status 2"),
			wantOmit: []string{"mínima ✓", "hermes tools disable", "Traceback"},
		},
		{
			name:     "unparsable output falls back to the commands",
			channel:  "telegram",
			output:   "warning: unknown platform\n",
			wantOmit: []string{"mínima ✓"},
			wantContain: []string{
				"hermes tools disable --platform telegram",
			},
		},
		{
			name:     "empty output is not evidence of a minimal allowlist",
			channel:  "whatsapp",
			output:   "   \n",
			wantOmit: []string{"mínima ✓", "hermes tools disable"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ChannelAllowlistAdvice(context.Background(), tt.channel,
				fakeToolsList(tt.output, tt.err, nil))

			if tt.wantExact != "" && got != tt.wantExact {
				t.Fatalf("ChannelAllowlistAdvice() = %q, want %q", got, tt.wantExact)
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("advice = %q, want it to contain %q", got, want)
				}
			}
			for _, omit := range tt.wantOmit {
				if strings.Contains(got, omit) {
					t.Errorf("advice = %q, must not contain %q", got, omit)
				}
			}
		})
	}
}

// TestChannelAllowlistAdvice_UnauditableReturnsEmpty pins the contract the
// console relies on: an empty advice means "this channel could not be audited",
// so the composer decides whether the generic advisory is due.
func TestChannelAllowlistAdvice_UnauditableReturnsEmpty(t *testing.T) {
	for _, tt := range []struct {
		name   string
		output string
		err    error
	}{
		{name: "runner error", err: errors.New("boom")},
		{name: "blank output", output: "\n\t\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := ChannelAllowlistAdvice(context.Background(), "whatsapp",
				fakeToolsList(tt.output, tt.err, nil)); got != "" {
				t.Errorf("ChannelAllowlistAdvice() = %q, want an empty string", got)
			}
		})
	}
}

// TestAllowlistAdvisory_OneSectionPerChannel locks the multi-channel contract:
// every audited channel gets its own section and a channel that cannot be
// audited is named instead of vanishing (native review R3-partial-audit-silent-skip).
func TestAllowlistAdvisory_OneSectionPerChannel(t *testing.T) {
	var asked []string
	runner := func(_ context.Context, platform string) (string, error) {
		asked = append(asked, platform)
		if platform == "telegram" {
			return "", errors.New("hermes tools list failed")
		}
		return minimalHermesToolsList, nil
	}

	got := AllowlistAdvisory(context.Background(), []string{"whatsapp", "telegram"}, runner)

	// The audited channel keeps its one-liner and the unauditable one is named:
	// a partial audit that printed nothing for telegram would read as "both fine".
	if !strings.HasPrefix(got, "\n"+expectedMinimalWhatsAppLine) {
		t.Errorf("AllowlistAdvisory() = %q, want it to start with %q", got, expectedMinimalWhatsAppLine)
	}
	for _, want := range []string{
		"Allowlist del canal telegram: no se pudo auditar",
		"el comando `hermes tools list` falló",
		"sólo los tools del CRM",
		"ninguna ejecución de código o shell",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("AllowlistAdvisory() = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "cualquier otro agente con canal") {
		t.Errorf("AllowlistAdvisory() = %q, must not print the generic advice when a channel was audited", got)
	}
	if len(asked) != 2 || asked[0] != "whatsapp" || asked[1] != "telegram" {
		t.Errorf("audited platforms = %v, want [whatsapp telegram]", asked)
	}
}

// TestAllowlistAdvisory_GenericNamesTheFailureCategory locks R4-001: the three
// ways an audit can fail are operationally different, so the generic advisory
// names the category instead of collapsing them into one sentence. Raw error text
// stays out of the console either way.
func TestAllowlistAdvisory_GenericNamesTheFailureCategory(t *testing.T) {
	tests := []struct {
		name        string
		output      string
		err         error
		wantContain string
		wantOmit    []string
	}{
		{
			name:        "binary unavailable",
			err:         &exec.Error{Name: hermesBinary, Err: exec.ErrNotFound},
			wantContain: "binario `hermes` no disponible en este host",
			wantOmit:    []string{"executable file not found", hermesBinary + ": "},
		},
		{
			name:        "command failed",
			err:         errors.New("exit status 2"),
			wantContain: "el comando `hermes tools list` falló",
			wantOmit:    []string{"exit status"},
		},
		{
			name:        "empty report",
			output:      "   \n",
			wantContain: "el comando no devolvió ningún reporte",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AllowlistAdvisory(context.Background(), []string{"whatsapp", "telegram"},
				fakeToolsList(tt.output, tt.err, nil))

			for _, want := range []string{"no se pudo auditar", tt.wantContain} {
				if !strings.Contains(got, want) {
					t.Errorf("generic advice = %q, want it to contain %q", got, want)
				}
			}
			for _, omit := range tt.wantOmit {
				if strings.Contains(got, omit) {
					t.Errorf("generic advice = %q, must not contain %q", got, omit)
				}
			}
		})
	}
}

// TestAllowlistAdvisory_GenericReportsEveryCategory locks the mixed-host case:
// when the channels failed for different reasons the operator sees both, so
// fixing the PATH is not confused with a broken command.
func TestAllowlistAdvisory_GenericReportsEveryCategory(t *testing.T) {
	runner := func(_ context.Context, platform string) (string, error) {
		if platform == "whatsapp" {
			return "", &exec.Error{Name: hermesBinary, Err: exec.ErrNotFound}
		}
		return "", errors.New("exit status 2")
	}

	got := AllowlistAdvisory(context.Background(), []string{"whatsapp", "telegram"}, runner)

	for _, want := range []string{
		"binario `hermes` no disponible en este host",
		"el comando `hermes tools list` falló",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generic advice = %q, want it to contain %q", got, want)
		}
	}
}

// TestChannelAllowlistAdvice_DerivesExtraToolsetsFromReport locks R1-001: the
// printed command must cover every toolset the report shows as enabled beyond the
// minimal set, even one the documented list does not name yet. Documented names
// come first (so the known command stays recognizable), derived names after it in
// report order.
func TestChannelAllowlistAdvice_DerivesExtraToolsetsFromReport(t *testing.T) {
	unknownToolsets := `  ✓ enabled  skills · todo · memory · clarify · notebook · terminal · alpha
  MCP servers:
    mcp-appointments  all tools enabled
`

	got := ChannelAllowlistAdvice(context.Background(), "whatsapp",
		fakeToolsList(unknownToolsets, nil, nil))

	wantCommand := expectedWhatsAppDisableCommand + " notebook alpha"
	if !strings.Contains(got, wantCommand) {
		t.Errorf("advice = %q, want the command %q", got, wantCommand)
	}
	if strings.Contains(got, "terminal notebook") || strings.Contains(got, "alpha notebook") {
		t.Errorf("advice = %q, want the derived names in report order after the documented list", got)
	}
}

// TestChannelAllowlistAdvice_DocumentedCommandWithoutExtras keeps the synthetic
// case honest: a report whose extras are all documented prints the literal
// documented command, unchanged.
func TestChannelAllowlistAdvice_DocumentedCommandWithoutExtras(t *testing.T) {
	documentedOnly := `  ✓ enabled  skills · todo · memory · clarify · terminal · file
  MCP servers:
    mcp-appointments  all tools enabled
`

	got := ChannelAllowlistAdvice(context.Background(), "whatsapp",
		fakeToolsList(documentedOnly, nil, nil))

	if !strings.Contains(got, expectedWhatsAppDisableCommand+"\n") {
		t.Errorf("advice = %q, want the documented command %q", got, expectedWhatsAppDisableCommand)
	}
}

// TestBoundedRuntimeReport locks R4-002: the runtime report is printed verbatim
// but bounded, and a truncated report says so and names how to see it whole.
func TestBoundedRuntimeReport(t *testing.T) {
	small := "  ✓ enabled  skills · todo\n  MCP servers:\n"
	if got := boundedRuntimeReport("whatsapp", small); got != strings.TrimRight(small, "\n") {
		t.Errorf("boundedRuntimeReport(small) = %q, want the report unchanged", got)
	}
	if strings.Contains(boundedRuntimeReport("whatsapp", small), "truncado") {
		t.Error("boundedRuntimeReport(small) must not claim a truncation")
	}

	var big strings.Builder
	for i := 0; i < 4000; i++ {
		big.WriteString("    tool_that_does_something_verbose  enabled\n")
	}
	long := big.String()

	got := boundedRuntimeReport("whatsapp", long)
	marker := "[… reporte truncado: se muestran sólo los primeros 8192 bytes; corré `hermes tools list --platform whatsapp` para verlo completo]"
	if !strings.HasSuffix(got, marker) {
		t.Errorf("boundedRuntimeReport(long) = %q, want it to end with %q", got, marker)
	}
	if body := strings.TrimSuffix(got, "\n"+marker); len(body) > 8192 {
		t.Errorf("report body = %d bytes, want at most 8192", len(body))
	}
	if !strings.HasSuffix(strings.TrimSuffix(got, "\n"+marker), "verbose  enabled") {
		t.Errorf("boundedRuntimeReport(long) = %q, want the cut on a line boundary", got)
	}
}

// TestChannelAllowlistAdvice_BoundsTheRuntimeReport locks the console-level
// effect of R4-002: an oversized report can never flood the terminal.
func TestChannelAllowlistAdvice_BoundsTheRuntimeReport(t *testing.T) {
	var big strings.Builder
	big.WriteString("  ✓ enabled  skills · terminal\n  MCP servers:\n")
	for i := 0; i < 4000; i++ {
		big.WriteString("    tool_that_does_something_verbose  enabled\n")
	}

	got := ChannelAllowlistAdvice(context.Background(), "whatsapp",
		fakeToolsList(big.String(), nil, nil))

	if len(got) > 8192+2048 {
		t.Errorf("advice is %d bytes, want it bounded near 8192", len(got))
	}
	if !strings.Contains(got, "reporte truncado") {
		t.Errorf("advice = %q, want the truncation marker", got)
	}
	if !strings.Contains(got, "hermes tools list --platform whatsapp") {
		t.Errorf("advice = %q, want the marker to name the whole-report command", got)
	}
}

// TestHermesToolsListContext_IgnoresCallerCancellation locks R3-timeout-context-inherit:
// the probe owns its 5s budget instead of inheriting the caller's cancellation, so
// an unrelated abort of the console flow cannot cancel the audit.
func TestHermesToolsListContext_IgnoresCallerCancellation(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	probe, cancelProbe := hermesToolsListContext(parent)
	defer cancelProbe()

	if err := probe.Err(); err != nil {
		t.Fatalf("probe context error = %v, want nil: the audit must not inherit the caller's cancellation", err)
	}
	deadline, ok := probe.Deadline()
	if !ok {
		t.Fatal("probe context has no deadline, want the 5s audit budget")
	}
	if remaining := time.Until(deadline); remaining > hermesToolsListTimeout || remaining < hermesToolsListTimeout-time.Second {
		t.Errorf("probe budget = %v, want ~%v", remaining, hermesToolsListTimeout)
	}
}

// TestAllowlistAdvisory_PrintsBothMinimalLines locks the all-good case: one line
// per documented channel, nothing else.
func TestAllowlistAdvisory_PrintsBothMinimalLines(t *testing.T) {
	got := AllowlistAdvisory(context.Background(), []string{"whatsapp", "telegram"},
		fakeToolsList(minimalHermesToolsList, nil, nil))

	want := "\n" + expectedMinimalWhatsAppLine + expectedMinimalTelegramLine
	if got != want {
		t.Errorf("AllowlistAdvisory() = %q, want %q", got, want)
	}
}

// TestAllowlistAdvisory_GenericWhenNothingCanBeAudited locks the risk that is
// NOT Hermes-specific: when no channel can be audited the operator still gets a
// single advisory, and it names any channelled agent instead of only Hermes.
func TestAllowlistAdvisory_GenericWhenNothingCanBeAudited(t *testing.T) {
	got := AllowlistAdvisory(context.Background(), []string{"whatsapp", "telegram"},
		fakeToolsList("", errors.New("exec: hermes: not found"), nil))

	for _, want := range []string{
		"no se pudo auditar",
		"cualquier otro agente con canal",
		"sólo los tools del CRM",
		"ninguna ejecución de código o shell",
		`docs/installation.md → "Allowlist mínima recomendada"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("generic advice = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "hermes tools disable") {
		t.Errorf("generic advice = %q, must not prescribe hermes commands for unknown agents", got)
	}
	if strings.Contains(got, "not found") {
		t.Errorf("generic advice = %q, must never echo raw command errors", got)
	}
}

// TestChannelAllowlistIsMinimal locks the conservative detector: anything the
// parser cannot decide with confidence is reported as NOT minimal, because
// claiming safety is the expensive mistake.
func TestChannelAllowlistIsMinimal(t *testing.T) {
	tests := []struct {
		name   string
		output string
		want   bool
	}{
		{name: "documented minimal shape", output: minimalHermesToolsList, want: true},
		{
			name:   "dangerous bundle",
			output: dangerousHermesToolsList,
			want:   false,
		},
		{
			name: "minimal toolsets plus an unknown extra",
			output: `  ✓ enabled  skills · todo · memory · clarify · notebook
  MCP servers:
    mcp-appointments  all tools enabled
`,
			want: false,
		},
		{
			name: "missing a minimal toolset",
			output: `  ✓ enabled  skills · todo · memory
  MCP servers:
    mcp-appointments  all tools enabled
`,
			want: false,
		},
		{
			name: "no MCP server line",
			output: `  ✓ enabled  skills · todo · memory · clarify
`,
			want: false,
		},
		{
			name: "unparsable output",
			output: `everything looks great, trust me
`,
			want: false,
		},
		{name: "empty output", output: "", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := channelAllowlistIsMinimal(tt.output); got != tt.want {
				t.Errorf("channelAllowlistIsMinimal(%q) = %v, want %v", tt.output, got, tt.want)
			}
		})
	}
}
