package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// ── mock ports (fn-table, same pattern as the use case tests) ──

type mockCheckAvailabilityPort struct {
	executeFn func(ctx context.Context, in dto.CheckAvailabilityInput) (*dto.CheckAvailabilityResult, error)
}

func (m *mockCheckAvailabilityPort) Execute(ctx context.Context, in dto.CheckAvailabilityInput) (*dto.CheckAvailabilityResult, error) {
	return m.executeFn(ctx, in)
}

type mockCreateBookingPort struct {
	executeFn func(ctx context.Context, in dto.CreateBookingInput) (*dto.CreateBookingResult, error)
}

func (m *mockCreateBookingPort) Execute(ctx context.Context, in dto.CreateBookingInput) (*dto.CreateBookingResult, error) {
	return m.executeFn(ctx, in)
}

type mockGetBookingPort struct {
	executeFn func(ctx context.Context, in dto.GetBookingInput) (*dto.GetBookingResult, error)
}

func (m *mockGetBookingPort) Execute(ctx context.Context, in dto.GetBookingInput) (*dto.GetBookingResult, error) {
	return m.executeFn(ctx, in)
}

type mockCancelBookingPort struct {
	executeFn func(ctx context.Context, in dto.CancelBookingInput) (*dto.CancelBookingResult, error)
}

func (m *mockCancelBookingPort) Execute(ctx context.Context, in dto.CancelBookingInput) (*dto.CancelBookingResult, error) {
	return m.executeFn(ctx, in)
}

type mockRescheduleBookingPort struct {
	executeFn func(ctx context.Context, in dto.RescheduleBookingInput) (*dto.RescheduleBookingResult, error)
}

func (m *mockRescheduleBookingPort) Execute(ctx context.Context, in dto.RescheduleBookingInput) (*dto.RescheduleBookingResult, error) {
	return m.executeFn(ctx, in)
}

type mockBusinessProfilePort struct {
	executeFn func(ctx context.Context) (*entity.BusinessProfile, error)
}

func (m *mockBusinessProfilePort) Execute(ctx context.Context) (*entity.BusinessProfile, error) {
	return m.executeFn(ctx)
}

type mockSearchClientsAdvancedPort struct {
	executeFn func(ctx context.Context, in dto.SearchClientsAdvancedInput) (*dto.SearchClientsAdvancedResult, error)
}

func (m *mockSearchClientsAdvancedPort) Execute(ctx context.Context, in dto.SearchClientsAdvancedInput) (*dto.SearchClientsAdvancedResult, error) {
	return m.executeFn(ctx, in)
}

type mockSearchServicesAdvancedPort struct {
	executeFn func(ctx context.Context, in dto.SearchServicesAdvancedInput) (*dto.SearchServicesAdvancedResult, error)
}

func (m *mockSearchServicesAdvancedPort) Execute(ctx context.Context, in dto.SearchServicesAdvancedInput) (*dto.SearchServicesAdvancedResult, error) {
	return m.executeFn(ctx, in)
}

type mockGetPendingAlertsPort struct {
	executeFn func(ctx context.Context, in dto.GetPendingAlertsInput) (*dto.GetPendingAlertsResult, error)
}

func (m *mockGetPendingAlertsPort) Execute(ctx context.Context, in dto.GetPendingAlertsInput) (*dto.GetPendingAlertsResult, error) {
	return m.executeFn(ctx, in)
}

type mockMarkAlertAsSentPort struct {
	executeFn func(ctx context.Context, in dto.MarkAlertAsSentInput) (*dto.MarkAlertAsSentResult, error)
}

func (m *mockMarkAlertAsSentPort) Execute(ctx context.Context, in dto.MarkAlertAsSentInput) (*dto.MarkAlertAsSentResult, error) {
	return m.executeFn(ctx, in)
}

