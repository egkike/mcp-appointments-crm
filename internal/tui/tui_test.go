package tui

import (
	"os"
	"path/filepath"
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
	// as a decision, not as three blank values. The summary stays neutral about
	// the file: the skip path never verified whether a MCP_TELEGRAM_* block was
	// already on disk, and a surviving block must not be reported as "not
	// configured" (B15 C3).
	if got, want := skipped.Summary(), "omitido (sin cambios en el bloque de Telegram)"; got != want {
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
// server phone first, then the Telegram token, the explicit removal question,
// the chat id and the owner phone. A reorder in the shared step builder must
// fail here instead of silently skipping the wrong question.
func TestHermesStepIndexesMatchTheSharedWizard(t *testing.T) {
	steps := hermesSteps(telegramPhone)

	want := map[int]string{
		hermesServerPhoneStep: hermesPhoneLabel,
		hermesTokenStep:       hermesTokenLabel,
		hermesRemoveStep:      hermesTelegramRemovalLabel,
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
// operator disables the optional channel: the token step is left empty, the
// explicit removal question is answered "no" (so a pre-existing block would be
// preserved), the two remaining Telegram questions are not asked, the
// confirmation states the disabled channel, and the written server entry carries
// no Telegram block. The counting assertions pin that the skipped group never
// shifts the "campo X de N" labels of the questions that are still asked.
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
	if !strings.Contains(firstView, "campo 1 de 5") {
		t.Errorf("first step = %q, want the counter of the declared fields", firstView)
	}
	if !strings.Contains(firstView, "opcional") {
		t.Errorf("first step = %q, want the notice that says the Telegram channel is optional", firstView)
	}

	d.press("enter") // accept the prefilled server phone
	if d.model.form.index != hermesTokenStep {
		t.Fatalf("step = %d, want the Telegram token field", d.model.form.index)
	}
	if tokenView := d.view(); !strings.Contains(tokenView, "campo 2 de 5") {
		t.Errorf("token step = %q, want the counter of the declared fields", tokenView)
	}

	d.press("enter") // empty token: the operator leaves the optional group

	if d.model.form.index != hermesRemoveStep {
		t.Fatalf("step = %d, want the explicit removal question", d.model.form.index)
	}
	removalView := d.view()
	if !strings.Contains(removalView, "Retirar el canal Telegram del bot") {
		t.Errorf("removal step = %q, want the explicit removal question", removalView)
	}
	if !strings.Contains(removalView, "campo 3 de 5") {
		t.Errorf("removal step = %q, want the counter of the declared fields", removalView)
	}

	d.press("enter") // empty answer keeps the channel state: no removal

	if d.model.screen != screenConfirm || d.model.confirm.action != confirmHermesConfig {
		t.Fatalf("screen = %v action = %v, want the Hermes confirmation",
			d.model.screen, d.model.confirm.action)
	}
	confirmView := d.view()
	if !strings.Contains(confirmView, "Telegram: omitido (sin cambios en el bloque de Telegram)") {
		t.Errorf("confirmation = %q, want the skip summary line", confirmView)
	}
	if strings.Contains(confirmView, "campo 3 de 5") {
		t.Errorf("confirmation = %q, the closed group must not ask a further Telegram question", confirmView)
	}

	d.press("s")

	if d.model.screen != screenInfo || d.model.infoKind != infoSuccess {
		t.Fatalf("screen = %v kind = %v, want the success info screen", d.model.screen, d.model.infoKind)
	}
	successView := d.view()
	if !strings.Contains(successView, "Telegram: omitido (sin cambios en el bloque de Telegram)") {
		t.Errorf("success = %q, want the skip summary line", successView)
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

// seedHermesConfigWithTelegramBlock writes a Hermes config that already carries
// the Telegram owner-bot block plus foreign keys, so the removal path is driven
// against the exact state a retiring business has on disk.
func seedHermesConfigWithTelegramBlock(t *testing.T, f fixture) string {
	t.Helper()
	seed := strings.Join([]string{
		"model: hermes-default",
		"mcp_servers:",
		"  openai:",
		"    url: https://api.example/mcp",
		"  " + admin.HermesServerName + ":",
		"    url: " + f.hermes.EndpointURL,
		"    headers:",
		`      X-Caller-Id: "` + ownerPhone + `"`,
		"    env:",
		`      MCP_TELEGRAM_BOT_TOKEN: "` + telegramToken + `"`,
		`      MCP_TELEGRAM_ALLOWED_CHAT_ID: "` + telegramChatID + `"`,
		`      MCP_TELEGRAM_OWNER_PHONE: "` + telegramPhone + `"`,
		"",
	}, "\n")
	if err := os.MkdirAll(filepath.Dir(f.hermesPath), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(f.hermesPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed hermes config: %v", err)
	}
	return seed
}

// TestAppModelHermesConfigRemovesTheTelegramBlock drives the THIRD path of the
// wizard (S11 unit B): the operator leaves the token empty, answers \"s\" to
// "Retirar el canal Telegram del bot", the confirmation states the removed block
// and the write deletes it while preserving the rest of the file.
func TestAppModelHermesConfigRemovesTheTelegramBlock(t *testing.T) {
	f := newFixture(t)
	f.seedOwner(t, ownerPhone, ownerName)
	// The write success cascades into the skill regeneration (ADR-0020 §4).
	f.deps.BusinessProfile = fakeBusinessProfile{name: "Barbería El Corte"}
	seedSkillTemplate(t, f)
	seedHermesConfigWithTelegramBlock(t, f)

	d := newDriver(t, f)
	d.press("7")

	d.press("enter") // accept the prefilled server phone
	d.press("enter") // empty token: the group closes

	if d.model.form.index != hermesRemoveStep {
		t.Fatalf("step = %d, want the explicit removal question", d.model.form.index)
	}
	d.typeText("s")
	d.press("enter")

	if d.model.screen != screenConfirm || d.model.confirm.action != confirmHermesConfig {
		t.Fatalf("screen = %v action = %v, want the Hermes confirmation",
			d.model.screen, d.model.confirm.action)
	}
	const removalSummary = "Telegram: canal deshabilitado (bloque MCP_TELEGRAM eliminado)"
	if confirmView := d.view(); !strings.Contains(confirmView, removalSummary) {
		t.Errorf("confirmation = %q, want the removal summary line", confirmView)
	}

	d.press("s")

	if d.model.screen != screenInfo || d.model.infoKind != infoSuccess {
		t.Fatalf("screen = %v kind = %v, want the success info screen", d.model.screen, d.model.infoKind)
	}
	if successView := d.view(); !strings.Contains(successView, removalSummary) {
		t.Errorf("success = %q, want the removal summary line", successView)
	}

	content := f.hermesFile(t)
	if strings.Contains(content, "MCP_TELEGRAM") {
		t.Errorf("removal left the Telegram block behind\n%s", content)
	}
	if strings.Contains(content, "env:") {
		t.Errorf("removal left an empty env block behind\n%s", content)
	}
	for _, want := range []string{
		"model: hermes-default",
		"openai",
		admin.HermesServerName,
		f.hermes.EndpointURL,
		`X-Caller-Id: "` + ownerPhone + `"`,
	} {
		if !strings.Contains(content, want) {
			t.Errorf("written Hermes config missing %q\n%s", want, content)
		}
	}
}
