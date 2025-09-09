package users

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockUserStoreForRBAC for testing
type MockUserStoreForRBAC struct {
	mock.Mock
}

func (m *MockUserStoreForRBAC) GetUser(ctx context.Context, userID string) (*User, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserStoreForRBAC) GetProjectMembership(ctx context.Context, userID, projectID string) (*ProjectMembership, error) {
	args := m.Called(ctx, userID, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ProjectMembership), args.Error(1)
}

func (m *MockUserStoreForRBAC) CheckUserProjectAccess(ctx context.Context, userID, projectID string) (bool, error) {
	args := m.Called(ctx, userID, projectID)
	return args.Bool(0), args.Error(1)
}

func (m *MockUserStoreForRBAC) GetUserProjects(ctx context.Context, userID string) ([]*ProjectMembership, error) {
	args := m.Called(ctx, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*ProjectMembership), args.Error(1)
}

// Implement other required methods as no-ops for testing
func (m *MockUserStoreForRBAC) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	args := m.Called(ctx, email)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*User), args.Error(1)
}

func (m *MockUserStoreForRBAC) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) ListUsers(ctx context.Context, limit, offset int, filters map[string]interface{}) ([]*User, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) SearchUsers(ctx context.Context, query string, limit, offset int) ([]*User, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) CreateUser(ctx context.Context, user *User) error {
	return nil
}

func (m *MockUserStoreForRBAC) UpdateUser(ctx context.Context, user *User) error {
	return nil
}

func (m *MockUserStoreForRBAC) PatchUser(ctx context.Context, patch *PatchUserInput) error {
	return nil
}

func (m *MockUserStoreForRBAC) DeleteUser(ctx context.Context, userID string) error {
	return nil
}

func (m *MockUserStoreForRBAC) GetUserCredentials(ctx context.Context, userID string) ([]*UserCredential, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) GetUserCredentialByProvider(ctx context.Context, userID, provider string) (*UserCredential, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) GetUserByOAuthProvider(ctx context.Context, provider, providerID string) (*User, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) CreateUserCredential(ctx context.Context, credential *UserCredential) error {
	return nil
}

func (m *MockUserStoreForRBAC) UpdateUserCredential(ctx context.Context, credential *UserCredential) error {
	return nil
}

func (m *MockUserStoreForRBAC) DeleteUserCredential(ctx context.Context, credentialID string) error {
	return nil
}

func (m *MockUserStoreForRBAC) GetUserSession(ctx context.Context, sessionID string) (*UserSession, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) GetUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) GetActiveUserSessions(ctx context.Context, userID string) ([]*UserSession, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) CreateUserSession(ctx context.Context, session *UserSession) error {
	return nil
}

func (m *MockUserStoreForRBAC) UpdateUserSession(ctx context.Context, session *UserSession) error {
	return nil
}

func (m *MockUserStoreForRBAC) DeleteUserSession(ctx context.Context, sessionID string) error {
	return nil
}

func (m *MockUserStoreForRBAC) DeleteAllUserSessions(ctx context.Context, userID string) error {
	return nil
}

func (m *MockUserStoreForRBAC) DeleteExpiredSessions(ctx context.Context) (int, error) {
	return 0, nil
}

func (m *MockUserStoreForRBAC) GetProjectMembers(ctx context.Context, projectID string) ([]*User, error) {
	args := m.Called(ctx, projectID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*User), args.Error(1)
}

func (m *MockUserStoreForRBAC) CreateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}

func (m *MockUserStoreForRBAC) UpdateProjectMembership(ctx context.Context, membership *ProjectMembership) error {
	return nil
}

func (m *MockUserStoreForRBAC) DeleteProjectMembership(ctx context.Context, userID, projectID string) error {
	return nil
}

func (m *MockUserStoreForRBAC) GetEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) GetEmailVerificationTokenByUserID(ctx context.Context, userID string) (*EmailVerificationToken, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) CreateEmailVerificationToken(ctx context.Context, token *EmailVerificationToken) error {
	return nil
}

func (m *MockUserStoreForRBAC) DeleteEmailVerificationToken(ctx context.Context, token string) error {
	return nil
}

func (m *MockUserStoreForRBAC) GetPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) CreatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}

func (m *MockUserStoreForRBAC) UpdatePasswordResetToken(ctx context.Context, token *PasswordResetToken) error {
	return nil
}

func (m *MockUserStoreForRBAC) DeletePasswordResetToken(ctx context.Context, token string) error {
	return nil
}

func (m *MockUserStoreForRBAC) GetMagicMailToken(ctx context.Context, token string) (*MagicMailToken, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) CreateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}

func (m *MockUserStoreForRBAC) UpdateMagicMailToken(ctx context.Context, token *MagicMailToken) error {
	return nil
}

func (m *MockUserStoreForRBAC) DeleteMagicMailToken(ctx context.Context, token string) error {
	return nil
}

