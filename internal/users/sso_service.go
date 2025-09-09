package users

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/markbates/goth"
	"github.com/markbates/goth/gothic"
)

// SSOService handles SSO authentication flows
type SSOService struct {
	ssoManager   *SSOManager
	userStore    UserStore
	sessionStore map[string]*SSOSession // In production, use Redis or similar
}

// NewSSOService creates a new SSO service instance
func NewSSOService(ssoManager *SSOManager, userStore UserStore) *SSOService {
	return &SSOService{
		ssoManager:   ssoManager,
		userStore:    userStore,
		sessionStore: make(map[string]*SSOSession),
	}
}

// BeginSSOFlow initiates the SSO authentication flow
func (s *SSOService) BeginSSOFlow(ctx context.Context, provider string, w http.ResponseWriter, r *http.Request) (string, error) {
	// Normalize provider name
	provider = strings.ToLower(strings.TrimSpace(provider))

	// Validate provider
	if !s.ssoManager.IsProviderEnabled(provider) {
		return "", fmt.Errorf("SSO provider %s is not enabled", provider)
	}

	// Generate secure state parameter for CSRF protection
	state, err := s.generateSecureState()
	if err != nil {
		return "", fmt.Errorf("failed to generate state parameter: %w", err)
	}

	// Store SSO session
	now := time.Now()
	session := &SSOSession{
		ID:        generateID(),
		State:     state,
		Provider:  provider,
		CreatedAt: now,
		ExpiresAt: now.Add(10 * time.Minute), // SSO flow expires in 10 minutes
		Metadata:  make(map[string]interface{}),
	}
	s.sessionStore[state] = session

	// Set provider in context for Gothic
	r = r.WithContext(context.WithValue(r.Context(), "provider", provider))

	// Use Gothic to begin SSO flow
	gothic.BeginAuthHandler(w, r)
	return "", nil
}

// CompleteSSOFlow completes the SSO authentication flow
func (s *SSOService) CompleteSSOFlow(ctx context.Context, provider, code, state string, r *http.Request) (*goth.User, *SSOUserAttributes, error) {
	// Normalize provider name
	provider = strings.ToLower(strings.TrimSpace(provider))

	// Validate state parameter
	session, exists := s.sessionStore[state]
	if !exists {
		return nil, nil, fmt.Errorf("invalid or expired SSO state")
	}

	// Check session expiration
	if time.Now().After(session.ExpiresAt) {
		delete(s.sessionStore, state)
		return nil, nil, fmt.Errorf("SSO session has expired")
	}

	// Validate provider matches
	if session.Provider != provider {
		delete(s.sessionStore, state)
		return nil, nil, fmt.Errorf("provider mismatch in SSO flow")
	}

	// Clean up session (one-time use)
	delete(s.sessionStore, state)

	// Validate provider is enabled
	if !s.ssoManager.IsProviderEnabled(provider) {
		return nil, nil, fmt.Errorf("SSO provider %s is not enabled", provider)
	}

	// Set provider in context for Gothic
	r = r.WithContext(context.WithValue(r.Context(), "provider", provider))

	// Use Gothic to complete SSO flow
	gothUser, err := gothic.CompleteUserAuth(nil, r)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to complete SSO authentication: %w", err)
	}

	// Extract user attributes from SSO response
	attributes, err := s.extractUserAttributes(&gothUser, provider)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to extract user attributes: %w", err)
	}

	return &gothUser, attributes, nil
}

// extractUserAttributes extracts user attributes from SSO response
func (s *SSOService) extractUserAttributes(gothUser *goth.User, provider string) (*SSOUserAttributes, error) {
	attributes := &SSOUserAttributes{
		Email:      gothUser.Email,
		FirstName:  gothUser.FirstName,
		LastName:   gothUser.LastName,
		Username:   gothUser.NickName,
		Groups:     make([]string, 0),
		Roles:      make([]string, 0),
		Attributes: make(map[string]string),
	}

	// If username is empty, generate from name or email
	if attributes.Username == "" {
		if gothUser.Name != "" {
			attributes.Username = strings.ToLower(strings.ReplaceAll(gothUser.Name, " ", "_"))
		} else if gothUser.Email != "" {
			parts := strings.Split(gothUser.Email, "@")
			if len(parts) > 0 {
				attributes.Username = strings.ToLower(parts[0])
			}
		}
	}

	// Extract groups and roles from raw data
	if gothUser.RawData != nil {
		if err := s.extractGroupsAndRoles(gothUser.RawData, provider, attributes); err != nil {
			// Log error but don't fail authentication
			fmt.Printf("Warning: Failed to extract groups/roles from SSO response: %v\n", err)
		}
	}

	// Map SSO groups to Tickit roles
	if len(attributes.Groups) > 0 {
		mappedRoles := s.ssoManager.MapGroupsToRoles(provider, attributes.Groups)
		attributes.Roles = append(attributes.Roles, mappedRoles...)
	}

	return attributes, nil
}

