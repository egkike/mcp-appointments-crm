package entity

import (
	"testing"
	"time"
)

func TestService_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		active bool
		want   bool
	}{
		{"active service", true, true},
		{"inactive service", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{Active: tt.active}
			if got := s.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestService_Duration(t *testing.T) {
	tests := []struct {
		name            string
		durationMinutes int
		want            time.Duration
	}{
		{"30 minutes", 30, 30 * time.Minute},
		{"60 minutes", 60, 60 * time.Minute},
		{"90 minutes", 90, 90 * time.Minute},
		{"zero minutes returns zero", 0, 0},
		{"negative minutes returns zero", -15, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := &Service{DurationMinutes: tt.durationMinutes}
			if got := s.Duration(); got != tt.want {
				t.Errorf("Duration() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestService_Validate(t *testing.T) {
	tests := []struct {
		name    string
		svc     *Service
		wantErr bool
	}{
		{
			name: "valid service",
			svc:  &Service{Name: "Corte de pelo", DurationMinutes: 30, Price: 1500.0},
		},
		{
			name:    "empty name",
			svc:     &Service{Name: "", DurationMinutes: 30, Price: 1500.0},
			wantErr: true,
		},
		{
			name:    "whitespace-only name",
			svc:     &Service{Name: "   ", DurationMinutes: 30, Price: 1500.0},
			wantErr: true,
		},
		{
			name:    "zero duration",
			svc:     &Service{Name: "Corte", DurationMinutes: 0, Price: 1500.0},
			wantErr: true,
		},
		{
			name:    "negative duration",
			svc:     &Service{Name: "Corte", DurationMinutes: -10, Price: 1500.0},
			wantErr: true,
		},
		{
			name:    "zero price",
			svc:     &Service{Name: "Corte", DurationMinutes: 30, Price: 0},
			wantErr: true,
		},
		{
			name:    "negative price",
			svc:     &Service{Name: "Corte", DurationMinutes: 30, Price: -100},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.svc.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
