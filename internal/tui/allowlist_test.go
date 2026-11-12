package tui

import (
	"context"
	"errors"
	"strings"
	"testing"
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
// audited is skipped silently.
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

	if want := "\n" + expectedMinimalWhatsAppLine; got != want {
		t.Errorf("AllowlistAdvisory() = %q, want %q", got, want)
	}
	if len(asked) != 2 || asked[0] != "whatsapp" || asked[1] != "telegram" {
		t.Errorf("audited platforms = %v, want [whatsapp telegram]", asked)
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
