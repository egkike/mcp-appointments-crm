package mcp

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// TestIntegrationAlertLifecycle proves that create_booking inserts a
// confirmation alert and cancel_booking cancels it.
func TestIntegrationAlertLifecycle(t *testing.T) {
	mux := newIntegrationMux(t)

	// Owner creates a booking. The start time is in the future so the slot
	// validates, but we will query get_pending_alerts with a fixed clock set
	// after the scheduled alert time.
	rec := postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"create_booking","arguments":{"client_id":"c1","service_id":"s1","professional_id":"p1","start_datetime":"2026-09-07T10:00:00-03:00"}}}`)
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
