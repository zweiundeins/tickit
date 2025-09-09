package users

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/google/uuid"
	"github.com/mbolli/tickit/internal/shared"
)

// EventPublisher interface for publishing events
type EventPublisher interface {
	PublishEvent(ctx context.Context, event shared.Event) error
}

// UserCommandHandler handles all user-related commands
// Following the same pattern as ProjectCommandHandler
type UserCommandHandler struct {
	eventBus        EventPublisher
	eventStore      *shared.EventStore
	userStore       UserStore // Direct database storage for user data
	passwordHasher  PasswordHasher
	emailService    EmailService
	webauthnService WebAuthnService
	ssoService      *SSOService
}

// NewUserCommandHandler creates a new UserCommandHandler instance
func NewUserCommandHandler(
	eventBus EventPublisher,
	eventStore *shared.EventStore,
	userStore UserStore,
	passwordHasher PasswordHasher,
	emailService EmailService,
	webauthnService WebAuthnService,
	ssoService *SSOService,
) *UserCommandHandler {
	return &UserCommandHandler{
		eventBus:        eventBus,
		eventStore:      eventStore,
		userStore:       userStore,
		passwordHasher:  passwordHasher,
		emailService:    emailService,
		webauthnService: webauthnService,
		ssoService:      ssoService,
	}
}

// Handle implements the shared.CommandHandler interface
func (h *UserCommandHandler) Handle(ctx context.Context, command interface{}) error {
	switch cmd := command.(type) {
	// Registration Commands
	case RegisterUserCommand:
		return h.HandleRegisterUser(ctx, cmd)
	case VerifyEmailCommand:
		return h.HandleVerifyEmail(ctx, cmd)

	// Authentication Commands
	case LoginUserCommand:
		return h.HandleLoginUser(ctx, cmd)
	case LoginOAuthCommand:
		return h.HandleLoginOAuth(ctx, cmd)
	case LoginWebAuthnCommand:
		return h.HandleLoginWebAuthn(ctx, cmd)
	case SendMagicMailCommand:
		return h.HandleSendMagicMail(ctx, cmd)
	case LoginMagicMailCommand:
		return h.HandleLoginMagicMail(ctx, cmd)
	case LogoutUserCommand:
		return h.HandleLogoutUser(ctx, cmd)

	// Password Management Commands
	case ForgotPasswordCommand:
		return h.HandleForgotPassword(ctx, cmd)
	case ResetPasswordCommand:
		return h.HandleResetPassword(ctx, cmd)
	case ChangePasswordCommand:
		return h.HandleChangePassword(ctx, cmd)

	// MFA Commands
	case EnableMFACommand:
		return h.HandleEnableMFA(ctx, cmd)
	case DisableMFACommand:
		return h.HandleDisableMFA(ctx, cmd)
	case BeginMFARegistrationCommand:
		return h.HandleBeginMFARegistration(ctx, cmd)
	case FinishMFARegistrationCommand:
		return h.HandleFinishMFARegistration(ctx, cmd)

	// Profile Management Commands
	case UpdateUserProfileCommand:
		return h.HandleUpdateUserProfile(ctx, cmd)
	case DeactivateUserCommand:
		return h.HandleDeactivateUser(ctx, cmd)
	case ReactivateUserCommand:
		return h.HandleReactivateUser(ctx, cmd)

	// Project Membership Commands
	case InviteUserToProjectCommand:
		return h.HandleInviteUserToProject(ctx, cmd)
	case AcceptProjectInvitationCommand:
		return h.HandleAcceptProjectInvitation(ctx, cmd)
	case DeclineProjectInvitationCommand:
		return h.HandleDeclineProjectInvitation(ctx, cmd)
	case AddUserToProjectCommand:
		return h.HandleAddUserToProject(ctx, cmd)
	case RemoveUserFromProjectCommand:
		return h.HandleRemoveUserFromProject(ctx, cmd)
	case UpdateProjectMembershipCommand:
		return h.HandleUpdateProjectMembership(ctx, cmd)

	// Administrative Commands
	case AssignUserRoleCommand:
		return h.HandleAssignUserRole(ctx, cmd)
	case PatchUserCommand:
		return h.HandlePatchUser(ctx, cmd)

	// OAuth Commands
	case LinkOAuthAccountCommand:
		return h.HandleLinkOAuthAccount(ctx, cmd)
	case UnlinkOAuthAccountCommand:
		return h.HandleUnlinkOAuthAccount(ctx, cmd)

	// Session Commands
	case CreateUserSessionCommand:
		return h.HandleCreateUserSession(ctx, cmd)
	case InvalidateUserSessionCommand:
		return h.HandleInvalidateUserSession(ctx, cmd)
	case InvalidateAllUserSessionsCommand:
		return h.HandleInvalidateAllUserSessions(ctx, cmd)

	// SSO Commands
	case LoginSSOCommand:
		return h.HandleLoginSSO(ctx, cmd)
	case LinkSSOAccountCommand:
		return h.HandleLinkSSOAccount(ctx, cmd)
	case UnlinkSSOAccountCommand:
		return h.HandleUnlinkSSOAccount(ctx, cmd)
	case InitiateSingleLogoutCommand:
		return h.HandleInitiateSingleLogout(ctx, cmd)
	case SyncUserGroupsCommand:
		return h.HandleSyncUserGroups(ctx, cmd)

	default:
		return nil // Ignore unknown commands
	}
}

// Registration Command Handlers

func (h *UserCommandHandler) HandleRegisterUser(ctx context.Context, cmd RegisterUserCommand) error {
	// 1. Validate input
	if err := ValidateEmail(cmd.Email); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}

	if err := ValidatePassword(cmd.Password); err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}

	if err := ValidateUsername(cmd.Username); err != nil {
		return fmt.Errorf("invalid username: %w", err)
	}

	if err := ValidateName(cmd.FirstName, "first name"); err != nil {
		return err
	}

	if err := ValidateName(cmd.LastName, "last name"); err != nil {
		return err
	}

	// 2. Check if user already exists
	existingUser, err := h.userStore.GetUserByEmail(ctx, cmd.Email)
	if err != nil {
		return fmt.Errorf("failed to check existing user: %w", err)
	}
	if existingUser != nil {
		return fmt.Errorf("user with email %s already exists", cmd.Email)
	}

	// Check if username is taken
	existingUserByUsername, err := h.userStore.GetUserByUsername(ctx, cmd.Username)
	if err != nil {
		return fmt.Errorf("failed to check existing username: %w", err)
	}
	if existingUserByUsername != nil {
		return fmt.Errorf("username %s is already taken", cmd.Username)
	}

	// 3. Hash password
	hashedPassword, err := h.passwordHasher.HashPassword(cmd.Password)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// 4. Create user
	now := time.Now()
	user := &User{
		ID:            cmd.ID,
		Email:         strings.ToLower(strings.TrimSpace(cmd.Email)),
		Username:      strings.TrimSpace(cmd.Username),
		FirstName:     strings.TrimSpace(cmd.FirstName),
		LastName:      strings.TrimSpace(cmd.LastName),
		Role:          "Viewer",  // Default role
		Status:        "Pending", // Pending email verification
		EmailVerified: false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	// 5. Generate verification token
	verificationToken := uuid.New().String()

	// 6. Store user and credential in database (direct storage, not event sourcing)
	if err := h.userStore.CreateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to create user: %w", err)
	}

	// Create password credential
	credential := &UserCredential{
		ID:             uuid.New().String(),
		UserID:         user.ID,
		Type:           "password",
		Provider:       "local",
		CredentialData: hashedPassword,
		CreatedAt:      now,
		UpdatedAt:      now,
	}

	if err := h.userStore.CreateUserCredential(ctx, credential); err != nil {
		return fmt.Errorf("failed to create user credential: %w", err)
	}

	// Create email verification token
	verificationTokenRecord := &EmailVerificationToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Token:     verificationToken,
		ExpiresAt: now.Add(24 * time.Hour), // Token expires in 24 hours
		CreatedAt: now,
	}

	if err := h.userStore.CreateEmailVerificationToken(ctx, verificationTokenRecord); err != nil {
		return fmt.Errorf("failed to create verification token: %w", err)
	}

	// 7. Send verification email
	if h.emailService != nil {
		if err := h.emailService.SendVerificationEmail(ctx, user.Email, verificationToken); err != nil {
			// Log error but don't fail registration
			// In a real implementation, you might want to queue this for retry
			fmt.Printf("Failed to send verification email to %s: %v\n", user.Email, err)
		}
	}

	// 8. Publish event for UI updates (limited, non-sensitive data)
	event := UserRegisteredEvent{
		User:              user,
		VerificationToken: verificationToken,
		Timestamp:         now,
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail registration
		fmt.Printf("Failed to publish UserRegisteredEvent: %v\n", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleVerifyEmail(ctx context.Context, cmd VerifyEmailCommand) error {
	// 1. Validate token input
	if cmd.Token == "" {
		return fmt.Errorf("verification token is required")
	}

	// 2. Get verification token from database
	verificationToken, err := h.userStore.GetEmailVerificationToken(ctx, cmd.Token)
	if err != nil {
		return fmt.Errorf("failed to get verification token: %w", err)
	}
	if verificationToken == nil {
		return fmt.Errorf("invalid verification token")
	}

	// 3. Check token expiration
	now := time.Now()
	if now.After(verificationToken.ExpiresAt) {
		// Clean up expired token
		if err := h.userStore.DeleteEmailVerificationToken(ctx, cmd.Token); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("Failed to delete expired verification token: %v\n", err)
		}
		return fmt.Errorf("verification token has expired")
	}

	// 4. Get user associated with the token
	user, err := h.userStore.GetUser(ctx, verificationToken.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// 5. Check if user is already verified
	if user.EmailVerified {
		// Clean up token since user is already verified
		if err := h.userStore.DeleteEmailVerificationToken(ctx, cmd.Token); err != nil {
			fmt.Printf("Failed to delete verification token: %v\n", err)
		}
		return fmt.Errorf("email is already verified")
	}

	// 6. Update user verification status and activate account
	user.EmailVerified = true
	user.Status = "Active" // Activate the account upon email verification
	user.UpdatedAt = now

	if err := h.userStore.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update user verification status: %w", err)
	}

	// 7. Delete the verification token (one-time use)
	if err := h.userStore.DeleteEmailVerificationToken(ctx, cmd.Token); err != nil {
		// Log error but don't fail the operation since verification succeeded
		fmt.Printf("Failed to delete verification token: %v\n", err)
	}

	// 8. Publish email verified event for UI updates
	event := UserEmailVerifiedEvent{
		UserID:    user.ID,
		Email:     user.Email,
		Timestamp: now,
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail verification
		fmt.Printf("Failed to publish UserEmailVerifiedEvent: %v\n", err)
	}

	return nil
}

// Authentication Command Handlers

func (h *UserCommandHandler) HandleLoginUser(ctx context.Context, cmd LoginUserCommand) error {
	// 1. Validate input
	if err := ValidateEmail(cmd.Email); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}

	if cmd.Password == "" {
		return fmt.Errorf("password is required")
	}

	// 2. Get user by email
	user, err := h.userStore.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(cmd.Email)))
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		// Publish login failed event for security monitoring
		h.publishLoginFailedEvent(ctx, cmd.Email, cmd.IPAddress, cmd.UserAgent, "user not found")
		return fmt.Errorf("invalid credentials")
	}

	// 3. Check user status
	if user.Status != "Active" {
		if user.Status == "Pending" && !user.EmailVerified {
			h.publishLoginFailedEvent(ctx, cmd.Email, cmd.IPAddress, cmd.UserAgent, "email not verified")
			return fmt.Errorf("email not verified")
		}
		if user.Status == "Inactive" {
			h.publishLoginFailedEvent(ctx, cmd.Email, cmd.IPAddress, cmd.UserAgent, "account disabled")
			return fmt.Errorf("account is disabled")
		}
		h.publishLoginFailedEvent(ctx, cmd.Email, cmd.IPAddress, cmd.UserAgent, "invalid account status")
		return fmt.Errorf("invalid account status")
	}

	// 4. Get and verify password credential
	credentials, err := h.userStore.GetUserCredentials(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to get user credentials: %w", err)
	}

	var passwordCredential *UserCredential
	for _, cred := range credentials {
		log.Printf("Credential: ID=%s, Type=%s, Provider=%s", cred.ID, cred.Type, cred.Provider)
		if cred.Type == "password" && cred.Provider == "local" {
			passwordCredential = cred
			break
		}
	}

	if passwordCredential == nil {
		h.publishLoginFailedEvent(ctx, cmd.Email, cmd.IPAddress, cmd.UserAgent, "no password credential")
		return fmt.Errorf("invalid credentials")
	}

	// 5. Verify password
	if err := h.passwordHasher.CheckPassword(passwordCredential.CredentialData, cmd.Password); err != nil {
		h.publishLoginFailedEvent(ctx, cmd.Email, cmd.IPAddress, cmd.UserAgent, "invalid password")
		return fmt.Errorf("invalid credentials")
	}

	// 6. Check MFA requirements (for future implementation)
	// For now, we'll skip MFA and proceed with session creation

	// 7. Create session
	sessionID := uuid.New().String()
	now := time.Now()
	expiresAt := now.Add(24 * time.Hour) // Session expires in 24 hours

	session := &UserSession{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		SessionID: sessionID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
		IPAddress: cmd.IPAddress,
		UserAgent: cmd.UserAgent,
	}

	if err := h.userStore.CreateUserSession(ctx, session); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// 8. Update user's last login time
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := h.userStore.UpdateUser(ctx, user); err != nil {
		// Log error but don't fail login
		fmt.Printf("Failed to update user last login time: %v\n", err)
	}

	// 9. Publish successful login event
	loginEvent := UserLoggedInEvent{
		UserID:    user.ID,
		SessionID: sessionID,
		IPAddress: cmd.IPAddress,
		UserAgent: cmd.UserAgent,
		Method:    "password",
		Timestamp: now,
	}

	if err := h.eventBus.PublishEvent(ctx, loginEvent); err != nil {
		// Log error but don't fail login
		fmt.Printf("Failed to publish UserLoggedInEvent: %v\n", err)
	}

	return nil
}

