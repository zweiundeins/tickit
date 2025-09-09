package users

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/mux"
	"github.com/gorilla/sessions"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// MockSessionStore for testing
type MockSessionStore struct {
	mock.Mock
	sessions map[string]*sessions.Session
}

func NewMockSessionStore() *MockSessionStore {
	return &MockSessionStore{
		sessions: make(map[string]*sessions.Session),
	}
}

func (m *MockSessionStore) Get(r *http.Request, name string) (*sessions.Session, error) {
	args := m.Called(r, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sessions.Session), args.Error(1)
}

func (m *MockSessionStore) New(r *http.Request, name string) (*sessions.Session, error) {
	args := m.Called(r, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*sessions.Session), args.Error(1)
}

func (m *MockSessionStore) Save(r *http.Request, w http.ResponseWriter, s *sessions.Session) error {
	args := m.Called(r, w, s)
	return args.Error(0)
}

func createAuthenticatedSession(userID string) *sessions.Session {
	session := &sessions.Session{
		Values: make(map[interface{}]interface{}),
	}
	session.Values["user_id"] = userID
	session.Values["authenticated"] = true
	return session
}

func createUnauthenticatedSession() *sessions.Session {
	session := &sessions.Session{
		Values: make(map[interface{}]interface{}),
	}
	return session
}

func TestRequireSystemPermission(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		userRole       string
		permission     string
		authenticated  bool
		expectedStatus int
	}{
		{
			name:           "authenticated admin with manage users permission",
			userID:         "admin-user",
			userRole:       RoleAdmin,
			permission:     PermissionManageUsers,
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "authenticated developer without manage users permission",
			userID:         "dev-user",
			userRole:       RoleDeveloper,
			permission:     PermissionManageUsers,
			authenticated:  true,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "unauthenticated user",
			userID:         "",
			userRole:       "",
			permission:     PermissionManageUsers,
			authenticated:  false,
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockStore := NewMockSessionStore()
			mockUserStore := new(MockUserStore)
			rbacService := NewRBACService(mockUserStore)

			// Setup session
			var session *sessions.Session
			if tt.authenticated {
				session = createAuthenticatedSession(tt.userID)
			} else {
				session = createUnauthenticatedSession()
			}

			// Setup user
			if tt.authenticated {
				user := &User{
					ID:   tt.userID,
					Role: tt.userRole,
				}
				mockUserStore.On("GetUser", mock.Anything, tt.userID).Return(user, nil)
			}

			// Create request and response
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			// Setup session store mock
			mockStore.On("Get", req, "tickit_session").Return(session, nil)

			// Create handler
			handler := RequireSystemPermission(mockStore, rbacService, tt.permission)(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Execute
			handler.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "success", w.Body.String())
			}

			mockStore.AssertExpectations(t)
			mockUserStore.AssertExpectations(t)
		})
	}
}

func TestRequireProjectPermission(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		userRole       string
		projectID      string
		projectRole    string
		permission     string
		authenticated  bool
		expectedStatus int
	}{
		{
			name:           "authenticated user with project permission",
			userID:         "user-1",
			userRole:       RoleDeveloper,
			projectID:      "project-1",
			projectRole:    ProjectRoleDeveloper,
			permission:     PermissionCreateIssue,
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "authenticated user without project permission",
			userID:         "user-1",
			userRole:       RoleDeveloper,
			projectID:      "project-1",
			projectRole:    ProjectRoleViewer,
			permission:     PermissionCreateIssue,
			authenticated:  true,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "admin user with any project permission",
			userID:         "admin-user",
			userRole:       RoleAdmin,
			projectID:      "project-1",
			projectRole:    "", // Not needed for admin
			permission:     PermissionCreateIssue,
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockStore := NewMockSessionStore()
			mockUserStore := new(MockUserStore)
			rbacService := NewRBACService(mockUserStore)

			// Setup session
			session := createAuthenticatedSession(tt.userID)

			// Setup user
			user := &User{
				ID:   tt.userID,
				Role: tt.userRole,
			}
			mockUserStore.On("GetUser", mock.Anything, tt.userID).Return(user, nil)

			// Setup project membership for non-admin users
			if tt.userRole != RoleAdmin {
				membership := &ProjectMembership{
					UserID:    tt.userID,
					ProjectID: tt.projectID,
					Role:      tt.projectRole,
				}
				mockUserStore.On("GetProjectMembership", mock.Anything, tt.userID, tt.projectID).Return(membership, nil)
			}

			// Create request with project ID in URL
			req := httptest.NewRequest("GET", "/projects/"+tt.projectID+"/test", nil)
			req = mux.SetURLVars(req, map[string]string{"projectId": tt.projectID})
			w := httptest.NewRecorder()

			// Setup session store mock
			mockStore.On("Get", req, "tickit_session").Return(session, nil)

			// Create handler
			handler := RequireProjectPermission(mockStore, rbacService, tt.permission)(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Execute
			handler.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "success", w.Body.String())
			}

			mockStore.AssertExpectations(t)
			mockUserStore.AssertExpectations(t)
		})
	}
}

