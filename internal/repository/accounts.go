package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
)

// Compile-time interface conformance check.
var _ domainrepo.AccountsRepo = (*AccountsRepo)(nil)

// AccountsRepo provides CRUD operations for the accounts table.
// All mutations emit structured audit logs via the injected *slog.Logger.
type AccountsRepo struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewAccountsRepo creates a repo with an already-open *sql.DB and configured *slog.Logger.
// It does NOT open connections or run migrations.
func NewAccountsRepo(db *sql.DB, logger *slog.Logger) *AccountsRepo {
	return &AccountsRepo{db: db, logger: logger}
}

// validRole checks if the role is one of the three allowed values.
func validRole(role entity.AccountRole) bool {
	return role == entity.RoleOwner || role == entity.RoleAdmin || role == entity.RoleStaff
}

// actorFromContext extracts the actor ID from the context's auth.Caller.
// Returns empty string when no Caller is present (omitted from audit log).
func actorFromContext(ctx context.Context) string {
	if caller, ok := auth.FromContext(ctx); ok {
		return caller.ID
	}
	return ""
}

// auditAttrs builds the common audit log attributes, omitting actor_id when empty.
func auditAttrs(actorID, targetID, targetRole string) []any {
	attrs := make([]any, 0, 7)
	if actorID != "" {
		attrs = append(attrs, "actor_id", actorID)
	}
	attrs = append(attrs, "target_id", targetID)
	attrs = append(attrs, "target_role", targetRole)
	attrs = append(attrs, "ts", time.Now().UTC().Format(time.RFC3339Nano))
	return attrs
}

// Create inserts a new account. Validates before touching the DB.
// For role "owner", performs a single-owner pre-check (defense-in-depth with the SQLite trigger).
// Requires admin or owner role.
func (r *AccountsRepo) Create(ctx context.Context, a *entity.Account) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("crear cuenta: %w", err)
	}

	if err := a.Validate(); err != nil {
		return fmt.Errorf("crear cuenta: %w", err)
	}

	// Single-owner pre-check (defense-in-depth with SQLite trigger)
	if a.Role == entity.RoleOwner && a.Active {
		var count int
		if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM accounts WHERE role = 'owner' AND is_active = 1").Scan(&count); err != nil {
			return fmt.Errorf("verificar owner activo: %w", err)
		}
		if count > 0 {
			// Emit security audit log for the rejection attempt
			attrs := auditAttrs(actorFromContext(ctx), a.ID, string(a.Role))
			attrs = append(attrs, "result", "rejected")
			r.logger.Warn("second active owner rejected", attrs...)
			return fmt.Errorf("crear cuenta: %w: ya existe un owner activo; desactívalo antes de crear otro", domain.ErrConflict)
		}
	}

	isActive := 0
	if a.Active {
		isActive = 1
	}

	var profID *string
	if a.ProfessionalID != nil && *a.ProfessionalID != "" {
		profID = a.ProfessionalID
	}

	_, err := r.db.ExecContext(ctx,
		`INSERT INTO accounts (id, role, display_name, professional_id, is_active) VALUES (?, ?, ?, ?, ?)`,
		a.ID, string(a.Role), a.DisplayName, profID, isActive,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("crear cuenta: %w: ya existe una cuenta con id %q", domain.ErrConflict, a.ID)
		}
		if isSingleOwnerViolation(err) {
			return fmt.Errorf("crear cuenta: %w: ya existe un owner activo; desactívalo antes de crear otro", domain.ErrConflict)
		}
		return fmt.Errorf("crear cuenta: %w", err)
	}

	r.logger.Info("account created", auditAttrs(actorFromContext(ctx), a.ID, string(a.Role))...)
	return nil
}

