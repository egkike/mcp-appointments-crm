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

// findActiveProfessionalsQuery mirrors internal/repository/professionals.go
// byte for byte (raw string with a newline and the indentation tabs) so the
// QueryMatcherEqual mock fails loudly if the production query changes.
const findActiveProfessionalsQuery = "SELECT id, name, role_specialty, status, email, phone, specialties, created_at, updated_at\n" +
	"\t\t\t FROM professionals WHERE status = 'active' ORDER BY name"

// professionalColumns returns the column set ProfessionalsRepo.FindActive scans.
func professionalColumns() []string {
	return []string{"id", "name", "role_specialty", "status", "email", "phone", "specialties", "created_at", "updated_at"}
}

// newMockAdminRepos builds the real AccountsRepo and ProfessionalsRepo on top
// of one go-sqlmock handle so Add Staff is exercised against the production
// query contract of both ports.
func newMockAdminRepos(t *testing.T) (*repository.AccountsRepo, *repository.ProfessionalsRepo, sqlmock.Sqlmock) {
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
	return repository.NewAccountsRepo(db, slog.New(slog.DiscardHandler)), repository.NewProfessionalsRepo(db), mock
}

// activeProfessionalRow describes one professionals row for the mock.
type activeProfessionalRow struct {
	id     string
	name   string
	status string
	phone  *string
}

// activeProfessionalsRows builds the FindActive result set in the given order.
func activeProfessionalsRows(rows ...activeProfessionalRow) *sqlmock.Rows {
	result := sqlmock.NewRows(professionalColumns())
	for _, row := range rows {
		result.AddRow(row.id, row.name, nil, row.status, nil, row.phone, nil,
			"2026-01-01T00:00:00.000Z", "2026-01-01T00:00:00.000Z")
	}
	return result
}

func strPtr(s string) *string { return &s }

func TestMapPickableProfessionals_ProjectsIDNameAndPhone(t *testing.T) {
	t.Run("empty list maps to an empty slice", func(t *testing.T) {
		got := mapPickableProfessionals(nil)
		if got == nil {
			t.Error("mapPickableProfessionals(nil) = nil, want an empty slice")
		}
		if len(got) != 0 {
			t.Errorf("mapPickableProfessionals(nil) length = %d, want 0", len(got))
		}
	})

	t.Run("professionals keep the repository order", func(t *testing.T) {
		active := []*entity.Professional{
			{ID: "p1", Name: "Ana", Status: "active", Phone: strPtr("+5491100000001")},
			{ID: "p2", Name: "Bruno", Status: "active", Phone: nil},
			{ID: "p3", Name: "Carla", Status: "active", Phone: strPtr("+5491100000003")},
		}

		got := mapPickableProfessionals(active)
		if len(got) != 3 {
			t.Fatalf("mapPickableProfessionals() length = %d, want 3", len(got))
		}
		for i, want := range []string{"p1", "p2", "p3"} {
			if got[i].ID != want {
				t.Errorf("pickable[%d].ID = %q, want %q", i, got[i].ID, want)
			}
		}
		if got[0].Name != "Ana" || got[2].Name != "Carla" {
			t.Errorf("names = %q/%q, want Ana/Carla", got[0].Name, got[2].Name)
		}
	})

	t.Run("phone prefill copies the professional phone and tolerates nil", func(t *testing.T) {
		active := []*entity.Professional{
			{ID: "p1", Name: "Ana", Status: "active", Phone: strPtr("+5491100000001")},
			{ID: "p2", Name: "Bruno", Status: "active", Phone: nil},
		}

		got := mapPickableProfessionals(active)
		if got[0].Phone != "+5491100000001" {
			t.Errorf("pickable[0].Phone = %q, want the professional phone", got[0].Phone)
		}
		if got[1].Phone != "" {
			t.Errorf("pickable[1].Phone = %q, want an empty prefill for a nil phone", got[1].Phone)
		}
	})
}

