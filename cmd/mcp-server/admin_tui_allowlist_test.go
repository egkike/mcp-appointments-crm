package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/tui"
)

// This file pins the console wiring of the option-7 allowlist advisory (issue
// #195): what the operator reads after the config write, and the guarantee that
// a failed audit never breaks a flow whose write already succeeded. Every case
// injects a fake runner through the allowlistAudit seam; no test executes a real
// hermes binary.

// narrowToolsList is the runtime report of a channel already at the minimal
// allowlist.
const narrowToolsList = `  ✓ enabled  skills · todo · memory · clarify
  MCP servers:
    mcp-appointments  all tools enabled
`

// fullAccessToolsList is the default messaging bundle (full access).
const fullAccessToolsList = `  ✓ enabled  skills · todo · memory · clarify · terminal · file
  MCP servers:
    mcp-appointments  all tools enabled
`

// auditFor builds the injectable seam from a fake runner. The audited channels
// are the production defaults, so the table also pins which platforms the
// advisory probes.
func auditFor(run toolsListRunner) allowlistAudit {
	return allowlistAudit{runToolsList: run, platforms: defaultAllowlistAudit.platforms}
}

// TestWriteAllowlistAdvisory covers the three operator-visible states of the
// advisory through the seam.
func TestWriteAllowlistAdvisory(t *testing.T) {
	tests := []struct {
		name        string
		run         toolsListRunner
		wantExact   string
		wantContain []string
		wantOmit    []string
	}{
		{
			name: "minimal allowlist prints one line per documented channel",
			run: func(_ context.Context, _ string) (string, error) {
				return narrowToolsList, nil
			},
			wantExact: "\n" +
				"Allowlist del canal whatsapp: mínima ✓ (skills, todo, memory, clarify + mcp-appointments)\n" +
				"Allowlist del canal telegram: mínima ✓ (skills, todo, memory, clarify + mcp-appointments)\n",
		},
		{
			name: "full access bundle prints the block and the exact command",
			run: func(_ context.Context, platform string) (string, error) {
				if platform == "whatsapp" {
					return fullAccessToolsList, nil
				}
				return narrowToolsList, nil
			},
			wantContain: []string{
				"Allowlist del canal whatsapp:",
				strings.TrimRight(fullAccessToolsList, "\n"),
				"mcp-appointments, skills, clarify, memory, todo",
				"hermes tools disable --platform whatsapp code_execution terminal file browser web computer_use delegation cronjob image_gen tts vision session_search connections homeassistant",
				"Allowlist del canal telegram: mínima ✓",
			},
			wantOmit: []string{"Allowlist del canal whatsapp: mínima ✓"},
		},
		{
			name: "hermes absent falls back to the generic advisory",
			run: func(_ context.Context, _ string) (string, error) {
				return "", errors.New("exec: \"hermes\": executable file not found in $PATH")
			},
			wantContain: []string{"no se pudo auditar", `docs/installation.md → "Allowlist mínima recomendada"`},
			wantOmit:    []string{"mínima ✓", "hermes tools disable", "executable file not found"},
		},
		{
			name: "command failure falls back to the generic advisory",
			run: func(_ context.Context, _ string) (string, error) {
				return "boom: /home/kike/.hermes/config.yaml is corrupt", errors.New("exit status 1")
			},
			wantContain: []string{"no se pudo auditar"},
			wantOmit:    []string{"mínima ✓", "hermes tools disable", "config.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := writeAllowlistAdvisory(context.Background(), &out, auditFor(tt.run)); err != nil {
				t.Fatalf("writeAllowlistAdvisory() error = %v, want nil", err)
			}

			got := out.String()
			if tt.wantExact != "" && got != tt.wantExact {
				t.Fatalf("output = %q, want %q", got, tt.wantExact)
			}
			for _, want := range tt.wantContain {
				if !strings.Contains(got, want) {
					t.Errorf("output = %q, want it to contain %q", got, want)
				}
			}
			for _, omit := range tt.wantOmit {
				if strings.Contains(got, omit) {
					t.Errorf("output = %q, must not contain %q", got, omit)
				}
			}
		})
	}
}

