package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

func TestRequireCaller(t *testing.T) {
	t.Run("no caller in context returns SemanticError with ErrCodeUnauthenticated", func(t *testing.T) {
		_, err := requireCaller(context.Background())
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		var sErr *SemanticError
		if !errors.As(err, &sErr) {
			t.Fatalf("expected *SemanticError, got %T: %v", err, err)
		}
		if sErr.Code != ErrCodeUnauthenticated {
			t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
		}
		if !errors.Is(sErr, ErrUnauthenticated) {
			t.Errorf("expected errors.Is(ErrUnauthenticated) to be true")
		}
	})

	t.Run("caller present returns pointer to caller", func(t *testing.T) {
		c := auth.Caller{ID: "+5491155554444", Role: auth.RoleAdmin}
		ctx := auth.WithCaller(context.Background(), c)

		got, err := requireCaller(ctx)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ID != c.ID {
			t.Errorf("got ID=%q, want %q", got.ID, c.ID)
		}
		if got.Role != auth.RoleAdmin {
			t.Errorf("got Role=%q, want %q", got.Role, auth.RoleAdmin)
		}
	})
}

func TestRequireRole(t *testing.T) {
	tests := []struct {
		name     string
		caller   *auth.Caller
		allowed  []string
		wantErr  bool
		wantCode ErrCode
	}{
		{
			name:    "admin allowed when admin is in set",
			caller:  &auth.Caller{ID: "a-1", Role: auth.RoleAdmin},
			allowed: []string{auth.RoleAdmin, auth.RoleOwner},
			wantErr: false,
		},
		{
			name:    "owner allowed when owner is in set",
			caller:  &auth.Caller{ID: "o-1", Role: auth.RoleOwner},
			allowed: []string{auth.RoleAdmin, auth.RoleOwner},
			wantErr: false,
		},
		{
			name:     "client rejected when only admin/owner allowed",
			caller:   &auth.Caller{ID: "c-1", Role: auth.RoleClient},
			allowed:  []string{auth.RoleAdmin, auth.RoleOwner},
			wantErr:  true,
			wantCode: ErrCodeUnauthenticated,
		},
		{
			name:     "staff rejected when only admin/owner allowed",
			caller:   &auth.Caller{ID: "s-1", Role: auth.RoleStaff},
			allowed:  []string{auth.RoleAdmin, auth.RoleOwner},
			wantErr:  true,
			wantCode: ErrCodeUnauthenticated,
		},
		{
			name:    "staff allowed when staff is in set",
			caller:  &auth.Caller{ID: "s-1", Role: auth.RoleStaff},
			allowed: []string{auth.RoleStaff, auth.RoleAdmin},
			wantErr: false,
		},
		{
			name:     "no caller returns ErrCodeUnauthenticated",
			caller:   nil,
			allowed:  []string{auth.RoleAdmin},
			wantErr:  true,
			wantCode: ErrCodeUnauthenticated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.caller != nil {
				ctx = auth.WithCaller(ctx, *tt.caller)
			}

			got, err := requireRole(ctx, tt.allowed...)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var sErr *SemanticError
				if !errors.As(err, &sErr) {
					t.Fatalf("expected *SemanticError, got %T", err)
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
		caller      *auth.Caller
		inputClient string
		inputProf   string
		wantErr     bool
	}{
		{
			name: "client match passes",
			caller: &auth.Caller{
				ID:       "+5491100001111",
				Role:     auth.RoleClient,
				ClientID: &clientID,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     false,
		},
		{
			name: "client mismatch fails",
			caller: &auth.Caller{
				ID:       "+5491100002222",
				Role:     auth.RoleClient,
				ClientID: &otherClientID,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     true,
		},
		{
			name: "admin bypass — any client ID passes",
			caller: &auth.Caller{
				ID:   "admin-1",
				Role: auth.RoleAdmin,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     false,
		},
		{
			name: "owner bypass — any client ID passes",
			caller: &auth.Caller{
				ID:   "owner-1",
				Role: auth.RoleOwner,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     false,
		},
		{
			name: "staff with matching ProfessionalID passes",
			caller: &auth.Caller{
				ID:             "staff-1",
				Role:           auth.RoleStaff,
				ProfessionalID: &profID,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     false,
		},
		{
			name: "staff with mismatched ProfessionalID fails",
			caller: &auth.Caller{
				ID:             "staff-1",
				Role:           auth.RoleStaff,
				ProfessionalID: &otherProfID,
			},
			inputClient: clientID,
			inputProf:   profID,
			wantErr:     true,
		},
		{
			name: "staff with nil ProfessionalID fails",
			caller: &auth.Caller{
				ID:             "staff-nil",
				Role:           auth.RoleStaff,
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
			caller: &auth.Caller{
				ID:       "+5491100003333",
				Role:     auth.RoleClient,
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
				ctx = auth.WithCaller(ctx, *tt.caller)
			}

			err := requireClientMatch(ctx, tt.inputClient, tt.inputProf)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var sErr *SemanticError
				if !errors.As(err, &sErr) {
					t.Fatalf("expected *SemanticError, got %T: %v", err, err)
				}
				if sErr.Code != ErrCodeUnauthenticated {
					t.Errorf("got Code=%q, want %q", sErr.Code, ErrCodeUnauthenticated)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
