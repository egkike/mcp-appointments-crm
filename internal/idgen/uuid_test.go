package idgen_test

import (
	"regexp"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/idgen"
)

// uuidV4Regex matches the canonical UUID v4 string format:
// xxxxxxxx-xxxx-4xxx-yxxx-xxxxxxxxxxxx where:
//   - x is any hex digit
//   - the 13th hex digit (version) must be 4
//   - the 17th hex digit (variant) must be 8, 9, a, or b
var uuidV4Regex = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNew(t *testing.T) {
	t.Run("returns a non-empty string", func(t *testing.T) {
		id, err := idgen.New()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if id == "" {
			t.Fatal("New() returned empty string")
		}
	})

	t.Run("matches UUID v4 format", func(t *testing.T) {
		for i := 0; i < 100; i++ {
			id, err := idgen.New()
			if err != nil {
				t.Fatalf("unexpected error at iter %d: %v", i, err)
			}
			if !uuidV4Regex.MatchString(id) {
				t.Errorf("New() = %q; want valid UUID v4 format", id)
			}
		}
	})

	t.Run("returns unique values across many calls", func(t *testing.T) {
		const n = 10000
		seen := make(map[string]struct{}, n)
		for i := 0; i < n; i++ {
			id, err := idgen.New()
			if err != nil {
				t.Fatalf("unexpected error at iter %d: %v", i, err)
			}
			if _, dup := seen[id]; dup {
				t.Fatalf("collision after %d iterations: id %q appeared twice", i, id)
			}
			seen[id] = struct{}{}
		}
	})
}