func TestRequireProjectAccess(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		userRole       string
		projectID      string
		hasAccess      bool
		authenticated  bool
		expectedStatus int
	}{
		{
			name:           "authenticated user with project access",
			userID:         "user-1",
			userRole:       RoleDeveloper,
			projectID:      "project-1",
			hasAccess:      true,
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "authenticated user without project access",
			userID:         "user-1",
			userRole:       RoleDeveloper,
			projectID:      "project-1",
			hasAccess:      false,
			authenticated:  true,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "admin user always has access",
			userID:         "admin-user",
			userRole:       RoleAdmin,
			projectID:      "project-1",
			hasAccess:      false, // This shouldn't matter for admin
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockStore := NewMockSessionStore()
			mockUserStore := new(MockUserStore)
			rbacService := NewRBACService(mockUserStore)

			// Setup session
			session := createAuthenticatedSession(tt.userID)

			// Setup user
			user := &User{
				ID:   tt.userID,
				Role: tt.userRole,
			}
			mockUserStore.On("GetUser", mock.Anything, tt.userID).Return(user, nil)

			// Setup project access check for non-admin users
			if tt.userRole != RoleAdmin {
				mockUserStore.On("CheckUserProjectAccess", mock.Anything, tt.userID, tt.projectID).Return(tt.hasAccess, nil)
			}

			// Create request with project ID in URL
			req := httptest.NewRequest("GET", "/projects/"+tt.projectID+"/test", nil)
			req = mux.SetURLVars(req, map[string]string{"projectId": tt.projectID})
			w := httptest.NewRecorder()

			// Setup session store mock
			mockStore.On("Get", req, "tickit_session").Return(session, nil)

			// Create handler
			handler := RequireProjectAccess(mockStore, rbacService)(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Execute
			handler.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "success", w.Body.String())
			}

			mockStore.AssertExpectations(t)
			mockUserStore.AssertExpectations(t)
		})
	}
}

func TestRequireSystemRole(t *testing.T) {
	tests := []struct {
		name           string
		userID         string
		userRole       string
		requiredRole   string
		authenticated  bool
		expectedStatus int
	}{
		{
			name:           "admin user accessing admin endpoint",
			userID:         "admin-user",
			userRole:       RoleAdmin,
			requiredRole:   RoleAdmin,
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "project manager accessing admin endpoint",
			userID:         "pm-user",
			userRole:       RoleProjectManager,
			requiredRole:   RoleAdmin,
			authenticated:  true,
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "project manager accessing project manager endpoint",
			userID:         "pm-user",
			userRole:       RoleProjectManager,
			requiredRole:   RoleProjectManager,
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
		{
			name:           "admin accessing project manager endpoint (higher role)",
			userID:         "admin-user",
			userRole:       RoleAdmin,
			requiredRole:   RoleProjectManager,
			authenticated:  true,
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mocks
			mockStore := NewMockSessionStore()
			mockUserStore := new(MockUserStore)
			rbacService := NewRBACService(mockUserStore)

			// Setup session
			session := createAuthenticatedSession(tt.userID)

			// Setup user
			user := &User{
				ID:   tt.userID,
				Role: tt.userRole,
			}
			mockUserStore.On("GetUser", mock.Anything, tt.userID).Return(user, nil)

			// Create request and response
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()

			// Setup session store mock
			mockStore.On("Get", req, "tickit_session").Return(session, nil)

			// Create handler
			handler := RequireSystemRole(mockStore, rbacService, tt.requiredRole)(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("success"))
			})

			// Execute
			handler.ServeHTTP(w, req)

			// Assert
			assert.Equal(t, tt.expectedStatus, w.Code)

			if tt.expectedStatus == http.StatusOK {
				assert.Equal(t, "success", w.Body.String())
			}

			mockStore.AssertExpectations(t)
			mockUserStore.AssertExpectations(t)
		})
	}
}

