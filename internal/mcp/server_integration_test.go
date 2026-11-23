package mcp

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/application/usecase"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/config"
	"github.com/egkike/mcp-appointments-crm/internal/db"
	"github.com/egkike/mcp-appointments-crm/internal/domain/service"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

// ── Integration: the production composition against a real SQLite file ──

// integrationOptions parameterizes the shared production composition the
// integration suite runs (newIntegrationMuxWithOptions).
type integrationOptions struct {
	// registrationLimit is the per-phone auto-registration cap the composition
	// root reads from MCP_REGISTRATION_RATE_LIMIT. 0 disables auto-registration
	// entirely (fail closed, design §1.5).
	registrationLimit int
}

// newIntegrationMux assembles the exact production composition (main.go
// order) on a temp-file SQLite database: repositories → use cases → tools →
// AuthMiddleware with the real RBAC map → AuthHandler, plus /healthz. This
// proves the transport contract end-to-end at the HTTP layer.
func newIntegrationMux(t *testing.T) http.Handler {
	t.Helper()
	mux, _ := newIntegrationMuxWithDB(t)
	return mux
}

// newIntegrationMuxWithDB is like newIntegrationMux but also returns the
// underlying database connection so tests can seed extra state.
func newIntegrationMuxWithDB(t *testing.T) (http.Handler, *sql.DB) {
	t.Helper()
	return newIntegrationMuxWithOptions(t, integrationOptions{
		registrationLimit: config.DefaultRegistrationRateLimit,
	})
}

// newIntegrationMuxWithRegistrationLimit is like newIntegrationMux but wires the
// anonymous registration use case with a test-chosen per-phone limit (0 =
// auto-registration disabled) and returns the connection for the row assertions.
func newIntegrationMuxWithRegistrationLimit(t *testing.T, limit int) (http.Handler, *sql.DB) {
	t.Helper()
	return newIntegrationMuxWithOptions(t, integrationOptions{registrationLimit: limit})
}

