package repository

import (
	"errors"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

func TestApplyClientsAuthFilter_ErrorSentinels(t *testing.T) {
	t.Run("nil caller -> ErrUnauthenticated", func(t *testing.T) {
		_, _, err := applyClientsAuthFilter(nil, "SELECT * FROM clients WHERE x = ?", "", nil)
		if !errors.Is(err, domain.ErrUnauthenticated) {
			t.Fatalf("expected ErrUnauthenticated, got %v", err)
		}
	})

	t.Run("staff -> ErrForbidden", func(t *testing.T) {
		profID := "p1"
		caller := &auth.Caller{ID: "s1", Role: auth.RoleStaff, ProfessionalID: &profID}
		_, _, err := applyClientsAuthFilter(caller, "SELECT * FROM clients WHERE x = ?", "", nil)
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("unknown role -> ErrForbidden", func(t *testing.T) {
		caller := &auth.Caller{ID: "x1", Role: "vendor"}
		_, _, err := applyClientsAuthFilter(caller, "SELECT * FROM clients WHERE x = ?", "", nil)
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("client without ClientID -> ErrForbidden", func(t *testing.T) {
		caller := &auth.Caller{ID: "c1", Role: auth.RoleClient}
		_, _, err := applyClientsAuthFilter(caller, "SELECT * FROM clients WHERE x = ?", "", nil)
		if !errors.Is(err, domain.ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})
}

func TestApplyClientsAuthFilter_Composition(t *testing.T) {
	t.Run("client scope placed before suffix", func(t *testing.T) {
		caller := &auth.Caller{ID: "c1", Role: auth.RoleClient, ClientID: strPtr("cli-1")}
		query, args, err := applyClientsAuthFilter(
			caller,
			"SELECT * FROM clients WHERE x = ?",
			" ORDER BY name",
			sqlArgs{"val"},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "SELECT * FROM clients WHERE x = ? AND id = ? ORDER BY name"
		if query != want {
			t.Errorf("got query %q, want %q", query, want)
		}
		if len(args) != 2 || args[0] != "val" || args[1] != "cli-1" {
			t.Errorf("got args %v, want [val cli-1]", args)
		}
	})

	t.Run("admin query and args unchanged", func(t *testing.T) {
		caller := &auth.Caller{ID: "a1", Role: auth.RoleAdmin}
		query, args, err := applyClientsAuthFilter(
			caller,
			"SELECT * FROM clients WHERE x = ?",
			" ORDER BY name",
			sqlArgs{"val"},
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "SELECT * FROM clients WHERE x = ? ORDER BY name"
		if query != want {
			t.Errorf("got query %q, want %q", query, want)
		}
		if len(args) != 1 || args[0] != "val" {
			t.Errorf("got args %v, want [val]", args)
		}
	})
}
