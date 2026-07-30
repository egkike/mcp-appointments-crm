package repository

import (
	"context"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// PendingAlertsRepo defines the persistence contract for queued notifications.
type PendingAlertsRepo interface {
	// Save inserts a new pending alert.
	Save(ctx context.Context, a *entity.PendingAlert) error

	// FindPending returns all alerts whose scheduled time is at or before now
	// and whose status is still pending.
	FindPending(ctx context.Context, now time.Time) ([]*entity.PendingAlert, error)

	// MarkAsSent transitions the alert to sent status by ID.
	MarkAsSent(ctx context.Context, id int) error

	// Cancel transitions the alert to cancelled status by ID.
	Cancel(ctx context.Context, id int) error
}
