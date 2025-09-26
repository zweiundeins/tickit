package migrations

import (
	"context"
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/delaneyj/toolbelt"
	"zombiezen.com/go/sqlite"
)

//go:embed sql/*.sql
var migrationFiles embed.FS

// Migration represents a single database migration
type Migration struct {
	Version     int
	Name        string
	UpSQL       string
	DownSQL     string
	AppliedAt   *time.Time
	Description string
}

// MigrationManager handles database migrations
type MigrationManager struct {
	db *toolbelt.Database
}

// NewMigrationManager creates a new migration manager
func NewMigrationManager(ctx context.Context, dbFilename string) (*MigrationManager, error) {
	// Create the migrations table first
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at DATETIME NOT NULL,
			description TEXT
		)`,
		`CREATE INDEX IF NOT EXISTS idx_schema_migrations_version ON schema_migrations(version)`,
	}

	db, err := toolbelt.NewDatabase(ctx, dbFilename, migrations)
	if err != nil {
		return nil, fmt.Errorf("failed to create migration manager database: %w", err)
	}

	return &MigrationManager{db: db}, nil
}

// GetAppliedMigrations returns all applied migrations
func (m *MigrationManager) GetAppliedMigrations(ctx context.Context) ([]*Migration, error) {
	var migrations []*Migration
	err := m.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT version, name, applied_at, description
			FROM schema_migrations
			ORDER BY version
		`)
		if err != nil {
			// If the table doesn't exist, return empty list (no migrations applied yet)
			if strings.Contains(err.Error(), "no such table: schema_migrations") {
				return nil
			}
			return err
		}
		defer stmt.Finalize()

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			appliedAt := time.Unix(stmt.ColumnInt64(2), 0)
			migration := &Migration{
				Version:     int(stmt.ColumnInt64(0)),
				Name:        stmt.ColumnText(1),
				AppliedAt:   &appliedAt,
				Description: stmt.ColumnText(3),
			}
			migrations = append(migrations, migration)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return migrations, nil
}

// IsMigrationApplied checks if a specific migration version has been applied
func (m *MigrationManager) IsMigrationApplied(ctx context.Context, version int) (bool, error) {
	var exists bool
	err := m.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`SELECT 1 FROM schema_migrations WHERE version = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindInt64(1, int64(version))
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		exists = rowReturned
		return nil
	})
	if err != nil {
		return false, err
	}
	return exists, nil
}

// ApplyMigration applies a single migration
func (m *MigrationManager) ApplyMigration(ctx context.Context, migration *Migration) error {
	return m.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		// Execute the migration SQL
		if migration.UpSQL != "" {
			// Split SQL into individual statements
			statements := splitSQLStatements(migration.UpSQL)
			for i, sqlStmt := range statements {
				sqlStmt = strings.TrimSpace(sqlStmt)
				if sqlStmt == "" || strings.HasPrefix(sqlStmt, "--") {
					continue // Skip empty lines and comments
				}

				stmt, err := tx.Prepare(sqlStmt)
				if err != nil {
					return fmt.Errorf("failed to prepare migration %d statement %d: %w", migration.Version, i+1, err)
				}
				_, err = stmt.Step()
				stmt.Finalize()
				if err != nil {
					return fmt.Errorf("failed to execute migration %d statement %d: %w", migration.Version, i+1, err)
				}
			}
		}

		// Record the migration as applied
		stmt, err := tx.Prepare(`
			INSERT INTO schema_migrations (version, name, applied_at, description)
			VALUES (?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindInt64(1, int64(migration.Version))
		stmt.BindText(2, migration.Name)
		stmt.BindInt64(3, time.Now().Unix())
		stmt.BindText(4, migration.Description)

		_, err = stmt.Step()
		if err != nil {
			return fmt.Errorf("failed to record migration %d: %w", migration.Version, err)
		}

		log.Printf("Applied migration %d: %s", migration.Version, migration.Name)
		return nil
	})
}

