package users

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/sessions"
	"github.com/justinas/nosurf"
	"github.com/mbolli/tickit/internal/session"
	"github.com/mbolli/tickit/internal/sharedhttp"
	"github.com/mbolli/tickit/internal/ui/pages"
)

// UserRoutes handles HTTP routes for user management
// Following the same pattern as other route handlers in the system
type UserRoutes struct {
	userService     *UserService
	sessionStore    sessions.Store
	authService     AuthService
	webauthnService WebAuthnService
}

// NewUserRoutes creates a new UserRoutes instance
func NewUserRoutes(
	userService *UserService,
	sessionStore sessions.Store,
	authService AuthService,
	webauthnService WebAuthnService,
) *UserRoutes {
	return &UserRoutes{
		userService:     userService,
		sessionStore:    sessionStore,
		authService:     authService,
		webauthnService: webauthnService,
	}
}

// Middleware methods

// requireAuthentication middleware ensures user is authenticated
func (ur *UserRoutes) RequireAuthentication(next http.Handler) http.Handler {
	return sharedhttp.RequireAuthentication(ur.sessionStore, func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

// requireAdminRole middleware ensures user has admin role
func (ur *UserRoutes) RequireAdminRole(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, authenticated := sharedhttp.GetAuthenticatedUser(ur.sessionStore, r)
		if !authenticated {
			http.Error(w, "Authentication required", http.StatusUnauthorized)
			return
		}

		// Get user to check role
		user, err := ur.userService.GetUserByID(r.Context(), userID)
		if err != nil {
			http.Error(w, "Failed to get user information", http.StatusInternalServerError)
			return
		}

		if user.Role != RoleAdmin {
			http.Error(w, "Admin role required", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Helper methods

// writeJSONResponse writes a JSON response with proper headers
func (ur *UserRoutes) WriteJSONResponse(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON response: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeJSONError writes a JSON error response
func (ur *UserRoutes) WriteJSONError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

// addCSRFTokenToResponse adds CSRF token to response for form-based endpoints
func (ur *UserRoutes) AddCSRFTokenToResponse(r *http.Request, response map[string]interface{}) {
	if token := nosurf.Token(r); token != "" {
		response["csrf_token"] = token
	}
}

// getUserFromSession gets the current user from session
func (ur *UserRoutes) GetUserFromSession(r *http.Request) (*User, error) {
	userID, authenticated := sharedhttp.GetAuthenticatedUser(ur.sessionStore, r)
	if !authenticated {
		return nil, fmt.Errorf("user not authenticated")
	}

	user, err := ur.userService.GetUserByID(r.Context(), userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	return user, nil
}

// Authentication handlers (stubs for now)

func (ur *UserRoutes) HandleRegister(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var req struct {
		Email     string `json:"email"`
		Password  string `json:"password"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Username  string `json:"username"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ur.WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Email == "" || req.Password == "" || req.Username == "" {
		ur.WriteJSONError(w, "Email, password, and username are required", http.StatusBadRequest)
		return
	}

	// Register user
	userID, err := ur.userService.RegisterUser(ctx, req.Email, req.Password, req.FirstName, req.LastName, req.Username)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			ur.WriteJSONError(w, "Email already registered", http.StatusConflict)
			return
		}
		ur.WriteJSONError(w, fmt.Sprintf("Registration failed: %v", err), http.StatusBadRequest)
		return
	}

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"message": "Registration successful. Please check your email for verification.",
		"user_id": userID,
	}

	// Add CSRF token for subsequent requests
	ur.AddCSRFTokenToResponse(r, response)

	w.WriteHeader(http.StatusCreated)
	ur.WriteJSONResponse(w, response)
}

// HandleTestSession tests if the current session is valid - useful for debugging
func (ur *UserRoutes) HandleTestSession(w http.ResponseWriter, r *http.Request) {
	// Check what cookies we have
	cookies := r.Cookies()
	cookieInfo := make(map[string]string)
	for _, cookie := range cookies {
		value := cookie.Value
		if len(value) > 20 {
			value = value[:20] + "..."
		}
		cookieInfo[cookie.Name] = value
	}

	// Try to get the session directly
	sessionName := session.GetSessionName()
	session, sessionErr := ur.sessionStore.Get(r, sessionName)

	userID, authenticated := sharedhttp.GetAuthenticatedUser(ur.sessionStore, r)

	response := map[string]interface{}{
		"authenticated":  authenticated,
		"user_id":        userID,
		"timestamp":      time.Now().Unix(),
		"cookies":        cookieInfo,
		"session_error":  nil,
		"session_values": nil,
	}

	if sessionErr != nil {
		response["session_error"] = sessionErr.Error()
	} else {
		response["session_values"] = session.Values
	}

	if authenticated {
		response["message"] = "Session is valid"
	} else {
		response["message"] = "No valid session found"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleClearCookies clears all session cookies - useful for debugging
func (ur *UserRoutes) HandleClearCookies(w http.ResponseWriter, r *http.Request) {
	// Clear user_session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     session.GetSessionName(),
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
	})

	// Clear connections cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "connections",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
	})

	// Clear oauth-session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth-session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
	})

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "All cookies cleared",
	})
}