func (m *MockUserStoreForRBAC) GetProjectInvitations(ctx context.Context, projectID string) ([]*UserInvitation, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) GetUserInvitations(ctx context.Context, email string) ([]*UserInvitation, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) GetInvitationByToken(ctx context.Context, token string) (*UserInvitation, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) CreateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}

func (m *MockUserStoreForRBAC) UpdateInvitation(ctx context.Context, invitation *UserInvitation) error {
	return nil
}

func (m *MockUserStoreForRBAC) DeleteInvitation(ctx context.Context, invitationID string) error {
	return nil
}

func (m *MockUserStoreForRBAC) GetUserActivity(ctx context.Context, userID string, limit, offset int) ([]interface{}, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) GetUserStats(ctx context.Context, userID string) (map[string]interface{}, error) {
	return nil, nil
}

func (m *MockUserStoreForRBAC) GetSystemUserStats(ctx context.Context) (map[string]interface{}, error) {
	return nil, nil
}

func TestRBACService_ValidateSystemRole(t *testing.T) {
	mockStore := new(MockUserStore)
	rbacService := NewRBACService(mockStore)

	tests := []struct {
		name    string
		role    string
		wantErr bool
	}{
		{
			name:    "valid admin role",
			role:    RoleAdmin,
			wantErr: false,
		},
		{
			name:    "valid project manager role",
			role:    RoleProjectManager,
			wantErr: false,
		},
		{
			name:    "valid developer role",
			role:    RoleDeveloper,
			wantErr: false,
		},
		{
			name:    "valid viewer role",
			role:    RoleViewer,
			wantErr: false,
		},
		{
			name:    "invalid role",
			role:    "InvalidRole",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rbacService.ValidateSystemRole(tt.role)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRBACService_ValidateProjectRole(t *testing.T) {
	mockStore := new(MockUserStore)
	rbacService := NewRBACService(mockStore)

	tests := []struct {
		name    string
		role    string
		wantErr bool
	}{
		{
			name:    "valid project admin role",
			role:    ProjectRoleAdmin,
			wantErr: false,
		},
		{
			name:    "valid project developer role",
			role:    ProjectRoleDeveloper,
			wantErr: false,
		},
		{
			name:    "valid project viewer role",
			role:    ProjectRoleViewer,
			wantErr: false,
		},
		{
			name:    "invalid role",
			role:    "InvalidProjectRole",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rbacService.ValidateProjectRole(tt.role)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRBACService_HasSystemPermission(t *testing.T) {
	mockStore := new(MockUserStore)
	rbacService := NewRBACService(mockStore)
	ctx := context.Background()

	tests := []struct {
		name       string
		userRole   string
		permission string
		want       bool
		wantErr    bool
	}{
		{
			name:       "admin has manage users permission",
			userRole:   RoleAdmin,
			permission: PermissionManageUsers,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "developer does not have manage users permission",
			userRole:   RoleDeveloper,
			permission: PermissionManageUsers,
			want:       false,
			wantErr:    false,
		},
		{
			name:       "project manager has create project permission",
			userRole:   RoleProjectManager,
			permission: PermissionCreateProject,
			want:       true,
			wantErr:    false,
		},
		{
			name:       "viewer does not have create project permission",
			userRole:   RoleViewer,
			permission: PermissionCreateProject,
			want:       false,
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{
				ID:   "test-user",
				Role: tt.userRole,
			}

			mockStore.On("GetUser", ctx, "test-user").Return(user, nil)

			got, err := rbacService.HasSystemPermission(ctx, "test-user", tt.permission)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestRBACService_CanAccessProject(t *testing.T) {
	mockStore := new(MockUserStore)
	rbacService := NewRBACService(mockStore)
	ctx := context.Background()

	tests := []struct {
		name      string
		userRole  string
		hasAccess bool
		accessErr error
		want      bool
		wantErr   bool
	}{
		{
			name:      "admin can access any project",
			userRole:  RoleAdmin,
			hasAccess: false, // This shouldn't be checked for admin
			want:      true,
			wantErr:   false,
		},
		{
			name:      "developer with project access",
			userRole:  RoleDeveloper,
			hasAccess: true,
			want:      true,
			wantErr:   false,
		},
		{
			name:      "developer without project access",
			userRole:  RoleDeveloper,
			hasAccess: false,
			want:      false,
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			user := &User{
				ID:   "test-user",
				Role: tt.userRole,
			}

			mockStore.On("GetUser", ctx, "test-user").Return(user, nil)

			if tt.userRole != RoleAdmin {
				mockStore.On("CheckUserProjectAccess", ctx, "test-user", "test-project").Return(tt.hasAccess, tt.accessErr)
			}

			got, err := rbacService.CanAccessProject(ctx, "test-user", "test-project")
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestRBACService_CanManageUser(t *testing.T) {
	mockStore := new(MockUserStore)
	rbacService := NewRBACService(mockStore)
	ctx := context.Background()

	tests := []struct {
		name         string
		managerRole  string
		targetUserID string
		setupMocks   func()
		want         bool
		wantErr      bool
	}{
		{
			name:         "admin can manage any user",
			managerRole:  RoleAdmin,
			targetUserID: "target-user",
			setupMocks: func() {
				manager := &User{ID: "manager-user", Role: RoleAdmin}
				mockStore.On("GetUser", ctx, "manager-user").Return(manager, nil)
			},
			want:    true,
			wantErr: false,
		},
		{
			name:         "developer cannot manage users",
			managerRole:  RoleDeveloper,
			targetUserID: "target-user",
			setupMocks: func() {
				manager := &User{ID: "manager-user", Role: RoleDeveloper}
				mockStore.On("GetUser", ctx, "manager-user").Return(manager, nil)
			},
			want:    false,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset mock
			mockStore.ExpectedCalls = nil
			mockStore.Calls = nil

			tt.setupMocks()

			got, err := rbacService.CanManageUser(ctx, "manager-user", tt.targetUserID)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.want, got)
			}

			mockStore.AssertExpectations(t)
		})
	}
}

func TestNormalizeRoleName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "admin variations",
			input:    "admin",
			expected: RoleAdmin,
		},
		{
			name:     "administrator",
			input:    "administrator",
			expected: RoleAdmin,
		},
		{
			name:     "project manager variations",
			input:    "project_manager",
			expected: RoleProjectManager,
		},
		{
			name:     "pm",
			input:    "pm",
			expected: RoleProjectManager,
		},
		{
			name:     "developer variations",
			input:    "developer",
			expected: RoleDeveloper,
		},
		{
			name:     "dev",
			input:    "dev",
			expected: RoleDeveloper,
		},
		{
			name:     "viewer variations",
			input:    "viewer",
			expected: RoleViewer,
		},
		{
			name:     "readonly",
			input:    "readonly",
			expected: RoleViewer,
		},
		{
			name:     "project admin",
			input:    "project_admin",
			expected: ProjectRoleAdmin,
		},
		{
			name:     "unknown role",
			input:    "unknown",
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizeRoleName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRBACService_IsValidRoleTransition(t *testing.T) {
	mockStore := new(MockUserStore)
	rbacService := NewRBACService(mockStore)

	tests := []struct {
		name     string
		fromRole string
		toRole   string
		want     bool
	}{
		{
			name:     "admin can change to any role",
			fromRole: RoleAdmin,
			toRole:   RoleViewer,
			want:     true,
		},
		{
			name:     "project manager can be demoted to developer",
			fromRole: RoleProjectManager,
			toRole:   RoleDeveloper,
			want:     true,
		},
		{
			name:     "project manager can be demoted to viewer",
			fromRole: RoleProjectManager,
			toRole:   RoleViewer,
			want:     true,
		},
		{
			name:     "developer can be promoted to project manager",
			fromRole: RoleDeveloper,
			toRole:   RoleProjectManager,
			want:     true,
		},
		{
			name:     "developer can be demoted to viewer",
			fromRole: RoleDeveloper,
			toRole:   RoleViewer,
			want:     true,
		},
		{
			name:     "viewer can be promoted to any role",
			fromRole: RoleViewer,
			toRole:   RoleAdmin,
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rbacService.IsValidRoleTransition(tt.fromRole, tt.toRole)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRBACService_GetAvailableSystemRoles(t *testing.T) {
	mockStore := new(MockUserStore)
	rbacService := NewRBACService(mockStore)

	roles := rbacService.GetAvailableSystemRoles()

	assert.Len(t, roles, 4) // Admin, ProjectManager, Developer, Viewer

	roleNames := make([]string, len(roles))
	for i, role := range roles {
		roleNames[i] = role.Name
	}

	assert.Contains(t, roleNames, RoleAdmin)
	assert.Contains(t, roleNames, RoleProjectManager)
	assert.Contains(t, roleNames, RoleDeveloper)
	assert.Contains(t, roleNames, RoleViewer)
}

func TestRBACService_GetAvailableProjectRoles(t *testing.T) {
	mockStore := new(MockUserStore)
	rbacService := NewRBACService(mockStore)

	roles := rbacService.GetAvailableProjectRoles()

	assert.Len(t, roles, 3) // ProjectAdmin, ProjectDeveloper, ProjectViewer

	roleNames := make([]string, len(roles))
	for i, role := range roles {
		roleNames[i] = role.Name
	}

	assert.Contains(t, roleNames, ProjectRoleAdmin)
	assert.Contains(t, roleNames, ProjectRoleDeveloper)
	assert.Contains(t, roleNames, ProjectRoleViewer)
}
