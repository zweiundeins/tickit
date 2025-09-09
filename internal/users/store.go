package users

import (
	"context"
	"fmt"
	"time"

	"github.com/delaneyj/toolbelt"
	"zombiezen.com/go/sqlite"
)

// UserStoreImpl implements the UserStore interface using direct database storage
// This follows the design decision to use traditional database storage instead of event sourcing
// for user data due to security and privacy requirements
type UserStoreImpl struct {
	db *toolbelt.Database
}

// UserStoreImpl will implement the UserStore interface when it's defined

// NewUserStore creates a new UserStore instance with database migrations
func NewUserStore(ctx context.Context, dbFilename string) (*UserStoreImpl, error) {
	migrations := []string{
		// Users table
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			username TEXT NOT NULL,
			first_name TEXT,
			last_name TEXT,
			avatar_url TEXT,
			role TEXT NOT NULL DEFAULT 'Viewer',
			status TEXT NOT NULL DEFAULT 'Pending',
			email_verified BOOLEAN DEFAULT FALSE,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			last_login_at DATETIME
		)`,
		// User credentials (passwords, OAuth, WebAuthn)
		`CREATE TABLE IF NOT EXISTS user_credentials (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			type TEXT NOT NULL, -- 'password', 'oauth', 'webauthn'
			provider TEXT NOT NULL, -- 'local', 'github', 'google', etc.
			provider_id TEXT, -- External provider user ID
			credential_data TEXT NOT NULL, -- Encrypted/hashed data
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		// User sessions
		`CREATE TABLE IF NOT EXISTS user_sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			session_id TEXT NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			ip_address TEXT,
			user_agent TEXT,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		// Project memberships
		`CREATE TABLE IF NOT EXISTS project_memberships (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			project_id TEXT NOT NULL,
			role TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			created_by TEXT NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id),
			UNIQUE(user_id, project_id)
		)`,
		// Email verification tokens
		`CREATE TABLE IF NOT EXISTS email_verification_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		// Password reset tokens
		`CREATE TABLE IF NOT EXISTS password_reset_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			used_at DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		// Magic mail login tokens (development only)
		`CREATE TABLE IF NOT EXISTS magic_mail_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			used_at DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id)
		)`,
		// User invitations
		`CREATE TABLE IF NOT EXISTS user_invitations (
			id TEXT PRIMARY KEY,
			project_id TEXT NOT NULL,
			email TEXT NOT NULL,
			role TEXT NOT NULL,
			token TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME NOT NULL,
			created_by TEXT NOT NULL,
			accepted_at DATETIME,
			declined_at DATETIME
		)`,
		// Indexes for performance
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
		`CREATE INDEX IF NOT EXISTS idx_users_username ON users(username)`,
		`CREATE INDEX IF NOT EXISTS idx_user_credentials_user_id ON user_credentials(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_credentials_provider ON user_credentials(provider, provider_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_sessions_user_id ON user_sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_user_sessions_session_id ON user_sessions(session_id)`,
		`CREATE INDEX IF NOT EXISTS idx_project_memberships_user_id ON project_memberships(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_project_memberships_project_id ON project_memberships(project_id)`,
		`CREATE INDEX IF NOT EXISTS idx_email_verification_tokens_token ON email_verification_tokens(token)`,
		`CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_token ON password_reset_tokens(token)`,
		`CREATE INDEX IF NOT EXISTS idx_magic_mail_tokens_token ON magic_mail_tokens(token)`,
		`CREATE INDEX IF NOT EXISTS idx_user_invitations_token ON user_invitations(token)`,
		`CREATE INDEX IF NOT EXISTS idx_user_invitations_email ON user_invitations(email)`,
	}

	db, err := toolbelt.NewDatabase(ctx, dbFilename, migrations)
	if err != nil {
		return nil, fmt.Errorf("failed to create user store database: %w", err)
	}

	return &UserStoreImpl{db: db}, nil
}

// NewUserStoreWithDB creates a new UserStore with an existing database connection
func NewUserStoreWithDB(db *toolbelt.Database) *UserStoreImpl {
	return &UserStoreImpl{db: db}
}

// User CRUD operations

func (s *UserStoreImpl) GetUser(ctx context.Context, userID string) (*User, error) {
	var user User
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, email, username, first_name, last_name, avatar_url, role, status,
				   email_verified, created_at, updated_at, last_login_at
			FROM users WHERE id = ?
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
			return nil // not found
		}

		user.ID = stmt.ColumnText(0)
		user.Email = stmt.ColumnText(1)
		user.Username = stmt.ColumnText(2)
		user.FirstName = stmt.ColumnText(3)
		user.LastName = stmt.ColumnText(4)
		user.AvatarURL = stmt.ColumnText(5)
		user.Role = stmt.ColumnText(6)
		user.Status = stmt.ColumnText(7)
		user.EmailVerified = stmt.ColumnBool(8)
		user.CreatedAt = time.Unix(stmt.ColumnInt64(9), 0)
		user.UpdatedAt = time.Unix(stmt.ColumnInt64(10), 0)
		if !stmt.ColumnIsNull(11) {
			lastLogin := time.Unix(stmt.ColumnInt64(11), 0)
			user.LastLoginAt = &lastLogin
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	if user.ID == "" {
		return nil, nil // not found
	}
	return &user, nil
}

func (s *UserStoreImpl) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var user User
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, email, username, first_name, last_name, avatar_url, role, status,
				   email_verified, created_at, updated_at, last_login_at
			FROM users WHERE email = ?
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
			return nil // not found
		}

		user.ID = stmt.ColumnText(0)
		user.Email = stmt.ColumnText(1)
		user.Username = stmt.ColumnText(2)
		user.FirstName = stmt.ColumnText(3)
		user.LastName = stmt.ColumnText(4)
		user.AvatarURL = stmt.ColumnText(5)
		user.Role = stmt.ColumnText(6)
		user.Status = stmt.ColumnText(7)
		user.EmailVerified = stmt.ColumnBool(8)
		user.CreatedAt = time.Unix(stmt.ColumnInt64(9), 0)
		user.UpdatedAt = time.Unix(stmt.ColumnInt64(10), 0)
		if !stmt.ColumnIsNull(11) {
			lastLogin := time.Unix(stmt.ColumnInt64(11), 0)
			user.LastLoginAt = &lastLogin
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	if user.ID == "" {
		return nil, nil // not found
	}
	return &user, nil
}

func (s *UserStoreImpl) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	var user User
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, email, username, first_name, last_name, avatar_url, role, status,
				   email_verified, created_at, updated_at, last_login_at
			FROM users WHERE username = ?
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
			return nil // not found
		}

		user.ID = stmt.ColumnText(0)
		user.Email = stmt.ColumnText(1)
		user.Username = stmt.ColumnText(2)
		user.FirstName = stmt.ColumnText(3)
		user.LastName = stmt.ColumnText(4)
		user.AvatarURL = stmt.ColumnText(5)
		user.Role = stmt.ColumnText(6)
		user.Status = stmt.ColumnText(7)
		user.EmailVerified = stmt.ColumnBool(8)
		user.CreatedAt = time.Unix(stmt.ColumnInt64(9), 0)
		user.UpdatedAt = time.Unix(stmt.ColumnInt64(10), 0)
		if !stmt.ColumnIsNull(11) {
			lastLogin := time.Unix(stmt.ColumnInt64(11), 0)
			user.LastLoginAt = &lastLogin
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	if user.ID == "" {
		return nil, nil // not found
	}
	return &user, nil
}

func (s *UserStoreImpl) ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error) {
	var users []*User
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		query := `
			SELECT id, email, username, first_name, last_name, avatar_url, role, status,
				   email_verified, created_at, updated_at, last_login_at
			FROM users
		`
		args := []interface{}{}

		// Apply filters
		if len(filters) > 0 {
			query += " WHERE "
			conditions := []string{}
			for key, value := range filters {
				switch key {
				case "role":
					conditions = append(conditions, "role = ?")
					args = append(args, value)
				case "status":
					conditions = append(conditions, "status = ?")
					args = append(args, value)
				case "email_verified":
					conditions = append(conditions, "email_verified = ?")
					args = append(args, value)
				}
			}
			if len(conditions) > 0 {
				query += conditions[0]
				for i := 1; i < len(conditions); i++ {
					query += " AND " + conditions[i]
				}
			}
		}

		query += " ORDER BY created_at DESC"
		if limit > 0 {
			query += " LIMIT ?"
			args = append(args, limit)
		}
		if offset > 0 {
			query += " OFFSET ?"
			args = append(args, offset)
		}

		stmt, err := tx.Prepare(query)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		for i, arg := range args {
			switch v := arg.(type) {
			case string:
				stmt.BindText(i+1, v)
			case int:
				stmt.BindInt64(i+1, int64(v))
			case bool:
				stmt.BindBool(i+1, v)
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
				EmailVerified: stmt.ColumnBool(8),
				CreatedAt:     time.Unix(stmt.ColumnInt64(9), 0),
				UpdatedAt:     time.Unix(stmt.ColumnInt64(10), 0),
			}
			if !stmt.ColumnIsNull(11) {
				lastLogin := time.Unix(stmt.ColumnInt64(11), 0)
				user.LastLoginAt = &lastLogin
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

func (s *UserStoreImpl) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error) {
	var users []*User
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, email, username, first_name, last_name, avatar_url, role, status,
				   email_verified, created_at, updated_at, last_login_at
			FROM users
			WHERE email LIKE ? OR username LIKE ? OR first_name LIKE ? OR last_name LIKE ?
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		searchPattern := "%" + query + "%"
		stmt.BindText(1, searchPattern)
		stmt.BindText(2, searchPattern)
		stmt.BindText(3, searchPattern)
		stmt.BindText(4, searchPattern)
		stmt.BindInt64(5, int64(limit))
		stmt.BindInt64(6, int64(offset))

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
				EmailVerified: stmt.ColumnBool(8),
				CreatedAt:     time.Unix(stmt.ColumnInt64(9), 0),
				UpdatedAt:     time.Unix(stmt.ColumnInt64(10), 0),
			}
			if !stmt.ColumnIsNull(11) {
				lastLogin := time.Unix(stmt.ColumnInt64(11), 0)
				user.LastLoginAt = &lastLogin
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

func (s *UserStoreImpl) CreateUser(ctx context.Context, user *User) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			INSERT INTO users (id, email, username, first_name, last_name, avatar_url, role, status,
							   email_verified, created_at, updated_at, last_login_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, user.ID)
		stmt.BindText(2, user.Email)
		stmt.BindText(3, user.Username)
		stmt.BindText(4, user.FirstName)
		stmt.BindText(5, user.LastName)
		stmt.BindText(6, user.AvatarURL)
		stmt.BindText(7, user.Role)
		stmt.BindText(8, user.Status)
		stmt.BindBool(9, user.EmailVerified)
		stmt.BindInt64(10, user.CreatedAt.Unix())
		stmt.BindInt64(11, user.UpdatedAt.Unix())
		if user.LastLoginAt != nil {
			stmt.BindInt64(12, user.LastLoginAt.Unix())
		} else {
			stmt.BindNull(12)
		}

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) UpdateUser(ctx context.Context, user *User) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			UPDATE users
			SET email = ?, username = ?, first_name = ?, last_name = ?, avatar_url = ?,
				role = ?, status = ?, email_verified = ?, updated_at = ?, last_login_at = ?
			WHERE id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, user.Email)
		stmt.BindText(2, user.Username)
		stmt.BindText(3, user.FirstName)
		stmt.BindText(4, user.LastName)
		stmt.BindText(5, user.AvatarURL)
		stmt.BindText(6, user.Role)
		stmt.BindText(7, user.Status)
		stmt.BindBool(8, user.EmailVerified)
		stmt.BindInt64(9, user.UpdatedAt.Unix())
		if user.LastLoginAt != nil {
			stmt.BindInt64(10, user.LastLoginAt.Unix())
		} else {
			stmt.BindNull(10)
		}
		stmt.BindText(11, user.ID)

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) PatchUser(ctx context.Context, patch *PatchUserInput) error {
	if patch.ID == "" {
		return fmt.Errorf("user ID is required for patch operation")
	}

	// Build dynamic query based on non-nil fields
	setParts := []string{}
	args := []interface{}{}

	if patch.Email != nil {
		setParts = append(setParts, "email = ?")
		args = append(args, *patch.Email)
	}
	if patch.Username != nil {
		setParts = append(setParts, "username = ?")
		args = append(args, *patch.Username)
	}
	if patch.FirstName != nil {
		setParts = append(setParts, "first_name = ?")
		args = append(args, *patch.FirstName)
	}
	if patch.LastName != nil {
		setParts = append(setParts, "last_name = ?")
		args = append(args, *patch.LastName)
	}
	if patch.AvatarURL != nil {
		setParts = append(setParts, "avatar_url = ?")
		args = append(args, *patch.AvatarURL)
	}
	if patch.Role != nil {
		setParts = append(setParts, "role = ?")
		args = append(args, *patch.Role)
	}
	if patch.Status != nil {
		setParts = append(setParts, "status = ?")
		args = append(args, *patch.Status)
	}
	if patch.EmailVerified != nil {
		setParts = append(setParts, "email_verified = ?")
		args = append(args, *patch.EmailVerified)
	}

	if len(setParts) == 0 {
		return fmt.Errorf("no fields to update")
	}

	// Always update the updated_at timestamp
	setParts = append(setParts, "updated_at = ?")
	args = append(args, time.Now().Unix())
	args = append(args, patch.ID) // WHERE clause parameter

	query := "UPDATE users SET " + setParts[0]
	for i := 1; i < len(setParts); i++ {
		query += ", " + setParts[i]
	}
	query += " WHERE id = ?"

	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(query)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		for i, arg := range args {
			switch v := arg.(type) {
			case string:
				stmt.BindText(i+1, v)
			case int64:
				stmt.BindInt64(i+1, v)
			case bool:
				stmt.BindBool(i+1, v)
			}
		}

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) DeleteUser(ctx context.Context, userID string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM users WHERE id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		_, err = stmt.Step()
		return err
	})
}

// UserCredential CRUD operations

func (s *UserStoreImpl) GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error) {
	var credentials []*UserCredential

	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, type, provider, provider_id, credential_data, created_at, updated_at
			FROM user_credentials WHERE user_id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			credential := &UserCredential{
				ID:             stmt.ColumnText(0),
				UserID:         stmt.ColumnText(1),
				Type:           stmt.ColumnText(2),
				Provider:       stmt.ColumnText(3),
				ProviderID:     stmt.ColumnText(4),
				CredentialData: stmt.ColumnText(5),
				CreatedAt:      time.Unix(stmt.ColumnInt64(6), 0),
				UpdatedAt:      time.Unix(stmt.ColumnInt64(7), 0),
			}
			credentials = append(credentials, credential)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return credentials, nil
}

func (s *UserStoreImpl) GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error) {
	var credential UserCredential
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, type, provider, provider_id, credential_data, created_at, updated_at
			FROM user_credentials WHERE user_id = ? AND provider = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		stmt.BindText(2, provider)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}

		credential.ID = stmt.ColumnText(0)
		credential.UserID = stmt.ColumnText(1)
		credential.Type = stmt.ColumnText(2)
		credential.Provider = stmt.ColumnText(3)
		credential.ProviderID = stmt.ColumnText(4)
		credential.CredentialData = stmt.ColumnText(5)
		credential.CreatedAt = time.Unix(stmt.ColumnInt64(6), 0)
		credential.UpdatedAt = time.Unix(stmt.ColumnInt64(7), 0)

		return nil
	})
	if err != nil {
		return nil, err
	}
	if credential.ID == "" {
		return nil, nil // not found
	}
	return &credential, nil
}

func (s *UserStoreImpl) GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error) {
	var user User
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT u.id, u.email, u.username, u.first_name, u.last_name, u.avatar_url, u.role, u.status,
				   u.email_verified, u.created_at, u.updated_at, u.last_login_at
			FROM users u
			JOIN user_credentials uc ON u.id = uc.user_id
			WHERE uc.provider = ? AND uc.provider_id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, provider)
		stmt.BindText(2, providerID)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}

		user.ID = stmt.ColumnText(0)
		user.Email = stmt.ColumnText(1)
		user.Username = stmt.ColumnText(2)
		user.FirstName = stmt.ColumnText(3)
		user.LastName = stmt.ColumnText(4)
		user.AvatarURL = stmt.ColumnText(5)
		user.Role = stmt.ColumnText(6)
		user.Status = stmt.ColumnText(7)
		user.EmailVerified = stmt.ColumnBool(8)
		user.CreatedAt = time.Unix(stmt.ColumnInt64(9), 0)
		user.UpdatedAt = time.Unix(stmt.ColumnInt64(10), 0)
		if !stmt.ColumnIsNull(11) {
			lastLogin := time.Unix(stmt.ColumnInt64(11), 0)
			user.LastLoginAt = &lastLogin
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	if user.ID == "" {
		return nil, nil // not found
	}
	return &user, nil
}

func (s *UserStoreImpl) CreateUserCredential(ctx context.Context, credential *UserCredential) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			INSERT INTO user_credentials (id, user_id, type, provider, provider_id, credential_data, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, credential.ID)
		stmt.BindText(2, credential.UserID)
		stmt.BindText(3, credential.Type)
		stmt.BindText(4, credential.Provider)
		stmt.BindText(5, credential.ProviderID)
		stmt.BindText(6, credential.CredentialData)
		stmt.BindInt64(7, credential.CreatedAt.Unix())
		stmt.BindInt64(8, credential.UpdatedAt.Unix())

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) UpdateUserCredential(ctx context.Context, credential *UserCredential) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			UPDATE user_credentials
			SET type = ?, provider = ?, provider_id = ?, credential_data = ?, updated_at = ?
			WHERE id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, credential.Type)
		stmt.BindText(2, credential.Provider)
		stmt.BindText(3, credential.ProviderID)
		stmt.BindText(4, credential.CredentialData)
		stmt.BindInt64(5, credential.UpdatedAt.Unix())
		stmt.BindText(6, credential.ID)

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) DeleteUserCredential(ctx context.Context, credentialID string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM user_credentials WHERE id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, credentialID)
		_, err = stmt.Step()
		return err
	})
}

// UserSession CRUD operations

func (s *UserStoreImpl) GetUserSession(ctx context.Context, sessionID string) (*UserSession, error) {
	var session UserSession
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, session_id, expires_at, created_at, ip_address, user_agent
			FROM user_sessions WHERE session_id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, sessionID)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}

		session.ID = stmt.ColumnText(0)
		session.UserID = stmt.ColumnText(1)
		session.SessionID = stmt.ColumnText(2)
		session.ExpiresAt = time.Unix(stmt.ColumnInt64(3), 0)
		session.CreatedAt = time.Unix(stmt.ColumnInt64(4), 0)
		session.IPAddress = stmt.ColumnText(5)
		session.UserAgent = stmt.ColumnText(6)

		return nil
	})
	if err != nil {
		return nil, err
	}
	if session.ID == "" {
		return nil, nil // not found
	}
	return &session, nil
}

func (s *UserStoreImpl) GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	var sessions []*UserSession
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, session_id, expires_at, created_at, ip_address, user_agent
			FROM user_sessions WHERE user_id = ? ORDER BY created_at DESC
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			session := &UserSession{
				ID:        stmt.ColumnText(0),
				UserID:    stmt.ColumnText(1),
				SessionID: stmt.ColumnText(2),
				ExpiresAt: time.Unix(stmt.ColumnInt64(3), 0),
				CreatedAt: time.Unix(stmt.ColumnInt64(4), 0),
				IPAddress: stmt.ColumnText(5),
				UserAgent: stmt.ColumnText(6),
			}
			sessions = append(sessions, session)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *UserStoreImpl) GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	var sessions []*UserSession
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, session_id, expires_at, created_at, ip_address, user_agent
			FROM user_sessions WHERE user_id = ? AND expires_at > ? ORDER BY created_at DESC
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		stmt.BindInt64(2, time.Now().Unix())

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			session := &UserSession{
				ID:        stmt.ColumnText(0),
				UserID:    stmt.ColumnText(1),
				SessionID: stmt.ColumnText(2),
				ExpiresAt: time.Unix(stmt.ColumnInt64(3), 0),
				CreatedAt: time.Unix(stmt.ColumnInt64(4), 0),
				IPAddress: stmt.ColumnText(5),
				UserAgent: stmt.ColumnText(6),
			}
			sessions = append(sessions, session)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return sessions, nil
}

func (s *UserStoreImpl) CreateUserSession(ctx context.Context, session *UserSession) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			INSERT INTO user_sessions (id, user_id, session_id, expires_at, created_at, ip_address, user_agent)
			VALUES (?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, session.ID)
		stmt.BindText(2, session.UserID)
		stmt.BindText(3, session.SessionID)
		stmt.BindInt64(4, session.ExpiresAt.Unix())
		stmt.BindInt64(5, session.CreatedAt.Unix())
		stmt.BindText(6, session.IPAddress)
		stmt.BindText(7, session.UserAgent)

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) UpdateUserSession(ctx context.Context, session *UserSession) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			UPDATE user_sessions
			SET expires_at = ?, ip_address = ?, user_agent = ?
			WHERE id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindInt64(1, session.ExpiresAt.Unix())
		stmt.BindText(2, session.IPAddress)
		stmt.BindText(3, session.UserAgent)
		stmt.BindText(4, session.ID)

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) DeleteUserSession(ctx context.Context, sessionID string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM user_sessions WHERE session_id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, sessionID)
		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) DeleteAllUserSessions(ctx context.Context, userID string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM user_sessions WHERE user_id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		_, err = stmt.Step()
		return err
	})
}

// InvalidateAllUserSessions marks all user sessions as expired
func (s *UserStoreImpl) InvalidateAllUserSessions(ctx context.Context, userID string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			UPDATE user_sessions
			SET expires_at = ?
			WHERE user_id = ? AND expires_at > ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		now := time.Now().Unix()
		stmt.BindInt64(1, now)
		stmt.BindText(2, userID)
		stmt.BindInt64(3, now)
		_, err = stmt.Step()
		return err
	})
}

// InvalidateAllUserSessionsExcept marks all user sessions as expired except the specified one
func (s *UserStoreImpl) InvalidateAllUserSessionsExcept(ctx context.Context, userID, exceptSessionID string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			UPDATE user_sessions
			SET expires_at = ?
			WHERE user_id = ? AND session_id != ? AND expires_at > ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		now := time.Now().Unix()
		stmt.BindInt64(1, now)
		stmt.BindText(2, userID)
		stmt.BindText(3, exceptSessionID)
		stmt.BindInt64(4, now)
		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) DeleteExpiredSessions(ctx context.Context) (int, error) {
	var deletedCount int
	err := s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM user_sessions WHERE expires_at <= ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindInt64(1, time.Now().Unix())
		_, err = stmt.Step()
		if err != nil {
			return err
		}

		// Get the number of deleted rows
		deletedCount = int(tx.Changes())
		return nil
	})
	if err != nil {
		return 0, err
	}
	return deletedCount, nil
}

// ProjectMembership CRUD operations

func (s *UserStoreImpl) GetProjectMembers(ctx context.Context, projectID string) ([]*User, error) {
	var users []*User
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT u.id, u.email, u.username, u.first_name, u.last_name, u.avatar_url, u.role, u.status,
				   u.email_verified, u.created_at, u.updated_at, u.last_login_at
			FROM users u
			JOIN project_memberships pm ON u.id = pm.user_id
			WHERE pm.project_id = ?
			ORDER BY u.username
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, projectID)

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
				EmailVerified: stmt.ColumnBool(8),
				CreatedAt:     time.Unix(stmt.ColumnInt64(9), 0),
				UpdatedAt:     time.Unix(stmt.ColumnInt64(10), 0),
			}
			if !stmt.ColumnIsNull(11) {
				lastLogin := time.Unix(stmt.ColumnInt64(11), 0)
				user.LastLoginAt = &lastLogin
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

func (s *UserStoreImpl) GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error) {
	var memberships []*ProjectMembership
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, project_id, role, created_at, created_by
			FROM project_memberships WHERE user_id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			membership := &ProjectMembership{
				ID:        stmt.ColumnText(0),
				UserID:    stmt.ColumnText(1),
				ProjectID: stmt.ColumnText(2),
				Role:      stmt.ColumnText(3),
				CreatedAt: time.Unix(stmt.ColumnInt64(4), 0),
				CreatedBy: stmt.ColumnText(5),
			}
			memberships = append(memberships, membership)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return memberships, nil
}

func (s *UserStoreImpl) GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error) {
	var membership ProjectMembership
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, project_id, role, created_at, created_by
			FROM project_memberships WHERE user_id = ? AND project_id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		stmt.BindText(2, projectID)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}

		membership.ID = stmt.ColumnText(0)
		membership.UserID = stmt.ColumnText(1)
		membership.ProjectID = stmt.ColumnText(2)
		membership.Role = stmt.ColumnText(3)
		membership.CreatedAt = time.Unix(stmt.ColumnInt64(4), 0)
		membership.CreatedBy = stmt.ColumnText(5)

		return nil
	})
	if err != nil {
		return nil, err
	}
	if membership.ID == "" {
		return nil, nil // not found
	}
	return &membership, nil
}

func (s *UserStoreImpl) CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	var hasAccess bool
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT COUNT(*) FROM project_memberships WHERE user_id = ? AND project_id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		stmt.BindText(2, projectID)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			hasAccess = stmt.ColumnInt64(0) > 0
		}

		return nil
	})
	if err != nil {
		return false, err
	}
	return hasAccess, nil
}

func (s *UserStoreImpl) CreateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			INSERT INTO project_memberships (id, user_id, project_id, role, created_at, created_by)
			VALUES (?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, membership.ID)
		stmt.BindText(2, membership.UserID)
		stmt.BindText(3, membership.ProjectID)
		stmt.BindText(4, membership.Role)
		stmt.BindInt64(5, membership.CreatedAt.Unix())
		stmt.BindText(6, membership.CreatedBy)

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) UpdateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			UPDATE project_memberships
			SET role = ?
			WHERE user_id = ? AND project_id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, membership.Role)
		stmt.BindText(2, membership.UserID)
		stmt.BindText(3, membership.ProjectID)

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) DeleteProjectMembership(ctx context.Context, userID, projectID string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM project_memberships WHERE user_id = ? AND project_id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		stmt.BindText(2, projectID)
		_, err = stmt.Step()
		return err
	})
}

// Token operations

func (s *UserStoreImpl) GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	var verificationToken EmailVerificationToken
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, token, expires_at, created_at
			FROM email_verification_tokens WHERE token = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, token)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}

		verificationToken.ID = stmt.ColumnText(0)
		verificationToken.UserID = stmt.ColumnText(1)
		verificationToken.Token = stmt.ColumnText(2)
		verificationToken.ExpiresAt = time.Unix(stmt.ColumnInt64(3), 0)
		verificationToken.CreatedAt = time.Unix(stmt.ColumnInt64(4), 0)

		return nil
	})
	if err != nil {
		return nil, err
	}
	if verificationToken.ID == "" {
		return nil, nil // not found
	}
	return &verificationToken, nil
}

func (s *UserStoreImpl) CreateEmailVerificationToken(ctx context.Context, token *EmailVerificationToken) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			INSERT INTO email_verification_tokens (id, user_id, token, expires_at, created_at)
			VALUES (?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, token.ID)
		stmt.BindText(2, token.UserID)
		stmt.BindText(3, token.Token)
		stmt.BindInt64(4, token.ExpiresAt.Unix())
		stmt.BindInt64(5, token.CreatedAt.Unix())

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error) {
	var verificationToken EmailVerificationToken

	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, token, expires_at, created_at
			FROM email_verification_tokens WHERE user_id = ?
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
			return nil // not found
		}

		verificationToken.ID = stmt.ColumnText(0)
		verificationToken.UserID = stmt.ColumnText(1)
		verificationToken.Token = stmt.ColumnText(2)
		verificationToken.ExpiresAt = time.Unix(stmt.ColumnInt64(3), 0)
		verificationToken.CreatedAt = time.Unix(stmt.ColumnInt64(4), 0)

		return nil
	})
	if err != nil {
		return nil, err
	}
	if verificationToken.ID == "" {
		return nil, nil // not found
	}
	return &verificationToken, nil
}

func (s *UserStoreImpl) DeleteEmailVerificationToken(ctx context.Context, token string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM email_verification_tokens WHERE token = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, token)
		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	var resetToken PasswordResetToken
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, token, expires_at, created_at, used_at
			FROM password_reset_tokens WHERE token = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, token)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}

		resetToken.ID = stmt.ColumnText(0)
		resetToken.UserID = stmt.ColumnText(1)
		resetToken.Token = stmt.ColumnText(2)
		resetToken.ExpiresAt = time.Unix(stmt.ColumnInt64(3), 0)
		resetToken.CreatedAt = time.Unix(stmt.ColumnInt64(4), 0)
		if !stmt.ColumnIsNull(5) {
			usedAt := time.Unix(stmt.ColumnInt64(5), 0)
			resetToken.UsedAt = &usedAt
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	if resetToken.ID == "" {
		return nil, nil // not found
	}
	return &resetToken, nil
}

func (s *UserStoreImpl) CreatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			INSERT INTO password_reset_tokens (id, user_id, token, expires_at, created_at, used_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, token.ID)
		stmt.BindText(2, token.UserID)
		stmt.BindText(3, token.Token)
		stmt.BindInt64(4, token.ExpiresAt.Unix())
		stmt.BindInt64(5, token.CreatedAt.Unix())
		if token.UsedAt != nil {
			stmt.BindInt64(6, token.UsedAt.Unix())
		} else {
			stmt.BindNull(6)
		}

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) UpdatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			UPDATE password_reset_tokens
			SET used_at = ?
			WHERE id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		if token.UsedAt != nil {
			stmt.BindInt64(1, token.UsedAt.Unix())
		} else {
			stmt.BindNull(1)
		}
		stmt.BindText(2, token.ID)

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) DeletePasswordResetToken(ctx context.Context, token string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM password_reset_tokens WHERE token = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, token)
		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error) {
	var magicToken MagicMailToken
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, user_id, token, expires_at, created_at, used_at
			FROM magic_mail_tokens WHERE token = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, token)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}

		magicToken.ID = stmt.ColumnText(0)
		magicToken.UserID = stmt.ColumnText(1)
		magicToken.Token = stmt.ColumnText(2)
		magicToken.ExpiresAt = time.Unix(stmt.ColumnInt64(3), 0)
		magicToken.CreatedAt = time.Unix(stmt.ColumnInt64(4), 0)
		if !stmt.ColumnIsNull(5) {
			usedAt := time.Unix(stmt.ColumnInt64(5), 0)
			magicToken.UsedAt = &usedAt
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	if magicToken.ID == "" {
		return nil, nil // not found
	}
	return &magicToken, nil
}

func (s *UserStoreImpl) CreateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			INSERT INTO magic_mail_tokens (id, user_id, token, expires_at, created_at, used_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, token.ID)
		stmt.BindText(2, token.UserID)
		stmt.BindText(3, token.Token)
		stmt.BindInt64(4, token.ExpiresAt.Unix())
		stmt.BindInt64(5, token.CreatedAt.Unix())
		if token.UsedAt != nil {
			stmt.BindInt64(6, token.UsedAt.Unix())
		} else {
			stmt.BindNull(6)
		}

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) UpdateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			UPDATE magic_mail_tokens
			SET used_at = ?
			WHERE id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		if token.UsedAt != nil {
			stmt.BindInt64(1, token.UsedAt.Unix())
		} else {
			stmt.BindNull(1)
		}
		stmt.BindText(2, token.ID)

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) DeleteMagicMailToken(ctx context.Context, token string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM magic_mail_tokens WHERE token = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, token)
		_, err = stmt.Step()
		return err
	})
}

// Invitation operations

func (s *UserStoreImpl) GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error) {
	var invitations []*UserInvitation
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, project_id, email, role, token, expires_at, created_at, created_by, accepted_at, declined_at
			FROM user_invitations WHERE project_id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, projectID)

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			invitation := &UserInvitation{
				ID:        stmt.ColumnText(0),
				ProjectID: stmt.ColumnText(1),
				Email:     stmt.ColumnText(2),
				Role:      stmt.ColumnText(3),
				Token:     stmt.ColumnText(4),
				ExpiresAt: time.Unix(stmt.ColumnInt64(5), 0),
				CreatedAt: time.Unix(stmt.ColumnInt64(6), 0),
				CreatedBy: stmt.ColumnText(7),
			}
			if !stmt.ColumnIsNull(8) {
				acceptedAt := time.Unix(stmt.ColumnInt64(8), 0)
				invitation.AcceptedAt = &acceptedAt
			}
			if !stmt.ColumnIsNull(9) {
				declinedAt := time.Unix(stmt.ColumnInt64(9), 0)
				invitation.DeclinedAt = &declinedAt
			}
			invitations = append(invitations, invitation)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return invitations, nil
}

func (s *UserStoreImpl) GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error) {
	var invitations []*UserInvitation
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, project_id, email, role, token, expires_at, created_at, created_by, accepted_at, declined_at
			FROM user_invitations WHERE email = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, email)

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			invitation := &UserInvitation{
				ID:        stmt.ColumnText(0),
				ProjectID: stmt.ColumnText(1),
				Email:     stmt.ColumnText(2),
				Role:      stmt.ColumnText(3),
				Token:     stmt.ColumnText(4),
				ExpiresAt: time.Unix(stmt.ColumnInt64(5), 0),
				CreatedAt: time.Unix(stmt.ColumnInt64(6), 0),
				CreatedBy: stmt.ColumnText(7),
			}
			if !stmt.ColumnIsNull(8) {
				acceptedAt := time.Unix(stmt.ColumnInt64(8), 0)
				invitation.AcceptedAt = &acceptedAt
			}
			if !stmt.ColumnIsNull(9) {
				declinedAt := time.Unix(stmt.ColumnInt64(9), 0)
				invitation.DeclinedAt = &declinedAt
			}
			invitations = append(invitations, invitation)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return invitations, nil
}

func (s *UserStoreImpl) GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error) {
	var invitation UserInvitation
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT id, project_id, email, role, token, expires_at, created_at, created_by, accepted_at, declined_at
			FROM user_invitations WHERE token = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, token)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}

		invitation.ID = stmt.ColumnText(0)
		invitation.ProjectID = stmt.ColumnText(1)
		invitation.Email = stmt.ColumnText(2)
		invitation.Role = stmt.ColumnText(3)
		invitation.Token = stmt.ColumnText(4)
		invitation.ExpiresAt = time.Unix(stmt.ColumnInt64(5), 0)
		invitation.CreatedAt = time.Unix(stmt.ColumnInt64(6), 0)
		invitation.CreatedBy = stmt.ColumnText(7)
		if !stmt.ColumnIsNull(8) {
			acceptedAt := time.Unix(stmt.ColumnInt64(8), 0)
			invitation.AcceptedAt = &acceptedAt
		}
		if !stmt.ColumnIsNull(9) {
			declinedAt := time.Unix(stmt.ColumnInt64(9), 0)
			invitation.DeclinedAt = &declinedAt
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	if invitation.ID == "" {
		return nil, nil // not found
	}
	return &invitation, nil
}

func (s *UserStoreImpl) CreateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			INSERT INTO user_invitations (id, project_id, email, role, token, expires_at, created_at, created_by, accepted_at, declined_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, invitation.ID)
		stmt.BindText(2, invitation.ProjectID)
		stmt.BindText(3, invitation.Email)
		stmt.BindText(4, invitation.Role)
		stmt.BindText(5, invitation.Token)
		stmt.BindInt64(6, invitation.ExpiresAt.Unix())
		stmt.BindInt64(7, invitation.CreatedAt.Unix())
		stmt.BindText(8, invitation.CreatedBy)
		if invitation.AcceptedAt != nil {
			stmt.BindInt64(9, invitation.AcceptedAt.Unix())
		} else {
			stmt.BindNull(9)
		}
		if invitation.DeclinedAt != nil {
			stmt.BindInt64(10, invitation.DeclinedAt.Unix())
		} else {
			stmt.BindNull(10)
		}

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) UpdateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			UPDATE user_invitations
			SET accepted_at = ?, declined_at = ?
			WHERE id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		if invitation.AcceptedAt != nil {
			stmt.BindInt64(1, invitation.AcceptedAt.Unix())
		} else {
			stmt.BindNull(1)
		}
		if invitation.DeclinedAt != nil {
			stmt.BindInt64(2, invitation.DeclinedAt.Unix())
		} else {
			stmt.BindNull(2)
		}
		stmt.BindText(3, invitation.ID)

		_, err = stmt.Step()
		return err
	})
}

func (s *UserStoreImpl) DeleteInvitation(ctx context.Context, invitationID string) error {
	return s.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`DELETE FROM user_invitations WHERE id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, invitationID)
		_, err = stmt.Step()
		return err
	})
}

