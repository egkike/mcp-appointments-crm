package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
)

// Integration coverage for the three owner-only business-hours exception tools
// (feat-hours-exception-tool, REQ-MT-005 / REQ-MT-015, REQ-BHE-MCP-001..007).
//
// The harness is the production composition over a real SQLite file
// (newIntegrationMuxWithOptions): real repositories → real use cases → tools →
// AuthMiddleware with the real owner-only RBAC rows. The mock-port suite in
// tools_hours_exception_test.go pins the handler contract; these tests prove the
// same contract holds with a live database, a live SDK schema and the transport
// RBAC gate — and, crucially, that a wire-created exception actually governs the
// availability read path with no read-path change (REQ-BHE-MCP-007).

// hoursExceptionSlot returns the RFC3339 start and the YYYY-MM-DD date (both in
// the seeded business timezone) of the next future Monday 10:00 slot: the one
// weekday the seed makes bookable end-to-end (business opens 09:00-18:00 every
// day, p1 works Mondays 09:00-17:00, service s1 lasts 60 minutes).
func hoursExceptionSlot(t *testing.T) (startRFC3339, date string) {
	t.Helper()
	monday := nextMonday10AM(t)
	return monday.Format("2006-01-02T15:04:05-07:00"), monday.Format("2006-01-02")
}

// checkAvailabilitySlot drives check_availability through the mux for the
// seeded s1/p1 pair and returns the raw envelope, so the caller can assert
// either the bookable slot or the closed semantic.
func checkAvailabilitySlot(t *testing.T, h http.Handler, callerID, startRFC3339 string) (json.RawMessage, int64, string) {
	t.Helper()
	return callMCPTool(t, h, callerID, "check_availability",
		fmt.Sprintf(`{"service_id":"s1","professional_id":"p1","start_datetime":%q}`, startRFC3339))
}

// listHoursExceptions runs list_hours_exceptions through the mux and decodes
// its structuredContent (a top-level JSON array of hoursExceptionOut).
func listHoursExceptions(t *testing.T, h http.Handler, callerID, from, to string) []hoursExceptionOut {
	t.Helper()
	result := mustCallTool(t, h, callerID, "list_hours_exceptions",
		fmt.Sprintf(`{"from":%q,"to":%q}`, from, to))
	var rows []hoursExceptionOut
	decodeToolStructured(t, result, &rows)
	return rows
}

// ── registration over the real mux (REQ-MT-005) ──

// TestIntegrationHoursExceptionToolsRegistered proves the three exception tools
// are live in the production composition: the harness injects every other port,
// so tools/list must expose the full 23-tool surface, three names included.
func TestIntegrationHoursExceptionToolsRegistered(t *testing.T) {
	mux := newIntegrationMux(t)

	rec := postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)
	result, code, msg := decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("tools/list failed: %d %q", code, msg)
	}
	var list struct {
		Tools []struct {
			Name string `json:"name"`
		} `json:"tools"`
	}
	if err := json.Unmarshal(result, &list); err != nil {
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	if len(list.Tools) != expectedToolCount {
		t.Errorf("tools = %d; want %d", len(list.Tools), expectedToolCount)
	}
	registered := make(map[string]bool, len(list.Tools))
	for _, tool := range list.Tools {
		registered[tool.Name] = true
	}
	for _, want := range []string{"create_hours_exception", "delete_hours_exception", "list_hours_exceptions"} {
		if !registered[want] {
			t.Errorf("hours-exception tool %q is not registered", want)
		}
	}
	if registered["update_hours_exception"] {
		t.Error("update_hours_exception is registered; exceptions are immutable (delete + create, D3)")
	}
}

// ── end-to-end: a wire-created exception governs the availability read path ──

