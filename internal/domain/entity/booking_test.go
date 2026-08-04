package entity

import (
	"testing"
	"time"
)

func TestBooking_CanCancel(t *testing.T) {
	tests := []struct {
		name   string
		status BookingStatus
		want   bool
	}{
		{"pending allows cancel", BookingStatusPending, true},
		{"confirmed allows cancel", BookingStatusConfirmed, true},
		{"cancelled does not allow cancel", BookingStatusCancelled, false},
		{"unknown status does not allow cancel", BookingStatus("unknown"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Booking{Status: tt.status}
			if got := b.CanCancel(); got != tt.want {
				t.Errorf("CanCancel() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBooking_CanReschedule(t *testing.T) {
	tests := []struct {
		name   string
		status BookingStatus
		want   bool
	}{
		{"pending allows reschedule", BookingStatusPending, true},
		{"confirmed allows reschedule", BookingStatusConfirmed, true},
		{"cancelled does not allow reschedule", BookingStatusCancelled, false},
		{"unknown status does not allow reschedule", BookingStatus("draft"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Booking{Status: tt.status}
			if got := b.CanReschedule(); got != tt.want {
				t.Errorf("CanReschedule() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBookingStatus_ValidTransitions(t *testing.T) {
	tests := []struct {
		name   string
		status BookingStatus
		want   []BookingStatus
	}{
		{
			name:   "pending transitions to confirmed and cancelled",
			status: BookingStatusPending,
			want:   []BookingStatus{BookingStatusConfirmed, BookingStatusCancelled},
		},
		{
			name:   "confirmed transitions to cancelled only",
			status: BookingStatusConfirmed,
			want:   []BookingStatus{BookingStatusCancelled},
		},
		{
			name:   "cancelled has no transitions",
			status: BookingStatusCancelled,
			want:   nil,
		},
		{
			name:   "unknown status has no transitions",
			status: BookingStatus("draft"),
			want:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.status.ValidTransitions()
			if len(got) != len(tt.want) {
				t.Errorf("ValidTransitions() = %v (len=%d), want %v (len=%d)", got, len(got), tt.want, len(tt.want))
				return
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("ValidTransitions()[%d] = %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestBookingStatus_IsValidTransition(t *testing.T) {
	tests := []struct {
		name   string
		from   BookingStatus
		target BookingStatus
		want   bool
	}{
		{"pending to confirmed", BookingStatusPending, BookingStatusConfirmed, true},
		{"pending to cancelled", BookingStatusPending, BookingStatusCancelled, true},
		{"pending to pending (self)", BookingStatusPending, BookingStatusPending, false},
		{"confirmed to cancelled", BookingStatusConfirmed, BookingStatusCancelled, true},
		{"confirmed to pending (backward)", BookingStatusConfirmed, BookingStatusPending, false},
		{"cancelled to anything", BookingStatusCancelled, BookingStatusConfirmed, false},
		{"unknown to anything", BookingStatus("draft"), BookingStatusConfirmed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.from.IsValidTransition(tt.target); got != tt.want {
				t.Errorf("IsValidTransition(%q) from %q = %v, want %v", tt.target, tt.from, got, tt.want)
			}
		})
	}
}

func TestBooking_CanTransitionTo(t *testing.T) {
	tests := []struct {
		name   string
		status BookingStatus
		target BookingStatus
		want   bool
	}{
		{"pending to confirmed", BookingStatusPending, BookingStatusConfirmed, true},
		{"pending to cancelled", BookingStatusPending, BookingStatusCancelled, true},
		{"pending to pending", BookingStatusPending, BookingStatusPending, false},
		{"confirmed to cancelled", BookingStatusConfirmed, BookingStatusCancelled, true},
		{"confirmed to pending (backward)", BookingStatusConfirmed, BookingStatusPending, false},
		{"cancelled to confirmed (dead)", BookingStatusCancelled, BookingStatusConfirmed, false},
		{"unknown to anything", BookingStatus("draft"), BookingStatusConfirmed, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Booking{Status: tt.status}
			if got := b.CanTransitionTo(tt.target); got != tt.want {
				t.Errorf("CanTransitionTo(%q) from %q = %v, want %v", tt.target, tt.status, got, tt.want)
			}
		})
	}
}

func TestBooking_ValidDuration(t *testing.T) {
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name            string
		start           time.Time
		end             time.Time
		serviceDuration time.Duration
		want            bool
	}{
		{
			name:            "exact match with 60min service",
			start:           base,
			end:             base.Add(60 * time.Minute),
			serviceDuration: 60 * time.Minute,
			want:            true,
		},
		{
			name:            "exact match with 30min service",
			start:           base,
			end:             base.Add(30 * time.Minute),
			serviceDuration: 30 * time.Minute,
			want:            true,
		},
		{
			name:            "duration mismatch shorter than service",
			start:           base,
			end:             base.Add(30 * time.Minute),
			serviceDuration: 60 * time.Minute,
			want:            false,
		},
		{
			name:            "duration mismatch longer than service",
			start:           base,
			end:             base.Add(90 * time.Minute),
			serviceDuration: 60 * time.Minute,
			want:            false,
		},
		{
			name:            "zero duration (equal times)",
			start:           base,
			end:             base,
			serviceDuration: 60 * time.Minute,
			want:            false,
		},
		{
			name:            "negative duration (end before start)",
			start:           base.Add(60 * time.Minute),
			end:             base,
			serviceDuration: 60 * time.Minute,
			want:            false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Booking{StartDatetime: tt.start, EndDatetime: tt.end}
			if got := b.ValidDuration(tt.serviceDuration); got != tt.want {
				t.Errorf("ValidDuration(%v) with range [%v, %v] = %v, want %v",
					tt.serviceDuration, tt.start, tt.end, got, tt.want)
			}
		})
	}
}

func TestBooking_IsValidTimeRange(t *testing.T) {
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name  string
		start time.Time
		end   time.Time
		want  bool
	}{
		{
			name:  "valid range: start before end",
			start: base,
			end:   base.Add(1 * time.Hour),
			want:  true,
		},
		{
			name:  "valid range: exact service slot",
			start: base,
			end:   base.Add(30 * time.Minute),
			want:  true,
		},
		{
			name:  "zero start time",
			start: time.Time{},
			end:   base,
			want:  false,
		},
		{
			name:  "zero end time",
			start: base,
			end:   time.Time{},
			want:  false,
		},
		{
			name:  "both zero",
			start: time.Time{},
			end:   time.Time{},
			want:  false,
		},
		{
			name:  "start equals end (zero duration)",
			start: base,
			end:   base,
			want:  false,
		},
		{
			name:  "end before start (negative)",
			start: base.Add(1 * time.Hour),
			end:   base,
			want:  false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := &Booking{StartDatetime: tt.start, EndDatetime: tt.end}
			if got := b.IsValidTimeRange(); got != tt.want {
				t.Errorf("IsValidTimeRange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBooking_IsOverlapping(t *testing.T) {
	base := time.Date(2026, 8, 1, 10, 0, 0, 0, time.UTC)
	end := base.Add(1 * time.Hour) // 11:00

	b := &Booking{
		ProfessionalID: "prof-1",
		StartDatetime:  base,
		EndDatetime:    end,
	}

	tests := []struct {
		name  string
		other *Booking
		want  bool
	}{
		{
			name: "overlapping same professional",
			other: &Booking{
				ProfessionalID: "prof-1",
				StartDatetime:  base.Add(30 * time.Minute),
				EndDatetime:    base.Add(90 * time.Minute),
			},
			want: true,
		},
		{
			name: "no overlap different professional",
			other: &Booking{
				ProfessionalID: "prof-2",
				StartDatetime:  base.Add(30 * time.Minute),
				EndDatetime:    base.Add(90 * time.Minute),
			},
			want: false,
		},
		{
			name: "adjacent not overlapping",
			other: &Booking{
				ProfessionalID: "prof-1",
				StartDatetime:  end,                    // starts exactly when b ends
				EndDatetime:    end.Add(1 * time.Hour), // 11:00–12:00
			},
			want: false,
		},
		{
			name: "completely after",
			other: &Booking{
				ProfessionalID: "prof-1",
				StartDatetime:  base.Add(2 * time.Hour),
				EndDatetime:    base.Add(3 * time.Hour),
			},
			want: false,
		},
		{
			name: "contained within",
			other: &Booking{
				ProfessionalID: "prof-1",
				StartDatetime:  base.Add(15 * time.Minute),
				EndDatetime:    base.Add(45 * time.Minute),
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := b.IsOverlapping(tt.other); got != tt.want {
				t.Errorf("IsOverlapping() = %v, want %v", got, tt.want)
			}
		})
	}
}
