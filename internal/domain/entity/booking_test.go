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
