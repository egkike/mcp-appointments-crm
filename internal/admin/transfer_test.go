package admin

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
)

// Exact statement text mirrors internal/repository/accounts.go; the mock uses
// QueryMatcherEqual so a repository statement change fails this suite loudly.
const (
	selectAccountExistsQuery = `SELECT 1 FROM accounts WHERE id = ?`
	updateAccountFullQuery   = `UPDATE accounts SET role = ?, display_name = ?, professional_id = ?, is_active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`

	transferStateQuery        = `SELECT role, is_active FROM accounts WHERE id = ?`
	transferUpdateActiveQuery = `UPDATE accounts SET is_active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`
)

const (
	transferFromPhone = "+5491100000000"
	transferToPhone   = "+5491100000001"
	transferNewPhone  = "+5491100000002"
)

// fakeAccountsTransfer records every port call of the transfer flow so the
// core pre-checks can be tested without a database.
type fakeAccountsTransfer struct {
	listCalls  int
	listRows   []*entity.Account
	listErr    error
	listCaller auth.Caller
	listHas    bool

	createCalls int
	created     *entity.Account
	createErr   error

	updateCalls int
	updated     *entity.Account
	updateErr   error

	transferCalls  int
	transferFrom   string
	transferTo     string
	transferErr    error
	transferCaller auth.Caller
	transferHas    bool
}

func (f *fakeAccountsTransfer) List(ctx context.Context) ([]*entity.Account, error) {
	f.listCalls++
	f.listCaller, f.listHas = auth.FromContext(ctx)
	return f.listRows, f.listErr
}

func (f *fakeAccountsTransfer) Create(_ context.Context, a *entity.Account) error {
	f.createCalls++
	f.created = a
	return f.createErr
}

func (f *fakeAccountsTransfer) Update(_ context.Context, a *entity.Account) error {
	f.updateCalls++
	f.updated = a
	return f.updateErr
}

func (f *fakeAccountsTransfer) TransferOwnership(ctx context.Context, fromID, toID string) error {
	f.transferCalls++
	f.transferFrom, f.transferTo = fromID, toID
	f.transferCaller, f.transferHas = auth.FromContext(ctx)
	return f.transferErr
}

func TestActiveOwner_ReturnsTheOnlyActiveOwner(t *testing.T) {
	fake := &fakeAccountsTransfer{listRows: []*entity.Account{
		accountFixture(transferToPhone, entity.RoleOwner, false),
		accountFixture(transferFromPhone, entity.RoleOwner, true),
		accountFixture(transferNewPhone, entity.RoleStaff, true),
	}}

	got, err := ActiveOwner(context.Background(), fake)
	if err != nil {
		t.Fatalf("ActiveOwner() error = %v", err)
	}
	if got.ID != transferFromPhone {
		t.Errorf("ActiveOwner().ID = %q, want the active owner %q", got.ID, transferFromPhone)
	}
	if !got.Active || got.Role != entity.RoleOwner {
		t.Errorf("ActiveOwner() = %+v, want the active owner view", got)
	}
	if fake.listCalls != 1 {
		t.Errorf("List calls = %d, want 1", fake.listCalls)
	}
}

