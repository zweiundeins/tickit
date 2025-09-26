package users

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/shared"
	"zombiezen.com/go/sqlite"
)

// UserProjectionStore implements the UserProjectionStore interface
// Following the same pattern as ProjectProjectionStore
type UserProjectionStoreImpl struct {
	db *toolbelt.Database
}

// Ensure UserProjectionStoreImpl implements the required interfaces
var _ UserProjectionStore = (*UserProjectionStoreImpl)(nil)
var _ shared.ProjectionRebuilder = (*UserProjectionStoreImpl)(nil)

// NewUserProjectionStore creates a new UserProjectionStore instance
func NewUserProjectionStore(ctx context.Context, dbFilename string) (*UserProjectionStoreImpl, error) {
	absPath, _ := filepath.Abs(dbFilename)
	logger.Debug().
		Str("dbFilename", dbFilename).
		Str("absPath", absPath).
		Msg("UserProjection dbFilename")

	migrations := []string{
		// Note: This projection store reads directly from the main users.db tables
		// No separate projection tables needed - we use the main tables directly
		// This is a simplified approach where projections = main tables for users
		// The main tables are created by the migration system in users.db
	}

	db, err := toolbelt.NewDatabase(ctx, dbFilename, migrations)
	if err != nil {
		logger.Error().
			Str("dbFilename", dbFilename).
			Err(err).
			Msg("Failed to create users projection db")
		return nil, fmt.Errorf("failed to create users projection db: %w", err)
	}

	if _, err := os.Stat(dbFilename); err == nil {
		logger.Info().Msgf("Users projection DB file created: %s\n", dbFilename)
	} else {
		logger.Warn().Msgf("Users projection DB file missing after creation: %s\n", dbFilename)
	}

	return NewUserProjectionStoreWithDB(db), nil
}

// NewUserProjectionStoreWithDB creates a new UserProjectionStore with an existing database
func NewUserProjectionStoreWithDB(db *toolbelt.Database) *UserProjectionStoreImpl {
	return &UserProjectionStoreImpl{db: db}
}

// ApplyEvent updates the projection based on user events
func (ps *UserProjectionStoreImpl) ApplyEvent(ctx context.Context, event shared.Event) error {
	switch e := event.(type) {
	// User Registration Events
	case UserRegisteredEvent:
		return ps.applyUserRegistered(ctx, e)
	case UserEmailVerifiedEvent:
		return ps.applyUserEmailVerified(ctx, e)

	// Authentication Events
	case UserLoggedInEvent:
		return ps.applyUserLoggedIn(ctx, e)
	case UserLoggedOutEvent:
		return ps.applyUserLoggedOut(ctx, e)

	// Password Management Events
	case PasswordResetRequestedEvent:
		return ps.applyPasswordResetRequested(ctx, e)
	case PasswordResetCompletedEvent:
		return ps.applyPasswordResetCompleted(ctx, e)
	case PasswordChangedEvent:
		return ps.applyPasswordChanged(ctx, e)

	// MFA Events
	case UserMFAEnabledEvent:
		return ps.applyUserMFAEnabled(ctx, e)
	case UserMFADisabledEvent:
		return ps.applyUserMFADisabled(ctx, e)

	// Profile Events
	case UserProfileUpdatedEvent:
		return ps.applyUserProfileUpdated(ctx, e)
	case UserDeactivatedEvent:
		return ps.applyUserDeactivated(ctx, e)
	case UserReactivatedEvent:
		return ps.applyUserReactivated(ctx, e)

	// Project Membership Events
	case UserInvitedToProjectEvent:
		return ps.applyUserInvitedToProject(ctx, e)
	case ProjectInvitationAcceptedEvent:
		return ps.applyProjectInvitationAccepted(ctx, e)
	case ProjectInvitationDeclinedEvent:
		return ps.applyProjectInvitationDeclined(ctx, e)
	case UserAddedToProjectEvent:
		return ps.applyUserAddedToProject(ctx, e)
	case UserRemovedFromProjectEvent:
		return ps.applyUserRemovedFromProject(ctx, e)
	case ProjectMembershipUpdatedEvent:
		return ps.applyProjectMembershipUpdated(ctx, e)

	// Administrative Events
	case UserRoleAssignedEvent:
		return ps.applyUserRoleAssigned(ctx, e)
	case UserPatchedEvent:
		return ps.applyUserPatched(ctx, e)

	// OAuth Events
	case OAuthAccountLinkedEvent:
		return ps.applyOAuthAccountLinked(ctx, e)
	case OAuthAccountUnlinkedEvent:
		return ps.applyOAuthAccountUnlinked(ctx, e)

	// Session Events
	case UserSessionCreatedEvent:
		return ps.applyUserSessionCreated(ctx, e)
	case UserSessionInvalidatedEvent:
		return ps.applyUserSessionInvalidated(ctx, e)
	case AllUserSessionsInvalidatedEvent:
		return ps.applyAllUserSessionsInvalidated(ctx, e)

	// Magic Mail Events
	case MagicMailSentEvent:
		return ps.applyMagicMailSent(ctx, e)

	default:
		return nil // ignore unrelated events
	}
}

