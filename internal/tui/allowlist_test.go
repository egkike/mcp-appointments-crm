package tui

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
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

// TestDefaultAllowlistPlatforms pins the audited channels to the two the product
// documents end-to-end (whatsapp as the primary business channel, telegram as the
// alternative one plus the owner bot) and pins that callers get a copy: the list
// is the single source both presentations read, so no caller can reshape what the
// next audit asks about.
func TestDefaultAllowlistPlatforms(t *testing.T) {
	want := []string{"whatsapp", "telegram"}
	if got := DefaultAllowlistPlatforms(); !slices.Equal(got, want) {
		t.Fatalf("DefaultAllowlistPlatforms() = %v, want %v", got, want)
	}

	mutated := append(DefaultAllowlistPlatforms(), "slack")
	mutated[0] = "discord"
	if got := DefaultAllowlistPlatforms(); !slices.Equal(got, want) {
		t.Errorf("DefaultAllowlistPlatforms() after a caller mutated its slice = %v, want %v", got, want)
	}
}

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
	body, marker := splitBoundedReport(t, got)
	if len(body) > maxRuntimeReportBytes {
		t.Errorf("report body = %d bytes, want at most %d", len(body), maxRuntimeReportBytes)
	}
	if !strings.HasSuffix(body, "verbose  enabled") {
		t.Errorf("boundedRuntimeReport(long) = %q, want the cut on a line boundary", got)
	}
	// R3-marker-precision: the marker names what was ACTUALLY kept, not the cap,
	// so the operator is never told a larger number than they can read.
	if want := fmt.Sprintf("los primeros %d bytes", len(body)); !strings.Contains(marker, want) {
		t.Errorf("marker = %q, want it to contain %q", marker, want)
	}
	if !strings.Contains(marker, "hermes tools list --platform whatsapp") {
		t.Errorf("marker = %q, want it to name the whole-report command", marker)
	}
}

// splitBoundedReport separates the kept report body from the truncation marker
// the bounded report appends, so a test can assert both independently: the body
// length is what the marker must quote, and quoting a self-computed length would
// make the assertion circular.
func splitBoundedReport(t *testing.T, bounded string) (body, marker string) {
	t.Helper()
	idx := strings.LastIndex(bounded, "\n[")
	if idx < 0 {
		t.Fatalf("bounded report = %q, want a truncation marker", bounded)
	}
	return bounded[:idx], bounded[idx+1:]
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

// hostileToolsList is a runtime report whose enabled-toolset line carries names
// that are NOT toolset identifiers: a report is runtime-owned data, so a name
// with spaces, `;`, `$(`, quotes or a pipe is attacker-controlled the moment the
// operator pastes the printed command into a shell.
const hostileToolsList = `  ✓ enabled  skills · todo · memory · clarify · notebook · hermes-whatsapp · alpha; touch /tmp/pwned · $(id) · "quoted" | sh
  MCP servers:
    mcp-appointments  all tools enabled
`

// TestDisableCommandToolsets_DropsUnsafeReportNames locks R1-report-derived-command:
// the remediation command is copied and pasted into a shell, so only plain
// toolset identifiers may be appended to it. Legitimate extras the documented
// list does not name yet are still derived from the report.
func TestDisableCommandToolsets_DropsUnsafeReportNames(t *testing.T) {
	names := disableCommandToolsets(hostileToolsList)

	want := append(slices.Clone(dangerousChannelToolsets), "notebook", "hermes-whatsapp")
	if !slices.Equal(names, want) {
		t.Fatalf("disableCommandToolsets() = %v, want %v", names, want)
	}
	for _, name := range names {
		if strings.ContainsAny(name, " \t\n;&|`$()\"'") {
			t.Errorf("toolset name %q carries shell metacharacters", name)
		}
	}
}

// TestChannelAllowlistAdvice_RemediationCommandIsSafeButReportIsVerbatim pins
// both halves of the R1 contract at the console level: the printed command
// carries no injected name, while the report itself is still printed as the
// runtime sent it (bounded), so dropping the name from the command hides nothing
// from the operator.
func TestChannelAllowlistAdvice_RemediationCommandIsSafeButReportIsVerbatim(t *testing.T) {
	got := ChannelAllowlistAdvice(context.Background(), "whatsapp",
		fakeToolsList(hostileToolsList, nil, nil))

	command := remediationCommandLine(t, got)
	if !strings.HasSuffix(command, " notebook hermes-whatsapp") {
		t.Errorf("command = %q, want it to end with the safe derived names", command)
	}
	for _, injected := range []string{"touch /tmp/pwned", "$(id)", `"quoted"`, "| sh"} {
		if strings.Contains(command, injected) {
			t.Errorf("command = %q, must not carry the report name %q", command, injected)
		}
	}
	for _, reported := range []string{"alpha; touch /tmp/pwned", "$(id)"} {
		if !strings.Contains(got, reported) {
			t.Errorf("advice = %q, want the verbatim report to keep %q", got, reported)
		}
	}
}

// remediationCommandLine extracts the `hermes tools disable` line from the
// advice, so shell-safety assertions are scoped to the command the operator
// copies instead of the whole block (which legitimately prints the report).
func remediationCommandLine(t *testing.T, advice string) string {
	t.Helper()
	for _, line := range strings.Split(advice, "\n") {
		if strings.HasPrefix(line, hermesBinary+" tools disable") {
			return line
		}
	}
	t.Fatalf("advice = %q, want a %q line", advice, hermesBinary+" tools disable")
	return ""
}

// TestValidUTF8Prefix_DropsOnlyTheCutRune locks R3-unbounded-validutf8: a byte
// boundary cut leaves at most three bytes of ONE broken rune (UTF-8 encodes a
// rune in at most four bytes), so the backwalk is bounded and the prefix is
// always valid UTF-8 without re-scanning the whole string per byte dropped.
func TestValidUTF8Prefix_DropsOnlyTheCutRune(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "empty", input: "", want: ""},
		{name: "ascii only", input: "abc", want: "abc"},
		{name: "valid multi-byte tail is kept", input: "abcé", want: "abcé"},
		{name: "one byte of a two-byte rune", input: "abcé"[:4], want: "abc"},
		{name: "two bytes of a three-byte rune", input: "abc€"[:5], want: "abc"},
		{name: "three bytes of a four-byte rune", input: "abc🙂"[:6], want: "abc"},
		{name: "report that is not UTF-8 at all", input: strings.Repeat("\xff", 8), want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validUTF8Prefix(tt.input)
			if got != tt.want {
				t.Errorf("validUTF8Prefix(%q) = %q, want %q", tt.input, got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Errorf("validUTF8Prefix(%q) = %q, want valid UTF-8", tt.input, got)
			}
		})
	}
}

