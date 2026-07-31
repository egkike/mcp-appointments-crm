package dto

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// TestDTOPackageCompiles is a smoke test that verifies every exported DTO
// type can be instantiated. If any type is removed or renamed, compilation
// breaks and this test catches the gap.
func TestDTOPackageCompiles(t *testing.T) {
	_ = CreateBookingInput{}
	_ = CreateBookingResult{}
	_ = CancelBookingInput{}
	_ = CancelBookingResult{}
	_ = RescheduleBookingInput{}
	_ = RescheduleBookingResult{}
	_ = CheckAvailabilityParams{}
	_ = CheckAvailabilityResult{}
	_ = GetBookingInput{}
	_ = GetBookingResult{}
	_ = BookingView{}
}

// TestCallerFieldNotSerialized verifies that the Caller field on every Input
// DTO is excluded from JSON output (tagged json:"-"). This is a security
// contract: auth context must never leak into transport payloads.
func TestCallerFieldNotSerialized(t *testing.T) {
	caller := auth.Caller{ID: "caller-secret-id", Role: "admin"}
	inputs := []any{
		CreateBookingInput{Caller: caller, ClientID: "c1"},
		CancelBookingInput{Caller: caller, BookingID: "b1"},
		RescheduleBookingInput{Caller: caller, BookingID: "b1"},
		CheckAvailabilityParams{Caller: caller, ServiceID: "s1"},
		GetBookingInput{Caller: caller, BookingID: "b1"},
	}
	for _, input := range inputs {
		data, err := json.Marshal(input)
		if err != nil {
			t.Fatalf("Marshal(%T): %v", input, err)
		}
		if strings.Contains(string(data), "caller-secret-id") {
			t.Errorf("Marshal(%T) leaked Caller ID into JSON: %s", input, data)
		}
	}
}

// fieldSpec describes an expected struct field and its JSON tag value.
type fieldSpec struct {
	name    string
	jsonTag string
}

