package entity

import "time"

// PendingAlert represents a queued notification to be sent to a client.
type PendingAlert struct {
	ID                int
	Type              string
	Message           string
	ScheduledDatetime time.Time
	Status            string
	RelatedBookingID  *string
	CreatedAt         string
}

// IsDue reports whether the alert's scheduled time has arrived or passed
// relative to the given reference time (typically time.Now()).
func (a *PendingAlert) IsDue(now time.Time) bool {
	return !a.ScheduledDatetime.After(now)
}

// CanBeSent reports whether the alert is in a sendable state.
// Only alerts with status "pending" can be sent.
func (a *PendingAlert) CanBeSent() bool {
	return a.Status == "pending"
}