// TestIntegrationHoursExceptionLifecycle drives the full contract over the real
// mux: baseline bookable slot → owner creates a CLOSING exception → the slot now
// reports the business closed (the exact semantic the booking/availability chain
// emits) → list returns it inside an inclusive [date, date] range → delete
// restores the weekly-schedule behavior.
func TestIntegrationHoursExceptionLifecycle(t *testing.T) {
	mux := newIntegrationMux(t)
	start, date := hoursExceptionSlot(t)

	// Baseline: the seeded Monday slot is bookable. Without this assertion the
	// "reverts" proof at the end could pass on a slot that never worked.
	result, code, msg := checkAvailabilitySlot(t, mux, "owner-1", start)
	if code != 0 {
		t.Fatalf("baseline check_availability failed: %d %q", code, msg)
	}
	var avail dto.CheckAvailabilityResult
	decodeToolStructured(t, result, &avail)
	if !avail.Available {
		t.Fatalf("baseline available = false; want true for the seeded Monday slot")
	}

	// Owner creates the CLOSING exception through the tool, with a reason so the
	// closed message carries it.
	createArgs := fmt.Sprintf(`{"exception_date":%q,"is_closed":true,"reason":"Feriado de prueba"}`, date)
	result = mustCallTool(t, mux, "owner-1", "create_hours_exception", createArgs)
	var created hoursExceptionOut
	decodeToolStructured(t, result, &created)
	if created.ID <= 0 || created.ExceptionDate != date || !created.IsClosed {
		t.Fatalf("created = %+v; want the stored closing exception for %s with a read-back id", created, date)
	}
	if created.OpenTime != nil || created.CloseTime != nil {
		t.Errorf("created times = %v/%v; want nil for a closed day", created.OpenTime, created.CloseTime)
	}

	// created_at stays off the wire (OQ-3, PRD §513): assert on the raw JSON, the
	// typed shape cannot see a field it does not declare.
	var raw struct {
		StructuredContent map[string]json.RawMessage `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &raw); err != nil {
		t.Fatalf("unmarshal raw create result: %v", err)
	}
	if _, ok := raw.StructuredContent["created_at"]; ok {
		t.Error("created_at leaked onto the wire; exceptions carry no timestamps (OQ-3)")
	}

	// The date now reports the business closed. This is the exact message of
	// booking_time_validator step 2 ("Negocio está cerrado el <date> (<reason>)."),
	// shared by check_availability, create_booking and reschedule_booking.
	_, code, msg = checkAvailabilitySlot(t, mux, "owner-1", start)
	wantClosed := fmt.Sprintf("Negocio está cerrado el %s (Feriado de prueba).", date)
	if code != -32002 || msg != wantClosed {
		t.Fatalf("check_availability after close: code=%d msg=%q; want -32002 %q", code, msg, wantClosed)
	}

	// list_hours_exceptions returns it. The range is exactly [date, date], so both
	// bounds are proven inclusive.
	listed := listHoursExceptions(t, mux, "owner-1", date, date)
	if len(listed) != 1 || listed[0].ID != created.ID || listed[0].ExceptionDate != date {
		t.Fatalf("listed = %+v; want the created row via the inclusive [%s, %s] range", listed, date, date)
	}

	// Delete it and prove only the exception is gone: the weekly schedule rules.
	result = mustCallTool(t, mux, "owner-1", "delete_hours_exception",
		fmt.Sprintf(`{"exception_id":%d}`, created.ID))
	var deleted dto.DeleteHoursExceptionResult
	decodeToolStructured(t, result, &deleted)
	if deleted.ExceptionID != created.ID || deleted.Status != "deleted" {
		t.Fatalf("deleted = %+v; want %d/deleted", deleted, created.ID)
	}
	if rows := listHoursExceptions(t, mux, "owner-1", date, date); len(rows) != 0 {
		t.Errorf("listed after delete = %+v; want no rows", rows)
	}
	result, code, msg = checkAvailabilitySlot(t, mux, "owner-1", start)
	if code != 0 {
		t.Fatalf("check_availability after delete: %d %q", code, msg)
	}
	decodeToolStructured(t, result, &avail)
	if !avail.Available {
		t.Errorf("available after delete = false; want true (weekly schedule restored)")
	}
}

// ── negative cases over the real mux ──

// TestIntegrationHoursExceptionRoleRejection proves the owner-only gate is live
// at the transport RBAC layer for all three tools: admin, staff and client are
// rejected with -32001 before any handler runs, while the owner succeeds.
//
// The observed code over the real mux is -32001 (the auth translator's 403
// envelope), NOT -32002: the ToolRBAC row {owner} rejects first, so the use
// case's own ErrCodeForbidden -> *domain.SemanticError gate (which toMCPError
// would emit as -32002 if it were ever reached) is the second, inner barrier and
// is exercised in-process by TestHoursExceptionUseCaseSemanticMessages.
func TestIntegrationHoursExceptionRoleRejection(t *testing.T) {
	mux := newIntegrationMux(t)
	_, date := hoursExceptionSlot(t)

	// The admin caller is included on purpose: admin is privileged everywhere
	// else, so pinning its denial proves the rows are owner-only and not
	// accidentally broad.
	cases := []struct {
		tool string
		args string
	}{
		{"create_hours_exception", fmt.Sprintf(`{"exception_date":%q,"is_closed":true}`, date)},
		{"delete_hours_exception", `{"exception_id":1}`},
		{"list_hours_exceptions", fmt.Sprintf(`{"from":%q,"to":%q}`, date, date)},
	}
	for _, caller := range []string{"admin-1", "staff-1", "c1"} {
		for _, tc := range cases {
			t.Run(caller+"/"+tc.tool, func(t *testing.T) {
				_, code, msg := callMCPTool(t, mux, caller, tc.tool, tc.args)
				if code != -32001 || msg != msgForbidden {
					t.Errorf("got code=%d msg=%q; want -32001 %q", code, msg, msgForbidden)
				}
			})
		}
	}

	// The create attempts above MUST NOT have written: an owner list over the
	// widest range comes back empty.
	if rows := listHoursExceptions(t, mux, "owner-1", "2000-01-01", "2100-12-31"); len(rows) != 0 {
		t.Errorf("stored rows = %+v; want none (a rejected create MUST NOT write)", rows)
	}
}

// TestIntegrationHoursExceptionDuplicateConflict proves the duplicate-date
// conflict reaches the client through the mux as the use case's Spanish
// semantic message, and that the original row is untouched.
func TestIntegrationHoursExceptionDuplicateConflict(t *testing.T) {
	mux := newIntegrationMux(t)
	_, date := hoursExceptionSlot(t)
	args := fmt.Sprintf(`{"exception_date":%q,"is_closed":true,"reason":"Feriado"}`, date)

	result := mustCallTool(t, mux, "owner-1", "create_hours_exception", args)
	var first hoursExceptionOut
	decodeToolStructured(t, result, &first)

	_, code, msg := callMCPTool(t, mux, "owner-1", "create_hours_exception", args)
	if code != -32002 || msg != msgHoursExceptionConflict {
		t.Fatalf("duplicate create: code=%d msg=%q; want -32002 %q", code, msg, msgHoursExceptionConflict)
	}

	rows := listHoursExceptions(t, mux, "owner-1", date, date)
	if len(rows) != 1 || rows[0].ID != first.ID {
		t.Errorf("rows after conflict = %+v; want the original row %d unchanged", rows, first.ID)
	}
}
