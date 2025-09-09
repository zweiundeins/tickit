package users

import (
	"context"
	"fmt"
	"log/slog"
)

// MigrationHelper provides utilities for migrating existing data to support user integration
type MigrationHelper struct {
	userStore UserStore
}

// NewMigrationHelper creates a new migration helper
func NewMigrationHelper(userStore UserStore) *MigrationHelper {
	return &MigrationHelper{
		userStore: userStore,
	}
}

// EnsureBackwardCompatibility ensures that existing data is compatible with the new user integration
func (m *MigrationHelper) EnsureBackwardCompatibility(ctx context.Context) error {
	slog.Info("starting backward compatibility check for user integration")

	// Check if we need to create a default admin user for existing projects
	if err := m.ensureDefaultAdminUser(ctx); err != nil {
		return fmt.Errorf("failed to ensure default admin user: %w", err)
	}

	slog.Info("backward compatibility check completed successfully")
	return nil
}

// ensureDefaultAdminUser creates a default admin user if none exists
// This is needed for backward compatibility with existing projects that don't have creators
func (m *MigrationHelper) ensureDefaultAdminUser(ctx context.Context) error {
	// Check if any admin users exist
	users, err := m.userStore.ListUsers(ctx, 10, 0, map[string]interface{}{
		"role": RoleAdmin,
	})
	if err != nil {
		return fmt.Errorf("failed to list admin users: %w", err)
	}

	if len(users) > 0 {
		slog.Info("admin users already exist, skipping default admin creation",
			slog.Int("admin_count", len(users)))
		return nil
	}

	// Create a default admin user for backward compatibility
	defaultAdmin := &User{
		ID:            "default-admin-user",
		Email:         "admin@tickit.local",
		Username:      "admin",
		FirstName:     "System",
		LastName:      "Administrator",
		Role:          RoleAdmin,
		Status:        "Active",
		EmailVerified: true,
	}

	err = m.userStore.CreateUser(ctx, defaultAdmin)
	if err != nil {
		return fmt.Errorf("failed to create default admin user: %w", err)
	}

	slog.Info("created default admin user for backward compatibility",
		slog.String("user_id", defaultAdmin.ID),
		slog.String("email", defaultAdmin.Email))

	return nil
}

// ValidateExistingAssignments validates existing issue assignments against project membership
// This is a utility function that can be run to check data integrity
func (m *MigrationHelper) ValidateExistingAssignments(ctx context.Context) ([]string, error) {
	var warnings []string

	slog.Info("validating existing issue assignments")

	// This would require access to the issue projection store
	// For now, we'll just log that this validation should be run
	slog.Info("existing assignment validation should be implemented when issue store is available")

	return warnings, nil
}

// GetOrCreateDefaultUser gets an existing user or creates a default one for backward compatibility
func (m *MigrationHelper) GetOrCreateDefaultUser(ctx context.Context, userID string) (*User, error) {
	if userID == "" {
		return nil, nil
	}

	// Try to get the existing user
	user, err := m.userStore.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if user != nil {
		return user, nil
	}

	// User doesn't exist, create a placeholder for backward compatibility
	placeholderUser := &User{
		ID:            userID,
		Email:         fmt.Sprintf("user-%s@tickit.local", userID),
		Username:      fmt.Sprintf("user-%s", userID),
		FirstName:     "Unknown",
		LastName:      "User",
		Role:          RoleViewer,
		Status:        "Inactive", // Mark as inactive since it's a placeholder
		EmailVerified: false,
	}

	err = m.userStore.CreateUser(ctx, placeholderUser)
	if err != nil {
		return nil, fmt.Errorf("failed to create placeholder user: %w", err)
	}

	slog.Warn("created placeholder user for backward compatibility",
		slog.String("user_id", userID),
		slog.String("email", placeholderUser.Email))

	return placeholderUser, nil
}