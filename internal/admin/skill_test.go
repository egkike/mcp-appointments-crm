package admin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

const testBusinessName = "Barbería El Corte"

// seedSkillFile writes content at the installed skill path under an explicit
// Hermes home, creating the skill directory 0700, and returns the path.
func seedSkillFile(t *testing.T, hermesHome, content string) string {
	t.Helper()
	path := skillPathFor(hermesHome)
	if err := os.MkdirAll(filepath.Dir(path), skillDirMode); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), skillFileMode); err != nil {
		t.Fatalf("seed skill: %v", err)
	}
	return path
}

func TestSkillFilePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("HOME override does not drive os.UserHomeDir on windows")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("HERMES_HOME", "")

	got, err := SkillFilePath()
	if err != nil {
		t.Fatalf("SkillFilePath() error = %v", err)
	}
	want := filepath.Join(home, hermesDirName, hermesSkillsDirName, SkillSlug, SkillFileName)
	if got != want {
		t.Errorf("SkillFilePath() = %q, want %q", got, want)
	}
}

func TestSkillTemplatePath(t *testing.T) {
	got, err := SkillTemplatePath()
	if err != nil {
		t.Fatalf("SkillTemplatePath() error = %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("SkillTemplatePath() = %q, want an absolute path", got)
	}
	suffix := filepath.Join(defaultConfigDirName, skillTemplateDirName, SkillSlug, SkillFileName)
	if !strings.HasSuffix(got, suffix) {
		t.Errorf("SkillTemplatePath() = %q, want suffix %q", got, suffix)
	}
}

func TestRenderSkillTemplate_ReplacesEveryOccurrence(t *testing.T) {
	template := "A {business_name} B {business_name} {nombre} C"

	got, err := RenderSkillTemplate(template, testBusinessName)
	if err != nil {
		t.Fatalf("RenderSkillTemplate() error = %v", err)
	}
	want := "A " + testBusinessName + " B " + testBusinessName + " {nombre} C"
	if got != want {
		t.Errorf("RenderSkillTemplate() = %q, want %q", got, want)
	}
	if strings.Contains(got, businessNamePlaceholder) {
		t.Errorf("RenderSkillTemplate() left a placeholder: %q", got)
	}
	if !strings.Contains(got, "{nombre}") {
		t.Errorf("RenderSkillTemplate() dropped the runtime {nombre} variable: %q", got)
	}
}

func TestRenderSkillTemplate_RejectsEmptyName(t *testing.T) {
	for _, name := range []string{"", "   ", "\t\n"} {
		_, err := RenderSkillTemplate("hola {business_name}", name)
		if err == nil {
			t.Fatalf("RenderSkillTemplate(%q) error = nil, want a semantic failure", name)
		}
		var semErr *domain.SemanticError
		if !errors.As(err, &semErr) {
			t.Fatalf("RenderSkillTemplate(%q) error = %T, want *domain.SemanticError", name, err)
		}
		if semErr.Code != domain.ErrCodeInvalidInput {
			t.Errorf("RenderSkillTemplate(%q) code = %q, want %q", name, semErr.Code, domain.ErrCodeInvalidInput)
		}
		if !strings.Contains(semErr.Message, "obligatorio") {
			t.Errorf("RenderSkillTemplate(%q) message = %q, want it to state the name is required", name, semErr.Message)
		}
	}
}

func TestDetectSkillState(t *testing.T) {
	const rendered = "contenido renderizado"

	t.Run("missing", func(t *testing.T) {
		got, err := DetectSkillState(t.TempDir(), rendered)
		if err != nil {
			t.Fatalf("DetectSkillState() error = %v", err)
		}
		if got != SkillStateMissing {
			t.Errorf("DetectSkillState() = %v, want %v", got, SkillStateMissing)
		}
	})

	t.Run("up to date", func(t *testing.T) {
		hermesHome := t.TempDir()
		seedSkillFile(t, hermesHome, rendered)

		got, err := DetectSkillState(hermesHome, rendered)
		if err != nil {
			t.Fatalf("DetectSkillState() error = %v", err)
		}
		if got != SkillStateUpToDate {
			t.Errorf("DetectSkillState() = %v, want %v", got, SkillStateUpToDate)
		}
	})

	t.Run("modified", func(t *testing.T) {
		hermesHome := t.TempDir()
		seedSkillFile(t, hermesHome, "contenido distinto")

		got, err := DetectSkillState(hermesHome, rendered)
		if err != nil {
			t.Fatalf("DetectSkillState() error = %v", err)
		}
		if got != SkillStateModified {
			t.Errorf("DetectSkillState() = %v, want %v", got, SkillStateModified)
		}
	})
}

func TestWriteSkillFile_CreatesRestrictedDirAndFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows reports synthesized 0666/0777 modes; POSIX bits are not asserted")
	}
	hermesHome := t.TempDir()
	const rendered = "contenido de la skill"

	if err := WriteSkillFile(hermesHome, rendered); err != nil {
		t.Fatalf("WriteSkillFile() error = %v", err)
	}

	path := skillPathFor(hermesHome)
	dirInfo, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != skillDirMode {
		t.Errorf("skill dir mode = %04o, want %04o", got, skillDirMode)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != skillFileMode {
		t.Errorf("skill file mode = %04o, want %04o", got, skillFileMode)
	}
	if got := readHermesFile(t, path); string(got) != rendered {
		t.Errorf("skill file = %q, want %q", got, rendered)
	}
}

