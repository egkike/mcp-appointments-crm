package admin

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	"github.com/egkike/mcp-appointments-crm/internal/repository"
)

// Exact query text mirrors internal/repository/{accounts,clients}.go; the mock
// uses QueryMatcherEqual so a repository query change fails this suite loudly.
const (
	selectAccountByIDQuery = `SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE id = ?`
	selectClientByIDQuery  = "SELECT id, name, phone, email, preferences, created_at, updated_at\n\t\t FROM clients WHERE id = ?"
	insertClientQuery      = "INSERT INTO clients (id, name, phone, email, preferences)\n\t\t VALUES (?, ?, ?, ?, ?)"
)

// selfClientPhone is the phone/account id the add-self fixtures use.
const selfClientPhone = "+5491100000000"

// clientRow describes one clients row for the mock result sets.
type clientRow struct {
	id    string
	name  string
	phone string
}

// clientColumns returns the column set ClientsRepo.FindByID scans.
func clientColumns() []string {
	return []string{"id", "name", "phone", "email", "preferences", "created_at", "updated_at"}
}

// clientRows builds the clients result set in the given order.
func clientRows(rows ...clientRow) *sqlmock.Rows {
	result := sqlmock.NewRows(clientColumns())
	for _, row := range rows {
		result.AddRow(row.id, row.name, row.phone, nil, nil,
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
	}
	return result
}

// newMockIdentityRepos builds the real AccountsRepo and ClientsRepo on top of
// one go-sqlmock handle, so AddSelfAsClient is exercised against the production
// query contract of both ports (mirrors newMockAdminRepos).
func newMockIdentityRepos(t *testing.T) (*repository.AccountsRepo, *repository.ClientsRepo, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() {
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("unmet sqlmock expectations: %v", err)
		}
		_ = db.Close()
	})
	return repository.NewAccountsRepo(db, slog.New(slog.DiscardHandler)), repository.NewClientsRepo(db), mock
}

// activeOwnerRow describes the operator's own account in the mock.
func activeOwnerRow(id string) accountRow {
	return accountRow{id: id, role: entity.RoleOwner, displayName: "Dueño", isActive: 1}
}

func TestAddSelfAsClient_CreatesRowWithIDEqualToTheCallerPhone(t *testing.T) {
	accounts, clients, mock := newMockIdentityRepos(t)

	mock.ExpectQuery(selectAccountByIDQuery).
		WithArgs(selfClientPhone).
		WillReturnRows(accountsRows(activeOwnerRow(selfClientPhone)))
	mock.ExpectQuery(selectClientByIDQuery).
		WithArgs(selfClientPhone).
		WillReturnError(sql.ErrNoRows)
	// id and phone carry the SAME value: the id is what CallerResolver step 2
	// matches, the phone is the UNIQUE business key.
	mock.ExpectExec(insertClientQuery).
		WithArgs(selfClientPhone, "Dueño Cliente", selfClientPhone, nil, nil).
		WillReturnResult(sqlmock.NewResult(1, 1))

	outcome, err := AddSelfAsClient(context.Background(), accounts, clients, selfClientPhone, "  Dueño Cliente  ")
	if err != nil {
		t.Fatalf("AddSelfAsClient() error = %v", err)
	}
	if outcome.AlreadyRegistered {
		t.Error("AddSelfAsClient() AlreadyRegistered = true, want a fresh registration")
	}
	if outcome.ClientID != selfClientPhone {
		t.Errorf("AddSelfAsClient() ClientID = %q, want the caller phone %q", outcome.ClientID, selfClientPhone)
	}
}

func TestAddSelfAsClient_IdempotentWhenTheRowAlreadyExists(t *testing.T) {
	accounts, clients, mock := newMockIdentityRepos(t)

	mock.ExpectQuery(selectAccountByIDQuery).
		WithArgs(selfClientPhone).
		WillReturnRows(accountsRows(activeOwnerRow(selfClientPhone)))
	mock.ExpectQuery(selectClientByIDQuery).
		WithArgs(selfClientPhone).
		WillReturnRows(clientRows(clientRow{id: selfClientPhone, name: "Dueño Cliente", phone: selfClientPhone}))

	// No INSERT is expected: a second run must not rewrite the row nor fail.
	outcome, err := AddSelfAsClient(context.Background(), accounts, clients, selfClientPhone, "Dueño Cliente")
	if err != nil {
		t.Fatalf("AddSelfAsClient() error = %v", err)
	}
	if !outcome.AlreadyRegistered {
		t.Error("AddSelfAsClient() AlreadyRegistered = false, want the idempotent outcome")
	}
	if outcome.ClientID != selfClientPhone {
		t.Errorf("AddSelfAsClient() ClientID = %q, want %q", outcome.ClientID, selfClientPhone)
	}
}