// TestBoundedRuntimeReport_MultiByteCutStaysValidUTF8 locks the console-visible
// effect of R3-unbounded-validutf8: a report full of multi-byte characters cut
// mid-rune still prints valid UTF-8, and the marker quotes the bytes kept.
func TestBoundedRuntimeReport_MultiByteCutStaysValidUTF8(t *testing.T) {
	// 1 + 8200 bytes: the 8192-byte cut lands in the middle of an "é".
	report := "x" + strings.Repeat("é", 4100)

	got := boundedRuntimeReport("whatsapp", report)
	if !utf8.ValidString(got) {
		t.Fatalf("bounded report is not valid UTF-8: %q", got)
	}
	body, marker := splitBoundedReport(t, got)
	if want := fmt.Sprintf("los primeros %d bytes", len(body)); !strings.Contains(marker, want) {
		t.Errorf("marker = %q, want it to contain %q", marker, want)
	}
	if !strings.HasPrefix(body, "xé") {
		t.Errorf("report body = %q, want the report's own content", body)
	}
}

// TestBoundedRuntimeReport_UnprintableBodyExplainsItself locks R4-001: when the
// valid prefix is empty (a report that opens with bytes that are not UTF-8) the
// block must not print a header, an empty body and a marker — it prints one
// fixed line saying the report could not be shown and how to see it.
func TestBoundedRuntimeReport_UnprintableBodyExplainsItself(t *testing.T) {
	report := strings.Repeat("\xff", maxRuntimeReportBytes+16)

	got := boundedRuntimeReport("whatsapp", report)
	if !utf8.ValidString(got) {
		t.Fatalf("bounded report is not valid UTF-8: %q", got)
	}
	body, marker := splitBoundedReport(t, got)
	if strings.TrimSpace(body) == "" {
		t.Errorf("report body is empty, want the fixed explanation line")
	}
	for _, want := range []string{"no se pudo mostrar", "hermes tools list --platform whatsapp"} {
		if !strings.Contains(body, want) {
			t.Errorf("report body = %q, want it to contain %q", body, want)
		}
	}
	// The marker stays, with the honest count: zero report bytes were shown.
	if !strings.Contains(marker, "los primeros 0 bytes") {
		t.Errorf("marker = %q, want the truncation marker with the bytes actually kept", marker)
	}
}