// newIntegrationMuxWithOptions is the parameterized composition behind the two
// helpers above: same wiring as the composition root, at the HTTP layer.
func newIntegrationMuxWithOptions(t *testing.T, opts integrationOptions) (http.Handler, *sql.DB) {
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
	clientsRepo := repository.NewClientsRepo(database.Conn)
	accountsRepo := repository.NewAccountsRepo(database.Conn, discardLogger())
	pendingAlertsRepo := repository.NewPendingAlertsRepo(database.Conn)
	bookingValidator := service.NewBookingValidator()
	alertStore := usecase.NewEnsurePendingAlertsRepo(pendingAlertsRepo)

	getBookingUC := usecase.NewGetBookingUseCase(bookingsRepo)
	cancelBookingUC := usecase.NewCancelBookingUseCase(bookingsRepo, alertStore, nil)
	createBookingUC := usecase.NewCreateBookingUseCase(
		bookingsRepo, servicesRepo, prosRepo, bizProfRepo, bizHoursExRepo, schedulesRepo, clientsRepo, bookingValidator, alertStore, nil,
	)
	rescheduleBookingUC := usecase.NewRescheduleBookingUseCase(
		bookingsRepo, servicesRepo, prosRepo, bizProfRepo, bizHoursExRepo, schedulesRepo, clientsRepo, bookingValidator, alertStore, nil,
	)
	availabilityChecker := service.NewAvailabilityService()
	// Issue #243: availability reads bookings through the role-neutral adapter,
	// exactly like the composition root. Using the caller-scoped bookingsRepo here
	// would silently reintroduce the defect the integration suite guards.
	availabilityBookings := repository.NewAvailabilityBookingsReader(database.Conn)
	availabilityDeps := service.AvailabilityDeps{
		Services:                servicesRepo,
		Professionals:           prosRepo,
		BusinessProfile:         bizProfRepo,
		BusinessHoursExceptions: bizHoursExRepo,
		Schedules:               schedulesRepo,
		Bookings:                availabilityBookings,
	}
	checkAvailabilityUC := usecase.NewCheckAvailabilityUseCase(availabilityChecker, availabilityDeps)
	// agent-surface T2: the date-range slot search reads the availability adapter
	// for its single range read (one read for the whole range, the shared chain
	// per slot).
	findAvailableSlotsUC := usecase.NewFindAvailableSlotsUseCase(
		servicesRepo, prosRepo, bizProfRepo, bizHoursExRepo, schedulesRepo, availabilityBookings,
	)
	getBusinessProfileUC := usecase.NewGetBusinessProfileUseCase(bizProfRepo)

	// Anonymous self-registration (feat-whatsapp-bot): the same auth-free
	// RegistrationLookup adapter and per-phone limiter the composition root
	// wires, with the limit chosen by the test harness.
	registerClientUC := usecase.NewGetOrCreateClientUseCase(
		clientsRepo,
		repository.NewRegistrationLookup(accountsRepo, clientsRepo),
		usecase.NewRegistrationRateLimiter(opts.registrationLimit),
		discardLogger(),
	)

	searchClientsAdvancedUC := usecase.NewSearchClientsAdvancedUseCase(clientsRepo)
	searchServicesAdvancedUC := usecase.NewSearchServicesAdvancedUseCase(servicesRepo)
	// feat-owner-chat S7a: composed exactly like the composition root, including
	// the shared query validator adapter (one text policy for every *_advanced tool).
	searchProfessionalsAdvancedUC := usecase.NewSearchProfessionalsAdvancedUseCase(
		prosRepo, servicesRepo, repository.QueryValidator{},
	)
	getPendingAlertsUC := usecase.NewGetPendingAlertsUseCase(pendingAlertsRepo)
	markAlertAsSentUC := usecase.NewMarkAlertAsSentUseCase(pendingAlertsRepo)
	getLoyaltyReportUC := usecase.NewGetLoyaltyReportUseCase(bookingsRepo)

	// Maintenance WRITE use cases (ADR-0015), composed exactly like the
	// composition root: shared real repositories + a discard logger (the use
	// cases emit one structured audit record per mutation).
	updateBusinessProfileUC := usecase.NewUpdateBusinessProfileUseCase(bizProfRepo, discardLogger())
	createServiceUC := usecase.NewCreateServiceUseCase(servicesRepo, discardLogger())
	updateServiceUC := usecase.NewUpdateServiceUseCase(servicesRepo, discardLogger())
	deleteServiceUC := usecase.NewDeleteServiceUseCase(servicesRepo, discardLogger())
	createProfessionalUC := usecase.NewCreateProfessionalUseCase(prosRepo, discardLogger())
	updateProfessionalUC := usecase.NewUpdateProfessionalUseCase(prosRepo, discardLogger())
	upsertScheduleUC := usecase.NewUpsertScheduleUseCase(schedulesRepo, discardLogger())
	deleteScheduleUC := usecase.NewDeleteScheduleUseCase(schedulesRepo, discardLogger())
	// feat-owner-chat S7b: agenda read over the real repositories; roles widened
	// to owner/admin/staff by feat-staff-self-read S3.
	getProfessionalSchedulesUC := usecase.NewGetProfessionalSchedulesUseCase(prosRepo, schedulesRepo)
	// feat-staff-self-read S1: single-record read over the same repository, with
	// the scope and PII rules inside the use case.
	getProfessionalUC := usecase.NewGetProfessionalUseCase(prosRepo)

	// Business-hours exception use cases (feat-hours-exception-tool), composed
	// like the composition root over the shared exception repository.
	createHoursExceptionUC := usecase.NewCreateHoursExceptionUseCase(bizHoursExRepo, discardLogger())
	deleteHoursExceptionUC := usecase.NewDeleteHoursExceptionUseCase(bizHoursExRepo, discardLogger())
	listHoursExceptionsUC := usecase.NewListHoursExceptionsUseCase(bizHoursExRepo)

	// The 27-port Config literal is composed by hand here on purpose: the
	// integration suite wires REAL use cases over REAL repositories, while
	// configFromMockPorts (tools_test.go) only renders the mock bundle — this
	// literal is the composition-root mirror, not a mock variant. The compiler
	// forces field exhaustiveness at every Config construction; the
	// ToolCount/expectedToolCount registration checks keep guarding the count.
	// The harness consumes the SAME policy the composition root does
	// (ToolRoles, internal/mcp/tool_roles.go), for BOTH gates: authorization
	// and the tools/list discovery filter. It used to carry its own copy,
	// which meant a row edited in only one place passed every test (debt L11,
	// issue #249).
	rbac := ToolRoles()

	srv := NewServer(Config{
		Version:                "test",
		Logger:                 discardLogger(),
		ToolRBAC:               rbac,
		CheckAvailability:      checkAvailabilityUC,
		FindAvailableSlots:     findAvailableSlotsUC,
		CreateBooking:          createBookingUC,
		GetBooking:             getBookingUC,
		CancelBooking:          cancelBookingUC,
		RescheduleBooking:      rescheduleBookingUC,
		GetBusinessProfile:     getBusinessProfileUC,
		SearchClientsAdvanced:  searchClientsAdvancedUC,
		SearchServicesAdvanced: searchServicesAdvancedUC,

		SearchProfessionalsAdvanced: searchProfessionalsAdvancedUC,
		GetProfessional:             getProfessionalUC,
		GetProfessionalSchedules:    getProfessionalSchedulesUC,
		GetPendingAlerts:            getPendingAlertsUC,
		MarkAlertAsSent:             markAlertAsSentUC,
		GetLoyaltyReport:            getLoyaltyReportUC,
		RegisterClient:              registerClientUC,

		UpdateBusinessProfile: updateBusinessProfileUC,
		CreateService:         createServiceUC,
		UpdateService:         updateServiceUC,
		DeleteService:         deleteServiceUC,
		CreateProfessional:    createProfessionalUC,
		UpdateProfessional:    updateProfessionalUC,
		UpsertSchedule:        upsertScheduleUC,
		DeleteSchedule:        deleteScheduleUC,

		CreateHoursException: createHoursExceptionUC,
		DeleteHoursException: deleteHoursExceptionUC,
		ListHoursExceptions:  listHoursExceptionsUC,
	})
	resolver := auth.NewCallerResolver(database.Conn)
	authMW := auth.NewAuthMiddleware(resolver, rbac, discardLogger())

	mux := http.NewServeMux()
	mux.Handle("/healthz", Healthz("test"))
	mux.Handle("/mcp", srv.AuthHandler(authMW))
	return mux, database.Conn
}