type mockGetLoyaltyReportPort struct {
	executeFn func(ctx context.Context, in dto.GetLoyaltyReportInput) (*dto.GetLoyaltyReportResult, error)
}

func (m *mockGetLoyaltyReportPort) Execute(ctx context.Context, in dto.GetLoyaltyReportInput) (*dto.GetLoyaltyReportResult, error) {
	return m.executeFn(ctx, in)
}

// ── helpers ──

func ownerCaller() auth.Caller {
	return auth.Caller{ID: "owner-1", Role: auth.RoleOwner}
}

func ownerCallerPtr() *auth.Caller {
	c := ownerCaller()
	return &c
}

// newToolServer builds a Server with all eleven ports mocked and returns its
// unauthenticated Handler (unit level: caller is injected via the request
// context, exactly as AuthMiddleware would).
func newToolServer(t *testing.T) (*Server, *mockToolPorts) {
	t.Helper()
	ports := newMockPorts()
	srv := NewServer(Config{
		Version:                "test",
		Logger:                 discardLogger(),
		CheckAvailability:      ports.checkAvail,
		CreateBooking:          ports.create,
		GetBooking:             ports.get,
		CancelBooking:          ports.cancel,
		RescheduleBooking:      ports.reschedule,
		GetBusinessProfile:     ports.profile,
		SearchClientsAdvanced:  ports.searchClients,
		SearchServicesAdvanced: ports.searchServices,
		GetPendingAlerts:       ports.getPendingAlerts,
		MarkAlertAsSent:        ports.markAlertAsSent,
		GetLoyaltyReport:       ports.loyalty,
	})
	return srv, ports
}

type mockToolPorts struct {
	checkAvail       *mockCheckAvailabilityPort
	create           *mockCreateBookingPort
	get              *mockGetBookingPort
	cancel           *mockCancelBookingPort
	reschedule       *mockRescheduleBookingPort
	profile          *mockBusinessProfilePort
	searchClients    *mockSearchClientsAdvancedPort
	searchServices   *mockSearchServicesAdvancedPort
	getPendingAlerts *mockGetPendingAlertsPort
	markAlertAsSent  *mockMarkAlertAsSentPort
	loyalty          *mockGetLoyaltyReportPort
}

func newMockPorts() *mockToolPorts {
	return &mockToolPorts{
		checkAvail:       &mockCheckAvailabilityPort{},
		create:           &mockCreateBookingPort{},
		get:              &mockGetBookingPort{},
		cancel:           &mockCancelBookingPort{},
		reschedule:       &mockRescheduleBookingPort{},
		profile:          &mockBusinessProfilePort{},
		searchClients:    &mockSearchClientsAdvancedPort{},
		searchServices:   &mockSearchServicesAdvancedPort{},
		getPendingAlerts: &mockGetPendingAlertsPort{},
		markAlertAsSent:  &mockMarkAlertAsSentPort{},
		loyalty:          &mockGetLoyaltyReportPort{},
	}
}