func TestListPickableProfessionals_UsesTheActiveQueryInNameOrder(t *testing.T) {
	_, professionals, mock := newMockAdminRepos(t)

	mock.ExpectQuery(findActiveProfessionalsQuery).WillReturnRows(
		activeProfessionalsRows(
			activeProfessionalRow{id: "p1", name: "Ana", status: "active", phone: strPtr("+5491100000001")},
			activeProfessionalRow{id: "p2", name: "Bruno", status: "active", phone: nil},
		),
	)

	got, err := ListPickableProfessionals(context.Background(), professionals)
	if err != nil {
		t.Fatalf("ListPickableProfessionals() error = %v", err)
	}
	if len(got) != 2 || got[0].ID != "p1" || got[1].ID != "p2" {
		t.Fatalf("ListPickableProfessionals() = %+v, want Ana then Bruno", got)
	}
}

// fakeProfessionalsReader records the caller and returns scripted active rows.
type fakeProfessionalsReader struct {
	caller auth.Caller
	has    bool
	calls  int
	active []*entity.Professional
	err    error
}

func (f *fakeProfessionalsReader) FindActive(ctx context.Context) ([]*entity.Professional, error) {
	f.calls++
	f.caller, f.has = auth.FromContext(ctx)
	return f.active, f.err
}

// fakeAccountsCreator records the caller and the accounts it was asked to write.
type fakeAccountsCreator struct {
	caller  auth.Caller
	has     bool
	calls   int
	created []*entity.Account
	err     error
}

func (f *fakeAccountsCreator) Create(ctx context.Context, a *entity.Account) error {
	f.calls++
	f.caller, f.has = auth.FromContext(ctx)
	f.created = append(f.created, a)
	return f.err
}

func activeProfessional(id, name string) *entity.Professional {
	return &entity.Professional{ID: id, Name: name, Status: "active", Phone: strPtr("+5491100000001")}
}

