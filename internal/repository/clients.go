package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/egkike/mcp-appointments-crm/internal/model"
)

// ClientsRepo provides CRUD, FTS5 search, and phone-based lookup for the
// clients table. Phone is UNIQUE (serves as the chat ID for WhatsApp/Telegram).
type ClientsRepo struct {
	db *sql.DB
}

// NewClientsRepo creates a new ClientsRepo.
func NewClientsRepo(db *sql.DB) *ClientsRepo {
	return &ClientsRepo{db: db}
}

// Create inserts a new client. Returns ErrInvalidInput if name or phone is empty.
// Returns ErrConflict if the phone is already in use (UNIQUE violation).
func (r *ClientsRepo) Create(ctx context.Context, c *model.Client) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("create client: name must not be empty: %w", ErrInvalidInput)
	}
	if strings.TrimSpace(c.Phone) == "" {
		return fmt.Errorf("create client: phone must not be empty: %w", ErrInvalidInput)
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO clients (id, name, phone, email, preferences)
		 VALUES (?, ?, ?, ?, ?)`,
		c.ID, c.Name, c.Phone, c.Email, c.Preferences,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("create client: phone %s already exists: %w", c.Phone, ErrConflict)
		}
		return fmt.Errorf("create client: %w", err)
	}
	return nil
}

// Get returns a client by ID. Returns ErrNotFound if not found.
func (r *ClientsRepo) Get(ctx context.Context, id string) (*model.Client, error) {
	c := &model.Client{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, phone, email, preferences, created_at, updated_at
		 FROM clients WHERE id = ?`, id,
	).Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.Preferences,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get client %s: %w", id, ErrNotFound)
		}
		return nil, fmt.Errorf("get client %s: %w", id, err)
	}
	return c, nil
}

// GetByPhone returns a client by phone number. Returns ErrNotFound if not found.
func (r *ClientsRepo) GetByPhone(ctx context.Context, phone string) (*model.Client, error) {
	c := &model.Client{}
	err := r.db.QueryRowContext(ctx,
		`SELECT id, name, phone, email, preferences, created_at, updated_at
		 FROM clients WHERE phone = ?`, phone,
	).Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.Preferences,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("get client by phone %s: %w", phone, ErrNotFound)
		}
		return nil, fmt.Errorf("get client by phone %s: %w", phone, err)
	}
	return c, nil
}

// GetOrCreate inserts a new client if the phone does not exist, or returns
// the existing client. Idempotent: does not overwrite the existing name.
func (r *ClientsRepo) GetOrCreate(ctx context.Context, phone, name string) (*model.Client, error) {
	_, err := r.db.ExecContext(ctx,
		`INSERT OR IGNORE INTO clients (id, name, phone) VALUES (?, ?, ?)`,
		model.NewUUID(), name, phone,
	)
	if err != nil {
		return nil, fmt.Errorf("get or create client: insert: %w", err)
	}

	c := &model.Client{}
	err = r.db.QueryRowContext(ctx,
		`SELECT id, name, phone, email, preferences, created_at, updated_at
		 FROM clients WHERE phone = ?`, phone,
	).Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.Preferences,
		&c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get or create client: select: %w", err)
	}
	return c, nil
}

// Update updates an existing client. Returns ErrInvalidInput if name or phone
// is empty. Returns ErrNotFound if no row matches.
// Returns ErrConflict if the new phone violates the UNIQUE constraint.
func (r *ClientsRepo) Update(ctx context.Context, c *model.Client) error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("update client: name must not be empty: %w", ErrInvalidInput)
	}
	if strings.TrimSpace(c.Phone) == "" {
		return fmt.Errorf("update client: phone must not be empty: %w", ErrInvalidInput)
	}
	result, err := r.db.ExecContext(ctx,
		`UPDATE clients SET name=?, phone=?, email=?, preferences=?,
		 updated_at=strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		 WHERE id=?`,
		c.Name, c.Phone, c.Email, c.Preferences, c.ID,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("update client: phone %s already exists: %w", c.Phone, ErrConflict)
		}
		return fmt.Errorf("update client: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update client rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("update client: %w", ErrNotFound)
	}
	return nil
}

// Delete removes a client by ID. Returns ErrNotFound if no row matches.
func (r *ClientsRepo) Delete(ctx context.Context, id string) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM clients WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete client: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("delete client rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("delete client: %w", ErrNotFound)
	}
	return nil
}

// SearchFTS performs a full-text search on clients using FTS5 MATCH.
// Results are ordered by FTS5 rank (most relevant first).
// Returns ErrInvalidInput if the query contains FTS5 operator characters.
func (r *ClientsRepo) SearchFTS(ctx context.Context, query string) ([]*model.Client, error) {
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("search clients FTS: empty query: %w", ErrInvalidInput)
	}
	if ftsQueryRe.MatchString(query) {
		return nil, fmt.Errorf("search clients FTS: query contains forbidden characters: %w", ErrInvalidInput)
	}

	rows, err := r.db.QueryContext(ctx,
		`SELECT c.id, c.name, c.phone, c.email, c.preferences,
			c.created_at, c.updated_at
		 FROM clients c
		 JOIN clients_fts f ON c.rowid = f.rowid
		 WHERE clients_fts MATCH ?
		 ORDER BY bm25(clients_fts)`,
		query,
	)
	if err != nil {
		return nil, fmt.Errorf("search clients FTS: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Close errors are non-critical after iteration

	var clients []*model.Client
	for rows.Next() {
		c := &model.Client{}
		if err := rows.Scan(&c.ID, &c.Name, &c.Phone, &c.Email, &c.Preferences,
			&c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("search clients FTS: scan: %w", err)
		}
		clients = append(clients, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("search clients FTS: rows: %w", err)
	}
	return clients, nil
}
