package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/auth"
	"github.com/egkike/mcp-appointments-crm/internal/domain"
	"github.com/egkike/mcp-appointments-crm/internal/domain/entity"
	domainrepo "github.com/egkike/mcp-appointments-crm/internal/domain/repository"
	"github.com/egkike/mcp-appointments-crm/internal/idgen"
)

// Compile-time interface conformance check.
var _ domainrepo.ClientsRepo = (*ClientsRepo)(nil)

// sqlArgs is the driver-bound argument slice. Kept as []any only because
// database/sql.QueryContext/ExecContext accept variadic ...any. Domain inputs
// (phone, id, etc.) remain concrete types.
type sqlArgs = []any

// applyClientsAuthFilter composes a clients query and args based on the caller's
// role. Unlike applyAuthFilter (bookings), the clients table has no client_id
// column — the row's own PK id IS the client id — so the client scope clause is
// " AND id = ?". Staff and unknown roles have no legitimate scope on clients and
// are rejected outright (ErrForbidden). Admin/owner: query unchanged.
//
// baseQuery must end at the WHERE clause (no trailing ORDER BY/LIMIT). suffix
// holds any trailing clauses and is appended after the optional scope filter.
// This avoids parsing SQL with string searches.
//
// filterClause is built only from the allowlisted constants below; callers never
// pass dynamic SQL fragments.
func applyClientsAuthFilter(caller *auth.Caller, baseQuery string, suffix string, baseArgs sqlArgs) (string, sqlArgs, error) {
	if caller == nil { // defensive backstop; callers use RequireCaller first
		return "", nil, &domain.SemanticError{
			Code:    domain.ErrCodeUnauthenticated,
			Message: "se requiere autenticación",
			Cause:   domain.ErrUnauthenticated,
		}
	}

	args := make(sqlArgs, len(baseArgs), len(baseArgs)+1)
	copy(args, baseArgs)

	const (
		filterByClientID      = " AND id = ?"
		filterByStaffBookings = " AND id IN (SELECT client_id FROM bookings WHERE professional_id = ?)"
	)

	var filterClause string
	var filterArg string
	switch caller.Role {
	case auth.RoleClient:
		if caller.ClientID == nil {
			return "", nil, &domain.SemanticError{
				Code:    domain.ErrCodeForbidden,
				Message: "Cliente no tiene ID asignado",
				Cause:   domain.ErrForbidden,
			}
		}
		filterClause = filterByClientID
		filterArg = *caller.ClientID
	case auth.RoleStaff:
		if caller.ProfessionalID == nil {
			return "", nil, &domain.SemanticError{
				Code:    domain.ErrCodeForbidden,
				Message: "Personal no tiene un profesional asignado",
				Cause:   domain.ErrForbidden,
			}
		}
		filterClause = filterByStaffBookings
		filterArg = *caller.ProfessionalID
	case auth.RoleAdmin, auth.RoleOwner:
		// no extra filter
	default:
		return "", nil, &domain.SemanticError{
			Code:    domain.ErrCodeForbidden,
			Message: fmt.Sprintf("Rol %q no tiene permiso para acceder a clientes", caller.Role),
			Cause:   domain.ErrForbidden,
		}
	}

	if filterClause == "" {
		return baseQuery + suffix, args, nil
	}

	return baseQuery + filterClause + suffix, append(args, filterArg), nil
}

// ClientsRepo provides CRUD, FTS5 search, and phone-based lookup for the
// clients table. Phone is UNIQUE (serves as the chat ID for WhatsApp/Telegram).
type ClientsRepo struct {
	db *sql.DB
}

// NewClientsRepo creates a new ClientsRepo.
func NewClientsRepo(db *sql.DB) *ClientsRepo {
	return &ClientsRepo{db: db}
}

