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
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
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

// --- Create ---

func TestAccountsRepo_Create_Admin_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)
	ctx := adminCtx()

	mock.ExpectExec(
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
	).WithArgs("+5491100000000", "admin", "Juan", nil, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(ctx, &entity.Account{
		ID:          "+5491100000000",
		Role:        entity.RoleAdmin,
		DisplayName: "Juan",
		Active:      true,
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
	ctx := adminCtx()

	mock.ExpectQuery(`SELECT COUNT(*) FROM accounts WHERE role = 'owner' AND is_active = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectExec(
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
	).WithArgs("+5491100000000", "owner", "Dueño", nil, 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(ctx, &entity.Account{
		ID:          "+5491100000000",
		Role:        entity.RoleOwner,
		DisplayName: "Dueño",
		Active:      true,
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
	if _, ok := attrs["actor_id"]; !ok {
		t.Error("expected actor_id attribute when ctx has admin caller")
	}
}

func TestAccountsRepo_Create_Staff_WithProfessionalID_Success(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)
	ctx := adminCtx()

	mock.ExpectExec(
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
	).WithArgs("+5491100002222", "staff", "Ana", "p-001", 1).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.Create(ctx, &entity.Account{
		ID:             "+5491100002222",
		Role:           entity.RoleStaff,
		DisplayName:    "Ana",
		ProfessionalID: ptr("p-001"),
		Active:         true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestAccountsRepo_Create_NoAuth_ErrUnauthenticated(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Create(context.Background(), &entity.Account{
		ID:   "+5491100000000",
		Role: entity.RoleAdmin,
	})
	if err == nil {
		t.Fatal("expected error when no auth context")
	}
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expected domain.ErrUnauthenticated, got %v", err)
	}
}

func TestAccountsRepo_Create_EmptyID_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Create(adminCtx(), &entity.Account{
		ID:   "",
		Role: entity.RoleAdmin,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_Create_InvalidRole_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Create(adminCtx(), &entity.Account{
		ID:   "+5491100000000",
		Role: entity.AccountRole("manager"),
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_Create_StaffWithoutProfessionalID_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Create(adminCtx(), &entity.Account{
		ID:   "+5491100002222",
		Role: entity.RoleStaff,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_Create_SecondOwner_ErrConflict_WithWarnAudit(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)
	ctx := adminCtx()

	mock.ExpectQuery(`SELECT COUNT(*) FROM accounts WHERE role = 'owner' AND is_active = 1`).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	err := repo.Create(ctx, &entity.Account{
		ID:          "+5491100009999",
		Role:        entity.RoleOwner,
		DisplayName: "Otro",
		Active:      true,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected domain.ErrConflict, got %v", err)
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

	err := repo.Create(adminCtx(), &entity.Account{
		ID:          "+5491100000000",
		Role:        entity.RoleAdmin,
		DisplayName: "Dup",
		Active:      true,
	})
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected domain.ErrConflict, got %v", err)
	}
}

// TestAccountsRepo_Create_DuplicateID_PrimaryKey_ErrConflict_RealSQLite covers the
// operator-facing path end to end: accounts.id is TEXT PRIMARY KEY, so a duplicate
// insert raises SQLITE_CONSTRAINT_PRIMARYKEY (1555) on a real database, which must
// surface as domain.ErrConflict with the semantic message — never as raw driver text.
func TestAccountsRepo_Create_DuplicateID_PrimaryKey_ErrConflict_RealSQLite(t *testing.T) {
	// Real tmp-file SQLite in WAL mode running the production schema (tmp WAL file,
	// not :memory:, so the driver reports the same extended result codes as
	// production). Helper defined in bookings_aggregate_test.go.
	db, cleanup := newAggregateTestDB(t)
	defer cleanup()

	logger, _ := newTestLogger()
	repo := NewAccountsRepo(db, logger)

	first := &entity.Account{ID: "+5491100001111", Role: entity.RoleAdmin, DisplayName: "First", Active: true}
	if err := repo.Create(adminCtx(), first); err != nil {
		t.Fatalf("first Create: unexpected error: %v", err)
	}

	dup := &entity.Account{ID: "+5491100001111", Role: entity.RoleAdmin, DisplayName: "Dup", Active: true}
	err := repo.Create(adminCtx(), dup)
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected domain.ErrConflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "ya existe una cuenta con id") {
		t.Errorf("error should carry the semantic conflict message, got %v", err)
	}
}

func TestAccountsRepo_Create_DBError_Wrapped(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectExec(
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
	).WithArgs("+5491100000000", "admin", "X", nil, 1).
		WillReturnError(errors.New("connection lost"))

	err := repo.Create(adminCtx(), &entity.Account{
		ID: "+5491100000000", Role: entity.RoleAdmin, DisplayName: "X", Active: true,
	})
	if err == nil || errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected wrapped DB error (not domain.ErrConflict), got %v", err)
	}
	if !strings.Contains(err.Error(), "crear cuenta") {
		t.Errorf("error should mention 'crear cuenta' context, got %v", err)
	}
}

// --- FindByID ---

func TestAccountsRepo_FindByID_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	row := sqlmock.NewRows([]string{"id", "role", "display_name", "professional_id", "is_active", "created_at", "updated_at"}).
		AddRow("+5491100000000", "admin", "Juan", nil, 1, "2026-07-01T10:00:00.000Z", "2026-07-01T10:00:00.000Z")
	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE id = ?`,
	).WithArgs("+5491100000000").WillReturnRows(row)

	a, err := repo.FindByID(adminCtx(), "+5491100000000")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if a.ID != "+5491100000000" || a.Role != entity.RoleAdmin || !a.Active {
		t.Errorf("unexpected account: %+v", a)
	}
	if len(handler.records) != 0 {
		t.Errorf("read method should not emit audit logs, got %d records", len(handler.records))
	}
}

func TestAccountsRepo_FindByID_NotFound_ErrNotFound(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE id = ?`,
	).WithArgs("nope").WillReturnError(sql.ErrNoRows)

	_, err := repo.FindByID(adminCtx(), "nope")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
}

func TestAccountsRepo_FindByID_NoAuth_ErrUnauthenticated(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	_, err := repo.FindByID(context.Background(), "+5491100000000")
	if err == nil {
		t.Fatal("expected error when no auth context")
	}
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expected domain.ErrUnauthenticated, got %v", err)
	}
}

func TestAccountsRepo_FindByID_DBError_Wrapped(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectQuery(
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE id = ?`,
	).WithArgs("+5491100000000").WillReturnError(errors.New("connection lost"))

	_, err := repo.FindByID(adminCtx(), "+5491100000000")
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

	list, err := repo.GetByRole(adminCtx(), entity.RoleAdmin)
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

	list, err := repo.GetByRole(adminCtx(), entity.RoleStaff)
	if err != nil {
		t.Fatalf("GetByRole: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("expected empty non-nil slice, got %v", list)
	}
}

func TestAccountsRepo_GetByRole_InvalidRole_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	_, err := repo.GetByRole(adminCtx(), entity.AccountRole("client"))
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput for role=client, got %v", err)
	}
}

