package repository

import (
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

func TestApplyClientsAuthFilter_ErrorSentinels(t *testing.T) {
	t.Run("nil caller -> ErrUnauthenticated", func(t *testing.T) {
		_, _, err := applyClientsAuthFilter(nil, "SELECT * FROM clients", nil)
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})

	t.Run("staff -> ErrForbidden", func(t *testing.T) {
		profID := "p1"
		caller := &auth.Caller{ID: "s1", Role: auth.RoleStaff, ProfessionalID: &profID}
		_, _, err := applyClientsAuthFilter(caller, "SELECT * FROM clients", nil)
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("unknown role -> ErrForbidden", func(t *testing.T) {
		caller := &auth.Caller{ID: "x1", Role: "vendor"}
		_, _, err := applyClientsAuthFilter(caller, "SELECT * FROM clients", nil)
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("client without ClientID -> ErrForbidden", func(t *testing.T) {
		caller := &auth.Caller{ID: "c1", Role: auth.RoleClient}
		_, _, err := applyClientsAuthFilter(caller, "SELECT * FROM clients", nil)
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})
}