// extractGroupsAndRoles extracts groups and roles from raw SSO data
func (s *SSOService) extractGroupsAndRoles(rawData map[string]interface{}, provider string, attributes *SSOUserAttributes) error {
	switch provider {
	case "azuread":
		return s.extractAzureADGroups(rawData, attributes)
	case "oidc":
		return s.extractOIDCGroups(rawData, attributes)
	case "saml":
		return s.extractSAMLGroups(rawData, attributes)
	default:
		return fmt.Errorf("unsupported provider for group extraction: %s", provider)
	}
}

// extractAzureADGroups extracts groups from Azure AD response
func (s *SSOService) extractAzureADGroups(rawData map[string]interface{}, attributes *SSOUserAttributes) error {
	// Azure AD groups can be in different fields depending on configuration
	groupFields := []string{"groups", "roles", "group_membership"}

	for _, field := range groupFields {
		if groups, exists := rawData[field]; exists {
			switch groupsData := groups.(type) {
			case []interface{}:
				for _, group := range groupsData {
					if groupStr, ok := group.(string); ok {
						attributes.Groups = append(attributes.Groups, groupStr)
					}
				}
			case []string:
				attributes.Groups = append(attributes.Groups, groupsData...)
			case string:
				// Single group as string
				attributes.Groups = append(attributes.Groups, groupsData)
			}
		}
	}

	// Extract additional attributes
	if upn, exists := rawData["upn"]; exists {
		if upnStr, ok := upn.(string); ok {
			attributes.Attributes["upn"] = upnStr
		}
	}

	if objectId, exists := rawData["oid"]; exists {
		if objectIdStr, ok := objectId.(string); ok {
			attributes.Attributes["object_id"] = objectIdStr
		}
	}

	return nil
}

// extractOIDCGroups extracts groups from generic OIDC response
func (s *SSOService) extractOIDCGroups(rawData map[string]interface{}, attributes *SSOUserAttributes) error {
	// Standard OIDC group claims
	groupFields := []string{"groups", "roles", "group_membership", "memberOf"}

	for _, field := range groupFields {
		if groups, exists := rawData[field]; exists {
			switch groupsData := groups.(type) {
			case []interface{}:
				for _, group := range groupsData {
					if groupStr, ok := group.(string); ok {
						attributes.Groups = append(attributes.Groups, groupStr)
					}
				}
			case []string:
				attributes.Groups = append(attributes.Groups, groupsData...)
			case string:
				attributes.Groups = append(attributes.Groups, groupsData)
			}
		}
	}

	return nil
}

// extractSAMLGroups extracts groups from SAML response
func (s *SSOService) extractSAMLGroups(rawData map[string]interface{}, attributes *SSOUserAttributes) error {
	// SAML attributes are typically in a different format
	// This would need to be implemented based on specific SAML provider
	// For now, we'll look for common attribute names
	groupFields := []string{"groups", "roles", "memberOf", "Group", "Role"}

	for _, field := range groupFields {
		if groups, exists := rawData[field]; exists {
			switch groupsData := groups.(type) {
			case []interface{}:
				for _, group := range groupsData {
					if groupStr, ok := group.(string); ok {
						attributes.Groups = append(attributes.Groups, groupStr)
					}
				}
			case []string:
				attributes.Groups = append(attributes.Groups, groupsData...)
			case string:
				attributes.Groups = append(attributes.Groups, groupsData)
			}
		}
	}

	return nil
}

