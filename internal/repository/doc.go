// Package repository implements the data access layer over the SQLite database.
//
// Each entity in internal/model has a corresponding *Repo type (e.g., BookingsRepo,
// ClientsRepo) that centralizes all SQL for that table. All queries use prepared
// statements (? placeholders) to prevent SQL injection.
//
// The repository layer is the only layer that knows about SQL. The MCP handlers and
// any future business-logic layer consume *Repo instances via interfaces defined
// here (e.g., BookingsRepository interface).
package repository
