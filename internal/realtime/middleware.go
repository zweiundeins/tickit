package realtime

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/sessions"
	"github.com/justinas/nosurf"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/sharedhttp"
	"github.com/starfederation/datastar-go/datastar"
)

// Helper function to get map keys for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Package-level storage for subscription contexts per connection
var (
	connectionContextMap = make(map[string]*SubscriptionContext)
	connectionContextMu  sync.RWMutex
)

// StoreSubscriptionContext stores a subscription context by connection ID
func StoreSubscriptionContext(connectionID string, ctx *SubscriptionContext) {
	connectionContextMu.Lock()
	defer connectionContextMu.Unlock()
	connectionContextMap[connectionID] = ctx
}

// NotificationContextMiddleware fetches user notifications and adds them to request context
func NotificationContextMiddleware(notificationService *notifications.NotificationService) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only fetch notifications for authenticated users
			if userID, ok := sharedhttp.GetUserIDFromContext(r.Context()); ok && userID != "" {
				// Create a context with user_id for the notification service
				ctx := context.WithValue(r.Context(), "user_id", userID)

				logger.Debug().
					Str("user_id", userID).
					Msg("NotificationContextMiddleware: Fetching notifications for user")

				// Fetch both user-specific and general notifications
				if userNotifications, err := notificationService.GetAllNotificationsForUser(ctx); err == nil {
					logger.Debug().
						Str("user_id", userID).
						Int("notification_count", len(userNotifications.Notifications)).
						Msg("NotificationContextMiddleware: Successfully fetched all notifications")
					// Add notifications to request context
					ctx = context.WithValue(ctx, "user_notifications", userNotifications)
					r = r.WithContext(ctx)
				} else {
					logger.Error().
						Str("user_id", userID).
						Err(err).
						Msg("NotificationContextMiddleware: Failed to fetch all notifications")
				}
			} else {
				logger.Debug().Msg("NotificationContextMiddleware: No authenticated user found")
			}
			next.ServeHTTP(w, r)
		})
	}
} // SetEventPatterns creates and stores a subscription context with the specified event patterns
// This replaces the need for subscription middlewares - call this directly in route handlers
func SetEventPatterns(r *http.Request, eventPatterns []string) {
	connectionID, ok := r.Context().Value("connection_id").(string)
	if !ok {
		// If no connection ID, we can't set patterns - this is not an error as
		// some routes may not need realtime subscriptions
		return
	}

	subCtx := &SubscriptionContext{
		Filter:        make(map[string]interface{}),
		EventPatterns: eventPatterns,
	}

	StoreSubscriptionContext(connectionID, subCtx)

	logger.Debug().
		Str("connection_id", connectionID).
		Strs("event_patterns", eventPatterns).
		Msg("Set event patterns for request")
}

// SetEventPatternsWithFilter creates a subscription context with patterns and filter data
// Useful for routes that need query parameter parsing and filtering
func SetEventPatternsWithFilter(r *http.Request, eventPatterns []string, filter map[string]interface{}) {
	connectionID, ok := r.Context().Value("connection_id").(string)
	if !ok {
		return
	}

	subCtx := &SubscriptionContext{
		Filter:        filter,
		EventPatterns: eventPatterns,
	}

	StoreSubscriptionContext(connectionID, subCtx)

	logger.Debug().
		Str("connection_id", connectionID).
		Strs("event_patterns", eventPatterns).
		Interface("filter", filter).
		Msg("Set event patterns with filter for request")
}

// ParseProjectsTableFilter parses query parameters for projects table view
// Helper function to extract common filter patterns
func ParseProjectsTableFilter(r *http.Request) map[string]interface{} {
	filter := make(map[string]interface{})

	// Default pagination
	filter["page"] = 1
	filter["page_size"] = 10

	// Parse query parameters
	if page := r.URL.Query().Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			filter["page"] = p
		}
	}
	if pageSize := r.URL.Query().Get("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil {
			filter["page_size"] = ps
		}
	}

	// Extract filter criteria
	filterData := make(map[string]interface{})
	if status := r.URL.Query().Get("status"); status != "" {
		filterData["status"] = status
	}
	if ownerID := r.URL.Query().Get("owner_id"); ownerID != "" {
		filterData["owner_id"] = ownerID
	}

	if len(filterData) > 0 {
		filter["filter"] = filterData
	}

	return filter
}