func (ur *UserRoutes) HandleLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse form data
	if err := r.ParseForm(); err != nil {
		ur.renderLoginPageWithError(w, r, "Invalid form data")
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	// Validate required fields
	if email == "" || password == "" {
		ur.renderLoginPageWithError(w, r, "Email and password are required")
		return
	}

	// Get client IP and user agent
	ipAddress := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ipAddress = forwarded
	}
	userAgent := r.UserAgent()

	// Attempt login
	user, err := ur.userService.LoginUser(ctx, email, password, ipAddress, userAgent)
	if err != nil {
		// Generic error message to prevent user enumeration
		ur.renderLoginPageWithError(w, r, "Invalid credentials")
		return
	}

	// Create HTTP session
	sessionName := session.GetSessionName()
	session, err := ur.sessionStore.Get(r, sessionName)
	if err != nil {
		log.Printf("Failed to get session: %v", err)
		ur.renderLoginPageWithError(w, r, "Failed to create session")
		return
	}

	// Clear any existing session data first
	session.Values = make(map[interface{}]interface{})

	// Set session data
	session.Values["user_id"] = user.ID
	session.Values["authenticated"] = true

	// Ensure session options are properly set (they should inherit from store but let's be explicit)
	if session.Options == nil {
		session.Options = &sessions.Options{}
	}
	// Copy options from store if they exist
	if store, ok := ur.sessionStore.(*sessions.CookieStore); ok && len(store.Options.Path) > 0 {
		session.Options.Path = store.Options.Path
		session.Options.MaxAge = store.Options.MaxAge
		session.Options.HttpOnly = store.Options.HttpOnly
		session.Options.Secure = store.Options.Secure
		session.Options.SameSite = store.Options.SameSite
	}

	log.Printf("Attempting to save session for user: %s", user.ID)
	log.Printf("Session values before save: %+v", session.Values)
	log.Printf("Session options: MaxAge=%d, Secure=%v, HttpOnly=%v, Path=%s, SameSite=%v",
		session.Options.MaxAge, session.Options.Secure, session.Options.HttpOnly, session.Options.Path, session.Options.SameSite)
	log.Printf("Session store type: %T", ur.sessionStore)

	// Check if we can write headers
	if rw, ok := w.(http.ResponseWriter); ok {
		log.Printf("Response writer type: %T", rw)
	}

	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save session - Error type: %T, Error: %v", err, err)
		// Try to provide more specific error information
		if strings.Contains(err.Error(), "gob") {
			log.Printf("GOB encoding error - check session data types")
		}
		if strings.Contains(err.Error(), "header") {
			log.Printf("Header already written error")
		}
		ur.renderLoginPageWithError(w, r, fmt.Sprintf("Failed to save session: %v", err))
		return
	}

	log.Printf("Session saved successfully for user: %s", user.ID)

	// Log the Set-Cookie headers that will be sent
	if headers := w.Header()["Set-Cookie"]; len(headers) > 0 {
		for i, header := range headers {
			log.Printf("Set-Cookie header %d: %s", i, header)
		}
	} else {
		log.Printf("WARNING: No Set-Cookie headers found!")
	}

	log.Printf("About to redirect to /dashboard")

	// Check if we can read the session back immediately
	if testSession, testErr := ur.sessionStore.Get(r, sessionName); testErr == nil {
		if testUserID, ok := testSession.Values["user_id"].(string); ok {
			log.Printf("Session verification successful - can read back user_id: %s", testUserID)
		} else {
			log.Printf("Session verification failed - cannot read back user_id")
		}
	} else {
		log.Printf("Session verification failed - error reading session: %v", testErr)
	}

	// Redirect to dashboard on successful login
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// renderLoginPageWithError renders the login page with an error message
func (ur *UserRoutes) renderLoginPageWithError(w http.ResponseWriter, r *http.Request, errorMsg string) {
	pages.LoginPage(ur.sessionStore, r, errorMsg).Render(r.Context(), w)
}

func (ur *UserRoutes) HandleLogout(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get current session
	sessionName := session.GetSessionName()
	session, err := ur.sessionStore.Get(r, sessionName)
	if err != nil {
		// If we can't get the session, just redirect to login
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
		return
	}

	userID, _ := session.Values["user_id"].(string)
	sessionID, _ := session.Values["session_id"].(string)

	// Call logout service if we have user and session info
	if userID != "" && sessionID != "" {
		if err := ur.userService.LogoutUser(ctx, sessionID, userID); err != nil {
			// Log error but continue with session cleanup
			log.Printf("Failed to logout user in service: %v", err)
		}
	}

	// Clear session
	session.Values = make(map[interface{}]interface{})
	session.Options.MaxAge = -1

	if err := session.Save(r, w); err != nil {
		// Log error but continue with redirect
		log.Printf("Failed to save cleared session: %v", err)
	}

	// Redirect to login page
	http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
}

func (ur *UserRoutes) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get token from query parameter
	token := r.URL.Query().Get("token")
	if token == "" {
		ur.WriteJSONError(w, "Verification token is required", http.StatusBadRequest)
		return
	}

	// Verify email
	if err := ur.userService.VerifyEmail(ctx, token); err != nil {
		if strings.Contains(err.Error(), "invalid token") {
			ur.WriteJSONError(w, "Invalid or expired verification token", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "already verified") {
			ur.WriteJSONError(w, "Email already verified", http.StatusBadRequest)
			return
		}
		ur.WriteJSONError(w, "Email verification failed", http.StatusInternalServerError)
		return
	}

	// Return success response
	ur.WriteJSONResponse(w, map[string]string{
		"message": "Email verified successfully",
	})
}

// OAuth handlers (stubs for now)

