package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/brianvoe/gofakeit/v6"
	"github.com/delaneyj/toolbelt/embeddednats"
	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/migrations"
	"github.com/mbolli/tickit/internal/users"
)

func main() {
	purgeData() // Delete DB files before initializing CQRS/event store
	ctx := context.Background()

	// Run database migrations first
	fmt.Println("Running database migrations...")
	if err := runMigrations(ctx); err != nil {
		log.Fatalf("Failed to run migrations: %v", err)
	}

	fmt.Println("Starting embedded NATS and wiring CQRS...")
	ns, err := embeddednats.New(ctx)
	if err != nil {
		log.Fatalf("Failed to start embedded NATS: %v", err)
	}
	ns.WaitForServer()
	cqrs, err := bootstrap.WireCQRS(ctx, ns)
	if err != nil {
		log.Fatalf("Failed to wire CQRS: %v", err)
	}

	// Seed user data first so users exist for issue assignments
	if err := seedUserData(ctx, []string{}); err != nil {
		log.Fatalf("Failed to seed user data: %v", err)
	}

	// Get the seeded user IDs for issue assignments
	userIDs, err := getSeededUserIDs(ctx)
	if err != nil {
		log.Fatalf("Failed to get seeded user IDs: %v", err)
	}

	// Seed projects and issues after users exist
	_ = seedProjectsAndIssues(ctx, cqrs, userIDs)
}

func purgeData() {
	// Just delete the DB files for a true reset
	files := []string{
		"data/projects.db", "data/issues.db", "data/events.db", "data/notifications.db",
		"data/projects.db-shm", "data/projects.db-wal",
		"data/issues.db-shm", "data/issues.db-wal",
		"data/events.db-shm", "data/events.db-wal",
		"data/notifications.db-shm", "data/notifications.db-wal",
	}
	for _, f := range files {
		if err := os.Remove(f); err == nil {
			fmt.Printf("Deleted %s\n", f)
		}
	}
}

func seedProjectsAndIssues(ctx context.Context, cqrs *bootstrap.CQRSDeps, userIDs []string) []string {
	gofakeit.Seed(time.Now().UnixNano())
	projectNames := []string{
		gofakeit.Company(),
		gofakeit.Company(),
		gofakeit.Company(),
		gofakeit.Company(),
		gofakeit.Company(),
	}
	projectIDs := []string{}
	for _, name := range projectNames {
		slug := gofakeit.LetterN(uint(gofakeit.Number(2, 6)))
		slug = strings.ToUpper(slug)
		desc := gofakeit.BS()
		id, err := cqrs.ProjectService.CreateProject(ctx, name, slug, desc)
		if err != nil {
			log.Fatalf("Failed to create project: %v", err)
		}
		projectIDs = append(projectIDs, id)
		fmt.Printf("Created project: %s (%s)\n", name, slug)

		// Create project memberships immediately after creating the project
		if err := createProjectMemberships(ctx, id, userIDs); err != nil {
			log.Fatalf("Failed to create project memberships for project %s: %v", id, err)
		}
	}

	for _, projectID := range projectIDs {
		// Use actual user IDs from seeded users instead of random names
		for n := 1; n <= 20; n++ {
			title := gofakeit.HipsterSentence(4)
			desc := gofakeit.HipsterParagraph(1, 2, 8, ". ")
			priority := gofakeit.RandomString([]string{"Low", "Medium", "High", "Critical"})
			assignee := ""
			reporter := ""
			if len(userIDs) > 0 {
				assignee = gofakeit.RandomString(userIDs)
				reporter = gofakeit.RandomString(userIDs)
			}
			err := error(nil)
			_, err = cqrs.IssueService.CreateIssue(ctx, title, desc, assignee, reporter, projectID, priority)
			if err != nil {
				log.Fatalf("Failed to create issue: %v", err)
			}
			fmt.Printf("Created issue for project %s: %s\n", projectID, title)
		}
	}
	fmt.Println("Projects and issues seeding complete!")

	// Log the existence of DB files
	dbFiles := []string{
		"data/projects.db", "data/issues.db", "data/events.db", "data/notifications.db", "data/users.db",
	}
	for _, f := range dbFiles {
		if _, err := os.Stat(f); err == nil {
			fmt.Printf("[INFO] DB file exists: %s\n", f)
		} else {
			fmt.Printf("[WARN] DB file missing: %s\n", f)
		}
	}

	return projectIDs
}