// TestDTOFieldTags asserts that every exported DTO has the expected fields
// with the correct JSON tags. This catches accidental renames or tag drift.
func TestDTOFieldTags(t *testing.T) {
	tests := []struct {
		name   string
		target any
		fields []fieldSpec
	}{
		{
			name:   "CreateBookingInput",
			target: CreateBookingInput{},
			fields: []fieldSpec{
				{"Caller", "-"},
				{"ClientID", "client_id"},
				{"ServiceID", "service_id"},
				{"ProfessionalID", "professional_id"},
				{"StartTime", "start_time"},
				{"Notes", "notes,omitempty"},
				{"PaymentMethod", "payment_method,omitempty"},
			},
		},
		{
			name:   "CreateBookingResult",
			target: CreateBookingResult{},
			fields: []fieldSpec{
				{"BookingID", "booking_id"},
			},
		},
		{
			name:   "CancelBookingInput",
			target: CancelBookingInput{},
			fields: []fieldSpec{
				{"Caller", "-"},
				{"BookingID", "booking_id"},
			},
		},
		{
			name:   "CancelBookingResult",
			target: CancelBookingResult{},
			fields: []fieldSpec{
				{"BookingID", "booking_id"},
				{"Status", "status"},
			},
		},
		{
			name:   "RescheduleBookingInput",
			target: RescheduleBookingInput{},
			fields: []fieldSpec{
				{"Caller", "-"},
				{"BookingID", "booking_id"},
				{"NewStartTime", "new_start_time"},
			},
		},
		{
			name:   "RescheduleBookingResult",
			target: RescheduleBookingResult{},
			fields: []fieldSpec{
				{"BookingID", "booking_id"},
				{"Status", "status"},
			},
		},
		{
			name:   "CheckAvailabilityParams",
			target: CheckAvailabilityParams{},
			fields: []fieldSpec{
				{"Caller", "-"},
				{"ServiceID", "service_id"},
				{"ProfessionalID", "professional_id"},
				{"StartDatetime", "start_datetime"},
			},
		},
		{
			name:   "CheckAvailabilityResult",
			target: CheckAvailabilityResult{},
			fields: []fieldSpec{
				{"Available", "available"},
			},
		},
		{
			name:   "GetBookingInput",
			target: GetBookingInput{},
			fields: []fieldSpec{
				{"Caller", "-"},
				{"BookingID", "booking_id"},
			},
		},
		{
			name:   "GetBookingResult",
			target: GetBookingResult{},
			fields: []fieldSpec{
				{"Booking", "booking"},
			},
		},
		{
			name:   "BookingView",
			target: BookingView{},
			fields: []fieldSpec{
				{"ID", "id"},
				{"ClientID", "client_id"},
				{"ProfessionalID", "professional_id"},
				{"ServiceID", "service_id"},
				{"StartDatetime", "start_datetime"},
				{"EndDatetime", "end_datetime"},
				{"Status", "status"},
				{"Notes", "notes,omitempty"},
				{"PaymentMethod", "payment_method,omitempty"},
				{"CreatedAt", "created_at"},
				{"UpdatedAt", "updated_at"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			typ := reflect.TypeOf(tt.target)
			for _, f := range tt.fields {
				field, ok := typ.FieldByName(f.name)
				if !ok {
					t.Errorf("field %s not found on %s", f.name, tt.name)
					continue
				}
				got := field.Tag.Get("json")
				if got != f.jsonTag {
					t.Errorf("%s.%s: json tag = %q, want %q", tt.name, f.name, got, f.jsonTag)
				}
			}
		})
	}
}

// TestResultJSONRoundTrip verifies that each Result DTO survives a JSON
// marshal → unmarshal cycle with all fields intact. This proves the DTOs
// are transport-ready for the MCP JSON-RPC boundary.
func TestResultJSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 7, 31, 14, 0, 0, 0, time.UTC)
	notes := "Bring documents"
	payment := "cash"

	t.Run("CreateBookingResult", func(t *testing.T) {
		original := CreateBookingResult{BookingID: "bk-001"}
		assertRoundTrip(t, original, func(decoded CreateBookingResult) {
			if decoded.BookingID != original.BookingID {
				t.Errorf("BookingID = %q, want %q", decoded.BookingID, original.BookingID)
			}
		})
	})

	t.Run("CancelBookingResult", func(t *testing.T) {
		original := CancelBookingResult{BookingID: "bk-002", Status: "cancelled"}
		assertRoundTrip(t, original, func(decoded CancelBookingResult) {
			if decoded.BookingID != original.BookingID {
				t.Errorf("BookingID = %q, want %q", decoded.BookingID, original.BookingID)
			}
			if decoded.Status != original.Status {
				t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
			}
		})
	})

	t.Run("RescheduleBookingResult", func(t *testing.T) {
		original := RescheduleBookingResult{BookingID: "bk-003", Status: "confirmed"}
		assertRoundTrip(t, original, func(decoded RescheduleBookingResult) {
			if decoded.BookingID != original.BookingID {
				t.Errorf("BookingID = %q, want %q", decoded.BookingID, original.BookingID)
			}
			if decoded.Status != original.Status {
				t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
			}
		})
	})

	t.Run("CheckAvailabilityResult_available", func(t *testing.T) {
		original := CheckAvailabilityResult{Available: true}
		assertRoundTrip(t, original, func(decoded CheckAvailabilityResult) {
			if decoded.Available != original.Available {
				t.Errorf("Available = %v, want %v", decoded.Available, original.Available)
			}
		})
	})

	t.Run("CheckAvailabilityResult_unavailable", func(t *testing.T) {
		original := CheckAvailabilityResult{Available: false}
		assertRoundTrip(t, original, func(decoded CheckAvailabilityResult) {
			if decoded.Available != original.Available {
				t.Errorf("Available = %v, want %v", decoded.Available, original.Available)
			}
		})
	})

	t.Run("GetBookingResult", func(t *testing.T) {
		original := GetBookingResult{
			Booking: BookingView{
				ID:             "bk-004",
				ClientID:       "cl-10",
				ProfessionalID: "pr-5",
				ServiceID:      "sv-2",
				StartDatetime:  now,
				EndDatetime:    now.Add(60 * time.Minute),
				Status:         "confirmed",
				Notes:          &notes,
				PaymentMethod:  &payment,
				CreatedAt:      now.Add(-24 * time.Hour),
				UpdatedAt:      now.Add(-1 * time.Hour),
			},
		}
		assertRoundTrip(t, original, func(decoded GetBookingResult) {
			b := decoded.Booking
			if b.ID != original.Booking.ID {
				t.Errorf("ID = %q, want %q", b.ID, original.Booking.ID)
			}
			if b.ClientID != original.Booking.ClientID {
				t.Errorf("ClientID = %q, want %q", b.ClientID, original.Booking.ClientID)
			}
			if b.ProfessionalID != original.Booking.ProfessionalID {
				t.Errorf("ProfessionalID = %q, want %q", b.ProfessionalID, original.Booking.ProfessionalID)
			}
			if b.ServiceID != original.Booking.ServiceID {
				t.Errorf("ServiceID = %q, want %q", b.ServiceID, original.Booking.ServiceID)
			}
			if !b.StartDatetime.Equal(original.Booking.StartDatetime) {
				t.Errorf("StartDatetime = %v, want %v", b.StartDatetime, original.Booking.StartDatetime)
			}
			if !b.EndDatetime.Equal(original.Booking.EndDatetime) {
				t.Errorf("EndDatetime = %v, want %v", b.EndDatetime, original.Booking.EndDatetime)
			}
			if b.Status != original.Booking.Status {
				t.Errorf("Status = %q, want %q", b.Status, original.Booking.Status)
			}
			if b.Notes == nil || *b.Notes != notes {
				t.Errorf("Notes = %v, want %q", b.Notes, notes)
			}
			if b.PaymentMethod == nil || *b.PaymentMethod != payment {
				t.Errorf("PaymentMethod = %v, want %q", b.PaymentMethod, payment)
			}
			if !b.CreatedAt.Equal(original.Booking.CreatedAt) {
				t.Errorf("CreatedAt = %v, want %v", b.CreatedAt, original.Booking.CreatedAt)
			}
			if !b.UpdatedAt.Equal(original.Booking.UpdatedAt) {
				t.Errorf("UpdatedAt = %v, want %v", b.UpdatedAt, original.Booking.UpdatedAt)
			}
		})
	})
}

// assertRoundTrip marshals v to JSON, unmarshals into a new T, and calls
// the check callback with the decoded value.
func assertRoundTrip[T any](t *testing.T, v T, check func(T)) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var decoded T
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	check(decoded)
}
