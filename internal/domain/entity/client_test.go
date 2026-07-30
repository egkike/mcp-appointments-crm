package entity

import "testing"

func TestClient_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		active bool
		want   bool
	}{
		{"active client", true, true},
		{"inactive client", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{Active: tt.active}
			if got := c.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestClient_HasValidPhone(t *testing.T) {
	tests := []struct {
		name  string
		phone string
		want  bool
	}{
		{"valid E.164 with plus", "+5491155554444", true},
		{"valid digits only", "1234567890", true},
		{"valid minimum length", "+1234", true},
		{"too short", "123", false},
		{"empty phone", "", false},
		{"contains letters", "+54abc1234", false},
		{"contains spaces", "+54 11 5555", false},
		{"plus in middle", "54+911", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := &Client{Phone: tt.phone}
			if got := c.HasValidPhone(); got != tt.want {
				t.Errorf("HasValidPhone() = %v, want %v", got, tt.want)
			}
		})
	}
}
