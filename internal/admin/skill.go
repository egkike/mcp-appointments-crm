package admin

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/config"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// This file is the framework-free core of the receptionist-skill regeneration
// capability (ADR-0020, design.md §1/§4): it resolves the installed skill and
// repository template paths, renders the single install placeholder, detects
// whether the installed file matches a fresh rendering, and writes atomically.
// No Bubble Tea and no transport imports live here: the TUI screens (unit B)
// call these functions, which take explicit paths/bytes so a test never touches
// a real home.
//
// The skill is data at rest, not code: the repository template is the source of
// truth and regeneration only resolves {business_name}. The four SkillOutcome
// strings are the exact Spanish vocabulary scripts/install.sh reports, so the
// TUI and the installer speak one language.

const (
	// SkillSlug is the official Hermes install slug (the directory name under
	// ~/.hermes/skills/) and the repository template directory name.
	SkillSlug = "recepcionista-negocio"

	// SkillFileName is the fixed file name inside the skill directory.
	SkillFileName = "SKILL.md"

	// hermesSkillsDirName is the official Hermes skills directory under the
	// user home (design.md §1.1).
	hermesSkillsDirName = "skills"

	// skillTemplateDirName is the repository template subdirectory under the
	// operator config dir.
	skillTemplateDirName = "templates"

	// businessNamePlaceholder is the only install-time placeholder. {nombre}
	// is a runtime variable and is preserved verbatim.
	businessNamePlaceholder = "{business_name}"

	// skillDirMode and skillFileMode restrict the installed skill to the OS
	// user that owns the installation, matching the Hermes config artifacts.
	skillDirMode  = 0o700
	skillFileMode = 0o600
)

// SkillOutcome is the operator-facing result of installing or regenerating the
// receptionist skill. Its values are the exact Spanish strings
// scripts/install.sh reports in the install summary, so both surfaces agree.
type SkillOutcome string

const (
	// SkillOutcomeInstalled means the rendered skill was written.
	SkillOutcomeInstalled SkillOutcome = "instalada"

	// SkillOutcomeAlreadyInstalled means the installed bytes already equal the
	// rendering; nothing was written.
	SkillOutcomeAlreadyInstalled SkillOutcome = "ya instalada sin cambios"

	// SkillOutcomeModified means the installed file differs from the rendering
	// and the caller did not confirm replacing it; nothing was written.
	SkillOutcomeModified SkillOutcome = "modificada por el usuario, no se sobrescribió"

	// SkillOutcomeWriteError means the skill could not be written.
	SkillOutcomeWriteError SkillOutcome = "error: no se pudo escribir"
)

// SkillState is the detected relationship between the installed skill and a
// fresh rendering.
type SkillState int

const (
	// SkillStateMissing means no installed skill file exists.
	SkillStateMissing SkillState = iota

	// SkillStateUpToDate means the installed bytes equal the rendering.
	SkillStateUpToDate

	// SkillStateModified means the installed file exists but differs.
	SkillStateModified
)

// BusinessProfileReader is the read slice of repository.BusinessProfileRepo that
// the receptionist-skill regeneration needs to resolve the business name.
// *repository.BusinessProfileRepo satisfies it unchanged; the narrow port keeps
// this package testable without a database (mirrors AccountsReader).
type BusinessProfileReader interface {
	Get(ctx context.Context) (*entity.BusinessProfile, error)
}

// SkillFilePath returns the installed receptionist skill path:
// ~/.hermes/skills/recepcionista-negocio/SKILL.md. The caller resolves it once
// and passes the Hermes home explicitly into the detect/write functions, so
// tests never touch a real home.
func SkillFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		// Fail secure: without a home directory there is no safe place to read
		// or write the Hermes skill, and guessing one could edit an unrelated
		// file. The message never leaks a path.
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo resolver la skill de Hermes: no hay directorio home",
			Cause:   err,
		}
	}
	return filepath.Join(home, hermesDirName, hermesSkillsDirName, SkillSlug, SkillFileName), nil
}

// SkillTemplatePath returns the repository skill template path inside the
// operator config dir, resolved through the shared canonical table
// (config.ResolveConfigDir — install.sh CONFIG_DIR, incl. the Darwin
// Application.Support spelling):
//
//	<CONFIG_DIR>/templates/recepcionista-negocio/SKILL.md
//
// install.sh seeds that exact path on every install (the always-overwrite
// pristine copy), so the compiled binary and the installer can never disagree
// on where the template lives. Without a config dir the unit fails secure and
// the message never leaks a path.
func SkillTemplatePath() (string, error) {
	configDir, err := config.ResolveConfigDir()
	if err != nil {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo resolver el template de la skill: no hay directorio de configuración",
			Cause:   err,
		}
	}
	return filepath.Join(configDir, skillTemplateDirName, SkillSlug, SkillFileName), nil
}

// RenderSkillTemplate replaces EVERY {business_name} occurrence with
// businessName. The business name is mandatory: an empty name would install a
// skill that cannot identify the business. {nombre} is a runtime variable
// collected during registration and is preserved verbatim.
func RenderSkillTemplate(template, businessName string) (string, error) {
	if strings.TrimSpace(businessName) == "" {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el nombre del negocio es obligatorio para regenerar la skill",
		}
	}
	return strings.ReplaceAll(template, businessNamePlaceholder, businessName), nil
}

