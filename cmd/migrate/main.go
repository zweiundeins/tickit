package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/mbolli/tickit/internal/migrations"
	"github.com/mbolli/tickit/internal/users"
)

func main() {
	var (
		dbFile     = flag.String("db", "data/users.db", "Database file path")
		action     = flag.String("action", "up", "Migration action: up, down, status, create, seed, reset")
		version    = flag.String("version", "", "Target version for rollback")
		name       = flag.String("name", "", "Migration name for create action")
		projectIDs = flag.String("projects", "", "Comma-separated project IDs for seeding (optional)")
	)
	flag.Parse()

	ctx := context.Background()

	switch *action {
	case "up":
		if err := runMigrationsUp(ctx, *dbFile); err != nil {
			log.Fatalf("Failed to run migrations: %v", err)
		}
	case "down":
		if *version == "" {
			log.Fatal("Version is required for rollback")
		}
		targetVersion, err := strconv.Atoi(*version)
		if err != nil {
			log.Fatalf("Invalid version number: %v", err)
		}
		if err := runMigrationsDown(ctx, *dbFile, targetVersion); err != nil {
			log.Fatalf("Failed to rollback migrations: %v", err)
		}
	case "status":
		if err := showMigrationStatus(ctx, *dbFile); err != nil {
			log.Fatalf("Failed to show migration status: %v", err)
		}
	case "create":
		if *name == "" {
			log.Fatal("Migration name is required for create action")
		}
		if err := migrations.CreateMigrationFile(*name); err != nil {
			log.Fatalf("Failed to create migration file: %v", err)
		}
	case "seed":
		if err := seedDatabase(ctx, *dbFile, *projectIDs); err != nil {
			log.Fatalf("Failed to seed database: %v", err)
		}
	case "reset":
		if err := resetAndSeedDatabase(ctx, *dbFile, *projectIDs); err != nil {
			log.Fatalf("Failed to reset and seed database: %v", err)
		}
	default:
		log.Fatalf("Unknown action: %s. Use: up, down, status, create, seed, reset", *action)
	}
}

func runMigrationsUp(ctx context.Context, dbFile string) error {
	// Create user store
	userStore, err := users.NewUserStore(ctx, dbFile)
	if err != nil {
		return fmt.Errorf("failed to create user store: %w", err)
	}

	// Create migration runner
	runner, err := migrations.NewRunner(ctx, dbFile, userStore)
	if err != nil {
		return fmt.Errorf("failed to create migration runner: %w", err)
	}

	// Run migrations
	return runner.RunMigrations(ctx)
}

func runMigrationsDown(ctx context.Context, dbFile string, targetVersion int) error {
	// Create user store
	userStore, err := users.NewUserStore(ctx, dbFile)
	if err != nil {
		return fmt.Errorf("failed to create user store: %w", err)
	}

	// Create migration runner
	runner, err := migrations.NewRunner(ctx, dbFile, userStore)
	if err != nil {
		return fmt.Errorf("failed to create migration runner: %w", err)
	}

	// Rollback migrations
	return runner.RollbackMigrations(ctx, targetVersion)
}

func showMigrationStatus(ctx context.Context, dbFile string) error {
	// Create user store
	userStore, err := users.NewUserStore(ctx, dbFile)
	if err != nil {
		return fmt.Errorf("failed to create user store: %w", err)
	}

	// Create migration runner
	runner, err := migrations.NewRunner(ctx, dbFile, userStore)
	if err != nil {
		return fmt.Errorf("failed to create migration runner: %w", err)
	}

	// Show status
	return runner.GetMigrationStatus(ctx)
}

// seedDatabase seeds the database with default users and project memberships
func seedDatabase(ctx context.Context, dbFile string, projectIDsStr string) error {
	log.Println("Seeding database with default data...")

	// Create user store
	userStore, err := users.NewUserStore(ctx, dbFile)
	if err != nil {
		return fmt.Errorf("failed to create user store: %w", err)
	}

	// Create migration runner
	runner, err := migrations.NewRunner(ctx, dbFile, userStore)
	if err != nil {
		return fmt.Errorf("failed to create migration runner: %w", err)
	}

	// Parse project IDs if provided
	var projectIDs []string
	if projectIDsStr != "" {
		projectIDs = strings.Split(strings.TrimSpace(projectIDsStr), ",")
		for i, id := range projectIDs {
			projectIDs[i] = strings.TrimSpace(id)
		}
	}

	// Seed database
	if err := runner.SeedDatabase(ctx, projectIDs); err != nil {
		return fmt.Errorf("failed to seed database: %w", err)
	}

	log.Println("Database seeding completed successfully!")
	return nil
}

// resetAndSeedDatabase removes existing database files, runs migrations, and seeds data
func resetAndSeedDatabase(ctx context.Context, dbFile string, projectIDsStr string) error {
	log.Println("Resetting database (removing existing files)...")

	// Remove existing database files
	dbFiles := []string{
		dbFile,
		dbFile + "-shm",
		dbFile + "-wal",
		"data/projects.db",
		"data/projects.db-shm",
		"data/projects.db-wal",
		"data/issues.db",
		"data/issues.db-shm",
		"data/issues.db-wal",
		"data/events.db",
		"data/events.db-shm",
		"data/events.db-wal",
		"data/notifications.db",
		"data/notifications.db-shm",
		"data/notifications.db-wal",
	}

	for _, file := range dbFiles {
		if err := os.Remove(file); err == nil {
			log.Printf("Removed %s", file)
		}
	}

	// Run migrations
	log.Println("Running migrations...")
	if err := runMigrationsUp(ctx, dbFile); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	// Seed database
	log.Println("Seeding database...")
	if err := seedDatabase(ctx, dbFile, projectIDsStr); err != nil {
		return fmt.Errorf("failed to seed database: %w", err)
	}

	log.Println("Database reset and seeding completed successfully!")
	return nil
}
