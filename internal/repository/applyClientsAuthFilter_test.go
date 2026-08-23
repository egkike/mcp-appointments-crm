package repository

import (
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

func TestApplyClientsAuthFilter(t *testing.T) {
	t.Run("admin leaves query unchanged", func(t *testing.T) {
		caller := &auth.Caller{ID: "admin-1", Role: auth.RoleAdmin}
		q, args, err := applyClientsAuthFilter(caller, "SELECT * FROM clients WHERE x = ?", []any{"a"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q != "SELECT * FROM clients WHERE x = ?" {
			t.Errorf("query unchanged: got %q", q)
		}
		if len(args) != 1 || args[0] != "a" {
			t.Errorf("args unchanged: got %v", args)
		}
	})

	t.Run("owner leaves query unchanged", func(t *testing.T) {
		caller := &auth.Caller{ID: "owner-1", Role: auth.RoleOwner}
		q, args, err := applyClientsAuthFilter(caller, "SELECT * FROM clients WHERE x = ?", []any{"a"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q != "SELECT * FROM clients WHERE x = ?" {
			t.Errorf("query unchanged: got %q", q)
		}
		if len(args) != 1 || args[0] != "a" {
			t.Errorf("args unchanged: got %v", args)
		}
	})

	t.Run("client appends id filter before ORDER BY", func(t *testing.T) {
		clientID := "cli-1"
		caller := &auth.Caller{ID: "client-1", Role: auth.RoleClient, ClientID: &clientID}
		base := "SELECT * FROM clients WHERE x = ? ORDER BY y"
		q, args, err := applyClientsAuthFilter(caller, base, []any{"a"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "SELECT * FROM clients WHERE x = ?  AND id = ? ORDER BY y"
		if q != want {
			t.Errorf("got %q, want %q", q, want)
		}
		if len(args) != 2 || args[0] != "a" || args[1] != "cli-1" {
			t.Errorf("args: got %v", args)
		}
	})

	t.Run("client appends id filter before LIMIT", func(t *testing.T) {
		clientID := "cli-1"
		caller := &auth.Caller{ID: "client-1", Role: auth.RoleClient, ClientID: &clientID}
		base := "SELECT * FROM clients WHERE x = ? LIMIT 10"
		q, args, err := applyClientsAuthFilter(caller, base, []any{"a"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "SELECT * FROM clients WHERE x = ?  AND id = ? LIMIT 10"
		if q != want {
			t.Errorf("got %q, want %q", q, want)
		}
		if len(args) != 2 || args[1] != "cli-1" {
			t.Errorf("args: got %v", args)
		}
	})

	t.Run("client without ClientID returns forbidden", func(t *testing.T) {
		caller := &auth.Caller{ID: "client-1", Role: auth.RoleClient}
		_, _, err := applyClientsAuthFilter(caller, "SELECT * FROM clients WHERE x = ?", []any{"a"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("staff returns forbidden", func(t *testing.T) {
		profID := "prof-1"
		caller := &auth.Caller{ID: "staff-1", Role: auth.RoleStaff, ProfessionalID: &profID}
		_, _, err := applyClientsAuthFilter(caller, "SELECT * FROM clients WHERE x = ?", []any{"a"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("unknown role returns forbidden", func(t *testing.T) {
		caller := &auth.Caller{ID: "x-1", Role: "vendor"}
		_, _, err := applyClientsAuthFilter(caller, "SELECT * FROM clients WHERE x = ?", []any{"a"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("nil caller returns unauthenticated", func(t *testing.T) {
		_, _, err := applyClientsAuthFilter(nil, "SELECT * FROM clients WHERE x = ?", []any{"a"})
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("defensive copy does not mutate original args", func(t *testing.T) {
		clientID := "cli-1"
		caller := &auth.Caller{ID: "client-1", Role: auth.RoleClient, ClientID: &clientID}
		original := []any{"a"}
		_, _, err := applyClientsAuthFilter(caller, "SELECT * FROM clients WHERE x = ? ORDER BY y", original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(original) != 1 || original[0] != "a" {
			t.Errorf("original args mutated: %v", original)
		}
	})
}
