package repository

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/model"
)

// testHandler is a slog.Handler that captures records for inspection in tests.
type testHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *testHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }

func (h *testHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r.Clone())
	return nil
}

func (h *testHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *testHandler) WithGroup(_ string) slog.Handler      { return h }

// recordsByMsg returns records whose message matches the given string.
func (h *testHandler) recordsByMsg(msg string) []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := []slog.Record{}
	for _, r := range h.records {
		if r.Message == msg {
			out = append(out, r)
		}
	}
	return out
}

// recordAttrs returns the attributes of the first record matching msg, or nil.
func (h *testHandler) recordAttrs(msg string) map[string]any {
	recs := h.recordsByMsg(msg)
	if len(recs) == 0 {
		return nil
	}
	out := make(map[string]any)
	recs[0].Attrs(func(a slog.Attr) bool {
		out[a.Key] = a.Value.Any()
		return true
	})
	return out
}

// newTestLogger returns a logger that writes to a testHandler.
func newTestLogger() (*slog.Logger, *testHandler) {
	h := &testHandler{}
	return slog.New(h), h
}

// newRepoWithMock creates an AccountsRepo backed by go-sqlmock and a test logger.
func newRepoWithMock(t *testing.T) (*AccountsRepo, sqlmock.Sqlmock, *testHandler) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.QueryMatcherOption(sqlmock.QueryMatcherEqual))
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	_, handler := newTestLogger()
	// discard everything below INFO (we only care about audit-relevant levels)
	logger := slog.New(&levelFilter{inner: handler, min: slog.LevelInfo})

	return NewAccountsRepo(db, logger), mock, handler
}

// levelFilter discards records below a minimum level. Used to keep tests fast.
type levelFilter struct {
	inner slog.Handler
	min   slog.Level
}

func (f *levelFilter) Enabled(_ context.Context, l slog.Level) bool { return l >= f.min }
func (f *levelFilter) Handle(ctx context.Context, r slog.Record) error {
	return f.inner.Handle(ctx, r)
}
func (f *levelFilter) WithAttrs(a []slog.Attr) slog.Handler {
	return &levelFilter{inner: f.inner.WithAttrs(a), min: f.min}
}
func (f *levelFilter) WithGroup(g string) slog.Handler {
	return &levelFilter{inner: f.inner.WithGroup(g), min: f.min}
}

// ptr is a small helper to take the address of a literal.
func ptr[T any](v T) *T { return &v }

// withActorContext returns a context carrying an auth.Caller.
func withActorContext(actorID string) context.Context {
	return auth.WithCaller(context.Background(), auth.Caller{ID: actorID, Role: auth.RoleAdmin})
}

// --- Create ---

