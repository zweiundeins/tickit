package users

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/shared"
)

// SSORoutes handles HTTP routes for SSO functionality
type SSORoutes struct {
	ssoService     *SSOService
	commandBus     *shared.CommandBus
	queryBus       *shared.QueryBus
	sessionManager SessionManager
}

// NewSSORoutes creates a new SSO routes handler
func NewSSORoutes(
	ssoService *SSOService,
	commandBus *shared.CommandBus,
	queryBus *shared.QueryBus,
	sessionManager SessionManager,
) *SSORoutes {
	return &SSORoutes{
		ssoService:     ssoService,
		commandBus:     commandBus,
		queryBus:       queryBus,
		sessionManager: sessionManager,
	}
}

// RegisterRoutes registers SSO routes with the router
func (h *SSORoutes) RegisterRoutes(r chi.Router) {
	r.Route("/auth/sso", func(r chi.Router) {
		// SSO authentication routes
		r.Get("/providers", h.handleGetSSOProviders)
		r.Get("/{provider}/login", h.handleBeginSSOLogin)
		r.Get("/{provider}/callback", h.handleSSOCallback)
		r.Post("/{provider}/logout", h.handleSSOLogout)

		// SSO account management routes (require authentication)
		r.Group(func(r chi.Router) {
			// Add authentication middleware here
			r.Post("/{provider}/link", h.handleLinkSSOAccount)
			r.Delete("/{provider}/unlink", h.handleUnlinkSSOAccount)
			r.Post("/logout-all", h.handleSingleLogout)
		})
	})

	// Admin routes for SSO management
	r.Route("/admin/sso", func(r chi.Router) {
		// Add admin authentication middleware here
		r.Get("/providers", h.handleGetSSOProviderConfigs)
		r.Get("/users/{userID}/credentials", h.handleGetUserSSOCredentials)
		r.Post("/users/{userID}/sync-groups", h.handleSyncUserGroups)
	})
}

// handleGetSSOProviders returns the list of enabled SSO providers
func (h *SSORoutes) handleGetSSOProviders(w http.ResponseWriter, r *http.Request) {
	query := GetSSOProvidersQuery{}

	result, err := h.queryBus.Execute(r.Context(), query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get SSO providers: %v", err), http.StatusInternalServerError)
		return
	}

	providers, ok := result.([]string)
	if !ok {
		http.Error(w, "Invalid response type", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"providers": providers,
	})
}

// handleBeginSSOLogin initiates SSO authentication flow
func (h *SSORoutes) handleBeginSSOLogin(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		http.Error(w, "Provider parameter is required", http.StatusBadRequest)
		return
	}

	// Validate provider
	query := ValidateSSOProviderQuery{Provider: provider}
	result, err := h.queryBus.Execute(r.Context(), query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to validate provider: %v", err), http.StatusInternalServerError)
		return
	}

	isValid, ok := result.(bool)
	if !ok || !isValid {
		http.Error(w, "Invalid or unsupported SSO provider", http.StatusBadRequest)
		return
	}

	// Begin SSO flow
	authURL, err := h.ssoService.BeginSSOFlow(r.Context(), provider, w, r)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to begin SSO flow: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect to SSO provider
	http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
}

// handleSSOCallback handles SSO authentication callback
func (h *SSORoutes) handleSSOCallback(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		http.Error(w, "Provider parameter is required", http.StatusBadRequest)
		return
	}

	// Get callback parameters
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "Missing required callback parameters", http.StatusBadRequest)
		return
	}

	// Complete SSO flow
	gothUser, attributes, err := h.ssoService.CompleteSSOFlow(r.Context(), provider, code, state, r)
	if err != nil {
		http.Error(w, fmt.Sprintf("SSO authentication failed: %v", err), http.StatusUnauthorized)
		return
	}

	// Create or link user
	ipAddress := getClientIP(r)
	userAgent := r.UserAgent()

	user, session, err := h.ssoService.CreateOrLinkSSOUser(r.Context(), gothUser, attributes, ipAddress, userAgent)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create/link user: %v", err), http.StatusInternalServerError)
		return
	}

	// Create session in session manager
	httpSession, err := h.sessionManager.GetSession(r, "user-session")
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get session: %v", err), http.StatusInternalServerError)
		return
	}

	httpSession.Values["user_id"] = user.ID
	httpSession.Values["session_id"] = session.SessionID

	if err := h.sessionManager.SaveSession(r, w, httpSession); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save session: %v", err), http.StatusInternalServerError)
		return
	}

	// Redirect to dashboard or intended page
	redirectURL := r.URL.Query().Get("redirect")
	if redirectURL == "" {
		redirectURL = "/dashboard"
	}

	http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
}

// handleSSOLogout handles SSO logout
func (h *SSORoutes) handleSSOLogout(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		http.Error(w, "Provider parameter is required", http.StatusBadRequest)
		return
	}

	// Get user from session
	userID, err := h.getUserFromSession(r)
	if err != nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Initiate single logout
	cmd := InitiateSingleLogoutCommand{
		UserID: userID,
	}

	if err := h.commandBus.Execute(r.Context(), cmd); err != nil {
		http.Error(w, fmt.Sprintf("Failed to initiate logout: %v", err), http.StatusInternalServerError)
		return
	}

	// Destroy local session
	if err := h.sessionManager.DestroySession(r, w, "user-session"); err != nil {
		// Log error but continue
		logger.Error().Err(err).Msg("Failed to destroy session")
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "logged_out",
	})
}