// RollbackMigration rolls back a single migration
func (m *MigrationManager) RollbackMigration(ctx context.Context, migration *Migration) error {
	return m.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		// Execute the rollback SQL
		if migration.DownSQL != "" {
			stmt, err := tx.Prepare(migration.DownSQL)
			if err != nil {
				return fmt.Errorf("failed to prepare rollback %d: %w", migration.Version, err)
			}
			defer stmt.Finalize()
			_, err = stmt.Step()
			if err != nil {
				return fmt.Errorf("failed to rollback migration %d: %w", migration.Version, err)
			}
		}

		// Remove the migration record
		stmt, err := tx.Prepare(`DELETE FROM schema_migrations WHERE version = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindInt64(1, int64(migration.Version))
		_, err = stmt.Step()
		if err != nil {
			return fmt.Errorf("failed to remove migration record %d: %w", migration.Version, err)
		}

		log.Printf("Rolled back migration %d: %s", migration.Version, migration.Name)
		return nil
	})
}

// ApplyMigrations applies all pending migrations
func (m *MigrationManager) ApplyMigrations(ctx context.Context, migrations []*Migration) error {
	// Ensure schema_migrations table exists
	if err := m.ensureSchemaMigrationsTable(ctx); err != nil {
		return fmt.Errorf("failed to ensure schema_migrations table: %w", err)
	}

	// Get applied migrations
	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	appliedVersions := make(map[int]bool)
	for _, migration := range applied {
		appliedVersions[migration.Version] = true
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	// Apply pending migrations
	for _, migration := range migrations {
		if !appliedVersions[migration.Version] {
			if err := m.ApplyMigration(ctx, migration); err != nil {
				return fmt.Errorf("failed to apply migration %d: %w", migration.Version, err)
			}
		}
	}

	return nil
}

// RollbackToVersion rolls back migrations to a specific version
func (m *MigrationManager) RollbackToVersion(ctx context.Context, targetVersion int, migrations []*Migration) error {
	// Get applied migrations
	applied, err := m.GetAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Create a map of migrations by version for easy lookup
	migrationMap := make(map[int]*Migration)
	for _, migration := range migrations {
		migrationMap[migration.Version] = migration
	}

	// Sort applied migrations in reverse order (newest first)
	sort.Slice(applied, func(i, j int) bool {
		return applied[i].Version > applied[j].Version
	})

	// Rollback migrations newer than target version
	for _, appliedMigration := range applied {
		if appliedMigration.Version > targetVersion {
			migration, exists := migrationMap[appliedMigration.Version]
			if !exists {
				return fmt.Errorf("migration %d not found in migration definitions", appliedMigration.Version)
			}
			if err := m.RollbackMigration(ctx, migration); err != nil {
				return fmt.Errorf("failed to rollback migration %d: %w", appliedMigration.Version, err)
			}
		}
	}

	return nil
}

// LoadMigrationsFromDirectory loads migration files from a directory
func LoadMigrationsFromDirectory(directory string) ([]*Migration, error) {
	var migrations []*Migration

	err := filepath.Walk(directory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if !strings.HasSuffix(path, ".sql") {
			return nil
		}

		filename := info.Name()
		parts := strings.Split(filename, "_")
		if len(parts) < 2 {
			return fmt.Errorf("invalid migration filename format: %s (expected: version_name.sql)", filename)
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return fmt.Errorf("invalid version number in filename: %s", filename)
		}

		name := strings.Join(parts[1:], "_")
		name = strings.TrimSuffix(name, ".sql")

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read migration file %s: %w", path, err)
		}

		// Split content into up and down migrations
		contentStr := string(content)
		upSQL := contentStr
		downSQL := ""

		// Look for -- +migrate Down comment to separate up and down migrations
		if strings.Contains(contentStr, "-- +migrate Down") {
			parts := strings.Split(contentStr, "-- +migrate Down")
			if len(parts) == 2 {
				upSQL = strings.TrimSpace(parts[0])
				downSQL = strings.TrimSpace(parts[1])
			}
		}

		migration := &Migration{
			Version:     version,
			Name:        name,
			UpSQL:       upSQL,
			DownSQL:     downSQL,
			Description: fmt.Sprintf("Migration %d: %s", version, name),
		}

		migrations = append(migrations, migration)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return migrations, nil
}

// LoadMigrations loads migration files from embedded filesystem
func LoadMigrations() ([]*Migration, error) {
	var migrations []*Migration

	entries, err := migrationFiles.ReadDir("sql")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded migration directory: %w", err)
	}

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		filename := entry.Name()
		parts := strings.Split(filename, "_")
		if len(parts) < 2 {
			return nil, fmt.Errorf("invalid migration filename format: %s (expected: version_name.sql)", filename)
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid version number in filename: %s", filename)
		}

		name := strings.Join(parts[1:], "_")
		name = strings.TrimSuffix(name, ".sql")

		content, err := migrationFiles.ReadFile("sql/" + filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read embedded migration file %s: %w", filename, err)
		}

		// Split content into up and down migrations
		contentStr := string(content)
		upSQL := contentStr
		downSQL := ""

		// Look for -- +migrate Down comment to separate up and down migrations
		if strings.Contains(contentStr, "-- +migrate Down") {
			parts := strings.Split(contentStr, "-- +migrate Down")
			if len(parts) == 2 {
				upSQL = strings.TrimSpace(parts[0])
				downSQL = strings.TrimSpace(parts[1])
			}
		}

		migration := &Migration{
			Version:     version,
			Name:        name,
			UpSQL:       upSQL,
			DownSQL:     downSQL,
			Description: fmt.Sprintf("Migration %d: %s", version, name),
		}

		migrations = append(migrations, migration)
	}

	// Sort migrations by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// ValidateMigrations checks for version conflicts and missing migrations
func ValidateMigrations(migrations []*Migration) error {
	versions := make(map[int]bool)

	for _, migration := range migrations {
		if versions[migration.Version] {
			return fmt.Errorf("duplicate migration version: %d", migration.Version)
		}
		versions[migration.Version] = true
	}

	return nil
}

// ensureSchemaMigrationsTable creates the schema_migrations table if it doesn't exist
func (m *MigrationManager) ensureSchemaMigrationsTable(ctx context.Context) error {
	return m.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			CREATE TABLE IF NOT EXISTS schema_migrations (
				version INTEGER PRIMARY KEY,
				name TEXT NOT NULL,
				applied_at DATETIME NOT NULL,
				description TEXT
			)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		_, err = stmt.Step()
		if err != nil {
			return err
		}

		// Create index
		stmt2, err := tx.Prepare(`
			CREATE INDEX IF NOT EXISTS idx_schema_migrations_version ON schema_migrations(version)
		`)
		if err != nil {
			return err
		}
		defer stmt2.Finalize()

		_, err = stmt2.Step()
		return err
	})
}

// splitSQLStatements splits a multi-statement SQL string into individual statements
func splitSQLStatements(sql string) []string {
	// Simple SQL statement splitter - splits on semicolons not inside quotes
	var statements []string
	var current strings.Builder
	inQuotes := false
	var quoteChar rune

	for i, char := range sql {
		switch char {
		case '\'', '"', '`':
			if !inQuotes {
				inQuotes = true
				quoteChar = char
			} else if char == quoteChar {
				// Check if it's escaped
				if i > 0 && rune(sql[i-1]) != '\\' {
					inQuotes = false
				}
			}
			current.WriteRune(char)
		case ';':
			current.WriteRune(char)
			if !inQuotes {
				stmt := strings.TrimSpace(current.String())
				if stmt != "" {
					statements = append(statements, stmt)
				}
				current.Reset()
			}
		default:
			current.WriteRune(char)
		}
	}

	// Add any remaining content
	if current.Len() > 0 {
		stmt := strings.TrimSpace(current.String())
		if stmt != "" {
			statements = append(statements, stmt)
		}
	}

	return statements
}
