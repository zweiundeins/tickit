package users

import (
	"context"
	"fmt"
	"log"
	"os"
)

// EmailServiceImpl implements the EmailService interface
// This is a simple implementation that logs emails in development mode
// In production, this would integrate with an actual email service like SendGrid, AWS SES, etc.
type EmailServiceImpl struct {
	isDevelopment bool
	baseURL       string
}

// NewEmailService creates a new EmailService instance
func NewEmailService(isDevelopment bool, baseURL string) *EmailServiceImpl {
	return &EmailServiceImpl{
		isDevelopment: isDevelopment,
		baseURL:       baseURL,
	}
}

// SendVerificationEmail sends an email verification email to the user
func (s *EmailServiceImpl) SendVerificationEmail(ctx context.Context, email, token string) error {
	verificationURL := fmt.Sprintf("%s/auth/verify-email?token=%s", s.baseURL, token)

	subject := "Verify your email address - Tickit"
	body := fmt.Sprintf(`
Hello,

Please verify your email address by clicking the link below:

%s

This link will expire in 24 hours.

If you didn't create an account with Tickit, you can safely ignore this email.

Best regards,
The Tickit Team
`, verificationURL)

	if s.isDevelopment {
		// In development mode, log the email instead of sending it
		log.Printf("=== EMAIL VERIFICATION ===")
		log.Printf("To: %s", email)
		log.Printf("Subject: %s", subject)
		log.Printf("Verification URL: %s", verificationURL)
		log.Printf("Body: %s", body)
		log.Printf("========================")
		return nil
	}

	// In production, this would integrate with an actual email service
	// For now, we'll just log and return an error to indicate it's not implemented
	log.Printf("Email service not configured for production. Would send verification email to: %s", email)
	return fmt.Errorf("email service not configured for production environment")
}

// SendPasswordResetEmail sends a password reset email to the user
func (s *EmailServiceImpl) SendPasswordResetEmail(ctx context.Context, email, token string) error {
	resetURL := fmt.Sprintf("%s/auth/reset-password?token=%s", s.baseURL, token)

	subject := "Reset your password - Tickit"
	body := fmt.Sprintf(`
Hello,

You requested to reset your password. Click the link below to set a new password:

%s

This link will expire in 1 hour.

If you didn't request a password reset, you can safely ignore this email.

Best regards,
The Tickit Team
`, resetURL)

	if s.isDevelopment {
		// In development mode, log the email instead of sending it
		log.Printf("=== PASSWORD RESET EMAIL ===")
		log.Printf("To: %s", email)
		log.Printf("Subject: %s", subject)
		log.Printf("Reset URL: %s", resetURL)
		log.Printf("Body: %s", body)
		log.Printf("===========================")
		return nil
	}

	// In production, this would integrate with an actual email service
	log.Printf("Email service not configured for production. Would send password reset email to: %s", email)
	return fmt.Errorf("email service not configured for production environment")
}

// SendInvitationEmail sends a project invitation email to the user
func (s *EmailServiceImpl) SendInvitationEmail(ctx context.Context, email, projectName, token string) error {
	invitationURL := fmt.Sprintf("%s/invitations/accept?token=%s", s.baseURL, token)

	subject := fmt.Sprintf("You've been invited to join %s - Tickit", projectName)
	body := fmt.Sprintf(`
Hello,

You've been invited to join the project "%s" on Tickit.

Click the link below to accept the invitation:

%s

This invitation will expire in 7 days.

If you don't have a Tickit account yet, you'll be able to create one when you accept the invitation.

Best regards,
The Tickit Team
`, projectName, invitationURL)

	if s.isDevelopment {
		// In development mode, log the email instead of sending it
		log.Printf("=== PROJECT INVITATION EMAIL ===")
		log.Printf("To: %s", email)
		log.Printf("Subject: %s", subject)
		log.Printf("Project: %s", projectName)
		log.Printf("Invitation URL: %s", invitationURL)
		log.Printf("Body: %s", body)
		log.Printf("===============================")
		return nil
	}

	// In production, this would integrate with an actual email service
	log.Printf("Email service not configured for production. Would send invitation email to: %s", email)
	return fmt.Errorf("email service not configured for production environment")
}

// SendMagicMailLink sends a magic mail login link (development only)
func (s *EmailServiceImpl) SendMagicMailLink(ctx context.Context, email, token string) error {
	if !s.isDevelopment {
		return fmt.Errorf("magic mail is only available in development mode")
	}

	magicURL := fmt.Sprintf("%s/auth/magic-login?token=%s", s.baseURL, token)

	// Always log magic mail links in development
	log.Printf("=== MAGIC MAIL LOGIN ===")
	log.Printf("To: %s", email)
	log.Printf("Magic Login URL: %s", magicURL)
	log.Printf("=======================")

	return nil
}

// NewEmailServiceFromEnv creates an EmailService from environment variables
func NewEmailServiceFromEnv() *EmailServiceImpl {
	isDevelopment := os.Getenv("ENVIRONMENT") != "production"
	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080" // Default for development
	}

	return NewEmailService(isDevelopment, baseURL)
}
