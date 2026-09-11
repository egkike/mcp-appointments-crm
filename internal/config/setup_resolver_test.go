package config

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveSetupDirOS(t *testing.T) {
	tests := []struct {
		name    string
		goos    string
		env     map[string]string
		want    string
		wantErr bool
		errSub  string
	}{
		{
			name: "linux default",
			goos: "linux",
			env: map[string]string{
				"HOME": "/home/user",
			},
			want: filepath.Join("/home", "user", ".config", "mcp-appointments-crm", "setup"),
		},
		{
			name: "linux with XDG_CONFIG_HOME",
			goos: "linux",
			env: map[string]string{
				"HOME":            "/home/user",
				"XDG_CONFIG_HOME": "/custom/config",
			},
			want: filepath.Join("/custom", "config", "mcp-appointments-crm", "setup"),
		},
		{
			name: "macos default",
			goos: "darwin",
			env: map[string]string{
				"HOME": "/Users/user",
			},
			want: filepath.Join("/Users", "user", "Library", "Application Support", "MCP Appointments CRM", "setup"),
		},
		{
			name: "MCP_SETUP_DIR wins on linux",
			goos: "linux",
			env: map[string]string{
				"HOME":          "/home/user",
				"MCP_SETUP_DIR": "/tmp/test-setup",
			},
			want: "/tmp/test-setup",
		},
		{
			name: "MCP_SETUP_DIR wins on darwin",
			goos: "darwin",
			env: map[string]string{
				"HOME":          "/Users/user",
				"MCP_SETUP_DIR": "/tmp/test-setup",
			},
			want: "/tmp/test-setup",
		},
		{
			name:    "empty HOME without MCP_SETUP_DIR",
			goos:    "linux",
			env:     map[string]string{},
			wantErr: true,
			errSub:  "HOME no está definida",
		},
		{
			name: "windows or unknown OS falls back to XDG rule",
			goos: "windows",
			env: map[string]string{
				"HOME": "/home/user",
			},
			want: filepath.Join("/home", "user", ".config", "mcp-appointments-crm", "setup"),
		},
		{
			name: "empty MCP_SETUP_DIR treated as unset",
			goos: "linux",
			env: map[string]string{
				"HOME":          "/home/user",
				"MCP_SETUP_DIR": "",
			},
			want: filepath.Join("/home", "user", ".config", "mcp-appointments-crm", "setup"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string {
				return tt.env[key]
			}
			got, err := resolveSetupDirOS(tt.goos, getenv)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.errSub)
				}
				if tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
					t.Fatalf("error %q does not contain %q", err.Error(), tt.errSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}