func TestAccountsRepo_GetByRole_NoAuth_ErrUnauthenticated(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	_, err := repo.GetByRole(context.Background(), entity.RoleAdmin)
	if err == nil {
		t.Fatal("expected error when no auth context")
	}
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expected domain.ErrUnauthenticated, got %v", err)
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

	list, err := repo.List(adminCtx())
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

	list, err := repo.List(adminCtx())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("expected empty non-nil slice, got %v", list)
	}
}

func TestAccountsRepo_List_NoAuth_ErrUnauthenticated(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	_, err := repo.List(context.Background())
	if err == nil {
		t.Fatal("expected error when no auth context")
	}
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expected domain.ErrUnauthenticated, got %v", err)
	}
}

// --- Update ---

func TestAccountsRepo_Update_Admin_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)
	ctx := adminCtx()

	mock.ExpectQuery(`SELECT 1 FROM accounts WHERE id = ?`).
		WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))
	mock.ExpectExec(
		`UPDATE accounts SET role = ?, display_name = ?, professional_id = ?, is_active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
	).WithArgs("admin", "Juan Updated", nil, 1, "+5491100000000").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err := repo.Update(ctx, &entity.Account{
		ID:          "+5491100000000",
		Role:        entity.RoleAdmin,
		DisplayName: "Juan Updated",
		Active:      true,
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

	err := repo.Update(adminCtx(), &entity.Account{
		ID:          "nope",
		Role:        entity.RoleAdmin,
		DisplayName: "X",
		Active:      true,
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
}

func TestAccountsRepo_Update_NoAuth_ErrUnauthenticated(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Update(context.Background(), &entity.Account{
		ID:   "+5491100000000",
		Role: entity.RoleAdmin,
	})
	if err == nil {
		t.Fatal("expected error when no auth context")
	}
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expected domain.ErrUnauthenticated, got %v", err)
	}
}

func TestAccountsRepo_Update_InvalidRole_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Update(adminCtx(), &entity.Account{
		ID: "+5491100000000", Role: entity.AccountRole("client"), Active: true,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_Update_StaffWithoutProfessionalID_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Update(adminCtx(), &entity.Account{
		ID: "+5491100002222", Role: entity.RoleStaff, Active: true,
	})
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput, got %v", err)
	}
}

// --- Deactivate ---

func TestAccountsRepo_Deactivate_ActiveToInactive_Success(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)
	ctx := adminCtx()

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

	err := repo.Deactivate(adminCtx(), "+5491100000000")
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

	err := repo.Deactivate(adminCtx(), "nope")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
}

func TestAccountsRepo_Deactivate_EmptyID_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Deactivate(adminCtx(), "")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_Deactivate_NoAuth_ErrUnauthenticated(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	err := repo.Deactivate(context.Background(), "+5491100000000")
	if err == nil {
		t.Fatal("expected error when no auth context")
	}
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expected domain.ErrUnauthenticated, got %v", err)
	}
}

// --- IsActive ---

func TestAccountsRepo_IsActive_True(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	mock.ExpectQuery(`SELECT is_active FROM accounts WHERE id = ?`).
		WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"is_active"}).AddRow(1))

	active, err := repo.IsActive(adminCtx(), "+5491100000000")
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

	active, err := repo.IsActive(adminCtx(), "+5491100000000")
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

	active, err := repo.IsActive(adminCtx(), "nope")
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

func TestAccountsRepo_IsActive_NoAuth_ErrUnauthenticated(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	_, err := repo.IsActive(context.Background(), "acc-1")
	if err == nil {
		t.Fatal("expected error when no auth context")
	}
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expected domain.ErrUnauthenticated, got %v", err)
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

	list, err := repo.ListByProfessional(adminCtx(), "p-001")
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

	list, err := repo.ListByProfessional(adminCtx(), "p-999")
	if err != nil {
		t.Fatalf("ListByProfessional: %v", err)
	}
	if list == nil || len(list) != 0 {
		t.Errorf("expected empty non-nil slice, got %v", list)
	}
}

func TestAccountsRepo_ListByProfessional_EmptyProfessionalID_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	_, err := repo.ListByProfessional(adminCtx(), "")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_ListByProfessional_NoAuth_ErrUnauthenticated(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)
	_, err := repo.ListByProfessional(context.Background(), "p-1")
	if err == nil {
		t.Fatal("expected error when no auth context")
	}
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expected domain.ErrUnauthenticated, got %v", err)
	}
}

// ── TransferOwnership (T5) ─────────────────────────────────────────────────

// Exact statement text mirrors internal/repository/accounts.go; the mock uses
// QueryMatcherEqual so a repository statement change fails this suite loudly.
const (
	transferAccountStateQuery = `SELECT role, is_active FROM accounts WHERE id = ?`
	transferUpdateActiveQuery = `UPDATE accounts SET is_active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`
)

// activeOwnerIDs returns the ids of the ACTIVE owner accounts of db.
func activeOwnerIDs(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.QueryContext(context.Background(),
		`SELECT id FROM accounts WHERE role = 'owner' AND is_active = 1 ORDER BY id`)
	if err != nil {
		t.Fatalf("query active owners: %v", err)
	}
	defer rows.Close() //nolint:errcheck // test helper

	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatalf("scan active owner: %v", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate active owners: %v", err)
	}
	return ids
}

// accountIsActive reads one account's is_active flag directly from db.
func accountIsActive(t *testing.T, db *sql.DB, id string) bool {
	t.Helper()
	var isActive int
	if err := db.QueryRowContext(context.Background(),
		`SELECT is_active FROM accounts WHERE id = ?`, id).Scan(&isActive); err != nil {
		t.Fatalf("read is_active of %q: %v", id, err)
	}
	return isActive == 1
}

func TestAccountsRepo_TransferOwnership_AdminRole_ErrForbidden(t *testing.T) {
	repo, _, _ := newRepoWithMock(t) // no expectations: the gate must reject before any access

	err := repo.TransferOwnership(adminCtx(), "+5491100000000", "+5491100000001")
	if !errors.Is(err, domain.ErrForbidden) {
		t.Fatalf("expected domain.ErrForbidden, got %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_NoCaller_ErrUnauthenticated(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)

	err := repo.TransferOwnership(context.Background(), "+5491100000000", "+5491100000001")
	if !errors.Is(err, domain.ErrUnauthenticated) {
		t.Fatalf("expected domain.ErrUnauthenticated, got %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_BlankIDs_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)

	err := repo.TransferOwnership(ownerCtx(), "", "+5491100000001")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput for a blank fromID, got %v", err)
	}

	err = repo.TransferOwnership(ownerCtx(), "+5491100000000", "")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput for a blank toID, got %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_SameAccount_ErrInvalidInput(t *testing.T) {
	repo, _, _ := newRepoWithMock(t)

	err := repo.TransferOwnership(ownerCtx(), "+5491100000000", "+5491100000000")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput for a self-transfer, got %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_FromMissing_ErrNotFound(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000009").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}))
	mock.ExpectRollback()

	err := repo.TransferOwnership(ownerCtx(), "+5491100000009", "+5491100000001")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_FromNotActiveOwner_ErrConflict(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 0))
	mock.ExpectRollback()

	err := repo.TransferOwnership(ownerCtx(), "+5491100000000", "+5491100000001")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected domain.ErrConflict, got %v", err)
	}
	if !strings.Contains(err.Error(), "no es un owner activo") {
		t.Errorf("error should name the missing active owner, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_FromWrongRole_ErrInvalidInput(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000002").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("staff", 1))
	mock.ExpectRollback()

	err := repo.TransferOwnership(ownerCtx(), "+5491100000002", "+5491100000001")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput, got %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_ToMissing_ErrNotFound(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 1))
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000009").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}))
	mock.ExpectRollback()

	err := repo.TransferOwnership(ownerCtx(), "+5491100000000", "+5491100000009")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected domain.ErrNotFound, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_ToNotOwnerRole_ErrInvalidInput(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 1))
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000002").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("staff", 0))
	mock.ExpectRollback()

	err := repo.TransferOwnership(ownerCtx(), "+5491100000000", "+5491100000002")
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("expected domain.ErrInvalidInput, got %v", err)
	}
	if !strings.Contains(err.Error(), "no tiene rol owner") {
		t.Errorf("error should name the wrong role, got %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_ToAlreadyActive_ErrConflict(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 1))
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000001").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 1))
	mock.ExpectRollback()

	err := repo.TransferOwnership(ownerCtx(), "+5491100000000", "+5491100000001")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected domain.ErrConflict, got %v", err)
	}
}

// TestAccountsRepo_TransferOwnership_Success_DeactivatesThenActivatesInOneTx
// locks the atomic contract AND the statement order: sqlmock is ordered, so the
// deactivate must be executed before the activate (activating first would make
// the single-owner trigger observe two active owners).
func TestAccountsRepo_TransferOwnership_Success_DeactivatesThenActivatesInOneTx(t *testing.T) {
	repo, mock, handler := newRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 1))
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000001").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 0))
	mock.ExpectExec(transferUpdateActiveQuery).WithArgs(0, "+5491100000000").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(transferUpdateActiveQuery).WithArgs(1, "+5491100000001").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	if err := repo.TransferOwnership(ownerCtx(), "+5491100000000", "+5491100000001"); err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("mock expectations: %v", err)
	}

	attrs := handler.recordAttrs("ownership transferred")
	if attrs == nil {
		t.Fatal("expected the 'ownership transferred' audit record")
	}
	if got := attrs["target_id"]; got != "+5491100000001" {
		t.Errorf("target_id: expected the new owner, got %v", got)
	}
	if got := attrs["target_role"]; got != "owner" {
		t.Errorf("target_role: expected 'owner', got %v", got)
	}
	if got := attrs["previous_owner_id"]; got != "+5491100000000" {
		t.Errorf("previous_owner_id: expected the old owner, got %v", got)
	}
	if _, ok := attrs["actor_id"]; !ok {
		t.Error("expected actor_id attribute when ctx has caller")
	}
}

func TestAccountsRepo_TransferOwnership_TriggerViolation_ErrConflict(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 1))
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000001").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 0))
	mock.ExpectExec(transferUpdateActiveQuery).WithArgs(0, "+5491100000000").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(transferUpdateActiveQuery).WithArgs(1, "+5491100000001").
		WillReturnError(errors.New("single-owner invariant: only one active owner allowed"))
	mock.ExpectRollback()

	err := repo.TransferOwnership(ownerCtx(), "+5491100000000", "+5491100000001")
	if !errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected domain.ErrConflict, got %v", err)
	}
	if strings.Contains(err.Error(), "single-owner invariant") {
		t.Errorf("error should be semantic, not a driver dump, got %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_ActivationFailure_RollsBack(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectBegin()
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000000").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 1))
	mock.ExpectQuery(transferAccountStateQuery).WithArgs("+5491100000001").
		WillReturnRows(sqlmock.NewRows([]string{"role", "is_active"}).AddRow("owner", 0))
	mock.ExpectExec(transferUpdateActiveQuery).WithArgs(0, "+5491100000000").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(transferUpdateActiveQuery).WithArgs(1, "+5491100000001").
		WillReturnError(errors.New("disk I/O error"))
	mock.ExpectRollback()

	err := repo.TransferOwnership(ownerCtx(), "+5491100000000", "+5491100000001")
	if err == nil {
		t.Fatal("expected the activation failure")
	}
	if errors.Is(err, domain.ErrConflict) {
		t.Fatalf("expected a wrapped I/O failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "activar al nuevo owner") {
		t.Errorf("error should mention the activation step, got %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("the transaction was not rolled back: %v", err)
	}
}

func TestAccountsRepo_TransferOwnership_BeginTxFailure_Wrapped(t *testing.T) {
	repo, mock, _ := newRepoWithMock(t)

	mock.ExpectBegin().WillReturnError(errors.New("database is locked"))

	err := repo.TransferOwnership(ownerCtx(), "+5491100000000", "+5491100000001")
	if err == nil {
		t.Fatal("expected the begin-transaction failure")
	}
	if !strings.Contains(err.Error(), "iniciar la transacción") {
		t.Errorf("error should mention the transaction start, got %v", err)
	}
}

// TestAccountsRepo_TransferOwnership_RealSQLite_SwapsASingleActiveOwner runs the
// production schema on a real WAL SQLite file: exactly one ACTIVE owner before
// and after, the previous owner soft-deleted (never deleted), and the
// single-owner trigger still rejecting a second ACTIVE owner written by hand.
func TestAccountsRepo_TransferOwnership_RealSQLite_SwapsASingleActiveOwner(t *testing.T) {
	db, cleanup := newAggregateTestDB(t)
	defer cleanup()

	logger, _ := newTestLogger()
	repo := NewAccountsRepo(db, logger)

	from := &entity.Account{ID: "+5491100000000", Role: entity.RoleOwner, DisplayName: "Dueño", Active: true}
	if err := repo.Create(ownerCtx(), from); err != nil {
		t.Fatalf("create current owner: %v", err)
	}
	to := &entity.Account{ID: "+5491100000001", Role: entity.RoleOwner, DisplayName: "Sucesor", Active: false}
	if err := repo.Create(ownerCtx(), to); err != nil {
		t.Fatalf("create prepared successor: %v", err)
	}

	if got := activeOwnerIDs(t, db); len(got) != 1 || got[0] != from.ID {
		t.Fatalf("active owners before the transfer = %v, want [%s]", got, from.ID)
	}

	if err := repo.TransferOwnership(ownerCtx(), from.ID, to.ID); err != nil {
		t.Fatalf("TransferOwnership: %v", err)
	}

	if got := activeOwnerIDs(t, db); len(got) != 1 || got[0] != to.ID {
		t.Fatalf("active owners after the transfer = %v, want exactly [%s]", got, to.ID)
	}
	if accountIsActive(t, db, from.ID) {
		t.Errorf("account %q is still active, want the previous owner deactivated", from.ID)
	}
	if !accountIsActive(t, db, to.ID) {
		t.Errorf("account %q is not active, want the successor activated", to.ID)
	}

	// Defense in depth: a hand-written activation of a third owner must still be
	// rejected by the schema trigger, so the invariant does not depend on Go.
	third := &entity.Account{ID: "+5491100000002", Role: entity.RoleOwner, DisplayName: "Tercero", Active: false}
	if err := repo.Create(ownerCtx(), third); err != nil {
		t.Fatalf("create third owner row: %v", err)
	}
	_, err := db.ExecContext(context.Background(), `UPDATE accounts SET is_active = 1 WHERE id = ?`, third.ID)
	if err == nil {
		t.Fatal("the single-owner trigger allowed a second ACTIVE owner")
	}
	if !strings.Contains(err.Error(), "single-owner invariant") {
		t.Errorf("unexpected trigger error: %v", err)
	}
}

// TestAccountsRepo_TransferOwnership_RealSQLite_RollsBackOnFailure forces the
// activation statement to fail AFTER the deactivation statement already ran
// inside the same transaction. The rollback must leave the original owner
// active: the installation is never ownerless and never has two owners.
//
// The blocking trigger is created with a static literal id on purpose (no SQL
// string building) and lives only in the throwaway test database.
func TestAccountsRepo_TransferOwnership_RealSQLite_RollsBackOnFailure(t *testing.T) {
	const blockedSuccessor = "+5491100000777"

	db, cleanup := newAggregateTestDB(t)
	defer cleanup()

	logger, _ := newTestLogger()
	repo := NewAccountsRepo(db, logger)

	from := &entity.Account{ID: "+5491100000000", Role: entity.RoleOwner, DisplayName: "Dueño", Active: true}
	if err := repo.Create(ownerCtx(), from); err != nil {
		t.Fatalf("create current owner: %v", err)
	}
	blocked := &entity.Account{ID: blockedSuccessor, Role: entity.RoleOwner, DisplayName: "Bloqueado", Active: false}
	if err := repo.Create(ownerCtx(), blocked); err != nil {
		t.Fatalf("create prepared successor: %v", err)
	}

	execSeed(t, db, `CREATE TRIGGER test_abort_activation BEFORE UPDATE ON accounts
		WHEN NEW.id = '+5491100000777' AND NEW.is_active = 1
	BEGIN
		SELECT RAISE(ABORT, 'test-blocked activation');
	END`)

	err := repo.TransferOwnership(ownerCtx(), from.ID, blockedSuccessor)
	if err == nil {
		t.Fatal("expected the forced activation failure")
	}
	if !strings.Contains(err.Error(), "test-blocked activation") {
		t.Fatalf("expected the forced failure after the deactivation ran, got %v", err)
	}
	if errors.Is(err, domain.ErrConflict) {
		t.Errorf("a forced failure is not a business conflict, got %v", err)
	}

	if got := activeOwnerIDs(t, db); len(got) != 1 || got[0] != from.ID {
		t.Fatalf("active owners after the failed transfer = %v, want the original owner [%s] — rollback did not run", got, from.ID)
	}
	if accountIsActive(t, db, blockedSuccessor) {
		t.Errorf("account %q is active, want the failed transfer rolled back", blockedSuccessor)
	}
}
