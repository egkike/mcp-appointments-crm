// Domain port for the persisted account audit trail.
//
// The TUI audit view reads through this port so the presentation never calls
// the concrete adapter (ADR-0013: dependencies point inward, ports live in
// domain). The WRITE side deliberately has no port: the audit rows are emitted
// from inside the repository layer at the exact slog emission sites, where the
// account row and the actor reference already exist (design §1.4).
package repository

import (
	"context"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// AccountAuditReader reads the append-only account audit trail, newest first.
// Implementations clamp the limit defensively; the caller passes its desired
// page (the TUI view asks for the OQ-6 default).
type AccountAuditReader interface {
	LastAccountsAudit(ctx context.Context, limit int) ([]*entity.AccountAuditEvent, error)
}
