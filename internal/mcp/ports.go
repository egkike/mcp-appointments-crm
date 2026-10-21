package mcp

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/application/dto"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// Consumer-side ports for the MCP tools (data-access C5): declared in the
// consumer package, satisfied structurally by the concrete use cases from
// internal/application/usecase that the composition root injects (same
// pattern as internal/application/usecase/validator.go). This file must never
// import internal/repository — TestNoRepositoryImport enforces it
// (REQ-MT-012, REQ-ARCH-INTMCP-003).

// CheckAvailabilityPort answers availability queries for any authenticated
// caller (no RBAC entry in main.go — open set).
type CheckAvailabilityPort interface {
	Execute(context.Context, dto.CheckAvailabilityInput) (*dto.CheckAvailabilityResult, error)
}

// CreateBookingPort creates a booking. Roles: owner/admin/staff (coarse RBAC
// in main.go; auth.RequireClientMatch inside the use case for fine-grained
// staff-calendar / client-self checks).
type CreateBookingPort interface {
	Execute(context.Context, dto.CreateBookingInput) (*dto.CreateBookingResult, error)
}

// GetBookingPort retrieves a single booking. Roles: owner/admin/staff/client
// (cross-tenant isolation inside the use case).
type GetBookingPort interface {
	Execute(context.Context, dto.GetBookingInput) (*dto.GetBookingResult, error)
}

// CancelBookingPort cancels a booking. Roles: owner/admin/staff.
type CancelBookingPort interface {
	Execute(context.Context, dto.CancelBookingInput) (*dto.CancelBookingResult, error)
}

// RescheduleBookingPort moves a booking to a new start time. Roles:
// owner/admin/staff.
type RescheduleBookingPort interface {
	Execute(context.Context, dto.RescheduleBookingInput) (*dto.RescheduleBookingResult, error)
}

// BusinessProfilePort returns the singleton business profile. Roles:
// owner/admin/staff (enforced by the RBAC entry; the profile is not
// tenant-scoped, so the port takes no caller input).
type BusinessProfilePort interface {
	Execute(context.Context) (*entity.BusinessProfile, error)
}

// SearchClientsAdvancedPort performs a role-scoped FTS search on clients.
// No RBAC entry: all authenticated callers are admitted at the transport;
// role scoping lives in the repository.
type SearchClientsAdvancedPort interface {
	Execute(context.Context, dto.SearchClientsAdvancedInput) (*dto.SearchClientsAdvancedResult, error)
}

// SearchServicesAdvancedPort performs an owner/admin FTS search on services.
// No RBAC entry: all authenticated callers are admitted at the transport;
// role enforcement lives in the use case.
type SearchServicesAdvancedPort interface {
	Execute(context.Context, dto.SearchServicesAdvancedInput) (*dto.SearchServicesAdvancedResult, error)
}

// GetPendingAlertsPort returns due pending alerts ordered oldest first.
// ToolRBAC entry: owner/admin only.
type GetPendingAlertsPort interface {
	Execute(context.Context, dto.GetPendingAlertsInput) (*dto.GetPendingAlertsResult, error)
}

// MarkAlertAsSentPort marks a pending alert as sent.
// ToolRBAC entry: owner/admin only.
type MarkAlertAsSentPort interface {
	Execute(context.Context, dto.MarkAlertAsSentInput) (*dto.MarkAlertAsSentResult, error)
}

// GetLoyaltyReportPort returns the most frequent clients in a period.
// ToolRBAC entry: owner/admin only because rows expose phone PII.
type GetLoyaltyReportPort interface {
	Execute(context.Context, dto.GetLoyaltyReportInput) (*dto.GetLoyaltyReportResult, error)
}

// ── Anonymous registration port (feat-whatsapp-bot D2) ──
//
// This is the ONE port whose identity is a bare header value: the method takes
// the phone and the optional display name and there is deliberately no
// caller/role parameter, so the transport cannot pass a role into the
// registration path (compile-time guarantee; client-registration "the seam
// grants no role").
//
// The result carrier is dto.GetOrCreateClientResult — a cross-layer DTO, exactly
// like the dto.* carriers above. It was originally declared in
// internal/application/usecase (feat-whatsapp-bot), which made this transport
// contract depend on a concrete use-case package; the carrier moved to
// internal/application/dto in the follow-up refactor that closed that ADR-0013
// deviation (flagged by GGA round 4 on the S7a slice, PR #108 legacy). Keep the
// carrier in dto: this port must never import a use case again.