func TestAddSelfAsClient_DuplicatePhoneIsSemanticConflict(t *testing.T) {
	accounts, clients, mock := newMockIdentityRepos(t)

	mock.ExpectQuery(selectAccountByIDQuery).
		WithArgs(selfClientPhone).
		WillReturnRows(accountsRows(activeOwnerRow(selfClientPhone)))
	mock.ExpectQuery(selectClientByIDQuery).
		WithArgs(selfClientPhone).
		WillReturnError(sql.ErrNoRows)
	// clients.phone is UNIQUE: a row for the same phone but a different id (the
	// UUID bootstrap path) rejects the insert.
	mock.ExpectExec(insertClientQuery).
		WillReturnError(errors.New("UNIQUE constraint failed: clients.phone"))

	_, err := AddSelfAsClient(context.Background(), accounts, clients, selfClientPhone, "Dueño Cliente")
	if err == nil {
		t.Fatal("AddSelfAsClient() error = nil, want the duplicate-phone conflict")
	}
	if !errors.Is(err, ErrClientPhoneTaken) {
		t.Errorf("AddSelfAsClient() error = %v, want errors.Is(ErrClientPhoneTaken)", err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("AddSelfAsClient() error = %v, want errors.Is(domain.ErrConflict)", err)
	}
	assertSemanticConflictMessage(t, err, "teléfono")
}

func TestAddSelfAsClient_PrimaryKeyDuplicateIsSemanticConflict(t *testing.T) {
	accounts, clients, mock := newMockIdentityRepos(t)

	mock.ExpectQuery(selectAccountByIDQuery).
		WithArgs(selfClientPhone).
		WillReturnRows(accountsRows(activeOwnerRow(selfClientPhone)))
	mock.ExpectQuery(selectClientByIDQuery).
		WithArgs(selfClientPhone).
		WillReturnError(sql.ErrNoRows)
	// The row appeared between the read and the write: clients.id is the
	// PRIMARY KEY, so the race surfaces as 1555 and must map to the same
	// business conflict.
	mock.ExpectExec(insertClientQuery).
		WillReturnError(errors.New("PRIMARY KEY constraint failed: clients.id"))

	_, err := AddSelfAsClient(context.Background(), accounts, clients, selfClientPhone, "Dueño Cliente")
	if err == nil {
		t.Fatal("AddSelfAsClient() error = nil, want the duplicate-id conflict")
	}
	if !errors.Is(err, ErrClientPhoneTaken) {
		t.Errorf("AddSelfAsClient() error = %v, want errors.Is(ErrClientPhoneTaken)", err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("AddSelfAsClient() error = %v, want errors.Is(domain.ErrConflict)", err)
	}
	assertSemanticConflictMessage(t, err, "teléfono")
}

// assertSemanticConflictMessage locks the operator-facing contract of the
// conflict mapping: a semantic message that names the offending field and never
// leaks driver detail.
func assertSemanticConflictMessage(t *testing.T, err error, wantField string) {
	t.Helper()
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeConflict {
		t.Errorf("error code = %q, want %q", semErr.Code, domain.ErrCodeConflict)
	}
	if !strings.Contains(semErr.Message, wantField) {
		t.Errorf("error message = %q, want it to name %q", semErr.Message, wantField)
	}
	for _, leak := range []string{"SQL", "constraint", "UNIQUE", "PRIMARY KEY"} {
		if strings.Contains(semErr.Message, leak) {
			t.Errorf("error message = %q, want no driver detail (%q)", semErr.Message, leak)
		}
	}
}

func TestAddSelfAsClient_RejectsInvalidInputBeforeAnyPortCall(t *testing.T) {
	tests := []struct {
		name       string
		callerID   string
		display    string
		wantPhrase string
	}{
		{
			name:       "blank caller id",
			callerID:   "   ",
			display:    "Dueño Cliente",
			wantPhrase: "no puede estar vacío",
		},
		{
			name:       "caller id is not a phone",
			callerID:   "no-es-un-telefono",
			display:    "Dueño Cliente",
			wantPhrase: "el teléfono no es válido",
		},
		{
			name:       "blank display name",
			callerID:   selfClientPhone,
			display:    "   ",
			wantPhrase: "el nombre para mostrar no puede estar vacío",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := &fakeAccountsByIDReader{}
			clients := &fakeClientsSelfService{}

			_, err := AddSelfAsClient(context.Background(), accounts, clients, tt.callerID, tt.display)
			if err == nil {
				t.Fatal("AddSelfAsClient() error = nil, want a semantic rejection")
			}
			var semErr *domain.SemanticError
			if !errors.As(err, &semErr) {
				t.Fatalf("error = %T, want *domain.SemanticError", err)
			}
			if semErr.Code != domain.ErrCodeInvalidInput {
				t.Errorf("error code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
			}
			if !strings.Contains(semErr.Message, tt.wantPhrase) {
				t.Errorf("error message = %q, want it to contain %q", semErr.Message, tt.wantPhrase)
			}
			if accounts.calls != 0 || clients.findCalls != 0 || clients.createCalls != 0 {
				t.Errorf("invalid input reached the ports: accounts=%d find=%d create=%d",
					accounts.calls, clients.findCalls, clients.createCalls)
			}
		})
	}
}

func TestAddSelfAsClient_MissingAccountIsSemanticNotFound(t *testing.T) {
	accounts := &fakeAccountsByIDReader{err: domain.ErrNotFound}
	clients := &fakeClientsSelfService{}

	_, err := AddSelfAsClient(context.Background(), accounts, clients, selfClientPhone, "Dueño Cliente")
	if err == nil {
		t.Fatal("AddSelfAsClient() error = nil, want the missing-account rejection")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeNotFound {
		t.Errorf("error code = %q, want %q", semErr.Code, domain.ErrCodeNotFound)
	}
	if clients.createCalls != 0 {
		t.Errorf("AddSelfAsClient() created %d rows for a missing account, want 0", clients.createCalls)
	}
}

func TestAddSelfAsClient_InactiveAccountIsRejected(t *testing.T) {
	accounts := &fakeAccountsByIDReader{
		account: &entity.Account{ID: selfClientPhone, Role: entity.RoleOwner, Active: false},
	}
	clients := &fakeClientsSelfService{}

	_, err := AddSelfAsClient(context.Background(), accounts, clients, selfClientPhone, "Dueño Cliente")
	if err == nil {
		t.Fatal("AddSelfAsClient() error = nil, want the disabled-account rejection")
	}
	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeForbidden {
		t.Errorf("error code = %q, want %q", semErr.Code, domain.ErrCodeForbidden)
	}
	if !errors.Is(err, domain.ErrForbidden) {
		t.Errorf("error = %v, want errors.Is(domain.ErrForbidden)", err)
	}
	if clients.findCalls != 0 || clients.createCalls != 0 {
		t.Errorf("disabled account reached the clients port: find=%d create=%d",
			clients.findCalls, clients.createCalls)
	}
}

func TestAddSelfAsClient_UnexpectedPortFailuresAreWrapped(t *testing.T) {
	t.Run("account lookup", func(t *testing.T) {
		accounts := &fakeAccountsByIDReader{err: errors.New("disk I/O error")}
		clients := &fakeClientsSelfService{}

		_, err := AddSelfAsClient(context.Background(), accounts, clients, selfClientPhone, "Dueño Cliente")
		if err == nil {
			t.Fatal("AddSelfAsClient() error = nil, want the port failure")
		}
		if !strings.Contains(err.Error(), "verificar la cuenta del operador") {
			t.Errorf("error = %v, want it to wrap the account lookup", err)
		}
		if errors.Is(err, domain.ErrConflict) {
			t.Errorf("error = %v, want no conflict classification for an I/O failure", err)
		}
	})

	t.Run("client lookup", func(t *testing.T) {
		accounts := &fakeAccountsByIDReader{
			account: &entity.Account{ID: selfClientPhone, Role: entity.RoleOwner, Active: true},
		}
		clients := &fakeClientsSelfService{findErr: errors.New("database is down")}

		_, err := AddSelfAsClient(context.Background(), accounts, clients, selfClientPhone, "Dueño Cliente")
		if err == nil {
			t.Fatal("AddSelfAsClient() error = nil, want the port failure")
		}
		if !strings.Contains(err.Error(), "verificar el cliente del operador") {
			t.Errorf("error = %v, want it to wrap the client lookup", err)
		}
		if clients.createCalls != 0 {
			t.Errorf("AddSelfAsClient() created %d rows after a read failure, want 0", clients.createCalls)
		}
	})

	t.Run("client creation", func(t *testing.T) {
		accounts := &fakeAccountsByIDReader{
			account: &entity.Account{ID: selfClientPhone, Role: entity.RoleOwner, Active: true},
		}
		clients := &fakeClientsSelfService{findErr: domain.ErrNotFound, createErr: errors.New("disk I/O error")}

		_, err := AddSelfAsClient(context.Background(), accounts, clients, selfClientPhone, "Dueño Cliente")
		if err == nil {
			t.Fatal("AddSelfAsClient() error = nil, want the port failure")
		}
		if !strings.Contains(err.Error(), "registrar el cliente del operador") {
			t.Errorf("error = %v, want it to wrap the client creation", err)
		}
		if errors.Is(err, ErrClientPhoneTaken) {
			t.Errorf("error = %v, want no conflict classification for an I/O failure", err)
		}
	})
}

func TestAddSelfAsClient_RunsUnderTheFabricatedOwnerCaller(t *testing.T) {
	accounts := &fakeAccountsByIDReader{
		account: &entity.Account{ID: selfClientPhone, Role: entity.RoleOwner, Active: true},
	}
	clients := &fakeClientsSelfService{findErr: domain.ErrNotFound}

	outcome, err := AddSelfAsClient(context.Background(), accounts, clients, selfClientPhone, "Dueño Cliente")
	if err != nil {
		t.Fatalf("AddSelfAsClient() error = %v", err)
	}
	if outcome.ClientID != selfClientPhone {
		t.Errorf("ClientID = %q, want %q", outcome.ClientID, selfClientPhone)
	}

	for name, caller := range map[string]auth.Caller{
		"account lookup": accounts.caller,
		"client lookup":  clients.caller,
	} {
		if caller.ID != TUICallerID || caller.Role != auth.RoleOwner {
			t.Errorf("%s caller = %+v, want the fabricated %s owner", name, caller, TUICallerID)
		}
	}
	if clients.created == nil {
		t.Fatal("AddSelfAsClient() did not create a client row")
	}
	if clients.created.ID != selfClientPhone || clients.created.Phone != selfClientPhone {
		t.Errorf("created client id/phone = %q/%q, want both %q",
			clients.created.ID, clients.created.Phone, selfClientPhone)
	}
	if clients.created.Name != "Dueño Cliente" {
		t.Errorf("created client name = %q, want the trimmed display name", clients.created.Name)
	}
}

// fakeAccountsByIDReader records the calls and caller of the account port.
type fakeAccountsByIDReader struct {
	caller  auth.Caller
	calls   int
	lastID  string
	account *entity.Account
	err     error
}

func (f *fakeAccountsByIDReader) FindByID(ctx context.Context, id string) (*entity.Account, error) {
	f.calls++
	f.lastID = id
	f.caller, _ = auth.FromContext(ctx)
	return f.account, f.err
}

// fakeClientsSelfService records the calls and caller of the clients port.
type fakeClientsSelfService struct {
	caller      auth.Caller
	findCalls   int
	createCalls int
	existing    *entity.Client
	findErr     error
	created     *entity.Client
	createErr   error
}

func (f *fakeClientsSelfService) FindByID(ctx context.Context, id string) (*entity.Client, error) {
	f.findCalls++
	f.caller, _ = auth.FromContext(ctx)
	return f.existing, f.findErr
}

func (f *fakeClientsSelfService) Create(ctx context.Context, c *entity.Client) error {
	f.createCalls++
	f.caller, _ = auth.FromContext(ctx)
	f.created = c
	return f.createErr
}
