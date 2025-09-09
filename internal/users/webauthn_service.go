package users

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
)

// WebAuthnServiceImpl implements the WebAuthnService interface
type WebAuthnServiceImpl struct {
	webauthn  *webauthn.WebAuthn
	userStore UserStore
}

// NewWebAuthnService creates a new WebAuthnService instance
func NewWebAuthnService(userStore UserStore) WebAuthnService {
	// Configure WebAuthn
	wconfig := &webauthn.Config{
		RPDisplayName: "Tickit",                                                    // Display Name for your site
		RPID:          "localhost",                                                 // Generally the domain name for your site
		RPOrigins:     []string{"http://localhost:8080", "https://localhost:8080"}, // The origin URLs allowed for WebAuthn requests
	}

	webAuthn, err := webauthn.New(wconfig)
	if err != nil {
		log.Printf("Failed to create WebAuthn instance: %v", err)
		return &WebAuthnServiceImpl{
			webauthn:  nil,
			userStore: userStore,
		}
	}

	return &WebAuthnServiceImpl{
		webauthn:  webAuthn,
		userStore: userStore,
	}
}

// WebAuthnUser implements the webauthn.User interface
type WebAuthnUser struct {
	ID          []byte
	Name        string
	DisplayName string
	Credentials []webauthn.Credential
}

// WebAuthnID returns the user's ID
func (u WebAuthnUser) WebAuthnID() []byte {
	return u.ID
}

// WebAuthnName returns the user's username
func (u WebAuthnUser) WebAuthnName() string {
	return u.Name
}

// WebAuthnDisplayName returns the user's display name
func (u WebAuthnUser) WebAuthnDisplayName() string {
	return u.DisplayName
}

// WebAuthnIcon returns the user's icon URL
func (u WebAuthnUser) WebAuthnIcon() string {
	return ""
}

// WebAuthnCredentials returns the user's credentials
func (u WebAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.Credentials
}

// BeginRegistration starts the WebAuthn registration process
func (s *WebAuthnServiceImpl) BeginRegistration(ctx context.Context, userID string) ([]byte, error) {
	if s.webauthn == nil {
		return nil, fmt.Errorf("WebAuthn not properly initialized")
	}

	// Get user from store
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Get existing WebAuthn credentials for the user
	credentials, err := s.getUserWebAuthnCredentials(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user credentials: %w", err)
	}

	// Create WebAuthn user
	webauthnUser := WebAuthnUser{
		ID:          []byte(userID),
		Name:        user.Email,
		DisplayName: user.Username,
		Credentials: credentials,
	}

	// Begin registration
	creation, sessionData, err := s.webauthn.BeginRegistration(webauthnUser)
	if err != nil {
		return nil, fmt.Errorf("failed to begin registration: %w", err)
	}

	// Store session data temporarily
	sessionJSON, err := json.Marshal(sessionData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session data: %w", err)
	}

	// Clean up any existing registration sessions first
	s.cleanupSessionCredentials(ctx, userID, "registration_session")

	sessionCredential := &UserCredential{
		ID:             uuid.New().String(),
		UserID:         userID,
		Type:           "webauthn_session",
		Provider:       "webauthn",
		ProviderID:     "registration_session",
		CredentialData: string(sessionJSON),
	}

	err = s.userStore.CreateUserCredential(ctx, sessionCredential)
	if err != nil {
		return nil, fmt.Errorf("failed to store session data: %w", err)
	}

	// Return the creation options as JSON
	creationJSON, err := json.Marshal(creation)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal creation options: %w", err)
	}

	return creationJSON, nil
}