// skillPathFor returns the installed skill path under an explicit Hermes home.
// It is unexported because callers resolve the path once and inject it; tests
// use it to seed/read fixtures under t.TempDir().
func skillPathFor(hermesHome string) string {
	return filepath.Join(hermesHome, hermesSkillsDirName, SkillSlug, SkillFileName)
}

// DetectSkillState compares the installed skill under hermesHome with rendered.
// A missing file is SkillStateMissing (not an error); a present file with
// byte-identical content is SkillStateUpToDate; anything else is
// SkillStateModified. An unreadable file is a semantic failure with no path in
// its message.
func DetectSkillState(hermesHome, rendered string) (SkillState, error) {
	path := skillPathFor(hermesHome)
	// #nosec G304 -- path is the resolved Hermes skill path under an explicit
	// home (constant slug/filename), not attacker-controlled input.
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return SkillStateMissing, nil
		}
		return SkillStateMissing, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo leer la skill recepcionista instalada",
			Cause:   err,
		}
	}
	if bytes.Equal(data, []byte(rendered)) {
		return SkillStateUpToDate, nil
	}
	return SkillStateModified, nil
}

// WriteSkillFile writes rendered to
// <hermesHome>/skills/recepcionista-negocio/SKILL.md atomically: a temp file in
// the same directory followed by a rename, so a crash or a full disk never
// leaves a half-written skill. It uses the same temp-file+rename pattern as
// WriteHermesConfig.
//
// Mode discipline: the skill directory is created 0700 and the file 0600, and
// only when this flow creates them; a pre-existing directory is never chmod-ed.
// The final path is replaced by the rename, so no explicit chmod touches a file
// this call did not create.
func WriteSkillFile(hermesHome, rendered string) error {
	path := skillPathFor(hermesHome)
	dir := filepath.Dir(path)
	if err := ensureSkillDir(dir); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".hermes-skill-*.tmp")
	if err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo crear el archivo temporal de la skill recepcionista",
			Cause:   err,
		}
	}
	tmpPath := tmp.Name()

	// Any failure before the rename must remove the temp file: leaving it behind
	// would litter the skill directory with a partial copy.
	written := false
	defer func() {
		if !written {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if _, err := tmp.WriteString(rendered); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo escribir la skill recepcionista",
			Cause:   err,
		}
	}
	// os.CreateTemp already requests 0600, but an inherited umask could loosen
	// it; this file is ours, so tightening it here is safe and deterministic.
	if err := tmp.Chmod(skillFileMode); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudieron restringir los permisos de la skill recepcionista",
			Cause:   err,
		}
	}
	if err := tmp.Sync(); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo sincronizar la skill recepcionista",
			Cause:   err,
		}
	}
	if err := tmp.Close(); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo cerrar la skill recepcionista",
			Cause:   err,
		}
	}

	if err := os.Rename(tmpPath, path); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo reemplazar la skill recepcionista",
			Cause:   err,
		}
	}
	written = true
	return nil
}

// ensureSkillDir guarantees dir exists, creating the skill directory 0700 only
// when this call creates it. A pre-existing directory is left with its current
// mode, and a non-directory path is a semantic failure rather than a confusing
// write error (mirrors ensureHermesDir).
func ensureSkillDir(dir string) error {
	info, err := os.Stat(dir)
	switch {
	case err == nil:
		if !info.IsDir() {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInternal,
				Message: "la ruta de la skill recepcionista no es un directorio",
			}
		}
		return nil
	case !errors.Is(err, os.ErrNotExist):
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo preparar el directorio de la skill recepcionista",
			Cause:   err,
		}
	}

	if err := os.MkdirAll(dir, skillDirMode); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo crear el directorio de la skill recepcionista",
			Cause:   err,
		}
	}
	// MkdirAll passes the mode through the umask; enforce 0700 on the directory
	// this call created so the skill never lands in a group-readable directory.
	if err := os.Chmod(dir, skillDirMode); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudieron restringir los permisos del directorio de la skill recepcionista",
			Cause:   err,
		}
	}
	return nil
}

// RegenSkill renders templateBytes with businessName and reconciles the
// installed skill under hermesHome. It is the single entry point the TUI and
// console flows call; paths and template bytes are injected, so this function
// performs no home or config-dir lookup itself.
//
// Outcome matrix:
//   - missing skill                -> write, SkillOutcomeInstalled
//   - up-to-date skill             -> no write, SkillOutcomeAlreadyInstalled
//   - modified skill, unconfirmed  -> no write, SkillOutcomeModified
//   - modified skill, confirmed    -> write, SkillOutcomeInstalled
//
// A render or write failure returns SkillOutcomeWriteError plus the semantic
// error, so the caller can branch and still show the installer vocabulary.
func RegenSkill(hermesHome string, templateBytes []byte, businessName string, replacementConfirmed bool) (SkillOutcome, error) {
	rendered, err := RenderSkillTemplate(string(templateBytes), businessName)
	if err != nil {
		return SkillOutcomeWriteError, err
	}

	state, err := DetectSkillState(hermesHome, rendered)
	if err != nil {
		return SkillOutcomeWriteError, err
	}

	switch state {
	case SkillStateUpToDate:
		return SkillOutcomeAlreadyInstalled, nil
	case SkillStateModified:
		if !replacementConfirmed {
			return SkillOutcomeModified, nil
		}
	case SkillStateMissing:
	default:
		return SkillOutcomeWriteError, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "estado de la skill recepcionista desconocido",
		}
	}

	if err := WriteSkillFile(hermesHome, rendered); err != nil {
		return SkillOutcomeWriteError, err
	}
	return SkillOutcomeInstalled, nil
}