// CreateOrLinkSSOUser creates a new user or links SSO account to existing user
func (s *SSOService) CreateOrLinkSSOUser(ctx context.Context, gothUser *goth.User, attributes *SSOUserAttributes, ipAddress, userAgent string) (*User, *UserSession, error) {
	// Check if user already exists with this SSO provider
	existingUser, err := s.userStore.GetUserByOAuthProvider(ctx, gothUser.Provider, gothUser.UserID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to check existing SSO user: %w", err)
	}

	var user *User

	if existingUser != nil {
		// User already exists with this SSO provider
		user = existingUser

		// Update user attributes from SSO
		if err := s.updateUserFromSSO(ctx, user, attributes); err != nil {
			return nil, nil, fmt.Errorf("failed to update user from SSO: %w", err)
		}
	} else {
		// Check if user exists with the same email
		if attributes.Email != "" {
			existingEmailUser, err := s.userStore.GetUserByEmail(ctx, strings.ToLower(attributes.Email))
			if err != nil {
				return nil, nil, fmt.Errorf("failed to check existing email user: %w", err)
			}

			if existingEmailUser != nil {
				// Link SSO account to existing user
				user = existingEmailUser
				if err := s.linkSSOToUser(ctx, user, gothUser); err != nil {
					return nil, nil, fmt.Errorf("failed to link SSO account: %w", err)
				}

				// Update user attributes from SSO
				if err := s.updateUserFromSSO(ctx, user, attributes); err != nil {
					return nil, nil, fmt.Errorf("failed to update user from SSO: %w", err)
				}
			} else {
				// Create new user from SSO data
				user, err = s.createUserFromSSO(ctx, gothUser, attributes)
				if err != nil {
					return nil, nil, fmt.Errorf("failed to create user from SSO: %w", err)
				}
			}
		} else {
			return nil, nil, fmt.Errorf("SSO provider did not provide email address")
		}
	}

	// Check user status
	if user.Status != "Active" {
		if user.Status == "Inactive" {
			return nil, nil, fmt.Errorf("account is disabled")
		}
		// For SSO users, we can activate them immediately since email is verified by the provider
		if user.Status == "Pending" {
			user.Status = "Active"
			user.EmailVerified = true
			user.UpdatedAt = time.Now()
			if err := s.userStore.UpdateUser(ctx, user); err != nil {
				return nil, nil, fmt.Errorf("failed to activate SSO user: %w", err)
			}
		}
	}

	// Create session
	session, err := s.createUserSession(ctx, user.ID, ipAddress, userAgent)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Update last login time
	now := time.Now()
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := s.userStore.UpdateUser(ctx, user); err != nil {
		// Log error but don't fail login
		fmt.Printf("Failed to update user last login time: %v\n", err)
	}

	// Log SSO authentication event
	s.logSSOEvent(ctx, user.ID, gothUser.Provider, "login", ipAddress, userAgent, map[string]interface{}{
		"groups": attributes.Groups,
		"roles":  attributes.Roles,
	})

	return user, session, nil
}

// updateUserFromSSO updates user attributes from SSO data
func (s *SSOService) updateUserFromSSO(ctx context.Context, user *User, attributes *SSOUserAttributes) error {
	updated := false

	// Update role based on SSO groups (if configured)
	if len(attributes.Roles) > 0 {
		newRole := s.ssoManager.GetHighestRole(attributes.Roles)
		if user.Role != newRole {
			user.Role = newRole
			updated = true
		}
	}

	// Update profile information if provided
	if attributes.FirstName != "" && user.FirstName != attributes.FirstName {
		user.FirstName = attributes.FirstName
		updated = true
	}

	if attributes.LastName != "" && user.LastName != attributes.LastName {
		user.LastName = attributes.LastName
		updated = true
	}

	if updated {
		user.UpdatedAt = time.Now()
		if err := s.userStore.UpdateUser(ctx, user); err != nil {
			return fmt.Errorf("failed to update user: %w", err)
		}

		// Log group sync event
		s.logSSOEvent(ctx, user.ID, "", "group_sync", "", "", map[string]interface{}{
			"old_role": user.Role,
			"new_role": user.Role,
			"groups":   attributes.Groups,
		})
	}

	return nil
}

