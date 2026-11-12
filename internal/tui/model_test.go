package tui

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

// The option-7 success screen must warn about the channel allowlist in BOTH
// presentations (issue #195). The console prints it via writeAllowlistAdvisory;
// the Bubble Tea success screen used to skip it entirely, so `mcp-server admin
// tui` — the primary interface — never showed the advisory on the release that
// shipped it (v0.12.0).
//
// These tests drive the model the way the program does: the skill result lands,
// the advisory arrives LATER as its own message. Every probe goes through the
// ToolsListRunner seam and no test executes a real `hermes` binary.

const (
	testHermesEndpoint = "http://127.0.0.1:3000/sse"
	testHermesPhone    = "+5491100000000"
	testHermesHome     = "hermes-home"
	// testSkillHeading is the first line of the success screen, before and after
	// the advisory: the config write already succeeded.
	testSkillHeading = "Configuración de Hermes actualizada."
)

// advisoryModel builds the state the cascade leaves behind: a passed gate, the
// resolved Hermes bootstrap the success screen renders, and the probe seam.
func advisoryModel(run ToolsListRunner) AppModel {
	m := NewAppModel(context.Background(), Deps{
		AllowlistRunner: run,
		Hermes: HermesBootstrap{HermesConfig: HermesConfig{
			Path:        filepath.Join(testHermesHome, admin.HermesConfigFileName),
			EndpointURL: testHermesEndpoint,
		}},
	})
	m.width, m.height = 120, 30
	m.viewport.Width = m.contentWidth()
	m.viewport.Height = tableHeight(m.height)
	m.gatePassed = true
	m.hermesPhone = testHermesPhone
	return m
}

// landSkillResult feeds one skill result and then whatever the model dispatched,
// returning both frames: the screen the operator sees immediately and the screen
// after the advisory arrived. The two-frame contract is the point — the probe is
// a child process and must never run inside Update.
func landSkillResult(t *testing.T, m AppModel, msg skillResultMsg) (immediate, withAdvisory AppModel) {
	t.Helper()

	next, cmd := m.Update(msg)
	immediate, ok := next.(AppModel)
	if !ok {
		t.Fatalf("Update(skillResultMsg) returned %T, want AppModel", next)
	}
	if cmd == nil {
		t.Fatal("Update(skillResultMsg) dispatched no command, want the allowlist probe")
	}

	produced := cmd()
	advisory, ok := produced.(allowlistMsg)
	if !ok {
		t.Fatalf("the dispatched command produced %T, want allowlistMsg", produced)
	}

	next, _ = immediate.Update(advisory)
	withAdvisory, ok = next.(AppModel)
	if !ok {
		t.Fatalf("Update(allowlistMsg) returned %T, want AppModel", next)
	}
	return immediate, withAdvisory
}

// TestSkillSuccessScreenShowsTheMinimalAllowlistAdvisory locks the happy path:
// a channel already narrowed reports the one-line ✓ state, in the same words the
// console prints, and the probe does not delay the first frame.
func TestSkillSuccessScreenShowsTheMinimalAllowlistAdvisory(t *testing.T) {
	var asked []string
	m := advisoryModel(fakeToolsList(minimalHermesToolsList, nil, &asked))

	immediate, final := landSkillResult(t, m, skillResultMsg{outcome: admin.SkillOutcomeInstalled})

	if got := immediate.View(); strings.Contains(got, "Allowlist del canal") {
		t.Errorf("first frame = %q, want the config facts only: the probe must never run inline", got)
	}

	got := final.View()
	for _, want := range []string{
		testSkillHeading,
		"Allowlist del canal whatsapp: mínima ✓ (skills, todo, memory, clarify + mcp-appointments)",
		"Allowlist del canal telegram: mínima ✓ (skills, todo, memory, clarify + mcp-appointments)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("success screen = %q, want it to contain %q", got, want)
		}
	}

	if final.infoKind != infoSuccess {
		t.Errorf("infoKind = %v, want infoSuccess", final.infoKind)
	}
	if want := DefaultAllowlistPlatforms(); !slices.Equal(asked, want) {
		t.Errorf("audited platforms = %v, want %v", asked, want)
	}
}

