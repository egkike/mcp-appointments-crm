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