func TestAddStaff_CreatesActiveStaffWithProfessionalID(t *testing.T) {
	accounts, professionals, mock := newMockAdminRepos(t)

	mock.ExpectQuery(findActiveProfessionalsQuery).WillReturnRows(
		activeProfessionalsRows(activeProfessionalRow{id: "prof-1", name: "Ana", status: "active", phone: strPtr("+5491100000001")}),
	)
	mock.ExpectExec(insertAccountQuery).
		WithArgs("+5491100000001", "staff", "Ana Staff", "prof-1", 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := AddStaff(context.Background(), professionals, accounts, StaffInput{
		ProfessionalID: "prof-1",
		Phone:          "+5491100000001",
		DisplayName:    "  Ana Staff  ",
	})
	if err != nil {
		t.Fatalf("AddStaff() error = %v", err)
	}
}

func TestAddStaff_DuplicatePhoneIsSemanticConflict(t *testing.T) {
	accounts, professionals, mock := newMockAdminRepos(t)

	mock.ExpectQuery(findActiveProfessionalsQuery).WillReturnRows(
		activeProfessionalsRows(activeProfessionalRow{id: "prof-1", name: "Ana", status: "active", phone: strPtr("+5491100000001")}),
	)
	mock.ExpectExec(insertAccountQuery).
		WillReturnError(errors.New("UNIQUE constraint failed: accounts.id"))

	err := AddStaff(context.Background(), professionals, accounts, StaffInput{
		ProfessionalID: "prof-1",
		Phone:          "+5491100000001",
		DisplayName:    "Ana Staff",
	})
	if err == nil {
		t.Fatal("AddStaff() error = nil, want the duplicate-phone outcome")
	}
	if !errors.Is(err, ErrStaffAlreadyExists) {
		t.Errorf("AddStaff() error = %v, want errors.Is(ErrStaffAlreadyExists)", err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("AddStaff() error = %v, want errors.Is(domain.ErrConflict)", err)
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("AddStaff() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeConflict {
		t.Errorf("AddStaff() code = %q, want %q", semErr.Code, domain.ErrCodeConflict)
	}
	if !strings.Contains(semErr.Message, "teléfono") {
		t.Errorf("AddStaff() message = %q, want it to name the phone", semErr.Message)
	}
	if strings.Contains(semErr.Message, "SQL") || strings.Contains(semErr.Message, "constraint") {
		t.Errorf("AddStaff() message = %q, want a semantic message without driver detail", semErr.Message)
	}
}

func TestAddStaff_UnexpectedRepoErrorIsWrapped(t *testing.T) {
	accounts, professionals, mock := newMockAdminRepos(t)

	mock.ExpectQuery(findActiveProfessionalsQuery).WillReturnRows(
		activeProfessionalsRows(activeProfessionalRow{id: "prof-1", name: "Ana", status: "active", phone: nil}),
	)
	mock.ExpectExec(insertAccountQuery).WillReturnError(errors.New("disk I/O error"))

	err := AddStaff(context.Background(), professionals, accounts, StaffInput{
		ProfessionalID: "prof-1",
		Phone:          "+5491100000001",
		DisplayName:    "Ana Staff",
	})
	if err == nil {
		t.Fatal("AddStaff() error = nil, want the repo failure")
	}
	if !strings.Contains(err.Error(), "crear cuenta de staff") {
		t.Errorf("AddStaff() error = %v, want it to wrap the staff creation", err)
	}
	if errors.Is(err, ErrStaffAlreadyExists) {
		t.Errorf("AddStaff() error = %v, want no conflict classification for an I/O failure", err)
	}
}

func TestAddStaff_FindActiveFailureIsWrapped(t *testing.T) {
	reader := &fakeProfessionalsReader{err: errors.New("database is down")}
	creator := &fakeAccountsCreator{}

	err := AddStaff(context.Background(), reader, creator, StaffInput{
		ProfessionalID: "prof-1",
		Phone:          "+5491100000001",
		DisplayName:    "Ana Staff",
	})
	if err == nil {
		t.Fatal("AddStaff() error = nil, want the port failure")
	}
	if !strings.Contains(err.Error(), "listar profesionales activos") {
		t.Errorf("AddStaff() error = %v, want it to wrap the professional listing", err)
	}
	if creator.calls != 0 {
		t.Errorf("AddStaff() created %d accounts after a read failure, want 0", creator.calls)
	}
}

func TestAddStaff_DanglingProfessionalIsRejected(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		active []*entity.Professional
	}{
		{
			name:   "no active professionals at all",
			input:  "prof-1",
			active: nil,
		},
		{
			name:   "id not present in the active list",
			input:  "prof-2",
			active: []*entity.Professional{activeProfessional("prof-1", "Ana")},
		},
		{
			name:   "id present but the row is not active",
			input:  "prof-1",
			active: []*entity.Professional{{ID: "prof-1", Name: "Ana", Status: "inactive"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeProfessionalsReader{active: tt.active}
			creator := &fakeAccountsCreator{}

			err := AddStaff(context.Background(), reader, creator, StaffInput{
				ProfessionalID: tt.input,
				Phone:          "+5491100000001",
				DisplayName:    "Ana Staff",
			})
			if err == nil {
				t.Fatal("AddStaff() error = nil, want a semantic rejection")
			}
			var semErr *domain.SemanticError
			if !errors.As(err, &semErr) {
				t.Fatalf("AddStaff() error = %T, want *domain.SemanticError", err)
			}
			if semErr.Code != domain.ErrCodeInvalidInput {
				t.Errorf("AddStaff() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
			}
			if !strings.Contains(semErr.Message, "no existe o no está activo") {
				t.Errorf("AddStaff() message = %q, want the not-active/not-found detail", semErr.Message)
			}
			if creator.calls != 0 {
				t.Errorf("AddStaff() created %d accounts for a dangling professional, want 0", creator.calls)
			}
		})
	}
}

func TestAddStaff_InvalidInputNeverTouchesThePorts(t *testing.T) {
	tests := []struct {
		name  string
		input StaffInput
		want  string
	}{
		{
			name:  "blank professional id",
			input: StaffInput{ProfessionalID: "  ", Phone: "+5491100000001", DisplayName: "Ana Staff"},
			want:  "profesional",
		},
		{
			name:  "phone with letters",
			input: StaffInput{ProfessionalID: "prof-1", Phone: "no-es-un-telefono", DisplayName: "Ana Staff"},
			want:  "teléfono",
		},
		{
			name:  "too short phone",
			input: StaffInput{ProfessionalID: "prof-1", Phone: "123", DisplayName: "Ana Staff"},
			want:  "teléfono",
		},
		{
			name:  "blank display name",
			input: StaffInput{ProfessionalID: "prof-1", Phone: "+5491100000001", DisplayName: "   "},
			want:  "nombre",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reader := &fakeProfessionalsReader{active: []*entity.Professional{activeProfessional("prof-1", "Ana")}}
			creator := &fakeAccountsCreator{}

			err := AddStaff(context.Background(), reader, creator, tt.input)
			if err == nil {
				t.Fatal("AddStaff() error = nil, want a validation error")
			}
			var semErr *domain.SemanticError
			if !errors.As(err, &semErr) {
				t.Fatalf("AddStaff() error = %T, want *domain.SemanticError", err)
			}
			if semErr.Code != domain.ErrCodeInvalidInput {
				t.Errorf("AddStaff() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
			}
			if !strings.Contains(semErr.Message, tt.want) {
				t.Errorf("AddStaff() message = %q, want it to mention %q", semErr.Message, tt.want)
			}
			if reader.calls != 0 {
				t.Errorf("AddStaff() read the professionals port %d times for invalid input, want 0", reader.calls)
			}
			if creator.calls != 0 {
				t.Errorf("AddStaff() created %d accounts for invalid input, want 0", creator.calls)
			}
		})
	}
}

func TestAddStaff_FabricatesOwnerCallerForBothPorts(t *testing.T) {
	reader := &fakeProfessionalsReader{active: []*entity.Professional{activeProfessional("prof-1", "Ana")}}
	creator := &fakeAccountsCreator{}

	err := AddStaff(context.Background(), reader, creator, StaffInput{
		ProfessionalID: "prof-1",
		Phone:          "+5491100000001",
		DisplayName:    "Ana Staff",
	})
	if err != nil {
		t.Fatalf("AddStaff() error = %v", err)
	}

	want := auth.Caller{ID: TUICallerID, Role: auth.RoleOwner}
	if !reader.has || reader.caller != want {
		t.Errorf("AddStaff() professionals caller = %+v (attached=%v), want %+v", reader.caller, reader.has, want)
	}
	if !creator.has || creator.caller != want {
		t.Errorf("AddStaff() accounts caller = %+v (attached=%v), want %+v", creator.caller, creator.has, want)
	}

	if len(creator.created) != 1 {
		t.Fatalf("AddStaff() created %d accounts, want 1", len(creator.created))
	}
	account := creator.created[0]
	if account.Role != entity.RoleStaff {
		t.Errorf("account.Role = %q, want %q", account.Role, entity.RoleStaff)
	}
	if !account.Active {
		t.Error("account.Active = false, want an active staff account")
	}
	if account.ProfessionalID == nil || *account.ProfessionalID != "prof-1" {
		t.Errorf("account.ProfessionalID = %v, want prof-1", account.ProfessionalID)
	}
	if account.ID != "+5491100000001" {
		t.Errorf("account.ID = %q, want the phone", account.ID)
	}
	if account.DisplayName != "Ana Staff" {
		t.Errorf("account.DisplayName = %q, want the trimmed name", account.DisplayName)
	}
}