// FinishRegistration completes the WebAuthn registration process
func (s *WebAuthnServiceImpl) FinishRegistration(ctx context.Context, userID string, response []byte) error {
	if s.webauthn == nil {
		return fmt.Errorf("WebAuthn not properly initialized")
	}

	// Get user from store
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Get existing WebAuthn credentials for the user
	credentials, err := s.getUserWebAuthnCredentials(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user credentials: %w", err)
	}

	// Create WebAuthn user
	webauthnUser := WebAuthnUser{
		ID:          []byte(userID),
		Name:        user.Email,
		DisplayName: user.Username,
		Credentials: credentials,
	}

	// Get stored session data
	sessionCredential, err := s.getSessionCredential(ctx, userID, "registration_session")
	if err != nil {
		return fmt.Errorf("failed to get session data: %w", err)
	}

	var sessionData webauthn.SessionData
	err = json.Unmarshal([]byte(sessionCredential.CredentialData), &sessionData)
	if err != nil {
		return fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	// Parse the response
	parsedResponse, err := protocol.ParseCredentialCreationResponseBody(bytes.NewReader(response))
	if err != nil {
		return fmt.Errorf("failed to parse credential creation response: %w", err)
	}

	// Finish registration
	credential, err := s.webauthn.CreateCredential(webauthnUser, sessionData, parsedResponse)
	if err != nil {
		return fmt.Errorf("failed to finish registration: %w", err)
	}

	// Store the credential
	credentialJSON, err := json.Marshal(credential)
	if err != nil {
		return fmt.Errorf("failed to marshal credential: %w", err)
	}

	userCredential := &UserCredential{
		ID:             uuid.New().String(),
		UserID:         userID,
		Type:           "webauthn",
		Provider:       "webauthn",
		ProviderID:     string(credential.ID),
		CredentialData: string(credentialJSON),
	}

	err = s.userStore.CreateUserCredential(ctx, userCredential)
	if err != nil {
		return fmt.Errorf("failed to store credential: %w", err)
	}

	// Clean up session data
	err = s.userStore.DeleteUserCredential(ctx, sessionCredential.ID)
	if err != nil {
		log.Printf("Failed to clean up session data: %v", err)
	}

	return nil
}

// BeginLogin starts the WebAuthn login process
func (s *WebAuthnServiceImpl) BeginLogin(ctx context.Context, userID string) ([]byte, error) {
	if s.webauthn == nil {
		return nil, fmt.Errorf("WebAuthn not properly initialized")
	}

	// Get user from store
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	// Get existing WebAuthn credentials for the user
	credentials, err := s.getUserWebAuthnCredentials(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user credentials: %w", err)
	}

	if len(credentials) == 0 {
		return nil, fmt.Errorf("no WebAuthn credentials found for user")
	}

	// Create WebAuthn user
	webauthnUser := WebAuthnUser{
		ID:          []byte(userID),
		Name:        user.Email,
		DisplayName: user.Username,
		Credentials: credentials,
	}

	// Begin login
	assertion, sessionData, err := s.webauthn.BeginLogin(webauthnUser)
	if err != nil {
		return nil, fmt.Errorf("failed to begin login: %w", err)
	}

	// Store session data temporarily
	sessionJSON, err := json.Marshal(sessionData)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal session data: %w", err)
	}

	// Clean up any existing login sessions first
	s.cleanupSessionCredentials(ctx, userID, "login_session")

	sessionCredential := &UserCredential{
		ID:             uuid.New().String(),
		UserID:         userID,
		Type:           "webauthn_session",
		Provider:       "webauthn",
		ProviderID:     "login_session",
		CredentialData: string(sessionJSON),
	}

	err = s.userStore.CreateUserCredential(ctx, sessionCredential)
	if err != nil {
		return nil, fmt.Errorf("failed to store session data: %w", err)
	}

	// Return the assertion options as JSON
	assertionJSON, err := json.Marshal(assertion)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal assertion options: %w", err)
	}

	return assertionJSON, nil
}

