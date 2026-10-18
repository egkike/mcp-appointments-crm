package admin

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

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

	// SkillOutcomeInvalidInput means the regeneration inputs were rejected
	// before any state detection or write (GGA advisory A2/S5: structural
	// instead of documentary — install.sh never emits this token, so its own
	// vocabulary is untouched).
	SkillOutcomeInvalidInput SkillOutcome = "datos inválidos para la skill"
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
	dir, err := HermesDir()
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
	return filepath.Join(dir, hermesSkillsDirName, SkillSlug, SkillFileName), nil
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
// businessName. The business name is mandatory and length-bounded (GGA advisory
// A1: the name is re-rendered from persisted state on every regeneration, so an
// oversized value from a hand-edited row must not break the SKILL.md structure):
// blank or over-length values are Spanish semantic input errors. {nombre} is a
// runtime variable collected during registration and is preserved verbatim.
func RenderSkillTemplate(template, businessName string) (string, error) {
	trimmed := strings.TrimSpace(businessName)
	if trimmed == "" {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el nombre del negocio es obligatorio para regenerar la skill",
		}
	}
	if utf8.RuneCountInString(trimmed) > entity.MaxBusinessNameLen {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el nombre del negocio supera la longitud permitida para regenerar la skill",
		}
	}
	// Defense in depth, second barrier of the domain rules (entity.Validate):
	// the name is interpolated into the SKILL.md the Hermes agent loads, so a
	// stored multi-line value must not become a markdown/instruction injection,
	// and a stored name with markdown-active characters could still alter the
	// generated skill (GGA round 8).
	if strings.ContainsAny(trimmed, "\r\n\t") {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el nombre del negocio no puede contener saltos de línea ni tabulaciones.",
		}
	}
	if entity.BusinessNameHasUnsafeChars(trimmed) {
		return "", &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "el nombre del negocio no puede contener caracteres especiales de marcado.",
		}
	}
	return strings.ReplaceAll(template, businessNamePlaceholder, trimmed), nil
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
	if strings.TrimSpace(hermesHome) == "" {
		return SkillStateMissing, &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "no se indicó dónde está instalada la skill de Hermes",
		}
	}
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
	if strings.TrimSpace(hermesHome) == "" {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInvalidInput,
			Message: "no se indicó dónde instalar la skill de Hermes",
		}
	}
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
// ensureSkillDir creates the skill directory atomically with its final
// mode (skillDirMode): os.Mkdir applies the umask, which can only REMOVE
// permission bits, so the created directory can never be more permissive
// than 0700 and no follow-up chmod is needed — any post-Mkdir chmod, path-
// or fd-based, races with a swap of the newly created path (GGA rounds 4/5).
// A pre-existing path must be a directory and is never chmod-ed.
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
	if err := os.MkdirAll(filepath.Dir(dir), skillDirMode); err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo crear el directorio de la skill recepcionista",
			Cause:   err,
		}
	}
	if err := os.Mkdir(dir, skillDirMode); err != nil {
		// EEXIST: another creator won the race — validate it and never chmod.
		info, statErr := os.Stat(dir)
		if statErr != nil {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInternal,
				Message: "no se pudo verificar el directorio de la skill recepcionista",
				Cause:   statErr,
			}
		}
		if !info.IsDir() {
			return &domain.SemanticError{
				Code:    domain.ErrCodeInternal,
				Message: "la ruta de la skill recepcionista no es un directorio",
			}
		}
		return nil
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
// A render/validation failure returns SkillOutcomeInvalidInput (plus the
// semantic error); a write failure returns SkillOutcomeWriteError. The TUI and
// console always surface the error's Spanish message on failure — the outcome
// is the summary line, never a substitute for the error text.

