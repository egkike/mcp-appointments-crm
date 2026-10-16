package admin

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
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

	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name} y {business_name}"), testBusinessName, false)
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
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, skillDirMode) })

	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name}"), testBusinessName, true)
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

	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name}"), testBusinessName, false)
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

	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name}"), testBusinessName, true)
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
	if err := os.Chmod(skillsRoot, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(skillsRoot, skillDirMode) })

	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name}"), testBusinessName, false)
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

	outcome, err := RegenSkill(hermesHome, []byte("Hola {business_name}"), "   ", false)
	if err == nil {
		t.Fatal("RegenSkill() error = nil, want a validation failure")
	}
	if outcome != SkillOutcomeWriteError {
		t.Errorf("outcome = %q, want %q", outcome, SkillOutcomeWriteError)
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("RegenSkill() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("RegenSkill() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
}
