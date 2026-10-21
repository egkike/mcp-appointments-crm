package tui

import (
	"strconv"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
)

// ── S11: Telegram as an OPTIONAL channel (owner decision 2026-10-19) ──────
//
// The owner decided "skip + deshabilitar": the Telegram owner-bot group of the
// option 7 flow is optional, an empty group writes the server entry WITHOUT the
// MCP_TELEGRAM_* block, and the summary states the channel is disabled. The
// required-pair semantics of a HALF-FILLED group stay exactly as they were.

// TestTelegramFieldsSkipSemantics pins the skip path of the field group: the
// empty group validates with NO error (it is a skip, not a rejection) and
// reports the disabled channel instead of an empty token/chat/phone line.
func TestTelegramFieldsSkipSemantics(t *testing.T) {
	skipped := TelegramFields{}

	if !skipped.Disabled() {
		t.Error("Disabled() = false, want true for the empty group")
	}
	if err := skipped.Validate(); err != nil {
		t.Fatalf("Validate() error = %v, want nil (an empty group skips the channel)", err)
	}
	// The exact operator copy both presentations render: an empty group must read
	// as a decision, not as three blank values.
	if got, want := skipped.Summary(), "no configurado (canal deshabilitado)"; got != want {
		t.Errorf("Summary() = %q, want %q", got, want)
	}
}

// TestTelegramFieldsPartialGroupKeepsTheRequiredPair locks the semantics that
// predate S11: the token, the chat id and the owner phone were collected as a
// required pair, so a group with only ONE of the pair still fails with the
// message of the missing field. Only "nothing at all" is a skip.
func TestTelegramFieldsPartialGroupKeepsTheRequiredPair(t *testing.T) {
	cases := []struct {
		name     string
		fields   TelegramFields
		wantCopy string
	}{
		{
			name:     "token without chat id",
			fields:   NewTelegramFields(telegramToken, "", telegramPhone),
			wantCopy: "chat id",
		},
		{
			name:     "chat id without token",
			fields:   NewTelegramFields("", telegramChatID, telegramPhone),
			wantCopy: "token del bot de Telegram",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.fields.Disabled() {
				t.Error("Disabled() = true, want false for an incomplete pair")
			}
			err := tc.fields.Validate()
			if err == nil {
				t.Fatalf("Validate() error = nil, want the required-pair rejection naming %q", tc.wantCopy)
			}
			if !strings.Contains(err.Error(), tc.wantCopy) {
				t.Errorf("Validate() error = %q, want it to name %q", err.Error(), tc.wantCopy)
			}
		})
	}
}

// TestHermesStepIndexesMatchTheSharedWizard pins the step indexes the skip
// semantics reason about against the wizard the operator actually walks: the
// server phone first, then the Telegram token, the chat id and the owner phone.
// A reorder in the shared step builder must fail here instead of silently
// skipping the wrong question.
func TestHermesStepIndexesMatchTheSharedWizard(t *testing.T) {
	steps := hermesSteps(telegramPhone)

	want := map[int]string{
		hermesServerPhoneStep: hermesPhoneLabel,
		hermesTokenStep:       hermesTokenLabel,
		hermesChatIDStep:      hermesChatIDLabel,
		hermesOwnerPhoneStep:  hermesOwnerPhoneLabel,
	}
	if len(steps) != len(want) {
		t.Fatalf("hermesSteps() = %d steps, want %d", len(steps), len(want))
	}
	for index, label := range want {
		if got := steps[index].label; got != label {
			t.Errorf("steps[%d].label = %q, want %q", index, got, label)
		}
	}
}

// TestHermesStepLabelsCountTheDeclaredFields pins the "campo X de N" progress
// label of the wizard that now carries an optional group: N is derived from the
// declared steps (never hardcoded), so a skipped group can never leave the
// operator reading a step number that does not belong to the question on screen.
func TestHermesStepLabelsCountTheDeclaredFields(t *testing.T) {
	steps := hermesSteps(telegramPhone)
	form := newForm(hermesTitle, steps)
	total := strconv.Itoa(len(steps))

	for index := range steps {
		form.index = index
		got := steps[index].label + form.stepCounter()
		want := steps[index].label + " (campo " + strconv.Itoa(index+1) + " de " + total + ")"
		if got != want {
			t.Errorf("step %d label = %q, want %q", index, got, want)
		}
	}
}

// TestAppModelHermesConfigSkipsTheTelegramGroup drives the wizard the way an
// operator disables the optional channel: the token step is left empty, so the
// two remaining Telegram questions are not asked, the confirmation states the
// disabled channel, and the written server entry carries no Telegram block.
// The counting assertions pin that the skipped group never shifts the "campo X
// de N" labels of the questions that are still asked.
func TestAppModelHermesConfigSkipsTheTelegramGroup(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)
	// The write success cascades into the skill regeneration (ADR-0020 §4): the
	// cascade inputs (profile name + template) must exist before the driver
	// copies deps.
	f.deps.BusinessProfile = fakeBusinessProfile{name: "Barbería El Corte"}
	seedSkillTemplate(t, f)

	d := newDriver(t, f)
	d.press("7")

	if d.model.screen != screenHermesForm {
		t.Fatalf("screen = %v, want the Hermes wizard", d.model.screen)
	}
	firstView := d.view()
	if !strings.Contains(firstView, "campo 1 de 4") {
		t.Errorf("first step = %q, want the counter of the declared fields", firstView)
	}
	if !strings.Contains(firstView, "opcional") {
		t.Errorf("first step = %q, want the notice that says the Telegram channel is optional", firstView)
	}

	d.press("enter") // accept the prefilled server phone
	if d.model.form.index != hermesTokenStep {
		t.Fatalf("step = %d, want the Telegram token field", d.model.form.index)
	}
	if tokenView := d.view(); !strings.Contains(tokenView, "campo 2 de 4") {
		t.Errorf("token step = %q, want the counter of the declared fields", tokenView)
	}

	d.press("enter") // empty token: the operator disables the channel

	if d.model.screen != screenConfirm || d.model.confirm.action != confirmHermesConfig {
		t.Fatalf("screen = %v action = %v, want the Hermes confirmation",
			d.model.screen, d.model.confirm.action)
	}
	confirmView := d.view()
	if !strings.Contains(confirmView, "Telegram: no configurado (canal deshabilitado)") {
		t.Errorf("confirmation = %q, want the disabled-channel summary line", confirmView)
	}
	if strings.Contains(confirmView, "campo 3 de 4") {
		t.Errorf("confirmation = %q, the skipped group must not ask a further Telegram question", confirmView)
	}

	d.press("s")

	if d.model.screen != screenInfo || d.model.infoKind != infoSuccess {
		t.Fatalf("screen = %v kind = %v, want the success info screen", d.model.screen, d.model.infoKind)
	}
	successView := d.view()
	if !strings.Contains(successView, "Telegram: no configurado (canal deshabilitado)") {
		t.Errorf("success = %q, want the disabled-channel summary line", successView)
	}

	content := f.hermesFile(t)
	for _, want := range []string{
		admin.HermesServerName,
		f.hermes.EndpointURL,
		`X-Caller-Id: "` + ownerPhone + `"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("written Hermes config missing %q\n%s", want, content)
		}
	}
	if strings.Contains(content, "MCP_TELEGRAM") {
		t.Errorf("skipped Telegram group wrote an env block\n%s", content)
	}
}