// Save inserts or updates a client (upsert by ID).
func (r *ClientsRepo) Save(ctx context.Context, c *entity.Client) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("guardar cliente: %w", err)
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("guardar cliente: el nombre no puede estar vacío: %w", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(c.Phone) == "" {
		return fmt.Errorf("guardar cliente: el teléfono no puede estar vacío: %w", domain.ErrInvalidInput)
	}
	// UPSERT by id — never INSERT OR REPLACE: REPLACE deletes the conflicting
	// row, which with foreign_keys=ON fires ON DELETE CASCADE and would wipe
	// every booking of an existing client (and reset created_at). DO UPDATE
	// keeps the row (and its created_at) in place.
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO clients (id, name, phone, email, preferences, updated_at)
		 VALUES (?, ?, ?, ?, ?, strftime('%Y-%m-%dT%H:%M:%fZ', 'now'))
		 ON CONFLICT(id) DO UPDATE SET
			name        = excluded.name,
			phone       = excluded.phone,
			email       = excluded.email,
			preferences = excluded.preferences,
			updated_at  = excluded.updated_at`,
		c.ID, c.Name, c.Phone, c.Email, c.Preferences,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("guardar cliente: el teléfono ya está registrado: %w", domain.ErrConflict)
		}
		return fmt.Errorf("guardar cliente: %w", err)
	}
	return nil
}

// Create inserts a new client. Returns domain.ErrInvalidInput if name or phone is empty.
// Returns domain.ErrConflict if the phone is already in use (UNIQUE violation).
func (r *ClientsRepo) Create(ctx context.Context, c *entity.Client) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("crear cliente: %w", err)
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("crear cliente: el nombre no puede estar vacío: %w", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(c.Phone) == "" {
		return fmt.Errorf("crear cliente: el teléfono no puede estar vacío: %w", domain.ErrInvalidInput)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO clients (id, name, phone, email, preferences)
		 VALUES (?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Phone, c.Email, c.Preferences,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("crear cliente: el teléfono ya está registrado: %w", domain.ErrConflict)
		}
		return fmt.Errorf("crear cliente: %w", err)
	}
	return nil
}

// FindByID returns a client by ID. Returns domain.ErrNotFound if not found.
func (r *ClientsRepo) FindByID(ctx context.Context, id string) (*entity.Client, error) {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtener cliente %s: %w", id, err)
	}

	c := &entity.Client{Active: true}
	query := `SELECT id, name, phone, email, preferences, created_at, updated_at
		 FROM clients WHERE id = ?`
	args := sqlArgs{id}
	query, args, err = applyClientsAuthFilter(caller, query, "", args)
	if err != nil {
		return nil, fmt.Errorf("obtener cliente %s: %w", id, err)
	}

	err = r.db.QueryRowContext(ctx, query, args...).Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.Preferences,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeNotFound,
				Message: "Cliente no encontrado",
				Cause:   domain.ErrNotFound,
			}
		}
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "Error interno al obtener el cliente",
			Cause:   err,
		}
	}
	return c, nil
}

// FindByPhone returns a client by phone number. Returns domain.ErrNotFound if not found.
// Only admin/owner may call this endpoint. Admin is a trusted role with
// unrestricted client PII access by design (PRD §3.8.4), so no additional
// per-row scope is applied; the role gate itself prevents disclosure to
// staff/clients regardless of whether the phone exists.
func (r *ClientsRepo) FindByPhone(ctx context.Context, phone string) (*entity.Client, error) {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return nil, fmt.Errorf("obtener cliente por teléfono: %w", err)
	}

	c := &entity.Client{Active: true}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, phone, email, preferences, created_at, updated_at
		 FROM clients WHERE phone = ?`, phone,
	).Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.Preferences,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeNotFound,
				Message: "Cliente no encontrado",
				Cause:   domain.ErrNotFound,
			}
		}
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "Error interno al buscar el cliente",
			Cause:   err,
		}
	}
	return c, nil
}