func TestAccountsRepo_Create_Admin_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)
	ctx := withActorContext("admin-001")

	mock.ExpectExec(
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
	).WithArgs("+5491100000000", "admin", "Juan", nil, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(ctx, &model.Account{
		ID:          "+5491100000000",
		Role:        "admin",
		DisplayName: "Juan",
		IsActive:    true,
	})
	if err != nil {
		t.Fatalf("Create: unexpected error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
	recs := handler.recordsByMsg("account created")
	if len(recs) != 1 {
		t.Fatalf("expected 1 audit record, got %d", len(recs))
	}
	attrs := handler.recordAttrs("account created")
	if attrs == nil {
		t.Fatal("expected 'account created' record")
	}
	if got := attrs["target_id"]; got != "+5491100000000" {
		t.Errorf("target_id: expected '+5491100000000', got %v", got)
	}
	if got := attrs["target_role"]; got != "admin" {
		t.Errorf("target_role: expected 'admin', got %v", got)
	}
	if _, ok := attrs["actor_id"]; !ok {
		t.Error("expected actor_id attribute when ctx has caller")
	}
	if _, ok := attrs["ts"]; !ok {
		t.Error("expected ts attribute")
	}
}

func TestAccountsRepo_Create_Owner_NoExistingOwner_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT COUNT(*) FROM accounts WHERE role = 'owner' AND is_active = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
	).WithArgs("+5491100000000", "owner", "Dueño", nil, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(ctx, &model.Account{
		ID:          "+5491100000000",
		Role:        "owner",
		DisplayName: "Dueño",
		IsActive:    true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
	attrs := handler.recordAttrs("account created")
	if attrs == nil {
		t.Fatal("expected 'account created' record")
	}
	if _, ok := attrs["actor_id"]; ok {
		t.Error("expected NO actor_id attribute when ctx has no caller")
	}
}

func TestAccountsRepo_Create_Staff_WithProfessionalID_Success(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)
	ctx := withActorContext("admin-001")

	mock.ExpectExec(
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
	).WithArgs("+5491100002222", "staff", "Ana", "p-001", 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(ctx, &model.Account{
		ID:             "+5491100002222",
		Role:           "staff",
		DisplayName:    "Ana",
		ProfessionalID: ptr("p-001"),
		IsActive:       true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestAccountsRepo_Create_EmptyID_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Create(context.Background(), &model.Account{
		ID:   "",
		Role: "admin",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_Create_InvalidRole_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Create(context.Background(), &model.Account{
		ID:   "+5491100000000",
		Role: "manager",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_Create_StaffWithoutProfessionalID_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Create(context.Background(), &model.Account{
		ID:   "+5491100002222",
		Role: "staff",
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_Create_SecondOwner_ErrConflict_WithWarnAudit(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)
	ctx := context.Background()

	mock.ExpectQuery(`SELECT COUNT(*) FROM accounts WHERE role = 'owner' AND is_active = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err := repo.Create(ctx, &model.Account{
		ID:          "+5491100009999",
		Role:        "owner",
		DisplayName: "Otro",
		IsActive:    true,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}

	recs := handler.recordsByMsg("second active owner rejected")
	if len(recs) != 1 {
		t.Fatalf("expected 1 warn audit record, got %d", len(recs))
	}
}

func TestAccountsRepo_Create_UniqueViolation_ErrConflict(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectExec(
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
	).WithArgs("+5491100000000", "admin", "Dup", nil, 1).
		WillReturnError(errors.New("UNIQUE constraint failed: accounts.id"))

	err := repo.Create(context.Background(), &model.Account{
		ID:          "+5491100000000",
		Role:        "admin",
		DisplayName: "Dup",
		IsActive:    true,
	})
	if !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}
}

func TestAccountsRepo_Create_DBError_Wrapped(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectExec(
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
	).WithArgs("+5491100000000", "admin", "X", nil, 1).
		WillReturnError(errors.New("connection lost"))

	err := repo.Create(context.Background(), &model.Account{
		ID: "+5491100000000", Role: "admin", DisplayName: "X", IsActive: true,
	})
	if err == nil || errors.Is(err, ErrConflict) {
		t.Fatalf("expected wrapped DB error (not ErrConflict), got %v", err)
	}
	if !strings.Contains(err.Error(), "crear cuenta") {
		t.Errorf("error should mention 'crear cuenta' context, got %v", err)
	}
}

// --- Get ---

func TestAccountsRepo_Get_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	row := sqlmock.NewRows([]string{"id", "role", "display_name", "professional_id", "is_active", "created_at", "updated_at"}).
		AddRow("+5491100000000", "admin", "Juan", nil, 1, "2026-07-01T10:00:00.000Z", "2026-07-01T10:00:00.000Z")
	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE id = ?`,
	).WithArgs("+5491100000000").WillReturnRows(row)

	a, err := repo.Get(context.Background(), "+5491100000000")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if a.ID != "+5491100000000" || a.Role != "admin" || !a.IsActive {
		t.Errorf("unexpected account: %+v", a)
	}
	if len(handler.records) != 0 {
		t.Errorf("read method should not emit audit logs, got %d records", len(handler.records))
	}
}

func TestAccountsRepo_Get_NotFound_ErrNotFound(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE id = ?`,
	).WithArgs("nope").WillReturnError(sql.ErrNoRows)

	_, err := repo.Get(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAccountsRepo_Get_DBError_Wrapped(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE id = ?`,
	).WithArgs("+5491100000000").WillReturnError(errors.New("connection lost"))

	_, err := repo.Get(context.Background(), "+5491100000000")
	if err == nil {
		t.Fatalf("expected error")
	}
	if !strings.Contains(err.Error(), "leer cuenta") {
		t.Errorf("expected 'leer cuenta' context, got %v", err)
	}
}

// --- GetByRole ---

func TestAccountsRepo_GetByRole_Admin_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	rows := sqlmock.NewRows([]string{"id", "role", "display_name", "professional_id", "is_active", "created_at", "updated_at"}).
		AddRow("+5491100000000", "admin", "Juan", nil, 1, "2026-07-01T10:00:00.000Z", "2026-07-01T10:00:00.000Z").
		AddRow("+5491100001111", "admin", "Pedro", nil, 1, "2026-07-02T10:00:00.000Z", "2026-07-02T10:00:00.000Z")
	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE role = ? ORDER BY created_at ASC`,
	).WithArgs("admin").WillReturnRows(rows)

	list, err := repo.GetByRole(context.Background(), "admin")
	if err != nil {
		t.Fatalf("GetByRole: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 admins, got %d", len(list))
	}
	if len(handler.records) != 0 {
		t.Errorf("read method should not emit audit logs, got %d records", len(handler.records))
	}
}

func TestAccountsRepo_GetByRole_NoMatch_EmptySlice(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE role = ? ORDER BY created_at ASC`,
	).WithArgs("staff").WillReturnRows(sqlmock.NewRows([]string{"id", "role", "display_name", "professional_id", "is_active", "created_at", "updated_at"}))

	list, err := repo.GetByRole(context.Background(), "staff")
	if err != nil {
		t.Fatalf("GetByRole: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("expected empty non-nil slice, got %v", list)
	}
}

func TestAccountsRepo_GetByRole_InvalidRole_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	_, err := repo.GetByRole(context.Background(), "client")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput for role=client, got %v", err)
	}
}

// --- List ---

func TestAccountsRepo_List_All_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	rows := sqlmock.NewRows([]string{"id", "role", "display_name", "professional_id", "is_active", "created_at", "updated_at"}).
		AddRow("+5491100000000", "owner", "Dueño", nil, 1, "2026-07-01T10:00:00.000Z", "2026-07-01T10:00:00.000Z").
		AddRow("+5491100001111", "admin", "Juan", nil, 1, "2026-07-02T10:00:00.000Z", "2026-07-02T10:00:00.000Z")
	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts ORDER BY created_at ASC`,
	).WillReturnRows(rows)

	list, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(list))
	}
	if len(handler.records) != 0 {
		t.Errorf("read method should not emit audit logs, got %d records", len(handler.records))
	}
}

func TestAccountsRepo_List_Empty_EmptySlice(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts ORDER BY created_at ASC`,
	).WillReturnRows(sqlmock.NewRows([]string{"id", "role", "display_name", "professional_id", "is_active", "created_at", "updated_at"}))

	list, err := repo.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("expected empty non-nil slice, got %v", list)
	}
}

// --- Update ---

func TestAccountsRepo_Update_Admin_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)
	ctx := withActorContext("admin-001")

	mock.ExpectQuery(`SELECT 1 FROM accounts WHERE id = ?`).
		WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectExec(
		`UPDATE accounts SET role = ?, display_name = ?, professional_id = ?, is_active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
	).WithArgs("admin", "Juan Updated", nil, 1, "+5491100000000").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Update(ctx, &model.Account{
		ID:          "+5491100000000",
		Role:        "admin",
		DisplayName: "Juan Updated",
		IsActive:    true,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if recs := handler.recordsByMsg("account updated"); len(recs) != 1 {
		t.Errorf("expected 1 update audit record, got %d", len(recs))
	}
	attrs := handler.recordAttrs("account updated")
	if attrs == nil {
		t.Fatal("expected 'account updated' record")
	}
	if got := attrs["target_id"]; got != "+5491100000000" {
		t.Errorf("target_id: expected '+5491100000000', got %v", got)
	}
	if got := attrs["target_role"]; got != "admin" {
		t.Errorf("target_role: expected 'admin', got %v", got)
	}
	if _, ok := attrs["actor_id"]; !ok {
		t.Error("expected actor_id attribute when ctx has caller")
	}
	if _, ok := attrs["ts"]; !ok {
		t.Error("expected ts attribute")
	}
}

func TestAccountsRepo_Update_NotFound_ErrNotFound(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectQuery(`SELECT 1 FROM accounts WHERE id = ?`).
		WithArgs("nope").WillReturnError(sql.ErrNoRows)

	err := repo.Update(context.Background(), &model.Account{
		ID:          "nope",
		Role:        "admin",
		DisplayName: "X",
		IsActive:    true,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAccountsRepo_Update_InvalidRole_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Update(context.Background(), &model.Account{
		ID: "+5491100000000", Role: "client", IsActive: true,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_Update_StaffWithoutProfessionalID_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Update(context.Background(), &model.Account{
		ID: "+5491100002222", Role: "staff", IsActive: true,
	})
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

// --- Deactivate ---

func TestAccountsRepo_Deactivate_ActiveToInactive_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)
	ctx := withActorContext("admin-001")

	mock.ExpectQuery(`SELECT is_active, role FROM accounts WHERE id = ?`).
		WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"is_active", "role"}).AddRow(1, "admin"))
	mock.ExpectExec(
		`UPDATE accounts SET is_active = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
	).WithArgs("+5491100000000").WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Deactivate(ctx, "+5491100000000")
	if err != nil {
		t.Fatalf("Deactivate: %v", err)
	}
	if recs := handler.recordsByMsg("account deactivated"); len(recs) != 1 {
		t.Errorf("expected 1 deactivate audit record, got %d", len(recs))
	}
	attrs := handler.recordAttrs("account deactivated")
	if attrs == nil {
		t.Fatal("expected 'account deactivated' record")
	}
	if got := attrs["target_id"]; got != "+5491100000000" {
		t.Errorf("target_id: expected '+5491100000000', got %v", got)
	}
	if got := attrs["target_role"]; got != "admin" {
		t.Errorf("target_role: expected 'admin', got %v", got)
	}
	if _, ok := attrs["actor_id"]; !ok {
		t.Error("expected actor_id attribute when ctx has caller")
	}
	if _, ok := attrs["ts"]; !ok {
		t.Error("expected ts attribute")
	}
}

func TestAccountsRepo_Deactivate_AlreadyInactive_NoOp_NoAudit(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	mock.ExpectQuery(`SELECT is_active, role FROM accounts WHERE id = ?`).
		WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"is_active", "role"}).AddRow(0, "admin"))

	err := repo.Deactivate(context.Background(), "+5491100000000")
	if err != nil {
		t.Fatalf("Deactivate (already inactive): %v", err)
	}
	if recs := handler.recordsByMsg("account deactivated"); len(recs) != 0 {
		t.Errorf("expected NO audit record for no-op, got %d", len(recs))
	}
}

