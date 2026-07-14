package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// newResolverWithMock creates a CallerResolver backed by go-sqlmock.
func newResolverWithMock(t *testing.T) (*CallerResolver, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewCallerResolver(db), mock
}

func TestResolve_AdminInAccounts(t *testing.T) {
	t.Parallel()
	r, mock := newResolverWithMock(t)

	id := "+5491100000000"

	// Query 1: accounts — admin, active, no professional_id
	mock.ExpectQuery("SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "professional_id", "is_active"}).
			AddRow(id, "admin", nil, 1))

	// Query 2: clients — no row (admin without client row)
	mock.ExpectQuery("SELECT id FROM clients WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	caller, err := r.Resolve(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caller.ID != id {
		t.Errorf("ID = %q; want %q", caller.ID, id)
	}
	if caller.Role != RoleAdmin {
		t.Errorf("Role = %q; want %q", caller.Role, RoleAdmin)
	}
	if caller.ProfessionalID != nil {
		t.Errorf("ProfessionalID = %v; want nil", caller.ProfessionalID)
	}
	if caller.ClientID != nil {
		t.Errorf("ClientID = %v; want nil", caller.ClientID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestResolve_StaffWithProfessionalID(t *testing.T) {
	t.Parallel()
	r, mock := newResolverWithMock(t)

	id := "+5491100002222"
	profID := "p-001"

	// Query 1: accounts — staff, active, with professional_id
	mock.ExpectQuery("SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "professional_id", "is_active"}).
			AddRow(id, "staff", profID, 1))

	// Query 2: clients — no row
	mock.ExpectQuery("SELECT id FROM clients WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	caller, err := r.Resolve(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caller.Role != RoleStaff {
		t.Errorf("Role = %q; want %q", caller.Role, RoleStaff)
	}
	if caller.ProfessionalID == nil || *caller.ProfessionalID != profID {
		t.Errorf("ProfessionalID = %v; want pointer to %q", caller.ProfessionalID, profID)
	}
	if caller.ClientID != nil {
		t.Errorf("ClientID = %v; want nil", caller.ClientID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestResolve_AccountDisabled(t *testing.T) {
	t.Parallel()
	r, mock := newResolverWithMock(t)

	id := "+5491100000000"

	// Query 1: accounts — inactive
	mock.ExpectQuery("SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "professional_id", "is_active"}).
			AddRow(id, "admin", nil, 0))

	// NO query to clients — disabled account short-circuits

	_, err := r.Resolve(context.Background(), id)
	if err == nil {
		t.Fatal("expected error for disabled account; got nil")
	}
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("error should wrap ErrUnauthenticated; got %v", err)
	}
	// Check the Spanish message
	if !strings.Contains(err.Error(), "deshabilitada") {
		t.Errorf("error message should mention 'deshabilitada'; got %q", err.Error())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestResolve_ClientInClients(t *testing.T) {
	t.Parallel()
	r, mock := newResolverWithMock(t)

	id := "+5491100003333"

	// Query 1: accounts — no row
	mock.ExpectQuery("SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "professional_id", "is_active"}))

	// Query 2: clients — row found
	mock.ExpectQuery("SELECT id FROM clients WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))

	caller, err := r.Resolve(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caller.Role != RoleClient {
		t.Errorf("Role = %q; want %q", caller.Role, RoleClient)
	}
	if caller.ProfessionalID != nil {
		t.Errorf("ProfessionalID = %v; want nil", caller.ProfessionalID)
	}
	if caller.ClientID == nil || *caller.ClientID != id {
		t.Errorf("ClientID = %v; want pointer to %q", caller.ClientID, id)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestResolve_UnknownID(t *testing.T) {
	t.Parallel()
	r, mock := newResolverWithMock(t)

	id := "+5491100099999"

	// Query 1: accounts — no row
	mock.ExpectQuery("SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "professional_id", "is_active"}))

	// Query 2: clients — no row
	mock.ExpectQuery("SELECT id FROM clients WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	_, err := r.Resolve(context.Background(), id)
	if err == nil {
		t.Fatal("expected error for unknown ID; got nil")
	}
	if !errors.Is(err, ErrUnauthenticated) {
		t.Errorf("error should wrap ErrUnauthenticated; got %v", err)
	}
	if !strings.Contains(err.Error(), "reconozco") {
		t.Errorf("error message should mention 'reconozco'; got %q", err.Error())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestResolve_OwnerAlsoClient(t *testing.T) {
	t.Parallel()
	r, mock := newResolverWithMock(t)

	id := "+5491100000000"

	// Query 1: accounts — owner, active
	mock.ExpectQuery("SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "professional_id", "is_active"}).
			AddRow(id, "owner", nil, 1))

	// Query 2: clients — row found (owner is also a client)
	mock.ExpectQuery("SELECT id FROM clients WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id))

	caller, err := r.Resolve(context.Background(), id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if caller.Role != RoleOwner {
		t.Errorf("Role = %q; want %q", caller.Role, RoleOwner)
	}
	if caller.ClientID == nil || *caller.ClientID != id {
		t.Errorf("ClientID = %v; want pointer to %q", caller.ClientID, id)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

func TestResolve_AccountsDBError(t *testing.T) {
	t.Parallel()
	r, mock := newResolverWithMock(t)

	id := "+5491100000000"

	mock.ExpectQuery("SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnError(errors.New("connection refused"))

	_, err := r.Resolve(context.Background(), id)
	if err == nil {
		t.Fatal("expected error for DB failure; got nil")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}

// TestResolve_ClientsDBError covers F-RES-1: when the active-account path's
// second query (clients) fails with a non-ErrNoRows error, the resolver MUST
// return the wrapped error (so the middleware responds 500) rather than mask
// it as a successful (caller, nil) with ClientID = nil.
func TestResolve_ClientsDBError(t *testing.T) {
	t.Parallel()
	r, mock := newResolverWithMock(t)

	id := "+5491100000000"

	// Query 1: accounts — admin, active
	mock.ExpectQuery("SELECT id, role, professional_id, is_active FROM accounts WHERE id = ?").
		WithArgs(id).
		WillReturnRows(sqlmock.NewRows([]string{"id", "role", "professional_id", "is_active"}).
			AddRow(id, "admin", nil, 1))

	// Query 2: clients — DB error (NOT sql.ErrNoRows)
	mock.ExpectQuery("SELECT id FROM clients WHERE id = ?").
		WithArgs(id).
		WillReturnError(errors.New("connection lost mid-resolution"))

	_, err := r.Resolve(context.Background(), id)
	if err == nil {
		t.Fatal("expected error for clients DB failure; got nil (false success)")
	}
	if errors.Is(err, ErrUnauthenticated) {
		t.Errorf("DB error must NOT be wrapped as ErrUnauthenticated; got %v", err)
	}
	if !strings.Contains(err.Error(), "connection lost") {
		t.Errorf("error should wrap the underlying error; got %q", err.Error())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet expectations: %v", err)
	}
}
