package admin

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// serverSeededConfig writes seed to a throwaway config path and loads it back,
// asserting first that the document round-trips the seed VERBATIM: the removal
// contract is stated against bytes, so the fixture must be the file. It mirrors
// telegramSeededConfig (same fixture discipline, different flow).
func serverSeededConfig(t *testing.T, seed string) HermesDocument {
	t.Helper()
	path := filepath.Join(t.TempDir(), HermesConfigFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(seed), 0o600); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	doc, err := LoadHermesConfig(path)
	if err != nil {
		t.Fatalf("load seeded config: %v", err)
	}
	roundTrip, err := marshalHermesDocument(doc)
	if err != nil {
		t.Fatalf("marshal round trip: %v", err)
	}
	if string(roundTrip) != seed {
		t.Fatalf("fixture does not round-trip verbatim:\n got:\n%s\nwant:\n%s", roundTrip, seed)
	}
	return doc
}

// TestRemoveHermesServer_RemovesOnlyTheServerEntry pins the byte-clean
// contract against a golden document: the mcp_servers.mcp-appointments entry
// is gone in full, and every other byte — the foreign top-level keys, the
// sibling server, and the section itself — survives verbatim.
func TestRemoveHermesServer_RemovesOnlyTheServerEntry(t *testing.T) {
	seed := strings.Join([]string{
		"model: hermes-default",
		"telemetry:",
		"  enabled: true",
		"mcp_servers:",
		"  openai:",
		"    url: https://api.example/mcp",
		"  mcp-appointments:",
		"    url: http://127.0.0.1:3000/mcp",
		"    headers:",
		`      X-Caller-Id: "` + validOwnerPhone + `"`,
		"    env:",
		`      ` + hermesTelegramTokenKey + `: "` + validTelegramToken + `"`,
		`      ` + hermesTelegramChatIDKey + `: "` + validTelegramChatID + `"`,
		`      ` + hermesTelegramPhoneKey + `: "` + validOwnerPhone + `"`,
		`      MCP_POLLERS_INTERVAL: "30"`,
		"",
	}, "\n")
	doc := serverSeededConfig(t, seed)

	// The removal is observable: the entry existed, so the call must report
	// that something changed.
	if removed := doc.RemoveHermesServer(); !removed {
		t.Fatal("RemoveHermesServer() = false, want true for a document holding the entry")
	}

	got, err := marshalHermesDocument(doc)
	if err != nil {
		t.Fatalf("marshal after removal: %v", err)
	}
	want := strings.Join([]string{
		"model: hermes-default",
		"telemetry:",
		"  enabled: true",
		"mcp_servers:",
		"  openai:",
		"    url: https://api.example/mcp",
		"",
	}, "\n")
	if string(got) != want {
		t.Errorf("document after removal =\n%s\nwant:\n%s", got, want)
	}
}

// TestRemoveHermesServer_DropsTheServersSectionWhenItEmpties pins the shape of
// the common single-server install: once the entry is gone the section key
// itself is dropped too (same rule the uninstall.sh mirror applies: a key
// holding no children is removed), instead of leaving an empty
// `mcp_servers:` shell.
func TestRemoveHermesServer_DropsTheServersSectionWhenItEmpties(t *testing.T) {
	seed := strings.Join([]string{
		"model: hermes-default",
		"mcp_servers:",
		"  mcp-appointments:",
		"    url: http://127.0.0.1:3000/mcp",
		"    headers:",
		`      X-Caller-Id: "` + validOwnerPhone + `"`,
		"",
	}, "\n")
	doc := serverSeededConfig(t, seed)

	if removed := doc.RemoveHermesServer(); !removed {
		t.Fatal("RemoveHermesServer() = false, want true when the entry existed")
	}

	got, err := marshalHermesDocument(doc)
	if err != nil {
		t.Fatalf("marshal after removal: %v", err)
	}
	want := "model: hermes-default\n"
	if string(got) != want {
		t.Errorf("document after removal =\n%s\nwant:\n%s", got, want)
	}
}

// TestRemoveHermesServer_IsIdempotentWhenAbsent pins the no-diff promise:
// removing a server that was never registered (or removing twice) leaves the
// document byte-identical, so the flow can run on every host without touching
// unrelated bytes — and the zero-value document is never materialized into a
// mapping by a removal.
func TestRemoveHermesServer_IsIdempotentWhenAbsent(t *testing.T) {
	cases := map[string]string{
		"no servers section": strings.Join([]string{
			"model: hermes-default",
			"telemetry:",
			"  enabled: true",
			"",
		}, "\n"),
		"section without the entry": strings.Join([]string{
			"model: hermes-default",
			"mcp_servers:",
			"  openai:",
			"    url: https://api.example/mcp",
			"",
		}, "\n"),
	}
	for name, seed := range cases {
		t.Run(name, func(t *testing.T) {
			doc := serverSeededConfig(t, seed)
			if removed := doc.RemoveHermesServer(); removed {
				t.Fatalf("RemoveHermesServer() = true, want false for %s", name)
			}
			got, err := marshalHermesDocument(doc)
			if err != nil {
				t.Fatalf("marshal after no-op removal: %v", err)
			}
			if string(got) != seed {
				t.Fatalf("no-op removal changed the document:\n got:\n%s\nwant:\n%s", got, seed)
			}
		})
	}

	// A zero-value document is never materialized by a removal.
	empty := &HermesDocument{}
	if removed := empty.RemoveHermesServer(); removed {
		t.Fatal("RemoveHermesServer() = true, want false for a zero-value document")
	}
}

// TestRemoveHermesServer_DropsAScalarEntry pins the orphaned-shape fail-safe:
// the mcp-appointments key is owned by this repo for every shape it can hold,
// so a scalar entry (a hand-edited or garbage leftover the server map flows
// reject) is dropped like any other shape of the same key — the shell mirror
// (uninstall.sh) already deletes the entry line regardless of its body,
// so the Go flow must not refuse what its mirror removes.
func TestRemoveHermesServer_DropsAScalarEntry(t *testing.T) {
	seed := strings.Join([]string{
		"model: hermes-default",
		"mcp_servers:",
		"  openai:",
		"    url: https://api.example/mcp",
		"  mcp-appointments: \"not-a-map\"",
		"",
	}, "\n")
	doc := serverSeededConfig(t, seed)

	if removed := doc.RemoveHermesServer(); !removed {
		t.Fatal("RemoveHermesServer() = false, want true for a scalar entry")
	}

	got, err := marshalHermesDocument(doc)
	if err != nil {
		t.Fatalf("marshal after removal: %v", err)
	}
	want := strings.Join([]string{
		"model: hermes-default",
		"mcp_servers:",
		"  openai:",
		"    url: https://api.example/mcp",
		"",
	}, "\n")
	if string(got) != want {
		t.Errorf("document after removal =\n%s\nwant:\n%s", got, want)
	}
}