// Administrative operations

func (s *UserStoreImpl) GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error) {
	var activities []interface{}
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		// Get user sessions as activity
		stmt, err := tx.Prepare(`
			SELECT 'login' as activity_type, created_at, ip_address, user_agent
			FROM user_sessions
			WHERE user_id = ?
			ORDER BY created_at DESC
			LIMIT ? OFFSET ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		stmt.BindInt64(2, int64(limit))
		stmt.BindInt64(3, int64(offset))

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			activity := map[string]interface{}{
				"type":       stmt.ColumnText(0),
				"timestamp":  time.Unix(stmt.ColumnInt64(1), 0),
				"ip_address": stmt.ColumnText(2),
				"user_agent": stmt.ColumnText(3),
			}
			activities = append(activities, activity)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return activities, nil
}

func (s *UserStoreImpl) GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		// Get user basic info
		stmt, err := tx.Prepare(`
			SELECT created_at, last_login_at, email_verified, status
			FROM users WHERE id = ?
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
		if rowReturned {
			stats["created_at"] = time.Unix(stmt.ColumnInt64(0), 0)
			if !stmt.ColumnIsNull(1) {
				stats["last_login_at"] = time.Unix(stmt.ColumnInt64(1), 0)
			}
			stats["email_verified"] = stmt.ColumnBool(2)
			stats["status"] = stmt.ColumnText(3)
		}
		stmt.Finalize()

		// Get session count
		stmt, err = tx.Prepare(`SELECT COUNT(*) FROM user_sessions WHERE user_id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		rowReturned, err = stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			stats["total_sessions"] = stmt.ColumnInt64(0)
		}
		stmt.Finalize()

		// Get active session count
		stmt, err = tx.Prepare(`SELECT COUNT(*) FROM user_sessions WHERE user_id = ? AND expires_at > ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		stmt.BindInt64(2, time.Now().Unix())
		rowReturned, err = stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			stats["active_sessions"] = stmt.ColumnInt64(0)
		}
		stmt.Finalize()

		// Get project membership count
		stmt, err = tx.Prepare(`SELECT COUNT(*) FROM project_memberships WHERE user_id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		rowReturned, err = stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			stats["project_memberships"] = stmt.ColumnInt64(0)
		}
		stmt.Finalize()

		// Get credential count
		stmt, err = tx.Prepare(`SELECT COUNT(*) FROM user_credentials WHERE user_id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, userID)
		rowReturned, err = stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			stats["credentials_count"] = stmt.ColumnInt64(0)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return stats, nil
}

func (s *UserStoreImpl) GetSystemUserStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	err := s.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		// Get total user count
		stmt, err := tx.Prepare(`SELECT COUNT(*) FROM users`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			stats["total_users"] = stmt.ColumnInt64(0)
		}
		stmt.Finalize()

		// Get active user count
		stmt, err = tx.Prepare(`SELECT COUNT(*) FROM users WHERE status = 'Active'`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		rowReturned, err = stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			stats["active_users"] = stmt.ColumnInt64(0)
		}
		stmt.Finalize()

		// Get verified user count
		stmt, err = tx.Prepare(`SELECT COUNT(*) FROM users WHERE email_verified = true`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		rowReturned, err = stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			stats["verified_users"] = stmt.ColumnInt64(0)
		}
		stmt.Finalize()

		// Get users by role
		stmt, err = tx.Prepare(`SELECT role, COUNT(*) FROM users GROUP BY role`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		roleStats := make(map[string]int64)
		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}
			roleStats[stmt.ColumnText(0)] = stmt.ColumnInt64(1)
		}
		stats["users_by_role"] = roleStats
		stmt.Finalize()

		// Get recent registrations (last 30 days)
		stmt, err = tx.Prepare(`SELECT COUNT(*) FROM users WHERE created_at > ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		thirtyDaysAgo := time.Now().AddDate(0, 0, -30)
		stmt.BindInt64(1, thirtyDaysAgo.Unix())
		rowReturned, err = stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			stats["recent_registrations"] = stmt.ColumnInt64(0)
		}
		stmt.Finalize()

		// Get total sessions
		stmt, err = tx.Prepare(`SELECT COUNT(*) FROM user_sessions`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		rowReturned, err = stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			stats["total_sessions"] = stmt.ColumnInt64(0)
		}
		stmt.Finalize()

		// Get active sessions
		stmt, err = tx.Prepare(`SELECT COUNT(*) FROM user_sessions WHERE expires_at > ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindInt64(1, time.Now().Unix())
		rowReturned, err = stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			stats["active_sessions"] = stmt.ColumnInt64(0)
		}

		return nil
	})
	if err != nil {
		return nil, err
	}
	return stats, nil
}

// Close closes the database connection
func (s *UserStoreImpl) Close() error {
	if s.db != nil {
		return s.db.Close()
	}
	return nil
}
