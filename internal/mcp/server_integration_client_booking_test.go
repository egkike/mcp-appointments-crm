package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestIntegrationClientBooksOwnBooking is the test that was missing when the
// transport denied the client role its own booking: the client-own path was
// covered only at use-case level, which never passes through the RBAC gate.
//
// What it pins (issue #251):
//   - a `client` caller may create a booking for ITSELF;
//   - the same caller may cancel that booking;
//   - a DIFFERENT client may not cancel it, and the booking survives the attempt.
//
// The last case is the safety argument for opening the gate: the transport no
// longer stands in the way, so the isolation claim rests entirely on the SQL
// scoping (applyAuthFilter adds `AND client_id = ?` to Cancel's read and write).
// If that scoping is ever removed, this test fails instead of leaking.
func TestIntegrationClientBooksOwnBooking(t *testing.T) {
	loc := slotsIntegrationLocation(t)
	monday := nextMonday(t, loc)
	start := time.Date(monday.Year(), monday.Month(), monday.Day(), 10, 0, 0, 0, loc)

	mux := newIntegrationMux(t)

	bookingID := createBookingAs(t, mux, "c1", "c1", start)
	if bookingID == "" {
		t.Fatalf("create_booking as client c1 returned no booking id")
	}

	t.Run("another client cannot cancel that booking", func(t *testing.T) {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"cancel_booking","arguments":{"booking_id":%q,"reason":"intento ajeno"}}}`, bookingID)
		rec := postMCPCaller(t, mux, "c2", body)
		_, code, msg := decodeRPCEnvelope(t, rec)
		if code == 0 {
			t.Fatalf("client c2 cancelled client c1's booking %s; the ownership chain below the transport is not holding", bookingID)
		}
		if msg == "" {
			t.Errorf("refusal carried no message")
		}

		// The booking must be untouched: a failed cancellation cannot be
		// half-applied.
		if status := bookingStatusAs(t, mux, "owner-1", bookingID); status == "cancelled" {
			t.Errorf("booking %s is cancelled after a foreign client's attempt: the refusal leaked a write", bookingID)
		}
	})

	t.Run("another client cannot reschedule that booking", func(t *testing.T) {
		before := bookingStartAs(t, mux, "owner-1", bookingID)
		newStart := start.Add(2 * time.Hour)

		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"reschedule_booking","arguments":{"booking_id":%q,"new_start_datetime":%q}}}`, bookingID, newStart.Format(time.RFC3339))
		rec := postMCPCaller(t, mux, "c2", body)
		_, code, msg := decodeRPCEnvelope(t, rec)
		if code == 0 {
			t.Fatalf("client c2 rescheduled client c1's booking %s; the ownership chain below the transport is not holding", bookingID)
		}
		if msg == "" {
			t.Errorf("refusal carried no message")
		}

		// The start must be untouched: reschedule is three scoped statements, and a
		// refusal that still moved the time would be a partial write.
		if after := bookingStartAs(t, mux, "owner-1", bookingID); after != before {
			t.Errorf("booking start moved from %q to %q after a foreign reschedule attempt", before, after)
		}
	})

	t.Run("the owning client cancels its own booking", func(t *testing.T) {
		body := fmt.Sprintf(`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"cancel_booking","arguments":{"booking_id":%q,"reason":"ya no puedo ir"}}}`, bookingID)
		rec := postMCPCaller(t, mux, "c1", body)
		_, code, msg := decodeRPCEnvelope(t, rec)
		if code != 0 {
			t.Fatalf("client c1 could not cancel its own booking: %d %q", code, msg)
		}
		if status := bookingStatusAs(t, mux, "owner-1", bookingID); status != "cancelled" {
			t.Errorf("booking status = %q after a successful cancel; want cancelled", status)
		}
	})
}

// createBookingAs books a slot through the transport as callerID and returns the
// created booking id, failing the test on any JSON-RPC error.
func createBookingAs(t *testing.T, mux http.Handler, callerID, clientID string, start time.Time) string {
	t.Helper()
	body := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"create_booking","arguments":{"client_id":%q,"service_id":"s1","professional_id":"p1","start_datetime":%q}}}`,
		clientID, start.Format(time.RFC3339))
	rec := postMCPCaller(t, mux, callerID, body)
	result, code, msg := decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("create_booking as %s failed: %d %q", callerID, code, msg)
	}

	var content struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &content); err != nil {
		t.Fatalf("create_booking result: %v", err)
	}
	if len(content.Content) == 0 {
		t.Fatalf("create_booking returned no content: %s", string(result))
	}
	var payload struct {
		BookingID string `json:"booking_id"`
	}
	if err := json.Unmarshal([]byte(content.Content[0].Text), &payload); err != nil {
		t.Fatalf("create_booking payload %q: %v", content.Content[0].Text, err)
	}
	return payload.BookingID
}

// bookingView is the subset of the wire view these assertions read.
type bookingView struct {
	Status        string `json:"status"`
	StartDatetime string `json:"start_datetime"`
}

// bookingStartAs reads a booking's start through the transport as callerID.
func bookingStartAs(t *testing.T, mux http.Handler, callerID, bookingID string) string {
	t.Helper()
	return bookingViewAs(t, mux, callerID, bookingID).StartDatetime
}

// bookingStatusAs reads a booking's status through the transport as callerID.
func bookingStatusAs(t *testing.T, mux http.Handler, callerID, bookingID string) string {
	t.Helper()
	return bookingViewAs(t, mux, callerID, bookingID).Status
}

// bookingViewAs reads the booking view through the transport as callerID.
// GetBookingResult wraps the view: {"booking": {...}}.
func bookingViewAs(t *testing.T, mux http.Handler, callerID, bookingID string) bookingView {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"get_booking","arguments":{"booking_id":%q}}}`, bookingID)
	rec := postMCPCaller(t, mux, callerID, body)
	result, code, msg := decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("get_booking as %s failed: %d %q", callerID, code, msg)
	}
	var content struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(result, &content); err != nil {
		t.Fatalf("get_booking result: %v", err)
	}
	if len(content.Content) == 0 {
		t.Fatalf("get_booking returned no content: %s", string(result))
	}
	var payload struct {
		Booking bookingView `json:"booking"`
	}
	if err := json.Unmarshal([]byte(content.Content[0].Text), &payload); err != nil {
		t.Fatalf("get_booking payload: %v", err)
	}
	return payload.Booking
}