func TestWriteSkillFile_LeavesNoTemporaryFileBehind(t *testing.T) {
	hermesHome := t.TempDir()
	if err := WriteSkillFile(hermesHome, "contenido"); err != nil {
		t.Fatalf("WriteSkillFile() error = %v", err)
	}

	entries, err := os.ReadDir(filepath.Dir(skillPathFor(hermesHome)))
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", entry.Name())
		}
	}
}

func TestWriteSkillFile_ExistingFilePreservedOnFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not enforced on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory write permissions")
	}
	hermesHome := t.TempDir()
	path := seedSkillFile(t, hermesHome, "original")
	dir := filepath.Dir(path)
	// #nosec G302 -- test-only: 0o500 is tighter than 0600; it forces the write-failure path.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, skillDirMode) })

	err := WriteSkillFile(hermesHome, "nuevo")
	if err == nil {
		t.Fatal("WriteSkillFile() error = nil, want a failure")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("WriteSkillFile() error = %T, want *domain.SemanticError", err)
	}
	if got := readHermesFile(t, path); string(got) != "original" {
		t.Errorf("existing skill = %q, want it preserved as %q", got, "original")
	}

	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatalf("ReadDir: %v", readErr)
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), ".tmp") {
			t.Errorf("temporary file left behind: %s", entry.Name())
		}
	}
}

func TestRegenSkill_MissingWritesAndReportsInstalled(t *testing.T) {
	hermesHome := t.TempDir()

	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name} y {business_name}"), testBusinessName, false, nil)
	if err != nil {
		t.Fatalf("RegenSkill() error = %v", err)
	}
	if outcome != SkillOutcomeInstalled {
		t.Errorf("outcome = %q, want %q", outcome, SkillOutcomeInstalled)
	}
	want := "Hola " + testBusinessName + " y " + testBusinessName
	if got := readHermesFile(t, skillPathFor(hermesHome)); string(got) != want {
		t.Errorf("installed skill = %q, want %q", got, want)
	}
}

