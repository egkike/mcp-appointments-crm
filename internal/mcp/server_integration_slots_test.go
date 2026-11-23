package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
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

// ── availability is a question about the calendar, not about the caller (S9) ──

// seedOtherClientBooking inserts a booking held by ANOTHER client (c2) on the
// seeded professional p1, at business-local 10:00 of the given Monday.
func seedOtherClientBooking(t *testing.T, conn *sql.DB, loc *time.Location, monday time.Time) (bookingID string, startUTC, endUTC time.Time) {
	t.Helper()
	startUTC = time.Date(monday.Year(), monday.Month(), monday.Day(), 10, 0, 0, 0, loc).UTC()
	endUTC = startUTC.Add(time.Hour)
	if _, err := conn.ExecContext(context.Background(),
		`INSERT INTO bookings (id, client_id, professional_id, service_id, start_datetime, end_datetime, status)
		 VALUES ('b-other-client', 'c2', 'p1', 's1', ?, ?, 'confirmed')`,
		startUTC.Format("2006-01-02T15:04:05.000Z"), endUTC.Format("2006-01-02T15:04:05.000Z"),
	); err != nil {
		t.Fatalf("seed booking b-other-client: %v", err)
	}
	return "b-other-client", startUTC, endUTC
}

// seedUnnamedRosterFixture prepares the ROSTER question for a staff caller: a
// second active professional (p2) plus the specialties the unnamed candidate path
// requires on BOTH professionals.
//
//   - Specialties: with no professional_id the use case resolves candidates
//     through FindAllActive and keeps only the professionals that offer the
//     service (entity.Professional.HasSpecialty, fail-closed on an empty/absent
//     value). The shared seed leaves p1 without specialties, so without the UPDATE
//     below the roster would be empty and this subtest would assert nothing.
//   - Identical shifts: p2 gets p1's 09:00-17:00 Monday shift so both agendas
//     produce the same 09:00..16:00 grid. That keeps the schedule read (whose own
//     staff scoping is out of scope here, S10/S11) from confounding the assertion:
//     the ONLY difference between the two agendas is the booking.
//   - p2's 11:00 is booked to another client (c2): that is the slot the staff case
//     must stop reporting as free.
func seedUnnamedRosterFixture(t *testing.T, conn *sql.DB, loc *time.Location, monday time.Time) {
	t.Helper()
	for _, stmt := range []string{
		`UPDATE professionals SET specialties = '["s1"]' WHERE id = 'p1'`,
		`INSERT INTO professionals (id, name, status, specialties) VALUES ('p2', 'Profesional Dos', 'active', '["s1"]')`,
		`INSERT INTO schedules (professional_id, day_of_week, start_time, end_time) VALUES ('p2', 1, '09:00', '17:00')`,
	} {
		if _, err := conn.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("seed %q: %v", stmt, err)
		}
	}
	startUTC := time.Date(monday.Year(), monday.Month(), monday.Day(), 11, 0, 0, 0, loc).UTC()
	endUTC := startUTC.Add(time.Hour)
	if _, err := conn.ExecContext(context.Background(),
		`INSERT INTO bookings (id, client_id, professional_id, service_id, start_datetime, end_datetime, status)
		 VALUES ('b-other-professional', 'c2', 'p2', 's1', ?, ?, 'confirmed')`,
		startUTC.Format("2006-01-02T15:04:05.000Z"), endUTC.Format("2006-01-02T15:04:05.000Z"),
	); err != nil {
		t.Fatalf("seed booking b-other-professional: %v", err)
	}
}

// wantSlotHours asserts the complete slot list: the exact clock of every entry,
// the professional identity of each one, the true candidate total and no
// truncation. It is the whole observable effect of the grid, not a subset.
func wantSlotHours(t *testing.T, out dto.FindAvailableSlotsResult, day time.Time, loc *time.Location, professionalID, professionalName string, hours ...int) {
	t.Helper()
	if out.TotalCandidates != len(hours) || out.Truncated || len(out.Slots) != len(hours) {
		t.Fatalf("total_candidates=%d truncated=%v slots=%d; want %d/false/%d (got %v)",
			out.TotalCandidates, out.Truncated, len(out.Slots), len(hours), len(hours), slotStarts(out))
	}
	for index, hour := range hours {
		want := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, loc)
		if !out.Slots[index].StartDatetime.Equal(want) {
			t.Errorf("slot[%d].start = %s; want %s", index, out.Slots[index].StartDatetime, want)
		}
		if out.Slots[index].ProfessionalID != professionalID {
			t.Errorf("slot[%d].professional_id = %q; want %q", index, out.Slots[index].ProfessionalID, professionalID)
		}
		if out.Slots[index].ProfessionalName != professionalName {
			t.Errorf("slot[%d].professional_name = %q; want %q", index, out.Slots[index].ProfessionalName, professionalName)
		}
	}
}

