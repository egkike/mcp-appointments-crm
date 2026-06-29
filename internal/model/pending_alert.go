package model

// PendingAlert represents a queued notification to be sent to a client.
type PendingAlert struct {
	ID                int     `json:"id"`
	Type              string  `json:"type"`
	Message           string  `json:"message"`
	ScheduledDatetime string  `json:"scheduled_datetime"`
	Status            string  `json:"status"`
	RelatedBookingID  *string `json:"related_booking_id"`
	CreatedAt         string  `json:"created_at"`
}