// TestSkillSuccessScreenShowsTheDisableCommand locks the dangerous case: the
// operator reads the runtime's own state and the exact command that fixes it,
// which is the only actionable output the advisory has.
func TestSkillSuccessScreenShowsTheDisableCommand(t *testing.T) {
	m := advisoryModel(fakeToolsList(dangerousHermesToolsList, nil, nil))

	_, final := landSkillResult(t, m, skillResultMsg{outcome: admin.SkillOutcomeInstalled})

	got := final.View()
	for _, want := range []string{
		testSkillHeading,
		"Allowlist del canal whatsapp: no puedo confirmar que sea la mínima ⚠",
		"Aplicala con:",
		expectedWhatsAppDisableCommand,
		"Detalle y verificación: " + installationDocPointer,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("success screen = %q, want it to contain %q", got, want)
		}
	}

	if final.infoKind != infoSuccess {
		t.Errorf("infoKind = %v, want infoSuccess", final.infoKind)
	}
}

// TestSkillSuccessScreenDegradesToTheGenericAdvisoryWhenTheProbeFails locks the
// non-fatal contract: a probe that cannot answer never turns the successful
// config write into an error screen. The facts stay, the generic risk notice is
// appended, and the raw failure is not printed (issue #157 sanitization).
func TestSkillSuccessScreenDegradesToTheGenericAdvisoryWhenTheProbeFails(t *testing.T) {
	probeErr := errors.New("hermes tools list --platform whatsapp: exit status 2: /home/operator/.hermes")
	m := advisoryModel(fakeToolsList("", probeErr, nil))

	_, final := landSkillResult(t, m, skillResultMsg{outcome: admin.SkillOutcomeInstalled})

	got := final.View()
	for _, want := range []string{
		testSkillHeading,
		"Endpoint: " + testHermesEndpoint,
		"Teléfono (X-Caller-Id): " + testHermesPhone,
		"Telegram: ",
		"Archivo: " + filepath.Join(testHermesHome, admin.HermesConfigFileName),
		"Allowlist del canal: no se pudo auditar (el comando `hermes tools list` falló)",
		installationDocPointer,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("success screen = %q, want it to contain %q", got, want)
		}
	}

	if final.infoKind != infoSuccess {
		t.Errorf("infoKind = %v, want infoSuccess: a failed probe is not an error", final.infoKind)
	}
	if strings.Contains(got, "exit status 2") || strings.Contains(got, "/home/operator") {
		t.Errorf("success screen = %q, want no runtime detail from the probe", got)
	}
	if strings.Contains(got, "Allowlist del canal whatsapp") {
		t.Errorf("success screen = %q, must not claim a per-channel state it could not audit", got)
	}
}

// TestSkillCascadeFailureAlsoGetsTheAdvisory locks the second console call site:
// the advisory is due on BOTH skill outcomes, and a failed cascade keeps its own
// severity instead of being repainted as a success.
func TestSkillCascadeFailureAlsoGetsTheAdvisory(t *testing.T) {
	m := advisoryModel(fakeToolsList(minimalHermesToolsList, nil, nil))

	_, final := landSkillResult(t, m, skillResultMsg{err: errors.New("no se pudo escribir la skill")})

	got := final.View()
	for _, want := range []string{
		testSkillHeading,
		"Error: no se pudo escribir la skill",
		"Allowlist del canal whatsapp: mínima ✓",
		"Allowlist del canal telegram: mínima ✓",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("failure screen = %q, want it to contain %q", got, want)
		}
	}

	if final.infoKind != infoError {
		t.Errorf("infoKind = %v, want the cascade failure severity preserved", final.infoKind)
	}
}

// TestAdvisoryDoesNotHijackAnotherScreen locks the late-arrival guard: the probe
// can take seconds, and an operator who already left the success screen must not
// have the advisory painted onto whatever screen is open when it answers.
func TestAdvisoryDoesNotHijackAnotherScreen(t *testing.T) {
	m := advisoryModel(fakeToolsList(minimalHermesToolsList, nil, nil))

	next, cmd := m.Update(skillResultMsg{outcome: admin.SkillOutcomeInstalled})
	info, ok := next.(AppModel)
	if !ok {
		t.Fatalf("Update(skillResultMsg) returned %T, want AppModel", next)
	}

	// The operator presses Enter while the probe is still running.
	left, _ := info.Update(testKey("enter"))
	menu, ok := left.(AppModel)
	if !ok {
		t.Fatalf("Update(enter) returned %T, want AppModel", left)
	}
	if menu.screen != screenMenu {
		t.Fatalf("screen after Enter = %v, want the menu", menu.screen)
	}

	produced := cmd()
	after, _ := menu.Update(produced)
	final, ok := after.(AppModel)
	if !ok {
		t.Fatalf("Update(allowlistMsg) returned %T, want AppModel", after)
	}

	if final.screen != screenMenu {
		t.Errorf("screen after the late advisory = %v, want the menu", final.screen)
	}
	if got := final.View(); strings.Contains(got, "Allowlist del canal") {
		t.Errorf("menu view = %q, want no advisory for a screen it does not belong to", got)
	}
}