// TestValidUTF8Prefix_KeepsTheValidPrefixBeforeACorruptedByte locks R4-001: an
// invalid byte in the MIDDLE of the string is not a mid-rune cut, so it must not
// discard the whole prefix — the operator keeps everything the runtime said
// before the corruption instead of losing the report for one bad byte.
func TestValidUTF8Prefix_KeepsTheValidPrefixBeforeACorruptedByte(t *testing.T) {
	input := "✓ enabled  skills" + "\xff" + "todo\n"

	got := validUTF8Prefix(input)

	if !utf8.ValidString(got) {
		t.Fatalf("validUTF8Prefix(corrupted middle byte) = %q, want valid UTF-8", got)
	}
	if want := "✓ enabled  skills"; got != want {
		t.Errorf("validUTF8Prefix(corrupted middle byte) = %q, want the valid prefix %q", got, want)
	}
}

// TestBoundedRuntimeReport_CorruptedMiddleByteKeepsThePrefix locks the
// operator-visible half of R4-001: a report that is valid up to a corrupted byte
// inside the printed window still shows what it could read instead of collapsing
// to the fixed "could not be shown" line.
func TestBoundedRuntimeReport_CorruptedMiddleByteKeepsThePrefix(t *testing.T) {
	head := "  ✓ enabled  skills · terminal"
	// The corrupted byte sits well inside the 8192-byte printed window, and the
	// report is longer than the cap, so the cut is a plain byte-boundary cut with
	// one bad byte behind it.
	report := head + "\n" + strings.Repeat("f", 4000-len(head)-1) + "\xff" + strings.Repeat("tail\n", 2000)
	if len(report) <= maxRuntimeReportBytes {
		t.Fatalf("report is %d bytes, want it past the %d-byte cap", len(report), maxRuntimeReportBytes)
	}

	got := boundedRuntimeReport("whatsapp", report)
	if !utf8.ValidString(got) {
		t.Fatalf("bounded report is not valid UTF-8: %q", got)
	}
	body, _ := splitBoundedReport(t, got)
	if !strings.HasPrefix(body, head) {
		t.Errorf("report body = %q, want it to keep the report's valid head %q", body, head)
	}
	if strings.Contains(body, "no se pudo mostrar") {
		t.Errorf("report body = %q, must not discard the whole report for one corrupted middle byte", body)
	}
}

// TestChannelAllowlistAdvice_UnprintableReportKeepsTheBlockReadable locks the
// operator-level shape of R4-001: the blocked block still names the state and the
// remediation command, and the unprintable report is explained instead of
// looking like a broken printer.
func TestChannelAllowlistAdvice_UnprintableReportKeepsTheBlockReadable(t *testing.T) {
	got := ChannelAllowlistAdvice(context.Background(), "whatsapp",
		fakeToolsList(strings.Repeat("\xff", maxRuntimeReportBytes+16), nil, nil))

	for _, want := range []string{
		"Allowlist del canal whatsapp: no puedo confirmar que sea la mínima",
		"no se pudo mostrar",
		"hermes tools disable --platform whatsapp",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("advice = %q, want it to contain %q", got, want)
		}
	}
	if strings.Contains(got, "\xff") {
		t.Errorf("advice = %q, must not print non-UTF-8 report bytes", got)
	}
}

// TestHermesToolsListContext_UsesTheDefaultBudget locks one half of
// R3-withoutcancel-values: with no deadline to honour, the probe gets its own 5s
// budget — also when the caller carried a LATER one, which the probe must not
// inherit (it only ever shortens, never extends).
func TestHermesToolsListContext_UsesTheDefaultBudget(t *testing.T) {
	for _, tt := range []struct {
		name  string
		build func() (context.Context, context.CancelFunc)
	}{
		{
			name:  "no caller deadline",
			build: func() (context.Context, context.CancelFunc) { return context.WithCancel(context.Background()) },
		},
		{
			name: "later caller deadline",
			build: func() (context.Context, context.CancelFunc) {
				return context.WithTimeout(context.Background(), time.Minute)
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			parent, cancelParent := tt.build()
			defer cancelParent()

			probe, cancelProbe := hermesToolsListContext(parent)
			defer cancelProbe()

			deadline, ok := probe.Deadline()
			if !ok {
				t.Fatal("probe context has no deadline, want the 5s audit budget")
			}
			if remaining := time.Until(deadline); remaining > hermesToolsListTimeout || remaining < hermesToolsListTimeout-time.Second {
				t.Errorf("probe budget = %v, want ~%v", remaining, hermesToolsListTimeout)
			}
		})
	}
}

