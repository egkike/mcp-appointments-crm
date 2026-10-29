package admin

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

// Duplicate mapping keys are an ambiguity, not a cosmetic detail (GGA round 1
// on the B15 auth commit): the yaml.Node helpers of this package are
// first-match by construction (childValue/childMapping scan for the first key),
// so a document carrying two `mcp_servers` keys, two entries named
// `mcp-appointments`, or two values for the same env key would let this flow
// validate and write ONE of them while a different YAML consumer resolves the
// other. The load gate rejects those documents outright (fail closed, no silent
// "winner").
//
// The check is deliberately narrow: the top-level document mapping, the
// `mcp_servers` mapping, each server entry mapping and each owned `env` mapping.
// Duplicates elsewhere are the operator's business and stay loadable (true
// negative below).

// duplicateSeedPath writes seed verbatim to a throwaway Hermes config path. It
// deliberately does NOT assert a byte-identical round trip (unlike
// telegramSeededConfig): these fixtures are ambiguous by design and must never
// reach the writer.
func duplicateSeedPath(t *testing.T, seed string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), HermesConfigFileName)
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	return path
}

// TestLoadHermesConfig_RejectsDuplicateKeysInGuardedScopes pins the fail-secure
// gate: every duplicated key inside a scope this flow reads or writes is a load
// failure with a Spanish semantic message that names the scope path.
func TestLoadHermesConfig_RejectsDuplicateKeysInGuardedScopes(t *testing.T) {
	cases := []struct {
		name        string
		seed        string
		wantMessage string
	}{
		{
			name: "duplicated top-level mcp_servers key",
			seed: `model: hermes-default
mcp_servers:
  mcp-appointments:
    url: http://127.0.0.1:3000/mcp
mcp_servers:
  other-server:
    url: http://127.0.0.1:9999/mcp
`,
			wantMessage: "la configuración de Hermes tiene claves duplicadas (mcp_servers)",
		},
		{
			name: "duplicated server entry key",
			seed: `mcp_servers:
  mcp-appointments:
    url: http://127.0.0.1:3000/mcp
  mcp-appointments:
    url: http://127.0.0.1:3001/mcp
`,
			wantMessage: "la configuración de Hermes tiene claves duplicadas (mcp_servers.mcp-appointments)",
		},
		{
			name: "duplicated url key inside the owned entry",
			seed: `mcp_servers:
  mcp-appointments:
    url: http://127.0.0.1:3000/mcp
    url: http://198.51.100.7:3000/mcp
`,
			wantMessage: "la configuración de Hermes tiene claves duplicadas (mcp_servers.mcp-appointments.url)",
		},
		{
			name: "duplicated env mapping inside the owned entry",
			seed: `mcp_servers:
  mcp-appointments:
    url: http://127.0.0.1:3000/mcp
    env:
      MCP_TELEGRAM_BOT_TOKEN: "111111:first"
    env:
      MCP_TELEGRAM_BOT_TOKEN: "222222:second"
`,
			wantMessage: "la configuración de Hermes tiene claves duplicadas (mcp_servers.mcp-appointments.env)",
		},
		{
			name: "duplicated env value inside the owned env map",
			seed: `mcp_servers:
  mcp-appointments:
    url: http://127.0.0.1:3000/mcp
    env:
      MCP_TELEGRAM_BOT_TOKEN: "111111:first"
      MCP_TELEGRAM_BOT_TOKEN: "222222:second"
`,
			wantMessage: "la configuración de Hermes tiene claves duplicadas (mcp_servers.mcp-appointments.env.MCP_TELEGRAM_BOT_TOKEN)",
		},
		{
			// "each env mapping" of constraint 1: an env block is credential
			// material whichever server entry owns it, so the same ambiguity
			// fails closed there too.
			name: "duplicated env value inside a sibling server env map",
			seed: `mcp_servers:
  other-server:
    url: http://127.0.0.1:9999/mcp
    env:
      POLL_INTERVAL: "30"
      POLL_INTERVAL: "60"
`,
			wantMessage: "la configuración de Hermes tiene claves duplicadas (mcp_servers.other-server.env.POLL_INTERVAL)",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := duplicateSeedPath(t, tc.seed)

			doc, err := LoadHermesConfig(path)
			var semErr *domain.SemanticError
			if !errors.As(err, &semErr) {
				t.Fatalf("LoadHermesConfig() err = %v, want a *domain.SemanticError", err)
			}
			if semErr.Code != domain.ErrCodeInvalidInput {
				t.Errorf("code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
			}
			if semErr.Message != tc.wantMessage {
				t.Errorf("message = %q, want %q", semErr.Message, tc.wantMessage)
			}
			// The operator message must never leak the filesystem path.
			if strings.Contains(semErr.Message, filepath.Dir(path)) {
				t.Errorf("message = %q, want no filesystem path", semErr.Message)
			}
			// Fail closed: no half-built document travels with the failure, so a
			// caller that ignored the error could not write an ambiguous tree.
			if doc.fields.root != nil {
				t.Error("LoadHermesConfig() returned a document alongside the duplicate-key failure")
			}
		})
	}
}

