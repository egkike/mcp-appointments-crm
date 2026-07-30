package entity

import (
	"testing"
	"time"
)

func TestPendingAlert_IsDue(t *testing.T) {
	past := time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC)
	future := time.Date(2026, 12, 31, 23, 0, 0, 0, time.UTC)
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		when time.Time
		now  time.Time
		want bool
	}{
		{"past alert is due", past, now, true},
		{"exact now is due", now, now, true},
		{"future alert is not due", future, now, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &PendingAlert{ScheduledDatetime: tt.when}
			if got := a.IsDue(tt.now); got != tt.want {
				t.Errorf("IsDue() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestPendingAlert_CanBeSent(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"pending can be sent", "pending", true},
		{"sent cannot be sent again", "sent", false},
		{"cancelled cannot be sent", "cancelled", false},
		{"empty status cannot be sent", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &PendingAlert{Status: tt.status}
			if got := a.CanBeSent(); got != tt.want {
				t.Errorf("CanBeSent() = %v, want %v", got, tt.want)
			}
		})
	}
}
