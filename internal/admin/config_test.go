package admin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

func TestConfigDir_EnvOverrideWins(t *testing.T) {
	want := filepath.Join(t.TempDir(), "custom-config")
	t.Setenv(ConfigDirEnvVar, want)

	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error = %v", err)
	}
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestConfigDir_EnvOverrideIsCleaned(t *testing.T) {
	base := t.TempDir()
	t.Setenv(ConfigDirEnvVar, base+string(filepath.Separator)+"nested"+string(filepath.Separator)+"..")

	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error = %v", err)
	}
	if got != base {
		t.Errorf("ConfigDir() = %q, want the cleaned %q", got, base)
	}
}

func TestConfigDir_DefaultIsHomeConfig(t *testing.T) {
	t.Setenv(ConfigDirEnvVar, "")
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skipf("no home directory available: %v", err)
	}

	got, err := ConfigDir()
	if err != nil {
		t.Fatalf("ConfigDir() error = %v", err)
	}
	want := filepath.Join(home, ".config", "mcp-appointments-crm")
	if got != want {
		t.Errorf("ConfigDir() = %q, want %q", got, want)
	}
}

func TestCallerIDPath(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	t.Setenv(ConfigDirEnvVar, dir)

	got, err := CallerIDPath()
	if err != nil {
		t.Fatalf("CallerIDPath() error = %v", err)
	}
	if want := filepath.Join(dir, CallerIDFileName); got != want {
		t.Errorf("CallerIDPath() = %q, want %q", got, want)
	}
}

func TestWriteCallerID_WritesRawIDWithRestrictedPermissions(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	t.Setenv(ConfigDirEnvVar, dir)

	const callerID = "+5491100000000"
	path, err := WriteCallerID(callerID)
	if err != nil {
		t.Fatalf("WriteCallerID() error = %v", err)
	}
	if want := filepath.Join(dir, CallerIDFileName); path != want {
		t.Errorf("WriteCallerID() path = %q, want %q", path, want)
	}

	// #nosec G304 -- path is inside t.TempDir() created by the test.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read caller-id file: %v", err)
	}
	if string(data) != callerID {
		t.Errorf("caller-id content = %q, want the raw id %q", string(data), callerID)
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat caller-id file: %v", err)
	}
	if got := fileInfo.Mode().Perm(); got != 0o600 {
		t.Errorf("caller-id mode = %04o, want 0600", got)
	}

	dirInfo, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat config dir: %v", err)
	}
	if got := dirInfo.Mode().Perm(); got != 0o700 {
		t.Errorf("config dir mode = %04o, want 0700", got)
	}
}

func TestWriteCallerID_TruncatesPreviousContent(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	t.Setenv(ConfigDirEnvVar, dir)

	if _, err := WriteCallerID("+5491100000000"); err != nil {
		t.Fatalf("WriteCallerID() first call error = %v", err)
	}
	path, err := WriteCallerID("+5491100002222")
	if err != nil {
		t.Fatalf("WriteCallerID() second call error = %v", err)
	}

	// #nosec G304 -- path is inside t.TempDir() created by the test.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read caller-id file: %v", err)
	}
	if string(data) != "+5491100002222" {
		t.Errorf("caller-id content = %q, want the second id", string(data))
	}
}

func TestWriteCallerID_TightensAnInheritedLooseMode(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "config")
	t.Setenv(ConfigDirEnvVar, dir)
	// The config dir inherits 0750 (group-readable, looser than the 0700 the
	// code enforces), and the caller-id file inherits 0644. The file mode is the
	// one under test: WriteCallerID must rewrite it as 0600.
	if err := os.MkdirAll(dir, 0o750); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	path := filepath.Join(dir, CallerIDFileName)
	// #nosec G306 -- the fixture must seed an inherited permissive file (0644);
	// seeding 0600 would make the assertion below pass without any tightening.
	if err := os.WriteFile(path, []byte("stale"), 0o644); err != nil {
		t.Fatalf("seed loose file: %v", err)
	}

	if _, err := WriteCallerID("+5491100000000"); err != nil {
		t.Fatalf("WriteCallerID() error = %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat caller-id file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("caller-id mode = %04o, want 0600 after rewriting a 0644 file", got)
	}
}

func TestWriteCallerID_UnwritableDirectoryFailsWithoutLeakingPath(t *testing.T) {
	// A regular file cannot be a parent directory, so MkdirAll fails before any
	// write happens.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("blocker"), 0o600); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	unwritable := filepath.Join(blocker, "config")
	t.Setenv(ConfigDirEnvVar, unwritable)

	_, err := WriteCallerID("+5491100000000")
	if err == nil {
		t.Fatal("WriteCallerID() error = nil, want a semantic failure")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("WriteCallerID() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInternal {
		t.Errorf("WriteCallerID() code = %q, want %q", semErr.Code, domain.ErrCodeInternal)
	}
	if strings.Contains(semErr.Message, unwritable) || strings.Contains(semErr.Message, blocker) {
		t.Errorf("WriteCallerID() message = %q, want no filesystem path in the operator message", semErr.Message)
	}
}

func TestWriteCallerID_EmptyCallerIDIsRejected(t *testing.T) {
	t.Setenv(ConfigDirEnvVar, filepath.Join(t.TempDir(), "config"))

	_, err := WriteCallerID("")
	if err == nil {
		t.Fatal("WriteCallerID(\"\") error = nil, want a validation error")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("WriteCallerID(\"\") error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("WriteCallerID(\"\") code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
}