// callTool performs a tools/call request through h. A nil caller leaves the
// context without a Caller (defensive path).
func callTool(h http.Handler, caller *auth.Caller, name, args string) *httptest.ResponseRecorder {
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, name, args)
	ctx := context.Background()
	if caller != nil {
		ctx = auth.WithCaller(ctx, *caller)
	}
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func callMethod(h http.Handler, method, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// toolResponse models the JSON-RPC response envelope of a tools/call.
type toolResponse struct {
	Result *struct {
		StructuredContent json.RawMessage `json:"structuredContent"`
		Content           []struct {
			Text string `json:"text"`
		} `json:"content"`
	} `json:"result"`
	Error *struct {
		Code    int64  `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func decodeToolResponse(t *testing.T, rec *httptest.ResponseRecorder) *toolResponse {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", rec.Code, rec.Body.String())
	}
	var resp toolResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("response is not JSON: %v; body=%s", err, rec.Body.String())
	}
	return &resp
}

func wantErrorCode(t *testing.T, resp *toolResponse, code int64) {
	t.Helper()
	if resp.Error == nil {
		t.Fatalf("expected JSON-RPC error %d, got result: %s", code, mustJSON(t, resp.Result))
	}
	if resp.Error.Code != code {
		t.Errorf("error.code = %d; want %d (msg=%q)", resp.Error.Code, code, resp.Error.Message)
	}
}

// wantStructured returns the tool output as raw JSON, failing the test if the
// SDK did not emit structuredContent.
func wantStructured(t *testing.T, resp *toolResponse) json.RawMessage {
	t.Helper()
	if resp.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: code=%d msg=%q", resp.Error.Code, resp.Error.Message)
	}
	if resp.Result == nil || len(resp.Result.StructuredContent) == 0 {
		t.Fatalf("missing structuredContent; result=%s", mustJSON(t, resp.Result))
	}
	return resp.Result.StructuredContent
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func fixedTime() time.Time {
	return time.Date(2026, 8, 3, 10, 0, 0, 0, time.UTC)
}

// ── tools/list exposes exactly the eleven registered tools (8 + 2 alerts + 1 loyalty) ──

func TestToolsListElevenTools(t *testing.T) {
	srv, _ := newToolServer(t)
	rec := callMethod(srv.Handler(), "tools/list", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`)

	var resp struct {
		Result struct {
			Tools []struct {
				Name string `json:"name"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body.String())
	}
	want := []string{"check_availability", "create_booking", "get_booking", "cancel_booking", "reschedule_booking", "get_business_profile", "search_clients_advanced", "search_services_advanced", "get_pending_alerts", "mark_alert_as_sent", "get_loyalty_report"}
	if len(resp.Result.Tools) != len(want) {
		t.Fatalf("tools/list returned %d tools; want %d: %s", len(resp.Result.Tools), len(want), mustJSON(t, resp.Result.Tools))
	}
	// The SDK serves tools sorted by name; compare as a set.
	got := make(map[string]bool, len(resp.Result.Tools))
	for _, tool := range resp.Result.Tools {
		got[tool.Name] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("tools/list missing %q; got %s", w, mustJSON(t, resp.Result.Tools))
		}
	}
}

// ── check_availability ──

func TestToolCheckAvailability(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.checkAvail.executeFn = func(ctx context.Context, in dto.CheckAvailabilityInput) (*dto.CheckAvailabilityResult, error) {
		if in.Caller.ID != "owner-1" || in.Caller.Role != auth.RoleOwner {
			t.Errorf("input.Caller = %+v; want owner-1/owner (REQ-MT-007 propagation)", in.Caller)
		}
		if in.ServiceID != "s1" || in.ProfessionalID != "p1" {
			t.Errorf("input = %+v; want s1/p1", in)
		}
		if !in.StartDatetime.Equal(fixedTime()) {
			t.Errorf("StartDatetime = %v; want %v", in.StartDatetime, fixedTime())
		}
		return &dto.CheckAvailabilityResult{Available: true}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "check_availability",
		`{"service_id":"s1","professional_id":"p1","start_datetime":"2026-08-03T10:00:00Z"}`))

	var out dto.CheckAvailabilityResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if !out.Available {
		t.Error("available = false; want true")
	}
}

// ── create_booking (REQ-MT-015 output contract: booking window included) ──

func TestToolCreateBooking(t *testing.T) {
	srv, ports := newToolServer(t)
	wantStart := fixedTime()
	wantEnd := fixedTime().Add(45 * time.Minute)
	ports.create.executeFn = func(ctx context.Context, in dto.CreateBookingInput) (*dto.CreateBookingResult, error) {
		if in.Caller.ID != "owner-1" {
			t.Errorf("input.Caller = %+v; want owner-1", in.Caller)
		}
		if in.ClientID != "c1" || in.ServiceID != "s1" || in.ProfessionalID != "p1" {
			t.Errorf("input = %+v; want c1/s1/p1", in)
		}
		if !in.StartTime.Equal(wantStart) {
			t.Errorf("StartTime = %v; want %v", in.StartTime, wantStart)
		}
		return &dto.CreateBookingResult{BookingID: "b-new", StartDatetime: wantStart, EndDatetime: wantEnd}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_booking",
		`{"client_id":"c1","service_id":"s1","professional_id":"p1","start_datetime":"2026-08-03T10:00:00Z"}`))

	var out dto.CreateBookingResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.BookingID != "b-new" {
		t.Errorf("booking_id = %q; want b-new", out.BookingID)
	}
	if !out.StartDatetime.Equal(wantStart) || !out.EndDatetime.Equal(wantEnd) {
		t.Errorf("window = %v..%v; want %v..%v", out.StartDatetime, out.EndDatetime, wantStart, wantEnd)
	}
}

// ── get_booking ──

func TestToolGetBooking(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.get.executeFn = func(ctx context.Context, in dto.GetBookingInput) (*dto.GetBookingResult, error) {
		if in.BookingID != "b1" {
			t.Errorf("BookingID = %q; want b1", in.BookingID)
		}
		return &dto.GetBookingResult{Booking: dto.BookingView{
			ID: "b1", ClientID: "c1", ProfessionalID: "p1", ServiceID: "s1",
			StartDatetime: fixedTime(), EndDatetime: fixedTime().Add(30 * time.Minute),
			Status: "pending",
		}}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_booking", `{"booking_id":"b1"}`))

	var out dto.GetBookingResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Booking.ID != "b1" || out.Booking.Status != "pending" || out.Booking.ClientID != "c1" {
		t.Errorf("booking = %+v; want b1/pending/c1", out.Booking)
	}
}

// ── cancel_booking ──

func TestToolCancelBooking(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.cancel.executeFn = func(ctx context.Context, in dto.CancelBookingInput) (*dto.CancelBookingResult, error) {
		if in.BookingID != "b1" {
			t.Errorf("BookingID = %q; want b1", in.BookingID)
		}
		return &dto.CancelBookingResult{BookingID: "b1", Status: "cancelled"}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "cancel_booking",
		`{"booking_id":"b1","reason":"cliente no asistió"}`))

	var out dto.CancelBookingResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.BookingID != "b1" || out.Status != "cancelled" {
		t.Errorf("out = %+v; want b1/cancelled", out)
	}
}

// ── reschedule_booking (REQ-MT-015 output contract: booking window included) ──

func TestToolRescheduleBooking(t *testing.T) {
	srv, ports := newToolServer(t)
	wantStart := fixedTime().Add(2 * time.Hour)
	wantEnd := wantStart.Add(60 * time.Minute)
	ports.reschedule.executeFn = func(ctx context.Context, in dto.RescheduleBookingInput) (*dto.RescheduleBookingResult, error) {
		if in.BookingID != "b1" {
			t.Errorf("BookingID = %q; want b1", in.BookingID)
		}
		if !in.NewStartTime.Equal(wantStart) {
			t.Errorf("NewStartTime = %v; want %v", in.NewStartTime, wantStart)
		}
		return &dto.RescheduleBookingResult{BookingID: "b1", Status: "pending", StartDatetime: wantStart, EndDatetime: wantEnd}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "reschedule_booking",
		`{"booking_id":"b1","new_start_datetime":"2026-08-03T12:00:00Z"}`))

	var out dto.RescheduleBookingResult
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.BookingID != "b1" || out.Status != "pending" {
		t.Errorf("out = %+v; want b1/pending", out)
	}
	if !out.StartDatetime.Equal(wantStart) || !out.EndDatetime.Equal(wantEnd) {
		t.Errorf("window = %v..%v; want %v..%v", out.StartDatetime, out.EndDatetime, wantStart, wantEnd)
	}
}

// ── get_business_profile ──

func TestToolGetBusinessProfile(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.profile.executeFn = func(ctx context.Context) (*entity.BusinessProfile, error) {
		return &entity.BusinessProfile{ID: "singleton", Name: "Mi Negocio", Timezone: "America/Argentina/Buenos_Aires"}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_business_profile", `{}`))

	var out businessProfileOut
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}
	if out.Name != "Mi Negocio" || out.Timezone != "America/Argentina/Buenos_Aires" {
		t.Errorf("profile = %+v; want Mi Negocio + tz", out)
	}
}

// ── get_business_profile output contract pinned (JD fix B-3) ──

func TestToolGetBusinessProfileOutputKeysPinned(t *testing.T) {
	srv, ports := newToolServer(t)
	// Every field populated, including all optional ones, so the emitted
	// JSON carries the FULL contract key set.
	ports.profile.executeFn = func(context.Context) (*entity.BusinessProfile, error) {
		return &entity.BusinessProfile{
			ID:                     "singleton",
			Name:                   "Mi Negocio",
			Industry:               strPtr("salud"),
			Country:                strPtr("AR"),
			Address:                strPtr("Av. Siempre Viva 742"),
			Latitude:               f64Ptr(-34.6037),
			Longitude:              f64Ptr(-58.3816),
			CoverPhotoURL:          strPtr("https://example.com/cover.jpg"),
			PublicPhone:            strPtr("+5491100000000"),
			MessengerPlatform:      strPtr("whatsapp"),
			MessengerID:            strPtr("5491100000000"),
			ContactEmail:           strPtr("hola@minegocio.com"),
			WebsiteURL:             strPtr("https://minegocio.com"),
			GeneralDescription:     strPtr("Descripción"),
			CurrencyCode:           "ARS",
			CurrencySymbol:         "$",
			AcceptedPaymentMethods: strPtr(`["cash"]`),
			Timezone:               "America/Argentina/Buenos_Aires",
			SlotIntervalMinutes:    30,
			BusinessHours:          `{"1":{"open":"09:00","close":"18:00"}}`,
			CreatedAt:              "2026-01-01T00:00:00Z",
			UpdatedAt:              "2026-01-02T00:00:00Z",
		}, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_business_profile", `{}`))
	var out map[string]json.RawMessage
	if err := json.Unmarshal(wantStructured(t, resp), &out); err != nil {
		t.Fatalf("unmarshal output: %v", err)
	}

	// The REQ-MT-015 output contract: exactly this snake_case key set,
	// independent of entity.BusinessProfile's Go field names (JD fix B-3).
	wantKeys := []string{
		"id", "name", "industry", "country", "address", "latitude", "longitude",
		"cover_photo_url", "public_phone", "messenger_platform", "messenger_id",
		"contact_email", "website_url", "general_description", "currency_code",
		"currency_symbol", "accepted_payment_methods", "timezone",
		"slot_interval_minutes", "business_hours", "created_at", "updated_at",
	}
	if len(out) != len(wantKeys) {
		t.Fatalf("output has %d keys; want exactly %d: %s", len(out), len(wantKeys), mustJSON(t, out))
	}
	for _, k := range wantKeys {
		if _, ok := out[k]; !ok {
			t.Errorf("output missing key %q; got %s", k, mustJSON(t, out))
		}
	}
}

func strPtr(s string) *string { return &s }

func f64Ptr(f float64) *float64 { return &f }

// ── get_business_profile input contract (GGA W-1 closure) ──

// The SDK infers {"type":"object","additionalProperties":false} from the
// empty input struct, so any non-empty argument object is rejected with
// -32602 before the handler runs (REQ-MT-015 input contract is exactly {}).
func TestToolGetBusinessProfileRejectsArguments(t *testing.T) {
	srv, _ := newToolServer(t)

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_business_profile", `{"foo":1}`))
	wantErrorCode(t, resp, -32602)
}

// A port returning (nil, nil) must fail closed with a business not-found
// error, never a nil dereference (GGA S-1).
func TestToolGetBusinessProfileNilProfileFailsClosed(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.profile.executeFn = func(context.Context) (*entity.BusinessProfile, error) {
		return nil, nil
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "get_business_profile", `{}`))
	wantErrorCode(t, resp, -32002)
	if resp.Error.Message != "perfil del negocio no encontrado" {
		t.Errorf("error.message = %q; want %q", resp.Error.Message, "perfil del negocio no encontrado")
	}
}

// ── unknown tool answers -32601 before the SDK (REQ-MT-006) ──

func TestToolUnknownToolMethodNotFound(t *testing.T) {
	srv, _ := newToolServer(t)

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "no_such_tool", `{}`))
	wantErrorCode(t, resp, -32601)
}

// ── argument validation is delegated to the SDK (-32602) ──

func TestToolMissingRequiredArgInvalidParams(t *testing.T) {
	srv, _ := newToolServer(t)

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_booking",
		`{"service_id":"s1","professional_id":"p1","start_datetime":"2026-08-03T10:00:00Z"}`))
	wantErrorCode(t, resp, -32602)
}

func TestToolInvalidDatetimeInvalidParams(t *testing.T) {
	srv, _ := newToolServer(t)

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_booking",
		`{"client_id":"c1","service_id":"s1","professional_id":"p1","start_datetime":"not-a-date"}`))
	wantErrorCode(t, resp, -32602)
}

// ── transport-level bounds (GGA W-1) ──

func TestToolCreateBookingNotesTooLong(t *testing.T) {
	srv, _ := newToolServer(t)

	notes := strings.Repeat("a", maxNotesLen+1)
	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "create_booking",
		fmt.Sprintf(`{"client_id":"c1","service_id":"s1","professional_id":"p1","start_datetime":"2026-08-03T10:00:00Z","notes":%q}`, notes)))
	wantErrorCode(t, resp, -32002)
	if resp.Error.Message != "Notas excede el largo máximo" {
		t.Errorf("error.message = %q; want %q", resp.Error.Message, "Notas excede el largo máximo")
	}
}

// ── defensive path: no caller in context → semantic unauthenticated ──

func TestToolMissingCallerUnauthenticated(t *testing.T) {
	srv, _ := newToolServer(t)

	resp := decodeToolResponse(t, callTool(srv.Handler(), nil, "create_booking",
		`{"client_id":"c1","service_id":"s1","professional_id":"p1","start_datetime":"2026-08-03T10:00:00Z"}`))
	wantErrorCode(t, resp, -32002)
}

// ── error mapping through the typed handler (T-08 contract) ──

func TestToolSemanticErrorMapsToBusinessCode(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.cancel.executeFn = func(context.Context, dto.CancelBookingInput) (*dto.CancelBookingResult, error) {
		return nil, &domain.SemanticError{Code: domain.ErrCodeNotFound, Message: "reserva no encontrada"}
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "cancel_booking", `{"booking_id":"ghost","reason":"x"}`))
	wantErrorCode(t, resp, -32002)
	if resp.Error.Message != "reserva no encontrada" {
		t.Errorf("error.message = %q; want %q", resp.Error.Message, "reserva no encontrada")
	}
}

func TestToolInfraErrorMapsToInternal(t *testing.T) {
	srv, ports := newToolServer(t)
	ports.cancel.executeFn = func(context.Context, dto.CancelBookingInput) (*dto.CancelBookingResult, error) {
		return nil, errors.New("sqlite: disk I/O error")
	}

	resp := decodeToolResponse(t, callTool(srv.Handler(), ownerCallerPtr(), "cancel_booking", `{"booking_id":"b1","reason":"x"}`))
	wantErrorCode(t, resp, -32603)
}
