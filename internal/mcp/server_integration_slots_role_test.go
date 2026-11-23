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

	t.Run("client without a professional is not refused", func(t *testing.T) {
		// Only the REFUSAL is asserted here. The grid can legitimately be empty:
		// the unnamed path filters candidates by service specialty
		// (find_available_slots.go, HasSpecialty) and this fixture seeds none, so an
		// empty grid is pre-existing behaviour unrelated to issue #259. What must
		// not change is that a client is served at all.
		_, code, msg := call("c1", "")
		if code != 0 {
			t.Fatalf("client without professional_id was refused: %d %q", code, msg)
		}
	})
}
