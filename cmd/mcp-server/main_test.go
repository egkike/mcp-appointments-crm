package main

import (
	"bytes"
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/buildinfo"
)

func TestWantsVersion(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{
			name: "with --version flag",
			args: []string{"mcp-server", "--version"},
			want: true,
		},
		{
			name: "no args",
			args: []string{"mcp-server"},
			want: false,
		},
		{
			name: "other args",
			args: []string{"mcp-server", "--help"},
			want: false,
		},
		{
			name: "empty args",
			args: []string{},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := wantsVersion(tt.args); got != tt.want {
				t.Errorf("wantsVersion(%v) = %v, want %v", tt.args, got, tt.want)
			}
		})
	}
}

func TestPrintVersion(t *testing.T) {
	var buf bytes.Buffer
	printVersion(&buf)

	got := strings.TrimSpace(buf.String())
	want := buildinfo.Version
	if got != want {
		t.Errorf("printVersion() = %q, want %q", got, want)
	}
}

func TestBinaryVersionFlag(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping exec-based test on windows")
	}

	bin := filepath.Join(t.TempDir(), "mcp-server")
	cmd := exec.CommandContext(context.Background(), "go", "build", "-o", bin, ".") //nolint:gosec // test builds local package, bin path is TempDir-controlled
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, out)
	}

	cmd = exec.CommandContext(context.Background(), bin, "--version") //nolint:gosec // test executes TempDir-built binary with fixed args
	out, err = cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("binary --version failed: %v\n%s", err, out)
	}

	got := strings.TrimSpace(string(out))
	want := buildinfo.Version
	if got != want {
		t.Errorf("binary --version = %q, want %q", got, want)
	}
}