// ParseIssuesTableFilter parses query parameters for issues table view
// Helper function to extract common filter patterns
func ParseIssuesTableFilter(r *http.Request) map[string]interface{} {
	filter := make(map[string]interface{})

	// Default pagination
	filter["page"] = 1
	filter["page_size"] = 25

	// Parse query parameters for filters
	if page := r.URL.Query().Get("page"); page != "" {
		if p, err := strconv.Atoi(page); err == nil {
			filter["page"] = p
		}
	}
	if pageSize := r.URL.Query().Get("page_size"); pageSize != "" {
		if ps, err := strconv.Atoi(pageSize); err == nil {
			filter["page_size"] = ps
		}
	}

	// Extract filter criteria
	filterData := make(map[string]interface{})
	if status := r.URL.Query().Get("status"); status != "" {
		filterData["status"] = status
	}
	if priority := r.URL.Query().Get("priority"); priority != "" {
		filterData["priority"] = priority
	}
	if projectID := r.URL.Query().Get("project_id"); projectID != "" {
		filterData["project_id"] = projectID
	}
	if assigneeID := r.URL.Query().Get("assignee_id"); assigneeID != "" {
		filterData["assignee_id"] = assigneeID
	}

	if len(filterData) > 0 {
		filter["filter"] = filterData
	}

	return filter
}

// GetSubscriptionContextByConnectionID retrieves a subscription context by connection ID
func GetSubscriptionContextByConnectionID(connectionID string) (*SubscriptionContext, bool) {
	connectionContextMu.RLock()
	defer connectionContextMu.RUnlock()
	subCtx, exists := connectionContextMap[connectionID]
	return subCtx, exists
}

// GetAllSubscriptionContexts returns all active subscription contexts
func GetAllSubscriptionContexts() map[string]*SubscriptionContext {
	connectionContextMu.RLock()
	defer connectionContextMu.RUnlock()
	// Create a copy to avoid external modifications
	result := make(map[string]*SubscriptionContext)
	for k, v := range connectionContextMap {
		result[k] = v
	}
	return result
}

// RemoveSubscriptionContext removes subscription context for a connection ID
func RemoveSubscriptionContext(connectionID string) {
	connectionContextMu.Lock()
	defer connectionContextMu.Unlock()
	delete(connectionContextMap, connectionID)
}

// SubscriptionContext holds subscription metadata for a request
type SubscriptionContext struct {
	Filter        map[string]interface{} `json:"filter"`         // Filtering criteria for the subscription
	EventPatterns []string               `json:"event_patterns"` // Event patterns to subscribe to (e.g., ["issues.*", "notifications.*"])
}

// GetRenderers returns all renderers that match this subscription's event patterns
func (sc *SubscriptionContext) GetRenderers() map[string]ComponentRenderer {
	return GetRenderersForPatterns(sc.EventPatterns)
}

// GetRenderer returns the best matching renderer for a specific event type
func (sc *SubscriptionContext) GetRenderer(eventType string) ComponentRenderer {
	return GetRenderer(eventType)
}

type contextKey string

const subscriptionContextKey contextKey = "subscription_context"

// WithSubscriptionContext adds subscription context to the request
func WithSubscriptionContext(ctx context.Context, subCtx *SubscriptionContext) context.Context {
	return context.WithValue(ctx, subscriptionContextKey, subCtx)
}

// GetSubscriptionContext retrieves subscription context from the request
func GetSubscriptionContext(ctx context.Context) (*SubscriptionContext, bool) {
	subCtx, ok := ctx.Value(subscriptionContextKey).(*SubscriptionContext)
	return subCtx, ok
}