// TestRunConfigureHermesFlowAudit_PrintsTheAdvisoryLast locks the placement the
// brief requires: the advisory is the last thing the operator reads, after the
// config write and after the ADR-0020 skill cascade.
func TestRunConfigureHermesFlowAudit_PrintsTheAdvisoryLast(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")
	hermesPath := prepareHermesConsole(t)
	identity := consoleIdentity(t, dbPath)

	var out bytes.Buffer
	// Configure Hermes -> prefilled phone -> token -> chat id -> prefilled
	// Telegram phone -> confirm. Trailing newlines keep an unexpected extra
	// prompt from failing the run for the wrong reason.
	stdin := newConsoleInput(strings.NewReader(
		"\n" + telegramTestToken + "\n" + telegramTestChatID + "\n\ns\n\n\n"))

	audit := auditFor(func(_ context.Context, _ string) (string, error) {
		return narrowToolsList, nil
	})
	err := runConfigureHermesFlowAudit(context.Background(), identity, stdin, &out,
		tui.HermesConfig{Path: hermesPath, EndpointURL: consoleHermesEndpoint(t)}, audit)
	if err != nil {
		t.Fatalf("runConfigureHermesFlowAudit() error = %v, want the flow to finish", err)
	}

	got := out.String()
	written := strings.Index(got, "Configuración de Hermes actualizada.")
	if written < 0 {
		t.Fatalf("output = %q, want the config confirmation", got)
	}
	wantTail := "\n" +
		"Allowlist del canal whatsapp: mínima ✓ (skills, todo, memory, clarify + mcp-appointments)\n" +
		"Allowlist del canal telegram: mínima ✓ (skills, todo, memory, clarify + mcp-appointments)\n"
	if !strings.HasSuffix(got, wantTail) {
		t.Errorf("output = %q, want it to end with %q", got, wantTail)
	}
	if idx := strings.Index(got, "Allowlist del canal whatsapp:"); idx < written {
		t.Errorf("advisory at %d, want it after the config confirmation at %d", idx, written)
	}
}

// TestRunConfigureHermesFlowAudit_AdvisoryFailureKeepsTheFlow locks the contract
// the whole advisory is subordinate to: a failed audit (hermes absent, command
// broken, unknown channel) never turns a successful config write into an error.
func TestRunConfigureHermesFlowAudit_AdvisoryFailureKeepsTheFlow(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")
	hermesPath := prepareHermesConsole(t)
	identity := consoleIdentity(t, dbPath)

	var out bytes.Buffer
	stdin := newConsoleInput(strings.NewReader(
		"\n" + telegramTestToken + "\n" + telegramTestChatID + "\n\ns\n\n\n"))

	audit := auditFor(func(_ context.Context, _ string) (string, error) {
		return "", errors.New("exec: hermes: not found")
	})
	err := runConfigureHermesFlowAudit(context.Background(), identity, stdin, &out,
		tui.HermesConfig{Path: hermesPath, EndpointURL: consoleHermesEndpoint(t)}, audit)
	if err != nil {
		t.Fatalf("runConfigureHermesFlowAudit() error = %v, want nil despite the failed audit", err)
	}

	got := out.String()
	if !strings.Contains(got, "Configuración de Hermes actualizada.") {
		t.Errorf("output = %q, want the config confirmation to survive", got)
	}
	if !strings.Contains(got, "no se pudo auditar") {
		t.Errorf("output = %q, want the generic advisory", got)
	}
	// The failed audit is not an error the operator must act on: after the
	// advisory line there is no menu "Error: ..." (the skill-cascade error that
	// may precede it belongs to a different step and is asserted elsewhere).
	if idx := strings.Index(got, "no se pudo auditar"); idx >= 0 && strings.Contains(got[idx:], "Error: ") {
		t.Errorf("output = %q, want no error line after the advisory", got)
	}
}

// TestWriteAllowlistAdvisory_ConsoleFailureIsNotAnError locks R4-003 at the seam:
// the advisory is the last, non-essential step of a flow whose config write
// already succeeded, so a console it cannot write to answers nil instead of
// surfacing an error the operator cannot act on. This case carries no fallback
// console, so it also pins the nil-fallback path (the retry on stderr is locked by
// TestWriteAllowlistAdvisory_RetriesOnTheFallbackConsole).
func TestWriteAllowlistAdvisory_ConsoleFailureIsNotAnError(t *testing.T) {
	audit := auditFor(func(_ context.Context, _ string) (string, error) {
		return narrowToolsList, nil
	})

	if err := writeAllowlistAdvisory(context.Background(), failingConsole{}, audit); err != nil {
		t.Fatalf("writeAllowlistAdvisory() error = %v, want nil despite the console failure", err)
	}
}

// failingConsole is an io.Writer that always refuses to write, standing in for a
// closed terminal: the advisory must swallow that failure.
type failingConsole struct{}

func (failingConsole) Write(_ []byte) (int, error) {
	return 0, errors.New("console closed")
}

// advisoryRefusingConsole succeeds on every write of the flow except the
// advisory, so the test exercises the real option-7 flow and fails only at the
// step R4-003 is about.
type advisoryRefusingConsole struct {
	inner bytes.Buffer
}

func (w *advisoryRefusingConsole) Write(p []byte) (int, error) {
	if strings.Contains(string(p), "Allowlist del canal") {
		return 0, errors.New("console closed")
	}
	return w.inner.Write(p)
}

