package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/usecase"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

// ── Integration: the production composition against a real SQLite file ──

// newIntegrationMux assembles the exact production composition (main.go
// order) on a temp-file SQLite database: repositories → use cases → tools →
// AuthMiddleware with the real RBAC map → AuthHandler, plus /healthz. This
// proves the transport contract end-to-end at the HTTP layer.
func newIntegrationMux(t *testing.T) http.Handler {
	t.Helper()
	dir := t.TempDir()
	database, err := db.NewDatabase(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("db.NewDatabase: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	seedIntegrationDB(t, database.Conn)

	bookingsRepo := repository.NewBookingsRepo(database.Conn)
	bizHoursExRepo := repository.NewBusinessHoursExceptionRepo(database.Conn)
	bizProfRepo := repository.NewBusinessProfileRepo(database.Conn)
	prosRepo := repository.NewProfessionalsRepo(database.Conn)
	schedulesRepo := repository.NewSchedulesRepo(database.Conn)
	servicesRepo := repository.NewServicesRepo(database.Conn)
	bookingValidator := service.NewBookingValidator()

	getBookingUC := usecase.NewGetBookingUseCase(bookingsRepo)
	cancelBookingUC := usecase.NewCancelBookingUseCase(bookingsRepo)
	createBookingUC := usecase.NewCreateBookingUseCase(
		bookingsRepo, servicesRepo, prosRepo, bizProfRepo, bizHoursExRepo, schedulesRepo, bookingValidator,
	)
	rescheduleBookingUC := usecase.NewRescheduleBookingUseCase(
		bookingsRepo, servicesRepo, prosRepo, bizProfRepo, bizHoursExRepo, schedulesRepo, bookingValidator,
	)
	availabilityChecker := service.NewAvailabilityService()
	availabilityDeps := service.AvailabilityDeps{
		Services:                servicesRepo,
		Professionals:           prosRepo,
		BusinessProfile:         bizProfRepo,
		BusinessHoursExceptions: bizHoursExRepo,
		Schedules:               schedulesRepo,
		Bookings:                bookingsRepo,
	}
	checkAvailabilityUC := usecase.NewCheckAvailabilityUseCase(availabilityChecker, availabilityDeps)
	getBusinessProfileUC := usecase.NewGetBusinessProfileUseCase(bizProfRepo)

	srv := NewServer(Config{
		Version:            "test",
		Logger:             discardLogger(),
		CheckAvailability:  checkAvailabilityUC,
		CreateBooking:      createBookingUC,
		GetBooking:         getBookingUC,
		CancelBooking:      cancelBookingUC,
		RescheduleBooking:  rescheduleBookingUC,
		GetBusinessProfile: getBusinessProfileUC,
	})
	resolver := auth.NewCallerResolver(database.Conn)
	rbac := auth.ToolRBAC{
		"create_booking":       {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"cancel_booking":       {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"reschedule_booking":   {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"get_booking":          {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff, auth.RoleClient},
		"get_business_profile": {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
	}
	authMW := auth.NewAuthMiddleware(resolver, rbac, discardLogger())

	mux := http.NewServeMux()
	mux.Handle("/healthz", Healthz("test"))
	mux.Handle("/mcp", srv.AuthHandler(authMW))
	return mux
}

// seedIntegrationDB inserts the minimal domain state for the happy path:
// a Monday-working professional with a 60-minute service, a full-week
// business (09:00-18:00, Buenos Aires), an owner account and a client.
func seedIntegrationDB(t *testing.T, conn *sql.DB) {
	t.Helper()
	businessHours := `{"1":{"open":"09:00","close":"18:00"},"2":{"open":"09:00","close":"18:00"},"3":{"open":"09:00","close":"18:00"},"4":{"open":"09:00","close":"18:00"},"5":{"open":"09:00","close":"18:00"},"6":{"open":"09:00","close":"18:00"},"7":{"open":"09:00","close":"18:00"}}`
	rows := []string{
		`INSERT INTO business_profile (id, name, timezone, slot_interval_minutes, business_hours) VALUES ('singleton', 'Mi Negocio', 'America/Argentina/Buenos_Aires', 30, '` + businessHours + `')`,
		`INSERT INTO professionals (id, name, status) VALUES ('p1', 'Profesional Uno', 'active')`,
		`INSERT INTO services (id, name, duration_minutes, price, is_active) VALUES ('s1', 'Consulta', 60, 100.0, 1)`,
		`INSERT INTO schedules (professional_id, day_of_week, start_time, end_time) VALUES ('p1', 1, '09:00', '17:00')`,
		`INSERT INTO accounts (id, role, display_name, is_active) VALUES ('owner-1', 'owner', 'Owner', 1)`,
		`INSERT INTO clients (id, name, phone) VALUES ('c1', 'Cliente Uno', '+5491100000001')`,
	}
	for _, q := range rows {
		if _, err := conn.ExecContext(context.Background(), q); err != nil {
			t.Fatalf("seed %q: %v", q, err)
		}
	}
}

// postMCP performs a JSON-RPC request against /mcp with the given caller id
// header (empty = no header).
func postMCPCaller(t *testing.T, h http.Handler, callerID, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if callerID != "" {
		req.Header.Set("X-Caller-Id", callerID)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// decodeRPCEnvelope decodes a JSON-RPC response into its raw result/error.
func decodeRPCEnvelope(t *testing.T, rec *httptest.ResponseRecorder) (result json.RawMessage, code int64, msg string) {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200; body=%s", rec.Code, rec.Body.String())
	}
	var env struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int64  `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body.String())
	}
	if env.Error != nil {
		return nil, env.Error.Code, env.Error.Message
	}
	return env.Result, 0, ""
}

// ── happy path: initialize → tools/list → tools/call ──

func TestIntegrationHappyPath(t *testing.T) {
	mux := newIntegrationMux(t)

	// initialize handshake (REQ-MT-002: protocol 2025-11-25).
	rec := postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"0"}}}`)
	result, code, msg := decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("initialize failed: %d %q", code, msg)
	}
	var init struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	if err := json.Unmarshal(result, &init); err != nil {
		t.Fatalf("initialize result: %v", err)
	}
	if init.ProtocolVersion != "2025-11-25" {
		t.Errorf("protocolVersion = %q; want 2025-11-25", init.ProtocolVersion)
	}

	// tools/list exposes the six registered tools.
	rec = postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	result, code, msg = decodeRPCEnvelope(t, rec)
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
	if len(list.Tools) != 6 {
		t.Errorf("tools = %d; want 6: %s", len(list.Tools), string(result))
	}

	// check_availability on a valid Monday slot answers available:true
	// (Monday 2026-08-24 10:00 Buenos Aires, service 60min, schedule 09-17).
	rec = postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"check_availability","arguments":{"service_id":"s1","professional_id":"p1","start_datetime":"2026-08-24T10:00:00-03:00"}}}`)
	result, code, msg = decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("check_availability failed: %d %q", code, msg)
	}
	var out struct {
		StructuredContent struct {
			Available bool `json:"available"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &out); err != nil {
		t.Fatalf("check_availability result: %v", err)
	}
	if !out.StructuredContent.Available {
		t.Errorf("available = false; want true (valid Monday slot)")
	}
}

// ── auth failures map to JSON-RPC envelopes (REQ-AM-WIRED-002) ──

func TestIntegrationMissingCallerIDMapsToEnvelope(t *testing.T) {
	mux := newIntegrationMux(t)

	rec := postMCPCaller(t, mux, "", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"check_availability","arguments":{}}}`)
	_, code, msg := decodeRPCEnvelope(t, rec)
	if code != -32000 || msg != "no se proporcionó X-Caller-Id" {
		t.Errorf("got code=%d msg=%q; want -32000 %q", code, msg, "no se proporcionó X-Caller-Id")
	}
}

func TestIntegrationClientRoleForbidden(t *testing.T) {
	mux := newIntegrationMux(t)

	// A client may not create bookings (RBAC: owner/admin/staff only).
	rec := postMCPCaller(t, mux, "c1", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"create_booking","arguments":{"client_id":"c1","service_id":"s1","professional_id":"p1","start_datetime":"2026-08-24T10:00:00-03:00"}}}`)
	_, code, msg := decodeRPCEnvelope(t, rec)
	if code != -32001 || msg != "no tienes permiso para realizar esta acción" {
		t.Errorf("got code=%d msg=%q; want -32001 %q", code, msg, "no tienes permiso para realizar esta acción")
	}
}

// ── healthz regression (W-4): liveness-only, no DB dependency ──

func TestIntegrationHealthzLiveness(t *testing.T) {
	mux := newIntegrationMux(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	var body struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v; body=%s", err, rec.Body.String())
	}
	if body.Status != "ok" || body.Version != "test" {
		t.Errorf("body = %+v; want {ok test} (liveness-only)", body)
	}
}

// ── 413 carry-over: oversized body rejected by jsonParseGuard ──

func TestServerOversizedBodyRejected(t *testing.T) {
	srv := NewServer(Config{Version: "test", Logger: discardLogger()})
	body := strings.Repeat("a", maxRequestBodyBytes+1)
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/mcp", strings.NewReader(body))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("status = %d; want 413 (1 MiB body budget)", rec.Code)
	}
	if got := fmt.Sprintf("%d", rec.Code); got != "413" {
		t.Errorf("status code string = %s; want 413", got)
	}
}
