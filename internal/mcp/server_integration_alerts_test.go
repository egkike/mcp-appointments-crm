package mcp

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"
)

// nextMonday10AM returns the next Monday at 10:00 in the seeded business
// timezone (America/Argentina/Buenos_Aires), strictly after today so the
// slot is always in the future no matter when the test runs.
func nextMonday10AM(t *testing.T) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	now := time.Now().In(loc)
	// Days until the next Monday strictly after today (1..7).
	days := (8 - int(now.Weekday())) % 7
	if days == 0 {
		days = 7
	}
	y, m, d := now.Date()
	return time.Date(y, m, d+days, 10, 0, 0, 0, loc)
}

// TestIntegrationAlertLifecycle proves that create_booking inserts a
// confirmation alert and cancel_booking cancels it.
func TestIntegrationAlertLifecycle(t *testing.T) {
	mux := newIntegrationMux(t)

	// Owner creates a booking. The start time must be a future Monday at
	// 10:00 (business timezone): p1 only works Mondays 09:00-17:00 per the
	// seed, and the slot validator rejects past dates. A hardcoded date
	// rots (it did on 2026-09-07), so compute the next Monday dynamically.
	start := nextMonday10AM(t)
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_booking","arguments":{"client_id":"c1","service_id":"s1","professional_id":"p1","start_datetime":%q}}}`, start.Format("2006-01-02T15:04:05-07:00"))
	rec := postMCPCaller(t, mux, "owner-1", body)
	result, code, msg := decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("create_booking failed: %d %q", code, msg)
	}
	var createOut struct {
		StructuredContent struct {
			BookingID string `json:"booking_id"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &createOut); err != nil {
		t.Fatalf("unmarshal create result: %v", err)
	}
	bookingID := createOut.StructuredContent.BookingID
	if bookingID == "" {
		t.Fatal("expected non-empty booking_id")
	}

	// get_pending_alerts by owner returns the confirmation alert.
	rec = postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_pending_alerts","arguments":{}}}`)
	result, code, msg = decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("get_pending_alerts failed: %d %q", code, msg)
	}
	var pendingOut struct {
		StructuredContent struct {
			Alerts []map[string]any `json:"alerts"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &pendingOut); err != nil {
		t.Fatalf("unmarshal pending result: %v", err)
	}
	if len(pendingOut.StructuredContent.Alerts) != 1 {
		t.Fatalf("expected 1 pending alert, got %d", len(pendingOut.StructuredContent.Alerts))
	}
	var alertID int
	if v, ok := pendingOut.StructuredContent.Alerts[0]["alert_id"].(float64); ok {
		alertID = int(v)
	} else {
		t.Fatalf("alert id not numeric: %v", pendingOut.StructuredContent.Alerts[0]["alert_id"])
	}

	// Cancel the booking.
	rec = postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"cancel_booking","arguments":{"booking_id":"`+bookingID+`","reason":"test"}}}`)
	_, code, msg = decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("cancel_booking failed: %d %q", code, msg)
	}

	// get_pending_alerts is now empty.
	rec = postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"get_pending_alerts","arguments":{}}}`)
	result, code, msg = decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("get_pending_alerts failed after cancel: %d %q", code, msg)
	}
	if err := json.Unmarshal(result, &pendingOut); err != nil {
		t.Fatalf("unmarshal pending result: %v", err)
	}
	if len(pendingOut.StructuredContent.Alerts) != 0 {
		t.Errorf("expected 0 pending alerts after cancel, got %d", len(pendingOut.StructuredContent.Alerts))
	}

	// mark_alert_as_sent on the previous id should now return NOT_FOUND (alert cancelled, no longer pending).
	rec = postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"mark_alert_as_sent","arguments":{"alert_id":`+strconv.Itoa(alertID)+`}}}`)
	_, code, msg = decodeRPCEnvelope(t, rec)
	if code != -32002 {
		t.Fatalf("mark_alert_as_sent expected not found after cancel, got %d %q", code, msg)
	}
	if !strings.Contains(msg, "alerta no encontrada") {
		t.Errorf("expected Spanish not-found message, got %q", msg)
	}
}

// TestIntegrationAlertToolsRoleRejection proves that staff and client callers
// are rejected from get_pending_alerts and mark_alert_as_sent with -32001.
func TestIntegrationAlertToolsRoleRejection(t *testing.T) {
	mux := newIntegrationMux(t)

	for _, caller := range []string{"staff-1", "c1"} {
		rec := postMCPCaller(t, mux, caller, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"get_pending_alerts","arguments":{}}}`)
		_, code, msg := decodeRPCEnvelope(t, rec)
		if code != -32001 {
			t.Errorf("caller %s get_pending_alerts code = %d; want -32001 (msg=%q)", caller, code, msg)
		}

		rec = postMCPCaller(t, mux, caller, `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"mark_alert_as_sent","arguments":{"alert_id":1}}}`)
		_, code, msg = decodeRPCEnvelope(t, rec)
		if code != -32001 {
			t.Errorf("caller %s mark_alert_as_sent code = %d; want -32001 (msg=%q)", caller, code, msg)
		}
	}
}