func (ur *UserRoutes) HandleOAuthLogin(w http.ResponseWriter, r *http.Request) {
	// 1. Get provider from URL params
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		http.Error(w, "Provider is required", http.StatusBadRequest)
		return
	}

	// 2. Normalize provider name
	provider = NormalizeProviderName(provider)

	// 3. Validate provider is supported
	if !IsProviderSupported(provider) {
		http.Error(w, fmt.Sprintf("OAuth provider %s is not supported", provider), http.StatusBadRequest)
		return
	}

	// 4. Initialize OAuth manager
	oauthManager := NewOAuthManager()
	if err := oauthManager.LoadConfigFromEnvironment(); err != nil {
		log.Printf("Failed to load OAuth configuration: %v", err)
		http.Error(w, "OAuth configuration error", http.StatusInternalServerError)
		return
	}

	// 5. Check if provider is enabled
	if !oauthManager.IsProviderEnabled(provider) {
		http.Error(w, fmt.Sprintf("OAuth provider %s is not enabled", provider), http.StatusBadRequest)
		return
	}

	// 6. Get provider configuration
	config, err := oauthManager.GetProviderConfig(provider)
	if err != nil {
		log.Printf("Failed to get provider config: %v", err)
		http.Error(w, "OAuth provider configuration error", http.StatusInternalServerError)
		return
	}

	// 7. Generate state parameter for CSRF protection
	state := generateSecureState()

	// 8. Store state in session for validation
	session, err := ur.sessionStore.Get(r, "oauth-session")
	if err != nil {
		log.Printf("Failed to get OAuth session: %v", err)
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	session.Values["oauth_state"] = state
	session.Values["oauth_provider"] = provider
	session.Values["oauth_timestamp"] = time.Now().Unix()

	if err := session.Save(r, w); err != nil {
		log.Printf("Failed to save OAuth session: %v", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	// 9. Build OAuth authorization URL
	authURL := fmt.Sprintf("https://github.com/login/oauth/authorize?client_id=%s&redirect_uri=%s&scope=%s&state=%s",
		config.ClientID,
		config.CallbackURL,
		strings.Join(config.Scopes, " "),
		state)

	// For other providers, we'd need different URLs
	switch provider {
	case "google":
		authURL = fmt.Sprintf("https://accounts.google.com/o/oauth2/auth?client_id=%s&redirect_uri=%s&scope=%s&response_type=code&state=%s",
			config.ClientID,
			config.CallbackURL,
			strings.Join(config.Scopes, " "),
			state)
	case "microsoft":
		authURL = fmt.Sprintf("https://login.microsoftonline.com/common/oauth2/v2.0/authorize?client_id=%s&redirect_uri=%s&scope=%s&response_type=code&state=%s",
			config.ClientID,
			config.CallbackURL,
			strings.Join(config.Scopes, " "),
			state)
	}

	// 10. Redirect to OAuth provider
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

func (ur *UserRoutes) HandleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// 1. Get provider from URL params
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		http.Error(w, "Provider is required", http.StatusBadRequest)
		return
	}

	// 2. Get code and state from query params
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" {
		http.Error(w, "Authorization code is required", http.StatusBadRequest)
		return
	}

	if state == "" {
		http.Error(w, "State parameter is required", http.StatusBadRequest)
		return
	}

	// 3. Validate state parameter from session
	oauthSession, err := ur.sessionStore.Get(r, "oauth-session")
	if err != nil {
		log.Printf("Failed to get OAuth session: %v", err)
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	storedState, ok := oauthSession.Values["oauth_state"].(string)
	if !ok || storedState != state {
		http.Error(w, "Invalid state parameter", http.StatusBadRequest)
		return
	}

	storedProvider, ok := oauthSession.Values["oauth_provider"].(string)
	if !ok || storedProvider != provider {
		http.Error(w, "Provider mismatch", http.StatusBadRequest)
		return
	}

	// Check session timestamp (10 minute expiry)
	timestamp, ok := oauthSession.Values["oauth_timestamp"].(int64)
	if !ok || time.Now().Unix()-timestamp > 600 {
		http.Error(w, "OAuth session expired", http.StatusBadRequest)
		return
	}

	// 4. Get client IP and user agent
	ipAddress := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ipAddress = forwarded
	}
	userAgent := r.Header.Get("User-Agent")

	// 5. Execute OAuth login command
	cmd := LoginOAuthCommand{
		Provider:  provider,
		Code:      code,
		State:     state,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}

	ur.userService.commandBus.Execute(ctx, cmd)

	// For now, we'll return a placeholder response since the OAuth integration
	// requires more complex setup with Goth and proper token exchange
	// This is a simplified implementation that shows the structure

	// 6. Clear OAuth session
	oauthSession.Values = make(map[interface{}]interface{})
	oauthSession.Save(r, w)

	// 7. Return success response (in a real implementation, this would redirect)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message":  "OAuth callback received successfully",
		"note":     "Full OAuth implementation requires proper token exchange and user creation",
		"provider": provider,
		"code":     code[:10] + "...", // Show partial code for debugging
	})
}

// WebAuthn handlers (stubs for now)

