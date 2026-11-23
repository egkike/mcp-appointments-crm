package mcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

// TestIntegrationFindAvailableSlotsStaffNeedsProfessional pins the refusal a
// staff caller must get when it names no professional (issue #259).
//
// Before this, the same permission produced two contracts depending on how the
// tool was called: a foreign professional_id was refused ("no tiene permiso para
// ver este profesional"), while an omitted one returned an EMPTY GRID — which the
// agent reads as "no availability", not "no permission". The agenda layer
// explains why: for a staff caller the day read overrides the professional with
// the caller's own identity and the cache drops foreign rows, so an unnamed call
// discards every candidate silently.
//
// The three cases below are what makes the change safe in both directions: the
// refusal, the valid staff path, and the client path that must not move.
func TestIntegrationFindAvailableSlotsStaffNeedsProfessional(t *testing.T) {
	loc := slotsIntegrationLocation(t)
	monday := nextMonday(t, loc)
	day := monday.Format("2006-01-02")
	mux := newIntegrationMux(t)

	callArgs := func(professionalID string) string {
		if professionalID == "" {
			return fmt.Sprintf(`{"service_id":"s1","from":%q,"to":%q}`, day, day)
		}
		return fmt.Sprintf(`{"service_id":"s1","professional_id":%q,"from":%q,"to":%q}`, professionalID, day, day)
	}

	// call returns how many slots the caller received, or the JSON-RPC error.
	call := func(callerID, professionalID string) (slots int, code int64, msg string) {
		result, code, msg := callToolAs(t, mux, callerID, "find_available_slots", callArgs(professionalID))
		if code != 0 {
			return 0, code, msg
		}
		var payload struct {
			Slots []json.RawMessage `json:"slots"`
		}
		if err := json.Unmarshal([]byte(toolText(t, result)), &payload); err != nil {
			t.Fatalf("find_available_slots payload: %v", err)
		}
		return len(payload.Slots), 0, ""
	}

	t.Run("staff without a professional is refused", func(t *testing.T) {
		_, code, msg := call("staff-1", "")
		if code == 0 {
			t.Fatal("staff received a slot grid with no professional_id; it must be refused")
		}
		if !strings.Contains(msg, "professional_id") {
			t.Errorf("refusal = %q; it must tell the caller what to pass", msg)
		}
	})

	t.Run("staff with its own professional still works", func(t *testing.T) {
		slots, code, msg := call("staff-1", "p1")
		if code != 0 {
			t.Fatalf("staff with its own professional was refused: %d %q", code, msg)
		}
		if slots == 0 {
			t.Error("staff got an empty grid for its own professional on a working day")
		}
	})

	t.Run("client without a professional is served the grid", func(t *testing.T) {
		// Specialties no longer filter availability (issue #261), so this fixture —
		// whose professional has none loaded — must now produce slots. Before, this
		// case could only assert "not refused": the unnamed path dropped every
		// professional for lack of a specialty row.
		slots, code, msg := call("c1", "")
		if code != 0 {
			t.Fatalf("client without professional_id was refused: %d %q", code, msg)
		}
		if slots == 0 {
			t.Error("client got an empty grid; with specialties no longer filtering, the active professional must be offered")
		}
	})
}

// TestIntegrationAvailabilityIgnoresServiceSpecialties pins the rule decided in
// issue #261: a professional's specialty list does NOT restrict availability.
//
// The fixture gives the professional a specialty that is NOT the requested
// service, which is the exact shape the old filter rejected. Both resolution
// paths must still offer that professional, because specialties are catalog
// metadata and no other layer (booking included) validates them — a filter here
// would make availability disagree with what the system actually allows.
func TestIntegrationAvailabilityIgnoresServiceSpecialties(t *testing.T) {
	loc := slotsIntegrationLocation(t)
	monday := nextMonday(t, loc)
	day := monday.Format("2006-01-02")

	mux, dbConn := newIntegrationMuxWithDB(t)
	defer dbConn.Close() //nolint:errcheck // test cleanup

	// A specialty that does not include the requested service "s1".
	if _, err := dbConn.ExecContext(t.Context(),
		`UPDATE professionals SET specialties = '["s-otro"]' WHERE id = 'p1'`); err != nil {
		t.Fatalf("seed specialties: %v", err)
	}

	slotsUnnamed := func(callerID string) (int, int64, string) {
		result, code, msg := callToolAs(t, mux, callerID, "find_available_slots",
			fmt.Sprintf(`{"service_id":"s1","from":%q,"to":%q}`, day, day))
		if code != 0 {
			return 0, code, msg
		}
		var payload struct {
			Slots []json.RawMessage `json:"slots"`
		}
		if err := json.Unmarshal([]byte(toolText(t, result)), &payload); err != nil {
			t.Fatalf("payload: %v", err)
		}
		return len(payload.Slots), 0, ""
	}

	t.Run("unnamed path offers the professional anyway", func(t *testing.T) {
		slots, code, msg := slotsUnnamed("c1")
		if code != 0 {
			t.Fatalf("unnamed call failed: %d %q", code, msg)
		}
		if slots == 0 {
			t.Error("a professional whose specialty excludes the service was dropped; specialties must not filter availability")
		}
	})

	t.Run("named path offers the professional too", func(t *testing.T) {
		result, code, msg := callToolAs(t, mux, "c1", "find_available_slots",
			fmt.Sprintf(`{"service_id":"s1","professional_id":"p1","from":%q,"to":%q}`, day, day))
		if code != 0 {
			t.Fatalf("named call failed: %d %q", code, msg)
		}
		var payload struct {
			Slots []json.RawMessage `json:"slots"`
		}
		if err := json.Unmarshal([]byte(toolText(t, result)), &payload); err != nil {
			t.Fatalf("payload: %v", err)
		}
		if len(payload.Slots) == 0 {
			t.Error("the named path dropped the professional; both paths must agree")
		}
	})
}