// FindByID retrieves a single account by ID. Returns domain.ErrNotFound if the row does not exist.
// Requires an authenticated caller.
func (r *AccountsRepo) FindByID(ctx context.Context, id string) (*entity.Account, error) {
	if _, err := auth.RequireCaller(ctx); err != nil {
		return nil, fmt.Errorf("obtener cuenta %s: %w", id, err)
	}

	row := r.db.QueryRowContext(ctx,
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE id = ?`, id,
	)
	return scanAccount(row)
}

// GetByRole returns all accounts matching the given role, ordered by created_at ASC.
// Returns domain.ErrInvalidInput for unrecognized roles. Returns empty slice (not nil) when no rows match.
// Requires an authenticated caller.
func (r *AccountsRepo) GetByRole(ctx context.Context, role entity.AccountRole) ([]*entity.Account, error) {
	if _, err := auth.RequireCaller(ctx); err != nil {
		return nil, fmt.Errorf("buscar cuentas por role: %w", err)
	}

	if !validRole(role) {
		return nil, fmt.Errorf("buscar por role: %w: role %q no válido", domain.ErrInvalidInput, role)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE role = ? ORDER BY created_at ASC`, string(role),
	)
	if err != nil {
		return nil, fmt.Errorf("buscar cuentas por role: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	accounts := make([]*entity.Account, 0)
	for rows.Next() {
		a, err := scanAccountRow(rows)
		if err != nil {
			return nil, fmt.Errorf("buscar cuentas por role: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("buscar cuentas por role: %w", err)
	}
	return accounts, nil
}

// List returns all accounts ordered by created_at ASC. Returns empty slice (not nil) when no rows exist.
// Requires an authenticated caller.
func (r *AccountsRepo) List(ctx context.Context) ([]*entity.Account, error) {
	if _, err := auth.RequireCaller(ctx); err != nil {
		return nil, fmt.Errorf("listar cuentas: %w", err)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listar cuentas: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	accounts := make([]*entity.Account, 0)
	for rows.Next() {
		a, err := scanAccountRow(rows)
		if err != nil {
			return nil, fmt.Errorf("listar cuentas: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listar cuentas: %w", err)
	}
	return accounts, nil
}

// Update modifies an existing account. Returns domain.ErrNotFound if the row does not exist.
// Regenerates updated_at with SQLite strftime.
// Requires admin or owner role.
func (r *AccountsRepo) Update(ctx context.Context, a *entity.Account) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("actualizar cuenta: %w", err)
	}

	if err := a.Validate(); err != nil {
		return fmt.Errorf("actualizar cuenta: %w", err)
	}

	// Verify the row exists before UPDATE so RowsAffected() == 0 unambiguously means
	// "not found" (not "no-op update with same values").
	var exists int
	if err := r.db.QueryRowContext(ctx, `SELECT 1 FROM accounts WHERE id = ?`, a.ID).Scan(&exists); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("actualizar cuenta: %w: cuenta con id %q no encontrada", domain.ErrNotFound, a.ID)
		}
		return fmt.Errorf("actualizar cuenta: verificar existencia: %w", err)
	}

	isActive := 0
	if a.Active {
		isActive = 1
	}

	var profID *string
	if a.ProfessionalID != nil && *a.ProfessionalID != "" {
		profID = a.ProfessionalID
	}

	result, err := r.db.ExecContext(ctx,
		`UPDATE accounts SET role = ?, display_name = ?, professional_id = ?, is_active = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`,
		string(a.Role), a.DisplayName, profID, isActive, a.ID,
	)
	if err != nil {
		if isSingleOwnerViolation(err) {
			return fmt.Errorf("actualizar cuenta: %w: ya existe un owner activo; desactívalo antes de crear otro", domain.ErrConflict)
		}
		return fmt.Errorf("actualizar cuenta: %w", err)
	}

	// RowsAffected == 0 after a confirmed-existing row means no-op update (same values). Not an error.
	_ = result

	r.logger.Info("account updated", auditAttrs(actorFromContext(ctx), a.ID, string(a.Role))...)
	return nil
}

// Deactivate soft-deletes an account by setting is_active=0. Idempotent: second call is no-op.
// Returns domain.ErrNotFound if the account does not exist. Returns nil if already deactivated.
// Requires admin or owner role.
func (r *AccountsRepo) Deactivate(ctx context.Context, id string) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("desactivar cuenta: %w", err)
	}

	if id == "" {
		return fmt.Errorf("desactivar cuenta: %w: el id no puede estar vacío", domain.ErrInvalidInput)
	}

	// Check current state to get role for audit log and handle idempotency
	var isActive int
	var role string
	err := r.db.QueryRowContext(ctx,
		`SELECT is_active, role FROM accounts WHERE id = ?`, id,
	).Scan(&isActive, &role)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("desactivar cuenta: %w: cuenta con id %q no encontrada", domain.ErrNotFound, id)
		}
		return fmt.Errorf("desactivar cuenta: %w", err)
	}

	// Idempotent: already deactivated → no-op, no audit log
	if isActive == 0 {
		return nil
	}

	_, err = r.db.ExecContext(ctx,
		`UPDATE accounts SET is_active = 0, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now') WHERE id = ?`, id,
	)
	if err != nil {
		return fmt.Errorf("desactivar cuenta: %w", err)
	}

	r.logger.Info("account deactivated", auditAttrs(actorFromContext(ctx), id, role)...)
	return nil
}

