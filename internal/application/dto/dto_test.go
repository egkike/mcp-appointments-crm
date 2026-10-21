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
	_ = CheckAvailabilityInput{}
	_ = CheckAvailabilityResult{}
	_ = GetBookingInput{}
	_ = GetBookingResult{}
	_ = BookingView{}
	_ = SearchClientsAdvancedInput{}
	_ = SearchClientsAdvancedResult{}
	_ = SearchServicesAdvancedInput{}
	_ = SearchServicesAdvancedResult{}
	_ = UpdateBusinessProfileInput{}
	_ = CreateServiceInput{}
	_ = UpdateServiceInput{}
	_ = DeleteServiceInput{}
	_ = DeleteServiceResult{}
	_ = CreateProfessionalInput{}
	_ = UpdateProfessionalInput{}
	_ = UpsertScheduleInput{}
	_ = DeleteScheduleInput{}
	_ = DeleteScheduleResult{}
}

// inputCarriers is the single registry of every '*Input' carrier in the
// package. Both guards (TestCallerFieldNotSerialized,
// TestInputCarriersAreWireFree) derive from it, so a NEW carrier only has to
// be added here — doc.go's contract (no wire tags, protective Caller guard)
// then applies to it automatically. This registry was itself the gap GGA V1
// caught on the batch PR: the two alerts carriers were initially missing.
var inputCarriers = []func(caller auth.Caller) any{
	func(c auth.Caller) any { return CreateBookingInput{Caller: c, ClientID: "c1"} },
	func(c auth.Caller) any { return CancelBookingInput{Caller: c, BookingID: "b1"} },
	func(c auth.Caller) any { return RescheduleBookingInput{Caller: c, BookingID: "b1"} },
	func(c auth.Caller) any { return CheckAvailabilityInput{Caller: c, ServiceID: "s1"} },
	func(c auth.Caller) any { return GetBookingInput{Caller: c, BookingID: "b1"} },
	func(c auth.Caller) any { return SearchClientsAdvancedInput{Caller: c, QueryText: "juan"} },
	func(c auth.Caller) any { return SearchServicesAdvancedInput{Caller: c, QueryText: "corte"} },
	func(c auth.Caller) any { return GetLoyaltyReportInput{Caller: c, Period: "month", TopN: nil} },
	// Maintenance inputs (ADR-0015) — owner-only write surface.
	func(c auth.Caller) any { return UpdateBusinessProfileInput{Caller: c} },
	func(c auth.Caller) any { return CreateServiceInput{Caller: c} },
	func(c auth.Caller) any { return UpdateServiceInput{Caller: c} },
	func(c auth.Caller) any { return DeleteServiceInput{Caller: c} },
	func(c auth.Caller) any { return CreateProfessionalInput{Caller: c} },
	func(c auth.Caller) any { return UpdateProfessionalInput{Caller: c} },
	func(c auth.Caller) any { return UpsertScheduleInput{Caller: c} },
	func(c auth.Caller) any { return DeleteScheduleInput{Caller: c} },
	// Alerts carriers (get_pending_alerts / mark_alert_as_sent).
	func(c auth.Caller) any { return GetPendingAlertsInput{Caller: c} },
	func(c auth.Caller) any { return MarkAlertAsSentInput{Caller: c, AlertID: 1} },
	// Hours-exception carriers (feat-hours-exception-tool).
	func(c auth.Caller) any { return CreateHoursExceptionInput{Caller: c, ExceptionDate: "2026-12-25"} },
	func(c auth.Caller) any { return DeleteHoursExceptionInput{Caller: c, ExceptionID: 1} },
	func(c auth.Caller) any {
		return ListHoursExceptionsInput{Caller: c, From: "2026-12-01", To: "2026-12-31"}
	},
}

// buildInputCarriers instantiates the registry for one caller value.
func buildInputCarriers(caller auth.Caller) []any {
	out := make([]any, 0, len(inputCarriers))
	for _, build := range inputCarriers {
		out = append(out, build(caller))
	}
	return out
}

