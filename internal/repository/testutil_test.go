package repository

import (
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
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
