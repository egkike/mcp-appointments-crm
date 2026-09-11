package config

import (
	"context"
	"database/sql"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egkike/mcp-appointments-crm/internal/db"
)

// All boot integration tests use a file-based temporary database. NEVER use
// ":memory:" because db.NewDatabase.verifyPragmas requires WAL mode, and SQLite
// returns "memory" for in-memory databases (ADR-SD-10).

func newBootDB(t *testing.T) *sql.DB {
	t.Helper()
	ctx := context.Background()
	database, err := db.NewDatabase(ctx, filepath.Join(t.TempDir(), "boot.db"))
	if err != nil {
		t.Fatalf("open test database: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	return database.Conn
}

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.NewFile(0, os.DevNull), nil))
}

func TestSeedOnBoot_FreshValidSeedsAndSecondCallNoOp(t *testing.T) {
	ctx := context.Background()
	conn := newBootDB(t)
	dir := prepareValidSetupDir(t)
	t.Setenv("MCP_SETUP_DIR", dir)

	if err := SeedOnBoot(ctx, conn, discardLogger()); err != nil {
		t.Fatalf("first SeedOnBoot failed: %v", err)
	}

	var name string
	if err := conn.QueryRowContext(ctx,
		"SELECT name FROM business_profile WHERE id = 'singleton'").Scan(&name); err != nil {
		t.Fatalf("query business_profile: %v", err)
	}
	if name != "Estudio de Belleza Rosa" {
		t.Errorf("business name = %q, want seeded name", name)
	}

	var profCount, svcCount int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM professionals").Scan(&profCount); err != nil {
		t.Fatalf("count professionals: %v", err)
	}
	if profCount != 1 {
		t.Errorf("professionals count = %d, want 1", profCount)
	}
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM services").Scan(&svcCount); err != nil {
		t.Fatalf("count services: %v", err)
	}
	if svcCount != 2 {
		t.Errorf("services count = %d, want 2", svcCount)
	}

	// Second call must be a no-op: guard sees name != '' and returns nil.
	if err := SeedOnBoot(ctx, conn, discardLogger()); err != nil {
		t.Fatalf("second SeedOnBoot failed: %v", err)
	}

	var profCount2, svcCount2 int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM professionals").Scan(&profCount2); err != nil {
		t.Fatalf("count professionals after second call: %v", err)
	}
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM services").Scan(&svcCount2); err != nil {
		t.Fatalf("count services after second call: %v", err)
	}
	if profCount2 != profCount || svcCount2 != svcCount {
		t.Errorf("second call changed data: professionals %d->%d, services %d->%d",
			profCount, profCount2, svcCount, svcCount2)
	}
}

func TestSeedOnBoot_FreshMalformedJSONAborts(t *testing.T) {
	ctx := context.Background()
	conn := newBootDB(t)
	dir := t.TempDir()
	mustCopy(t, filepath.Join("testdata", "setup_business.json"), filepath.Join(dir, "setup_business.json"))
	mustCopy(t, filepath.Join("testdata", "setup_staff.json"), filepath.Join(dir, "setup_staff.json"))
	if err := os.WriteFile(filepath.Join(dir, "setup_services.json"), []byte("not json"), 0600); err != nil {
		t.Fatalf("write malformed file: %v", err)
	}
	t.Setenv("MCP_SETUP_DIR", dir)

	err := SeedOnBoot(ctx, conn, discardLogger())
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "setup_services.json") {
		t.Errorf("error %q does not contain filename", msg)
	}
	if !strings.Contains(msg, "formato inválido") {
		t.Errorf("error %q does not indicate invalid format", msg)
	}

	var name string
	if err := conn.QueryRowContext(ctx,
		"SELECT name FROM business_profile WHERE id = 'singleton'").Scan(&name); err != nil {
		t.Fatalf("query business_profile: %v", err)
	}
	if name != "" {
		t.Errorf("business_profile.name = %q, want empty after failed seed", name)
	}

	var profCount, svcCount int
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM professionals").Scan(&profCount); err != nil {
		t.Fatalf("count professionals: %v", err)
	}
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM services").Scan(&svcCount); err != nil {
		t.Fatalf("count services: %v", err)
	}
	if profCount != 0 || svcCount != 0 {
		t.Errorf("pre-seed state not preserved: professionals=%d, services=%d", profCount, svcCount)
	}
}

func TestSeedOnBoot_NotFreshMissingSetupDirReturnsNil(t *testing.T) {
	ctx := context.Background()
	conn := newBootDB(t)

	// Make the DB not fresh by inserting a placeholder with a non-empty name.
	if _, err := conn.ExecContext(ctx,
		"INSERT OR REPLACE INTO business_profile (id, name) VALUES ('singleton', 'Existing Business')"); err != nil {
		t.Fatalf("mark db not fresh: %v", err)
	}

	// Point to a directory that does not exist; guard-first means loader never runs.
	t.Setenv("MCP_SETUP_DIR", filepath.Join(t.TempDir(), "missing", "setup"))

	if err := SeedOnBoot(ctx, conn, discardLogger()); err != nil {
		t.Fatalf("expected nil for not-fresh DB, got %v", err)
	}
}

func TestSeedOnBoot_FreshUnsetHomeReturnsError(t *testing.T) {
	ctx := context.Background()
	conn := newBootDB(t)
	t.Setenv("HOME", "")
	t.Setenv("MCP_SETUP_DIR", "")

	err := SeedOnBoot(ctx, conn, discardLogger())
	if err == nil {
		t.Fatal("expected error when HOME and MCP_SETUP_DIR are unset, got nil")
	}
	msg := err.Error()
	if !strings.Contains(msg, "HOME no está definida") {
		t.Errorf("error %q does not contain expected Spanish HOME message", msg)
	}
}

func prepareValidSetupDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"setup_business.json", "setup_staff.json", "setup_services.json"} {
		mustCopy(t, filepath.Join("testdata", name), filepath.Join(dir, name))
	}
	return dir
}
