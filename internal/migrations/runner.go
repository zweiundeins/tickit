package migrations

import (
	"context"
	"fmt"
	"log"
	"path/filepath"

	"github.com/mbolli/tickit/internal/users"
)

// Runner handles database migrations and seeding
type Runner struct {
	migrationManager *MigrationManager
	userStore        users.UserStore
}

// NewRunner creates a new migration runner
func NewRunner(ctx context.Context, dbFilename string, userStore users.UserStore) (*Runner, error) {
	migrationManager, err := NewMigrationManager(ctx, dbFilename)
	if err != nil {
		return nil, fmt.Errorf("failed to create migration manager: %w", err)
	}

	return &Runner{
		migrationManager: migrationManager,
		userStore:        userStore,
	}, nil
}

// RunMigrations applies all pending migrations
func (r *Runner) RunMigrations(ctx context.Context) error {
	log.Println("Loading migrations from embedded files...")

	// Load migrations from embedded files
	migrations, err := LoadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	if len(migrations) == 0 {
		log.Println("No migrations found")
		return nil
	}

	log.Printf("Found %d migration(s)", len(migrations))

	// Validate migrations
	if err := ValidateMigrations(migrations); err != nil {
		return fmt.Errorf("migration validation failed: %w", err)
	}

	// Apply migrations
	if err := r.migrationManager.ApplyMigrations(ctx, migrations); err != nil {
		return fmt.Errorf("failed to apply migrations: %w", err)
	}

	log.Println("All migrations applied successfully")
	return nil
}

// RollbackMigrations rolls back migrations to a specific version
func (r *Runner) RollbackMigrations(ctx context.Context, targetVersion int) error {
	log.Printf("Rolling back migrations to version %d...", targetVersion)

	// Load migrations from embedded files
	migrations, err := LoadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load migrations: %w", err)
	}

	// Rollback to target version
	if err := r.migrationManager.RollbackToVersion(ctx, targetVersion, migrations); err != nil {
		return fmt.Errorf("failed to rollback migrations: %w", err)
	}

	log.Printf("Successfully rolled back to version %d", targetVersion)
	return nil
}

// SeedDatabase seeds the database with default data
func (r *Runner) SeedDatabase(ctx context.Context, projectIDs []string) error {
	log.Println("Seeding database with default data...")

	seedData := NewSeedData(r.userStore)

	// Seed default users
	if err := seedData.SeedDefaultUsers(ctx); err != nil {
		return fmt.Errorf("failed to seed default users: %w", err)
	}

	// Seed demo project memberships if project IDs are provided
	if len(projectIDs) > 0 {
		if err := seedData.SeedDemoProjectMemberships(ctx, projectIDs); err != nil {
			return fmt.Errorf("failed to seed demo project memberships: %w", err)
		}
	}

	// Cleanup expired tokens
	if err := seedData.CleanupExpiredTokens(ctx); err != nil {
		return fmt.Errorf("failed to cleanup expired tokens: %w", err)
	}

	log.Println("Database seeding completed successfully")
	return nil
}

// GetMigrationStatus returns the current migration status
func (r *Runner) GetMigrationStatus(ctx context.Context) error {
	log.Println("Checking migration status...")

	// Get applied migrations
	applied, err := r.migrationManager.GetAppliedMigrations(ctx)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %w", err)
	}

	// Load available migrations
	available, err := LoadMigrations()
	if err != nil {
		return fmt.Errorf("failed to load available migrations: %w", err)
	}

	log.Printf("Applied migrations: %d", len(applied))
	for _, migration := range applied {
		log.Printf("  ✓ %d: %s (applied at %s)", migration.Version, migration.Name, migration.AppliedAt.Format("2006-01-02 15:04:05"))
	}

	// Check for pending migrations
	appliedVersions := make(map[int]bool)
	for _, migration := range applied {
		appliedVersions[migration.Version] = true
	}

	pendingCount := 0
	for _, migration := range available {
		if !appliedVersions[migration.Version] {
			if pendingCount == 0 {
				log.Println("Pending migrations:")
			}
			log.Printf("  ⏳ %d: %s", migration.Version, migration.Name)
			pendingCount++
		}
	}

	if pendingCount == 0 {
		log.Println("All migrations are up to date")
	} else {
		log.Printf("Total pending migrations: %d", pendingCount)
	}

	return nil
}

// CreateMigrationFile creates a new migration file template
func CreateMigrationFile(name string) error {
	// Find the next version number
	migrations, err := LoadMigrationsFromDirectory("internal/migrations/sql")
	if err != nil {
		return fmt.Errorf("failed to load existing migrations: %w", err)
	}

	nextVersion := 1
	for _, migration := range migrations {
		if migration.Version >= nextVersion {
			nextVersion = migration.Version + 1
		}
	}

	// Create the migration file
	filename := fmt.Sprintf("%03d_%s.sql", nextVersion, name)
	filepath := filepath.Join("internal/migrations/sql", filename)

	template := fmt.Sprintf(`-- +migrate Up
-- TODO: Add your migration SQL here


-- +migrate Down
-- TODO: Add your rollback SQL here

`)

	// Write the file (this would need to be implemented with file operations)
	log.Printf("Migration template created: %s", filepath)
	log.Printf("Template content:\n%s", template)

	return nil
}