// RegisterClientPort resolves or creates the client row identified by the
// anonymous caller phone. Roles: none — reachable ONLY through the anonymous
// allowlist in AuthMiddleware (no ToolRBAC entry in either layer).
//
// Reconciliation with the "double RBAC enforcement" standard (AGENTS.md,
// Boundary contract 1): that rule governs AUTHENTICATED tools, and this tool is
// the documented exception — a phone that has never registered has no caller to
// authorize, so RBAC is inapplicable by construction (ADR-0018; the
// register_client requirement in openspec/specs/client-registration, D2). The
// compensating controls are the ones the spec pins and the code implements:
// the static path allowlist evaluated BEFORE caller resolution, the accounts
// collision rejection (an owner/admin/staff phone can never become a client),
// the per-phone rate limit, and the masked-phone audit trail. The use case takes
// no caller/role parameter at all, so no code path can grant a role here. Any
// future change that makes this tool reachable with a resolved caller MUST add
// the double gate; until then, do not "fix" this by inventing a RequireRole.
type RegisterClientPort interface {
	Execute(ctx context.Context, callerID, displayName string) (*dto.GetOrCreateClientResult, error)
}

// ── Maintenance WRITE ports (ADR-0015, T3) ──
//
// The eight use cases behind the operational maintenance tools. Every one of
// them re-asserts auth.RequireRole(RoleOwner) after re-injecting the caller,
// and every one has an owner-only ToolRBAC entry in the composition root: the
// transport gate and the use-case gate are both required. On a nil error the
// returned entity/result is never nil (the transport still fails closed).

// UpdateBusinessProfilePort applies a partial merge to the singleton business
// profile and returns it — the same shape get_business_profile reads.
// ToolRBAC entry: owner only.
type UpdateBusinessProfilePort interface {
	Execute(context.Context, dto.UpdateBusinessProfileInput) (*entity.BusinessProfile, error)
}

// CreateServicePort inserts a catalog service and returns it as stored.
// ToolRBAC entry: owner only.
type CreateServicePort interface {
	Execute(context.Context, dto.CreateServiceInput) (*entity.Service, error)
}

// UpdateServicePort applies a partial merge to a catalog service and returns
// it as stored. ToolRBAC entry: owner only.
type UpdateServicePort interface {
	Execute(context.Context, dto.UpdateServiceInput) (*entity.Service, error)
}

// DeleteServicePort removes a catalog service.
// ToolRBAC entry: owner only.
type DeleteServicePort interface {
	Execute(context.Context, dto.DeleteServiceInput) (*dto.DeleteServiceResult, error)
}

// CreateProfessionalPort inserts a staff member and returns it as stored.
// ToolRBAC entry: owner only.
type CreateProfessionalPort interface {
	Execute(context.Context, dto.CreateProfessionalInput) (*entity.Professional, error)
}

// UpdateProfessionalPort applies a partial merge to a staff member and returns
// it as stored. ToolRBAC entry: owner only.
type UpdateProfessionalPort interface {
	Execute(context.Context, dto.UpdateProfessionalInput) (*entity.Professional, error)
}

// UpsertSchedulePort inserts or replaces one weekly slot and returns the row as
// stored. ToolRBAC entry: owner only.
type UpsertSchedulePort interface {
	Execute(context.Context, dto.UpsertScheduleInput) (*entity.Schedule, error)
}

// DeleteSchedulePort removes one weekly slot.
// ToolRBAC entry: owner only.
type DeleteSchedulePort interface {
	Execute(context.Context, dto.DeleteScheduleInput) (*dto.DeleteScheduleResult, error)
}

// ── Business-hours exception ports (feat-hours-exception-tool, PR2) ──
//
// The three owner-only tools over the date-specific business-hours overrides.
// They reuse the shared BusinessHoursExceptionRepo as-is (no repository or
// schema change) and, like the maintenance writes above, each use case
// re-asserts auth.RequireRole(RoleOwner) after re-injecting the caller while
// the composition root keeps its own ToolRBAC entry (double enforcement).

// CreateHoursExceptionPort adds an exception and returns the stored row read
// back by its unique date (the repository's Create never exposes the
// AUTOINCREMENT id). ToolRBAC entry: owner only.
type CreateHoursExceptionPort interface {
	Execute(context.Context, dto.CreateHoursExceptionInput) (*entity.BusinessHoursException, error)
}

// DeleteHoursExceptionPort removes an exception by its integer id.
// ToolRBAC entry: owner only.
type DeleteHoursExceptionPort interface {
	Execute(context.Context, dto.DeleteHoursExceptionInput) (*dto.DeleteHoursExceptionResult, error)
}

// ListHoursExceptionsPort reads the exceptions in an inclusive [from, to] range
// ordered by exception_date ASC; an empty range is an empty slice, never an
// error. ToolRBAC entry: owner only.
type ListHoursExceptionsPort interface {
	Execute(context.Context, dto.ListHoursExceptionsInput) ([]*entity.BusinessHoursException, error)
}