// rewriteViaTemp writes rendered bytes atomically next to the target (temp
// file in the same dir, fsync, chmod, rename). It never touches the original
// file until the rename succeeds, so a failed write or sync cannot leave a
// half-written skill.
// WriteSkillFileConsent rewrites the installed skill under a verified consent
// snapshot (GGA rounds 18-20). The snapshot bytes are read from an open fd
// (not from the path), compared against consentSnapshot under an exclusive
// advisory lock, and only then is the rendered content swapped in via the
// same-directory temp+rename: a concurrent edit keeps its bytes (the
// comparison fails and the call aborts without replacing), a concurrent lock
// holder makes the call fail closed, and a failed write or sync cannot leave
// the original half-written. A missing file is an error: consent is only
// granted for an EXISTING modified skill.
func WriteSkillFileConsent(hermesHome string, rendered string, consentSnapshot []byte) error {
	path := skillPathFor(hermesHome)
	// #nosec G304 -- constant-slug path under the resolved Hermes home; no
	// attacker-controlled component. O_EXCL fails when the file vanished
	// between the consent and this call.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_EXCL, hermesFileMode)
	if err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo abrir la skill instalada para verificar el consentimiento",
			Cause:   err,
		}
	}
	defer func() { _ = f.Close() }()

	// Advisory lock: fail closed when another writer holds the file.
	if lerr := lockFile(f); lerr != nil {
		return lerr
	}
	defer unlockFile(f)

	current, rerr := io.ReadAll(f)
	if rerr != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo leer la skill instalada para verificar el consentimiento",
			Cause:   rerr,
		}
	}
	if !bytes.Equal(current, consentSnapshot) {
		// NOT an error: the skill changed again after the consent. The outcome
		// flows as "modified" so the caller re-runs the detect-confirm cycle
		// with the new bytes as the consent baseline.
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "la skill cambió después del consentimiento; corré la regeneración de nuevo",
		}
	}
	// GGA round 21: re-read while STILL holding the lock, so an edit that
	// landed between the first read and the temp+rename aborts instead of
	// being silently overwritten. The single-writer design plus this second
	// read keep the check-to-rename window closed under the advisory lock; on
	// Windows the lock is a designed no-op (skill_lock_windows).
	if _, serr := f.Seek(0, 0); serr != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo reposicionar la skill instalada antes del reemplazo",
			Cause:   serr,
		}
	}
	recheck, rerr2 := io.ReadAll(f)
	if rerr2 != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo releer la skill instalada antes del reemplazo",
			Cause:   rerr2,
		}
	}
	if !bytes.Equal(recheck, consentSnapshot) || !bytes.Equal(recheck, current) {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "la skill cambió durante la verificación; corré la regeneración de nuevo",
		}
	}
	// B9 inode guard (issue #137): a non-cooperating writer that REPLACED the
	// file by rename carries its bytes in a different inode — the fd still
	// refs the old one, so the byte compare above passes and the final rename
	// would silently discard the external edit. Re-stat the PATH and fail
	// closed on divergence (same "run regeneration again" contract the
	// changed-after-consent path uses). Filesystems without inode semantics
	// (both zeros) skip the guard instead of failing closed on stat.
	if err := guardSkillSwap(f, path); err != nil {
		return err
	}
	return rewriteViaTemp(filepath.Dir(path), filepath.Base(path), rendered)
}

func rewriteViaTemp(dir, base, rendered string) error {
	tmp, err := os.CreateTemp(dir, ".skill-replace-*.tmp")
	if err != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo crear el archivo temporal de la skill",
			Cause:   err,
		}
	}
	tmpPath := tmp.Name()
	written := false
	defer func() {
		if !written {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()
	if _, werr := tmp.WriteString(rendered); werr != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo escribir la skill instalada",
			Cause:   werr,
		}
	}
	if serr := tmp.Sync(); serr != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo sincronizar la skill instalada",
			Cause:   serr,
		}
	}
	if cerr := tmp.Close(); cerr != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo cerrar la skill instalada",
			Cause:   cerr,
		}
	}
	// Mode is pinned explicitly: os.CreateTemp never asks for 0600 through the
	// fsync path.
	if cerr2 := os.Chmod(tmpPath, hermesFileMode); cerr2 != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudieron restringir los permisos de la skill instalada",
			Cause:   cerr2,
		}
	}
	if rerr := os.Rename(tmpPath, filepath.Join(dir, base)); rerr != nil {
		return &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo reemplazar la skill instalada",
			Cause:   rerr,
		}
	}
	written = true
	return nil
}

