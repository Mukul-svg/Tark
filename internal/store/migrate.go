package store

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RunEmbeddedMigrations reads all *.sql files from the embedded migrations/
// directory and applies them in order, skipping any already recorded in
// schema_migrations.
func RunEmbeddedMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	if _, err := pool.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version    INT PRIMARY KEY,
			checksum   TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);
	`); err != nil {
		return fmt.Errorf("ensure schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(embedMigrations, "migrations")
	if err != nil {
		return fmt.Errorf("read migration embed dir: %w", err)
	}

	// Filter to .sql files only and sort for deterministic ordering.
	var sqlFiles []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".sql") {
			sqlFiles = append(sqlFiles, e.Name())
		}
	}
	sort.Strings(sqlFiles)

	if len(sqlFiles) == 0 {
		slog.Info("no migrations found")
		return nil
	}

	for _, filename := range sqlFiles {
		version, name := parseFilename(filename)
		if version <= 0 {
			return fmt.Errorf("invalid migration filename: %s (expected <version>_name.sql)", filename)
		}

		if alreadyApplied(ctx, pool, version) {
			slog.Info("skip migration (already applied)", "version", version, "name", name)
			continue
		}

		slog.Info("applying migration", "version", version, "name", name)

		data, err := embedMigrations.ReadFile("migrations/" + filename)
		if err != nil {
			return fmt.Errorf("read migration %d: %w", version, err)
		}

		tx, err := pool.Begin(ctx)
		if err != nil {
			return fmt.Errorf("begin txn for migration %d: %w", version, err)
		}

		if _, err := tx.Exec(ctx, string(data)); err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("run migration %d %s: %w", version, name, err)
		}

		_, err = tx.Exec(ctx,
			"INSERT INTO schema_migrations (version, checksum) VALUES ($1, $2)",
			version,
			computeChecksum(data),
		)
		if err != nil {
			_ = tx.Rollback(ctx)
			return fmt.Errorf("record migration %d: %w", version, err)
		}

		if err := tx.Commit(ctx); err != nil {
			return fmt.Errorf("commit migration %d: %w", version, err)
		}

		slog.Info("migration applied", "version", version, "name", name)
	}

	slog.Info("all migrations up to date")
	return nil
}

func alreadyApplied(ctx context.Context, pool *pgxpool.Pool, version int) bool {
	var exists bool
	err := pool.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", version).Scan(&exists)
	return err == nil && exists
}

func parseFilename(filename string) (int, string) {
	name := strings.TrimSuffix(filename, ".sql") // e.g. "001_init"
	parts := strings.SplitN(name, "_", 2)
	if len(parts) != 2 {
		return -1, name
	}
	v, err := strconv.Atoi(parts[0])
	if err != nil {
		return -1, name
	}
	return v, parts[1]
}

func computeChecksum(data []byte) string {
	var sum uint32
	for _, b := range data {
		sum = sum*31 + uint32(b)
	}
	return fmt.Sprintf("%08x", sum)
}
