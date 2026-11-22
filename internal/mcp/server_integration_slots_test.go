package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
)

// ── find_available_slots over the real mux (agent-surface T2, issue #241) ──
//
// These tests go through the production chain (AuthMiddleware + JSON-RPC auth
// translator + SDK) on the shared integration composition, so they prove the
// observables Hermes sees: the tool is listed, 26 descriptors come back, and
// tools/call returns real slots computed by the REAL use case over the REAL
// SQLite adapters.
//
// The seeded business (seedIntegrationDB) is:
//
//	timezone        America/Argentina/Buenos_Aires
//	business hours  09:00-18:00 every day
//	service s1      60 minutes, active
//	professional p1 active, agenda Monday 09:00-17:00
//
// A 60-minute service inside the 09:00-17:00 Monday shift yields the grid
// 09:00..16:00 (8 slots), so every assertion below names an exact clock.

// slotsIntegrationLocation is the business timezone of the integration seed.
func slotsIntegrationLocation(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	return loc
}

// nextMonday returns the first Monday strictly after today in the business
// timezone. It keeps the fixtures in the future (the chain rejects past slots)
// without pinning a calendar date that would age out.
func nextMonday(t *testing.T, loc *time.Location) time.Time {
	t.Helper()
	now := time.Now().In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	for offset := 1; offset <= 7; offset++ {
		candidate := today.AddDate(0, 0, offset)
		if candidate.Weekday() == time.Monday {
			return candidate
		}
	}
	t.Fatal("no Monday found in the next 7 days")
	return time.Time{}
}

