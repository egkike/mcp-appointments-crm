package usecase

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// Maintenance mutation audit trail (ADR-0015: a new write surface widens the
// blast radius of a misdirected or compromised LLM, so critical mutations must
// leave a structured audit record).
//
// Exactly ONE record is emitted per successful mutation, at the use-case layer:
// the maintenance repositories are logger-free by design (they only take
// *sql.DB). Attribute naming mirrors repository.auditAttrs (accounts.go): an
// RFC3339Nano "ts" plus the operation identifiers.
//
// No caller ID is logged. Raw caller IDs are PII (phone numbers/emails) and the
// auth middleware already emits a "privileged access" record with a hashed
// caller reference per request; this record adds the operation detail the
// middleware cannot know.
const maintenanceAuditMessage = "maintenance mutation"

// maintenanceLogger normalizes the optional logger dependency. A nil logger
// falls back to slog.Default() (same fail-safe as auth.NewAuthMiddleware) so a
// wiring mistake degrades to the default sink instead of nil-dereferencing on
// the mutation path.
func maintenanceLogger(logger *slog.Logger) *slog.Logger {
	if logger == nil {
		return slog.Default()
	}
	return logger
}

// logMaintenanceMutation emits the single structured audit record of a
// successful maintenance mutation.
//
//   - tool: the MCP operation name (e.g. "update_service").
//   - entity: the touched aggregate ("business_profile", "service",
//     "professional", "schedule").
//   - entityID: the row identifier when the operation addresses one identity.
//     Schedule mutations have a composite key and pass
//     "<professional_id>:<day_of_week>" (see maintenanceScheduleID).
//   - action: "create", "update", "delete" or "upsert".
//   - role: the authorizing role (owner for this MVP).
func logMaintenanceMutation(logger *slog.Logger, caller *auth.Caller, tool, entity, entityID, action string) {
	role := ""
	if caller != nil {
		role = caller.Role
	}
	attrs := make([]any, 0, 12)
	attrs = append(attrs, "tool", tool, "entity", entity)
	if entityID != "" {
		attrs = append(attrs, "entity_id", entityID)
	}
	attrs = append(attrs, "action", action, "role", role, "ts", time.Now().UTC().Format(time.RFC3339Nano))
	logger.Info(maintenanceAuditMessage, attrs...)
}

// maintenanceScheduleID renders the composite identity of a weekly schedule row
// (schedules has no surrogate key exposed to callers) as the audit entity_id.
func maintenanceScheduleID(professionalID string, dayOfWeek int) string {
	return fmt.Sprintf("%s:%d", professionalID, dayOfWeek)
}