// handleLinkSSOAccount links an SSO account to the current user
func (h *SSORoutes) handleLinkSSOAccount(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		http.Error(w, "Provider parameter is required", http.StatusBadRequest)
		return
	}

	// Get user from session
	userID, err := h.getUserFromSession(r)
	if err != nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Parse request body
	var req struct {
		Code  string `json:"code"`
		State string `json:"state"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Complete SSO flow to get provider user ID
	gothUser, _, err := h.ssoService.CompleteSSOFlow(r.Context(), provider, req.Code, req.State, r)
	if err != nil {
		http.Error(w, fmt.Sprintf("SSO authentication failed: %v", err), http.StatusUnauthorized)
		return
	}

	// Link SSO account
	cmd := LinkSSOAccountCommand{
		UserID:     userID,
		Provider:   provider,
		ProviderID: gothUser.UserID,
		Code:       req.Code,
		State:      req.State,
	}

	if err := h.commandBus.Execute(r.Context(), cmd); err != nil {
		http.Error(w, fmt.Sprintf("Failed to link SSO account: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "linked",
	})
}

// handleUnlinkSSOAccount unlinks an SSO account from the current user
func (h *SSORoutes) handleUnlinkSSOAccount(w http.ResponseWriter, r *http.Request) {
	provider := chi.URLParam(r, "provider")
	if provider == "" {
		http.Error(w, "Provider parameter is required", http.StatusBadRequest)
		return
	}

	// Get user from session
	userID, err := h.getUserFromSession(r)
	if err != nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Unlink SSO account
	cmd := UnlinkSSOAccountCommand{
		UserID:   userID,
		Provider: provider,
	}

	if err := h.commandBus.Execute(r.Context(), cmd); err != nil {
		http.Error(w, fmt.Sprintf("Failed to unlink SSO account: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "unlinked",
	})
}

// handleSingleLogout initiates single logout across all SSO providers
func (h *SSORoutes) handleSingleLogout(w http.ResponseWriter, r *http.Request) {
	// Get user from session
	userID, err := h.getUserFromSession(r)
	if err != nil {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	// Initiate single logout
	cmd := InitiateSingleLogoutCommand{
		UserID: userID,
	}

	if err := h.commandBus.Execute(r.Context(), cmd); err != nil {
		http.Error(w, fmt.Sprintf("Failed to initiate single logout: %v", err), http.StatusInternalServerError)
		return
	}

	// Destroy local session
	if err := h.sessionManager.DestroySession(r, w, "user-session"); err != nil {
		// Log error but continue
		logger.Error().Err(err).Msg("Failed to destroy session")
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "logged_out",
	})
}

// Admin handlers

// handleGetSSOProviderConfigs returns SSO provider configurations (admin only)
func (h *SSORoutes) handleGetSSOProviderConfigs(w http.ResponseWriter, r *http.Request) {
	// This would return non-sensitive configuration information
	// Sensitive data like client secrets should never be returned

	query := GetSSOProvidersQuery{}
	result, err := h.queryBus.Execute(r.Context(), query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get SSO providers: %v", err), http.StatusInternalServerError)
		return
	}

	providers, ok := result.([]string)
	if !ok {
		http.Error(w, "Invalid response type", http.StatusInternalServerError)
		return
	}

	// Return basic configuration info (no secrets)
	configs := make(map[string]interface{})
	for _, provider := range providers {
		configs[provider] = map[string]interface{}{
			"enabled": true,
			"type":    "oidc", // This would be determined from actual config
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"providers": configs,
	})
}

// handleGetUserSSOCredentials returns SSO credentials for a user (admin only)
func (h *SSORoutes) handleGetUserSSOCredentials(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	if userID == "" {
		http.Error(w, "User ID parameter is required", http.StatusBadRequest)
		return
	}

	query := GetUserSSOCredentialsQuery{UserID: userID}
	result, err := h.queryBus.Execute(r.Context(), query)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get user SSO credentials: %v", err), http.StatusInternalServerError)
		return
	}

	credentials, ok := result.([]*UserCredential)
	if !ok {
		http.Error(w, "Invalid response type", http.StatusInternalServerError)
		return
	}

	// Return sanitized credential info (no sensitive data)
	sanitizedCredentials := make([]map[string]interface{}, len(credentials))
	for i, cred := range credentials {
		sanitizedCredentials[i] = map[string]interface{}{
			"id":         cred.ID,
			"provider":   cred.Provider,
			"created_at": cred.CreatedAt,
			"updated_at": cred.UpdatedAt,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"credentials": sanitizedCredentials,
	})
}

// handleSyncUserGroups manually syncs user groups from SSO provider (admin only)
func (h *SSORoutes) handleSyncUserGroups(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "userID")
	if userID == "" {
		http.Error(w, "User ID parameter is required", http.StatusBadRequest)
		return
	}

	// Parse request body
	var req struct {
		Provider string   `json:"provider"`
		Groups   []string `json:"groups"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Sync user groups
	cmd := SyncUserGroupsCommand{
		UserID:   userID,
		Provider: req.Provider,
		Groups:   req.Groups,
	}

	if err := h.commandBus.Execute(r.Context(), cmd); err != nil {
		http.Error(w, fmt.Sprintf("Failed to sync user groups: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "synced",
	})
}

// Helper functions

// getUserFromSession extracts user ID from session
func (h *SSORoutes) getUserFromSession(r *http.Request) (string, error) {
	session, err := h.sessionManager.GetSession(r, "user-session")
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	userID, ok := session.Values["user_id"].(string)
	if !ok || userID == "" {
		return "", fmt.Errorf("user not authenticated")
	}

	return userID, nil
}

// getClientIP extracts client IP address from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP if multiple are present
		ips := strings.Split(forwarded, ",")
		return strings.TrimSpace(ips[0])
	}

	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}
