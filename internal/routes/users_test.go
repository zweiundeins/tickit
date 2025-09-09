package routes

import (
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/users"
)

func TestUserRoutesRegistration(t *testing.T) {
	// Create a test router
	router := chi.NewRouter()

	// Create mock dependencies
	deps := &bootstrap.CQRSDeps{
		UserService: &users.UserService{},
	}

	// Create session store using centralized test configuration
	sessionStore := users.CreateTestSessionStore("test-secret")

	// Register user routes
	RegisterUserRoutes(router, deps, sessionStore)

	// Test that routes are registered by making a test request
	req := httptest.NewRequest("GET", "/auth/verify-email?token=test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// We expect some response (even if it's an error due to missing implementation)
	// The important thing is that the route is registered and accessible
	if w.Code == 404 {
		t.Error("User routes not properly registered - got 404")
	}
}
