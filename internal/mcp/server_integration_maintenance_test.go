package mcp

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
)

// Integration coverage for the eight owner-only maintenance WRITE tools
// (ADR-0015): the full production composition (real SQLite file → repositories
// → use cases → tools → AuthMiddleware with the real RBAC map), driven through
// the HTTP transport with X-Caller-Id exactly like the rest of the integration
// suite. The mock-port tests in tools_maintenance_test.go pin the tool handler
// contract; these tests prove the same contract holds against a real database,
// where the repository, the driver and the SQLite constraint layer (FK
// classification) are live.

// maintenanceToolNames and maintenanceToolArgs (tools_maintenance_test.go) are
// reused on purpose instead of duplicated: if the maintenance surface changes,
// the mock-port tests and these integration tests move together.

// weeklyHoursJSON renders a business_hours JSON object for the given days, all
// open 09:00-18:00. Keys follow the profile encoding (1=Monday..7=Sunday),
// which is exactly what the profile write tool stores verbatim.
func weeklyHoursJSON(days ...int) string {
	parts := make([]string, 0, len(days))
	for _, day := range days {
		parts = append(parts, fmt.Sprintf(`"%d":{"open":"09:00","close":"18:00"}`, day))
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// nextSunday10AM returns the next Sunday at 10:00 in the seeded business
// timezone, strictly after today so the slot is always in the future. Same
// rationale as nextMonday10AM: a hardcoded date rots.
func nextSunday10AM(t *testing.T) time.Time {
	t.Helper()
	loc, err := time.LoadLocation("America/Argentina/Buenos_Aires")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	now := time.Now().In(loc)
	// Days until the next Sunday strictly after today (1..7).
	days := (7 - int(now.Weekday())) % 7
	if days == 0 {
		days = 7
	}
	y, m, d := now.Date()
	return time.Date(y, m, d+days, 10, 0, 0, 0, loc)
}

// callMCPTool performs a tools/call for tool with argsJSON as the raw
// arguments object and returns the decoded envelope.
func callMCPTool(t *testing.T, h http.Handler, callerID, tool, argsJSON string) (json.RawMessage, int64, string) {
	t.Helper()
	body := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, tool, argsJSON)
	rec := postMCPCaller(t, h, callerID, body)
	return decodeRPCEnvelope(t, rec)
}

// mustCallTool is callMCPTool for the happy path: any non-zero code fails the
// test with the tool name and the semantic message.
func mustCallTool(t *testing.T, h http.Handler, callerID, tool, argsJSON string) json.RawMessage {
	t.Helper()
	result, code, msg := callMCPTool(t, h, callerID, tool, argsJSON)
	if code != 0 {
		t.Fatalf("%s failed: code=%d msg=%q", tool, code, msg)
	}
	return result
}

// decodeToolStructured decodes the structuredContent of a successful tools/call
// into out (a pointer to the pinned output type).
func decodeToolStructured(t *testing.T, result json.RawMessage, out any) {
	t.Helper()
	var env struct {
		StructuredContent json.RawMessage `json:"structuredContent"`
	}
	if err := json.Unmarshal(result, &env); err != nil {
		t.Fatalf("unmarshal tool result: %v; body=%s", err, string(result))
	}
	if len(env.StructuredContent) == 0 {
		t.Fatalf("tool result carries no structuredContent: %s", string(result))
	}
	if err := json.Unmarshal(env.StructuredContent, out); err != nil {
		t.Fatalf("unmarshal structuredContent: %v; body=%s", err, string(env.StructuredContent))
	}
}

// getBusinessProfile reads the singleton profile through the real read tool.
func getBusinessProfile(t *testing.T, h http.Handler, callerID string) businessProfileOut {
	t.Helper()
	result := mustCallTool(t, h, callerID, "get_business_profile", `{}`)
	var out businessProfileOut
	decodeToolStructured(t, result, &out)
	return out
}

// searchServices runs the FTS read tool and returns its results (the read-back
// path for the service write tools).
func searchServices(t *testing.T, h http.Handler, callerID, query string) []dto.ServiceSearchEntry {
	t.Helper()
	result := mustCallTool(t, h, callerID, "search_services_advanced", fmt.Sprintf(`{"query_text":%q}`, query))
	var out struct {
		Results []dto.ServiceSearchEntry `json:"results"`
	}
	decodeToolStructured(t, result, &out)
	return out.Results
}

// countScheduleRows counts the stored weekly slots for one (professional, day).
func countScheduleRows(t *testing.T, conn *sql.DB, professionalID string, day int) int {
	t.Helper()
	var n int
	if err := conn.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM schedules WHERE professional_id = ? AND day_of_week = ?`,
		professionalID, day).Scan(&n); err != nil {
		t.Fatalf("count schedules: %v", err)
	}
	return n
}

// ── registration + RBAC ──

// TestIntegrationMaintenanceToolsRegistered proves the eight maintenance tools
// are live in the production composition (not just the mock-port server): the
// harness injects all eight ports, so tools/list must expose 19 tools.
func TestIntegrationMaintenanceToolsRegistered(t *testing.T) {
	mux := newIntegrationMux(t)

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
		t.Fatalf("unmarshal tools/list: %v", err)
	}
	if len(list.Tools) != 19 {
		t.Errorf("tools = %d; want 19", len(list.Tools))
	}
	registered := make(map[string]bool, len(list.Tools))
	for _, tool := range list.Tools {
		registered[tool.Name] = true
	}
	for _, want := range maintenanceToolNames() {
		if !registered[want] {
			t.Errorf("maintenance tool %q is not registered", want)
		}
	}
}

// TestIntegrationMaintenanceRoleRejection proves the owner-only gate is live at
// the transport RBAC layer for every maintenance tool: a staff account and a
// client are rejected with -32001 before the handler runs, while the owner
// succeeds with the same tool.
func TestIntegrationMaintenanceRoleRejection(t *testing.T) {
	mux := newIntegrationMux(t)

	// Arguments are the schema-valid fixtures from the mock-port suite: the RBAC
	// denial is answered by the auth middleware before the SDK validates them.
	for _, caller := range []string{"staff-1", "c1"} {
		for _, tool := range maintenanceToolNames() {
			_, code, msg := callMCPTool(t, mux, caller, tool, maintenanceToolArgs[tool])
			if code != -32001 || msg != "no tienes permiso para realizar esta acción" {
				t.Errorf("caller %s %s: code=%d msg=%q; want -32001 %q",
					caller, tool, code, msg, "no tienes permiso para realizar esta acción")
			}
		}
	}

	// Same tool the staff caller was denied: the owner is admitted.
	result := mustCallTool(t, mux, "owner-1", "update_business_profile", `{"general_description":"actualizado por el owner"}`)
	var out businessProfileOut
	decodeToolStructured(t, result, &out)
	if out.GeneralDescription == nil || *out.GeneralDescription != "actualizado por el owner" {
		t.Errorf("general_description = %v; want %q", out.GeneralDescription, "actualizado por el owner")
	}
}

// TestIntegrationMaintenanceMissingCallerID proves the maintenance tools keep
// the fail-closed unauthenticated contract (-32000) when X-Caller-Id is absent.
func TestIntegrationMaintenanceMissingCallerID(t *testing.T) {
	mux := newIntegrationMux(t)

	for _, tool := range maintenanceToolNames() {
		_, code, msg := callMCPTool(t, mux, "", tool, maintenanceToolArgs[tool])
		if code != -32000 || msg != "no se proporcionó X-Caller-Id" {
			t.Errorf("%s: code=%d msg=%q; want -32000 %q", tool, code, msg, "no se proporcionó X-Caller-Id")
		}
	}
}

// ── happy path per tool family ──

// TestIntegrationMaintenanceBusinessProfileMerge proves the partial-merge
// contract end-to-end: only the sent fields change, the untouched columns keep
// their stored values, and the write is visible through get_business_profile.
func TestIntegrationMaintenanceBusinessProfileMerge(t *testing.T) {
	mux := newIntegrationMux(t)

	before := getBusinessProfile(t, mux, "owner-1")
	if before.Name != "Mi Negocio" {
		t.Fatalf("seeded profile name = %q; want %q", before.Name, "Mi Negocio")
	}

	// (1) Update two fields and assert every untouched field survived.
	result := mustCallTool(t, mux, "owner-1", "update_business_profile",
		`{"name":"Barbería Kike","general_description":"Cortes y barba"}`)
	var merged businessProfileOut
	decodeToolStructured(t, result, &merged)

	if merged.Name != "Barbería Kike" {
		t.Errorf("name = %q; want %q", merged.Name, "Barbería Kike")
	}
	if merged.GeneralDescription == nil || *merged.GeneralDescription != "Cortes y barba" {
		t.Errorf("general_description = %v; want %q", merged.GeneralDescription, "Cortes y barba")
	}
	if merged.Timezone != before.Timezone {
		t.Errorf("timezone = %q; want untouched %q", merged.Timezone, before.Timezone)
	}
	if merged.BusinessHours != before.BusinessHours {
		t.Errorf("business_hours changed without being sent: %q; want %q", merged.BusinessHours, before.BusinessHours)
	}
	if merged.SlotIntervalMinutes != before.SlotIntervalMinutes {
		t.Errorf("slot_interval_minutes = %d; want untouched %d", merged.SlotIntervalMinutes, before.SlotIntervalMinutes)
	}
	if merged.CurrencyCode != before.CurrencyCode || merged.CurrencySymbol != before.CurrencySymbol {
		t.Errorf("currency = %s/%s; want untouched %s/%s",
			merged.CurrencyCode, merged.CurrencySymbol, before.CurrencyCode, before.CurrencySymbol)
	}

	// (2) Persistence: the read tool observes the merged values.
	after := getBusinessProfile(t, mux, "owner-1")
	if after.Name != "Barbería Kike" {
		t.Errorf("read-back name = %q; want %q", after.Name, "Barbería Kike")
	}
	if after.GeneralDescription == nil || *after.GeneralDescription != "Cortes y barba" {
		t.Errorf("read-back general_description = %v; want %q", after.GeneralDescription, "Cortes y barba")
	}
	if after.Timezone != before.Timezone || after.BusinessHours != before.BusinessHours {
		t.Errorf("read-back clobbered untouched fields: timezone=%q business_hours=%q",
			after.Timezone, after.BusinessHours)
	}

	// (3) A second partial update touching only business_hours must keep the
	// name written by the first one.
	hours := weeklyHoursJSON(1, 2, 3, 4, 5, 6)
	result = mustCallTool(t, mux, "owner-1", "update_business_profile",
		fmt.Sprintf(`{"business_hours":%q}`, hours))
	var hoursOnly businessProfileOut
	decodeToolStructured(t, result, &hoursOnly)
	if hoursOnly.BusinessHours != hours {
		t.Errorf("business_hours = %q; want verbatim %q", hoursOnly.BusinessHours, hours)
	}
	if hoursOnly.Name != "Barbería Kike" {
		t.Errorf("name = %q; want preserved %q", hoursOnly.Name, "Barbería Kike")
	}
	if got := getBusinessProfile(t, mux, "owner-1").BusinessHours; got != hours {
		t.Errorf("read-back business_hours = %q; want %q", got, hours)
	}
}

// TestIntegrationMaintenanceServiceLifecycle proves create → read → partial
// update → read → delete against real SQLite and the real FTS read tool.
func TestIntegrationMaintenanceServiceLifecycle(t *testing.T) {
	mux := newIntegrationMux(t)

	// create_service: is_active omitted ⇒ the service is created active.
	result := mustCallTool(t, mux, "owner-1", "create_service",
		`{"name":"Corte de pelo","description":"corte clásico","duration_minutes":30,"price":2500.5}`)
	var created serviceOut
	decodeToolStructured(t, result, &created)
	if created.ID == "" {
		t.Fatal("create_service returned an empty id")
	}
	if created.Name != "Corte de pelo" || created.DurationMinutes != 30 || created.Price != 2500.5 {
		t.Errorf("created = %+v; want name/duration/price as sent", created)
	}
	if !created.IsActive {
		t.Error("created service is not active; want the omitted is_active to default to true")
	}
	if created.Description == nil || *created.Description != "corte clásico" {
		t.Errorf("created description = %v; want %q", created.Description, "corte clásico")
	}

	// Read-back through the FTS read tool (services_fts is trigger-synced).
	readBack := searchServices(t, mux, "owner-1", "Corte")
	if len(readBack) != 1 {
		t.Fatalf("search_services_advanced results = %d; want 1: %+v", len(readBack), readBack)
	}
	if readBack[0].ID != created.ID || readBack[0].DurationMinutes != 30 || readBack[0].Price != 2500.5 {
		t.Errorf("read-back = %+v; want the created service", readBack[0])
	}

	// Partial update: only duration and price are sent.
	result = mustCallTool(t, mux, "owner-1", "update_service",
		fmt.Sprintf(`{"service_id":%q,"duration_minutes":45,"price":3000}`, created.ID))
	var updated serviceOut
	decodeToolStructured(t, result, &updated)
	if updated.DurationMinutes != 45 || updated.Price != 3000 {
		t.Errorf("updated duration/price = %d/%v; want 45/3000", updated.DurationMinutes, updated.Price)
	}
	if updated.Name != "Corte de pelo" {
		t.Errorf("updated name = %q; want preserved %q", updated.Name, "Corte de pelo")
	}
	if updated.Description == nil || *updated.Description != "corte clásico" {
		t.Errorf("updated description = %v; want preserved", updated.Description)
	}
	if !updated.IsActive {
		t.Error("updated service became inactive; want is_active preserved")
	}
	readBack = searchServices(t, mux, "owner-1", "Corte")
	if len(readBack) != 1 || readBack[0].DurationMinutes != 45 || readBack[0].Price != 3000 {
		t.Errorf("read-back after update = %+v; want duration 45 / price 3000", readBack)
	}

	// delete_service removes the row (no bookings reference it).
	result = mustCallTool(t, mux, "owner-1", "delete_service", fmt.Sprintf(`{"service_id":%q}`, created.ID))
	var deleted dto.DeleteServiceResult
	decodeToolStructured(t, result, &deleted)
	if deleted.ServiceID != created.ID || deleted.Status != "deleted" {
		t.Errorf("delete result = %+v; want {%s deleted}", deleted, created.ID)
	}
	if got := searchServices(t, mux, "owner-1", "Corte"); len(got) != 0 {
		t.Errorf("search after delete = %+v; want 0 results", got)
	}

	// Updating a deleted service maps to the semantic not-found message.
	_, code, msg := callMCPTool(t, mux, "owner-1", "update_service",
		fmt.Sprintf(`{"service_id":%q,"duration_minutes":60}`, created.ID))
	if code != -32002 || msg != "el servicio no existe" {
		t.Errorf("update after delete: code=%d msg=%q; want -32002 %q", code, msg, "el servicio no existe")
	}
}

// TestIntegrationMaintenanceProfessionalLifecycle proves create → partial
// update against real SQLite (the professionals row is read back directly: no
// professional read tool exists yet).
func TestIntegrationMaintenanceProfessionalLifecycle(t *testing.T) {
	mux, conn := newIntegrationMuxWithDB(t)

	result := mustCallTool(t, mux, "owner-1", "create_professional",
		`{"name":"Ana","role_specialty":"Estilista","email":"ana@example.com","phone":"+5491100000003","specialties":["s1"]}`)
	var created professionalOut
	decodeToolStructured(t, result, &created)
	if created.ID == "" {
		t.Fatal("create_professional returned an empty id")
	}
	if created.Name != "Ana" || created.Status != "active" {
		t.Errorf("created = %+v; want name Ana / status active", created)
	}
	if len(created.Specialties) != 1 || created.Specialties[0] != "s1" {
		t.Errorf("created specialties = %v; want [s1]", created.Specialties)
	}

	// Persisted row carries the generated UUID and the JSON-encoded specialties.
	var storedName, storedRole, storedStatus, storedSpecialties string
	var storedEmail, storedPhone sql.NullString
	if err := conn.QueryRowContext(context.Background(),
		`SELECT name, role_specialty, status, email, phone, specialties FROM professionals WHERE id = ?`,
		created.ID,
	).Scan(&storedName, &storedRole, &storedStatus, &storedEmail, &storedPhone, &storedSpecialties); err != nil {
		t.Fatalf("read stored professional: %v", err)
	}
	if storedName != "Ana" || storedRole != "Estilista" || storedStatus != "active" {
		t.Errorf("stored = %q/%q/%q; want Ana/Estilista/active", storedName, storedRole, storedStatus)
	}
	if storedEmail.String != "ana@example.com" || storedPhone.String != "+5491100000003" {
		t.Errorf("stored contact = %q/%q; want the sent email/phone", storedEmail.String, storedPhone.String)
	}
	if storedSpecialties != `["s1"]` {
		t.Errorf("stored specialties = %q; want %q", storedSpecialties, `["s1"]`)
	}

	// Partial update: only name and status are sent.
	result = mustCallTool(t, mux, "owner-1", "update_professional",
		fmt.Sprintf(`{"professional_id":%q,"name":"Ana Gómez","status":"inactive"}`, created.ID))
	var updated professionalOut
	decodeToolStructured(t, result, &updated)
	if updated.Name != "Ana Gómez" || updated.Status != "inactive" {
		t.Errorf("updated = %+v; want name Ana Gómez / status inactive", updated)
	}
	if updated.RoleSpecialty == nil || *updated.RoleSpecialty != "Estilista" {
		t.Errorf("updated role_specialty = %v; want preserved Estilista", updated.RoleSpecialty)
	}
	if updated.Email == nil || *updated.Email != "ana@example.com" {
		t.Errorf("updated email = %v; want preserved ana@example.com", updated.Email)
	}
	if updated.Phone == nil || *updated.Phone != "+5491100000003" {
		t.Errorf("updated phone = %v; want preserved +5491100000003", updated.Phone)
	}
	if len(updated.Specialties) != 1 || updated.Specialties[0] != "s1" {
		t.Errorf("updated specialties = %v; want preserved [s1]", updated.Specialties)
	}

	// The merge is persisted.
	if err := conn.QueryRowContext(context.Background(),
		`SELECT name, status, role_specialty, specialties FROM professionals WHERE id = ?`,
		created.ID,
	).Scan(&storedName, &storedStatus, &storedRole, &storedSpecialties); err != nil {
		t.Fatalf("read updated professional: %v", err)
	}
	if storedName != "Ana Gómez" || storedStatus != "inactive" {
		t.Errorf("stored after update = %q/%q; want Ana Gómez/inactive", storedName, storedStatus)
	}
	if storedRole != "Estilista" || storedSpecialties != `["s1"]` {
		t.Errorf("stored after update clobbered role/specialties: %q/%q", storedRole, storedSpecialties)
	}
}

// TestIntegrationMaintenanceScheduleLifecycle proves insert → replace → delete
// of a weekly slot, including the id stability of the upsert and the untouched
// seeded Monday row.
func TestIntegrationMaintenanceScheduleLifecycle(t *testing.T) {
	mux, conn := newIntegrationMuxWithDB(t)

	// Wednesday = 3 (Go time.Weekday encoding).
	result := mustCallTool(t, mux, "owner-1", "upsert_schedule",
		`{"professional_id":"p1","day_of_week":3,"start_time":"09:00","end_time":"12:00"}`)
	var created scheduleOut
	decodeToolStructured(t, result, &created)
	if created.ID == 0 {
		t.Fatal("upsert_schedule returned schedule id 0; the stored row was not read back")
	}
	if created.ProfessionalID != "p1" || created.DayOfWeek != 3 {
		t.Errorf("created = %+v; want professional p1 / day 3", created)
	}
	if created.StartTime != "09:00" || created.EndTime != "12:00" {
		t.Errorf("created times = %s-%s; want 09:00-12:00", created.StartTime, created.EndTime)
	}
	if got := countScheduleRows(t, conn, "p1", 3); got != 1 {
		t.Fatalf("stored rows for day 3 = %d; want 1", got)
	}

	// Upsert the same day again: the row is replaced in place (same id).
	result = mustCallTool(t, mux, "owner-1", "upsert_schedule",
		`{"professional_id":"p1","day_of_week":3,"start_time":"10:00","end_time":"13:00"}`)
	var replaced scheduleOut
	decodeToolStructured(t, result, &replaced)
	if replaced.ID != created.ID {
		t.Errorf("upsert id = %d; want the replaced row id %d", replaced.ID, created.ID)
	}
	if replaced.StartTime != "10:00" || replaced.EndTime != "13:00" {
		t.Errorf("replaced times = %s-%s; want 10:00-13:00", replaced.StartTime, replaced.EndTime)
	}
	if got := countScheduleRows(t, conn, "p1", 3); got != 1 {
		t.Errorf("stored rows for day 3 after upsert = %d; want 1", got)
	}
	// The seeded Monday slot is untouched.
	if got := countScheduleRows(t, conn, "p1", 1); got != 1 {
		t.Errorf("stored rows for day 1 = %d; want the seeded row untouched", got)
	}

	// delete_schedule removes exactly that slot.
	result = mustCallTool(t, mux, "owner-1", "delete_schedule", `{"professional_id":"p1","day_of_week":3}`)
	var deleted dto.DeleteScheduleResult
	decodeToolStructured(t, result, &deleted)
	if deleted.ProfessionalID != "p1" || deleted.DayOfWeek != 3 || deleted.Status != "deleted" {
		t.Errorf("delete result = %+v; want {p1 3 deleted}", deleted)
	}
	if got := countScheduleRows(t, conn, "p1", 3); got != 0 {
		t.Errorf("stored rows for day 3 after delete = %d; want 0", got)
	}

	// Deleting the same slot again maps to the semantic not-found message.
	_, code, msg := callMCPTool(t, mux, "owner-1", "delete_schedule", `{"professional_id":"p1","day_of_week":3}`)
	if code != -32002 || msg != "el profesional no tiene un horario ese día" {
		t.Errorf("second delete: code=%d msg=%q; want -32002 %q", code, msg, "el profesional no tiene un horario ese día")
	}
}

// ── semantic error mapping (real SQLite / driver classification) ──

// TestIntegrationMaintenanceSemanticErrors proves the T2 classifier works
// end-to-end: the two foreign-key paths reach the client as LLM-actionable
// -32002 messages, never as internal errors or raw driver text.
func TestIntegrationMaintenanceSemanticErrors(t *testing.T) {
	t.Run("delete_service with an active booking", func(t *testing.T) {
		mux, conn := newIntegrationMuxWithDB(t)
		// The booking is inserted directly: the path under test is the DELETE
		// blocked by bookings.service_id ON DELETE RESTRICT, not the booking flow.
		if _, err := conn.ExecContext(context.Background(),
			`INSERT INTO bookings (id, client_id, professional_id, service_id, start_datetime, end_datetime, status)
			 VALUES ('b-fk', 'c1', 'p1', 's1', '2027-03-01T13:00:00.000Z', '2027-03-01T14:00:00.000Z', 'confirmed')`,
		); err != nil {
			t.Fatalf("seed booking: %v", err)
		}

		_, code, msg := callMCPTool(t, mux, "owner-1", "delete_service", `{"service_id":"s1"}`)
		if code != -32002 {
			t.Fatalf("delete_service code = %d (msg=%q); want -32002", code, msg)
		}
		if !strings.Contains(msg, "reservas") {
			t.Errorf("msg = %q; want it to mention the associated bookings", msg)
		}
		// RESTRICT kept the row: the conflict is a refusal, not a partial delete.
		var n int
		if err := conn.QueryRowContext(context.Background(),
			`SELECT COUNT(*) FROM services WHERE id = 's1'`).Scan(&n); err != nil {
			t.Fatalf("count services: %v", err)
		}
		if n != 1 {
			t.Errorf("services rows = %d; want the blocked service still stored", n)
		}
	})

	t.Run("upsert_schedule for a non-existent professional", func(t *testing.T) {
		mux := newIntegrationMux(t)

		_, code, msg := callMCPTool(t, mux, "owner-1", "upsert_schedule",
			`{"professional_id":"ghost","day_of_week":1,"start_time":"09:00","end_time":"12:00"}`)
		if code != -32002 {
			t.Fatalf("upsert_schedule code = %d (msg=%q); want -32002", code, msg)
		}
		if !strings.Contains(msg, "no existe") {
			t.Errorf("msg = %q; want it to say the professional does not exist", msg)
		}
		// No driver detail (constraint name, integer code, file path) leaks out.
		for _, leak := range []string{"FOREIGN KEY", "1811", "sqlite", "SQLite"} {
			if strings.Contains(msg, leak) {
				t.Errorf("msg %q leaks internal detail %q", msg, leak)
			}
		}
	})
}

// ── day-key contract end-to-end ──

// TestIntegrationMaintenanceDayKeyContract pins the two day encodings across
// the real write/read tools and the real availability/booking chain:
//
//   - schedules.day_of_week uses Go's time.Weekday (0=Sunday).
//   - business_hours uses "1".."7" (7=Sunday), stored verbatim by the profile
//     write tool and returned verbatim by get_business_profile.
//
// The T1 unit tests pin the translation helper; this test pins the tool-level
// contract: a Sunday slot is only bookable when the profile carries key "7".
func TestIntegrationMaintenanceDayKeyContract(t *testing.T) {
	mux, conn := newIntegrationMuxWithDB(t)
	sunday := nextSunday10AM(t)

	// (1) Sunday closed: the profile is stored without key "7" and the read
	// tool returns the object exactly as written.
	closedHours := weeklyHoursJSON(1, 2, 3, 4, 5, 6)
	result := mustCallTool(t, mux, "owner-1", "update_business_profile",
		fmt.Sprintf(`{"business_hours":%q}`, closedHours))
	var closedProfile businessProfileOut
	decodeToolStructured(t, result, &closedProfile)
	if closedProfile.BusinessHours != closedHours {
		t.Fatalf("business_hours = %q; want verbatim %q", closedProfile.BusinessHours, closedHours)
	}
	readBack := getBusinessProfile(t, mux, "owner-1")
	if readBack.BusinessHours != closedHours {
		t.Fatalf("read-back business_hours = %q; want %q", readBack.BusinessHours, closedHours)
	}
	var storedHours map[string]any
	if err := json.Unmarshal([]byte(readBack.BusinessHours), &storedHours); err != nil {
		t.Fatalf("stored business_hours is not a JSON object: %v", err)
	}
	for _, key := range []string{"1", "2", "3", "4", "5", "6"} {
		if _, ok := storedHours[key]; !ok {
			t.Errorf("stored business_hours is missing key %q: %s", key, readBack.BusinessHours)
		}
	}
	if _, ok := storedHours["7"]; ok {
		t.Errorf("stored business_hours unexpectedly carries the Sunday key: %s", readBack.BusinessHours)
	}

	// (2) The professional does have a Sunday schedule: day_of_week 0.
	result = mustCallTool(t, mux, "owner-1", "upsert_schedule",
		`{"professional_id":"p1","day_of_week":0,"start_time":"10:00","end_time":"14:00"}`)
	var sundaySlot scheduleOut
	decodeToolStructured(t, result, &sundaySlot)
	if sundaySlot.DayOfWeek != 0 {
		t.Fatalf("stored Sunday slot day_of_week = %d; want 0", sundaySlot.DayOfWeek)
	}
	if got := countScheduleRows(t, conn, "p1", 0); got != 1 {
		t.Fatalf("stored rows for day 0 = %d; want 1", got)
	}

	// (3) Sunday is closed for the business: the availability chain stops at
	// the business-hours step, which reads key "7" for Sunday.
	_, code, msg := callMCPTool(t, mux, "owner-1", "check_availability",
		fmt.Sprintf(`{"service_id":"s1","professional_id":"p1","start_datetime":%q}`, sunday.Format(time.RFC3339)))
	if code != -32002 {
		t.Fatalf("closed Sunday: code = %d (msg=%q); want -32002", code, msg)
	}
	// "los domingo" stays prefix-compatible with the pending pluralization fix.
	if !strings.Contains(msg, "no abre los domingo") {
		t.Errorf("closed Sunday msg = %q; want the business-closed message", msg)
	}

	// (4) Owner opens Sunday by adding key "7": the same slot becomes available.
	openHours := weeklyHoursJSON(1, 2, 3, 4, 5, 6, 7)
	result = mustCallTool(t, mux, "owner-1", "update_business_profile",
		fmt.Sprintf(`{"business_hours":%q}`, openHours))
	var openProfile businessProfileOut
	decodeToolStructured(t, result, &openProfile)
	if openProfile.BusinessHours != openHours {
		t.Fatalf("business_hours = %q; want verbatim %q", openProfile.BusinessHours, openHours)
	}
	readBack = getBusinessProfile(t, mux, "owner-1")
	if readBack.BusinessHours != openHours {
		t.Fatalf("read-back business_hours = %q; want %q", readBack.BusinessHours, openHours)
	}
	if err := json.Unmarshal([]byte(readBack.BusinessHours), &storedHours); err != nil {
		t.Fatalf("stored business_hours is not a JSON object: %v", err)
	}
	if _, ok := storedHours["7"]; !ok {
		t.Errorf("stored business_hours lost the Sunday key: %s", readBack.BusinessHours)
	}

	result = mustCallTool(t, mux, "owner-1", "check_availability",
		fmt.Sprintf(`{"service_id":"s1","professional_id":"p1","start_datetime":%q}`, sunday.Format(time.RFC3339)))
	var availability struct {
		Available bool `json:"available"`
	}
	decodeToolStructured(t, result, &availability)
	if !availability.Available {
		t.Error("Sunday slot reported unavailable after the profile opened key \"7\"")
	}

	// (5) The slot availability declared bookable is bookable: the booking
	// chain consumes the same day-key translation.
	result = mustCallTool(t, mux, "owner-1", "create_booking",
		fmt.Sprintf(`{"client_id":"c1","service_id":"s1","professional_id":"p1","start_datetime":%q}`, sunday.Format(time.RFC3339)))
	var booking struct {
		BookingID string `json:"booking_id"`
	}
	decodeToolStructured(t, result, &booking)
	if booking.BookingID == "" {
		t.Error("create_booking returned an empty booking_id for the Sunday slot")
	}
}
