package sharedhttp

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/delaneyj/toolbelt"
	"github.com/gorilla/sessions"
	internalSession "github.com/mbolli/tickit/internal/session"
	"github.com/starfederation/datastar-go/datastar"
)

// upsertSessionID ensures a session ID exists and returns it
func UpsertSessionID(store sessions.Store, r *http.Request, w http.ResponseWriter) (string, error) {
	sess, err := store.Get(r, "connections")
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	id, ok := sess.Values["id"].(string)

	if !ok {
		id = toolbelt.NextEncodedID()
		sess.Values["id"] = id
		if err := sess.Save(r, w); err != nil {
			return "", fmt.Errorf("failed to save session: %w", err)
		}
	}

	return id, nil
}

// WithSessionAndSignals prepares sessionID and reads signals into the provided struct
func WithSessionAndSignals(store sessions.Store, w http.ResponseWriter, r *http.Request, signals any, handler func(sessionID string, signals any)) {
	sessionID, err := UpsertSessionID(store, r, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if signals != nil {
		_ = datastar.ReadSignals(r, signals) // Ignore error, signals will be zero if not provided
	}
	handler(sessionID, signals)
}

// GetAuthenticatedUser extracts the authenticated user from the session
func GetAuthenticatedUser(store sessions.Store, r *http.Request) (string, bool) {
	sessionName := internalSession.GetSessionName()
	session, err := store.Get(r, sessionName)
	if err != nil {
		userAgent := r.UserAgent()
		if len(userAgent) > 50 {
			userAgent = userAgent[:50] + "..."
		}

		// Log what cookies we received
		cookies := r.Cookies()
		cookieNames := make([]string, len(cookies))
		for i, cookie := range cookies {
			cookieNames[i] = cookie.Name
		}

		slog.Info("Failed to get session",
			"error", err.Error(),
			"url", r.URL.Path,
			"method", r.Method,
			"user_agent", userAgent,
			"cookies_received", cookieNames)
		// If we can't get the session due to invalid cookie, try to clear it
		if strings.Contains(err.Error(), "securecookie") {
			slog.Info("Attempting to clear invalid session cookie", "url", r.URL.Path)
			// Create a new session to clear the invalid cookie
			if newSession, newErr := store.New(r, sessionName); newErr == nil {
				newSession.Options.MaxAge = -1 // This will delete the cookie
				// We can't save it here because we don't have ResponseWriter, but the caller should handle this
			}
		}
		return "", false
	}

	// Check if session is authenticated
	authenticated, ok := session.Values["authenticated"].(bool)
	if !ok || !authenticated {
		return "", false
	}

	// Get user ID from session
	userID, ok := session.Values["user_id"].(string)
	if !ok || userID == "" {
		return "", false
	}

	return userID, true
}

// SetAuthenticatedUser sets the user as authenticated in the session
func SetAuthenticatedUser(store sessions.Store, w http.ResponseWriter, r *http.Request, userID string) error {
	sessionName := internalSession.GetSessionName()
	session, err := store.Get(r, sessionName)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Ensure session has an ID
	if _, ok := session.Values["id"].(string); !ok {
		session.Values["id"] = toolbelt.NextEncodedID()
	}

	// Set authentication values
	session.Values["user_id"] = userID
	session.Values["authenticated"] = true

	// Save session
	if err := session.Save(r, w); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// ClearAuthenticatedUser removes authentication from the session
func ClearAuthenticatedUser(store sessions.Store, w http.ResponseWriter, r *http.Request) error {
	sessionName := internalSession.GetSessionName()
	session, err := store.Get(r, sessionName)
	if err != nil {
		return fmt.Errorf("failed to get session: %w", err)
	}

	// Clear authentication values
	delete(session.Values, "user_id")
	delete(session.Values, "authenticated")

	// Set MaxAge to -1 to delete the cookie
	session.Options.MaxAge = -1

	// Save session
	if err := session.Save(r, w); err != nil {
		return fmt.Errorf("failed to save session: %w", err)
	}

	return nil
}

// RequireAuthentication is a middleware that requires user authentication
func RequireAuthentication(store sessions.Store, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, authenticated := GetAuthenticatedUser(store, r)
		if !authenticated {
			http.Redirect(w, r, "/auth/login", 302)
			return
		}

		// Add user ID to request context
		ctx := context.WithValue(r.Context(), "user_id", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

// GetUserIDFromContext extracts the user ID from the request context
func GetUserIDFromContext(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value("user_id").(string)
	return userID, ok
}