// FinishLogin completes the WebAuthn login process
func (s *WebAuthnServiceImpl) FinishLogin(ctx context.Context, userID string, response []byte) error {
	if s.webauthn == nil {
		return fmt.Errorf("WebAuthn not properly initialized")
	}

	// Get user from store
	user, err := s.userStore.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// Get existing WebAuthn credentials for the user
	credentials, err := s.getUserWebAuthnCredentials(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user credentials: %w", err)
	}

	// Create WebAuthn user
	webauthnUser := WebAuthnUser{
		ID:          []byte(userID),
		Name:        user.Email,
		DisplayName: user.Username,
		Credentials: credentials,
	}

	// Get stored session data
	sessionCredential, err := s.getSessionCredential(ctx, userID, "login_session")
	if err != nil {
		return fmt.Errorf("failed to get session data: %w", err)
	}

	var sessionData webauthn.SessionData
	err = json.Unmarshal([]byte(sessionCredential.CredentialData), &sessionData)
	if err != nil {
		return fmt.Errorf("failed to unmarshal session data: %w", err)
	}

	// Parse the response
	parsedResponse, err := protocol.ParseCredentialRequestResponseBody(bytes.NewReader(response))
	if err != nil {
		return fmt.Errorf("failed to parse credential request response: %w", err)
	}

	// Finish login
	_, err = s.webauthn.ValidateLogin(webauthnUser, sessionData, parsedResponse)
	if err != nil {
		return fmt.Errorf("failed to finish login: %w", err)
	}

	// Clean up session data
	err = s.userStore.DeleteUserCredential(ctx, sessionCredential.ID)
	if err != nil {
		log.Printf("Failed to clean up session data: %v", err)
	}

	return nil
}

// GetUserCredentials retrieves WebAuthn credentials for a user
func (s *WebAuthnServiceImpl) GetUserCredentials(ctx context.Context, userID string) ([]interface{}, error) {
	credentials, err := s.getUserWebAuthnCredentials(ctx, userID)
	if err != nil {
		return nil, err
	}

	result := make([]interface{}, len(credentials))
	for i, cred := range credentials {
		result[i] = cred
	}

	return result, nil
}

// RemoveCredential removes a WebAuthn credential
func (s *WebAuthnServiceImpl) RemoveCredential(ctx context.Context, userID, credentialID string) error {
	// Get all user credentials
	userCredentials, err := s.userStore.GetUserCredentials(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to get user credentials: %w", err)
	}

	// Find and delete the specific WebAuthn credential
	for _, cred := range userCredentials {
		if cred.Type == "webauthn" && cred.ProviderID == credentialID {
			err = s.userStore.DeleteUserCredential(ctx, cred.ID)
			if err != nil {
				return fmt.Errorf("failed to delete credential: %w", err)
			}
			return nil
		}
	}

	return fmt.Errorf("credential not found")
}

// Helper functions

// getUserWebAuthnCredentials is a helper function to get WebAuthn credentials for a user
func (s *WebAuthnServiceImpl) getUserWebAuthnCredentials(ctx context.Context, userID string) ([]webauthn.Credential, error) {
	userCredentials, err := s.userStore.GetUserCredentials(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user credentials: %w", err)
	}

	var credentials []webauthn.Credential
	for _, cred := range userCredentials {
		if cred.Type == "webauthn" && cred.Provider == "webauthn" {
			var webauthnCred webauthn.Credential
			err = json.Unmarshal([]byte(cred.CredentialData), &webauthnCred)
			if err != nil {
				log.Printf("Failed to unmarshal WebAuthn credential: %v", err)
				continue
			}
			credentials = append(credentials, webauthnCred)
		}
	}

	return credentials, nil
}

// getSessionCredential retrieves a specific session credential
func (s *WebAuthnServiceImpl) getSessionCredential(ctx context.Context, userID, sessionType string) (*UserCredential, error) {
	userCredentials, err := s.userStore.GetUserCredentials(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user credentials: %w", err)
	}

	for _, cred := range userCredentials {
		if cred.Type == "webauthn_session" && cred.ProviderID == sessionType {
			return cred, nil
		}
	}

	return nil, fmt.Errorf("no %s session found", sessionType)
}

// cleanupSessionCredentials removes old session credentials
func (s *WebAuthnServiceImpl) cleanupSessionCredentials(ctx context.Context, userID, sessionType string) {
	userCredentials, err := s.userStore.GetUserCredentials(ctx, userID)
	if err != nil {
		return
	}

	for _, cred := range userCredentials {
		if cred.Type == "webauthn_session" && cred.ProviderID == sessionType {
			s.userStore.DeleteUserCredential(ctx, cred.ID)
		}
	}
}