// TestLoadHermesConfig_AllowsDuplicateKeysOutsideGuardedScopes is the true
// negative: the gate is scoped, not a blanket recursive walk, so a document
// whose ONLY duplicates live in sections this flow neither reads nor writes
// keeps loading exactly as before (backward compatibility).
func TestLoadHermesConfig_AllowsDuplicateKeysOutsideGuardedScopes(t *testing.T) {
	cases := []struct {
		name string
		seed string
	}{
		{
			name: "duplicate inside an unowned top-level section",
			seed: `model: hermes-default
telemetry:
  enabled: true
  enabled: false
mcp_servers:
  mcp-appointments:
    url: http://127.0.0.1:3000/mcp
`,
		},
		{
			name: "duplicate inside an entry headers map",
			seed: `mcp_servers:
  mcp-appointments:
    url: http://127.0.0.1:3000/mcp
    headers:
      X-Trace: "a"
      X-Trace: "b"
`,
		},
		{
			name: "duplicate inside a nested map of an entry that is not an owned scope",
			seed: `mcp_servers:
  mcp-appointments:
    url: http://127.0.0.1:3000/mcp
    launcher:
      command: run
      command: rerun
`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := duplicateSeedPath(t, tc.seed)

			doc, err := LoadHermesConfig(path)
			if err != nil {
				t.Fatalf("LoadHermesConfig() err = %v, want nil: duplicates outside the guarded scopes stay loadable", err)
			}
			if doc.fields.root == nil {
				t.Fatal("LoadHermesConfig() returned an empty document for a populated config")
			}
			// The guarded scopes are still reachable first-match, as before.
			if _, ok := childValue(doc.fields.root, hermesServersSection); !ok {
				t.Error("mcp_servers section missing from the loaded document")
			}
		})
	}
}

// TestLoadHermesConfig_RejectsKeyTextCollisionAcrossTags triangulates the
// identity rule: keys are compared by their literal text, so the integer 1 and
// the string "1" collide even though their YAML tags differ. Fail closed — two
// consumers may bind the same text to different values, and this flow must not
// pick a winner.
func TestLoadHermesConfig_RejectsKeyTextCollisionAcrossTags(t *testing.T) {
	path := duplicateSeedPath(t, `model: hermes-default
1: first
"1": second
mcp_servers:
  mcp-appointments:
    url: http://127.0.0.1:3000/mcp
`)

	_, err := LoadHermesConfig(path)
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("LoadHermesConfig() err = %v, want a *domain.SemanticError", err)
	}
	if want := "la configuración de Hermes tiene claves duplicadas (1)"; semErr.Message != want {
		t.Errorf("message = %q, want %q", semErr.Message, want)
	}
}

// TestHermesKeyLabel_NeutralizesHostileKeyText pins the message rendering: a key
// from the operator's file is echoed only after control characters are replaced,
// an empty key becomes a visible placeholder and an over-long key is truncated,
// so the semantic message is always one deterministic line.
func TestHermesKeyLabel_NeutralizesHostileKeyText(t *testing.T) {
	cases := []struct {
		name string
		key  string
		want string
	}{
		{"plain", "mcp_servers", "mcp_servers"},
		{"newline cannot inject a line", "a\nb", "a?b"},
		{"tab and del", "a\tb\x7f", "a?b?"},
		{"empty key", "", `""`},
		{"over-long key truncated", strings.Repeat("k", hermesDuplicateKeyLabelMaxLen+10),
			strings.Repeat("k", hermesDuplicateKeyLabelMaxLen) + "..."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hermesKeyLabel(tc.key); got != tc.want {
				t.Errorf("hermesKeyLabel(%q) = %q, want %q", tc.key, got, tc.want)
			}
		})
	}
}

// TestLoadHermesConfig_RejectsUnresolvableMappingKey pins the fail-closed
// rule for keys the duplicate gate cannot name: yaml.v3 itself loads an alias
// used AS a mapping key without any error, and an unrenderable complex key is
// equally ambiguous. Either way the sibling scopes could disagree about which
// value the slot holds. The load fails with a semantic error, never with a
// silently skipped ambiguity (GGA round 1 of the duplicate-key commit).
func TestLoadHermesConfig_RejectsUnresolvableMappingKey(t *testing.T) {
	seed := "k: &a v\nmcp_servers:\n  *a: q\n"
	_, err := LoadHermesConfig(duplicateSeedPath(t, seed))

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("LoadHermesConfig err = %v, want a *domain.SemanticError for an alias mapping key", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
}

// TestLoadHermesConfig_RejectsMergeKeyInGuardedScope pins the merge-key rule:
// yaml.v3 keeps `<<` unresolved at the Node level, so the first-match helpers
// never see inherited content — SetHermesServer would add a direct
// mcp_servers key that shadows whatever the merge inherited. The managed
// scopes therefore reject merge keys instead of resolving them into the
// document (ambiguity rejected, not rewritten; GGA round 2 of this commit).
func TestLoadHermesConfig_RejectsMergeKeyInGuardedScope(t *testing.T) {
	for name, seed := range map[string]string{
		"merge in entry": "mcp_servers:\n  <<: &base {url: x}\n  mcp-appointments: y\n",
		"merge in root":  "<<: &base {mcp_servers: {}}\n",
		"merge in env":   "mcp_servers:\n  mcp-appointments:\n    env:\n      <<: &e {k: v}\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := LoadHermesConfig(duplicateSeedPath(t, seed))
			var semErr *domain.SemanticError
			if !errors.As(err, &semErr) {
				t.Fatalf("LoadHermesConfig(%s) err = %v, want a *domain.SemanticError for the merge key", name, err)
			}
			if semErr.Code != domain.ErrCodeInvalidInput {
				t.Errorf("code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
			}
		})
	}
}