// linkSSOToUser links an SSO account to an existing user
func (s *SSOService) linkSSOToUser(ctx context.Context, user *User, gothUser *goth.User) error {
	// Check if this SSO provider is already linked
	existingCredential, err := s.userStore.GetUserCredentialByProvider(ctx, user.ID, gothUser.Provider)
	if err != nil {
		return fmt.Errorf("failed to check existing SSO credential: %w", err)
	}

	now := time.Now()

	if existingCredential != nil {
		// Update existing credential
		existingCredential.ProviderID = gothUser.UserID
		existingCredential.CredentialData = s.serializeSSOData(gothUser)
		existingCredential.UpdatedAt = now

		if err := s.userStore.UpdateUserCredential(ctx, existingCredential); err != nil {
			return fmt.Errorf("failed to update SSO credential: %w", err)
		}
	} else {
		// Create new SSO credential
		credential := &UserCredential{
			ID:             generateID(),
			UserID:         user.ID,
			Type:           "sso",
			Provider:       gothUser.Provider,
			ProviderID:     gothUser.UserID,
			CredentialData: s.serializeSSOData(gothUser),
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		if err := s.userStore.CreateUserCredential(ctx, credential); err != nil {
			return fmt.Errorf("failed to create SSO credential: %w", err)
		}
	}

	return nil
}

// createUserFromSSO creates a new user from SSO data
func (s *SSOService) createUserFromSSO(ctx context.Context, gothUser *goth.User, attributes *SSOUserAttributes) (*User, error) {
	now := time.Now()

	// Ensure username is unique
	uniqueUsername, err := s.ensureUniqueUsername(ctx, attributes.Username)
	if err != nil {
		return nil, fmt.Errorf("failed to generate unique username: %w", err)
	}

	// Determine user role from SSO attributes
	role := "Viewer" // Default role
	if len(attributes.Roles) > 0 {
		role = s.ssoManager.GetHighestRole(attributes.Roles)
	}

	// Create user
	user := &User{
		ID:            generateID(),
		Email:         strings.ToLower(strings.TrimSpace(attributes.Email)),
		Username:      uniqueUsername,
		FirstName:     attributes.FirstName,
		LastName:      attributes.LastName,
		AvatarURL:     gothUser.AvatarURL,
		Role:          role,
		Status:        "Active", // SSO users are immediately active
		EmailVerified: true,     // Email is verified by SSO provider
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	if err := s.userStore.CreateUser(ctx, user); err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}

	// Create SSO credential
	credential := &UserCredential{
		ID:             generateID(),
		UserID:         user.ID,
		Type:           "sso",
		Provider:       gothUser.Provider,
		ProviderID:     gothUser.UserID,
		CredentialData: s.serializeSSOData(gothUser),
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := s.userStore.CreateUserCredential(ctx, credential); err != nil {
		return nil, fmt.Errorf("failed to create SSO credential: %w", err)
	}

	return user, nil
}

// createUserSession creates a new user session
func (s *SSOService) createUserSession(ctx context.Context, userID, ipAddress, userAgent string) (*UserSession, error) {
	now := time.Now()
	session := &UserSession{
		ID:        generateID(),
		UserID:    userID,
		SessionID: generateID(),            // This would be the Gorilla session ID in practice
		ExpiresAt: now.Add(24 * time.Hour), // Session expires in 24 hours
		CreatedAt: now,
		IPAddress: ipAddress,
		UserAgent: userAgent,
	}

	if err := s.userStore.CreateUserSession(ctx, session); err != nil {
		return nil, fmt.Errorf("failed to create session: %w", err)
	}

	return session, nil
}

// ensureUniqueUsername ensures the username is unique by appending numbers if needed
func (s *SSOService) ensureUniqueUsername(ctx context.Context, baseUsername string) (string, error) {
	username := baseUsername
	counter := 1

	for {
		// Check if username exists
		existingUser, err := s.userStore.GetUserByUsername(ctx, username)
		if err != nil {
			return "", fmt.Errorf("failed to check username uniqueness: %w", err)
		}

		if existingUser == nil {
			// Username is available
			return username, nil
		}

		// Try with counter
		username = fmt.Sprintf("%s_%d", baseUsername, counter)
		counter++

		// Prevent infinite loop
		if counter > 1000 {
			return "", fmt.Errorf("failed to generate unique username after 1000 attempts")
		}
	}
}

// serializeSSOData serializes SSO user data for storage
func (s *SSOService) serializeSSOData(gothUser *goth.User) string {
	// Store relevant SSO data as JSON
	data := map[string]interface{}{
		"access_token":  gothUser.AccessToken,
		"refresh_token": gothUser.RefreshToken,
		"expires_at":    gothUser.ExpiresAt,
		"raw_data":      gothUser.RawData,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		// Fallback to simple format
		return fmt.Sprintf("access_token:%s", gothUser.AccessToken)
	}

	return string(jsonData)
}

// generateSecureState generates a cryptographically secure state parameter
func (s *SSOService) generateSecureState() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// logSSOEvent logs SSO authentication events for auditing
func (s *SSOService) logSSOEvent(ctx context.Context, userID, provider, event, ipAddress, userAgent string, details map[string]interface{}) {
	// In a real implementation, this would store to database or audit log
	fmt.Printf("SSO Event: user=%s, provider=%s, event=%s, ip=%s, details=%v\n",
		userID, provider, event, ipAddress, details)
}

// InitiateSingleLogout initiates single logout across all SSO providers
func (s *SSOService) InitiateSingleLogout(ctx context.Context, userID string) error {
	// Get all SSO credentials for the user
	credentials, err := s.userStore.GetUserCredentials(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user credentials: %w", err)
	}

	// Invalidate all user sessions
	if err := s.userStore.DeleteAllUserSessions(ctx, userID); err != nil {
		return fmt.Errorf("failed to invalidate user sessions: %w", err)
	}

	// Log logout events for each SSO provider
	for _, credential := range credentials {
		if credential.Type == "sso" {
			s.logSSOEvent(ctx, userID, credential.Provider, "logout", "", "", map[string]interface{}{
				"single_logout": true,
			})
		}
	}

	return nil
}

// CleanupExpiredSessions removes expired SSO sessions
func (s *SSOService) CleanupExpiredSessions() {
	now := time.Now()
	for state, session := range s.sessionStore {
		if now.After(session.ExpiresAt) {
			delete(s.sessionStore, state)
		}
	}
}
