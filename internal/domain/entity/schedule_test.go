package entity

import "testing"

func TestSchedule_IncludesTime(t *testing.T) {
	s := &Schedule{
		ProfessionalID: "prof-1",
		DayOfWeek:      1,
		StartTime:      "09:00",
		EndTime:        "18:00",
	}

	tests := []struct {
		name string
		hhmm string
		want bool
	}{
		{"start time included", "09:00", true},
		{"end time excluded", "18:00", false},
		{"mid-morning included", "12:30", true},
		{"before start excluded", "08:59", false},
		{"after end excluded", "18:01", false},
		{"one minute before end included", "17:59", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := s.IncludesTime(tt.hhmm); got != tt.want {
				t.Errorf("IncludesTime(%q) = %v, want %v", tt.hhmm, got, tt.want)
			}
		})
	}
}