// seedIntegrationDB inserts the minimal domain state for the happy path:
// a Monday-working professional with a 60-minute service, a full-week
// business (09:00-18:00, Buenos Aires), an owner, an admin and a staff account
// plus a client. The admin account needs no professional_id (schema CHECK:
// only the staff role requires one).
func seedIntegrationDB(t *testing.T, conn *sql.DB) {
	t.Helper()
	businessHours := `{"1":{"open":"09:00","close":"18:00"},"2":{"open":"09:00","close":"18:00"},"3":{"open":"09:00","close":"18:00"},"4":{"open":"09:00","close":"18:00"},"5":{"open":"09:00","close":"18:00"},"6":{"open":"09:00","close":"18:00"},"7":{"open":"09:00","close":"18:00"}}`
	rows := []string{
		`INSERT INTO business_profile (id, name, timezone, slot_interval_minutes, business_hours) VALUES ('singleton', 'Mi Negocio', 'America/Argentina/Buenos_Aires', 30, '` + businessHours + `')`,
		`INSERT INTO professionals (id, name, status) VALUES ('p1', 'Profesional Uno', 'active')`,
		`INSERT INTO services (id, name, description, duration_minutes, price, is_active) VALUES ('s1', 'Consulta', 'consulta general', 60, 100.0, 1)`,
		`INSERT INTO schedules (professional_id, day_of_week, start_time, end_time) VALUES ('p1', 1, '09:00', '17:00')`,
		`INSERT INTO accounts (id, role, display_name, is_active) VALUES ('owner-1', 'owner', 'Owner', 1)`,
		`INSERT INTO accounts (id, role, display_name, is_active) VALUES ('admin-1', 'admin', 'Admin', 1)`,
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES ('staff-1', 'staff', 'Staff', 'p1', 1)`,
		`INSERT INTO clients (id, name, phone) VALUES ('c1', 'Cliente Uno', '+5491100000001')`,
		`INSERT INTO clients (id, name, phone) VALUES ('c2', 'Cliente Dos', '+5491100000002')`,
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

	// tools/list exposes every registered tool; expectedToolCount pins the exact
	// number, not this comment (it drifted behind the surface twice: issue #249).
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
	if len(list.Tools) != expectedToolCount {
		t.Errorf("tools = %d; want %d: %s", len(list.Tools), expectedToolCount, string(result))
	}

	// search_clients_advanced by owner returns both seeded clients (no RBAC entry).
	rec = postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"search_clients_advanced","arguments":{"query_text":"Cliente"}}}`)
	result, code, msg = decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("search_clients_advanced failed: %d %q", code, msg)
	}
	var searchClients struct {
		StructuredContent struct {
			Results []dto.ClientSearchEntry `json:"results"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &searchClients); err != nil {
		t.Fatalf("search_clients_advanced result: %v", err)
	}
	if len(searchClients.StructuredContent.Results) != 2 {
		t.Errorf("client results = %d; want 2", len(searchClients.StructuredContent.Results))
	}

	// search_services_advanced by owner returns the seeded service (no RBAC entry).
	rec = postMCPCaller(t, mux, "owner-1", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"search_services_advanced","arguments":{"query_text":"Consulta"}}}`)
	result, code, msg = decodeRPCEnvelope(t, rec)
	if code != 0 {
		t.Fatalf("search_services_advanced failed: %d %q", code, msg)
	}
	var searchServices struct {
		StructuredContent struct {
			Results []dto.ServiceSearchEntry `json:"results"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &searchServices); err != nil {
		t.Fatalf("search_services_advanced result: %v", err)
	}
	if len(searchServices.StructuredContent.Results) != 1 {
		t.Errorf("service results = %d; want 1", len(searchServices.StructuredContent.Results))
	}
}

// ── auth failures map to JSON-RPC envelopes (REQ-AM-WIRED-002) ──

func TestIntegrationMissingCallerIDMapsToEnvelope(t *testing.T) {
	mux := newIntegrationMux(t)

	rec := postMCPCaller(t, mux, "", `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"check_availability","arguments":{}}}`)
	_, code, msg := decodeRPCEnvelope(t, rec)
	if code != -32000 || msg != msgAuthRequired {
		t.Errorf("got code=%d msg=%q; want -32000 %q", code, msg, msgAuthRequired)
	}
}

// TestIntegrationClientRoleForbiddenTools pins the boundary that SURVIVES issue
// #251: a client may operate its own bookings, but the gate must keep refusing it
// everywhere else. The previous version of this test asserted that create_booking
// itself was refused; that was the stale transport contract, and it is now
// replaced by TestIntegrationClientBooksOwnBooking, which proves the client's own
// path works AND that a foreign booking cannot be touched.
func TestIntegrationClientRoleForbiddenTools(t *testing.T) {
	mux := newIntegrationMux(t)

	// Map-level refusals: rows that do not list client at all. The message is the
	// middleware's own text, so a use-case refusal cannot satisfy this assertion.
	// get_business_profile left this list in issue #255: a client must be able to
	// read the business it books with. Maintenance writes and the two professionals
	// reads (get_professional_schedules since feat-staff-self-read S3 and
	// get_professional since S1, issue #276) stay.
	for _, tool := range []string{"create_service", "get_professional_schedules", "get_professional"} {
		t.Run(tool, func(t *testing.T) {
			rec := postMCPCaller(t, mux, "c1", `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"`+tool+`","arguments":{}}}`)
			_, code, msg := decodeRPCEnvelope(t, rec)
			if code != -32001 || msg != auth.MsgForbiddenTool {
				t.Errorf("got code=%d msg=%q; want -32001 %q", code, msg, auth.MsgForbiddenTool)
			}
		})
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

// TestToolSearchClientsAdvancedRejectsEmptyQuery runs the real mux (real SQLite
// repository) and lives HERE with the other integration coverage rather than
// inside the unit-tier search test file (R3-empty-query-mux): the empty query
// reaches the repository FTS validator — the single source of truth since
// PR #113 — and its semantic error must surface to the LLM as JSON-RPC -32002
// with the repository message. The transport no longer pre-validates, so this
// pins the repository message, not a transport mirror.
func TestToolSearchClientsAdvancedRejectsEmptyQuery(t *testing.T) {
	mux := newIntegrationMux(t)
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"search_clients_advanced","arguments":{"query_text":""}}}`
	_, code, msg := decodeRPCEnvelope(t, postMCPCaller(t, mux, "owner-1", body))
	if code != -32002 {
		t.Errorf("code = %d; want -32002 (msg=%q)", code, msg)
	}
	if msg != "la consulta de búsqueda está vacía." {
		t.Errorf("message = %q; want the repository empty-query message", msg)
	}
}