func TestActiveOwner_NoActiveOwner_ErrNoActiveOwner(t *testing.T) {
	fake := &fakeAccountsTransfer{listRows: []*entity.Account{
		accountFixture(transferFromPhone, entity.RoleOwner, false),
	}}

	_, err := ActiveOwner(context.Background(), fake)
	if err == nil {
		t.Fatal("ActiveOwner() error = nil, want the no-active-owner refusal")
	}
	if !errors.Is(err, ErrNoActiveOwner) {
		t.Errorf("ActiveOwner() error = %v, want errors.Is(ErrNoActiveOwner)", err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("ActiveOwner() error = %v, want errors.Is(domain.ErrConflict)", err)
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("ActiveOwner() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeConflict {
		t.Errorf("ActiveOwner() code = %q, want %q", semErr.Code, domain.ErrCodeConflict)
	}
}

// TestActiveOwner_MultipleActiveOwners pins the fail-secure choice: the SQLite
// triggers forbid that state, so it can only come from manual SQL or corruption,
// and the flow refuses instead of guessing which owner to deactivate.
func TestActiveOwner_MultipleActiveOwners_ErrMultipleActiveOwners(t *testing.T) {
	fake := &fakeAccountsTransfer{listRows: []*entity.Account{
		accountFixture(transferFromPhone, entity.RoleOwner, true),
		accountFixture(transferToPhone, entity.RoleOwner, true),
	}}

	_, err := ActiveOwner(context.Background(), fake)
	if !errors.Is(err, ErrMultipleActiveOwners) {
		t.Fatalf("ActiveOwner() error = %v, want errors.Is(ErrMultipleActiveOwners)", err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("ActiveOwner() error = %v, want errors.Is(domain.ErrConflict)", err)
	}
}

func TestActiveOwner_RepoErrorIsWrapped(t *testing.T) {
	fake := &fakeAccountsTransfer{listErr: errors.New("database is down")}

	_, err := ActiveOwner(context.Background(), fake)
	if err == nil {
		t.Fatal("ActiveOwner() error = nil, want the port failure")
	}
	if !strings.Contains(err.Error(), "listar cuentas") {
		t.Errorf("ActiveOwner() error = %v, want it to wrap the listing", err)
	}
}

func TestPrepareSuccessor_InactiveOwnerRowIsReusedWithoutWriting(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectAllAccountsQuery).WillReturnRows(accountsRows(
		accountRow{id: transferFromPhone, role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
		accountRow{id: transferToPhone, role: entity.RoleOwner, displayName: "Sucesor", isActive: 0},
	))

	got, err := PrepareSuccessor(context.Background(), accounts,
		TransferSuccessor{Kind: SuccessorInactiveOwner, ID: " " + transferToPhone + " "})
	if err != nil {
		t.Fatalf("PrepareSuccessor() error = %v", err)
	}
	if got.ID != transferToPhone || got.Active || got.Role != entity.RoleOwner {
		t.Errorf("PrepareSuccessor() = %+v, want the inactive owner row reused", got)
	}
	if got.DisplayName != "Sucesor" {
		t.Errorf("PrepareSuccessor().DisplayName = %q, want the stored name", got.DisplayName)
	}
}

func TestPrepareSuccessor_InactiveOwnerNotFound(t *testing.T) {
	fake := &fakeAccountsTransfer{listRows: []*entity.Account{
		accountFixture(transferFromPhone, entity.RoleOwner, true),
	}}

	_, err := PrepareSuccessor(context.Background(), fake,
		TransferSuccessor{Kind: SuccessorInactiveOwner, ID: transferToPhone})
	if err == nil {
		t.Fatal("PrepareSuccessor() error = nil, want the not-found rejection")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("PrepareSuccessor() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeNotFound {
		t.Errorf("PrepareSuccessor() code = %q, want %q", semErr.Code, domain.ErrCodeNotFound)
	}
	if fake.createCalls != 0 || fake.updateCalls != 0 || fake.transferCalls != 0 {
		t.Errorf("PrepareSuccessor() wrote through the port (create=%d, update=%d, transfer=%d), want 0",
			fake.createCalls, fake.updateCalls, fake.transferCalls)
	}
}

func TestPrepareSuccessor_InactiveOwnerAlreadyActive_ErrSuccessorNotTransferable(t *testing.T) {
	fake := &fakeAccountsTransfer{listRows: []*entity.Account{
		accountFixture(transferFromPhone, entity.RoleOwner, true),
		accountFixture(transferToPhone, entity.RoleOwner, true),
	}}

	_, err := PrepareSuccessor(context.Background(), fake,
		TransferSuccessor{Kind: SuccessorInactiveOwner, ID: transferToPhone})
	if !errors.Is(err, ErrSuccessorNotTransferable) {
		t.Fatalf("PrepareSuccessor() error = %v, want errors.Is(ErrSuccessorNotTransferable)", err)
	}
	if fake.updateCalls != 0 {
		t.Errorf("PrepareSuccessor() updated %d accounts, want 0 for an already active successor", fake.updateCalls)
	}
}

// TestPrepareSuccessor_StaffIsPromotedToInactiveOwner locks the promotion
// write: role=owner with is_active=0 (never active, so the single-owner trigger
// is not violated while the current owner is still active) and a dropped
// professional link (entity.Account documents professional_id as staff-only).
func TestPrepareSuccessor_StaffIsPromotedToInactiveOwner(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectAllAccountsQuery).WillReturnRows(accountsRows(
		accountRow{id: transferFromPhone, role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
		accountRow{id: transferToPhone, role: entity.RoleStaff, displayName: "Ana Staff",
			professionalID: strPtr("prof-1"), isActive: 1},
	))
	mock.ExpectQuery(selectAccountExistsQuery).WithArgs(transferToPhone).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectExec(updateAccountFullQuery).
		WithArgs("owner", "Ana Staff", nil, 0, transferToPhone).
		WillReturnResult(sqlmock.NewResult(0, 1))

	got, err := PrepareSuccessor(context.Background(), accounts,
		TransferSuccessor{Kind: SuccessorStaff, ID: transferToPhone})
	if err != nil {
		t.Fatalf("PrepareSuccessor() error = %v", err)
	}
	if got.Role != entity.RoleOwner {
		t.Errorf("promoted role = %q, want %q", got.Role, entity.RoleOwner)
	}
	if got.Active {
		t.Error("promoted account is ACTIVE, want it prepared inactive for the swap")
	}
	if got.ProfessionalID != "" {
		t.Errorf("promoted ProfessionalID = %q, want the professional link dropped", got.ProfessionalID)
	}
	if got.DisplayName != "Ana Staff" {
		t.Errorf("promoted DisplayName = %q, want the staff name preserved", got.DisplayName)
	}
}

func TestPrepareSuccessor_StaffMissing_NotFound(t *testing.T) {
	fake := &fakeAccountsTransfer{listRows: []*entity.Account{
		accountFixture(transferFromPhone, entity.RoleOwner, true),
	}}

	_, err := PrepareSuccessor(context.Background(), fake,
		TransferSuccessor{Kind: SuccessorStaff, ID: transferToPhone})
	if err == nil {
		t.Fatal("PrepareSuccessor() error = nil, want the not-found rejection")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("PrepareSuccessor() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeNotFound {
		t.Errorf("PrepareSuccessor() code = %q, want %q", semErr.Code, domain.ErrCodeNotFound)
	}
}

func TestPrepareSuccessor_StaffPromotionUpdateFailureIsWrapped(t *testing.T) {
	fake := &fakeAccountsTransfer{
		listRows:  []*entity.Account{accountFixture(transferToPhone, entity.RoleStaff, true)},
		updateErr: errors.New("disk I/O error"),
	}

	_, err := PrepareSuccessor(context.Background(), fake,
		TransferSuccessor{Kind: SuccessorStaff, ID: transferToPhone})
	if err == nil {
		t.Fatal("PrepareSuccessor() error = nil, want the write failure")
	}
	if !strings.Contains(err.Error(), "promover la cuenta de staff") {
		t.Errorf("PrepareSuccessor() error = %v, want it to wrap the promotion", err)
	}
}

// TestPrepareSuccessor_NewPhoneCreatesInactiveOwner locks step 1 of the
// transfer: the fresh owner row is INSERTed with is_active=0 so it can coexist
// with the current ACTIVE owner (the triggers count only active owners).
func TestPrepareSuccessor_NewPhoneCreatesInactiveOwner(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectExec(insertAccountQuery).
		WithArgs(transferNewPhone, "owner", "Nuevo Dueño", nil, 0).
		WillReturnResult(sqlmock.NewResult(1, 1))

	got, err := PrepareSuccessor(context.Background(), accounts, TransferSuccessor{
		Kind:        SuccessorNewPhone,
		Phone:       " " + transferNewPhone + " ",
		DisplayName: "  Nuevo Dueño  ",
	})
	if err != nil {
		t.Fatalf("PrepareSuccessor() error = %v", err)
	}
	if got.ID != transferNewPhone || got.Role != entity.RoleOwner || got.Active {
		t.Errorf("PrepareSuccessor() = %+v, want a new inactive owner row", got)
	}
	if got.DisplayName != "Nuevo Dueño" {
		t.Errorf("PrepareSuccessor().DisplayName = %q, want the trimmed name", got.DisplayName)
	}
}

func TestPrepareSuccessor_NewPhoneConflictIsSemanticWithGuidance(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectExec(insertAccountQuery).
		WithArgs(transferNewPhone, "owner", "Nuevo Dueño", nil, 0).
		WillReturnError(errors.New("UNIQUE constraint failed: accounts.id"))

	_, err := PrepareSuccessor(context.Background(), accounts, TransferSuccessor{
		Kind:        SuccessorNewPhone,
		Phone:       transferNewPhone,
		DisplayName: "Nuevo Dueño",
	})
	if !errors.Is(err, ErrSuccessorPhoneTaken) {
		t.Fatalf("PrepareSuccessor() error = %v, want errors.Is(ErrSuccessorPhoneTaken)", err)
	}
	if !errors.Is(err, domain.ErrConflict) {
		t.Errorf("PrepareSuccessor() error = %v, want errors.Is(domain.ErrConflict)", err)
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("PrepareSuccessor() error = %T, want *domain.SemanticError", err)
	}
	if !strings.Contains(semErr.Message, "teléfono ya pertenece") {
		t.Errorf("PrepareSuccessor() message = %q, want the phone conflict", semErr.Message)
	}
	if !strings.Contains(semErr.Message, "elegí esa cuenta") {
		t.Errorf("PrepareSuccessor() message = %q, want the guidance to pick the existing account", semErr.Message)
	}
	if strings.Contains(semErr.Message, "constraint") || strings.Contains(semErr.Message, "SQL") {
		t.Errorf("PrepareSuccessor() message = %q, want no driver detail", semErr.Message)
	}
}

func TestPrepareSuccessor_InvalidKindIsRejectedBeforeThePort(t *testing.T) {
	fake := &fakeAccountsTransfer{}

	_, err := PrepareSuccessor(context.Background(), fake, TransferSuccessor{Kind: "promote_everything"})
	if err == nil {
		t.Fatal("PrepareSuccessor() error = nil, want the invalid-kind rejection")
	}

	var semErr *domain.SemanticError
	if !errors.As(err, &semErr) {
		t.Fatalf("PrepareSuccessor() error = %T, want *domain.SemanticError", err)
	}
	if semErr.Code != domain.ErrCodeInvalidInput {
		t.Errorf("PrepareSuccessor() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
	}
	if fake.listCalls != 0 || fake.createCalls != 0 || fake.updateCalls != 0 {
		t.Errorf("PrepareSuccessor() reached the port (list=%d, create=%d, update=%d), want 0",
			fake.listCalls, fake.createCalls, fake.updateCalls)
	}
}

func TestPrepareSuccessor_NewPhoneInvalidInputNeverTouchesThePort(t *testing.T) {
	tests := []struct {
		name      string
		successor TransferSuccessor
		want      string
	}{
		{
			name:      "invalid phone",
			successor: TransferSuccessor{Kind: SuccessorNewPhone, Phone: "no-es-un-telefono", DisplayName: "Dueño"},
			want:      "teléfono",
		},
		{
			name:      "blank display name",
			successor: TransferSuccessor{Kind: SuccessorNewPhone, Phone: transferNewPhone, DisplayName: "   "},
			want:      "nombre",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeAccountsTransfer{}

			_, err := PrepareSuccessor(context.Background(), fake, tt.successor)
			if err == nil {
				t.Fatal("PrepareSuccessor() error = nil, want a validation error")
			}

			var semErr *domain.SemanticError
			if !errors.As(err, &semErr) {
				t.Fatalf("PrepareSuccessor() error = %T, want *domain.SemanticError", err)
			}
			if semErr.Code != domain.ErrCodeInvalidInput {
				t.Errorf("PrepareSuccessor() code = %q, want %q", semErr.Code, domain.ErrCodeInvalidInput)
			}
			if !strings.Contains(semErr.Message, tt.want) {
				t.Errorf("PrepareSuccessor() message = %q, want it to mention %q", semErr.Message, tt.want)
			}
			if fake.createCalls != 0 {
				t.Errorf("PrepareSuccessor() created %d accounts for invalid input, want 0", fake.createCalls)
			}
		})
	}
}

func TestTransferOwnership_SwapsThePreparedSuccessor(t *testing.T) {
	accounts, mock := newMockAccountsRepo(t)
	mock.ExpectQuery(selectAllAccountsQuery).WillReturnRows(accountsRows(
		accountRow{id: transferFromPhone, role: entity.RoleOwner, displayName: "Dueño", isActive: 1},
		accountRow{id: transferToPhone, role: entity.RoleOwner, displayName: "Sucesor", isActive: 0},
	))
	mock.ExpectBegin()
	mock.ExpectQuery(transferStateQuery).WithArgs(transferFromPhone).
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 1))
	mock.ExpectQuery(transferStateQuery).WithArgs(transferToPhone).
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 0))
	mock.ExpectExec(transferUpdateActiveQuery).WithArgs(0, transferFromPhone).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(transferUpdateActiveQuery).WithArgs(1, transferToPhone).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	outcome, err := TransferOwnership(context.Background(), accounts, transferFromPhone, transferToPhone)
	if err != nil {
		t.Fatalf("TransferOwnership() error = %v", err)
	}
	if outcome.From.ID != transferFromPhone || outcome.To.ID != transferToPhone {
		t.Errorf("TransferOwnership() outcome = %+v, want from %q to %q", outcome, transferFromPhone, transferToPhone)
	}
	if !outcome.To.Active || outcome.From.Active {
		t.Errorf("TransferOwnership() outcome = %+v, want the successor active and the old owner inactive", outcome)
	}
}

func TestTransferOwnership_FromIsNotTheActiveOwner(t *testing.T) {
	fake := &fakeAccountsTransfer{listRows: []*entity.Account{
		accountFixture(transferFromPhone, entity.RoleOwner, true),
		accountFixture(transferToPhone, entity.RoleOwner, false),
	}}

	_, err := TransferOwnership(context.Background(), fake, transferNewPhone, transferToPhone)
	if err == nil {
		t.Fatal("TransferOwnership() error = nil, want the wrong-from rejection")
	}
	if !errors.Is(err, ErrNoActiveOwner) {
		t.Errorf("TransferOwnership() error = %v, want errors.Is(ErrNoActiveOwner)", err)
	}
	if fake.transferCalls != 0 {
		t.Errorf("TransferOwnership() called the repo %d times, want 0 for a wrong fromID", fake.transferCalls)
	}
}

func TestTransferOwnership_ToIsNotAnInactiveOwner(t *testing.T) {
	tests := []struct {
		name string
		rows []*entity.Account
		to   string
	}{
		{
			name: "successor already active",
			rows: []*entity.Account{
				accountFixture(transferFromPhone, entity.RoleOwner, true),
				accountFixture(transferToPhone, entity.RoleOwner, true),
			},
			to: transferToPhone,
		},
		{
			name: "successor is not an owner",
			rows: []*entity.Account{
				accountFixture(transferFromPhone, entity.RoleOwner, true),
				accountFixture(transferToPhone, entity.RoleStaff, true),
			},
			to: transferToPhone,
		},
		{
			name: "successor does not exist",
			rows: []*entity.Account{
				accountFixture(transferFromPhone, entity.RoleOwner, true),
			},
			to: transferToPhone,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeAccountsTransfer{listRows: tt.rows}

			_, err := TransferOwnership(context.Background(), fake, transferFromPhone, tt.to)
			if err == nil {
				t.Fatal("TransferOwnership() error = nil, want the invalid-successor rejection")
			}

			var semErr *domain.SemanticError
			if !errors.As(err, &semErr) {
				t.Fatalf("TransferOwnership() error = %T, want *domain.SemanticError", err)
			}
			if fake.transferCalls != 0 {
				t.Errorf("TransferOwnership() called the repo %d times, want 0 for an invalid successor", fake.transferCalls)
			}
		})
	}
}