func TestAccountsRepo_Deactivate_NotFound_ErrNotFound(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectQuery(`SELECT is_active, role FROM accounts WHERE id = ?`).
		WithArgs("nope").WillReturnError(sql.ErrNoRows)

	err := repo.Deactivate(context.Background(), "nope")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestAccountsRepo_Deactivate_EmptyID_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Deactivate(context.Background(), "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}

// --- IsActive ---

func TestAccountsRepo_IsActive_True(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	mock.ExpectQuery(`SELECT is_active FROM accounts WHERE id = ?`).
		WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(1))

	active, err := repo.IsActive(context.Background(), "+5491100000000")
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if !active {
		t.Error("expected IsActive=true")
	}
	if len(handler.records) != 0 {
		t.Errorf("read method should not emit audit logs, got %d records", len(handler.records))
	}
}

func TestAccountsRepo_IsActive_False_ExistingRow(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	mock.ExpectQuery(`SELECT is_active FROM accounts WHERE id = ?`).
		WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(0))

	active, err := repo.IsActive(context.Background(), "+5491100000000")
	if err != nil {
		t.Fatalf("IsActive: %v", err)
	}
	if active {
		t.Error("expected IsActive=false")
	}
	if len(handler.records) != 0 {
		t.Errorf("read method should not emit audit logs, got %d records", len(handler.records))
	}
}