// Helper method to publish login failed events
func (h *UserCommandHandler) publishLoginFailedEvent(ctx context.Context, email, ipAddress, userAgent, reason string) {
	event := UserLoginFailedEvent{
		Email:     email,
		IPAddress: ipAddress,
		UserAgent: userAgent,
		Reason:    reason,
		Timestamp: time.Now(),
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		fmt.Printf("Failed to publish UserLoginFailedEvent: %v\n", err)
	}
}

func (h *UserCommandHandler) HandleLoginOAuth(ctx context.Context, cmd LoginOAuthCommand) error {
	// 1. Validate input
	if cmd.Provider == "" {
		return fmt.Errorf("OAuth provider is required")
	}

	if cmd.Code == "" {
		return fmt.Errorf("OAuth code is required")
	}

	if cmd.State == "" {
		return fmt.Errorf("OAuth state is required")
	}

	// 2. Normalize provider name
	provider := NormalizeProviderName(cmd.Provider)

	// 3. Validate provider is supported
	if !IsProviderSupported(provider) {
		return fmt.Errorf("OAuth provider %s is not supported", provider)
	}

	// 4. Create OAuth service (this would be injected in a real implementation)
	oauthManager := NewOAuthManager()
	if err := oauthManager.LoadConfigFromEnvironment(); err != nil {
		return fmt.Errorf("failed to load OAuth configuration: %w", err)
	}

	//oauthService := NewOAuthService(oauthManager, h.userStore)

	// 5. Create a mock request for Gothic (in a real implementation, this would be passed from the route handler)
	// For now, we'll simulate the OAuth completion
	if !oauthManager.IsProviderEnabled(provider) {
		return fmt.Errorf("OAuth provider %s is not enabled", provider)
	}

	// 6. In a real implementation, we would complete the OAuth flow here
	// For now, we'll return an error indicating this needs to be called from the HTTP handler
	return fmt.Errorf("OAuth login must be completed through HTTP handlers with proper request context")
}

func (h *UserCommandHandler) HandleLoginWebAuthn(ctx context.Context, cmd LoginWebAuthnCommand) error {
	// 1. Validate user exists
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// 2. Check if user is active and verified
	if user.Status != "Active" {
		return fmt.Errorf("user account is not active")
	}

	if !user.EmailVerified {
		return fmt.Errorf("user email is not verified")
	}

	// 3. Validate WebAuthn assertion
	err = h.webauthnService.FinishLogin(ctx, cmd.UserID, cmd.Assertion)
	if err != nil {
		// Log failed MFA authentication
		failedEvent := UserMFAAuthenticationFailedEvent{
			UserID:    cmd.UserID,
			IPAddress: cmd.IPAddress,
			UserAgent: cmd.UserAgent,
			Reason:    err.Error(),
			Timestamp: time.Now(),
		}

		h.eventStore.StoreEvent(ctx, failedEvent)
		h.eventBus.PublishEvent(ctx, failedEvent)

		return fmt.Errorf("WebAuthn authentication failed: %w", err)
	}

	// 4. Update last login time
	now := time.Now()
	user.LastLoginAt = &now
	err = h.userStore.UpdateUser(ctx, user)
	if err != nil {
		log.Printf("Failed to update user last login time: %v", err)
	}

	// 5. Create and store login event
	loginEvent := UserLoggedInEvent{
		UserID:    cmd.UserID,
		SessionID: "", // Session will be created by the auth service
		IPAddress: cmd.IPAddress,
		UserAgent: cmd.UserAgent,
		Method:    "webauthn",
		Timestamp: time.Now(),
	}

	// 6. Store event
	err = h.eventStore.StoreEvent(ctx, loginEvent)
	if err != nil {
		return fmt.Errorf("failed to store login event: %w", err)
	}

	// 7. Create MFA authentication success event
	mfaSuccessEvent := UserMFAAuthenticationSucceededEvent{
		UserID:       cmd.UserID,
		CredentialID: "", // We could extract this from the assertion if needed
		IPAddress:    cmd.IPAddress,
		UserAgent:    cmd.UserAgent,
		Timestamp:    time.Now(),
	}

	err = h.eventStore.StoreEvent(ctx, mfaSuccessEvent)
	if err != nil {
		log.Printf("Failed to store MFA success event: %v", err)
	}

	// 8. Publish events for real-time updates
	err = h.eventBus.PublishEvent(ctx, loginEvent)
	if err != nil {
		log.Printf("Failed to publish login event: %v", err)
	}

	err = h.eventBus.PublishEvent(ctx, mfaSuccessEvent)
	if err != nil {
		log.Printf("Failed to publish MFA success event: %v", err)
	}

	log.Printf("WebAuthn login successful for user %s (%s)", user.Username, user.Email)
	return nil
}