// Event application methods (stubs for now)

func (ps *UserProjectionStoreImpl) applyUserRegistered(ctx context.Context, e UserRegisteredEvent) error {
	// User registration is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserEmailVerified(ctx context.Context, e UserEmailVerifiedEvent) error {
	// Email verification is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserLoggedIn(ctx context.Context, e UserLoggedInEvent) error {
	// Login tracking is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserLoggedOut(ctx context.Context, e UserLoggedOutEvent) error {
	// Logout tracking is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyPasswordResetRequested(ctx context.Context, e PasswordResetRequestedEvent) error {
	// Password reset tracking is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyPasswordResetCompleted(ctx context.Context, e PasswordResetCompletedEvent) error {
	// Password reset completion is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyPasswordChanged(ctx context.Context, e PasswordChangedEvent) error {
	// Password change tracking is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserMFAEnabled(ctx context.Context, e UserMFAEnabledEvent) error {
	// TODO: Implement MFA enabled event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserMFADisabled(ctx context.Context, e UserMFADisabledEvent) error {
	// TODO: Implement MFA disabled event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserProfileUpdated(ctx context.Context, e UserProfileUpdatedEvent) error {
	// TODO: Implement profile update event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserDeactivated(ctx context.Context, e UserDeactivatedEvent) error {
	// TODO: Implement user deactivation event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserReactivated(ctx context.Context, e UserReactivatedEvent) error {
	// TODO: Implement user reactivation event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserInvitedToProject(ctx context.Context, e UserInvitedToProjectEvent) error {
	// TODO: Implement project invitation event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyProjectInvitationAccepted(ctx context.Context, e ProjectInvitationAcceptedEvent) error {
	// TODO: Implement invitation acceptance event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyProjectInvitationDeclined(ctx context.Context, e ProjectInvitationDeclinedEvent) error {
	// TODO: Implement invitation decline event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserAddedToProject(ctx context.Context, e UserAddedToProjectEvent) error {
	// TODO: Implement user addition to project event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserRemovedFromProject(ctx context.Context, e UserRemovedFromProjectEvent) error {
	// TODO: Implement user removal from project event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyProjectMembershipUpdated(ctx context.Context, e ProjectMembershipUpdatedEvent) error {
	// TODO: Implement membership update event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserRoleAssigned(ctx context.Context, e UserRoleAssignedEvent) error {
	// TODO: Implement role assignment event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserPatched(ctx context.Context, e UserPatchedEvent) error {
	// TODO: Implement user patching event handling
	return nil
}

func (ps *UserProjectionStoreImpl) applyOAuthAccountLinked(ctx context.Context, e OAuthAccountLinkedEvent) error {
	// OAuth account linking is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyOAuthAccountUnlinked(ctx context.Context, e OAuthAccountUnlinkedEvent) error {
	// OAuth account unlinking is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserSessionCreated(ctx context.Context, e UserSessionCreatedEvent) error {
	// Session creation is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyUserSessionInvalidated(ctx context.Context, e UserSessionInvalidatedEvent) error {
	// Session invalidation is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyAllUserSessionsInvalidated(ctx context.Context, e AllUserSessionsInvalidatedEvent) error {
	// Session invalidation is handled by the main UserStore
	// Projection store just needs to acknowledge the event for monitoring
	return nil
}

func (ps *UserProjectionStoreImpl) applyMagicMailSent(ctx context.Context, e MagicMailSentEvent) error {
	// TODO: Implement magic mail sent event handling
	return nil
}