// runMigrations runs database migrations for the user management system
func runMigrations(ctx context.Context) error {
	// Create user store to run migrations
	userStore, err := users.NewUserStore(ctx, "data/users.db")
	if err != nil {
		return fmt.Errorf("failed to create user store: %w", err)
	}

	// Create migration runner
	runner, err := migrations.NewRunner(ctx, "data/users.db", userStore)
	if err != nil {
		return fmt.Errorf("failed to create migration runner: %w", err)
	}

	// Run migrations
	if err := runner.RunMigrations(ctx); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	return nil
}

// seedUserData seeds the database with default users and project memberships
func seedUserData(ctx context.Context, projectIDs []string) error {
	// Create user store
	userStore, err := users.NewUserStore(ctx, "data/users.db")
	if err != nil {
		return fmt.Errorf("failed to create user store: %w", err)
	}

	// Create migration runner for seeding
	runner, err := migrations.NewRunner(ctx, "data/users.db", userStore)
	if err != nil {
		return fmt.Errorf("failed to create migration runner: %w", err)
	}

	// Seed database
	if err := runner.SeedDatabase(ctx, projectIDs); err != nil {
		return fmt.Errorf("failed to seed database: %w", err)
	}

	return nil
}

// getSeededUserIDs retrieves the IDs of the seeded users
func getSeededUserIDs(ctx context.Context) ([]string, error) {
	// Create user store
	userStore, err := users.NewUserStore(ctx, "data/users.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create user store: %w", err)
	}

	// Get the seeded users by their known usernames
	usernames := []string{"admin", "projectmanager", "developer", "viewer"}
	userIDs := []string{}

	for _, username := range usernames {
		user, err := userStore.GetUserByUsername(ctx, username)
		if err != nil {
			return nil, fmt.Errorf("failed to get user %s: %w", username, err)
		}
		if user != nil {
			userIDs = append(userIDs, user.ID)
		}
	}

	return userIDs, nil
}

// createProjectMemberships creates memberships for all seeded users in a specific project
func createProjectMemberships(ctx context.Context, projectID string, userIDs []string) error {
	// Create user store
	userStore, err := users.NewUserStore(ctx, "data/users.db")
	if err != nil {
		return fmt.Errorf("failed to create user store: %w", err)
	}

	now := time.Now()

	// Add all users to the project with appropriate roles
	for _, userID := range userIDs {
		user, err := userStore.GetUser(ctx, userID)
		if err != nil {
			return fmt.Errorf("failed to get user %s: %w", userID, err)
		}
		if user == nil {
			continue
		}

		// Assign roles based on user type
		var projectRole string
		switch user.Username {
		case "admin":
			projectRole = "ProjectAdmin"
		case "projectmanager":
			projectRole = "ProjectAdmin"
		case "developer":
			projectRole = "Developer"
		case "viewer":
			projectRole = "Viewer"
		default:
			projectRole = "Developer" // Default role
		}

		// Create project membership
		membership := &users.ProjectMembership{
			ID:        uuid.New().String(),
			UserID:    userID,
			ProjectID: projectID,
			Role:      projectRole,
			CreatedAt: now,
			CreatedBy: userIDs[0], // Use first user (admin) as creator
		}

		if err := userStore.CreateProjectMembership(ctx, membership); err != nil {
			return fmt.Errorf("failed to create project membership for user %s: %w", user.Username, err)
		}

		fmt.Printf("Added user %s to project %s as %s\n", user.Username, projectID, projectRole)
	}

	return nil
}

// seedProjectMemberships creates project memberships for the seeded users
func seedProjectMemberships(ctx context.Context, projectIDs []string) error {
	// Create user store
	userStore, err := users.NewUserStore(ctx, "data/users.db")
	if err != nil {
		return fmt.Errorf("failed to create user store: %w", err)
	}

	// Create migration runner for seeding
	runner, err := migrations.NewRunner(ctx, "data/users.db", userStore)
	if err != nil {
		return fmt.Errorf("failed to create migration runner: %w", err)
	}

	// Seed project memberships with actual project IDs
	if err := runner.SeedDatabase(ctx, projectIDs); err != nil {
		return fmt.Errorf("failed to seed project memberships: %w", err)
	}

	return nil
}