func TestIntegrationFindAvailableSlots(t *testing.T) {
	loc := slotsIntegrationLocation(t)
	monday := nextMonday(t, loc)
	day := monday.Format("2006-01-02")
	mux := newIntegrationMux(t)

	t.Run("tools/list reports the full surface including the new tool", func(t *testing.T) {
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
			t.Fatalf("tools/list result: %v", err)
		}
		if len(list.Tools) != expectedToolCount {
			t.Errorf("tools = %d; want %d", len(list.Tools), expectedToolCount)
		}
		// The SDK sorts alphabetically; membership is what the contract pins.
		var found bool
		for _, tool := range list.Tools {
			if tool.Name == "find_available_slots" {
				found = true
			}
		}
		if !found {
			t.Errorf("tools/list does not expose find_available_slots: %s", string(result))
		}
	})

	t.Run("tools/call returns the day grid", func(t *testing.T) {
		body := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"find_available_slots","arguments":{"service_id":"s1","professional_id":"p1","from":%q,"to":%q}}}`,
			day, day)
		rec := postMCPCaller(t, mux, "owner-1", body)
		result, code, msg := decodeRPCEnvelope(t, rec)
		if code != 0 {
			t.Fatalf("find_available_slots failed: %d %q", code, msg)
		}
		var call struct {
			StructuredContent dto.FindAvailableSlotsResult `json:"structuredContent"`
		}
		if err := json.Unmarshal(result, &call); err != nil {
			t.Fatalf("tools/call result: %v", err)
		}
		out := call.StructuredContent
		if out.TotalCandidates != 8 || out.Truncated || len(out.Slots) != 8 {
			t.Fatalf("total_candidates=%d truncated=%v slots=%d; want 8/false/8 (%s)",
				out.TotalCandidates, out.Truncated, len(out.Slots), string(result))
		}
		for index, slot := range out.Slots {
			wantHour := 9 + index
			want := time.Date(monday.Year(), monday.Month(), monday.Day(), wantHour, 0, 0, 0, loc)
			if !slot.StartDatetime.Equal(want) {
				t.Errorf("slot[%d].start = %s; want %s", index, slot.StartDatetime, want)
			}
			if slot.ProfessionalID != "p1" || slot.ProfessionalName != "Profesional Uno" {
				t.Errorf("slot[%d] professional = %q/%q; want p1/Profesional Uno",
					index, slot.ProfessionalID, slot.ProfessionalName)
			}
		}
	})

	t.Run("limit truncates and reports the true total", func(t *testing.T) {
		body := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"find_available_slots","arguments":{"service_id":"s1","professional_id":"p1","from":%q,"to":%q,"limit":3}}}`,
			day, day)
		rec := postMCPCaller(t, mux, "staff-1", body)
		result, code, msg := decodeRPCEnvelope(t, rec)
		if code != 0 {
			t.Fatalf("find_available_slots failed: %d %q", code, msg)
		}
		var call struct {
			StructuredContent dto.FindAvailableSlotsResult `json:"structuredContent"`
		}
		if err := json.Unmarshal(result, &call); err != nil {
			t.Fatalf("tools/call result: %v", err)
		}
		out := call.StructuredContent
		if out.TotalCandidates != 8 || !out.Truncated || len(out.Slots) != 3 {
			t.Errorf("total_candidates=%d truncated=%v slots=%d; want 8/true/3 (staff is any authenticated)",
				out.TotalCandidates, out.Truncated, len(out.Slots))
		}
	})

	t.Run("range over 14 days is a semantic error", func(t *testing.T) {
		from := monday.Format("2006-01-02")
		to := monday.AddDate(0, 0, 14).Format("2006-01-02")
		body := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"find_available_slots","arguments":{"service_id":"s1","professional_id":"p1","from":%q,"to":%q}}}`,
			from, to)
		rec := postMCPCaller(t, mux, "owner-1", body)
		_, code, msg := decodeRPCEnvelope(t, rec)
		if code != -32002 {
			t.Fatalf("code = %d; want -32002 (semantic)", code)
		}
		if msg != "el rango de fechas no puede superar los 14 días (se pidieron 15)." {
			t.Errorf("message = %q; want the 14-day semantic message", msg)
		}
	})
}

// TestIntegrationFindAvailableSlotsOverlapProvesTheInMemoryReader inserts a real
// booking through SQL and asserts the slot disappears, with no overlap error
// surfacing: the in-memory reader must reproduce the repository's storage format,
// cancelled exclusion and half-open interval comparison end to end.
func TestIntegrationFindAvailableSlotsOverlapProvesTheInMemoryReader(t *testing.T) {
	loc := slotsIntegrationLocation(t)
	monday := nextMonday(t, loc)
	day := monday.Format("2006-01-02")

	mux, conn := newIntegrationMuxWithDB(t)

	// Local 10:00-11:00 occupied (UTC-3 → 13:00Z-14:00Z, the storage format).
	startUTC := time.Date(monday.Year(), monday.Month(), monday.Day(), 10, 0, 0, 0, loc).UTC()
	endUTC := startUTC.Add(time.Hour)
	for _, row := range []struct {
		id        string
		status    string
		start     string
		end       string
		cancelled bool
	}{
		{id: "b-busy", status: "confirmed", start: startUTC.Format("2006-01-02T15:04:05.000Z"), end: endUTC.Format("2006-01-02T15:04:05.000Z")},
		{id: "b-cancelled", status: "cancelled", start: startUTC.Add(2 * time.Hour).Format("2006-01-02T15:04:05.000Z"), end: endUTC.Add(2 * time.Hour).Format("2006-01-02T15:04:05.000Z")},
	} {
		if _, err := conn.ExecContext(context.Background(),
			`INSERT INTO bookings (id, client_id, professional_id, service_id, start_datetime, end_datetime, status)
			 VALUES (?, 'c1', 'p1', 's1', ?, ?, ?)`,
			row.id, row.start, row.end, row.status,
		); err != nil {
			t.Fatalf("seed booking %s: %v", row.id, err)
		}
	}

	body := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"find_available_slots","arguments":{"service_id":"s1","professional_id":"p1","from":%q,"to":%q}}}`,
		day, day)
	rec := postMCPCaller(t, mux, "owner-1", body)
	result, code, msg := decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("find_available_slots failed: %d %q (an overlap must be filtered, never reported)", code, msg)
	}
	var call struct {
		StructuredContent dto.FindAvailableSlotsResult `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &call); err != nil {
		t.Fatalf("tools/call result: %v", err)
	}
	out := call.StructuredContent

	// 10:00 is gone; the cancelled booking at 12:00 blocks nothing.
	want := []int{9, 11, 12, 13, 14, 15, 16}
	if len(out.Slots) != len(want) || out.TotalCandidates != len(want) {
		t.Fatalf("slots=%d total_candidates=%d; want %d (%s)",
			len(out.Slots), out.TotalCandidates, len(want), string(result))
	}
	if out.Truncated {
		t.Error("truncated = true; want false")
	}
	for index, hour := range want {
		wantStart := time.Date(monday.Year(), monday.Month(), monday.Day(), hour, 0, 0, 0, loc)
		if !out.Slots[index].StartDatetime.Equal(wantStart) {
			t.Errorf("slot[%d].start = %s; want %s", index, out.Slots[index].StartDatetime, wantStart)
		}
	}
}
