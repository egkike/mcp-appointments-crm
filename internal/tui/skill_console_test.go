package tui

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/admin"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// consoleSkillEnv pins an XDG config home with the seeded template and returns
// an injectable Hermes home, so the console outcomes are hermetic.
func consoleSkillEnv(t *testing.T) (hermesConfig HermesConfig, templatePath string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("POSIX-only XDG resolution, mirroring install.sh")
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home := t.TempDir()
	t.Setenv("HOME", home)
	hermesHome := filepath.Join(home, ".hermes")
	if err := os.MkdirAll(hermesHome, 0o700); err != nil {
		t.Fatalf("mkdir hermes home: %v", err)
	}
	hermesConfig = HermesConfig{Path: filepath.Join(hermesHome, admin.HermesConfigFileName)}
	p, err := admin.SkillTemplatePath()
	if err != nil {
		t.Fatalf("SkillTemplatePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatalf("mkdir template dir: %v", err)
	}
	templateBytes := "skill {business_name}"
	// #nosec G703 -- a resolved constant-slug path, no untrusted component.
	if err := os.WriteFile(p, []byte(templateBytes), 0o600); err != nil {
		t.Fatalf("write template: %v", err)
	}
	return hermesConfig, p
}

func TestRegenerateSkillForConsoleOutcomes(t *testing.T) {
	hermes, _ := consoleSkillEnv(t)
	const name = "Barbería El Corte"
	profile := fakeBusinessProfile{name: name}
	skillPath := filepath.Join(filepath.Dir(hermes.Path), "skills", admin.SkillSlug, admin.SkillFileName)
	rendered := "skill " + name

	t.Run("missing installs without asking", func(t *testing.T) {
		outcome, err := RegenerateSkillForConsole(context.Background(), hermes, profile, func() (bool, error) {
			t.Error("confirm hook must not run for a missing skill")
			return false, nil
		})
		if err != nil || outcome != admin.SkillOutcomeInstalled {
			t.Fatalf("outcome = %v err = %v, want instalada", outcome, err)
		}
	})

	t.Run("up to date does not ask", func(t *testing.T) {
		_ = os.MkdirAll(filepath.Dir(skillPath), 0o700) //nolint:errcheck
		// #nosec G703 -- constant slug under the temp home.
		_ = os.WriteFile(skillPath, []byte(rendered), 0o600) //nolint:errcheck
		outcome, err := RegenerateSkillForConsole(context.Background(), hermes, profile, func() (bool, error) {
			t.Error("confirm hook must not run for an up-to-date skill")
			return false, nil
		})
		if err != nil || outcome != admin.SkillOutcomeAlreadyInstalled {
			t.Fatalf("outcome = %v err = %v, want ya instalada sin cambios", outcome, err)
		}
	})

	t.Run("modified asks and declines keep the file", func(t *testing.T) {
		// #nosec G703 -- constant slug under the temp home.
		if werr := os.WriteFile(skillPath, []byte("contenido del operador"), 0o600); werr != nil {
			t.Fatalf("seed modified skill: %v", werr)
		}
		asked := false
		outcome, err := RegenerateSkillForConsole(context.Background(), hermes, profile, func() (bool, error) {
			asked = true
			return false, nil
		})
		if err != nil || outcome != admin.SkillOutcomeModified || !asked {
			t.Fatalf("outcome = %v err = %v asked = %v, want modified-without-replace", outcome, err, asked)
		}
		// #nosec G304 -- constant-slug fixture path under the temp home.
		body, _ := os.ReadFile(skillPath) //nolint:errcheck
		if string(body) != "contenido del operador" {
			t.Errorf("file = %q, want the operator content preserved", body)
		}
	})

	t.Run("modified confirmed replaces", func(t *testing.T) {
		outcome, err := RegenerateSkillForConsole(context.Background(), hermes, profile, func() (bool, error) { return true, nil })
		if err != nil || outcome != admin.SkillOutcomeInstalled {
			t.Fatalf("outcome = %v err = %v, want replaced", outcome, err)
		}
		// #nosec G304 -- constant-slug fixture path under the temp home.
		body, _ := os.ReadFile(skillPath) //nolint:errcheck
		if string(body) != rendered {
			t.Errorf("file = %q, want the fresh rendering %q", body, rendered)
		}
	})

	t.Run("invalid empty name is an input rejection", func(t *testing.T) {
		outcome, err := RegenerateSkillForConsole(context.Background(), hermes, fakeBusinessProfile{name: "   "}, func() (bool, error) {
			return false, nil
		})
		var semErr *domain.SemanticError
		if err == nil || outcome != admin.SkillOutcomeInvalidInput {
			t.Fatalf("outcome = %v err = %v, want SkillOutcomeInvalidInput", outcome, err)
		}
		_ = semErr
		if !strings.Contains(err.Error(), "obligatorio") {
			t.Errorf("err = %v, want the mandatory-name message", err)
		}
	})
}