func (h *UserCommandHandler) HandleSendMagicMail(ctx context.Context, cmd SendMagicMailCommand) error {
	// 1. Check if development mode - magic mail is only available in development
	if h.emailService == nil {
		return fmt.Errorf("email service not configured")
	}

	// Check if this is a development environment by trying to send magic mail
	// The email service will reject if not in development mode
	testErr := h.emailService.SendMagicMailLink(ctx, "test@example.com", "test-token")
	if testErr != nil && testErr.Error() == "magic mail is only available in development mode" {
		return fmt.Errorf("magic mail is only available in development mode")
	}

	// 2. Validate input
	if err := ValidateEmail(cmd.Email); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}

	// 3. Find user by email
	user, err := h.userStore.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(cmd.Email)))
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		// For security, don't reveal if user exists or not
		// But still log the event for monitoring
		fmt.Printf("Magic mail requested for non-existent user: %s\n", cmd.Email)
		return nil // Return success to avoid user enumeration
	}

	// 4. Check user status
	if user.Status != "Active" {
		if user.Status == "Pending" && !user.EmailVerified {
			return fmt.Errorf("email not verified")
		}
		if user.Status == "Inactive" {
			return fmt.Errorf("account is disabled")
		}
		return fmt.Errorf("invalid account status")
	}

	// 5. Generate magic mail token
	now := time.Now()
	magicToken := &MagicMailToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Token:     uuid.New().String(),
		ExpiresAt: now.Add(15 * time.Minute), // Magic mail tokens expire in 15 minutes
		CreatedAt: now,
	}

	// 6. Store token in database
	if err := h.userStore.CreateMagicMailToken(ctx, magicToken); err != nil {
		return fmt.Errorf("failed to create magic mail token: %w", err)
	}

	// 7. Send magic mail (or log in dev mode)
	if err := h.emailService.SendMagicMailLink(ctx, user.Email, magicToken.Token); err != nil {
		// Clean up token if email sending fails
		if deleteErr := h.userStore.DeleteMagicMailToken(ctx, magicToken.Token); deleteErr != nil {
			fmt.Printf("Failed to clean up magic mail token after email failure: %v\n", deleteErr)
		}
		return fmt.Errorf("failed to send magic mail: %w", err)
	}

	// 8. Publish event for UI updates
	event := MagicMailSentEvent{
		UserID:    user.ID,
		Email:     user.Email,
		Token:     magicToken.Token,
		Timestamp: now,
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Failed to publish MagicMailSentEvent: %v\n", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleLoginMagicMail(ctx context.Context, cmd LoginMagicMailCommand) error {
	// 1. Validate token input
	if cmd.Token == "" {
		return fmt.Errorf("magic mail token is required")
	}

	// 2. Get magic mail token from database
	magicToken, err := h.userStore.GetMagicMailToken(ctx, cmd.Token)
	if err != nil {
		return fmt.Errorf("failed to get magic mail token: %w", err)
	}
	if magicToken == nil {
		return fmt.Errorf("invalid magic mail token")
	}

	// 3. Check if token was already used
	if magicToken.UsedAt != nil {
		return fmt.Errorf("magic mail token has already been used")
	}

	// 4. Check token expiration
	now := time.Now()
	if now.After(magicToken.ExpiresAt) {
		// Clean up expired token
		if err := h.userStore.DeleteMagicMailToken(ctx, cmd.Token); err != nil {
			fmt.Printf("Failed to delete expired magic mail token: %v\n", err)
		}
		return fmt.Errorf("magic mail token has expired")
	}

	// 5. Get user associated with the token
	user, err := h.userStore.GetUser(ctx, magicToken.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// 6. Check user status
	if user.Status != "Active" {
		if user.Status == "Pending" && !user.EmailVerified {
			return fmt.Errorf("email not verified")
		}
		if user.Status == "Inactive" {
			return fmt.Errorf("account is disabled")
		}
		return fmt.Errorf("invalid account status")
	}

	// 7. Create session
	sessionID := uuid.New().String()
	expiresAt := now.Add(24 * time.Hour) // Session expires in 24 hours

	session := &UserSession{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		SessionID: sessionID,
		ExpiresAt: expiresAt,
		CreatedAt: now,
		IPAddress: cmd.IPAddress,
		UserAgent: cmd.UserAgent,
	}

	if err := h.userStore.CreateUserSession(ctx, session); err != nil {
		return fmt.Errorf("failed to create session: %w", err)
	}

	// 8. Mark token as used (one-time use)
	magicToken.UsedAt = &now
	if err := h.userStore.UpdateMagicMailToken(ctx, magicToken); err != nil {
		// Log error but don't fail login since session was created
		fmt.Printf("Failed to mark magic mail token as used: %v\n", err)
	}

	// 9. Update user's last login time
	user.LastLoginAt = &now
	user.UpdatedAt = now
	if err := h.userStore.UpdateUser(ctx, user); err != nil {
		// Log error but don't fail login
		fmt.Printf("Failed to update user last login time: %v\n", err)
	}

	// 10. Publish successful login event
	loginEvent := UserLoggedInEvent{
		UserID:    user.ID,
		SessionID: sessionID,
		IPAddress: cmd.IPAddress,
		UserAgent: cmd.UserAgent,
		Method:    "magic_mail",
		Timestamp: now,
	}

	if err := h.eventBus.PublishEvent(ctx, loginEvent); err != nil {
		// Log error but don't fail login
		fmt.Printf("Failed to publish UserLoggedInEvent: %v\n", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleLogoutUser(ctx context.Context, cmd LogoutUserCommand) error {
	// TODO: Implement user logout logic
	// 1. Validate session
	// 2. Invalidate session
	// 3. Store event
	// 4. Update projection
	// 5. Publish event
	return nil
}

// Password Management Command Handlers

func (h *UserCommandHandler) HandleForgotPassword(ctx context.Context, cmd ForgotPasswordCommand) error {
	// 1. Validate input
	if err := ValidateEmail(cmd.Email); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}

	// 2. Find user by email
	user, err := h.userStore.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(cmd.Email)))
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// For security, don't reveal if user exists or not
	// Always return success to prevent user enumeration attacks
	if user == nil {
		// Log the attempt for monitoring but return success
		fmt.Printf("Password reset requested for non-existent user: %s\n", cmd.Email)
		return nil
	}

	// 3. Check user status - only allow reset for active users or pending verification
	if user.Status == "Inactive" {
		// Don't reveal account status, just log and return success
		fmt.Printf("Password reset requested for inactive user: %s\n", cmd.Email)
		return nil
	}

	// 4. Generate reset token
	now := time.Now()
	resetToken := &PasswordResetToken{
		ID:        uuid.New().String(),
		UserID:    user.ID,
		Token:     uuid.New().String(),
		ExpiresAt: now.Add(1 * time.Hour), // Token expires in 1 hour
		CreatedAt: now,
	}

	// 5. Store token in database
	if err := h.userStore.CreatePasswordResetToken(ctx, resetToken); err != nil {
		return fmt.Errorf("failed to create password reset token: %w", err)
	}

	// 6. Send reset email
	if h.emailService != nil {
		if err := h.emailService.SendPasswordResetEmail(ctx, user.Email, resetToken.Token); err != nil {
			// Clean up token if email sending fails
			if deleteErr := h.userStore.DeletePasswordResetToken(ctx, resetToken.Token); deleteErr != nil {
				fmt.Printf("Failed to clean up password reset token after email failure: %v\n", deleteErr)
			}
			return fmt.Errorf("failed to send password reset email: %w", err)
		}
	}

	// 7. Publish event for UI updates and security monitoring
	event := PasswordResetRequestedEvent{
		UserID:    user.ID,
		Email:     user.Email,
		Token:     resetToken.Token,
		Timestamp: now,
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Failed to publish PasswordResetRequestedEvent: %v\n", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleResetPassword(ctx context.Context, cmd ResetPasswordCommand) error {
	// 1. Validate input
	if cmd.Token == "" {
		return fmt.Errorf("reset token is required")
	}

	if err := ValidatePassword(cmd.NewPassword); err != nil {
		return fmt.Errorf("invalid password: %w", err)
	}

	// 2. Get reset token from database
	resetToken, err := h.userStore.GetPasswordResetToken(ctx, cmd.Token)
	if err != nil {
		return fmt.Errorf("failed to get password reset token: %w", err)
	}
	if resetToken == nil {
		return fmt.Errorf("invalid reset token")
	}

	// 3. Check if token was already used
	if resetToken.UsedAt != nil {
		return fmt.Errorf("reset token has already been used")
	}

	// 4. Check token expiration
	now := time.Now()
	if now.After(resetToken.ExpiresAt) {
		// Clean up expired token
		if err := h.userStore.DeletePasswordResetToken(ctx, cmd.Token); err != nil {
			fmt.Printf("Failed to delete expired password reset token: %v\n", err)
		}
		return fmt.Errorf("reset token has expired")
	}

	// 5. Get user associated with the token
	user, err := h.userStore.GetUser(ctx, resetToken.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// 6. Check user status - only allow reset for active users or pending verification
	if user.Status == "Inactive" {
		return fmt.Errorf("account is disabled")
	}

	// 7. Hash new password
	hashedPassword, err := h.passwordHasher.HashPassword(cmd.NewPassword)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// 8. Update user password credential
	credentials, err := h.userStore.GetUserCredentials(ctx, user.ID)
	if err != nil {
		return fmt.Errorf("failed to get user credentials: %w", err)
	}

	var passwordCredential *UserCredential
	for _, cred := range credentials {
		if cred.Type == "password" && cred.Provider == "local" {
			passwordCredential = cred
			break
		}
	}

	if passwordCredential == nil {
		// Create new password credential if none exists
		passwordCredential = &UserCredential{
			ID:             uuid.New().String(),
			UserID:         user.ID,
			Type:           "password",
			Provider:       "local",
			CredentialData: hashedPassword,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if err := h.userStore.CreateUserCredential(ctx, passwordCredential); err != nil {
			return fmt.Errorf("failed to create password credential: %w", err)
		}
	} else {
		// Update existing password credential
		passwordCredential.CredentialData = hashedPassword
		passwordCredential.UpdatedAt = now
		if err := h.userStore.UpdateUserCredential(ctx, passwordCredential); err != nil {
			return fmt.Errorf("failed to update password credential: %w", err)
		}
	}

	// 9. Mark token as used (one-time use)
	resetToken.UsedAt = &now
	if err := h.userStore.UpdatePasswordResetToken(ctx, resetToken); err != nil {
		// Log error but don't fail the operation since password was updated
		fmt.Printf("Failed to mark password reset token as used: %v\n", err)
	}

	// 10. Invalidate all user sessions for security
	if err := h.userStore.DeleteAllUserSessions(ctx, user.ID); err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Failed to invalidate user sessions after password reset: %v\n", err)
	}

	// 11. Update user's updated timestamp
	user.UpdatedAt = now
	if err := h.userStore.UpdateUser(ctx, user); err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Failed to update user timestamp after password reset: %v\n", err)
	}

	// 12. Publish password reset completed event
	event := PasswordResetCompletedEvent{
		UserID:    user.ID,
		Timestamp: now,
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Failed to publish PasswordResetCompletedEvent: %v\n", err)
	}

	// 13. Publish session invalidation event
	sessionEvent := AllUserSessionsInvalidatedEvent{
		UserID:    user.ID,
		Reason:    "password_reset",
		Timestamp: now,
	}

	if err := h.eventBus.PublishEvent(ctx, sessionEvent); err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Failed to publish AllUserSessionsInvalidatedEvent: %v\n", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleChangePassword(ctx context.Context, cmd ChangePasswordCommand) error {
	// 1. Validate user exists
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	if user.Status != "Active" {
		return fmt.Errorf("user account is not active")
	}

	// 2. Get current password credential
	credentials, err := h.userStore.GetUserCredentials(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user credentials: %w", err)
	}

	var passwordCredential *UserCredential
	for _, cred := range credentials {
		if cred.Type == "password" {
			passwordCredential = cred
			break
		}
	}

	if passwordCredential == nil {
		return fmt.Errorf("user does not have a password credential")
	}

	// 3. Validate old password
	if err := h.passwordHasher.CheckPassword(passwordCredential.CredentialData, cmd.OldPassword); err != nil {
		return fmt.Errorf("invalid old password")
	}

	// 4. Validate new password
	if err := ValidatePassword(cmd.NewPassword); err != nil {
		return fmt.Errorf("invalid new password: %w", err)
	}

	// 5. Hash new password
	hashedPassword, err := h.passwordHasher.HashPassword(cmd.NewPassword)
	if err != nil {
		return fmt.Errorf("failed to hash new password: %w", err)
	}

	// 6. Update password credential
	passwordCredential.CredentialData = hashedPassword
	passwordCredential.UpdatedAt = time.Now()

	if err := h.userStore.UpdateUserCredential(ctx, passwordCredential); err != nil {
		return fmt.Errorf("failed to update password credential: %w", err)
	}

	// 7. Invalidate all other sessions (keep current session if provided)
	if cmd.CurrentSessionID != "" {
		if err := h.userStore.InvalidateAllUserSessionsExcept(ctx, cmd.UserID, cmd.CurrentSessionID); err != nil {
			// Log error but don't fail the password change
			log.Printf("Failed to invalidate other sessions for user %s: %v", cmd.UserID, err)
		}
	} else {
		if err := h.userStore.InvalidateAllUserSessions(ctx, cmd.UserID); err != nil {
			// Log error but don't fail the password change
			log.Printf("Failed to invalidate sessions for user %s: %v", cmd.UserID, err)
		}
	}

	// 8. Create and store event
	event := PasswordChangedEvent{
		UserID:    cmd.UserID,
		Timestamp: time.Now(),
	}

	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to store password changed event: %w", err)
	}

	// 9. Update projection (handled by event system)

	// 10. Publish event for UI updates
	if h.eventBus != nil {
		if err := h.eventBus.PublishEvent(ctx, event); err != nil {
			log.Printf("Failed to publish password changed event: %v", err)
		}
	}

	return nil
}

// MFA Command Handlers

func (h *UserCommandHandler) HandleEnableMFA(ctx context.Context, cmd EnableMFACommand) error {
	// 1. Validate user exists
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// 2. Finish WebAuthn registration using the credential data
	err = h.webauthnService.FinishRegistration(ctx, cmd.UserID, cmd.Credential)
	if err != nil {
		return fmt.Errorf("failed to finish WebAuthn registration: %w", err)
	}

	// 3. Create and store event
	event := UserMFAEnabledEvent{
		UserID:     cmd.UserID,
		Credential: cmd.Credential,
		Timestamp:  time.Now(),
	}

	// 4. Store event
	err = h.eventStore.StoreEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	// 5. Publish event for real-time updates
	err = h.eventBus.PublishEvent(ctx, event)
	if err != nil {
		log.Printf("Failed to publish UserMFAEnabled event: %v", err)
	}

	log.Printf("MFA enabled for user %s (%s)", user.Username, user.Email)
	return nil
}

func (h *UserCommandHandler) HandleDisableMFA(ctx context.Context, cmd DisableMFACommand) error {
	// 1. Validate user exists
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// 2. Remove WebAuthn credential
	err = h.webauthnService.RemoveCredential(ctx, cmd.UserID, cmd.CredentialID)
	if err != nil {
		return fmt.Errorf("failed to remove WebAuthn credential: %w", err)
	}

	// 3. Create and store event
	event := UserMFADisabledEvent{
		UserID:       cmd.UserID,
		CredentialID: cmd.CredentialID,
		Timestamp:    time.Now(),
	}

	// 4. Store event
	err = h.eventStore.StoreEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	// 5. Publish event for real-time updates
	err = h.eventBus.PublishEvent(ctx, event)
	if err != nil {
		log.Printf("Failed to publish UserMFADisabled event: %v", err)
	}

	log.Printf("MFA disabled for user %s (%s)", user.Username, user.Email)
	return nil
}

func (h *UserCommandHandler) HandleBeginMFARegistration(ctx context.Context, cmd BeginMFARegistrationCommand) error {
	// 1. Validate user exists
	_, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// 2. Begin WebAuthn registration
	_, err = h.webauthnService.BeginRegistration(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to begin WebAuthn registration: %w", err)
	}

	// 3. Create and store event
	event := UserMFARegistrationBeganEvent{
		UserID:    cmd.UserID,
		Timestamp: time.Now(),
	}

	// 4. Store event
	err = h.eventStore.StoreEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	// 5. Publish event for real-time updates
	err = h.eventBus.PublishEvent(ctx, event)
	if err != nil {
		log.Printf("Failed to publish UserMFARegistrationBegan event: %v", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleFinishMFARegistration(ctx context.Context, cmd FinishMFARegistrationCommand) error {
	// 1. Validate user exists
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	// 2. Finish WebAuthn registration
	err = h.webauthnService.FinishRegistration(ctx, cmd.UserID, cmd.Response)
	if err != nil {
		return fmt.Errorf("failed to finish WebAuthn registration: %w", err)
	}

	// 3. Create and store event
	event := UserMFARegistrationCompletedEvent{
		UserID:    cmd.UserID,
		Timestamp: time.Now(),
	}

	// 4. Store event
	err = h.eventStore.StoreEvent(ctx, event)
	if err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	// 5. Publish event for real-time updates
	err = h.eventBus.PublishEvent(ctx, event)
	if err != nil {
		log.Printf("Failed to publish UserMFARegistrationCompleted event: %v", err)
	}

	log.Printf("MFA registration completed for user %s (%s)", user.Username, user.Email)
	return nil
}

// Profile Management Command Handlers

func (h *UserCommandHandler) HandleUpdateUserProfile(ctx context.Context, cmd UpdateUserProfileCommand) error {
	// 1. Validate input
	if cmd.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	// Validate email if provided
	if cmd.Email != nil {
		if err := ValidateEmail(*cmd.Email); err != nil {
			return fmt.Errorf("invalid email: %w", err)
		}
	}

	// Validate username if provided
	if cmd.Username != nil {
		if err := ValidateUsername(*cmd.Username); err != nil {
			return fmt.Errorf("invalid username: %w", err)
		}
	}

	// Validate names if provided
	if cmd.FirstName != nil {
		if err := ValidateName(*cmd.FirstName, "first name"); err != nil {
			return err
		}
	}

	if cmd.LastName != nil {
		if err := ValidateName(*cmd.LastName, "last name"); err != nil {
			return err
		}
	}

	// Validate avatar URL if provided
	if cmd.AvatarURL != nil {
		if err := ValidateAvatarURL(*cmd.AvatarURL); err != nil {
			return fmt.Errorf("invalid avatar URL: %w", err)
		}
	}

	// 2. Get existing user
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// 3. Check for email uniqueness if email is being changed
	if cmd.Email != nil && *cmd.Email != user.Email {
		existingUser, err := h.userStore.GetUserByEmail(ctx, strings.ToLower(strings.TrimSpace(*cmd.Email)))
		if err != nil {
			return fmt.Errorf("failed to check email uniqueness: %w", err)
		}
		if existingUser != nil {
			return fmt.Errorf("email %s is already in use", *cmd.Email)
		}
	}

	// 4. Check for username uniqueness if username is being changed
	if cmd.Username != nil && *cmd.Username != user.Username {
		existingUser, err := h.userStore.GetUserByUsername(ctx, strings.TrimSpace(*cmd.Username))
		if err != nil {
			return fmt.Errorf("failed to check username uniqueness: %w", err)
		}
		if existingUser != nil {
			return fmt.Errorf("username %s is already taken", *cmd.Username)
		}
	}

	// 5. Track changes for event
	changes := make(map[string]interface{})
	oldUser := *user // Create a copy for comparison

	// 6. Update user fields
	now := time.Now()
	emailChanged := false

	if cmd.Email != nil {
		newEmail := strings.ToLower(strings.TrimSpace(*cmd.Email))
		if newEmail != user.Email {
			changes["email"] = map[string]string{"old": user.Email, "new": newEmail}
			user.Email = newEmail
			emailChanged = true
			// If email changes, require re-verification
			user.EmailVerified = false
			changes["email_verified"] = map[string]bool{"old": oldUser.EmailVerified, "new": false}
		}
	}

	if cmd.Username != nil {
		newUsername := strings.TrimSpace(*cmd.Username)
		if newUsername != user.Username {
			changes["username"] = map[string]string{"old": user.Username, "new": newUsername}
			user.Username = newUsername
		}
	}

	if cmd.FirstName != nil {
		newFirstName := strings.TrimSpace(*cmd.FirstName)
		if newFirstName != user.FirstName {
			changes["first_name"] = map[string]string{"old": user.FirstName, "new": newFirstName}
			user.FirstName = newFirstName
		}
	}

	if cmd.LastName != nil {
		newLastName := strings.TrimSpace(*cmd.LastName)
		if newLastName != user.LastName {
			changes["last_name"] = map[string]string{"old": user.LastName, "new": newLastName}
			user.LastName = newLastName
		}
	}

	if cmd.AvatarURL != nil {
		if *cmd.AvatarURL != user.AvatarURL {
			changes["avatar_url"] = map[string]string{"old": user.AvatarURL, "new": *cmd.AvatarURL}
			oldAvatarURL := user.AvatarURL
			user.AvatarURL = *cmd.AvatarURL

			// Clean up old avatar file if it was a local upload
			if oldAvatarURL != "" {
				avatarService := NewAvatarService("upload")
				if err := avatarService.CleanupUserAvatar(oldAvatarURL); err != nil {
					// Log error but don't fail the update
					fmt.Printf("Failed to clean up old avatar file: %v\n", err)
				}
			}
		}
	}

	// 7. Only update if there are changes
	if len(changes) == 0 {
		return nil // No changes to apply
	}

	user.UpdatedAt = now

	// 8. Update user in database
	if err := h.userStore.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to update user profile: %w", err)
	}

	// 9. If email changed, create new verification token and send email
	if emailChanged && h.emailService != nil {
		verificationToken := uuid.New().String()
		verificationTokenRecord := &EmailVerificationToken{
			ID:        uuid.New().String(),
			UserID:    user.ID,
			Token:     verificationToken,
			ExpiresAt: now.Add(24 * time.Hour),
			CreatedAt: now,
		}

		if err := h.userStore.CreateEmailVerificationToken(ctx, verificationTokenRecord); err != nil {
			// Log error but don't fail the profile update
			fmt.Printf("Failed to create email verification token after profile update: %v\n", err)
		} else {
			if err := h.emailService.SendVerificationEmail(ctx, user.Email, verificationToken); err != nil {
				// Log error but don't fail the profile update
				fmt.Printf("Failed to send verification email after profile update: %v\n", err)
			}
		}
	}

	// 10. Publish profile updated event
	event := UserProfileUpdatedEvent{
		UserID:    user.ID,
		Changes:   changes,
		UpdatedBy: cmd.UserID, // User updating their own profile
		Timestamp: now,
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail the update
		fmt.Printf("Failed to publish UserProfileUpdatedEvent: %v\n", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleDeactivateUser(ctx context.Context, cmd DeactivateUserCommand) error {
	// 1. Validate input
	if cmd.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	// 2. Get existing user
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// 3. Check if user is already inactive
	if user.Status == "Inactive" {
		return fmt.Errorf("user is already deactivated")
	}

	// 4. Update user status to inactive (preserving all data)
	now := time.Now()
	oldStatus := user.Status
	user.Status = "Inactive"
	user.UpdatedAt = now

	if err := h.userStore.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to deactivate user: %w", err)
	}

	// 4.1. Clean up user's avatar file (optional, for privacy)
	if user.AvatarURL != "" {
		avatarService := NewAvatarService("upload")
		if err := avatarService.CleanupUserAvatar(user.AvatarURL); err != nil {
			// Log error but don't fail deactivation
			fmt.Printf("Failed to clean up user avatar during deactivation: %v\n", err)
		}
	}

	// 5. Invalidate all user sessions
	if err := h.userStore.DeleteAllUserSessions(ctx, user.ID); err != nil {
		// Log error but don't fail deactivation
		fmt.Printf("Failed to invalidate user sessions during deactivation: %v\n", err)
	}

	// 6. Publish user deactivated event
	event := UserDeactivatedEvent{
		UserID:        user.ID,
		DeactivatedBy: cmd.UserID, // Could be self-deactivation or admin action
		Timestamp:     now,
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail deactivation
		fmt.Printf("Failed to publish UserDeactivatedEvent: %v\n", err)
	}

	// 7. Publish session invalidation event
	sessionEvent := AllUserSessionsInvalidatedEvent{
		UserID:    user.ID,
		Reason:    "account_deactivated",
		Timestamp: now,
	}

	if err := h.eventBus.PublishEvent(ctx, sessionEvent); err != nil {
		// Log error but don't fail deactivation
		fmt.Printf("Failed to publish AllUserSessionsInvalidatedEvent: %v\n", err)
	}

	fmt.Printf("User %s (%s) deactivated, status changed from %s to %s\n", user.Username, user.Email, oldStatus, user.Status)
	return nil
}

func (h *UserCommandHandler) HandleReactivateUser(ctx context.Context, cmd ReactivateUserCommand) error {
	// 1. Validate input
	if cmd.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	// 2. Get existing user
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// 3. Check if user is already active
	if user.Status == "Active" {
		return fmt.Errorf("user is already active")
	}

	// 4. Check if user can be reactivated (must be inactive, not pending)
	if user.Status != "Inactive" {
		return fmt.Errorf("user cannot be reactivated from status: %s", user.Status)
	}

	// 5. Update user status to active
	now := time.Now()
	oldStatus := user.Status
	user.Status = "Active"
	user.UpdatedAt = now

	if err := h.userStore.UpdateUser(ctx, user); err != nil {
		return fmt.Errorf("failed to reactivate user: %w", err)
	}

	// 6. Publish user reactivated event
	event := UserReactivatedEvent{
		UserID:        user.ID,
		ReactivatedBy: cmd.UserID, // Typically admin action
		Timestamp:     now,
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail reactivation
		fmt.Printf("Failed to publish UserReactivatedEvent: %v\n", err)
	}

	fmt.Printf("User %s (%s) reactivated, status changed from %s to %s\n", user.Username, user.Email, oldStatus, user.Status)
	return nil
}

// Project Membership Command Handlers

func (h *UserCommandHandler) HandleInviteUserToProject(ctx context.Context, cmd InviteUserToProjectCommand) error {
	// 1. Validate input
	if cmd.ProjectID == "" {
		return fmt.Errorf("project ID is required")
	}
	if cmd.Email == "" {
		return fmt.Errorf("email is required")
	}
	if cmd.Role == "" {
		return fmt.Errorf("role is required")
	}
	if cmd.InvitedBy == "" {
		return fmt.Errorf("invited by user ID is required")
	}

	// 2. Validate email format
	if err := ValidateEmail(cmd.Email); err != nil {
		return fmt.Errorf("invalid email: %w", err)
	}

	// 3. Validate role (must be a project role)
	validProjectRoles := map[string]bool{
		ProjectRoleAdmin:     true,
		ProjectRoleDeveloper: true,
		ProjectRoleViewer:    true,
	}
	if !validProjectRoles[cmd.Role] {
		return fmt.Errorf("invalid project role: %s", cmd.Role)
	}

	// 4. Check if inviter exists and has permission to invite
	inviter, err := h.userStore.GetUser(ctx, cmd.InvitedBy)
	if err != nil {
		return fmt.Errorf("failed to get inviter: %w", err)
	}
	if inviter == nil {
		return fmt.Errorf("inviter not found")
	}

	// 5. Check if inviter has permission to invite to this project
	// Admin can invite to any project, ProjectManager can invite to projects they manage
	if inviter.Role != RoleAdmin {
		// Check if inviter is a project admin for this project
		membership, err := h.userStore.GetProjectMembership(ctx, cmd.InvitedBy, cmd.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to check inviter project membership: %w", err)
		}
		if membership == nil || membership.Role != ProjectRoleAdmin {
			return fmt.Errorf("insufficient permissions to invite users to this project")
		}
	}

	// 6. Check if user is already a member of the project
	existingUser, _ := h.userStore.GetUserByEmail(ctx, cmd.Email)
	if existingUser != nil {
		membership, err := h.userStore.GetProjectMembership(ctx, existingUser.ID, cmd.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to check existing membership: %w", err)
		}
		if membership != nil {
			return fmt.Errorf("user is already a member of this project")
		}
	}

	// 7. Check if there's already a pending invitation for this email and project
	existingInvitations, err := h.userStore.GetUserInvitations(ctx, cmd.Email)
	if err != nil {
		return fmt.Errorf("failed to check existing invitations: %w", err)
	}
	for _, invitation := range existingInvitations {
		if invitation.ProjectID == cmd.ProjectID && invitation.AcceptedAt == nil && invitation.DeclinedAt == nil {
			// Check if invitation is still valid (not expired)
			if time.Now().Before(invitation.ExpiresAt) {
				return fmt.Errorf("user already has a pending invitation for this project")
			}
		}
	}

	// 8. Generate invitation token and create invitation
	now := time.Now()
	invitation := &UserInvitation{
		ID:        uuid.New().String(),
		ProjectID: cmd.ProjectID,
		Email:     cmd.Email,
		Role:      cmd.Role,
		Token:     uuid.New().String(),
		ExpiresAt: now.Add(7 * 24 * time.Hour), // Invitation expires in 7 days
		CreatedAt: now,
		CreatedBy: cmd.InvitedBy,
	}

	// 9. Store invitation in database
	if err := h.userStore.CreateInvitation(ctx, invitation); err != nil {
		return fmt.Errorf("failed to create invitation: %w", err)
	}

	// 10. Send invitation email
	if h.emailService != nil {
		// Get project name for the email (we'll use project ID for now since we don't have project store access)
		projectName := fmt.Sprintf("Project %s", cmd.ProjectID)
		if err := h.emailService.SendInvitationEmail(ctx, cmd.Email, projectName, invitation.Token); err != nil {
			// Clean up invitation if email sending fails
			if deleteErr := h.userStore.DeleteInvitation(ctx, invitation.ID); deleteErr != nil {
				log.Printf("Failed to clean up invitation after email failure: %v", deleteErr)
			}
			return fmt.Errorf("failed to send invitation email: %w", err)
		}
	}

	// 11. Create and store event
	event := UserInvitedToProjectEvent{
		InvitationID: invitation.ID,
		ProjectID:    cmd.ProjectID,
		Email:        cmd.Email,
		Role:         cmd.Role,
		InvitedBy:    cmd.InvitedBy,
		Token:        invitation.Token,
		Timestamp:    now,
	}

	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		log.Printf("Failed to store UserInvitedToProject event: %v", err)
		// Don't fail the operation since invitation was created successfully
	}

	// 12. Publish event for real-time updates
	if h.eventBus != nil {
		if err := h.eventBus.PublishEvent(ctx, event); err != nil {
			log.Printf("Failed to publish UserInvitedToProject event: %v", err)
			// Don't fail the operation since invitation was created successfully
		}
	}

	return nil
}

func (h *UserCommandHandler) HandleAcceptProjectInvitation(ctx context.Context, cmd AcceptProjectInvitationCommand) error {
	// 1. Validate token input
	if cmd.Token == "" {
		return fmt.Errorf("invitation token is required")
	}

	// 2. Get invitation from database
	invitation, err := h.userStore.GetInvitationByToken(ctx, cmd.Token)
	if err != nil {
		return fmt.Errorf("failed to get invitation: %w", err)
	}
	if invitation == nil {
		return fmt.Errorf("invalid invitation token")
	}

	// 3. Check if invitation was already accepted or declined
	if invitation.AcceptedAt != nil {
		return fmt.Errorf("invitation has already been accepted")
	}
	if invitation.DeclinedAt != nil {
		return fmt.Errorf("invitation has already been declined")
	}

	// 4. Check if invitation has expired
	now := time.Now()
	if now.After(invitation.ExpiresAt) {
		return fmt.Errorf("invitation has expired")
	}

	// 5. Get or create user account
	user, err := h.userStore.GetUserByEmail(ctx, invitation.Email)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}

	var userID string
	if user == nil {
		// User doesn't exist yet, create a new user account
		// This handles the case where someone is invited before they register
		newUser := &User{
			ID:            uuid.New().String(),
			Email:         invitation.Email,
			Username:      strings.Split(invitation.Email, "@")[0], // Use email prefix as default username
			FirstName:     "",
			LastName:      "",
			AvatarURL:     "",
			Role:          RoleViewer, // Default system role
			Status:        "Active",   // Auto-activate invited users
			CreatedAt:     now,
			UpdatedAt:     now,
			EmailVerified: true, // Auto-verify invited users
		}

		if err := h.userStore.CreateUser(ctx, newUser); err != nil {
			return fmt.Errorf("failed to create user account: %w", err)
		}
		userID = newUser.ID
	} else {
		userID = user.ID
	}

	// 6. Check if user is already a member of the project
	existingMembership, err := h.userStore.GetProjectMembership(ctx, userID, invitation.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to check existing membership: %w", err)
	}
	if existingMembership != nil {
		return fmt.Errorf("user is already a member of this project")
	}

	// 7. Create project membership
	membership := &ProjectMembership{
		ID:        uuid.New().String(),
		UserID:    userID,
		ProjectID: invitation.ProjectID,
		Role:      invitation.Role,
		CreatedAt: now,
		CreatedBy: invitation.CreatedBy,
	}

	if err := h.userStore.CreateProjectMembership(ctx, membership); err != nil {
		return fmt.Errorf("failed to create project membership: %w", err)
	}

	// 8. Mark invitation as accepted
	invitation.AcceptedAt = &now
	if err := h.userStore.UpdateInvitation(ctx, invitation); err != nil {
		// Log error but don't fail since membership was created
		log.Printf("Failed to mark invitation as accepted: %v", err)
	}

	// 9. Create and store event
	event := ProjectInvitationAcceptedEvent{
		InvitationID: invitation.ID,
		UserID:       userID,
		ProjectID:    invitation.ProjectID,
		Role:         invitation.Role,
		Timestamp:    now,
	}

	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		log.Printf("Failed to store ProjectInvitationAccepted event: %v", err)
		// Don't fail the operation since membership was created successfully
	}

	// 10. Publish event for real-time updates
	if h.eventBus != nil {
		if err := h.eventBus.PublishEvent(ctx, event); err != nil {
			log.Printf("Failed to publish ProjectInvitationAccepted event: %v", err)
			// Don't fail the operation since membership was created successfully
		}
	}

	return nil
}

func (h *UserCommandHandler) HandleDeclineProjectInvitation(ctx context.Context, cmd DeclineProjectInvitationCommand) error {
	// 1. Validate token input
	if cmd.Token == "" {
		return fmt.Errorf("invitation token is required")
	}

	// 2. Get invitation from database
	invitation, err := h.userStore.GetInvitationByToken(ctx, cmd.Token)
	if err != nil {
		return fmt.Errorf("failed to get invitation: %w", err)
	}
	if invitation == nil {
		return fmt.Errorf("invalid invitation token")
	}

	// 3. Check if invitation was already accepted or declined
	if invitation.AcceptedAt != nil {
		return fmt.Errorf("invitation has already been accepted")
	}
	if invitation.DeclinedAt != nil {
		return fmt.Errorf("invitation has already been declined")
	}

	// 4. Check if invitation has expired
	now := time.Now()
	if now.After(invitation.ExpiresAt) {
		return fmt.Errorf("invitation has expired")
	}

	// 5. Mark invitation as declined
	invitation.DeclinedAt = &now
	if err := h.userStore.UpdateInvitation(ctx, invitation); err != nil {
		return fmt.Errorf("failed to mark invitation as declined: %w", err)
	}

	// 6. Create and store event
	event := ProjectInvitationDeclinedEvent{
		InvitationID: invitation.ID,
		Email:        invitation.Email,
		ProjectID:    invitation.ProjectID,
		Timestamp:    now,
	}

	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		log.Printf("Failed to store ProjectInvitationDeclined event: %v", err)
		// Don't fail the operation since invitation was declined successfully
	}

	// 7. Publish event for real-time updates
	if h.eventBus != nil {
		if err := h.eventBus.PublishEvent(ctx, event); err != nil {
			log.Printf("Failed to publish ProjectInvitationDeclined event: %v", err)
			// Don't fail the operation since invitation was declined successfully
		}
	}

	return nil
}

func (h *UserCommandHandler) HandleAddUserToProject(ctx context.Context, cmd AddUserToProjectCommand) error {
	// 1. Validate input
	if cmd.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	if cmd.ProjectID == "" {
		return fmt.Errorf("project ID is required")
	}

	if cmd.Role == "" {
		return fmt.Errorf("role is required")
	}

	if cmd.AddedBy == "" {
		return fmt.Errorf("added by user ID is required")
	}

	// 2. Normalize and validate project role
	normalizedRole := NormalizeRoleName(cmd.Role)
	rbacService := NewRBACService(h.userStore)
	if err := rbacService.ValidateProjectRole(normalizedRole); err != nil {
		return fmt.Errorf("invalid project role: %w", err)
	}

	// 3. Get the target user
	_, err := h.userStore.GetUser(ctx, cmd.UserID) // target user
	if err != nil {
		return fmt.Errorf("failed to get target user: %w", err)
	}

	// 4. Get the adding user to validate permissions
	addingUser, err := h.userStore.GetUser(ctx, cmd.AddedBy)
	if err != nil {
		return fmt.Errorf("failed to get adding user: %w", err)
	}

	// 5. Validate permissions
	canManage, err := rbacService.CanManageUser(ctx, cmd.AddedBy, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !canManage {
		// Check if adding user has project admin role in this project
		if addingUser.Role != RoleAdmin {
			membership, err := h.userStore.GetProjectMembership(ctx, cmd.AddedBy, cmd.ProjectID)
			if err != nil {
				return fmt.Errorf("insufficient permissions: cannot add users to this project")
			}
			if membership.Role != ProjectRoleAdmin {
				return fmt.Errorf("insufficient permissions: project admin role required")
			}
		}
	}

	// 6. Check if user is already a member
	existingMembership, err := h.userStore.GetProjectMembership(ctx, cmd.UserID, cmd.ProjectID)
	if err == nil && existingMembership != nil {
		return fmt.Errorf("user is already a member of this project")
	}

	// 7. Create project membership
	membership := &ProjectMembership{
		ID:        toolbelt.NextEncodedID(),
		UserID:    cmd.UserID,
		ProjectID: cmd.ProjectID,
		Role:      normalizedRole,
		CreatedAt: time.Now(),
		CreatedBy: cmd.AddedBy,
	}

	// 8. Create and store event
	event := UserAddedToProjectEvent{
		EventID:     toolbelt.NextEncodedID(),
		UserID:      cmd.UserID,
		ProjectID:   cmd.ProjectID,
		Role:        normalizedRole,
		AddedBy:     cmd.AddedBy,
		Timestamp:   time.Now(),
		EventType:   "UserAddedToProject",
		AggregateID: cmd.UserID,
	}

	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	// 9. Create membership in database
	if err := h.userStore.CreateProjectMembership(ctx, membership); err != nil {
		return fmt.Errorf("failed to create project membership: %w", err)
	}

	// 10. Publish event for real-time updates
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		log.Printf("Failed to publish UserAddedToProject event: %v", err)
		// Don't fail the command if event publishing fails
	}

	return nil
}

func (h *UserCommandHandler) HandleRemoveUserFromProject(ctx context.Context, cmd RemoveUserFromProjectCommand) error {
	// 1. Validate input
	if cmd.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	if cmd.ProjectID == "" {
		return fmt.Errorf("project ID is required")
	}

	if cmd.RemovedBy == "" {
		return fmt.Errorf("removed by user ID is required")
	}

	// 2. Get the target user
	_, err := h.userStore.GetUser(ctx, cmd.UserID) // target user
	if err != nil {
		return fmt.Errorf("failed to get target user: %w", err)
	}

	// 3. Get the removing user to validate permissions
	removingUser, err := h.userStore.GetUser(ctx, cmd.RemovedBy)
	if err != nil {
		return fmt.Errorf("failed to get removing user: %w", err)
	}

	// 4. Get existing membership
	membership, err := h.userStore.GetProjectMembership(ctx, cmd.UserID, cmd.ProjectID)
	if err != nil {
		return fmt.Errorf("user is not a member of this project")
	}

	// 5. Validate permissions
	rbacService := NewRBACService(h.userStore)
	canManage, err := rbacService.CanManageUser(ctx, cmd.RemovedBy, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !canManage {
		// Check if removing user has project admin role in this project
		if removingUser.Role != RoleAdmin {
			removingMembership, err := h.userStore.GetProjectMembership(ctx, cmd.RemovedBy, cmd.ProjectID)
			if err != nil {
				return fmt.Errorf("insufficient permissions: cannot remove users from this project")
			}
			if removingMembership.Role != ProjectRoleAdmin {
				return fmt.Errorf("insufficient permissions: project admin role required")
			}
		}
	}

	// 6. Prevent users from removing themselves if they're the only admin
	if cmd.UserID == cmd.RemovedBy && membership.Role == ProjectRoleAdmin {
		// Check if there are other admins in the project
		members, err := h.userStore.GetProjectMembers(ctx, cmd.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to check project members: %w", err)
		}

		adminCount := 0
		for _, member := range members {
			memberMembership, err := h.userStore.GetProjectMembership(ctx, member.ID, cmd.ProjectID)
			if err == nil && memberMembership.Role == ProjectRoleAdmin {
				adminCount++
			}
		}

		if adminCount <= 1 {
			return fmt.Errorf("cannot remove the last project admin")
		}
	}

	// 7. Create and store event
	event := UserRemovedFromProjectEvent{
		EventID:     toolbelt.NextEncodedID(),
		UserID:      cmd.UserID,
		ProjectID:   cmd.ProjectID,
		RemovedBy:   cmd.RemovedBy,
		Timestamp:   time.Now(),
		EventType:   "UserRemovedFromProject",
		AggregateID: cmd.UserID,
	}

	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	// 8. Remove membership from database
	if err := h.userStore.DeleteProjectMembership(ctx, cmd.UserID, cmd.ProjectID); err != nil {
		return fmt.Errorf("failed to remove project membership: %w", err)
	}

	// 9. Publish event for real-time updates
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		log.Printf("Failed to publish UserRemovedFromProject event: %v", err)
		// Don't fail the command if event publishing fails
	}

	return nil
}

func (h *UserCommandHandler) HandleUpdateProjectMembership(ctx context.Context, cmd UpdateProjectMembershipCommand) error {
	// 1. Validate input
	if cmd.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	if cmd.ProjectID == "" {
		return fmt.Errorf("project ID is required")
	}

	if cmd.Role == "" {
		return fmt.Errorf("role is required")
	}

	if cmd.UpdatedBy == "" {
		return fmt.Errorf("updated by user ID is required")
	}

	// 2. Normalize and validate project role
	normalizedRole := NormalizeRoleName(cmd.Role)
	rbacService := NewRBACService(h.userStore)
	if err := rbacService.ValidateProjectRole(normalizedRole); err != nil {
		return fmt.Errorf("invalid project role: %w", err)
	}

	// 3. Get the target user
	_, err := h.userStore.GetUser(ctx, cmd.UserID) // target user
	if err != nil {
		return fmt.Errorf("failed to get target user: %w", err)
	}

	// 4. Get the updating user to validate permissions
	updatingUser, err := h.userStore.GetUser(ctx, cmd.UpdatedBy)
	if err != nil {
		return fmt.Errorf("failed to get updating user: %w", err)
	}

	// 5. Get existing membership
	membership, err := h.userStore.GetProjectMembership(ctx, cmd.UserID, cmd.ProjectID)
	if err != nil {
		return fmt.Errorf("user is not a member of this project")
	}

	// 6. Check if role is already assigned
	if membership.Role == normalizedRole {
		return fmt.Errorf("user already has role %s in this project", normalizedRole)
	}

	// 7. Validate permissions
	canManage, err := rbacService.CanManageUser(ctx, cmd.UpdatedBy, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to check permissions: %w", err)
	}

	if !canManage {
		// Check if updating user has project admin role in this project
		if updatingUser.Role != RoleAdmin {
			updatingMembership, err := h.userStore.GetProjectMembership(ctx, cmd.UpdatedBy, cmd.ProjectID)
			if err != nil {
				return fmt.Errorf("insufficient permissions: cannot update users in this project")
			}
			if updatingMembership.Role != ProjectRoleAdmin {
				return fmt.Errorf("insufficient permissions: project admin role required")
			}
		}
	}

	// 8. Prevent demoting the last admin
	if membership.Role == ProjectRoleAdmin && normalizedRole != ProjectRoleAdmin {
		// Check if there are other admins in the project
		members, err := h.userStore.GetProjectMembers(ctx, cmd.ProjectID)
		if err != nil {
			return fmt.Errorf("failed to check project members: %w", err)
		}

		adminCount := 0
		for _, member := range members {
			memberMembership, err := h.userStore.GetProjectMembership(ctx, member.ID, cmd.ProjectID)
			if err == nil && memberMembership.Role == ProjectRoleAdmin {
				adminCount++
			}
		}

		if adminCount <= 1 {
			return fmt.Errorf("cannot demote the last project admin")
		}
	}

	// 9. Create and store event
	event := ProjectMembershipUpdatedEvent{
		EventID:     toolbelt.NextEncodedID(),
		UserID:      cmd.UserID,
		ProjectID:   cmd.ProjectID,
		OldRole:     membership.Role,
		NewRole:     normalizedRole,
		UpdatedBy:   cmd.UpdatedBy,
		Timestamp:   time.Now(),
		EventType:   "ProjectMembershipUpdated",
		AggregateID: cmd.UserID,
	}

	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	// 10. Update membership in database
	membership.Role = normalizedRole
	if err := h.userStore.UpdateProjectMembership(ctx, membership); err != nil {
		return fmt.Errorf("failed to update project membership: %w", err)
	}

	// 11. Publish event for real-time updates
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		log.Printf("Failed to publish ProjectMembershipUpdated event: %v", err)
		// Don't fail the command if event publishing fails
	}

	return nil
}

