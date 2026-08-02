package repository

import (
	"errors"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
)

func TestApplyAuthFilter(t *testing.T) {
	baseQuery := `SELECT id FROM bookings WHERE id = ?`
	baseArgs := []any{"booking-123"}

	t.Run("RoleClient with valid ClientID adds AND client_id = ?", func(t *testing.T) {
		clientID := "client-abc"
		caller := &auth.Caller{Role: auth.RoleClient, ClientID: &clientID}

		query, args, err := applyAuthFilter(caller, baseQuery, baseArgs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasSuffix(query, " AND client_id = ?") {
			t.Errorf("query = %q; want suffix ' AND client_id = ?'", query)
		}
		if len(args) != 2 {
			t.Fatalf("args length = %d; want 2", len(args))
		}
		if args[0] != "booking-123" {
			t.Errorf("args[0] = %v; want 'booking-123'", args[0])
		}
		if args[1] != "client-abc" {
			t.Errorf("args[1] = %v; want 'client-abc'", args[1])
		}
	})

	t.Run("RoleClient with nil ClientID returns SemanticError", func(t *testing.T) {
		caller := &auth.Caller{Role: auth.RoleClient, ClientID: nil}

		_, _, err := applyAuthFilter(caller, baseQuery, baseArgs)
		if err == nil {
			t.Fatal("expected error; got nil")
		}
		var semErr *domain.SemanticError
		if !errors.As(err, &semErr) {
			t.Fatalf("expected *domain.SemanticError; got %T", err)
		}
		if semErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("code = %q; want %q", semErr.Code, domain.ErrCodeUnauthenticated)
		}
		if !strings.Contains(semErr.Message, "cliente no tiene ID") {
			t.Errorf("message = %q; want contains 'cliente no tiene ID'", semErr.Message)
		}
	})

	t.Run("RoleStaff with valid ProfessionalID adds AND professional_id = ?", func(t *testing.T) {
		profID := "prof-xyz"
		caller := &auth.Caller{Role: auth.RoleStaff, ProfessionalID: &profID}

		query, args, err := applyAuthFilter(caller, baseQuery, baseArgs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.HasSuffix(query, " AND professional_id = ?") {
			t.Errorf("query = %q; want suffix ' AND professional_id = ?'", query)
		}
		if len(args) != 2 {
			t.Fatalf("args length = %d; want 2", len(args))
		}
		if args[1] != "prof-xyz" {
			t.Errorf("args[1] = %v; want 'prof-xyz'", args[1])
		}
	})

	t.Run("RoleStaff with nil ProfessionalID returns SemanticError", func(t *testing.T) {
		caller := &auth.Caller{Role: auth.RoleStaff, ProfessionalID: nil}

		_, _, err := applyAuthFilter(caller, baseQuery, baseArgs)
		if err == nil {
			t.Fatal("expected error; got nil")
		}
		var semErr *domain.SemanticError
		if !errors.As(err, &semErr) {
			t.Fatalf("expected *domain.SemanticError; got %T", err)
		}
		if semErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("code = %q; want %q", semErr.Code, domain.ErrCodeUnauthenticated)
		}
	})

	t.Run("RoleAdmin does not add extra filter", func(t *testing.T) {
		caller := &auth.Caller{Role: auth.RoleAdmin}

		query, args, err := applyAuthFilter(caller, baseQuery, baseArgs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if query != baseQuery {
			t.Errorf("query = %q; want unchanged %q", query, baseQuery)
		}
		if len(args) != 1 {
			t.Errorf("args length = %d; want 1", len(args))
		}
	})

	t.Run("RoleOwner does not add extra filter", func(t *testing.T) {
		caller := &auth.Caller{Role: auth.RoleOwner}

		query, args, err := applyAuthFilter(caller, baseQuery, baseArgs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if query != baseQuery {
			t.Errorf("query = %q; want unchanged %q", query, baseQuery)
		}
		if len(args) != 1 {
			t.Errorf("args length = %d; want 1", len(args))
		}
	})

	t.Run("Unknown role returns SemanticError", func(t *testing.T) {
		caller := &auth.Caller{Role: "superadmin"}

		_, _, err := applyAuthFilter(caller, baseQuery, baseArgs)
		if err == nil {
			t.Fatal("expected error; got nil")
		}
		var semErr *domain.SemanticError
		if !errors.As(err, &semErr) {
			t.Fatalf("expected *domain.SemanticError; got %T", err)
		}
		if semErr.Code != domain.ErrCodeUnauthenticated {
			t.Errorf("code = %q; want %q", semErr.Code, domain.ErrCodeUnauthenticated)
		}
		if !strings.Contains(semErr.Message, "superadmin") {
			t.Errorf("message = %q; want contains role name 'superadmin'", semErr.Message)
		}
	})

	t.Run("Empty baseArgs handled correctly", func(t *testing.T) {
		clientID := "client-abc"
		caller := &auth.Caller{Role: auth.RoleClient, ClientID: &clientID}
		emptyArgs := []any{}

		query, args, err := applyAuthFilter(caller, baseQuery, emptyArgs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(args) != 1 {
			t.Fatalf("args length = %d; want 1", len(args))
		}
		if args[0] != "client-abc" {
			t.Errorf("args[0] = %v; want 'client-abc'", args[0])
		}
		_ = query
	})

	t.Run("Does not mutate original baseArgs", func(t *testing.T) {
		clientID := "client-abc"
		caller := &auth.Caller{Role: auth.RoleClient, ClientID: &clientID}
		original := []any{"booking-123"}

		_, args, err := applyAuthFilter(caller, baseQuery, original)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Original must be untouched
		if len(original) != 1 {
			t.Errorf("original args mutated: length = %d; want 1", len(original))
		}
		// Returned args must have the extra element
		if len(args) != 2 {
			t.Errorf("returned args length = %d; want 2", len(args))
		}
	})

	t.Run("Nil caller returns error", func(t *testing.T) {
		_, _, err := applyAuthFilter(nil, baseQuery, baseArgs)
		if err == nil {
			t.Fatal("expected error for nil caller; got nil")
		}
		var semErr *domain.SemanticError
		if !errors.As(err, &semErr) {
			t.Fatalf("expected *domain.SemanticError; got %T", err)
		}
	})
}