func RegenSkill(hermesHome string, templateBytes []byte, businessName string, replacementConfirmed bool, consentSnapshot []byte) (SkillOutcome, error) {
	rendered, err := RenderSkillTemplate(string(templateBytes), businessName)
	if err != nil {
		return SkillOutcomeInvalidInput, err
	}

	state, err := DetectSkillState(hermesHome, rendered)
	if err != nil {
		return SkillOutcomeWriteError, err
	}

	switch state {
	case SkillStateUpToDate:
		return SkillOutcomeAlreadyInstalled, nil
	case SkillStateMissing:
		if err := WriteSkillFile(hermesHome, rendered); err != nil {
			return SkillOutcomeWriteError, err
		}
		return SkillOutcomeInstalled, nil
	case SkillStateModified:
		if !replacementConfirmed {
			return SkillOutcomeModified, nil
		}
		// GGA rounds 17/18: a replacement consented at detection time must
		// still find the SAME bytes. A missing snapshot is missing evidence,
		// never permission — fail closed (no write, operator re-runs the
		// cycle). An edit AFTER the confirmation re-opens the decision via
		// WriteSkillFileConsent's byte comparison.
		if consentSnapshot == nil {
			return SkillOutcomeWriteError, &domain.SemanticError{
				Code:    domain.ErrCodeInvalidInput,
				Message: "no se pudo verificar la versión de la skill instalada; no se reemplazó",
			}
		}
	default:
		return SkillOutcomeWriteError, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "estado de la skill recepcionista desconocido",
		}
	}

	// GGA round 18: the consented replacement runs ONLY through the snapshot-
	// verified, locked writer; a consenting-but-changed skill re-runs from the
	// "modified" outcome. The maybe-MISSING skill (state Missing) keeps the
	// plain atomic create path — there is no prior user content to protect.
	if werr := WriteSkillFileConsent(hermesHome, rendered, consentSnapshot); werr != nil {
		if werr.Error() == "la skill cambió después del consentimiento; corré la regeneración de nuevo" {
			return SkillOutcomeModified, werr
		}
		return SkillOutcomeWriteError, werr
	}
	return SkillOutcomeInstalled, nil
}

// LoadSkillTemplate reads the pristine skill template installed under the
// operator config dir (scripts/install.sh seed_config_template writes it on
// every install and always overwrites). A missing file is a Spanish semantic
// failure: the skill template cannot be invented at runtime (ADR-0020/design
// §1.1 — the repository template is the single source of truth). A present
// template is returned verbatim; the renderer only substitutes {business_name}.
func LoadSkillTemplate() ([]byte, error) {
	path, err := SkillTemplatePath()
	if err != nil {
		return nil, err
	}
	// #nosec G304 -- resolved constant-slug path under the config dir, not
	// attacker-controlled input.
	data, err := os.ReadFile(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se encontró el template de la skill recepcionista; corré el instalador (install.sh) de nuevo",
		}
	case err != nil:
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "no se pudo leer el template de la skill recepcionista",
			Cause:   err,
		}
	}
	return data, nil
}

// InstalledSkillSnapshot exposes the installed skill bytes for the consent
// recheck: the caller stores them when a confirm prompt renders and passes
// them back to RegenSkill, so an edit between consent and write re-opens the
// decision instead of being overwritten (GGA rounds 17/18). A read failure is
// an ERROR, never silent consent: the caller must surface it or ask again —
// missing evidence never means "replacement allowed" (GGA round 18).
func InstalledSkillSnapshot(hermesHome string) ([]byte, error) {
	path := skillPathFor(hermesHome)
	// #nosec G304 -- constant-slug path under the resolved Hermes home; NO
	// attacker-controlled component.
	return os.ReadFile(path)
}