func TestRegenSkill_UpToDateDoesNotWrite(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs enforced POSIX directory permissions")
	}
	hermesHome := t.TempDir()
	rendered := "Hola " + testBusinessName
	path := seedSkillFile(t, hermesHome, rendered)
	// A read-only directory proves nothing was written: any write attempt would
	// fail, so a nil error plus the up-to-date outcome pins the no-write path.
	dir := filepath.Dir(path)
	// #nosec G302 -- test-only: 0o500 is tighter than 0600; it forces the write-failure path.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, skillDirMode) })

	snapshot, snapshotErr := InstalledSkillSnapshot(hermesHome)
	if snapshotErr != nil {
		t.Fatalf("snapshot: %v", snapshotErr)
	}
	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name}"), testBusinessName, true, snapshot)
	if err != nil {
		t.Fatalf("RegenSkill() error = %v", err)
	}
	if outcome != SkillOutcomeAlreadyInstalled {
		t.Errorf("outcome = %q, want %q", outcome, SkillOutcomeAlreadyInstalled)
	}
}

func TestRegenSkill_ModifiedUnconfirmedIsPreserved(t *testing.T) {
	hermesHome := t.TempDir()
	path := seedSkillFile(t, hermesHome, "editado a mano")

	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name}"), testBusinessName, false, nil)
	if err != nil {
		t.Fatalf("RegenSkill() error = %v", err)
	}
	if outcome != SkillOutcomeModified {
		t.Errorf("outcome = %q, want %q", outcome, SkillOutcomeModified)
	}
	if got := readHermesFile(t, path); string(got) != "editado a mano" {
		t.Errorf("modified skill = %q, want it preserved", got)
	}
}

func TestRegenSkill_ModifiedConfirmedIsReplaced(t *testing.T) {
	hermesHome := t.TempDir()
	path := seedSkillFile(t, hermesHome, "editado a mano")

	snapshot, snapshotErr := InstalledSkillSnapshot(hermesHome)
	if snapshotErr != nil {
		t.Fatalf("failed to snapshot: %v", snapshotErr)
	}
	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name}"), testBusinessName, true, snapshot)
	if err != nil {
		t.Fatalf("RegenSkill() error = %v", err)
	}
	if outcome != SkillOutcomeInstalled {
		t.Errorf("outcome = %q, want %q", outcome, SkillOutcomeInstalled)
	}
	want := "Hola " + testBusinessName
	if got := readHermesFile(t, path); string(got) != want {
		t.Errorf("replaced skill = %q, want %q", got, want)
	}
}

func TestRegenSkill_WriteFailureReportsErrorOutcome(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("needs enforced POSIX directory permissions")
	}
	hermesHome := t.TempDir()
	skillsRoot := filepath.Join(hermesHome, hermesSkillsDirName)
	if err := os.MkdirAll(skillsRoot, skillDirMode); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Chmod(skillsRoot, 0o500); err != nil { // #nosec G302 -- test-only failure forcing
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(skillsRoot, skillDirMode) })

	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name}"), testBusinessName, false, nil)
	if err == nil {
		t.Fatal("RegenSkill() error = nil, want a write failure")
	}
	if outcome != SkillOutcomeWriteError {
		t.Errorf("outcome = %q, want %q", outcome, SkillOutcomeWriteError)
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("RegenSkill() error = %T, want *domain.SemanticError", err)
	}
}