// FindClientByPhoneAny returns the client whose phone column equals phone,
// WHATEVER its id, or domain.ErrNotFound when no row matches.
//
// It differs from FindByPhone in two deliberate ways: it performs no
// role/caller check and it does not assume the row's id equals its phone. It
// backs RegistrationLookup for the anonymous registration path, which must
// distinguish an existing id == phone row (idempotent no-op) from a legacy
// id != phone row (explicit conflict) — so returning the row as-is, rather
// than filtering by id, is the contract. Error mapping matches FindByPhone
// (domain.ErrNotFound on a miss, internal semantic error otherwise).
//
// This method is auth-free by design: the registration path runs before any
// Caller can be resolved. It is deliberately absent from the ClientsRepo
// interface and is wired ONLY into the registration use case through the
// RegistrationLookup adapter, so no authenticated flow can reach it.
func (r *ClientsRepo) FindClientByPhoneAny(ctx context.Context, phone string) (*entity.Client, error) {
	c := &entity.Client{Active: true}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, phone, email, preferences, created_at, updated_at
		 FROM clients WHERE phone = ?`, phone,
	).Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.Preferences,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeNotFound,
				Message: "Cliente no encontrado",
				Cause:   domain.ErrNotFound,
			}
		}
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "Error interno al buscar el cliente",
			Cause:   err,
		}
	}
	return c, nil
}

// GetOrCreate inserts a new client if the phone does not exist, or returns
// the existing client. Idempotent: does not overwrite the existing name.
func (r *ClientsRepo) GetOrCreate(ctx context.Context, phone, name string) (*entity.Client, error) {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("obtener o crear cliente: %w", err)
	}
	// Client self-service is anchored on the caller's chat/phone ID (caller.ID),
	// not on caller.ClientID (the UUID primary key). A client may only bootstrap
	// their own record using the phone number they are calling from.
	if caller.Role != auth.RoleAdmin && caller.Role != auth.RoleOwner {
		if caller.Role != auth.RoleClient || phone == "" || phone != caller.ID {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeForbidden,
				Message: "no tienes permiso para realizar esta acción",
				Cause:   domain.ErrForbidden,
			}
		}
	}

	if strings.TrimSpace(phone) == "" {
		return nil, fmt.Errorf("obtener o crear cliente: el teléfono no puede estar vacío: %w", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("obtener o crear cliente: el nombre no puede estar vacío: %w", domain.ErrInvalidInput)
	}
	_, err = r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO clients (id, name, phone) VALUES (?, ?, ?)`,
		idgen.NewUUID(), name, phone,
	)
	if err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "Error interno al crear el cliente",
			Cause:   err,
		}
	}

	c := &entity.Client{Active: true}
	err = r.db.QueryRowContext(ctx,
		`SELECT id, name, phone, email, preferences, created_at, updated_at
		 FROM clients WHERE phone = ?`, phone,
	).Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.Preferences,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "Error interno al recuperar el cliente",
			Cause:   err,
		}
	}
	return c, nil
}

// GetOrCreateByPhone returns the client whose id equals phone, creating it when
// absent; the bool reports whether this call inserted the row.
//
// The stored id is byte-identical to phone: CallerResolver matches
// clients.id == phone, so the UUID id generated by GetOrCreate could never
// resolve and this method MUST NOT delegate to it. No phone normalization.
//
// No RBAC guard: registration calls this before a caller has been resolved; the
// value guards (accounts collision, rate limit) live in the use case.
func (r *ClientsRepo) GetOrCreateByPhone(ctx context.Context, phone, displayName string) (entity.Client, bool, error) {
	if strings.TrimSpace(phone) == "" || strings.TrimSpace(displayName) == "" {
		return entity.Client{}, false, fmt.Errorf(
			"registrar cliente: teléfono y nombre son obligatorios: %w", domain.ErrInvalidInput)
	}
	conn, err := r.db.Conn(ctx)
	if err != nil {
		return entity.Client{}, false, errGetOrCreateByPhone(err)
	}
	defer func() { _ = conn.Close() }()

	// BEGIN IMMEDIATE locks the write path before the INSERT so concurrent
	// registrations serialize instead of racing on a deferred upgrade (which
	// SQLite refuses with SQLITE_BUSY).
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return entity.Client{}, false, errGetOrCreateByPhone(err)
	}
	committed := false
	defer func() {
		if !committed {
			// Rollback on every early return: otherwise the connection returns to
			// the pool inside an open transaction. Context detachment keeps the
			// cleanup working even when ctx was cancelled.
			_, _ = conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		}
	}()

	res, err := conn.ExecContext(ctx,
		`INSERT OR IGNORE INTO clients (id, name, phone) VALUES (?, ?, ?)`,
		phone, displayName, phone,
	)
	if err != nil {
		return entity.Client{}, false, errGetOrCreateByPhone(err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return entity.Client{}, false, errGetOrCreateByPhone(err)
	}

	c := entity.Client{Active: true}
	if err := conn.QueryRowContext(ctx,
		`SELECT id, name, phone FROM clients WHERE id = ?`, phone,
	).Scan(&c.ID, &c.Name, &c.Phone); err != nil {
		return entity.Client{}, false, errGetOrCreateByPhone(err)
	}

	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return entity.Client{}, false, errGetOrCreateByPhone(err)
	}
	committed = true

	return c, affected > 0, nil
}

