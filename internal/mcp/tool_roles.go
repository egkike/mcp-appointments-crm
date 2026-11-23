package mcp

import "github.com/egkike/mcp-appointments-crm/internal/auth"

// This file is the SINGLE SOURCE of the tool → role policy for /mcp.
//
// It used to live as a literal in cmd/mcp-server/main.go with a verbatim copy in
// internal/mcp/server_integration_test.go. Because cmd/mcp-server is package
// main, the test could not import the production map, so the harness carried its
// own copy: a row edited in one place and not the other was invisible to every
// test. Debt L11 (issue #249) closes that window by keeping one definition here,
// where both the composition root and the integration harness consume it.
//
// Contract: RBAC keys on r.URL.Path, so the JSON-RPC auth translator rewrites
// the path to the tool name for tools/call requests. An absent key means the
// middleware lets the request through, which is why the deliberately ungated
// tools must be declared in UngatedTools instead of simply omitted: the coverage
// test (tool_roles_test.go) fails when a registered tool appears in neither map.

// ToolRoles returns the tool → roles policy for every gated MCP tool.
//
// Roles per tool (ADR-0015 Decision 2): the eight maintenance write tools are
// owner-only because the partial admin scope stays deferred, so no admin role is
// granted for them here. The business-hours exception tools mirror that decision.
//
// Each call returns a FRESH literal. That is what protects the shared policy: a
// caller that mutates the map it received changes only its own copy, never what a
// later caller sees. The rows themselves are frozen policy — adding, removing or
// relaxing one is a security change, not a refactor.
func ToolRoles() auth.ToolRBAC {
	return auth.ToolRBAC{
		// Booking lifecycle: a client operates on its OWN bookings and nothing
		// else (issue #251). The transport gate cannot express "own", so ownership
		// is enforced three times below it, and the layers are deliberately
		// redundant (defense in depth):
		//   1. the read is scoped — FindByID runs through applyAuthFilter, so a
		//      foreign booking is simply not found;
		//   2. the use case re-asserts ownership on the fetched entity
		//      (auth.AuthorizeBookingAccess in cancel/reschedule) and create_booking
		//      compares input.ClientID with the caller's own id;
		//   3. the write statements are scoped too (Cancel 2 clauses, Reschedule 3),
		//      so even a race between read and write cannot touch a foreign row.
		// server_integration_client_booking_test.go asserts the observable outcome of
		// that chain, not any single guard: it fails only when the guards are gone
		// together, which is what makes the redundancy load-bearing rather than
		// decorative. Neutralizing one layer alone still refuses.
		"create_booking":       {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff, auth.RoleClient},
		"cancel_booking":       {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff, auth.RoleClient},
		"reschedule_booking":   {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff, auth.RoleClient},
		"get_booking":          {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff, auth.RoleClient},
		"get_business_profile": {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff, auth.RoleClient},
		"get_pending_alerts":   {auth.RoleOwner, auth.RoleAdmin},
		"mark_alert_as_sent":   {auth.RoleOwner, auth.RoleAdmin},
		"get_loyalty_report":   {auth.RoleOwner, auth.RoleAdmin},

		// Maintenance WRITE tools (ADR-0015): owner-only MVP.
		"update_business_profile": {auth.RoleOwner},
		"create_service":          {auth.RoleOwner},
		"update_service":          {auth.RoleOwner},
		"delete_service":          {auth.RoleOwner},
		"create_professional":     {auth.RoleOwner},
		"update_professional":     {auth.RoleOwner},
		"upsert_schedule":         {auth.RoleOwner},
		"delete_schedule":         {auth.RoleOwner},

		// Business-hours exception tools (feat-hours-exception-tool, D5):
		// owner-only, mirroring the maintenance rows above.
		"create_hours_exception": {auth.RoleOwner},
		"delete_hours_exception": {auth.RoleOwner},
		"list_hours_exceptions":  {auth.RoleOwner},

		// Professionals surface. search_professionals_advanced stays ungated (see
		// UngatedTools). The two record/agenda reads admit owner, admin and staff
		// (feat-staff-self-read S1/S3, issue #276): the ROLE is decided here, the
		// SCOPE (staff is own-only) below it, because this map keys on the tool
		// name and never sees the professional_id argument. A staff caller naming
		// a foreign professional is therefore refused by the shared
		// auth.AuthorizeProfessionalRead predicate with a FORBIDDEN semantic error
		// (-32002 + auth.MsgForbiddenTool), never by this row — the two layers
		// answer different questions and both are required.
		"get_professional":           {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
		"get_professional_schedules": {auth.RoleOwner, auth.RoleAdmin, auth.RoleStaff},
	}
}

// UngatedTools returns every MCP tool that deliberately carries NO ToolRBAC row,
// mapped to the reason its authorization lives elsewhere.
//
// Two distinct shapes are declared here, and they are not interchangeable:
//   - downstream role enforcement: the tool admits any authenticated caller and
//     the use case or repository applies the real scope;
//   - anonymous seam: the tool is reachable before caller resolution, so no role
//     exists to check (register_client).
//
// Adding a tool without deciding its gate makes the coverage test fail: a tool
// that is neither gated nor declared here would be open to every authenticated
// caller, which is the failure mode this list exists to make loud.
func UngatedTools() map[string]string {
	return map[string]string{
		"check_availability": "any authenticated caller; output is availability only",
		"find_available_slots": "any authenticated caller (agent surface T2, spec L10); " +
			"output is PII-free by construction (start + professional only)",
		"search_clients_advanced": "row scope by caller role in the repository " +
			"(internal/repository/clients.go)",
		"search_services_advanced": "two role-scoped views inside the use case: owner/admin get the " +
			"FTS maintenance view over every service, every other authenticated caller gets the " +
			"active-only booking catalog (issue #255)",
		"search_professionals_advanced": "auth.RequireAuthenticated only: the client booking flow is " +
			"the consumer (feat-owner-chat S7a); output is PII-free by construction",
		"register_client": "anonymous allowlist seam: reached before caller resolution and carries " +
			"no role in either layer (ADR-0018); guards are the accounts collision and the " +
			"registration rate limit inside the use case",
	}
}
