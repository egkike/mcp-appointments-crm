package mcp

import (
	"os"
	"strings"
	"testing"
)

// TestNoRepositoryImport guards REQ-MT-012: the transport package must NOT
// import internal/repository. The six consumer ports in ports.go are the only
// coupling to the application layer, and the composition root
// (cmd/mcp-server) wires the concrete repositories. A future refactor that
// drags repository imports into the transport would break this test at the
// source-file level (the compiler cannot express "no import" as a
// dependency, so the guard scans the non-test sources of this package).
func TestNoRepositoryImport(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	const repoImport = `"github.com/egkike/mcp-appointments-crm/internal/repository"`
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := os.ReadFile(e.Name())
		if err != nil {
			t.Fatalf("read %s: %v", e.Name(), err)
		}
		if strings.Contains(string(src), repoImport) {
			t.Errorf("%s imports internal/repository — the transport consumes ports, not repos (REQ-MT-012)", e.Name())
		}
	}
}