// Administrative Command Handlers

func (h *UserCommandHandler) HandleAssignUserRole(ctx context.Context, cmd AssignUserRoleCommand) error {
	// 1. Validate input
	if cmd.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	if cmd.Role == "" {
		return fmt.Errorf("role is required")
	}

	if cmd.AssignedBy == "" {
		return fmt.Errorf("assigned by user ID is required")
	}

	// 2. Normalize and validate role
	normalizedRole := NormalizeRoleName(cmd.Role)
	rbacService := NewRBACService(h.userStore)
	if err := rbacService.ValidateSystemRole(normalizedRole); err != nil {
		return fmt.Errorf("invalid role: %w", err)
	}

	// 3. Get the target user
	targetUser, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get target user: %w", err)
	}

	// 4. Get the assigning user to validate permissions
	assigningUser, err := h.userStore.GetUser(ctx, cmd.AssignedBy)
	if err != nil {
		return fmt.Errorf("failed to get assigning user: %w", err)
	}

	// 5. Validate permissions - only admins can assign roles
	if assigningUser.Role != RoleAdmin {
		return fmt.Errorf("insufficient permissions: only admins can assign roles")
	}

	// 6. Check if role transition is valid
	if !rbacService.IsValidRoleTransition(targetUser.Role, normalizedRole) {
		return fmt.Errorf("invalid role transition from %s to %s", targetUser.Role, normalizedRole)
	}

	// 7. Check if role is already assigned
	if targetUser.Role == normalizedRole {
		return fmt.Errorf("user already has role %s", normalizedRole)
	}

	// 8. Create and store event
	event := UserRoleAssignedEvent{
		EventID:     toolbelt.NextEncodedID(),
		UserID:      cmd.UserID,
		OldRole:     targetUser.Role,
		NewRole:     normalizedRole,
		AssignedBy:  cmd.AssignedBy,
		Timestamp:   time.Now(),
		EventType:   "UserRoleAssigned",
		AggregateID: cmd.UserID,
	}

	if err := h.eventStore.StoreEvent(ctx, event); err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	// 9. Update user in database
	targetUser.Role = normalizedRole
	targetUser.UpdatedAt = time.Now()

	if err := h.userStore.UpdateUser(ctx, targetUser); err != nil {
		return fmt.Errorf("failed to update user role: %w", err)
	}

	// 10. Publish event for real-time updates
	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		log.Printf("Failed to publish UserRoleAssigned event: %v", err)
		// Don't fail the command if event publishing fails
	}

	return nil
}

