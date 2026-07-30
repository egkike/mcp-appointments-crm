package entity

import "testing"

func TestAccount_IsActive(t *testing.T) {
	tests := []struct {
		name     string
		isActive bool
		want     bool
	}{
		{"active account", true, true},
		{"inactive account", false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Active: tt.isActive}
			if got := a.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAccount_HasRole(t *testing.T) {
	tests := []struct {
		name  string
		role  string
		check string
		want  bool
	}{
		{"owner matches owner", "owner", "owner", true},
		{"admin matches admin", "admin", "admin", true},
		{"staff matches staff", "staff", "staff", true},
		{"owner does not match admin", "owner", "admin", false},
		{"client does not match staff", "client", "staff", false},
		{"empty role does not match", "", "owner", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &Account{Role: tt.role}
			if got := a.HasRole(tt.check); got != tt.want {
				t.Errorf("HasRole(%q) = %v, want %v", tt.check, got, tt.want)
			}
		})
	}
}
