package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

func TestRequireCaller(t *testing.T) {
	t.Run("no caller in context returns SemanticError with ErrCodeUnauthenticated", func(t *testing.T) {
		_, err := RequireCaller(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sErr *domain.SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
		}
		if !errors.Is(sErr, domain.ErrUnauthenticated) {
			t.Errorf("expected errors.Is(domain.ErrUnauthenticated) to be true")
		}
	})

	t.Run("caller present returns pointer to caller", func(t *testing.T) {
		c := Caller{ID: "+5491155554444", Role: RoleAdmin}
		ctx := WithCaller(context.Background(), c)

		got, err := RequireCaller(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != c.ID {
			t.Errorf("got ID=%q, want %q", got.ID, c.ID)
		}
		if got.Role != RoleAdmin {
			t.Errorf("got Role=%q, want %q", got.Role, RoleAdmin)
		}
	})
}

func TestRequireRole(t *testing.T) {
	tests := []struct {
		name     string
		caller   *Caller
		allowed  []string
		wantErr  bool
		wantCode domain.ErrCode
	}{
		{
			name:    "admin allowed when admin is in set",
			caller:  &Caller{ID: "a-1", Role: RoleAdmin},
			allowed: []string{RoleAdmin, RoleOwner},
			wantErr: false,
		},
		{
			name:    "owner allowed when owner is in set",
			caller:  &Caller{ID: "o-1", Role: RoleOwner},
			allowed: []string{RoleAdmin, RoleOwner},
			wantErr: false,
		},
		{
			name:     "client rejected when only admin/owner allowed",
			caller:   &Caller{ID: "c-1", Role: RoleClient},
			allowed:  []string{RoleAdmin, RoleOwner},
			wantErr:  true,
			wantCode: domain.ErrCodeUnauthenticated,
		},
		{
			name:     "staff rejected when only admin/owner allowed",
			caller:   &Caller{ID: "s-1", Role: RoleStaff},
			allowed:  []string{RoleAdmin, RoleOwner},
			wantErr:  true,
			wantCode: domain.ErrCodeUnauthenticated,
		},
		{
			name:    "staff allowed when staff is in set",
			caller:  &Caller{ID: "s-1", Role: RoleStaff},
			allowed: []string{RoleStaff, RoleAdmin},
			wantErr: false,
		},
		{
			name:     "no caller returns ErrCodeUnauthenticated",
			caller:   nil,
			allowed:  []string{RoleAdmin},
			wantErr:  true,
			wantCode: domain.ErrCodeUnauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.caller != nil {
				ctx = WithCaller(ctx, *tt.caller)
			}

			got, err := RequireRole(ctx, tt.allowed...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var sErr *domain.SemanticError
				if !errors.As(err, &sErr) {
					t.Fatalf("expected *domain.SemanticError, got %T", err)
				}
				if sErr.Code != tt.wantCode {
					t.Errorf("got Code=%q, want %q", sErr.Code, tt.wantCode)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got == nil {
				t.Fatal("expected non-nil caller")
			}
		})
	}
}

func TestRequireClientMatch(t *testing.T) {
	clientID := "c-001"
	otherClientID := "c-999"
	profID := "p-1"
	otherProfID := "p-999"

	tests := []struct {
		name        string
		caller      *Caller
		inputClient string
		inputProf   string
		wantErr     bool
	}{
		{
			name: "client match passes",
			caller: &Caller{
				ID:       "+5491100001111",
				Role:     RoleClient,
				ClientID: &clientID,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     false,
		},
		{
			name: "client mismatch fails",
			caller: &Caller{
				ID:       "+5491100002222",
				Role:     RoleClient,
				ClientID: &otherClientID,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     true,
		},
		{
			name: "admin bypass — any client ID passes",
			caller: &Caller{
				ID:   "admin-1",
				Role: RoleAdmin,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     false,
		},
		{
			name: "owner bypass — any client ID passes",
			caller: &Caller{
				ID:   "owner-1",
				Role: RoleOwner,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     false,
		},
		{
			name: "staff with matching ProfessionalID passes",
			caller: &Caller{
				ID:             "staff-1",
				Role:           RoleStaff,
				ProfessionalID: &profID,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     false,
		},
		{
			name: "staff with mismatched ProfessionalID fails",
			caller: &Caller{
				ID:             "staff-1",
				Role:           RoleStaff,
				ProfessionalID: &otherProfID,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     true,
		},
		{
			name: "staff with nil ProfessionalID fails",
			caller: &Caller{
				ID:             "staff-nil",
				Role:           RoleStaff,
				ProfessionalID: nil,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     true,
		},
		{
			name:        "no caller returns ErrCodeUnauthenticated",
			caller:      nil,
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     true,
		},
		{
			name: "client with nil ClientID fails",
			caller: &Caller{
				ID:       "+5491100003333",
				Role:     RoleClient,
				ClientID: nil,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.caller != nil {
				ctx = WithCaller(ctx, *tt.caller)
			}

			err := RequireClientMatch(ctx, tt.inputClient, tt.inputProf)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var sErr *domain.SemanticError
				if !errors.As(err, &sErr) {
					t.Fatalf("expected *domain.SemanticError, got %T: %v", err, err)
				}
				if sErr.Code != domain.ErrCodeUnauthenticated {
					t.Errorf("got Code=%q, want %q", sErr.Code, domain.ErrCodeUnauthenticated)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