// TestIntegrationProfessionalSchedules runs get_professional_schedules through
// the real composition (real SQLite → repositories → use case → tool → real
// RBAC map) and pins the S7b double gate end to end:
//   - the owner reads the seeded agenda (identity + rows ordered by day);
//   - a professional with no slot answers [] and stays distinguishable from a
//     non-existent id, which answers the semantic NOT_FOUND;
//   - admin and staff read the agenda (feat-staff-self-read S3), staff own-only;
//     the client is stopped by the ToolRBAC layer with -32001 before the handler
//     runs, while the use case's and repository's scope gates (which answer the
//     -32002 semantic forbidden) are pinned here and in
//     tools_professionals_test.go + get_professional_schedules_test.go +
//     schedules_test.go.
func TestIntegrationProfessionalSchedules(t *testing.T) {
	mux, conn := newIntegrationMuxWithDB(t)

	var seeded struct {
		Professional struct {
			ID     string `json:"id"`
			Name   string `json:"name"`
			Status string `json:"status"`
		} `json:"professional"`
		Schedules []struct {
			ID        int    `json:"id"`
			DayOfWeek int    `json:"day_of_week"`
			StartTime string `json:"start_time"`
			EndTime   string `json:"end_time"`
		} `json:"schedules"`
	}
	decodeToolStructured(t, mustCallTool(t, mux, "owner-1", "get_professional_schedules", `{"professional_id":"p1"}`), &seeded)

	if seeded.Professional.ID != "p1" || seeded.Professional.Name != "Profesional Uno" || seeded.Professional.Status != "active" {
		t.Errorf("professional = %+v; want p1/Profesional Uno/active", seeded.Professional)
	}
	if len(seeded.Schedules) != 1 {
		t.Fatalf("schedules = %+v; want the single seeded Monday slot", seeded.Schedules)
	}
	if seeded.Schedules[0].DayOfWeek != 1 || seeded.Schedules[0].StartTime != "09:00" || seeded.Schedules[0].EndTime != "17:00" {
		t.Errorf("row = %+v; want day 1 09:00-17:00", seeded.Schedules[0])
	}

	// A professional without any slot: [] is not an error, and the identity still
	// comes back so the delete gate can name whom it confirms.
	if _, err := conn.ExecContext(context.Background(),
		`INSERT INTO professionals (id, name, status) VALUES ('p-sin-agenda', 'Sin Agenda', 'inactive')`); err != nil {
		t.Fatalf("seed professional without agenda: %v", err)
	}
	var empty struct {
		Professional struct {
			Status string `json:"status"`
		} `json:"professional"`
		Schedules []struct {
			DayOfWeek int `json:"day_of_week"`
		} `json:"schedules"`
	}
	decodeToolStructured(t, mustCallTool(t, mux, "owner-1", "get_professional_schedules", `{"professional_id":"p-sin-agenda"}`), &empty)
	if len(empty.Schedules) != 0 {
		t.Errorf("schedules = %+v; want [] for a professional without agenda", empty.Schedules)
	}
	if empty.Professional.Status != "inactive" {
		t.Errorf("status = %q; want inactive (inactive professionals are returned, not hidden)", empty.Professional.Status)
	}

	// Unknown id → semantic NOT_FOUND (-32002), never an empty list.
	if _, code, msg := callMCPTool(t, mux, "owner-1", "get_professional_schedules", `{"professional_id":"p-missing"}`); code != -32002 || msg != "el profesional indicado no existe" {
		t.Errorf("unknown id: code=%d msg=%q; want -32002 %q", code, msg, "el profesional indicado no existe")
	}

	// feat-staff-self-read S3 (issue #276): admin and staff read the agenda too.
	// The RBAC row admits them, the use case's RequireRole re-asserts the role and
	// auth.AuthorizeProfessionalRead decides the scope; the repository re-asserts
	// the same predicate as the third gate.
	for _, caller := range []string{"admin-1", "staff-1"} {
		var got struct {
			Professional struct {
				ID string `json:"id"`
			} `json:"professional"`
			Schedules []struct {
				DayOfWeek int `json:"day_of_week"`
			} `json:"schedules"`
		}
		decodeToolStructured(t, mustCallTool(t, mux, caller, "get_professional_schedules", `{"professional_id":"p1"}`), &got)
		if got.Professional.ID != "p1" {
			t.Errorf("caller %s: professional = %+v; want p1", caller, got.Professional)
		}
		if len(got.Schedules) != 1 {
			t.Errorf("caller %s: schedules = %+v; want the seeded Monday slot", caller, got.Schedules)
		}
	}

	// Staff is own-only (S2 semantics, S3 resource): the same FORBIDDEN answer as
	// the record read, never the repository's old UNAUTHENTICATED. staff-1 is
	// linked to p1, so p-sin-agenda is a foreign id.
	if _, code, msg := callMCPTool(t, mux, "staff-1", "get_professional_schedules", `{"professional_id":"p-sin-agenda"}`); code != -32002 || msg != auth.MsgForbiddenTool {
		t.Errorf("staff naming a foreign agenda: code=%d msg=%q; want -32002 %q", code, msg, auth.MsgForbiddenTool)
	}

	// Layer 1 stays load-bearing for the client, which the RBAC row keeps out
	// entirely (S5): the map refuses with -32001 before the handler runs.
	if _, code, msg := callMCPTool(t, mux, "c1", "get_professional_schedules", `{"professional_id":"p1"}`); code != -32001 || msg != auth.MsgForbiddenTool {
		t.Errorf("client: code=%d msg=%q; want -32001 %q", code, msg, auth.MsgForbiddenTool)
	}
}