// TestHermesToolsListContext_RespectsAnEarlierCallerDeadline locks the other half
// of R3-withoutcancel-values: detaching cancellation must not detach a real
// deadline — an earlier caller deadline wins over the 5s budget. The margin is
// deliberately generous and above hermesToolsListMinProbeBudget (900ms, no sleep
// involved): the caller's deadline only counts while it is usable and long enough
// to start the probe, so a deadline the test creates and then reads must expire
// neither in between nor below the minimum.
func TestHermesToolsListContext_RespectsAnEarlierCallerDeadline(t *testing.T) {
	parent, cancelParent := context.WithTimeout(context.Background(), 900*time.Millisecond)
	defer cancelParent()

	probe, cancelProbe := hermesToolsListContext(parent)
	defer cancelProbe()

	deadline, ok := probe.Deadline()
	if !ok {
		t.Fatal("probe context has no deadline, want the caller's earlier deadline")
	}
	if remaining := time.Until(deadline); remaining > time.Second {
		t.Errorf("probe budget = %v, want the caller's earlier deadline instead of %v", remaining, hermesToolsListTimeout)
	}
}

// TestHermesToolsListContext_ExpiredCallerDeadlineGetsTheDefaultBudget locks
// R4-002: an already-expired caller deadline (or a clock jump) must not hand the
// probe a context that is dead on arrival. Such a probe fails for a reason that
// has nothing to do with the command, and the advisory would report it as
// "el comando falló", pointing the operator at a command that is fine.
func TestHermesToolsListContext_ExpiredCallerDeadlineGetsTheDefaultBudget(t *testing.T) {
	parent, cancelParent := context.WithDeadline(context.Background(), time.Now().Add(-time.Minute))
	defer cancelParent()

	probe, cancelProbe := hermesToolsListContext(parent)
	defer cancelProbe()

	if err := probe.Err(); err != nil {
		t.Fatalf("probe context error = %v, want nil: an expired caller deadline must not kill the probe", err)
	}
	deadline, ok := probe.Deadline()
	if !ok {
		t.Fatal("probe context has no deadline, want the 5s audit budget")
	}
	if remaining := time.Until(deadline); remaining > hermesToolsListTimeout || remaining < hermesToolsListTimeout-time.Second {
		t.Errorf("probe budget = %v, want ~%v", remaining, hermesToolsListTimeout)
	}
}

// TestAllowlistAdvisory_ExpiredCallerDeadlineStillAudits locks the
// operator-visible half of R4-002: the expired caller deadline falls through to
// the normal probe budget, so a cancelled flow still gets its audit evidence
// instead of a "command failed" category it cannot act on.
func TestAllowlistAdvisory_ExpiredCallerDeadlineStillAudits(t *testing.T) {
	expired, cancelExpired := context.WithDeadline(context.Background(), time.Now().Add(-time.Minute))
	defer cancelExpired()

	// Models RunHermesToolsList: the command runs under the PROBE context, so a
	// probe context that is dead on arrival fails exactly like a broken command
	// (the classification this test forbids).
	run := func(ctx context.Context, platform string) (string, error) {
		probe, cancelProbe := hermesToolsListContext(ctx)
		defer cancelProbe()
		if err := probe.Err(); err != nil {
			return "", fmt.Errorf("hermes tools list --platform %s: %w", platform, err)
		}
		return minimalHermesToolsList, nil
	}

	got := AllowlistAdvisory(expired, []string{"whatsapp"}, run)

	if !strings.Contains(got, expectedMinimalWhatsAppLine) {
		t.Errorf("advisory = %q, want the audited minimal line for whatsapp", got)
	}
	if strings.Contains(got, auditCommandFailed.description()) {
		t.Errorf("advisory = %q, must not report a command failure for an expired caller deadline", got)
	}
}

// TestHermesToolsListContext_KeepsCallerValues pins the "values survive
// detachment" half of the same comment, so a later refactor cannot quietly swap
// WithoutCancel for Background.
func TestHermesToolsListContext_KeepsCallerValues(t *testing.T) {
	type contextKey struct{}
	parent := context.WithValue(context.Background(), contextKey{}, "evidence")

	probe, cancelProbe := hermesToolsListContext(parent)
	defer cancelProbe()

	if got := probe.Value(contextKey{}); got != "evidence" {
		t.Errorf("probe value = %v, want the caller's value", got)
	}
}

