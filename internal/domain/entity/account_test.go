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
		role  AccountRole
		check AccountRole
		want  bool
	}{
		{"owner matches owner", RoleOwner, RoleOwner, true},
		{"admin matches admin", RoleAdmin, RoleAdmin, true},
		{"staff matches staff", RoleStaff, RoleStaff, true},
		{"owner does not match admin", RoleOwner, RoleAdmin, false},
		{"client does not match staff", AccountRole("client"), RoleStaff, false},
		{"empty role does not match", AccountRole(""), RoleOwner, false},
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

func TestAccount_Validate(t *testing.T) {
	profID := "prof-1"
	emptyProfID := ""
	tests := []struct {
		name    string
		account *Account
		wantErr bool
	}{
		{
			name:    "valid owner account",
			account: &Account{ID: "user-1", Role: RoleOwner},
		},
		{
			name:    "valid admin account",
			account: &Account{ID: "user-2", Role: RoleAdmin},
		},
		{
			name:    "valid staff account with professional_id",
			account: &Account{ID: "user-3", Role: RoleStaff, ProfessionalID: &profID},
		},
		{
			name:    "empty ID",
			account: &Account{ID: "", Role: RoleAdmin},
			wantErr: true,
		},
		{
			name:    "invalid role",
			account: &Account{ID: "user-4", Role: AccountRole("client")},
			wantErr: true,
		},
		{
			name:    "staff without professional_id (nil)",
			account: &Account{ID: "user-5", Role: RoleStaff},
			wantErr: true,
		},
		{
			name:    "staff with empty professional_id",
			account: &Account{ID: "user-6", Role: RoleStaff, ProfessionalID: &emptyProfID},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.account.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