// TestRunConfigureHermesFlowAudit_AdvisoryWriteFailureKeepsTheFlow locks R4-003
// end to end: the config was already written when the advisory prints, so a
// console failure there must not turn a successful write into a flow error.
func TestRunConfigureHermesFlowAudit_AdvisoryWriteFailureKeepsTheFlow(t *testing.T) {
	dbPath, _ := prepareAdminTUI(t)
	seedOwnerForTest(t, dbPath, ownerPhone, "Dueño")
	hermesPath := prepareHermesConsole(t)
	identity := consoleIdentity(t, dbPath)

	out := &advisoryRefusingConsole{}
	stdin := newConsoleInput(strings.NewReader(
		"\n" + telegramTestToken + "\n" + telegramTestChatID + "\n\ns\n\n\n"))

	audit := auditFor(func(_ context.Context, _ string) (string, error) {
		return narrowToolsList, nil
	})
	err := runConfigureHermesFlowAudit(context.Background(), identity, stdin, out,
		tui.HermesConfig{Path: hermesPath, EndpointURL: consoleHermesEndpoint(t)}, audit)
	if err != nil {
		t.Fatalf("runConfigureHermesFlowAudit() error = %v, want nil despite the unwritable advisory", err)
	}

	got := out.inner.String()
	if !strings.Contains(got, "Configuración de Hermes actualizada.") {
		t.Errorf("output = %q, want the config confirmation to survive", got)
	}
	if strings.Contains(got, "Allowlist del canal") {
		t.Errorf("output = %q, want the advisory write to have failed silently", got)
	}
}

// TestDefaultAllowlistAudit_UsesTheRealRunner keeps the production wiring honest:
// the default seam is the real Hermes CLI runner and the audited platforms are
// the two channels this product documents.
func TestDefaultAllowlistAudit_UsesTheRealRunner(t *testing.T) {
	if defaultAllowlistAudit.runToolsList == nil {
		t.Fatal("defaultAllowlistAudit.runToolsList = nil, want the production runner")
	}
	if len(defaultAllowlistAudit.platforms) != 2 ||
		defaultAllowlistAudit.platforms[0] != "whatsapp" ||
		defaultAllowlistAudit.platforms[1] != "telegram" {
		t.Errorf("audited platforms = %v, want [whatsapp telegram]", defaultAllowlistAudit.platforms)
	}
	// R4-002: the production audit must carry a fallback console, or a refused
	// stdout write would drop the security notice with nobody learning it.
	if defaultAllowlistAudit.fallback == nil {
		t.Error("defaultAllowlistAudit.fallback = nil, want stderr as the advisory's fallback console")
	}
}

// TestWriteAllowlistAdvisory_RetriesOnTheFallbackConsole locks R4-002: the
// advisory is a security notice, so a console that refuses the write must not
// make it vanish without a trace. The same text is retried on the fallback
// console (stderr in production), and the flow still answers nil.
func TestWriteAllowlistAdvisory_RetriesOnTheFallbackConsole(t *testing.T) {
	var fallback bytes.Buffer
	audit := auditFor(func(_ context.Context, _ string) (string, error) {
		return narrowToolsList, nil
	})
	audit.fallback = &fallback

	if err := writeAllowlistAdvisory(context.Background(), failingConsole{}, audit); err != nil {
		t.Fatalf("writeAllowlistAdvisory() error = %v, want nil despite the console failure", err)
	}

	got := fallback.String()
	for _, want := range []string{
		"Allowlist del canal whatsapp: mínima ✓",
		"Allowlist del canal telegram: mínima ✓",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("fallback output = %q, want the advisory retried with %q", got, want)
		}
	}
}

// TestWriteAllowlistAdvisory_BothConsolesFailKeepsTheFlowNonFailing closes the
// loop: when the fallback fails too there is no operator action left, so the step
// stays non-failing instead of inventing an error the flow cannot use.
func TestWriteAllowlistAdvisory_BothConsolesFailKeepsTheFlowNonFailing(t *testing.T) {
	audit := auditFor(func(_ context.Context, _ string) (string, error) {
		return narrowToolsList, nil
	})
	audit.fallback = failingConsole{}

	if err := writeAllowlistAdvisory(context.Background(), failingConsole{}, audit); err != nil {
		t.Fatalf("writeAllowlistAdvisory() error = %v, want nil when both consoles fail", err)
	}
}

// TestWriteAllowlistAdvisory_PrimarySuccessLeavesTheFallbackSilent keeps the
// retry from becoming a duplicate print on a healthy console.
func TestWriteAllowlistAdvisory_PrimarySuccessLeavesTheFallbackSilent(t *testing.T) {
	var out, fallback bytes.Buffer
	audit := auditFor(func(_ context.Context, _ string) (string, error) {
		return narrowToolsList, nil
	})
	audit.fallback = &fallback

	if err := writeAllowlistAdvisory(context.Background(), &out, audit); err != nil {
		t.Fatalf("writeAllowlistAdvisory() error = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "Allowlist del canal whatsapp") {
		t.Errorf("output = %q, want the advisory on the primary console", out.String())
	}
	if fallback.Len() != 0 {
		t.Errorf("fallback output = %q, want it untouched when the primary console worked", fallback.String())
	}
}