// slotStarts renders the returned grid for a failure message: start plus the
// professional, because a multi-professional grid is unreadable by clock alone
// (two slots can share a start and differ only by agenda).
func slotStarts(out dto.FindAvailableSlotsResult) []string {
	starts := make([]string, 0, len(out.Slots))
	for _, slot := range out.Slots {
		starts = append(starts, slot.StartDatetime.Format("2006-01-02T15:04")+"/"+slot.ProfessionalID)
	}
	return starts
}

// findAvailableSlotsUnnamedAs drives find_available_slots through the mux with
// professional_id OMITTED: the candidate set is then the whole active roster
// (FindAllActive), which is the only path where a staff caller used to be able to
// see other professionals' agendas.
func findAvailableSlotsUnnamedAs(t *testing.T, h http.Handler, callerID, day string) dto.FindAvailableSlotsResult {
	t.Helper()
	// limit 50: the two-professional grid is 14 candidates and the default limit
	// is 10, so omitting it would truncate the very slots under assertion.
	body := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"find_available_slots","arguments":{"service_id":"s1","from":%q,"to":%q,"limit":50}}}`,
		day, day)
	result, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, h, callerID, body))
	if code != 0 {
		t.Fatalf("find_available_slots (no professional_id) as %q failed: %d %q", callerID, code, msg)
	}
	var call struct {
		StructuredContent dto.FindAvailableSlotsResult `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &call); err != nil {
		t.Fatalf("tools/call result: %v", err)
	}
	return call.StructuredContent
}

// slotIndex returns the position of the (professional, business-local hour) pair
// in the returned grid, or -1 when the slot is absent.
func slotIndex(out dto.FindAvailableSlotsResult, day time.Time, loc *time.Location, hour int, professionalID string) int {
	start := time.Date(day.Year(), day.Month(), day.Day(), hour, 0, 0, 0, loc)
	for index, slot := range out.Slots {
		if slot.ProfessionalID == professionalID && slot.StartDatetime.Equal(start) {
			return index
		}
	}
	return -1
}

// findAvailableSlotsAs drives find_available_slots through the mux for a caller
// and decodes its structured result.
func findAvailableSlotsAs(t *testing.T, h http.Handler, callerID, professionalID, day string) dto.FindAvailableSlotsResult {
	t.Helper()
	body := fmt.Sprintf(
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"find_available_slots","arguments":{"service_id":"s1","professional_id":%q,"from":%q,"to":%q}}}`,
		professionalID, day, day)
	result, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, h, callerID, body))
	if code != 0 {
		t.Fatalf("find_available_slots as %q failed: %d %q (any authenticated caller is admitted)", callerID, code, msg)
	}
	var call struct {
		StructuredContent dto.FindAvailableSlotsResult `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &call); err != nil {
		t.Fatalf("tools/call result: %v", err)
	}
	return call.StructuredContent
}