// errGetOrCreateByPhone collapses an adapter failure into a semantic error.
func errGetOrCreateByPhone(err error) error {
	return &domain.SemanticError{
		Code:    domain.ErrCodeInternal,
		Message: "Error interno al registrar el cliente",
		Cause:   err,
	}
}

// Update updates an existing client. Returns domain.ErrInvalidInput if name or phone
// is empty. Returns domain.ErrNotFound if no row matches.
// Returns domain.ErrConflict if the new phone violates the UNIQUE constraint.
func (r *ClientsRepo) Update(ctx context.Context, c *entity.Client) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("actualizar cliente: %w", err)
	}
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("actualizar cliente: el nombre no puede estar vacío: %w", domain.ErrInvalidInput)
	}
	if strings.TrimSpace(c.Phone) == "" {
		return fmt.Errorf("actualizar cliente: el teléfono no puede estar vacío: %w", domain.ErrInvalidInput)
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE clients SET name=?, phone=?, email=?, preferences=?,
		 updated_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE id=?`,
		c.Name, c.Phone, c.Email, c.Preferences, c.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("actualizar cliente: el teléfono ya está registrado: %w", domain.ErrConflict)
		}
		return fmt.Errorf("actualizar cliente: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("actualizar cliente: filas afectadas: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("actualizar cliente: %w", domain.ErrNotFound)
	}
	return nil
}

// Delete removes a client by ID. Returns domain.ErrNotFound if no row matches.
func (r *ClientsRepo) Delete(ctx context.Context, id string) error {
	if _, err := auth.RequireRole(ctx, auth.RoleAdmin, auth.RoleOwner); err != nil {
		return fmt.Errorf("eliminar cliente: %w", err)
	}
	result, err := r.db.ExecContext(ctx, `DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("eliminar cliente: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("eliminar cliente: filas afectadas: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("eliminar cliente: %w", domain.ErrNotFound)
	}
	return nil
}

// SearchFTS performs a full-text search on clients using FTS5 MATCH.
// Results are ordered by FTS5 rank (most relevant first).
// Returns domain.ErrInvalidInput if the query contains FTS5 operator characters.
func (r *ClientsRepo) SearchFTS(ctx context.Context, query string) ([]*entity.Client, error) {
	caller, err := auth.RequireCaller(ctx)
	if err != nil {
		return nil, fmt.Errorf("buscar clientes: %w", err)
	}

	if err := validateFTSQuery(query); err != nil {
		return nil, fmt.Errorf("buscar clientes: %w", err)
	}

	sqlQuery := `SELECT c.id, c.name, c.phone, c.email, c.preferences,
		c.created_at, c.updated_at
		 FROM clients c
		 JOIN clients_fts ON c.rowid = clients_fts.rowid
		 WHERE clients_fts MATCH ?`
	args := sqlArgs{query}
	suffix := " ORDER BY bm25(clients_fts)"
	sqlQuery, args, err = applyClientsAuthFilter(caller, sqlQuery, suffix, args)
	if err != nil {
		return nil, fmt.Errorf("buscar clientes: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "Error interno al buscar clientes",
			Cause:   err,
		}
	}
	defer func() { _ = rows.Close() }()

	var clients []*entity.Client
	for rows.Next() {
		c := &entity.Client{Active: true}
		if err := rows.Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.Preferences,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, &domain.SemanticError{
				Code:    domain.ErrCodeInternal,
				Message: "Error interno al leer resultados",
				Cause:   err,
			}
		}
		clients = append(clients, c)
	}
	if err := rows.Err(); err != nil {
		return nil, &domain.SemanticError{
			Code:    domain.ErrCodeInternal,
			Message: "Error interno durante la búsqueda",
			Cause:   err,
		}
	}
	return clients, nil
}
