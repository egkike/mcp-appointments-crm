package repository

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
)

// newMockDB creates a go-sqlmock mock database for repository unit tests.
// Returns the *sql.DB handle and the sqlmock interface for setting expectations.
// Registers a t.Cleanup that calls mock.ExpectationsWereMet() so unmet
// expectations are always reported.
func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sqlmock expectations: %v", err)
		}
		_ = db.Close()
	})
	return db, mock
}

func TestNewMockDB(t *testing.T) {
	db, mock := newMockDB(t)
	if db == nil {
		t.Fatal("expected non-nil *sql.DB")
	}
	if mock == nil {
		t.Fatal("expected non-nil Sqlmock")
	}
}

// adminCtx returns a context with an admin caller attached.
func adminCtx() context.Context {
	return auth.WithCaller(context.Background(), auth.Caller{ID: "admin-1", Role: auth.RoleAdmin})
}

// ownerCtx returns a context with an owner caller attached.
func ownerCtx() context.Context {
	return auth.WithCaller(context.Background(), auth.Caller{ID: "owner-1", Role: auth.RoleOwner})
}

// staffCtx returns a context with a staff caller attached, linked to the given professionalID.
func staffCtx(professionalID string) context.Context {
	return auth.WithCaller(context.Background(), auth.Caller{
		ID:             "staff-1",
		Role:           auth.RoleStaff,
		ProfessionalID: &professionalID,
	})
}

// clientCtx returns a context with a client caller attached, linked to the given clientID.
func clientCtx(clientID string) context.Context {
	return auth.WithCaller(context.Background(), auth.Caller{
		ID:       "client-1",
		Role:     auth.RoleClient,
		ClientID: &clientID,
	})
}
