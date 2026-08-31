package dto

import "github.com/egkike/mcp-appointments-crm/internal/auth"

// PendingAlertView is the public representation of a pending alert.
type PendingAlertView struct {
	AlertID           int     `json:"alert_id"`
	Type              string  `json:"type"`
	Message           string  `json:"message"`
	ScheduledDatetime string  `json:"scheduled_datetime"`
	RelatedBookingID  *string `json:"related_booking_id,omitempty"`
	CreatedAt         string  `json:"created_at"`
}

// GetPendingAlertsInput carries the authenticated caller for the
// get_pending_alerts use case.
type GetPendingAlertsInput struct {
	Caller auth.Caller
}

// GetPendingAlertsResult is the response body for get_pending_alerts.
type GetPendingAlertsResult struct {
	Alerts []PendingAlertView `json:"alerts"`
}

// MarkAlertAsSentInput carries the authenticated caller and the alert ID.
type MarkAlertAsSentInput struct {
	Caller  auth.Caller
	AlertID int
}

// MarkAlertAsSentResult is the response body for mark_alert_as_sent.
type MarkAlertAsSentResult struct {
	AlertID int    `json:"alert_id"`
	Status  string `json:"status"`
}