func (ur *UserRoutes) HandleWebAuthnRegisterBegin(w http.ResponseWriter, r *http.Request) {
	// Get user ID from session
	sessionName := session.GetSessionName()
	session, err := ur.sessionStore.Get(r, sessionName)
	if err != nil {
		http.Error(w, "Failed to get session", http.StatusInternalServerError)
		return
	}

	userID, ok := session.Values["user_id"].(string)
	if !ok || userID == "" {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Begin WebAuthn registration
	_, err = ur.authService.BeginMFARegistration(r.Context(), userID)
	if err != nil {
		log.Printf("Failed to begin WebAuthn registration: %v", err)
		http.Error(w, "Failed to begin registration", http.StatusInternalServerError)
		return
	}

	// Get the registration options from the WebAuthn service
	registrationOptions, err := ur.webauthnService.BeginRegistration(r.Context(), userID)
	if err != nil {
		log.Printf("Failed to get registration options: %v", err)
		http.Error(w, "Failed to get registration options", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(registrationOptions)
}

func (ur *UserRoutes) HandleWebAuthnRegisterFinish(w http.ResponseWriter, r *http.Request) {
	// Get user ID from session
	sessionName := session.GetSessionName()
	session, err := ur.sessionStore.Get(r, sessionName)
	if err != nil {
		http.Error(w, "Failed to get session", http.StatusInternalServerError)
		return
	}

	userID, ok := session.Values["user_id"].(string)
	if !ok || userID == "" {
		http.Error(w, "User not authenticated", http.StatusUnauthorized)
		return
	}

	// Read the WebAuthn response from the request body
	var response json.RawMessage
	err = json.NewDecoder(r.Body).Decode(&response)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Finish WebAuthn registration
	err = ur.authService.FinishMFARegistration(r.Context(), userID, response)
	if err != nil {
		log.Printf("Failed to finish WebAuthn registration: %v", err)
		http.Error(w, "Failed to finish registration", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (ur *UserRoutes) HandleWebAuthnLoginBegin(w http.ResponseWriter, r *http.Request) {
	// Parse request to get user identifier (email or userID)
	var req struct {
		Email  string `json:"email,omitempty"`
		UserID string `json:"user_id,omitempty"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var userID string
	if req.UserID != "" {
		userID = req.UserID
	} else if req.Email != "" {
		// Get user by email
		user, err := ur.userService.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		userID = user.ID
	} else {
		http.Error(w, "Email or user_id required", http.StatusBadRequest)
		return
	}

	// Check if user has MFA enabled
	hasMFA, err := ur.authService.CheckUserMFAStatus(r.Context(), userID)
	if err != nil {
		log.Printf("Failed to check MFA status: %v", err)
		http.Error(w, "Failed to check MFA status", http.StatusInternalServerError)
		return
	}

	if !hasMFA {
		http.Error(w, "MFA not enabled for this user", http.StatusBadRequest)
		return
	}

	// Begin WebAuthn login
	loginOptions, err := ur.webauthnService.BeginLogin(r.Context(), userID)
	if err != nil {
		log.Printf("Failed to begin WebAuthn login: %v", err)
		http.Error(w, "Failed to begin login", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(loginOptions)
}

func (ur *UserRoutes) HandleWebAuthnLoginFinish(w http.ResponseWriter, r *http.Request) {
	// Parse request to get user identifier and WebAuthn response
	var req struct {
		Email    string          `json:"email,omitempty"`
		UserID   string          `json:"user_id,omitempty"`
		Response json.RawMessage `json:"response"`
	}

	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var userID string
	if req.UserID != "" {
		userID = req.UserID
	} else if req.Email != "" {
		// Get user by email
		user, err := ur.userService.GetUserByEmail(r.Context(), req.Email)
		if err != nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}
		userID = user.ID
	} else {
		http.Error(w, "Email or user_id required", http.StatusBadRequest)
		return
	}

	// Get client IP and user agent
	ipAddress := GetClientIP(r)
	userAgent := r.UserAgent()

	// Finish WebAuthn login
	user, userSession, err := ur.authService.LoginWebAuthn(r.Context(), userID, req.Response, ipAddress, userAgent)
	if err != nil {
		log.Printf("Failed to finish WebAuthn login: %v", err)
		http.Error(w, "Failed to finish login", http.StatusUnauthorized)
		return
	}

	// Create session
	sessionName := session.GetSessionName()
	httpSession, err := ur.sessionStore.Get(r, sessionName)
	if err != nil {
		log.Printf("Failed to get session: %v", err)
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	httpSession.Values["user_id"] = user.ID
	httpSession.Values["authenticated"] = true
	httpSession.Values["login_time"] = time.Now()

	err = httpSession.Save(r, w)
	if err != nil {
		log.Printf("Failed to save session: %v", err)
		http.Error(w, "Failed to save session", http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"user":    user,
		"session": userSession,
	})
}

// Password management handlers

func (ur *UserRoutes) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var req struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate email is provided
	if req.Email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	// Execute forgot password command using auth service
	if err := ur.authService.ForgotPassword(ctx, req.Email); err != nil {
		if strings.Contains(err.Error(), "invalid email") {
			http.Error(w, "Invalid email address", http.StatusBadRequest)
			return
		}

		// Log error but return generic success message for security
		// This prevents user enumeration attacks
		log.Printf("Failed to process forgot password request: %v", err)
	}

	// Always return success to prevent user enumeration
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "If an account with that email exists, a password reset link has been sent",
	})
}

func (ur *UserRoutes) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var req struct {
		Token       string `json:"token"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.Token == "" {
		http.Error(w, "Reset token is required", http.StatusBadRequest)
		return
	}

	if req.NewPassword == "" {
		http.Error(w, "New password is required", http.StatusBadRequest)
		return
	}

	// Execute reset password command using auth service
	if err := ur.authService.ResetPassword(ctx, req.Token, req.NewPassword); err != nil {
		if strings.Contains(err.Error(), "invalid reset token") {
			http.Error(w, "Invalid or expired reset token", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "reset token has expired") {
			http.Error(w, "Reset token has expired", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "reset token has already been used") {
			http.Error(w, "Reset token has already been used", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "invalid password") {
			http.Error(w, "Password does not meet requirements", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "account is disabled") {
			http.Error(w, "Account is disabled", http.StatusForbidden)
			return
		}

		// Log error but return generic message
		log.Printf("Failed to reset password: %v", err)
		http.Error(w, "Failed to reset password", http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Password has been reset successfully. All sessions have been invalidated.",
	})
}

func (ur *UserRoutes) HandleChangePassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user from session
	user, err := ur.GetUserFromSession(r)
	if err != nil {
		ur.WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var req struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ur.WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.OldPassword == "" || req.NewPassword == "" {
		ur.WriteJSONError(w, "Old password and new password are required", http.StatusBadRequest)
		return
	}

	// Change password
	if err := ur.userService.ChangePassword(ctx, user.ID, req.OldPassword, req.NewPassword); err != nil {
		if strings.Contains(err.Error(), "invalid password") {
			ur.WriteJSONError(w, "Current password is incorrect", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "password requirements") {
			ur.WriteJSONError(w, "New password does not meet requirements", http.StatusBadRequest)
			return
		}
		ur.WriteJSONError(w, "Failed to change password", http.StatusInternalServerError)
		return
	}

	ur.WriteJSONResponse(w, map[string]string{
		"message": "Password changed successfully. All sessions have been invalidated.",
	})
}

// Magic mail handlers

func (ur *UserRoutes) HandleSendMagicMail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse request body
	var req struct {
		Email string `json:"email"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Execute command using auth service
	if err := ur.authService.SendMagicMail(ctx, req.Email); err != nil {
		if err.Error() == "magic mail is only available in development mode" {
			http.Error(w, "Magic mail is only available in development mode", http.StatusForbidden)
			return
		}
		if strings.Contains(err.Error(), "invalid email") {
			http.Error(w, "Invalid email address", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "email not verified") {
			http.Error(w, "Email not verified", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "account is disabled") {
			http.Error(w, "Account is disabled", http.StatusForbidden)
			return
		}

		// Log error but return generic message for security
		log.Printf("Failed to send magic mail: %v", err)
		http.Error(w, "Failed to send magic mail", http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Magic mail sent successfully",
	})
}

func (ur *UserRoutes) HandleMagicMailLogin(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get token from query parameter
	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	// Get client IP and user agent
	ipAddress := r.RemoteAddr
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ipAddress = forwarded
	}
	userAgent := r.Header.Get("User-Agent")

	// Execute command using auth service
	_, _, err := ur.authService.LoginMagicMail(ctx, token, ipAddress, userAgent)
	if err != nil {
		if strings.Contains(err.Error(), "invalid magic mail token") {
			http.Error(w, "Invalid or expired token", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "magic mail token has expired") {
			http.Error(w, "Token has expired", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "magic mail token has already been used") {
			http.Error(w, "Token has already been used", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "email not verified") {
			http.Error(w, "Email not verified", http.StatusBadRequest)
			return
		}
		if strings.Contains(err.Error(), "account is disabled") {
			http.Error(w, "Account is disabled", http.StatusForbidden)
			return
		}

		// Log error but return generic message
		log.Printf("Failed to login with magic mail: %v", err)
		http.Error(w, "Login failed", http.StatusInternalServerError)
		return
	}

	// For a web application, you might want to redirect to dashboard
	// For now, return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Login successful",
	})
}

// Profile management handlers (stubs for now)

func (ur *UserRoutes) HandleGetProfile(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	user, err := ur.GetUserFromSession(r)
	if err != nil {
		ur.WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Return user profile
	response := map[string]interface{}{
		"user": user,
	}

	// Add CSRF token for subsequent requests
	ur.AddCSRFTokenToResponse(r, response)

	ur.WriteJSONResponse(w, response)
}

func (ur *UserRoutes) HandleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// 1. Get user from session
	user, err := ur.GetUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Parse request body
	var updateRequest struct {
		Email     *string `json:"email,omitempty"`
		Username  *string `json:"username,omitempty"`
		FirstName *string `json:"first_name,omitempty"`
		LastName  *string `json:"last_name,omitempty"`
		AvatarURL *string `json:"avatar_url,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&updateRequest); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// 3. Create update command
	cmd := UpdateUserProfileCommand{
		UserID:    user.ID,
		Email:     updateRequest.Email,
		Username:  updateRequest.Username,
		FirstName: updateRequest.FirstName,
		LastName:  updateRequest.LastName,
		AvatarURL: updateRequest.AvatarURL,
	}

	ur.userService.commandBus.Execute(ctx, cmd)

	// 4. Update profile using service
	if err := ur.userService.UpdateUserProfile(r.Context(), user.ID, updateRequest.Email, updateRequest.Username, updateRequest.FirstName, updateRequest.LastName, updateRequest.AvatarURL); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update profile: %v", err), http.StatusBadRequest)
		return
	}

	// 5. Get updated user data
	updatedUser, err := ur.userService.GetUserByID(r.Context(), user.ID)
	if err != nil {
		http.Error(w, "Failed to retrieve updated profile", http.StatusInternalServerError)
		return
	}

	// 6. Return updated profile
	response := map[string]interface{}{
		"success": true,
		"user":    updatedUser,
		"message": "Profile updated successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (ur *UserRoutes) HandleUploadAvatar(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	// 1. Get user from session
	user, err := ur.GetUserFromSession(r)
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Limit request size to 5MB
	r.Body = http.MaxBytesReader(w, r.Body, 5<<20) // 5MB max

	// 3. Parse multipart form
	if err := r.ParseMultipartForm(5 << 20); err != nil {
		http.Error(w, "File too large", http.StatusRequestEntityTooLarge)
		return
	}

	// 4. Get file from form
	file, header, err := r.FormFile("avatar")
	if err != nil {
		http.Error(w, "Invalid file", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// 5. Validate file type
	if !isValidImageType(header.Header.Get("Content-Type")) {
		http.Error(w, "Invalid file type. Only JPEG, PNG, GIF, and WebP are allowed", http.StatusBadRequest)
		return
	}

	// 6. Validate file size (additional check)
	if header.Size > 5<<20 {
		http.Error(w, "File too large. Maximum size is 5MB", http.StatusRequestEntityTooLarge)
		return
	}

	// 7. Generate unique filename
	ext := getFileExtension(header.Filename)
	filename := fmt.Sprintf("avatar_%s_%d%s", user.ID, time.Now().Unix(), ext)

	// 8. Create upload directory if it doesn't exist
	uploadDir := "upload"
	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		http.Error(w, "Unable to create upload directory", http.StatusInternalServerError)
		return
	}

	// 9. Save file to disk
	filePath := filepath.Join(uploadDir, filename)
	out, err := os.Create(filePath)
	if err != nil {
		http.Error(w, "Unable to save file", http.StatusInternalServerError)
		return
	}
	defer out.Close()

	// 10. Copy file content
	if _, err := io.Copy(out, file); err != nil {
		// Clean up partial file
		os.Remove(filePath)
		http.Error(w, "Unable to save file", http.StatusInternalServerError)
		return
	}

	// 11. Update user profile with new avatar URL
	avatarURL := "/uploads/" + filename
	cmd := UpdateUserProfileCommand{
		UserID:    user.ID,
		AvatarURL: &avatarURL,
	}
	ur.userService.commandBus.Execute(ctx, cmd)

	if err := ur.userService.UpdateUserProfile(r.Context(), user.ID, nil, nil, nil, nil, &avatarURL); err != nil {
		// Clean up uploaded file if profile update fails
		os.Remove(filePath)
		http.Error(w, fmt.Sprintf("Failed to update profile: %v", err), http.StatusInternalServerError)
		return
	}

	// 12. Return success response
	response := map[string]interface{}{
		"success":    true,
		"avatar_url": avatarURL,
		"message":    "Avatar uploaded successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Project membership handlers (stubs for now)

func (ur *UserRoutes) HandleGetProjectMembers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get project ID from URL params
	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		ur.WriteJSONError(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Get project members
	members, err := ur.userService.GetProjectMembers(ctx, projectID)
	if err != nil {
		ur.WriteJSONError(w, "Failed to get project members", http.StatusInternalServerError)
		return
	}

	// Return members
	response := map[string]interface{}{
		"members": members,
	}

	ur.AddCSRFTokenToResponse(r, response)
	ur.WriteJSONResponse(w, response)
}

func (ur *UserRoutes) HandleInviteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get project ID from URL params
	projectID := chi.URLParam(r, "projectId")
	if projectID == "" {
		http.Error(w, "Project ID is required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Get current user from session
	sessionName := session.GetSessionName()
	session, err := ur.sessionStore.Get(r, sessionName)
	if err != nil {
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	userID, ok := session.Values["user_id"].(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Execute command
	if err := ur.userService.InviteUserToProject(ctx, projectID, req.Email, req.Role, userID); err != nil {
		log.Printf("Failed to invite user: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Invitation sent successfully",
	})
}

func (ur *UserRoutes) HandleRemoveUserFromProject(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get project ID and user ID from URL params
	projectID := chi.URLParam(r, "projectId")
	userID := chi.URLParam(r, "userId")

	if projectID == "" || userID == "" {
		ur.WriteJSONError(w, "Project ID and User ID are required", http.StatusBadRequest)
		return
	}

	// Get current user from session
	currentUser, err := ur.GetUserFromSession(r)
	if err != nil {
		ur.WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Remove user from project
	if err := ur.userService.RemoveUserFromProject(ctx, userID, projectID, currentUser.ID); err != nil {
		ur.WriteJSONError(w, fmt.Sprintf("Failed to remove user from project: %v", err), http.StatusBadRequest)
		return
	}

	ur.WriteJSONResponse(w, map[string]string{
		"message": "User removed from project successfully",
	})
}

func (ur *UserRoutes) HandleUpdateMemberRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get project ID and user ID from URL params
	projectID := chi.URLParam(r, "projectId")
	userID := chi.URLParam(r, "userId")

	if projectID == "" || userID == "" {
		ur.WriteJSONError(w, "Project ID and User ID are required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req struct {
		Role string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		ur.WriteJSONError(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Role == "" {
		ur.WriteJSONError(w, "Role is required", http.StatusBadRequest)
		return
	}

	// Get current user from session
	currentUser, err := ur.GetUserFromSession(r)
	if err != nil {
		ur.WriteJSONError(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Update member role
	if err := ur.userService.UpdateProjectMembership(ctx, userID, projectID, req.Role, currentUser.ID); err != nil {
		ur.WriteJSONError(w, fmt.Sprintf("Failed to update member role: %v", err), http.StatusBadRequest)
		return
	}

	ur.WriteJSONResponse(w, map[string]string{
		"message": "Member role updated successfully",
	})
}

// Invitation handlers (stubs for now)

func (ur *UserRoutes) HandleGetUserInvitations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get current user from session
	sessionName := session.GetSessionName()
	session, err := ur.sessionStore.Get(r, sessionName)
	if err != nil {
		http.Error(w, "Session error", http.StatusInternalServerError)
		return
	}

	userID, ok := session.Values["user_id"].(string)
	if !ok || userID == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get user to get their email
	user, err := ur.userService.GetUserByID(ctx, userID)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Get user invitations
	invitations, err := ur.userService.GetUserInvitations(ctx, user.Email)
	if err != nil {
		log.Printf("Failed to get user invitations: %v", err)
		http.Error(w, "Failed to get invitations", http.StatusInternalServerError)
		return
	}

	// Return invitations
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(invitations)
}

func (ur *UserRoutes) HandleAcceptInvitation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get token from URL params
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	// Execute command
	if err := ur.userService.AcceptProjectInvitation(ctx, token); err != nil {
		log.Printf("Failed to accept invitation: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Invitation accepted successfully",
	})
}

func (ur *UserRoutes) HandleDeclineInvitation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get token from URL params
	token := chi.URLParam(r, "token")
	if token == "" {
		http.Error(w, "Token is required", http.StatusBadRequest)
		return
	}

	// Execute command
	if err := ur.userService.DeclineProjectInvitation(ctx, token); err != nil {
		log.Printf("Failed to decline invitation: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Invitation declined successfully",
	})
}

// Administrative handlers (stubs for now)

func (ur *UserRoutes) HandleListUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Check admin permissions (in a real implementation, this would be middleware)
	// For now, we'll assume the user is authenticated and has admin role

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")
	status := r.URL.Query().Get("status")
	role := r.URL.Query().Get("role")
	search := r.URL.Query().Get("search")
	export := r.URL.Query().Get("export")

	// Set default values
	limit := 50
	offset := 0

	// Parse limit and offset
	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	var users []*User
	var err error

	// Handle search vs list
	if search != "" {
		users, err = ur.userService.SearchUsers(ctx, search, limit, offset)
	} else {
		users, err = ur.userService.ListUsers(ctx, limit, offset, status, role)
	}

	if err != nil {
		log.Printf("Failed to get users: %v", err)
		http.Error(w, "Failed to get users", http.StatusInternalServerError)
		return
	}

	// Handle export functionality
	if export != "" {
		ur.HandleExportUsers(w, r, export)
		return
	}

	// Enhance user data with additional info for admin view
	type AdminUserView struct {
		*User
		MFAEnabled   bool     `json:"mfa_enabled"`
		AuthMethods  []string `json:"auth_methods"`
		SessionCount int      `json:"session_count"`
		ProjectCount int      `json:"project_count"`
	}

	adminUsers := make([]*AdminUserView, len(users))
	for i, user := range users {
		// Get additional user info
		mfaEnabled, _ := ur.userService.CheckUserMFAStatus(ctx, user.ID)
		credentials, _ := ur.userService.GetUserCredentials(ctx, user.ID)
		sessions, _ := ur.userService.GetUserSessions(ctx, user.ID)
		projects, _ := ur.userService.GetUserProjects(ctx, user.ID)

		// Extract auth methods
		authMethods := make([]string, 0)
		for _, cred := range credentials {
			if cred.Type == "oauth" {
				authMethods = append(authMethods, cred.Provider)
			} else {
				authMethods = append(authMethods, cred.Type)
			}
		}

		adminUsers[i] = &AdminUserView{
			User:         user,
			MFAEnabled:   mfaEnabled,
			AuthMethods:  authMethods,
			SessionCount: len(sessions),
			ProjectCount: len(projects),
		}
	}

	// Return response
	response := map[string]interface{}{
		"users":  adminUsers,
		"limit":  limit,
		"offset": offset,
		"total":  len(adminUsers), // In a real implementation, you'd get total count separately
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode users response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (ur *UserRoutes) HandleGetUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user ID from URL params
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Get user
	user, err := ur.userService.GetUserByID(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user: %v", err)
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	// Get additional user information for admin view
	userStats, _ := ur.userService.GetUserStats(ctx, userID)
	userActivity, _ := ur.userService.GetUserActivity(ctx, userID, 10, 0)
	credentials, _ := ur.userService.GetUserCredentials(ctx, userID)
	sessions, _ := ur.userService.GetUserSessions(ctx, userID)
	projects, _ := ur.userService.GetUserProjects(ctx, userID)
	mfaEnabled, _ := ur.userService.CheckUserMFAStatus(ctx, userID)

	// Extract auth methods
	authMethods := make([]string, 0)
	for _, cred := range credentials {
		if cred.Type == "oauth" {
			authMethods = append(authMethods, cred.Provider)
		} else {
			authMethods = append(authMethods, cred.Type)
		}
	}

	// Build detailed response
	response := map[string]interface{}{
		"user":         user,
		"stats":        userStats,
		"activity":     userActivity,
		"auth_methods": authMethods,
		"mfa_enabled":  mfaEnabled,
		"sessions":     sessions,
		"projects":     projects,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode user response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (ur *UserRoutes) HandleUpdateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user ID from URL params
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var updateRequest struct {
		Email         *string `json:"email,omitempty"`
		Username      *string `json:"username,omitempty"`
		FirstName     *string `json:"first_name,omitempty"`
		LastName      *string `json:"last_name,omitempty"`
		AvatarURL     *string `json:"avatar_url,omitempty"`
		Role          *string `json:"role,omitempty"`
		Status        *string `json:"status,omitempty"`
		EmailVerified *bool   `json:"email_verified,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&updateRequest); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Get current admin user (in a real implementation, this would come from session)
	adminUserID := "admin" // Placeholder

	// Create patch input
	patchInput := PatchUserInput{
		ID:            userID,
		Email:         updateRequest.Email,
		Username:      updateRequest.Username,
		FirstName:     updateRequest.FirstName,
		LastName:      updateRequest.LastName,
		AvatarURL:     updateRequest.AvatarURL,
		Role:          updateRequest.Role,
		Status:        updateRequest.Status,
		EmailVerified: updateRequest.EmailVerified,
	}

	// Update user
	if err := ur.userService.PatchUser(ctx, patchInput, adminUserID); err != nil {
		log.Printf("Failed to update user: %v", err)
		http.Error(w, fmt.Sprintf("Failed to update user: %v", err), http.StatusBadRequest)
		return
	}

	// Get updated user
	updatedUser, err := ur.userService.GetUserByID(ctx, userID)
	if err != nil {
		log.Printf("Failed to get updated user: %v", err)
		http.Error(w, "Failed to retrieve updated user", http.StatusInternalServerError)
		return
	}

	// Return updated user
	response := map[string]interface{}{
		"success": true,
		"user":    updatedUser,
		"message": "User updated successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode update response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (ur *UserRoutes) HandleDeactivateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user ID from URL params
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Deactivate user
	if err := ur.userService.DeactivateUser(ctx, userID); err != nil {
		log.Printf("Failed to deactivate user: %v", err)
		http.Error(w, fmt.Sprintf("Failed to deactivate user: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"message": "User deactivated successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode deactivate response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (ur *UserRoutes) HandleReactivateUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user ID from URL params
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Reactivate user
	if err := ur.userService.ReactivateUser(ctx, userID); err != nil {
		log.Printf("Failed to reactivate user: %v", err)
		http.Error(w, fmt.Sprintf("Failed to reactivate user: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"message": "User reactivated successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode reactivate response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (ur *UserRoutes) HandleAssignRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user ID from URL params
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var roleRequest struct {
		Role string `json:"role"`
	}

	if err := json.NewDecoder(r.Body).Decode(&roleRequest); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	if roleRequest.Role == "" {
		http.Error(w, "Role is required", http.StatusBadRequest)
		return
	}

	// Get current admin user (in a real implementation, this would come from session)
	adminUserID := "admin" // Placeholder

	// Assign role
	if err := ur.userService.AssignUserRole(ctx, userID, roleRequest.Role, adminUserID); err != nil {
		log.Printf("Failed to assign role: %v", err)
		http.Error(w, fmt.Sprintf("Failed to assign role: %v", err), http.StatusBadRequest)
		return
	}

	// Return success response
	response := map[string]interface{}{
		"success": true,
		"message": fmt.Sprintf("Role '%s' assigned successfully", roleRequest.Role),
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode role assignment response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// Export handler for user data
func (ur *UserRoutes) HandleExportUsers(w http.ResponseWriter, r *http.Request, format string) {
	ctx := r.Context()

	// Export user data
	data, err := ur.userService.ExportUserData(ctx, format)
	if err != nil {
		log.Printf("Failed to export user data: %v", err)
		http.Error(w, fmt.Sprintf("Failed to export user data: %v", err), http.StatusInternalServerError)
		return
	}

	// Set appropriate headers based on format
	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", "attachment; filename=users.json")
	case "csv":
		w.Header().Set("Content-Type", "text/csv")
		w.Header().Set("Content-Disposition", "attachment; filename=users.csv")
	default:
		http.Error(w, "Unsupported export format", http.StatusBadRequest)
		return
	}

	// Write data
	w.Write(data)
}

// User activity handler
func (ur *UserRoutes) HandleGetUserActivity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user ID from URL params
	userID := chi.URLParam(r, "userId")
	if userID == "" {
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 50
	offset := 0

	if limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}
	if offsetStr != "" {
		if parsedOffset, err := strconv.Atoi(offsetStr); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Get user activity
	activity, err := ur.userService.GetUserActivity(ctx, userID, limit, offset)
	if err != nil {
		log.Printf("Failed to get user activity: %v", err)
		http.Error(w, "Failed to get user activity", http.StatusInternalServerError)
		return
	}

	// Return activity
	response := map[string]interface{}{
		"activity": activity,
		"limit":    limit,
		"offset":   offset,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("Failed to encode activity response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// System stats handler
func (ur *UserRoutes) HandleGetSystemStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get system user stats
	stats, err := ur.userService.GetSystemUserStats(ctx)
	if err != nil {
		log.Printf("Failed to get system stats: %v", err)
		http.Error(w, "Failed to get system stats", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("Failed to encode stats response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

// Session management handlers (stubs for now)

func (ur *UserRoutes) HandleGetUserSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user from session (this would require authentication middleware)
	// For now, we'll get user ID from query parameter or session
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		// In a real implementation, you'd get this from the authenticated session
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Get user sessions
	sessions, err := ur.userService.GetUserSessions(ctx, userID)
	if err != nil {
		log.Printf("Failed to get user sessions: %v", err)
		http.Error(w, "Failed to get user sessions", http.StatusInternalServerError)
		return
	}

	// Return sessions as JSON
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(sessions); err != nil {
		log.Printf("Failed to encode sessions response: %v", err)
		http.Error(w, "Failed to encode response", http.StatusInternalServerError)
		return
	}
}

func (ur *UserRoutes) HandleInvalidateSession(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get session ID from URL parameters
	sessionID := chi.URLParam(r, "sessionId")
	if sessionID == "" {
		http.Error(w, "Session ID is required", http.StatusBadRequest)
		return
	}

	// Invalidate the session
	if err := ur.userService.InvalidateUserSession(ctx, sessionID); err != nil {
		if strings.Contains(err.Error(), "not found") {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
		log.Printf("Failed to invalidate session: %v", err)
		http.Error(w, "Failed to invalidate session", http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Session invalidated successfully",
	})
}

func (ur *UserRoutes) HandleInvalidateAllSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Get user ID from query parameter or authenticated session
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		// In a real implementation, you'd get this from the authenticated session
		http.Error(w, "User ID is required", http.StatusBadRequest)
		return
	}

	// Invalidate all user sessions
	if err := ur.userService.InvalidateAllUserSessions(ctx, userID); err != nil {
		log.Printf("Failed to invalidate all user sessions: %v", err)
		http.Error(w, "Failed to invalidate all sessions", http.StatusInternalServerError)
		return
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "All sessions invalidated successfully",
	})
}

// Helper functions

// generateSecureState generates a secure random state parameter for OAuth
func generateSecureState() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		// Fallback to UUID if crypto/rand fails
		return uuid.New().String()
	}
	return base64.URLEncoding.EncodeToString(b)
}

// isValidImageType checks if the content type is a valid image type
func isValidImageType(contentType string) bool {
	validTypes := []string{
		"image/jpeg",
		"image/jpg",
		"image/png",
		"image/gif",
		"image/webp",
	}

	for _, validType := range validTypes {
		if contentType == validType {
			return true
		}
	}
	return false
}

// getFileExtension returns the file extension from a filename
func getFileExtension(filename string) string {
	if idx := strings.LastIndex(filename, "."); idx != -1 {
		return filename[idx:]
	}
	return ""
}