// TestCallerFieldNotSerialized verifies that the Caller field on every Input
// DTO is excluded from JSON output (tagged json:"-"). This is a security
// contract: auth context must never leak into transport payloads.
func TestCallerFieldNotSerialized(t *testing.T) {
	caller := auth.Caller{ID: "caller-secret-id", Role: "admin"}
	inputs := buildInputCarriers(caller)
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
				{"ClientID", ""},
				{"ServiceID", ""},
				{"ProfessionalID", ""},
				{"StartTime", ""},
				{"Notes", ""},
				{"PaymentMethod", ""},
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
				{"BookingID", ""},
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
				{"BookingID", ""},
				{"NewStartTime", ""},
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
			name:   "CheckAvailabilityInput",
			target: CheckAvailabilityInput{},
			fields: []fieldSpec{
				{"Caller", "-"},
				{"ServiceID", ""},
				{"ProfessionalID", ""},
				{"StartDatetime", ""},
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
				{"BookingID", ""},
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
		{
			// The delete RESULT carriers are wire-free too, exactly like the
			// exception inputs below: the transport owns the response shapes
			// (deleteServiceOut / deleteScheduleOut /
			// deleteHoursExceptionOut), so no data-field tag survives here
			// (ADR-0013 boundary contract 4).
			name:   "DeleteServiceResult",
			target: DeleteServiceResult{},
			fields: []fieldSpec{
				{"ServiceID", ""},
				{"Status", ""},
			},
		},
		{
			name:   "DeleteScheduleResult",
			target: DeleteScheduleResult{},
			fields: []fieldSpec{
				{"ProfessionalID", ""},
				{"DayOfWeek", ""},
				{"Status", ""},
			},
		},
		{
			name:   "DeleteHoursExceptionResult",
			target: DeleteHoursExceptionResult{},
			fields: []fieldSpec{
				{"ExceptionID", ""},
				{"Status", ""},
			},
		},
		{
			// The exception input carriers carry NO data-field tags (the
			// transport adapter owns the wire shape); Caller keeps the
			// protective json:"-" (GGA r2 round on the dto commit).
			name:   "CreateHoursExceptionInput",
			target: CreateHoursExceptionInput{},
			fields: []fieldSpec{
				{"Caller", "-"},
				{"ExceptionDate", ""},
				{"IsClosed", ""},
				{"OpenTime", ""},
				{"CloseTime", ""},
				{"Reason", ""},
			},
		},
		{
			name:   "DeleteHoursExceptionInput",
			target: DeleteHoursExceptionInput{},
			fields: []fieldSpec{
				{"Caller", "-"},
				{"ExceptionID", ""},
			},
		},
		{
			name:   "ListHoursExceptionsInput",
			target: ListHoursExceptionsInput{},
			fields: []fieldSpec{
				{"Caller", "-"},
				{"From", ""},
				{"To", ""},
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

	t.Run("DeleteServiceResult", func(t *testing.T) {
		original := DeleteServiceResult{ServiceID: "sv-9", Status: "deleted"}
		assertRoundTrip(t, original, func(decoded DeleteServiceResult) {
			if decoded.ServiceID != original.ServiceID {
				t.Errorf("ServiceID = %q, want %q", decoded.ServiceID, original.ServiceID)
			}
			if decoded.Status != original.Status {
				t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
			}
		})
	})

	t.Run("DeleteScheduleResult", func(t *testing.T) {
		original := DeleteScheduleResult{ProfessionalID: "pr-3", DayOfWeek: 3, Status: "deleted"}
		assertRoundTrip(t, original, func(decoded DeleteScheduleResult) {
			if decoded.ProfessionalID != original.ProfessionalID {
				t.Errorf("ProfessionalID = %q, want %q", decoded.ProfessionalID, original.ProfessionalID)
			}
			if decoded.DayOfWeek != original.DayOfWeek {
				t.Errorf("DayOfWeek = %d, want %d", decoded.DayOfWeek, original.DayOfWeek)
			}
			if decoded.Status != original.Status {
				t.Errorf("Status = %q, want %q", decoded.Status, original.Status)
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

// TestInputCarriersAreWireFree pins the dto↔transport boundary convention
// (see the package doc): every '*Input' carrier carries no wire tags for its
// data fields; the transport '*In' structs stay the single source of request
// shapes. Only marker guards (`json:"-"`) are allowed (Caller on every
// carrier; presence/clear markers where they exist). A future carrier that
// declares a data-field payload tag fails here.
func TestInputCarriersAreWireFree(t *testing.T) {
	inputs := buildInputCarriers(auth.Caller{ID: "x"})
	for _, input := range inputs {
		t.Run(strings.ReplaceAll(reflect.TypeOf(input).String(), "dto.", ""), func(t *testing.T) {
			typ := reflect.TypeOf(input)
			for i := 0; i < typ.NumField(); i++ {
				f := typ.Field(i)
				tag := f.Tag.Get("json")
				switch {
				case f.Name == "Caller":
					if tag != "-" {
						t.Errorf("Caller: json tag = %q, want exactly %q (protective guard)", tag, "-")
					}
				case tag == "" || tag == "-":
					// wire-free field or a marker guard: convention holds
				default:
					t.Errorf("%s: json tag %q declared on the INPUT carrier: INPUT carriers are wire-free; the shape belongs to the transport '*In' struct", f.Name, tag)
				}
			}
		})
	}
}
