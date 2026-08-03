package repository

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// Compile-time interface conformance check.
var _ domainrepo.PendingAlertsRepo = (*PendingAlertsRepo)(nil)

// PendingAlertsRepo provides CRUD operations for the pending_alerts table.
// In Fase 1, only the "confirmation_requested" alert type is supported.
type PendingAlertsRepo struct {
	db *sql.DB
}

// NewPendingAlertsRepo creates a new PendingAlertsRepo.
func NewPendingAlertsRepo(db *sql.DB) *PendingAlertsRepo {
	return &PendingAlertsRepo{db: db}
}

// allowedAlertTypesFase1 is the allowlist of alert types supported in Fase 1.
var allowedAlertTypesFase1 = map[string]bool{
	"confirmation_requested": true,
}

// validateAlertType checks that the alert type is supported in Fase 1.
func validateAlertType(alertType string) error {
	if !allowedAlertTypesFase1[alertType] {
		return fmt.Errorf("tipo de alerta %q no soportado en Fase 1; sólo 'confirmation_requested': %w",
			alertType, domain.ErrInvalidInput)
	}
	return nil
}

// Save inserts a new pending alert. The ID is auto-assigned by SQLite AUTOINCREMENT.
// Status defaults to "pending". RelatedBookingID may be nil.
// Returns domain.ErrInvalidInput if the alert type is not supported in Fase 1 or message is empty.
// Requires admin or owner role.
func (r *PendingAlertsRepo) Save(ctx context.Context, a *entity.PendingAlert) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("crear alerta: %w", err)
	}

	if err := validateAlertType(a.Type); err != nil {
		return fmt.Errorf("crear alerta: %w", err)
	}
	if strings.TrimSpace(a.Message) == "" {
		return fmt.Errorf("crear alerta: el mensaje no puede estar vacío: %w", domain.ErrInvalidInput)
	}

	a.Status = "pending"
	scheduledStr := FormatStorage(a.ScheduledDatetime)

	result, err := r.db.ExecContext(ctx,
		`INSERT INTO pending_alerts (type, message, scheduled_datetime, related_booking_id)
		 VALUES (?, ?, ?, ?)`,
		a.Type, a.Message, scheduledStr, a.RelatedBookingID,
	)
	if err != nil {
		return fmt.Errorf("crear alerta: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("crear alerta: obtener ID: %w", err)
	}
	a.ID = int(id)

	return nil
}

// FindPending returns all alerts whose scheduled time is at or before now
// and whose status is still pending, ordered by scheduled_datetime ASC.
// Returns an empty slice (not nil) when no alerts match.
// Requires admin or owner role.
func (r *PendingAlertsRepo) FindPending(ctx context.Context, now time.Time) ([]*entity.PendingAlert, error) {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return nil, fmt.Errorf("listar alertas pendientes: %w", err)
	}

	nowStr := FormatStorage(now)
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, type, message, scheduled_datetime, status, related_booking_id, created_at
		 FROM pending_alerts
		 WHERE status = 'pending' AND scheduled_datetime <= ?
		 ORDER BY scheduled_datetime ASC`,
		nowStr,
	)
	if err != nil {
		return nil, fmt.Errorf("listar alertas pendientes: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	alerts := make([]*entity.PendingAlert, 0)
	for rows.Next() {
		a := &entity.PendingAlert{}
		var scheduledStr, createdAtStr string
		if err := rows.Scan(&a.ID, &a.Type, &a.Message, &scheduledStr,
			&a.Status, &a.RelatedBookingID, &createdAtStr); err != nil {
			return nil, fmt.Errorf("listar alertas pendientes: escaneo: %w", err)
		}
		t, err := parseStorageTime(scheduledStr)
		if err != nil {
			return nil, fmt.Errorf("listar alertas pendientes: parse scheduled_datetime: %w", err)
		}
		a.ScheduledDatetime = t
		a.CreatedAt = createdAtStr
		alerts = append(alerts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar alertas pendientes: iteración: %w", err)
	}
	return alerts, nil
}

// MarkAsSent transitions a pending alert to "sent" status.
// Idempotent: marking an already-sent or cancelled alert is a no-op (returns nil).
// Requires admin or owner role.
func (r *PendingAlertsRepo) MarkAsSent(ctx context.Context, id int) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("marcar alerta %d como enviada: %w", id, err)
	}

	_, err := r.db.ExecContext(ctx,
		`UPDATE pending_alerts SET status = 'sent' WHERE id = ? AND status = 'pending'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("marcar alerta %d como enviada: %w", id, err)
	}
	// RowsAffected == 0 means alert was already sent or cancelled → no-op, not an error
	return nil
}

// Cancel transitions a pending alert to "cancelled" status.
// Idempotent: cancelling an already-cancelled or sent alert is a no-op (returns nil).
// Requires admin or owner role.
func (r *PendingAlertsRepo) Cancel(ctx context.Context, id int) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("cancelar alerta %d: %w", id, err)
	}

	_, err := r.db.ExecContext(ctx,
		`UPDATE pending_alerts SET status = 'cancelled' WHERE id = ? AND status = 'pending'`,
		id,
	)
	if err != nil {
		return fmt.Errorf("cancelar alerta %d: %w", id, err)
	}
	// RowsAffected == 0 means alert was already cancelled or sent → no-op, not an error
	return nil
}