func TestRegenSkill_EmptyNameIsSemanticError(t *testing.T) {
	hermesHome := t.TempDir()

	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name}"), "   ", false, nil)
	if err == nil {
		t.Fatal("RegenSkill() error = nil, want a validation failure")
	}
	if outcome != SkillOutcomeInvalidInput {
		t.Errorf("outcome = %q, want %q (input rejection is structurally distinct from a write failure)", outcome, SkillOutcomeInvalidInput)
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("RegenSkill() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("RegenSkill() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
}

func TestRenderSkillTemplate_TrimsSurroundingSpace(t *testing.T) {
	got, err := RenderSkillTemplate("A {business_name} B", "  Acme  ")
	if err != nil {
		t.Fatalf("RenderSkillTemplate() error = %v", err)
	}
	if got != "A Acme B" {
		t.Errorf("RenderSkillTemplate() = %q, want %q", got, "A Acme B")
	}
}

func TestRenderSkillTemplate_BoundsName(t *testing.T) {
	// Boundary: exactly entity.MaxBusinessNameLen runes still renders.
	name := strings.Repeat("á", entity.MaxBusinessNameLen)
	if _, err := RenderSkillTemplate("hola {business_name}", name); err != nil {
		t.Fatalf("RenderSkillTemplate(at limit) error = %v, want nil", err)
	}
	// One rune past the cap is a Spanish semantic input error.
	_, err := RenderSkillTemplate("hola {business_name}", name+"x")
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) || semErr.Code != domain.ErrCodeInvalidInput {
		t.Fatalf("RenderSkillTemplate(overlong) err = %v, want INVALID_INPUT semantic error", err)
	}
	if !strings.Contains(semErr.Message, "supera la longitud") {
		t.Errorf("message = %q, want it to name the length cap", semErr.Message)
	}
	// Control characters are rejected at the domain gate, but the skill render
	// must never carry them either (reuse of the domain rule).
	_, err = RenderSkillTemplate("hola {business_name}", "acme\ninyección")
	var semErr2 *domain.SemanticError
	if !errors.As(err, &semErr2) || semErr2.Code != domain.ErrCodeInvalidInput {
		t.Fatalf("RenderSkillTemplate(control char) err = %v, want INVALID_INPUT semantic error", err)
	}
}

func TestRegenSkill_InvalidInputOutcome(t *testing.T) {
	hermesHome := t.TempDir()
	outcome, err := RegenSkill(hermesHome, []byte("hola {business_name}"), "  \n\t ", false, nil)
	if err == nil {
		t.Fatal("RegenSkill(blank-ish name) error = nil, want input rejection")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("RegenSkill(blank-ish name) err = %T, want *domain.SemanticError", err)
	}
	if outcome != SkillOutcomeInvalidInput {
		t.Errorf("outcome = %q, want %q (input rejection, not a write failure)", outcome, SkillOutcomeInvalidInput)
	}
	if _, statErr := os.Stat(skillPathFor(hermesHome)); !os.IsNotExist(statErr) {
		t.Errorf("installed file must not exist after an input rejection; statErr = %v", statErr)
	}
}

func TestHermesDir_OverrideIsTheHermesDirectoryItself(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("env resolution mirrors install.sh POSIX semantics")
	}
	dir := filepath.Join(t.TempDir(), "hermes-home")
	t.Setenv("HERMES_HOME", dir)
	t.Setenv("HOME", t.TempDir())

	got, err := HermesDir()
	if err != nil {
		t.Fatalf("HermesDir() error = %v", err)
	}
	if got != dir {
		t.Errorf("HermesDir() = %q, want %q", got, dir)
	}
	// The contract scripts/tests/install_skill_test.sh pins: the config and the
	// skill live directly under HERMES_HOME, never under HERMES_HOME/.hermes.
	wantCfg := filepath.Join(dir, HermesConfigFileName)
	cfg, err := HermesConfigPath()
	if err != nil {
		t.Fatalf("HermesConfigPath() error = %v", err)
	}
	if cfg != wantCfg {
		t.Errorf("HermesConfigPath() = %q, want %q", cfg, wantCfg)
	}
	wantSkill := filepath.Join(dir, hermesSkillsDirName, SkillSlug, SkillFileName)
	skl, err := SkillFilePath()
	if err != nil {
		t.Fatalf("SkillFilePath() error = %v", err)
	}
	if skl != wantSkill {
		t.Errorf("SkillFilePath() = %q, want %q", skl, wantSkill)
	}
}

func TestDetectSkillState_RejectsBlankHome(t *testing.T) {
	state, err := DetectSkillState("   ", "rendered")
	if err == nil {
		t.Fatal("DetectSkillState(blank home) error = nil, want rejection")
	}
	if state != SkillStateMissing {
		t.Errorf("state = %v, want the zero value without a usable file", state)
	}
}

func TestWriteSkillFile_RejectsBlankHome(t *testing.T) {
	err := WriteSkillFile("", "rendered")
	if err == nil {
		t.Fatal("WriteSkillFile(blank home) error = nil, want rejection")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("WriteSkillFile(blank home) err = %T, want *domain.SemanticError", err)
	}
}