// TestIntegrationAvailabilityIgnoresCallerScope is the S9 proof, and the ONLY
// real one: the narrow AvailabilityDeps.Bookings interface does not stop the
// composition root from injecting the filtered BookingsRepo, because that repo
// satisfies it too (trap 3). Only a run through the REAL composition can show
// which adapter was injected.
//
// The scenario is the defect itself: c2 holds a 10:00 booking with p1, and c1
// asks the same question. With the caller-scoped reader c1 saw 10:00 as free
// (the AND client_id = ? clause hid c2's row); with the neutral reader the slot
// is reported as taken by both availability tools, and nothing else moves:
//
//   - find_available_slots returns the other seven hours, with the true count;
//   - check_availability answers -32002 with the overlap message (S7: the
//     professional and the window, never a client identity);
//   - a genuinely free hour is still available (the reader does not over-block);
//   - the DATA path is untouched: get_booking still hides c2's booking from c1
//     behind the same ErrNotFound-shaped answer, with no existence oracle.
//
// The last subtest covers the OTHER half of the defect (S1/S11): professional_id
// OMITTED. The candidate set there comes from FindAllActive, which is unscoped by
// design, so the grid can be built for several professionals — and any agenda the
// grid renders must not be answered from a range read scoped to the caller. With
// the caller-scoped reader each rendered agenda saw only the caller's own rows.
// For a client caller that is directly observable (both agendas rendered, both
// bookings owned by c2), which is what section 1 pins; for a staff caller a second
// scoping layer (the schedules read) limits the grid to the own agenda, documented
// in section 2 so the subtest is not credited with more than it proves.
func TestIntegrationAvailabilityIgnoresCallerScope(t *testing.T) {
	loc := slotsIntegrationLocation(t)
	monday := nextMonday(t, loc)
	day := monday.Format("2006-01-02")

	mux, conn := newIntegrationMuxWithDB(t)
	bookingID, startUTC, endUTC := seedOtherClientBooking(t, conn, loc, monday)

	t.Run("find_available_slots hides another client's booking from a client caller", func(t *testing.T) {
		out := findAvailableSlotsAs(t, mux, "c1", "p1", day)
		// 10:00 belongs to c2 and must be missing from c1's answer.
		wantSlotHours(t, out, monday, loc, "p1", "Profesional Uno", 9, 11, 12, 13, 14, 15, 16)
	})

	t.Run("check_availability reports the overlap to a client caller", func(t *testing.T) {
		body := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"check_availability","arguments":{"service_id":"s1","professional_id":"p1","start_datetime":%q}}}`,
			time.Date(monday.Year(), monday.Month(), monday.Day(), 10, 0, 0, 0, loc).Format(time.RFC3339))
		_, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, mux, "c1", body))
		if code != -32002 {
			t.Fatalf("code = %d msg = %q; want -32002 (the slot is taken)", code, msg)
		}
		want := fmt.Sprintf("Profesional Profesional Uno ya tiene una reserva de %s a %s.",
			startUTC.Format(time.RFC3339), endUTC.Format(time.RFC3339))
		if msg != want {
			t.Errorf("message = %q; want %q", msg, want)
		}
	})

	t.Run("a free slot is still reported available to a client caller", func(t *testing.T) {
		body := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"check_availability","arguments":{"service_id":"s1","professional_id":"p1","start_datetime":%q}}}`,
			time.Date(monday.Year(), monday.Month(), monday.Day(), 15, 0, 0, 0, loc).Format(time.RFC3339))
		result, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, mux, "c1", body))
		if code != 0 {
			t.Fatalf("check_availability of a free slot failed: %d %q", code, msg)
		}
		var call struct {
			StructuredContent dto.CheckAvailabilityResult `json:"structuredContent"`
		}
		if err := json.Unmarshal(result, &call); err != nil {
			t.Fatalf("tools/call result: %v", err)
		}
		if !call.StructuredContent.Available {
			t.Error("available = false; want true (15:00 is free for p1)")
		}
	})

	t.Run("the data path still hides the other client's booking", func(t *testing.T) {
		body := fmt.Sprintf(
			`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"get_booking","arguments":{"booking_id":%q}}}`,
			bookingID)
		_, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, mux, "c1", body))
		if code != -32002 || msg != "reserva no encontrada" {
			t.Errorf("code=%d msg=%q; want -32002 %q (no existence oracle for another client's booking)", code, msg, "reserva no encontrada")
		}
	})

	// ── the unnamed path (S1/S11): the roster question over two agendas ──
	//
	// With professional_id OMITTED the candidate set comes from FindAllActive,
	// which is unscoped BY DESIGN, so the grid is built for every active
	// professional that offers the service. The range read must be role-neutral for
	// the same reason: any agenda the grid renders needs that agenda's rows.
	//
	// TWO callers, because they do not see the same thing (verified, see the
	// comments inside each section): the client caller renders both agendas, and
	// the staff caller renders only their own — SchedulesRepo.FindByProfessionalAndDay
	// rewrites the professional id to the caller's own for staff (schedules.go:71),
	// and agendaCache.forDay discards a row whose ProfessionalID does not match the
	// requested professional (find_available_slots_agenda.go). That second scoping
	// is out of scope here (S10/S11); it is documented so nobody mistakes this
	// subtest for a stronger guarantee than it gives.
	t.Run("unnamed path: the roster question stays role-neutral across professionals", func(t *testing.T) {
		seedUnnamedRosterFixture(t, conn, loc, monday)

		// Ties at the same start are broken by professional NAME
		// (sortAvailableSlots), and "Profesional Dos" sorts before "Profesional
		// Uno", so p2 precedes p1 inside each hour.
		type gridSlot struct {
			hour    int
			proID   string
			proName string
		}
		grid := func(hour int, proID, proName string) gridSlot { return gridSlot{hour, proID, proName} }

		// Both agendas, 09:00..16:00, minus the two taken slots.
		bothAgendas := []gridSlot{
			grid(9, "p2", "Profesional Dos"), grid(9, "p1", "Profesional Uno"),
			grid(10, "p2", "Profesional Dos"), // p1's 10:00 is taken by another client
			grid(11, "p1", "Profesional Uno"), // p2's 11:00 is taken by another client
			grid(12, "p2", "Profesional Dos"), grid(12, "p1", "Profesional Uno"),
			grid(13, "p2", "Profesional Dos"), grid(13, "p1", "Profesional Uno"),
			grid(14, "p2", "Profesional Dos"), grid(14, "p1", "Profesional Uno"),
			grid(15, "p2", "Profesional Dos"), grid(15, "p1", "Profesional Uno"),
			grid(16, "p2", "Profesional Dos"), grid(16, "p1", "Profesional Uno"),
		}
		// The staff caller's own agenda only, minus its taken slot.
		ownAgenda := []gridSlot{
			grid(9, "p1", "Profesional Uno"),
			grid(11, "p1", "Profesional Uno"),
			grid(12, "p1", "Profesional Uno"),
			grid(13, "p1", "Profesional Uno"),
			grid(14, "p1", "Profesional Uno"),
			grid(15, "p1", "Profesional Uno"),
			grid(16, "p1", "Profesional Uno"),
		}

		assertGrid := func(t *testing.T, callerID string, out dto.FindAvailableSlotsResult, want []gridSlot) {
			t.Helper()
			if out.TotalCandidates != len(want) || out.Truncated || len(out.Slots) != len(want) {
				t.Fatalf("caller %s: total_candidates=%d truncated=%v slots=%d; want %d/false/%d (got %v)",
					callerID, out.TotalCandidates, out.Truncated, len(out.Slots), len(want), len(want), slotStarts(out))
			}
			for index, expected := range want {
				wantStart := time.Date(monday.Year(), monday.Month(), monday.Day(), expected.hour, 0, 0, 0, loc)
				if !out.Slots[index].StartDatetime.Equal(wantStart) {
					t.Errorf("caller %s: slot[%d].start = %s; want %s", callerID, index, out.Slots[index].StartDatetime, wantStart)
				}
				if out.Slots[index].ProfessionalID != expected.proID {
					t.Errorf("caller %s: slot[%d].professional_id = %q; want %q", callerID, index, out.Slots[index].ProfessionalID, expected.proID)
				}
				if out.Slots[index].ProfessionalName != expected.proName {
					t.Errorf("caller %s: slot[%d].professional_name = %q; want %q", callerID, index, out.Slots[index].ProfessionalName, expected.proName)
				}
			}
		}

		// ── Section 1: a client caller renders BOTH agendas (the role-neutral read
		// across two professionals). This is the part of the subtest that fails on
		// the pre-fix wiring: with the caller-scoped range read, c1's snapshot held
		// only rows with client_id = 'c1' — and both bookings below belong to c2 — so
		// (10:00,p1) and (11:00,p2) both came back as FREE.
		t.Run("client caller sees both agendas with both taken slots absent", func(t *testing.T) {
			out := findAvailableSlotsUnnamedAs(t, mux, "c1", day)
			assertGrid(t, "c1", out, bothAgendas)
			if index := slotIndex(out, monday, loc, 10, "p1"); index >= 0 {
				t.Errorf("caller c1: slot[%d] = 10:00/p1; want ABSENT (taken in p1's agenda by another client)", index)
			}
			if index := slotIndex(out, monday, loc, 11, "p2"); index >= 0 {
				t.Errorf("caller c1: slot[%d] = 11:00/p2; want ABSENT (taken in p2's agenda by another client)", index)
			}
		})

		// ── Section 2: the staff caller of the report. staff-1 belongs to p1.
		// Since issue #259 the UNNAMED path is refused for staff: that is a stronger
		// form of the guarantee this section always pinned (a staff caller cannot
		// reach a foreign agenda), so it is asserted first. The reachable truth is
		// still pinned below through the NAMED call — own agenda only, own taken slot
		// hidden, free slots intact — so that a future un-scoping of the schedules
		// read cannot silently start publishing foreign agendas against a scoped
		// bookings read.
		t.Run("staff caller is refused unnamed and renders the own agenda when named", func(t *testing.T) {
			unnamed := fmt.Sprintf(
				`{"jsonrpc":"2.0","id":16,"method":"tools/call","params":{"name":"find_available_slots","arguments":{"service_id":"s1","from":%q,"to":%q}}}`,
				day, day)
			if _, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, mux, "staff-1", unnamed)); code == 0 {
				t.Fatalf("staff-1 received a grid with no professional_id; it must be refused (issue #259), got %q", msg)
			}

			out := findAvailableSlotsAs(t, mux, "staff-1", "p1", day)
			assertGrid(t, "staff-1", out, ownAgenda)
			if index := slotIndex(out, monday, loc, 10, "p1"); index >= 0 {
				t.Errorf("caller staff-1: slot[%d] = 10:00/p1; want ABSENT (taken in p1's agenda by another client)", index)
			}
			if index := slotIndex(out, monday, loc, 11, "p2"); index >= 0 {
				t.Errorf("caller staff-1: slot[%d] = 11:00/p2; want ABSENT (never offered as free)", index)
			}
			// Negative control: the genuinely free hours stay offered.
			for _, hour := range []int{9, 15} {
				if index := slotIndex(out, monday, loc, hour, "p1"); index < 0 {
					t.Errorf("caller staff-1: slot %02d:00/p1 is missing; want it offered (free slot over-blocked)", hour)
				}
			}
		})
	})
}
