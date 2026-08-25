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