// TestIntegrationGetProfessional runs get_professional through the real
// composition (real SQLite → repository → use case → tool → real RBAC map) and
// pins feat-staff-self-read S1/S2/S4 end to end:
//   - owner and admin read the record, and only the owner (and the professional
//     itself) receives email/phone;
//   - staff reads its own record with an ABSENT professional_id (self) and with
//     its own id;
//   - staff naming a foreign professional is refused with -32002 and the shipped
//     message: a FORBIDDEN scope answer, never UNAUTHENTICATED;
//   - owner omitting the id gets the input error naming the missing field;
//   - the client never reaches the tool: the RBAC row refuses it with -32001.
func TestIntegrationGetProfessional(t *testing.T) {
	mux, conn := newIntegrationMuxWithDB(t)

	// The seed carries no contact data for p1; this test needs real values so
	// "present" and "omitted" are distinguishable instead of comparing nils.
	if _, err := conn.ExecContext(context.Background(),
		`UPDATE professionals SET email = 'p1@example.com', phone = '+5491100000001', role_specialty = 'Estilista', specialties = '["s1"]' WHERE id = 'p1'`); err != nil {
		t.Fatalf("seed contact data: %v", err)
	}

	type recordWire struct {
		Professional struct {
			ID            string   `json:"id"`
			Name          string   `json:"name"`
			RoleSpecialty *string  `json:"role_specialty"`
			Status        string   `json:"status"`
			Specialties   []string `json:"specialties"`
		} `json:"professional"`
		Email *string `json:"email"`
		Phone *string `json:"phone"`
	}

	t.Run("owner reads any record with contact data", func(t *testing.T) {
		var got recordWire
		decodeToolStructured(t, mustCallTool(t, mux, "owner-1", "get_professional", `{"professional_id":"p1"}`), &got)
		if got.Professional.ID != "p1" || got.Professional.Name != "Profesional Uno" || got.Professional.Status != "active" {
			t.Errorf("professional = %+v; want p1/Profesional Uno/active", got.Professional)
		}
		if got.Professional.RoleSpecialty == nil || *got.Professional.RoleSpecialty != "Estilista" {
			t.Errorf("role_specialty = %v; want Estilista", got.Professional.RoleSpecialty)
		}
		if len(got.Professional.Specialties) != 1 || got.Professional.Specialties[0] != "s1" {
			t.Errorf("specialties = %v; want [s1]", got.Professional.Specialties)
		}
		if got.Email == nil || *got.Email != "p1@example.com" || got.Phone == nil || *got.Phone != "+5491100000001" {
			t.Errorf("contact = %v/%v; the owner must receive email and phone (S4)", got.Email, got.Phone)
		}
	})

	t.Run("admin reads the record without contact data", func(t *testing.T) {
		result := mustCallTool(t, mux, "admin-1", "get_professional", `{"professional_id":"p1"}`)
		var got recordWire
		decodeToolStructured(t, result, &got)
		if got.Professional.ID != "p1" {
			t.Errorf("professional = %+v; admin must read the record (S1)", got.Professional)
		}
		// Asserted on the raw payload: a null pair would still decode into nils.
		if bytes.Contains(result, []byte(`"email"`)) || bytes.Contains(result, []byte(`"phone"`)) {
			t.Errorf("admin received contact data; S4 withholds it: %s", string(result))
		}
	})

	t.Run("staff reads its own record, with and without the id", func(t *testing.T) {
		for name, args := range map[string]string{
			"omitted": `{}`,
			"own id":  `{"professional_id":"p1"}`,
		} {
			t.Run(name, func(t *testing.T) {
				var got recordWire
				decodeToolStructured(t, mustCallTool(t, mux, "staff-1", "get_professional", args), &got)
				if got.Professional.ID != "p1" {
					t.Errorf("professional = %+v; want the caller's own p1", got.Professional)
				}
				if got.Email == nil || got.Phone == nil {
					t.Errorf("contact = %v/%v; the professional itself must see its own contact data (S4)",
						got.Email, got.Phone)
				}
			})
		}
	})

	t.Run("staff naming a foreign record is forbidden", func(t *testing.T) {
		if _, err := conn.ExecContext(context.Background(),
			`INSERT INTO professionals (id, name, status) VALUES ('p-otro', 'Otro', 'active')`); err != nil {
			t.Fatalf("seed foreign professional: %v", err)
		}
		_, code, msg := callMCPTool(t, mux, "staff-1", "get_professional", `{"professional_id":"p-otro"}`)
		// The scope answer is a semantic FORBIDDEN (-32002), not the old
		// repository UNAUTHENTICATED and not the middleware's -32001: the RBAC row
		// admits staff, and the per-id decision lives below the transport.
		if code != -32002 {
			t.Errorf("code = %d; want -32002 (a FORBIDDEN scope answer, msg=%q)", code, msg)
		}
		if msg != auth.MsgForbiddenTool {
			t.Errorf("message = %q; want the shipped refusal %q", msg, auth.MsgForbiddenTool)
		}
	})

	t.Run("a caller without a professional that omits the id gets an input error", func(t *testing.T) {
		_, code, msg := callMCPTool(t, mux, "owner-1", "get_professional", `{}`)
		if code != -32002 || msg != auth.MsgProfessionalScopeRequired {
			t.Errorf("code=%d msg=%q; want -32002 %q", code, msg, auth.MsgProfessionalScopeRequired)
		}
	})

	t.Run("unknown id is NOT_FOUND", func(t *testing.T) {
		_, code, msg := callMCPTool(t, mux, "owner-1", "get_professional", `{"professional_id":"p-missing"}`)
		if code != -32002 || msg != "el profesional indicado no existe" {
			t.Errorf("code=%d msg=%q; want -32002 %q", code, msg, "el profesional indicado no existe")
		}
	})

	t.Run("client is refused by the RBAC row", func(t *testing.T) {
		_, code, msg := callMCPTool(t, mux, "c1", "get_professional", `{"professional_id":"p1"}`)
		if code != -32001 || msg != auth.MsgForbiddenTool {
			t.Errorf("code=%d msg=%q; want -32001 %q (the client stays out of this surface, S5)",
				code, msg, auth.MsgForbiddenTool)
		}
	})
}