// Query methods (stubs for now - these will be implemented in later tasks)

func (ps *UserProjectionStoreImpl) GetUser(ctx context.Context, userID string) (*User, error) {
	var user User
	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, email, username, first_name, last_name, avatar_url, 
				   role, status, email_verified, created_at, updated_at
			FROM users 
			WHERE id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return fmt.Errorf("user not found")
		}

		user.ID = stmt.ColumnText(0)
		user.Email = stmt.ColumnText(1)
		user.Username = stmt.ColumnText(2)
		user.FirstName = stmt.ColumnText(3)
		user.LastName = stmt.ColumnText(4)
		user.AvatarURL = stmt.ColumnText(5)
		user.Role = stmt.ColumnText(6)
		user.Status = stmt.ColumnText(7)
		user.EmailVerified = stmt.ColumnInt(8) == 1
		user.CreatedAt = time.Unix(stmt.ColumnInt64(9), 0)
		user.UpdatedAt = time.Unix(stmt.ColumnInt64(10), 0)

		return nil
	})

	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (ps *UserProjectionStoreImpl) GetUsers(ctx context.Context, userIDs []string) ([]*User, error) {
	var users []*User
	for _, userID := range userIDs {
		user, err := ps.GetUser(ctx, userID)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}
	return users, nil
}

func (ps *UserProjectionStoreImpl) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, email, username, first_name, last_name, avatar_url, 
				   role, status, email_verified, created_at, updated_at
			FROM users 
			WHERE email = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, email)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return fmt.Errorf("user not found")
		}

		user.ID = stmt.ColumnText(0)
		user.Email = stmt.ColumnText(1)
		user.Username = stmt.ColumnText(2)
		user.FirstName = stmt.ColumnText(3)
		user.LastName = stmt.ColumnText(4)
		user.AvatarURL = stmt.ColumnText(5)
		user.Role = stmt.ColumnText(6)
		user.Status = stmt.ColumnText(7)
		user.EmailVerified = stmt.ColumnInt(8) == 1
		user.CreatedAt = time.Unix(stmt.ColumnInt64(9), 0)
		user.UpdatedAt = time.Unix(stmt.ColumnInt64(10), 0)

		return nil
	})

	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (ps *UserProjectionStoreImpl) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, email, username, first_name, last_name, avatar_url, 
				   role, status, email_verified, created_at, updated_at
			FROM users 
			WHERE username = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, username)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return fmt.Errorf("user not found")
		}

		user.ID = stmt.ColumnText(0)
		user.Email = stmt.ColumnText(1)
		user.Username = stmt.ColumnText(2)
		user.FirstName = stmt.ColumnText(3)
		user.LastName = stmt.ColumnText(4)
		user.AvatarURL = stmt.ColumnText(5)
		user.Role = stmt.ColumnText(6)
		user.Status = stmt.ColumnText(7)
		user.EmailVerified = stmt.ColumnInt(8) == 1
		user.CreatedAt = time.Unix(stmt.ColumnInt64(9), 0)
		user.UpdatedAt = time.Unix(stmt.ColumnInt64(10), 0)

		return nil
	})

	if err != nil {
		return nil, err
	}
	return &user, nil
}