func SetSessionMiddleware(store sessions.Store) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			sessionID, err := sharedhttp.UpsertSessionID(store, r, w)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			ctx := context.WithValue(r.Context(), "session_id", sessionID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func DatastarSignalsMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Only try to read signals for appropriate content types
			// Skip for form submissions to avoid interfering with CSRF
			datastarRequest := r.Header.Get("Datastar-Request")
			signals := make(map[string]interface{})

			// Only read signals if this looks like a datastar request
			if datastarRequest == "true" {
				err := datastar.ReadSignals(r, &signals)
				if err != nil {
					logger.Error().Err(err).Str("url", r.URL.String()).Str("method", r.Method).Msg("DatastarSignalsMiddleware: Error reading signals")
				}

				if len(signals) > 0 {
					logger.Debug().Interface("signals", signals).Msg("DatastarSignalsMiddleware: Parsed signals")
				}
			}

			// Store signals in context for later use
			ctx := context.WithValue(r.Context(), "datastar_signals", signals)
			next.ServeHTTP(w, r.WithContext(ctx))
		})

	}
}

func DefaultSubscriptionContextMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			signals := r.Context().Value("datastar_signals")
			subCtx := &SubscriptionContext{
				Filter:        make(map[string]interface{}),
				EventPatterns: []string{}, // Default: no patterns, no events
			} // Update subscription context with current signals
			UpdateSubscriptionFromSignals(subCtx, signals.(map[string]interface{}))

			ctx := WithSubscriptionContext(r.Context(), subCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ConnectionIDMiddleware generates and manages connection IDs for page requests
func ConnectionIDMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var connectionID string

			// Check if connection ID is provided in signals (from existing tab/connection)
			signals := r.Context().Value("datastar_signals")
			if signals != nil {
				if signalsMap, ok := signals.(map[string]interface{}); ok {
					if existingID, ok := signalsMap["connectionId"].(string); ok && existingID != "" {
						connectionID = existingID
					}
				}
			}

			// Generate new connection ID if not provided in signals
			if connectionID == "" {
				sessionID := r.Context().Value("session_id").(string)
				connectionID = fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano())
			}

			// Store connection ID in context
			ctx := context.WithValue(r.Context(), "connection_id", connectionID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CSRFToContextMiddleware adds CSRF token to request context
func CSRFToContextMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Get CSRF token from nosurf
			csrfToken := nosurf.Token(r)
			if csrfToken != "" {
				ctx := context.WithValue(r.Context(), "csrf_token", csrfToken)
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ExtractSignalsFilter extracts filter data from Datastar signals
// This would be called when processing SSE requests with signal data
func ExtractSignalsFilter(signals map[string]interface{}) map[string]interface{} {
	filter := make(map[string]interface{})

	// Extract pagination
	if page, ok := signals["page"].(float64); ok {
		filter["page"] = int(page)
	} else {
		filter["page"] = 1
	}

	if pageSize, ok := signals["page_size"].(float64); ok {
		filter["page_size"] = int(pageSize)
	} else {
		filter["page_size"] = 25
	}

	// Extract filter object
	if filterData, ok := signals["filter"].(map[string]interface{}); ok {
		filter["filter"] = filterData
	}

	return filter
}

// UpdateSubscriptionFromSignals updates subscription context with signal data
func UpdateSubscriptionFromSignals(subCtx *SubscriptionContext, signals map[string]interface{}) {
	// Update filter with current signal state
	newFilter := ExtractSignalsFilter(signals)

	// Preserve entity type and ID, update filter
	for key, value := range newFilter {
		subCtx.Filter[key] = value
	}
}

// MatchesEventPattern checks if an event type matches any of the subscription patterns
// Supports wildcard patterns like "issues.*" or "notifications.user123.*"
func MatchesEventPattern(eventType string, patterns []string) bool {
	for _, pattern := range patterns {
		if matchPattern(eventType, pattern) {
			return true
		}
	}
	return false
}

// matchPattern checks if an event type matches a pattern with wildcards
func matchPattern(eventType, pattern string) bool {
	// Handle exact matches
	if eventType == pattern {
		return true
	}

	// Handle wildcard patterns
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(eventType, prefix)
	}

	return false
}
