package migrations

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/users"
)

// SeedData represents seed data for the database
type SeedData struct {
	userStore users.UserStore
}

// NewSeedData creates a new seed data instance
func NewSeedData(userStore users.UserStore) *SeedData {
	return &SeedData{
		userStore: userStore,
	}
}

// SeedDefaultUsers creates default admin user and roles
func (s *SeedData) SeedDefaultUsers(ctx context.Context) error {
	log.Println("Seeding default users...")

	// Check if admin user already exists
	adminUser, err := s.userStore.GetUserByEmail(ctx, "admin@tickit.local")
	if err != nil {
		return fmt.Errorf("failed to check for existing admin user: %w", err)
	}

	if adminUser != nil {
		log.Println("Admin user already exists, skipping seed")
		return nil
	}

	// Create default admin user
	now := time.Now()
	adminID := uuid.New().String()

	admin := &users.User{
		ID:            adminID,
		Email:         "admin@tickit.local",
		Username:      "admin",
		FirstName:     "System",
		LastName:      "Administrator",
		Role:          "Admin",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// Create admin user
	if err := s.userStore.CreateUser(ctx, admin); err != nil {
		return fmt.Errorf("failed to create admin user: %w", err)
	}

	// Create default password credential for admin
	passwordHasher := users.NewDefaultBcryptPasswordHasher()
	hashedPassword, err := passwordHasher.HashPassword("admin123")
	if err != nil {
		return fmt.Errorf("failed to hash admin password: %w", err)
	}

	adminCredential := &users.UserCredential{
		ID:             uuid.New().String(),
		UserID:         adminID,
		Type:           "password",
		Provider:       "local",
		CredentialData: hashedPassword,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.userStore.CreateUserCredential(ctx, adminCredential); err != nil {
		return fmt.Errorf("failed to create admin credential: %w", err)
	}

	log.Printf("Created admin user: %s (password: admin123)", admin.Email)

	// Create demo project manager user
	pmID := uuid.New().String()
	projectManager := &users.User{
		ID:            pmID,
		Email:         "pm@tickit.local",
		Username:      "projectmanager",
		FirstName:     "Project",
		LastName:      "Manager",
		Role:          "ProjectManager",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.userStore.CreateUser(ctx, projectManager); err != nil {
		return fmt.Errorf("failed to create project manager user: %w", err)
	}

	// Create password credential for project manager
	pmHashedPassword, err := passwordHasher.HashPassword("pm123")
	if err != nil {
		return fmt.Errorf("failed to hash project manager password: %w", err)
	}

	pmCredential := &users.UserCredential{
		ID:             uuid.New().String(),
		UserID:         pmID,
		Type:           "password",
		Provider:       "local",
		CredentialData: pmHashedPassword,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.userStore.CreateUserCredential(ctx, pmCredential); err != nil {
		return fmt.Errorf("failed to create project manager credential: %w", err)
	}

	log.Printf("Created project manager user: %s (password: pm123)", projectManager.Email)

	// Create demo developer user
	devID := uuid.New().String()
	developer := &users.User{
		ID:            devID,
		Email:         "dev@tickit.local",
		Username:      "developer",
		FirstName:     "Demo",
		LastName:      "Developer",
		Role:          "Developer",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.userStore.CreateUser(ctx, developer); err != nil {
		return fmt.Errorf("failed to create developer user: %w", err)
	}

	// Create password credential for developer
	devHashedPassword, err := passwordHasher.HashPassword("dev123")
	if err != nil {
		return fmt.Errorf("failed to hash developer password: %w", err)
	}

	devCredential := &users.UserCredential{
		ID:             uuid.New().String(),
		UserID:         devID,
		Type:           "password",
		Provider:       "local",
		CredentialData: devHashedPassword,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.userStore.CreateUserCredential(ctx, devCredential); err != nil {
		return fmt.Errorf("failed to create developer credential: %w", err)
	}

	log.Printf("Created developer user: %s (password: dev123)", developer.Email)

	// Create demo viewer user
	viewerID := uuid.New().String()
	viewer := &users.User{
		ID:            viewerID,
		Email:         "viewer@tickit.local",
		Username:      "viewer",
		FirstName:     "Demo",
		LastName:      "Viewer",
		Role:          "Viewer",
		Status:        "Active",
		EmailVerified: true,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.userStore.CreateUser(ctx, viewer); err != nil {
		return fmt.Errorf("failed to create viewer user: %w", err)
	}

	// Create password credential for viewer
	viewerHashedPassword, err := passwordHasher.HashPassword("viewer123")
	if err != nil {
		return fmt.Errorf("failed to hash viewer password: %w", err)
	}

	viewerCredential := &users.UserCredential{
		ID:             uuid.New().String(),
		UserID:         viewerID,
		Type:           "password",
		Provider:       "local",
		CredentialData: viewerHashedPassword,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.userStore.CreateUserCredential(ctx, viewerCredential); err != nil {
		return fmt.Errorf("failed to create viewer credential: %w", err)
	}

	log.Printf("Created viewer user: %s (password: viewer123)", viewer.Email)

	log.Println("Default users seeded successfully!")
	return nil
}

// SeedDemoProjectMemberships creates demo project memberships
func (s *SeedData) SeedDemoProjectMemberships(ctx context.Context, projectIDs []string) error {
	if len(projectIDs) == 0 {
		log.Println("No project IDs provided, skipping project membership seeding")
		return nil
	}

	log.Println("Seeding demo project memberships...")

	// Get the demo users
	admin, err := s.userStore.GetUserByEmail(ctx, "admin@tickit.local")
	if err != nil {
		return fmt.Errorf("failed to get admin user: %w", err)
	}

	pm, err := s.userStore.GetUserByEmail(ctx, "pm@tickit.local")
	if err != nil {
		return fmt.Errorf("failed to get project manager user: %w", err)
	}

	dev, err := s.userStore.GetUserByEmail(ctx, "dev@tickit.local")
	if err != nil {
		return fmt.Errorf("failed to get developer user: %w", err)
	}

	viewer, err := s.userStore.GetUserByEmail(ctx, "viewer@tickit.local")
	if err != nil {
		return fmt.Errorf("failed to get viewer user: %w", err)
	}

	now := time.Now()

	// Add users to the first project (if available)
	if len(projectIDs) > 0 {
		projectID := projectIDs[0]

		// Admin as ProjectAdmin
		adminMembership := &users.ProjectMembership{
			ID:        uuid.New().String(),
			UserID:    admin.ID,
			ProjectID: projectID,
			Role:      "ProjectAdmin",
			CreatedAt: now,
			CreatedBy: admin.ID,
		}

		if err := s.userStore.CreateProjectMembership(ctx, adminMembership); err != nil {
			return fmt.Errorf("failed to create admin project membership: %w", err)
		}

		// PM as ProjectAdmin
		pmMembership := &users.ProjectMembership{
			ID:        uuid.New().String(),
			UserID:    pm.ID,
			ProjectID: projectID,
			Role:      "ProjectAdmin",
			CreatedAt: now,
			CreatedBy: admin.ID,
		}

		if err := s.userStore.CreateProjectMembership(ctx, pmMembership); err != nil {
			return fmt.Errorf("failed to create PM project membership: %w", err)
		}

		// Developer as Developer
		devMembership := &users.ProjectMembership{
			ID:        uuid.New().String(),
			UserID:    dev.ID,
			ProjectID: projectID,
			Role:      "Developer",
			CreatedAt: now,
			CreatedBy: admin.ID,
		}

		if err := s.userStore.CreateProjectMembership(ctx, devMembership); err != nil {
			return fmt.Errorf("failed to create developer project membership: %w", err)
		}

		// Viewer as Viewer
		viewerMembership := &users.ProjectMembership{
			ID:        uuid.New().String(),
			UserID:    viewer.ID,
			ProjectID: projectID,
			Role:      "Viewer",
			CreatedAt: now,
			CreatedBy: admin.ID,
		}

		if err := s.userStore.CreateProjectMembership(ctx, viewerMembership); err != nil {
			return fmt.Errorf("failed to create viewer project membership: %w", err)
		}

		log.Printf("Added demo users to project %s", projectID)
	}

	log.Println("Demo project memberships seeded successfully!")
	return nil
}

// CleanupExpiredTokens removes expired tokens from the database
func (s *SeedData) CleanupExpiredTokens(ctx context.Context) error {
	log.Println("Cleaning up expired tokens...")

	// This would need to be implemented in the user store
	// For now, we'll just log that cleanup would happen here
	log.Println("Token cleanup completed (implementation pending in user store)")

	return nil
}