// TestValidUTF8Prefix_HasBoundedCost locks the cost half of
// R3-unbounded-validutf8: the former loop re-validated the WHOLE prefix after
// every byte it dropped, so a string whose first invalid byte sits in the middle
// cost O(n²) — one full scan per dropped byte. The backwalk must stay bounded and
// the validation a single pass. The input is a valid head, one byte that can
// never form a rune, and a valid tail, which is the shape that maximises the old
// cost; size is well past the 8192-byte print cap so the difference between "one
// pass" and "one pass per byte dropped" cannot hide behind scheduling noise.
func TestValidUTF8Prefix_HasBoundedCost(t *testing.T) {
	const half = 1 << 20 // 2 MiB total, invalid byte in the middle
	garbage := strings.Repeat("a", half) + "\xff" + strings.Repeat("a", half)

	start := time.Now()
	got := validUTF8Prefix(garbage)
	elapsed := time.Since(start)

	if !utf8.ValidString(got) {
		t.Errorf("validUTF8Prefix(garbage) is not valid UTF-8: %q", got)
	}
	if elapsed > 5*time.Second {
		t.Errorf("validUTF8Prefix(%d bytes) took %v, want a bounded backwalk (≤5s) instead of one full re-scan per dropped byte", len(garbage), elapsed)
	}
}

// TestBoundedRuntimeReport_ShortReportWithCorruptedByteIsSanitized pins the
// second advisory round's R4-002: the size cut only decides WHETHER to bound the
// report, never whether it is safe to print. A report short enough to skip the
// cut used to be returned raw, pushing malformed bytes into the terminal.
func TestBoundedRuntimeReport_ShortReportWithCorruptedByteIsSanitized(t *testing.T) {
	report := "  ✓ enabled  skills\n  \xffcorrupted middle\n  ✓ enabled  memory"

	got := boundedRuntimeReport("whatsapp", report)
	if !utf8.ValidString(got) {
		t.Fatalf("short report came back with invalid UTF-8: %q", got)
	}
	if !strings.Contains(got, "✓ enabled  skills") {
		t.Errorf("valid head was dropped: %q", got)
	}
	if strings.Contains(got, "memory") {
		t.Errorf("tail after the corrupted byte leaked through: %q", got)
	}
	if !strings.Contains(got, "reporte truncado") {
		t.Errorf("processed report did not name the truncation: %q", got)
	}
}

// TestHermesToolsListContext_TooShortCallerDeadlineFallsBackToTheBudget pins the
// second advisory round's R4-001: a deadline that is alive but shorter than the
// cost of exec'ing the probe is the same trap as an expired one — the audit dies
// and the operator is told the command failed.
func TestHermesToolsListContext_TooShortCallerDeadlineFallsBackToTheBudget(t *testing.T) {
	parent, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	probe, cancelProbe := hermesToolsListContext(parent)
	defer cancelProbe()

	deadline, ok := probe.Deadline()
	if !ok {
		t.Fatal("probe context has no deadline")
	}
	if remaining := time.Until(deadline); remaining < hermesToolsListTimeout/2 {
		t.Fatalf("probe budget = %v, want the default %v budget instead of the too-short caller deadline", remaining, hermesToolsListTimeout)
	}
}

// TestChannelAllowlistIsMinimal_RealRuntimeReportReproducesTheFalseNegative pins
// the defect found on the production VM with v0.12.1: the parser split enabled
// names on the middle dot used only by this package's fixtures, but the runtime
// prints "<name>  <emoji> <description>", so every name carried its description
// and the conservative detector answered "not minimal" for a channel that was
// exactly minimal. A detector that always alarms is worse than no detector.
func TestChannelAllowlistIsMinimal_RealRuntimeReportReproducesTheFalseNegative(t *testing.T) {
	report := strings.Join([]string{
		"Built-in toolsets (whatsapp):",
		"  ✗ disabled  web  🔍 Web Search & Scraping",
		"  ✗ disabled  code_execution  ⚡ Code Execution",
		"  ✓ enabled  skills  📚 Skills",
		"  ✓ enabled  todo  📋 Task Planning",
		"  ✓ enabled  memory  💾 Memory",
		"  ✓ enabled  clarify  ❓ Clarifying Questions",
		"Plugin toolsets (whatsapp):",
		"  ✗ disabled  homeassistant  🔌 Homeassistant",
		"MCP servers:",
		"  mcp-appointments  all tools enabled",
		"",
	}, "\n")

	got := enabledChannelToolsets(report)
	want := []string{"skills", "todo", "memory", "clarify"}
	if len(got) != len(want) {
		t.Fatalf("enabledChannelToolsets() = %q (%d), want %q (%d)", got, len(got), want, len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("enabled[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	if !channelAllowlistIsMinimal(report) {
		t.Error("channelAllowlistIsMinimal() = false for a channel that IS minimal; the detector would alarm on every healthy install")
	}
}