func TestRenderSkillTemplate_RejectsMarkdownActiveChars(t *testing.T) {
	// GGA round 8: newline/tab rejection alone still let inline markdown into
	// the SKILL.md; the render barrier reuses the domain charset rule.
	for _, name := range []string{"Acme ** Sucursal", "Co`digo", "Ti[tular", "{business_name}"} {
		_, err := RenderSkillTemplate("hola {business_name}", name)
		var semErr *domain.SemanticError
		if !errors.As(err, &semErr) || semErr.Code != domain.ErrCodeInvalidInput {
			t.Fatalf("RenderSkillTemplate(%q) err = %v, want INVALID_INPUT semantic error", name, err)
		}
	}
}

// TestGuardSkillSwap_FileReplacedMidFlowAborts pins the B9 guard: an external
// rename-replace between the fd open and the final replacement aborts the
// consent flow without overwriting the external edit (issue #137 R-lost-update
// blind spot; the compare-by-fd alone cannot see a new inode).
func TestGuardSkillSwap_FileReplacedMidFlowAborts(t *testing.T) {
	hermesHome := t.TempDir()
	path := skillPathFor(hermesHome)
	if err := ensureSkillDir(filepath.Dir(path)); err != nil {
		t.Fatalf("ensure skill dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("INSTALLED-A"), hermesFileMode); err != nil {
		t.Fatalf("seed installed skill: %v", err)
	}

	// #nosec G304 -- constant-slug path under the fixture Hermes home.
	f, err := os.OpenFile(path, os.O_RDWR, hermesFileMode)
	if err != nil {
		t.Fatalf("open the installed skill fd: %v", err)
	}
	defer func() { _ = f.Close() }()

	// The external writer replaces the file at the PATH (rename swap): a new
	// inode with new bytes behind a fd that still refs the old one.
	if err := os.WriteFile(filepath.Join(hermesHome, ".external-swap"), []byte("EXTERNAL-B"), hermesFileMode); err != nil {
		t.Fatalf("external writer temp: %v", err)
	}
	if err := os.Rename(filepath.Join(hermesHome, ".external-swap"), path); err != nil {
		t.Fatalf("external rename swap: %v", err)
	}

	err = guardSkillSwap(f, path)
	if err == nil {
		t.Fatal("guardSkillSwap = nil, want the external-replacement semantic abort")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("err = %v, want a domain.SemanticError", err)
	}
	if !strings.Contains(semErr.Message, "reemplazada") {
		t.Errorf("message = %q, want the replaced-by-external-writer wording", semErr.Message)
	}

	// The external edit survives: the guard must abort WITHOUT the rename.
	if b, rerr := os.ReadFile(path); rerr != nil || string(b) != "EXTERNAL-B" { // #nosec G304 -- fixture path
		t.Errorf("path bytes = %q (%v), want EXTERNAL-B preserved", b, rerr)
	}
}

// TestGuardSkillSwap_SameInodePasses keeps the happy path: the fd and the path
// refer to the same inode on filesystems WITH inode semantics.
func TestGuardSkillSwap_SameInodePasses(t *testing.T) {
	hermesHome := t.TempDir()
	path := skillPathFor(hermesHome)
	if err := ensureSkillDir(filepath.Dir(path)); err != nil {
		t.Fatalf("ensure skill dir: %v", err)
	}
	if err := os.WriteFile(path, []byte("INSTALLED-A"), hermesFileMode); err != nil {
		t.Fatalf("seed installed skill: %v", err)
	}

	// #nosec G304 -- constant-slug path under the fixture Hermes home.
	f, err := os.OpenFile(path, os.O_RDWR, hermesFileMode)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = f.Close() }()

	if err := guardSkillSwap(f, path); err != nil {
		t.Fatalf("guardSkillSwap on the same inode = %v, want nil", err)
	}
}
