package tui

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/charmbracelet/bubbletea"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// This file is the receptionist-skill regeneration step of the "Configurar
// Hermes" capability (ADR-0020, design.md §4): after the operator confirms the
// Hermes config write, the TUI detects the relationship between the installed
// skill and a fresh rendering and either re-seeds it, asks for an explicit
// confirmation (a modified skill is user content), or reports that nothing
// changed. The mechanics live in internal/admin (RegenSkill); this file only
// resolves the inputs, sequences the two screens and renders one outcome line
// with the same Spanish vocabulary scripts/install.sh reports.

// skillInputs carries the three facts a regeneration decision needs, resolved
// once per decision so neither handler derives them again: the Hermes home,
// the stored business name and the pristine template bytes.
type skillInputs struct {
	hermesHome   string
	businessName string
	template     []byte
	snapshot     []byte
}

// resolveSkillInputs composes a regeneration's inputs. The Hermes skill home
// is derived from deps.Hermes.Path (the config file the composition root
// resolved) instead of a fresh home lookup, so the skill always lives next to
// the config the flow manages — and the flow stays injectable for tests. Errors
// are Spanish semantic failures: a missing profile name, an unresolvable Hermes
// path or an absent template mean the operator cannot finish the regeneration
// from here.
func resolveSkillInputs(ctx context.Context, deps Deps) (skillInputs, error) {
	if deps.Hermes.Path == "" {
		return skillInputs{}, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo resolver el directorio de skills de Hermes",
		}
	}
	hermesHome := filepath.Dir(deps.Hermes.Path)
	if deps.BusinessProfile == nil {
		return skillInputs{}, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no hay acceso al perfil del negocio para regenerar la skill",
		}
	}
	profile, err := deps.BusinessProfile.Get(ctx)
	if err != nil {
		return skillInputs{}, err
	}
	template, err := admin.LoadSkillTemplate()
	if err != nil {
		return skillInputs{}, err
	}
	return skillInputs{
		hermesHome:   hermesHome,
		businessName: profile.Name,
		template:     template,
	}, nil
}

// skillStateMsg lands the detection of the installed skill's relationship to a
// fresh rendering: the decision input for the follow-up (re-seed, confirm, or
// nothing).
type skillStateMsg struct {
	inputs skillInputs
	state  admin.SkillState
	err    error
}

// detectSkillCmd finds the installed skill state for one brand-new rendering.
func detectSkillCmd(ctx context.Context, deps Deps) tea.Cmd {
	return func() tea.Msg {
		inputs, err := resolveSkillInputs(ctx, deps)
		if err != nil {
			return skillStateMsg{err: err}
		}
		rendered, err := admin.RenderSkillTemplate(string(inputs.template), inputs.businessName)
		if err != nil {
			return skillStateMsg{inputs: inputs, err: err}
		}
		state, detectErr := admin.DetectSkillState(inputs.hermesHome, rendered)
		if detectErr == nil && state == admin.SkillStateModified {
			// GGA rounds 17/18: capture the bytes BEFORE consent and BEFORE the
			// follow-up replacement; the replacement is valid only against THIS
			// snapshot, and a snapshot read failure aborts instead of implying
			// silent consent.
			snapshot, snapErr := admin.InstalledSkillSnapshot(inputs.hermesHome)
			if snapErr != nil {
				return skillStateMsg{inputs: inputs, err: snapErr}
			}
			inputs.snapshot = snapshot
		}
		return skillStateMsg{inputs: inputs, state: state, err: detectErr}
	}
}

// skillResultMsg lands the regeneration outcome: the SkillOutcome summary token
// plus, on failure, the semantic error the operator must read (the outcome is
// never a substitute for the error text).
type skillResultMsg struct {
	outcome admin.SkillOutcome
	err     error
}

// regenSkillCmd applies one regeneration decision. replacementConfirmed is
// false for the walkup cases (missing/up-to-date) and true only after the
// operator confirmed replacing a user-modified skill on the confirm screen.
func regenSkillCmd(inputs skillInputs, replacementConfirmed bool) tea.Cmd {
	return func() tea.Msg {
		outcome, err := admin.RegenSkill(inputs.hermesHome, inputs.template, inputs.businessName, replacementConfirmed, inputs.snapshot)
		return skillResultMsg{outcome: outcome, err: err}
	}
}

// resultLines renders the final operator-facing lines of the whole option 7
// flow: the config facts the flow already showed plus the skill outcome and the
// loader-reload note Hermes needs. Shared by TTY and console so the two
// presentations speak one language.
func resultLines(outcome admin.SkillOutcome) []string {
	return []string{
		"Skill recepcionista: " + string(outcome),
		"Hermes recarga el índice de skills en su próximo arranque.",
	}
}

// skillOutcomeErrorLine is the operator-facing prefix for a failed step: the
// semantic error is always the message; the outcome only anchors the summary.
func skillOutcomeErrorLine(err error) string {
	return fmt.Sprintf("Error: %s", err)
}

// RegenerateSkillForConsole drives the same ADR-0020 §4 cascade for the TTY-less
// console flow, with one behavioral knob: replacement is only applied after the
// supplied confirm hook returns true (the console maps it to its y/n prompt; the
// injected indirection keeps this function free of I/O for tests).
func RegenerateSkillForConsole(ctx context.Context, hermes HermesConfig, profile admin.BusinessProfileReader, confirmReplacement func() (bool, error)) (admin.SkillOutcome, error) {
	inputs, err := resolveSkillInputs(ctx, Deps{Hermes: HermesBootstrap{HermesConfig: hermes}, BusinessProfile: profile})
	if err != nil {
		return admin.SkillOutcomeWriteError, err
	}
	rendered, err := admin.RenderSkillTemplate(string(inputs.template), inputs.businessName)
	if err != nil {
		return admin.SkillOutcomeInvalidInput, err
	}
	state, err := admin.DetectSkillState(inputs.hermesHome, rendered)
	if err != nil {
		return admin.SkillOutcomeWriteError, err
	}
	confirmed := false
	var snapshot []byte
	if state == admin.SkillStateModified {
		// GGA round 20: capture the snapshot BEFORE the confirmation prompt —
		// the operator consents to the bytes shown at detection time; edits
		// made while the prompt is open are NOT part of the consent baseline
		// and will abort the replacement in WriteSkillFileConsent.
		snap, snapErr := admin.InstalledSkillSnapshot(inputs.hermesHome)
		if snapErr != nil {
			// Missing evidence never grants replacement (GGA round 18).
			return admin.SkillOutcomeModified, snapErr
		}
		snapshot = snap
		// An interrupted prompt (EOF/stdin failure) is an error, not a decline:
		// the caller must see the interruption, never an ordinary "modificada".
		confirmed, err = confirmReplacement()
		if err != nil {
			return admin.SkillOutcomeModified, err
		}
	}
	return admin.RegenSkill(inputs.hermesHome, inputs.template, inputs.businessName, confirmed, snapshot)
}
