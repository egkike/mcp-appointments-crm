package entity

import "testing"

func TestBusinessHoursException_IsClosed(t *testing.T) {
	tests := []struct {
		name     string
		isClosed bool
		want     bool
	}{
		{"closed exception", true, true},
		{"open exception", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &BusinessHoursException{IsClosed: tt.isClosed}
			if got := e.IsClosedDay(); got != tt.want {
				t.Errorf("IsClosedDay() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestBusinessHoursException_EffectiveHours(t *testing.T) {
	tests := []struct {
		name      string
		exc       *BusinessHoursException
		wantOpen  string
		wantClose string
		wantOK    bool
	}{
		{
			name:      "closed day returns empty",
			exc:       &BusinessHoursException{IsClosed: true},
			wantOpen:  "",
			wantClose: "",
			wantOK:    false,
		},
		{
			name:      "open with hours",
			exc:       &BusinessHoursException{IsClosed: false, OpenTime: strPtr("10:00"), CloseTime: strPtr("14:00")},
			wantOpen:  "10:00",
			wantClose: "14:00",
			wantOK:    true,
		},
		{
			name:      "open but nil times returns not ok",
			exc:       &BusinessHoursException{IsClosed: false, OpenTime: nil, CloseTime: nil},
			wantOpen:  "",
			wantClose: "",
			wantOK:    false,
		},
		{
			name:      "open with only open time returns not ok",
			exc:       &BusinessHoursException{IsClosed: false, OpenTime: strPtr("10:00"), CloseTime: nil},
			wantOpen:  "",
			wantClose: "",
			wantOK:    false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			open, close, ok := tt.exc.EffectiveHours()
			if ok != tt.wantOK {
				t.Errorf("EffectiveHours() ok = %v, want %v", ok, tt.wantOK)
			}
			if open != tt.wantOpen {
				t.Errorf("EffectiveHours() open = %q, want %q", open, tt.wantOpen)
			}
			if close != tt.wantClose {
				t.Errorf("EffectiveHours() close = %q, want %q", close, tt.wantClose)
			}
		})
	}
}