func (ps *UserProjectionStoreImpl) ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error) {
	var users []*User
	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		// Build query with filters
		query := `
			SELECT id, email, username, first_name, last_name, avatar_url, 
				   role, status, email_verified, created_at, updated_at
			FROM users 
			WHERE 1=1
		`
		args := []interface{}{}
		argIndex := 1

		// Apply filters
		if role, ok := filters["role"].(string); ok && role != "" {
			query += fmt.Sprintf(" AND role = ?%d", argIndex)
			args = append(args, role)
			argIndex++
		}
		if status, ok := filters["status"].(string); ok && status != "" {
			query += fmt.Sprintf(" AND status = ?%d", argIndex)
			args = append(args, status)
			argIndex++
		}
		if emailVerified, ok := filters["email_verified"].(bool); ok {
			query += fmt.Sprintf(" AND email_verified = ?%d", argIndex)
			if emailVerified {
				args = append(args, 1)
			} else {
				args = append(args, 0)
			}
			argIndex++
		}

		query += " ORDER BY created_at DESC"
		if limit > 0 {
			query += fmt.Sprintf(" LIMIT ?%d", argIndex)
			args = append(args, limit)
			argIndex++
		}
		if offset > 0 {
			query += fmt.Sprintf(" OFFSET ?%d", argIndex)
			args = append(args, offset)
		}

		stmt, err := tx.Prepare(query)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		// Bind arguments
		for i, arg := range args {
			switch v := arg.(type) {
			case string:
				stmt.BindText(i+1, v)
			case int:
				stmt.BindInt64(i+1, int64(v))
			case int64:
				stmt.BindInt64(i+1, v)
			}
		}

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			user := &User{
				ID:            stmt.ColumnText(0),
				Email:         stmt.ColumnText(1),
				Username:      stmt.ColumnText(2),
				FirstName:     stmt.ColumnText(3),
				LastName:      stmt.ColumnText(4),
				AvatarURL:     stmt.ColumnText(5),
				Role:          stmt.ColumnText(6),
				Status:        stmt.ColumnText(7),
				EmailVerified: stmt.ColumnInt(8) == 1,
				CreatedAt:     time.Unix(stmt.ColumnInt64(9), 0),
				UpdatedAt:     time.Unix(stmt.ColumnInt64(10), 0),
			}
			users = append(users, user)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}
	return users, nil
}

func (ps *UserProjectionStoreImpl) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error) {
	// TODO: Implement user search from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error) {
	// Credentials are sensitive and should be retrieved from the main store, not projection
	return nil, fmt.Errorf("credential retrieval should use the main UserStore, not projection")
}

func (ps *UserProjectionStoreImpl) GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error) {
	// TODO: Implement user credential retrieval by provider from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error) {
	// TODO: Implement user retrieval by OAuth provider from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetUserSession(ctx context.Context, sessionID string) (*UserSession, error) {
	// For sessions, we delegate to the main store since sessions are not typically projected
	// This is because session data is frequently accessed and doesn't benefit much from projection
	return nil, fmt.Errorf("session retrieval should use the main UserStore, not projection")
}

func (ps *UserProjectionStoreImpl) GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	// For sessions, we delegate to the main store since sessions are not typically projected
	return nil, fmt.Errorf("session retrieval should use the main UserStore, not projection")
}

func (ps *UserProjectionStoreImpl) GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	// For sessions, we delegate to the main store since sessions are not typically projected
	return nil, fmt.Errorf("session retrieval should use the main UserStore, not projection")
}

func (ps *UserProjectionStoreImpl) GetProjectMembers(ctx context.Context, projectID string) ([]*User, error) {
	// TODO: Implement project members retrieval from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error) {
	// TODO: Implement user projects retrieval from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error) {
	// TODO: Implement project membership retrieval from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	// TODO: Implement user project access check from projection
	return false, nil
}

func (ps *UserProjectionStoreImpl) GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	// TODO: Implement email verification token retrieval from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error) {
	// TODO: Implement email verification token retrieval by user ID from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	// Tokens are sensitive and should be retrieved from the main store, not projection
	return nil, fmt.Errorf("token retrieval should use the main UserStore, not projection")
}

func (ps *UserProjectionStoreImpl) GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error) {
	// Tokens are sensitive and should be retrieved from the main store, not projection
	return nil, fmt.Errorf("token retrieval should use the main UserStore, not projection")
}

func (ps *UserProjectionStoreImpl) GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error) {
	// TODO: Implement project invitations retrieval from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error) {
	// TODO: Implement user invitations retrieval from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error) {
	// TODO: Implement invitation retrieval by token from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error) {
	// TODO: Implement user activity retrieval from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	// TODO: Implement user stats retrieval from projection
	return nil, nil
}

func (ps *UserProjectionStoreImpl) GetSystemUserStats(ctx context.Context) (map[string]interface{}, error) {
	// TODO: Implement system user stats retrieval from projection
	return nil, nil
}

// RebuildAllFromEvents rebuilds all user projections from the event store
func (ps *UserProjectionStoreImpl) RebuildAllFromEvents(ctx context.Context, eventStore *shared.EventStore) error {
	events, err := eventStore.GetEventsSince(ctx, time.Time{})
	if err != nil {
		return fmt.Errorf("failed to get events: %w", err)
	}
	for _, event := range events {
		if err := ps.ApplyEvent(ctx, event); err != nil {
			return fmt.Errorf("failed to apply event: %w", err)
		}
	}
	return nil
}