// IsActive checks if an account is active. Returns (false, nil) for missing rows — NOT domain.ErrNotFound.
// Requires an authenticated caller.
func (r *AccountsRepo) IsActive(ctx context.Context, id string) (bool, error) {
	if _, err := auth.RequireCaller(ctx); err != nil {
		return false, fmt.Errorf("verificar estado activo: %w", err)
	}

	var isActive int
	err := r.db.QueryRowContext(ctx,
		`SELECT is_active FROM accounts WHERE id = ?`, id,
	).Scan(&isActive)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return false, nil
		}
		return false, fmt.Errorf("verificar estado activo: %w", err)
	}
	return isActive == 1, nil
}

// ListByProfessional returns staff accounts matching the given professional ID, ordered by display_name ASC.
// Requires an authenticated caller.
func (r *AccountsRepo) ListByProfessional(ctx context.Context, professionalID string) ([]*entity.Account, error) {
	if _, err := auth.RequireCaller(ctx); err != nil {
		return nil, fmt.Errorf("buscar cuentas por profesional: %w", err)
	}

	if professionalID == "" {
		return nil, fmt.Errorf("buscar por profesional: %w: professional_id no puede estar vacío", domain.ErrInvalidInput)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT id, role, display_name, professional_id, is_active, created_at, updated_at FROM accounts WHERE role = 'staff' AND professional_id = ? ORDER BY display_name ASC`,
		professionalID,
	)
	if err != nil {
		return nil, fmt.Errorf("buscar cuentas por profesional: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	accounts := make([]*entity.Account, 0)
	for rows.Next() {
		a, err := scanAccountRow(rows)
		if err != nil {
			return nil, fmt.Errorf("buscar cuentas por profesional: %w", err)
		}
		accounts = append(accounts, a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("buscar cuentas por profesional: %w", err)
	}
	return accounts, nil
}

// scanAccount scans a *sql.Row into an *entity.Account. Wraps sql.ErrNoRows as domain.ErrNotFound.
func scanAccount(row *sql.Row) (*entity.Account, error) {
	var a entity.Account
	var isActive int
	var profID *string
	var roleStr string

	err := row.Scan(&a.ID, &roleStr, &a.DisplayName, &profID, &isActive, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("cuenta no encontrada: %w", domain.ErrNotFound)
		}
		return nil, fmt.Errorf("leer cuenta: %w", err)
	}

	a.Role = entity.AccountRole(roleStr)
	a.ProfessionalID = profID
	a.Active = isActive == 1
	return &a, nil
}

// scanAccountRow scans a *sql.Rows (from Query) into an *entity.Account.
func scanAccountRow(rows *sql.Rows) (*entity.Account, error) {
	var a entity.Account
	var isActive int
	var profID *string
	var roleStr string

	if err := rows.Scan(&a.ID, &roleStr, &a.DisplayName, &profID, &isActive, &a.CreatedAt, &a.UpdatedAt); err != nil {
		return nil, fmt.Errorf("leer cuenta: %w", err)
	}

	a.Role = entity.AccountRole(roleStr)
	a.ProfessionalID = profID
	a.Active = isActive == 1
	return &a, nil
}
