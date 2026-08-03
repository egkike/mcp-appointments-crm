package entity

import "testing"

func TestProfessional_IsActive(t *testing.T) {
	tests := []struct {
		name   string
		status string
		want   bool
	}{
		{"active status", "active", true},
		{"inactive status", "inactive", false},
		{"empty status", "", false},
		{"unknown status", "suspended", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Professional{Status: tt.status}
			if got := p.IsActive(); got != tt.want {
				t.Errorf("IsActive() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestProfessional_HasSpecialty(t *testing.T) {
	specialties := `["cutting","coloring","styling"]`
	tests := []struct {
		name      string
		specialty *string
		serviceID string
		want      bool
	}{
		{
			name:      "matching specialty",
			specialty: strPtr(specialties),
			serviceID: "coloring",
			want:      true,
		},
		{
			name:      "no match",
			specialty: strPtr(specialties),
			serviceID: "manicure",
			want:      false,
		},
		{
			name:      "nil specialties",
			specialty: nil,
			serviceID: "cutting",
			want:      false,
		},
		{
			name:      "empty specialties",
			specialty: strPtr(""),
			serviceID: "cutting",
			want:      false,
		},
		{
			name:      "first item match",
			specialty: strPtr(specialties),
			serviceID: "cutting",
			want:      true,
		},
		{
			name:      "last item match",
			specialty: strPtr(specialties),
			serviceID: "styling",
			want:      true,
		},
		{
			name:      "invalid JSON returns false",
			specialty: strPtr("not valid json"),
			serviceID: "cutting",
			want:      false,
		},
		{
			name:      "single-item array match",
			specialty: strPtr(`["only"]`),
			serviceID: "only",
			want:      true,
		},
		{
			name:      "single-item array no match",
			specialty: strPtr(`["only"]`),
			serviceID: "other",
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Professional{Specialties: tt.specialty}
			if got := p.HasSpecialty(tt.serviceID); got != tt.want {
				t.Errorf("HasSpecialty(%q) = %v, want %v", tt.serviceID, got, tt.want)
			}
		})
	}
}

func strPtr(s string) *string { return &s }
