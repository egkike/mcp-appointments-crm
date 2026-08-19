package mcp

import (
	"strings"
	"testing"
)

// TestValidateLoopback covers REQ-MT-001 and ADR-0007 §D4: the bind address
// must be a literal loopback IP (127.0.0.0/8 or ::1). Wildcards, hostnames and
// non-loopback addresses are rejected with Spanish fail-fast errors.
func TestValidateLoopback(t *testing.T) {
	tests := []struct {
		name    string
		bind    string
		wantErr string // substring of the expected Spanish error; "" means accept
	}{
		{name: "ipv4 loopback", bind: "127.0.0.1", wantErr: ""},
		{name: "ipv4 loopback /8 subnet", bind: "127.1.2.3", wantErr: ""},
		{name: "ipv6 loopback", bind: "::1", wantErr: ""},
		{name: "wildcard rejected", bind: "0.0.0.0", wantErr: "expone el server en TODAS las interfaces"},
		{name: "hostname rejected", bind: "localhost", wantErr: "es un hostname, no una IP"},
		{name: "private ipv4 rejected", bind: "192.168.1.5", wantErr: "no es una dirección loopback"},
		{name: "ipv4-mapped public ip rejected", bind: "::ffff:8.8.8.8", wantErr: "no es una dirección loopback"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLoopback(tt.bind)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("ValidateLoopback(%q) = %v, want nil", tt.bind, err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateLoopback(%q) = nil, want error containing %q", tt.bind, tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ValidateLoopback(%q) error = %q, want substring %q", tt.bind, err.Error(), tt.wantErr)
			}
		})
	}
}