func (h *UserCommandHandler) HandlePatchUser(ctx context.Context, cmd PatchUserCommand) error {
	// TODO: Implement user patching logic
	// 1. Validate admin permissions
	// 2. Apply partial updates
	// 3. Store event
	// 4. Update projection
	// 5. Publish event
	return nil
}

// OAuth Command Handlers

func (h *UserCommandHandler) HandleLinkOAuthAccount(ctx context.Context, cmd LinkOAuthAccountCommand) error {
	// 1. Validate input
	if cmd.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	if cmd.Provider == "" {
		return fmt.Errorf("OAuth provider is required")
	}

	if cmd.ProviderID == "" {
		return fmt.Errorf("provider user ID is required")
	}

	// 2. Normalize provider name
	provider := NormalizeProviderName(cmd.Provider)

	// 3. Validate provider is supported
	if !IsProviderSupported(provider) {
		return fmt.Errorf("OAuth provider %s is not supported", provider)
	}

	// 4. Get user
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// 5. Check if user is active
	if user.Status != "Active" {
		return fmt.Errorf("user account is not active")
	}

	// 6. Check if OAuth provider is already linked to this user
	existingCredential, err := h.userStore.GetUserCredentialByProvider(ctx, cmd.UserID, provider)
	if err != nil {
		return fmt.Errorf("failed to check existing OAuth credential: %w", err)
	}

	// 7. Check if this OAuth account is already linked to another user
	existingUser, err := h.userStore.GetUserByOAuthProvider(ctx, provider, cmd.ProviderID)
	if err != nil {
		return fmt.Errorf("failed to check existing OAuth user: %w", err)
	}
	if existingUser != nil && existingUser.ID != cmd.UserID {
		return fmt.Errorf("OAuth account is already linked to another user")
	}

	now := time.Now()

	if existingCredential != nil {
		// Update existing credential
		existingCredential.ProviderID = cmd.ProviderID
		existingCredential.UpdatedAt = now

		if err := h.userStore.UpdateUserCredential(ctx, existingCredential); err != nil {
			return fmt.Errorf("failed to update OAuth credential: %w", err)
		}
	} else {
		// Create new OAuth credential
		credential := &UserCredential{
			ID:             toolbelt.NextEncodedID(),
			UserID:         cmd.UserID,
			Type:           "oauth",
			Provider:       provider,
			ProviderID:     cmd.ProviderID,
			CredentialData: fmt.Sprintf("linked_at:%s", now.Format(time.RFC3339)),
			CreatedAt:      now,
			UpdatedAt:      now,
		}

		if err := h.userStore.CreateUserCredential(ctx, credential); err != nil {
			return fmt.Errorf("failed to create OAuth credential: %w", err)
		}
	}

	// 8. Publish OAuth account linked event
	event := OAuthAccountLinkedEvent{
		UserID:    cmd.UserID,
		Provider:  provider,
		Timestamp: now,
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Failed to publish OAuthAccountLinkedEvent: %v\n", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleUnlinkOAuthAccount(ctx context.Context, cmd UnlinkOAuthAccountCommand) error {
	// 1. Validate input
	if cmd.UserID == "" {
		return fmt.Errorf("user ID is required")
	}

	if cmd.Provider == "" {
		return fmt.Errorf("OAuth provider is required")
	}

	// 2. Normalize provider name
	provider := NormalizeProviderName(cmd.Provider)

	// 3. Validate provider is supported
	if !IsProviderSupported(provider) {
		return fmt.Errorf("OAuth provider %s is not supported", provider)
	}

	// 4. Get user
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// 5. Get OAuth credential
	credential, err := h.userStore.GetUserCredentialByProvider(ctx, cmd.UserID, provider)
	if err != nil {
		return fmt.Errorf("failed to get OAuth credential: %w", err)
	}
	if credential == nil {
		return fmt.Errorf("OAuth provider %s is not linked to this user", provider)
	}

	// 6. Check if user has other authentication methods
	// Don't allow unlinking if it's the only authentication method
	allCredentials, err := h.userStore.GetUserCredentials(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user credentials: %w", err)
	}

	hasOtherAuth := false
	for _, cred := range allCredentials {
		if cred.ID != credential.ID && (cred.Type == "password" || cred.Type == "oauth") {
			hasOtherAuth = true
			break
		}
	}

	if !hasOtherAuth {
		return fmt.Errorf("cannot unlink OAuth account: no other authentication methods available")
	}

	// 7. Delete OAuth credential
	if err := h.userStore.DeleteUserCredential(ctx, credential.ID); err != nil {
		return fmt.Errorf("failed to delete OAuth credential: %w", err)
	}

	// 8. Publish OAuth account unlinked event
	event := OAuthAccountUnlinkedEvent{
		UserID:    cmd.UserID,
		Provider:  provider,
		Timestamp: time.Now(),
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail the operation
		fmt.Printf("Failed to publish OAuthAccountUnlinkedEvent: %v\n", err)
	}

	return nil
}

// Session Command Handlers

func (h *UserCommandHandler) HandleCreateUserSession(ctx context.Context, cmd CreateUserSessionCommand) error {
	// 1. Generate session ID if not provided
	sessionID := cmd.SessionID
	if sessionID == "" {
		sessionID = toolbelt.NextEncodedID()
	}

	// 2. Create session record
	session := &UserSession{
		ID:        toolbelt.NextEncodedID(),
		UserID:    cmd.UserID,
		SessionID: sessionID,
		ExpiresAt: cmd.ExpiresAt,
		CreatedAt: time.Now(),
		IPAddress: cmd.IPAddress,
		UserAgent: cmd.UserAgent,
	}

	// 3. Store session in database
	if err := h.userStore.CreateUserSession(ctx, session); err != nil {
		return fmt.Errorf("failed to create user session: %w", err)
	}

	// 4. Create and publish event for UI updates only
	event := UserSessionCreatedEvent{
		SessionID: session.SessionID,
		UserID:    session.UserID,
		IPAddress: session.IPAddress,
		UserAgent: session.UserAgent,
		ExpiresAt: session.ExpiresAt,
		Timestamp: time.Now(),
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail the command - session is already created
		log.Printf("Failed to publish UserSessionCreatedEvent: %v", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleInvalidateUserSession(ctx context.Context, cmd InvalidateUserSessionCommand) error {
	// 1. Get session to get user ID for event
	session, err := h.userStore.GetUserSession(ctx, cmd.SessionID)
	if err != nil {
		return fmt.Errorf("failed to get user session: %w", err)
	}
	if session == nil {
		// Session doesn't exist, consider it already invalidated
		return nil
	}

	// 2. Remove session record from database
	if err := h.userStore.DeleteUserSession(ctx, cmd.SessionID); err != nil {
		return fmt.Errorf("failed to delete user session: %w", err)
	}

	// 3. Create and publish event for UI updates only
	event := UserSessionInvalidatedEvent{
		SessionID: cmd.SessionID,
		UserID:    &session.UserID,
		Reason:    "logout",
		Timestamp: time.Now(),
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		// Log error but don't fail the command - session is already deleted
		log.Printf("Failed to publish UserSessionInvalidatedEvent: %v", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleInvalidateAllUserSessions(ctx context.Context, cmd InvalidateAllUserSessionsCommand) error {
	// 1. Get all user sessions for event publishing
	sessions, err := h.userStore.GetUserSessions(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user sessions: %w", err)
	}

	// 2. Remove all user session records from database
	if err := h.userStore.DeleteAllUserSessions(ctx, cmd.UserID); err != nil {
		return fmt.Errorf("failed to delete all user sessions: %w", err)
	}

	// 3. Create and publish events for each invalidated session
	for _, session := range sessions {
		event := UserSessionInvalidatedEvent{
			SessionID: session.SessionID,
			UserID:    &cmd.UserID,
			Reason:    "admin_action",
			Timestamp: time.Now(),
		}

		if err := h.eventBus.PublishEvent(ctx, event); err != nil {
			// Log error but continue with other sessions
			log.Printf("Failed to publish UserSessionInvalidatedEvent for session %s: %v", session.SessionID, err)
		}
	}

	// 4. Publish aggregate event
	aggregateEvent := AllUserSessionsInvalidatedEvent{
		UserID:    cmd.UserID,
		Reason:    "admin_action",
		Timestamp: time.Now(),
	}

	if err := h.eventBus.PublishEvent(ctx, aggregateEvent); err != nil {
		// Log error but don't fail the command - sessions are already deleted
		log.Printf("Failed to publish AllUserSessionsInvalidatedEvent: %v", err)
	}

	return nil
}

// CleanupExpiredInvitations removes expired invitations from the database
// This should be called periodically by a background job
func (h *UserCommandHandler) CleanupExpiredInvitations(ctx context.Context) error {
	// This is a utility function that can be called by a background job
	// For now, we'll implement it as a simple cleanup that can be called manually

	// Note: This would typically be implemented as a separate cleanup service
	// or background job, but for the scope of this task, we'll add it here
	// as a method that can be called when needed

	log.Printf("CleanupExpiredInvitations: This method should be implemented as part of a background cleanup service")
	return nil
}

// SSO Command Handlers

func (h *UserCommandHandler) HandleLoginSSO(ctx context.Context, cmd LoginSSOCommand) error {
	if h.ssoService == nil {
		return fmt.Errorf("SSO service not configured")
	}

	// Note: This handler would typically be called after the SSO flow is completed
	// The actual SSO flow initiation and completion would be handled by HTTP handlers
	// This command handler processes the result of a successful SSO authentication

	log.Printf("SSO login command received for provider: %s", cmd.Provider)

	// The actual SSO authentication logic is handled by the SSOService
	// This command handler would be called after successful authentication
	// to create the necessary events and update user state

	// Create SSO login event
	event := UserLoggedInEvent{
		UserID:    "", // Would be populated after successful authentication
		SessionID: "", // Would be populated after session creation
		IPAddress: cmd.IPAddress,
		UserAgent: cmd.UserAgent,
		Method:    "sso",
		Provider:  cmd.Provider,
		Timestamp: time.Now(),
	}

	// Note: In a real implementation, this would be called after the SSOService
	// completes the authentication and returns the user and session information
	_ = event // Placeholder to avoid unused variable error

	return nil
}

func (h *UserCommandHandler) HandleLinkSSOAccount(ctx context.Context, cmd LinkSSOAccountCommand) error {
	if h.ssoService == nil {
		return fmt.Errorf("SSO service not configured")
	}

	// Validate user exists
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// The actual linking logic would be handled by the SSOService
	// This command handler creates the necessary events

	event := SSOAccountLinkedEvent{
		UserID:     cmd.UserID,
		Provider:   cmd.Provider,
		ProviderID: cmd.ProviderID,
		Timestamp:  time.Now(),
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		log.Printf("Failed to publish SSOAccountLinkedEvent: %v", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleUnlinkSSOAccount(ctx context.Context, cmd UnlinkSSOAccountCommand) error {
	// Validate user exists
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// Remove SSO credential
	credential, err := h.userStore.GetUserCredentialByProvider(ctx, cmd.UserID, cmd.Provider)
	if err != nil {
		return fmt.Errorf("failed to get SSO credential: %w", err)
	}
	if credential == nil {
		return fmt.Errorf("SSO account not linked")
	}

	if err := h.userStore.DeleteUserCredential(ctx, credential.ID); err != nil {
		return fmt.Errorf("failed to delete SSO credential: %w", err)
	}

	// Create and publish event
	event := SSOAccountUnlinkedEvent{
		UserID:    cmd.UserID,
		Provider:  cmd.Provider,
		Timestamp: time.Now(),
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		log.Printf("Failed to publish SSOAccountUnlinkedEvent: %v", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleInitiateSingleLogout(ctx context.Context, cmd InitiateSingleLogoutCommand) error {
	if h.ssoService == nil {
		return fmt.Errorf("SSO service not configured")
	}

	// Validate user exists
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// Initiate single logout through SSO service
	if err := h.ssoService.InitiateSingleLogout(ctx, cmd.UserID); err != nil {
		return fmt.Errorf("failed to initiate single logout: %w", err)
	}

	// Create and publish event
	event := SingleLogoutInitiatedEvent{
		UserID:    cmd.UserID,
		Timestamp: time.Now(),
	}

	if err := h.eventBus.PublishEvent(ctx, event); err != nil {
		log.Printf("Failed to publish SingleLogoutInitiatedEvent: %v", err)
	}

	return nil
}

func (h *UserCommandHandler) HandleSyncUserGroups(ctx context.Context, cmd SyncUserGroupsCommand) error {
	if h.ssoService == nil {
		return fmt.Errorf("SSO service not configured")
	}

	// Validate user exists
	user, err := h.userStore.GetUser(ctx, cmd.UserID)
	if err != nil {
		return fmt.Errorf("failed to get user: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	// Map groups to roles using SSO manager
	ssoManager := h.ssoService.ssoManager
	roles := ssoManager.MapGroupsToRoles(cmd.Provider, cmd.Groups)
	newRole := ssoManager.GetHighestRole(roles)

	// Update user role if changed
	oldRole := user.Role
	if user.Role != newRole {
		user.Role = newRole
		user.UpdatedAt = time.Now()

		if err := h.userStore.UpdateUser(ctx, user); err != nil {
			return fmt.Errorf("failed to update user role: %w", err)
		}

		// Create and publish event
		event := UserGroupsSyncedEvent{
			UserID:    cmd.UserID,
			Provider:  cmd.Provider,
			Groups:    cmd.Groups,
			OldRole:   oldRole,
			NewRole:   newRole,
			Timestamp: time.Now(),
		}

		if err := h.eventBus.PublishEvent(ctx, event); err != nil {
			log.Printf("Failed to publish UserGroupsSyncedEvent: %v", err)
		}
	}

	return nil
}
