package usecase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// AlertLifecycleStore is the narrow port used by booking use cases to emit
// confirmation alerts without depending on the full PendingAlertsRepo.
// Declared in the consumer package (internal/application/usecase) per the
// accept-interfaces-where-used idiom.
type AlertLifecycleStore interface {
	// InsertForBooking persists a confirmation alert for a newly created booking.
	InsertForBooking(ctx context.Context, a *entity.PendingAlert) error
	// CancelByBookingID cancels any pending alert linked to the given booking.
	CancelByBookingID(ctx context.Context, bookingID string) error
}

// AlertBuilder formats the human-readable confirmation message for a booking.
// It keeps time-formatting logic in a single pure helper so unit tests can
// assert the exact UTC string without touching a database.
type AlertBuilder struct{}

// BuildConfirmationMessage returns the Fase 1 Paso-5 confirmation message.
// The message includes the client and professional names plus the UTC datetime
// in ISO 8601 format to avoid timezone ambiguity in the notification channel.
func (AlertBuilder) BuildConfirmationMessage(clientName, proName string, start time.Time) string {
	return fmt.Sprintf("Confirmar reserva de %s con %s el %s",
		clientName, proName, start.UTC().Format(time.RFC3339))
}

// ConfirmationAlertFor creates a pending alert for a booking mutation.
// start is the booking start used in the message; scheduled is the time the
// alert becomes due (typically the mutation time, not the booking start).
func ConfirmationAlertFor(clientName, proName, bookingID string, start, scheduled time.Time) *entity.PendingAlert {
	msg := AlertBuilder{}.BuildConfirmationMessage(clientName, proName, start)
	return &entity.PendingAlert{
		Type:              "confirmation_requested",
		Message:           msg,
		ScheduledDatetime: scheduled.UTC(),
		Status:            "pending",
		RelatedBookingID:  &bookingID,
	}
}

// LogAlertFailure logs an failed alert operation without propagating the error.
// Booking use cases MUST call this helper so that alert failures never roll back
// the booking mutation (REQ-PA-LIFE-001).
func LogAlertFailure(logger *slog.Logger, op string, bookingID string, err error) {
	if logger == nil {
		logger = slog.Default()
	}
	logger.Error("alert lifecycle failure: booking mutation succeeded",
		"operation", op,
		"booking_id", bookingID,
		"error", err,
	)
}

// EnsurePendingAlertsRepo satisfies the consumer-side AlertLifecycleStore interface
// using the concrete repository. It is a thin adapter so the use case only depends
// on the port, not the repository type.
type EnsurePendingAlertsRepo struct {
	repo domainrepo.PendingAlertsRepo
}

// NewEnsurePendingAlertsRepo creates the adapter.
func NewEnsurePendingAlertsRepo(repo domainrepo.PendingAlertsRepo) *EnsurePendingAlertsRepo {
	return &EnsurePendingAlertsRepo{repo: repo}
}

// InsertForBooking delegates to the repository.
func (a *EnsurePendingAlertsRepo) InsertForBooking(ctx context.Context, pending *entity.PendingAlert) error {
	return a.repo.InsertForBooking(ctx, pending)
}

// CancelByBookingID delegates to the repository.
func (a *EnsurePendingAlertsRepo) CancelByBookingID(ctx context.Context, bookingID string) error {
	return a.repo.CancelByBookingID(ctx, bookingID)
}
