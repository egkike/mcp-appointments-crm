package admin

import (
	"context"
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

// Exact query text mirrors internal/repository/accounts.go; the mock uses
// QueryMatcherEqual so a repository query change fails this suite loudly.
const (
	selectOwnersQuery      = `SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE role = ? ORDER BY created_at ASC`
	insertAccountQuery     = `INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`
	countActiveOwnersQuery = `SELECT COUNT(*) FROM accounts WHERE role = 'owner' AND is_active = 1`
)

// accountColumns returns the column set AccountsRepo scans.
func accountColumns() []string {
	return []string{"id", "role", "display_name", "professional_id", "is_active", "created_at", "updated_at"}
}

// newMockAccountsRepo builds the real AccountsRepo on top of go-sqlmock so the
// seed decision and flow are exercised against the production query contract.
func newMockAccountsRepo(t *testing.T) (*repository.AccountsRepo, sqlmock.Sqlmock) {
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
	return repository.NewAccountsRepo(db, slog.New(slog.DiscardHandler)), mock
}

func TestTUIContext_SetsFabricatedOwnerCaller(t *testing.T) {
	ctx := TUIContext(context.Background())

	caller, ok := auth.FromContext(ctx)
	if !ok {
		t.Fatal("TUIContext() did not attach a Caller")
	}
	if caller.ID != TUICallerID {
		t.Errorf("caller.ID = %q, want %q", caller.ID, TUICallerID)
	}
	if caller.Role != auth.RoleOwner {
		t.Errorf("caller.Role = %q, want %q", caller.Role, auth.RoleOwner)
	}
	if caller.ProfessionalID != nil || caller.ClientID != nil {
		t.Errorf("caller = %+v, want no professional/client scope", caller)
	}
}

func TestNeedsSeed(t *testing.T) {
	tests := []struct {
		name string
		rows *sqlmock.Rows
		want bool
	}{
		{
			name: "zero owner accounts needs seed",
			rows: sqlmock.NewRows(accountColumns()),
			want: true,
		},
		{
			name: "only deactivated owners needs seed",
			rows: sqlmock.NewRows(accountColumns()).AddRow(
				"+5491100000000", "owner", "Dueño viejo", nil, 0,
				"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z"),
			want: true,
		},
		{
			name: "active owner does not need seed",
			rows: sqlmock.NewRows(accountColumns()).AddRow(
				"+5491100000000", "owner", "Dueño", nil, 1,
				"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, mock := newMockAccountsRepo(t)
			mock.ExpectQuery(selectOwnersQuery).WithArgs("owner").WillReturnRows(tt.rows)

			got, err := NeedsSeed(context.Background(), repo)
			if err != nil {
				t.Fatalf("NeedsSeed() error = %v", err)
			}
			if got != tt.want {
				t.Errorf("NeedsSeed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNeedsSeed_RepoErrorIsWrapped(t *testing.T) {
	repo, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectOwnersQuery).WithArgs("owner").
		WillReturnError(errors.New("database is down"))

	_, err := NeedsSeed(context.Background(), repo)
	if err == nil {
		t.Fatal("NeedsSeed() error = nil, want the repo failure")
	}
	if !strings.Contains(err.Error(), "verificar owner activo") {
		t.Errorf("NeedsSeed() error = %v, want it to wrap the active-owner check", err)
	}
	if !strings.Contains(err.Error(), "database is down") {
		t.Errorf("NeedsSeed() error = %v, want it to preserve the cause", err)
	}
}

// callerCapturingReader records what NeedsSeed passes to the repository, so the
// fabricated owner caller and the owner role filter are asserted directly.
type callerCapturingReader struct {
	caller auth.Caller
	has    bool
	role   entity.AccountRole
	owners []*entity.Account
	err    error
}

func (c *callerCapturingReader) GetByRole(ctx context.Context, role entity.AccountRole) ([]*entity.Account, error) {
	c.caller, c.has = auth.FromContext(ctx)
	c.role = role
	return c.owners, c.err
}

func TestNeedsSeed_FabricatesOwnerCallerForTheRepo(t *testing.T) {
	reader := &callerCapturingReader{}

	if _, err := NeedsSeed(context.Background(), reader); err != nil {
		t.Fatalf("NeedsSeed() error = %v", err)
	}

	if !reader.has {
		t.Fatal("NeedsSeed() did not attach a Caller before querying the repo")
	}
	if reader.caller != (auth.Caller{ID: TUICallerID, Role: auth.RoleOwner}) {
		t.Errorf("NeedsSeed() caller = %+v, want {ID: %q, Role: %q}", reader.caller, TUICallerID, auth.RoleOwner)
	}
	if reader.role != entity.RoleOwner {
		t.Errorf("NeedsSeed() role filter = %q, want %q", reader.role, entity.RoleOwner)
	}
}

func TestSeed_CreatesActiveOwnerWithPhoneAsID(t *testing.T) {
	repo, mock := newMockAccountsRepo(t)

	mock.ExpectQuery(countActiveOwnersQuery).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(insertAccountQuery).
		WithArgs("+5491100000000", "owner", "Dueño", nil, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := Seed(context.Background(), repo, SeedInput{Phone: "+5491100000000", DisplayName: "  Dueño  "})
	if err != nil {
		t.Fatalf("Seed() error = %v", err)
	}
}

func TestSeed_OwnerAlreadyExistsIsSemanticConflict(t *testing.T) {
	repo, mock := newMockAccountsRepo(t)

	mock.ExpectQuery(countActiveOwnersQuery).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err := Seed(context.Background(), repo, SeedInput{Phone: "+5491100009999", DisplayName: "Otro"})
	if err == nil {
		t.Fatal("Seed() error = nil, want the owner-already-exists outcome")
	}
	if !errors.Is(err, ErrOwnerAlreadyExists) {
		t.Errorf("Seed() error = %v, want errors.Is(ErrOwnerAlreadyExists)", err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("Seed() error = %v, want errors.Is(domain.ErrConflict)", err)
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("Seed() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeConflict {
		t.Errorf("Seed() code = %q, want %q", semErr.Code, domain.ErrCodeConflict)
	}
	if !strings.Contains(semErr.Message, "owner") {
		t.Errorf("Seed() message = %q, want it to name the existing owner", semErr.Message)
	}
	if strings.Contains(semErr.Message, "SQL") || strings.Contains(semErr.Message, "constraint") {
		t.Errorf("Seed() message = %q, want a semantic message without driver detail", semErr.Message)
	}
}

func TestSeed_UniqueViolationOnInsertIsAlsoSemanticConflict(t *testing.T) {
	repo, mock := newMockAccountsRepo(t)

	mock.ExpectQuery(countActiveOwnersQuery).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(insertAccountQuery).
		WithArgs("+5491100000000", "owner", "Dueño", nil, 1).
		WillReturnError(errors.New("UNIQUE constraint failed: accounts.id"))

	err := Seed(context.Background(), repo, SeedInput{Phone: "+5491100000000", DisplayName: "Dueño"})
	if !errors.Is(err, ErrOwnerAlreadyExists) {
		t.Errorf("Seed() error = %v, want errors.Is(ErrOwnerAlreadyExists)", err)
	}
}

func TestSeed_UnexpectedRepoErrorIsWrapped(t *testing.T) {
	repo, mock := newMockAccountsRepo(t)

	mock.ExpectQuery(countActiveOwnersQuery).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(insertAccountQuery).
		WithArgs("+5491100000000", "owner", "Dueño", nil, 1).
		WillReturnError(errors.New("disk I/O error"))

	err := Seed(context.Background(), repo, SeedInput{Phone: "+5491100000000", DisplayName: "Dueño"})
	if err == nil {
		t.Fatal("Seed() error = nil, want the repo failure")
	}
	if !strings.Contains(err.Error(), "crear owner") {
		t.Errorf("Seed() error = %v, want it to wrap the owner creation", err)
	}
	if errors.Is(err, ErrOwnerAlreadyExists) {
		t.Errorf("Seed() error = %v, want no already-exists classification for an I/O failure", err)
	}
}

func TestSeed_InvalidInputNeverTouchesTheDatabase(t *testing.T) {
	tests := []struct {
		name  string
		input SeedInput
		want  string
	}{
		{
			name:  "empty phone",
			input: SeedInput{Phone: "", DisplayName: "Dueño"},
			want:  "teléfono",
		},
		{
			name:  "phone with letters",
			input: SeedInput{Phone: "no-es-un-telefono", DisplayName: "Dueño"},
			want:  "teléfono",
		},
		{
			name:  "too short phone",
			input: SeedInput{Phone: "123", DisplayName: "Dueño"},
			want:  "teléfono",
		},
		{
			name:  "blank display name",
			input: SeedInput{Phone: "+5491100000000", DisplayName: "   "},
			want:  "nombre",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo, _ := newMockAccountsRepo(t) // no expectations: any DB access fails the test

			err := Seed(context.Background(), repo, tt.input)
			if err == nil {
				t.Fatal("Seed() error = nil, want a validation error")
			}
			var semErr *domain.SemanticError
			if !errors.As(err, &semErr) {
				t.Fatalf("Seed() error = %T, want *domain.SemanticError", err)
			}
			if semErr.Code != domain.ErrCodeInvalidInput {
				t.Errorf("Seed() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
			}
			if !strings.Contains(semErr.Message, tt.want) {
				t.Errorf("Seed() message = %q, want it to mention %q", semErr.Message, tt.want)
			}
		})
	}
}

func TestValidatePhone_DelegatesToEntityClient(t *testing.T) {
	tests := []struct {
		name  string
		phone string
		valid bool
	}{
		{name: "e164 with plus", phone: "+5491100000000", valid: true},
		{name: "four digits", phone: "1234", valid: true},
		{name: "plus and four digits", phone: "+1234", valid: true},
		{name: "empty", phone: "", valid: false},
		{name: "letters", phone: "abcdef", valid: false},
		{name: "three digits", phone: "123", valid: false},
		{name: "plus only three digits", phone: "+123", valid: false},
		{name: "spaces", phone: "12 34", valid: false},
		{name: "separators", phone: "123-4567", valid: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePhone(tt.phone)
			if tt.valid && err != nil {
				t.Errorf("ValidatePhone(%q) error = %v, want nil", tt.phone, err)
			}
			if !tt.valid && err == nil {
				t.Errorf("ValidatePhone(%q) = nil, want a semantic error", tt.phone)
			}
		})
	}
}

// ── R4-deactivated-owner-seed-deadend ──────────────────────────────────────

// fakeActivator records the deadend-recovery port calls so the guards can be
// tested without a database.
type fakeActivator struct {
	owners    []*entity.Account
	ownersErr error
	getCalls  int
	getCaller auth.Caller
	getHas    bool

	updateCalls  int
	updated      *entity.Account
	updateErr    error
	updateCaller auth.Caller
	updateHas    bool
}

func (f *fakeActivator) GetByRole(ctx context.Context, _ entity.AccountRole) ([]*entity.Account, error) {
	f.getCalls++
	f.getCaller, f.getHas = auth.FromContext(ctx)
	return f.owners, f.ownersErr
}

func (f *fakeActivator) Update(ctx context.Context, a *entity.Account) error {
	f.updateCalls++
	f.updated = a
	f.updateCaller, f.updateHas = auth.FromContext(ctx)
	return f.updateErr
}

func TestInactiveOwners_ReturnsOnlyInactiveOwnerRows(t *testing.T) {
	repo, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectOwnersQuery).WithArgs("owner").WillReturnRows(accountsRows(
		accountRow{id: "+5491100000000", role: entity.RoleOwner, displayName: "Dueño viejo", isActive: 0},
		accountRow{id: "+5491100000001", role: entity.RoleOwner, displayName: "Dueño activo", isActive: 1},
	))

	got, err := InactiveOwners(context.Background(), repo)
	if err != nil {
		t.Fatalf("InactiveOwners() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("InactiveOwners() length = %d, want 1", len(got))
	}
	if got[0].ID != "+5491100000000" || got[0].Active {
		t.Errorf("InactiveOwners()[0] = %+v, want the deactivated owner row", got[0])
	}
	if got[0].DisplayName != "Dueño viejo" {
		t.Errorf("InactiveOwners()[0].DisplayName = %q, want the stored name", got[0].DisplayName)
	}
}

func TestInactiveOwners_EmptyWhenOnlyActiveOwnersExist(t *testing.T) {
	repo, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectOwnersQuery).WithArgs("owner").WillReturnRows(accountsRows(
		accountRow{id: "+5491100000000", role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
	))

	got, err := InactiveOwners(context.Background(), repo)
	if err != nil {
		t.Fatalf("InactiveOwners() error = %v", err)
	}
	if got == nil {
		t.Error("InactiveOwners() = nil, want an empty slice")
	}
	if len(got) != 0 {
		t.Errorf("InactiveOwners() length = %d, want 0 for a healthy installation", len(got))
	}
}

func TestInactiveOwners_RepoErrorIsWrapped(t *testing.T) {
	repo, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectOwnersQuery).WithArgs("owner").
		WillReturnError(errors.New("database is down"))

	_, err := InactiveOwners(context.Background(), repo)
	if err == nil {
		t.Fatal("InactiveOwners() error = nil, want the port failure")
	}
	if !strings.Contains(err.Error(), "listar owners inactivos") {
		t.Errorf("InactiveOwners() error = %v, want it to wrap the listing", err)
	}
}

func TestInactiveOwners_FabricatesOwnerCallerForTheRepo(t *testing.T) {
	reader := &callerCapturingReader{owners: []*entity.Account{
		accountFixture("+5491100000000", entity.RoleOwner, false),
	}}

	if _, err := InactiveOwners(context.Background(), reader); err != nil {
		t.Fatalf("InactiveOwners() error = %v", err)
	}

	if !reader.has {
		t.Fatal("InactiveOwners() did not attach a Caller before querying the repo")
	}
	if reader.caller != (auth.Caller{ID: TUICallerID, Role: auth.RoleOwner}) {
		t.Errorf("InactiveOwners() caller = %+v, want the fabricated owner caller", reader.caller)
	}
	if reader.role != entity.RoleOwner {
		t.Errorf("InactiveOwners() role filter = %q, want %q", reader.role, entity.RoleOwner)
	}
}

// TestReactivateOwner_ActivatesInactiveOwner locks the deadend recovery write:
// the row keeps its name, gains is_active=1 and the write runs under the
// fabricated owner Caller.
func TestReactivateOwner_ActivatesInactiveOwner(t *testing.T) {
	repo, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectOwnersQuery).WithArgs("owner").WillReturnRows(accountsRows(
		accountRow{id: "+5491100000000", role: entity.RoleOwner, displayName: "Dueño viejo", isActive: 0},
	))
	mock.ExpectQuery(selectAccountExistsQuery).WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectExec(updateAccountFullQuery).
		WithArgs("owner", "Dueño viejo", nil, 1, "+5491100000000").
		WillReturnResult(sqlmock.NewResult(0, 1))

	got, err := ReactivateOwner(context.Background(), repo, "  +5491100000000  ")
	if err != nil {
		t.Fatalf("ReactivateOwner() error = %v", err)
	}
	if got.ID != "+5491100000000" || !got.Active || got.Role != entity.RoleOwner {
		t.Errorf("ReactivateOwner() = %+v, want the reactivated owner view", got)
	}
	if got.DisplayName != "Dueño viejo" {
		t.Errorf("ReactivateOwner().DisplayName = %q, want the stored name preserved", got.DisplayName)
	}
}

// TestReactivateOwner_RefusesWhenAnotherOwnerIsActive pins the boundary of the
// recovery: it is for the ownerless deadend only. With an ACTIVE owner in
// place, changing owners is the transfer flow, never a blind activation — and
// the SQLite trigger would reject the write anyway.
func TestReactivateOwner_RefusesWhenAnotherOwnerIsActive(t *testing.T) {
	fake := &fakeActivator{owners: []*entity.Account{
		accountFixture("+5491100000000", entity.RoleOwner, true),
		accountFixture("+5491100000001", entity.RoleOwner, false),
	}}

	_, err := ReactivateOwner(context.Background(), fake, "+5491100000001")
	if err == nil {
		t.Fatal("ReactivateOwner() error = nil, want the existing-owner refusal")
	}
	if !errors.Is(err, ErrOwnerAlreadyExists) {
		t.Errorf("ReactivateOwner() error = %v, want errors.Is(ErrOwnerAlreadyExists)", err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("ReactivateOwner() error = %v, want errors.Is(domain.ErrConflict)", err)
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("ReactivateOwner() error = %T, want *domain.SemanticError", err)
	}
	if !strings.Contains(semErr.Message, "transferencia") {
		t.Errorf("ReactivateOwner() message = %q, want it to point at the transfer flow", semErr.Message)
	}
	if fake.updateCalls != 0 {
		t.Errorf("ReactivateOwner() wrote %d times, want 0", fake.updateCalls)
	}
}

func TestReactivateOwner_TargetMissing_NotFound(t *testing.T) {
	fake := &fakeActivator{owners: []*entity.Account{
		accountFixture("+5491100000000", entity.RoleOwner, false),
	}}

	_, err := ReactivateOwner(context.Background(), fake, "+5491100000009")
	if err == nil {
		t.Fatal("ReactivateOwner() error = nil, want the not-found rejection")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("ReactivateOwner() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeNotFound {
		t.Errorf("ReactivateOwner() code = %q, want %q", semErr.Code, domain.ErrCodeNotFound)
	}
	if fake.updateCalls != 0 {
		t.Errorf("ReactivateOwner() wrote %d times for a missing row, want 0", fake.updateCalls)
	}
}

func TestReactivateOwner_BlankIDIsRejectedBeforeThePort(t *testing.T) {
	fake := &fakeActivator{}

	_, err := ReactivateOwner(context.Background(), fake, "   ")
	if err == nil {
		t.Fatal("ReactivateOwner() error = nil, want the blank-id rejection")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("ReactivateOwner() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("ReactivateOwner() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
	if fake.getCalls != 0 || fake.updateCalls != 0 {
		t.Errorf("ReactivateOwner() reached the port (get=%d, update=%d) for a blank id, want 0",
			fake.getCalls, fake.updateCalls)
	}
}

func TestReactivateOwner_UpdateFailureIsWrapped(t *testing.T) {
	t.Run("generic failure", func(t *testing.T) {
		fake := &fakeActivator{
			owners:    []*entity.Account{accountFixture("+5491100000000", entity.RoleOwner, false)},
			updateErr: errors.New("disk I/O error"),
		}

		_, err := ReactivateOwner(context.Background(), fake, "+5491100000000")
		if err == nil {
			t.Fatal("ReactivateOwner() error = nil, want the write failure")
		}
		if !strings.Contains(err.Error(), "reactivar la cuenta de owner") {
			t.Errorf("ReactivateOwner() error = %v, want it to wrap the reactivation", err)
		}
		if errors.Is(err, ErrOwnerAlreadyExists) {
			t.Errorf("ReactivateOwner() error = %v, want no business classification for an I/O failure", err)
		}
	})

	t.Run("read failure", func(t *testing.T) {
		fake := &fakeActivator{ownersErr: errors.New("database is down")}

		_, err := ReactivateOwner(context.Background(), fake, "+5491100000000")
		if err == nil {
			t.Fatal("ReactivateOwner() error = nil, want the port failure")
		}
		if !strings.Contains(err.Error(), "listar owners") {
			t.Errorf("ReactivateOwner() error = %v, want it to wrap the listing", err)
		}
		if fake.updateCalls != 0 {
			t.Errorf("ReactivateOwner() wrote %d times after a read failure, want 0", fake.updateCalls)
		}
	})
}

func TestReactivateOwner_FabricatesOwnerCallerForThePort(t *testing.T) {
	fake := &fakeActivator{owners: []*entity.Account{
		accountFixture("+5491100000000", entity.RoleOwner, false),
	}}

	if _, err := ReactivateOwner(context.Background(), fake, "+5491100000000"); err != nil {
		t.Fatalf("ReactivateOwner() error = %v", err)
	}

	want := auth.Caller{ID: TUICallerID, Role: auth.RoleOwner}
	if !fake.getHas || fake.getCaller != want {
		t.Errorf("GetByRole caller = %+v (attached=%v), want %+v", fake.getCaller, fake.getHas, want)
	}
	if !fake.updateHas || fake.updateCaller != want {
		t.Errorf("Update caller = %+v (attached=%v), want %+v", fake.updateCaller, fake.updateHas, want)
	}
}