func TestGetPermissionContextFromRequest(t *testing.T) {
	// Create a request with permission context
	req := httptest.NewRequest("GET", "/test", nil)

	expectedContext := &PermissionContext{
		UserID:     "test-user",
		ProjectID:  "test-project",
		Permission: "test-permission",
	}

	ctx := context.WithValue(req.Context(), "permission_context", expectedContext)
	req = req.WithContext(ctx)

	// Test getting context
	permCtx, ok := GetPermissionContextFromRequest(req)

	assert.True(t, ok)
	assert.Equal(t, expectedContext, permCtx)

	// Test with no context
	req2 := httptest.NewRequest("GET", "/test", nil)
	permCtx2, ok2 := GetPermissionContextFromRequest(req2)

	assert.False(t, ok2)
	assert.Nil(t, permCtx2)
}

func TestHasRequiredSystemRole(t *testing.T) {
	tests := []struct {
		name         string
		userRole     string
		requiredRole string
		expected     bool
	}{
		{
			name:         "admin has admin role",
			userRole:     RoleAdmin,
			requiredRole: RoleAdmin,
			expected:     true,
		},
		{
			name:         "admin has project manager role (higher)",
			userRole:     RoleAdmin,
			requiredRole: RoleProjectManager,
			expected:     true,
		},
		{
			name:         "project manager does not have admin role",
			userRole:     RoleProjectManager,
			requiredRole: RoleAdmin,
			expected:     false,
		},
		{
			name:         "developer has developer role",
			userRole:     RoleDeveloper,
			requiredRole: RoleDeveloper,
			expected:     true,
		},
		{
			name:         "project manager has developer role (higher)",
			userRole:     RoleProjectManager,
			requiredRole: RoleDeveloper,
			expected:     true,
		},
		{
			name:         "viewer does not have developer role",
			userRole:     RoleViewer,
			requiredRole: RoleDeveloper,
			expected:     false,
		},
		{
			name:         "invalid user role",
			userRole:     "InvalidRole",
			requiredRole: RoleAdmin,
			expected:     false,
		},
		{
			name:         "invalid required role",
			userRole:     RoleAdmin,
			requiredRole: "InvalidRole",
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasRequiredSystemRole(tt.userRole, tt.requiredRole)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHasRequiredProjectRole(t *testing.T) {
	tests := []struct {
		name         string
		userRole     string
		requiredRole string
		expected     bool
	}{
		{
			name:         "project admin has project admin role",
			userRole:     ProjectRoleAdmin,
			requiredRole: ProjectRoleAdmin,
			expected:     true,
		},
		{
			name:         "project admin has project developer role (higher)",
			userRole:     ProjectRoleAdmin,
			requiredRole: ProjectRoleDeveloper,
			expected:     true,
		},
		{
			name:         "project developer does not have project admin role",
			userRole:     ProjectRoleDeveloper,
			requiredRole: ProjectRoleAdmin,
			expected:     false,
		},
		{
			name:         "project developer has project developer role",
			userRole:     ProjectRoleDeveloper,
			requiredRole: ProjectRoleDeveloper,
			expected:     true,
		},
		{
			name:         "project developer has project viewer role (higher)",
			userRole:     ProjectRoleDeveloper,
			requiredRole: ProjectRoleViewer,
			expected:     true,
		},
		{
			name:         "project viewer does not have project developer role",
			userRole:     ProjectRoleViewer,
			requiredRole: ProjectRoleDeveloper,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := hasRequiredProjectRole(tt.userRole, tt.requiredRole)
			assert.Equal(t, tt.expected, result)
		})
	}
}