func TestAccountsRepo_IsActive_MissingRow_ReturnsFalseNoError(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	mock.ExpectQuery(`SELECT is_active FROM accounts WHERE id = ?`).
		WithArgs("nope").WillReturnError(sql.ErrNoRows)

	active, err := repo.IsActive(context.Background(), "nope")
	if err != nil {
		t.Fatalf("IsActive on missing row: unexpected error: %v", err)
	}
	if active {
		t.Error("expected IsActive=false for missing row")
	}
	if len(handler.records) != 0 {
		t.Errorf("read method should not emit audit logs, got %d records", len(handler.records))
	}
}

// --- ListByProfessional ---

func TestAccountsRepo_ListByProfessional_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	rows := sqlmock.NewRows([]string{"id", "role", "display_name", "professional_id", "is_active", "created_at", "updated_at"}).
		AddRow("+5491100002222", "staff", "Ana", "p-001", 1, "2026-07-01T10:00:00.000Z", "2026-07-01T10:00:00.000Z").
		AddRow("+5491100003333", "staff", "Beto", "p-001", 1, "2026-07-02T10:00:00.000Z", "2026-07-02T10:00:00.000Z")
	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE role = 'staff' AND professional_id = ? ORDER BY display_name ASC`,
	).WithArgs("p-001").WillReturnRows(rows)

	list, err := repo.ListByProfessional(context.Background(), "p-001")
	if err != nil {
		t.Fatalf("ListByProfessional: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 staff for p-001, got %d", len(list))
	}
	if len(handler.records) != 0 {
		t.Errorf("read method should not emit audit logs, got %d records", len(handler.records))
	}
}

func TestAccountsRepo_ListByProfessional_NoMatch_EmptySlice(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE role = 'staff' AND professional_id = ? ORDER BY display_name ASC`,
	).WithArgs("p-999").WillReturnRows(sqlmock.NewRows([]string{"id", "role", "display_name", "professional_id", "is_active", "created_at", "updated_at"}))

	list, err := repo.ListByProfessional(context.Background(), "p-999")
	if err != nil {
		t.Fatalf("ListByProfessional: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("expected empty non-nil slice, got %v", list)
	}
}

func TestAccountsRepo_ListByProfessional_EmptyProfessionalID_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	_, err := repo.ListByProfessional(context.Background(), "")
	if !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected ErrInvalidInput, got %v", err)
	}
}