func TestTransferOwnership_RepoFailureIsWrapped(t *testing.T) {
	fake := &fakeAccountsTransfer{
		listRows: []*entity.Account{
			accountFixture(transferFromPhone, entity.RoleOwner, true),
			accountFixture(transferToPhone, entity.RoleOwner, false),
		},
		transferErr: errors.New("disk I/O error"),
	}

	_, err := TransferOwnership(context.Background(), fake, transferFromPhone, transferToPhone)
	if err == nil {
		t.Fatal("TransferOwnership() error = nil, want the write failure")
	}
	if !strings.Contains(err.Error(), "transferir la propiedad") {
		t.Errorf("TransferOwnership() error = %v, want it to wrap the swap", err)
	}
	if errors.Is(err, ErrNoActiveOwner) {
		t.Errorf("TransferOwnership() error = %v, want no business classification for an I/O failure", err)
	}
}

func TestTransferOwnership_FabricatesOwnerCallerForThePort(t *testing.T) {
	fake := &fakeAccountsTransfer{listRows: []*entity.Account{
		accountFixture(transferFromPhone, entity.RoleOwner, true),
		accountFixture(transferToPhone, entity.RoleOwner, false),
	}}

	if _, err := TransferOwnership(context.Background(), fake, transferFromPhone, transferToPhone); err != nil {
		t.Fatalf("TransferOwnership() error = %v", err)
	}

	want := auth.Caller{ID: TUICallerID, Role: auth.RoleOwner}
	if !fake.listHas || fake.listCaller != want {
		t.Errorf("List caller = %+v (attached=%v), want %+v", fake.listCaller, fake.listHas, want)
	}
	if !fake.transferHas || fake.transferCaller != want {
		t.Errorf("TransferOwnership caller = %+v (attached=%v), want %+v", fake.transferCaller, fake.transferHas, want)
	}
	if fake.transferFrom != transferFromPhone || fake.transferTo != transferToPhone {
		t.Errorf("TransferOwnership(%q, %q), want (%q, %q)", fake.transferFrom, fake.transferTo, transferFromPhone, transferToPhone)
	}
}
