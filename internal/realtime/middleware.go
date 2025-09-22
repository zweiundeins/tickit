package realtime

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/sharedhttp"
	"github.com/starfederation/datastar-go/datastar"

	"github.com/go-chi/chi/v5"
)

// Helper function to get map keys for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Connection-based subscription context storage
var (
	connectionContextMap = make(map[string]*SubscriptionContext)
	connectionContextMu  sync.RWMutex
)

// StoreSubscriptionContext stores subscription context for a connection ID
func StoreSubscriptionContext(connectionID string, subCtx *SubscriptionContext) {
	connectionContextMu.Lock()
	defer connectionContextMu.Unlock()
	connectionContextMap[connectionID] = subCtx
}

// GetSubscriptionContext retrieves subscription context for a connection ID
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
	EntityType string                 `json:"entity_type"`
	EntityID   *string                `json:"entity_id"`
	Filter     map[string]interface{} `json:"filter"`
	Renderer   ComponentRenderer      `json:"-"`
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
			// Parse Datastar signals from request
			signals := make(map[string]interface{})
			err := datastar.ReadSignals(r, &signals)
			if err != nil {
				logger.Error().Err(err).Msg("DatastarSignalsMiddleware: Error reading signals")
			}

			if len(signals) > 0 {
				logger.Debug().Interface("signals", signals).Msg("DatastarSignalsMiddleware: Parsed signals")
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
				EntityType: "none",
				EntityID:   nil,
				Filter:     make(map[string]interface{}),
				Renderer:   nil,
			}

			// Update subscription context with current signals
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
						logger.Info().Str("connection_id", connectionID).Msg("ConnectionIDMiddleware: Reusing existing connection ID from signals")
					}
				}
			} else {
				logger.Warn().Str("Method", r.Method).Str("URL", r.URL.Path).Msg("ConnectionIDMiddleware: no datastar_signals set in context")
			}

			// Generate new connection ID if not provided in signals
			if connectionID == "" {
				sessionID := r.Context().Value("session_id").(string)
				connectionID = fmt.Sprintf("%s-%d", sessionID, time.Now().UnixNano())
				logger.Info().Str("connection_id", connectionID).Str("URL", r.URL.String()).Msg("ConnectionIDMiddleware: Generated new connection ID")
			}

			// Store connection ID in context
			ctx := context.WithValue(r.Context(), "connection_id", connectionID)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IssuesTableSubscription middleware for issues dashboard/table views
func IssuesTableSubscription(renderer ComponentRenderer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			logger.Info().Str("Method", r.Method).Str("URL", r.URL.Path).Msg("IssuesTableSubscription: Processing request")

			// Extract filter and pagination from query params or signals
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

			subCtx := &SubscriptionContext{
				EntityType: "issues",
				EntityID:   nil, // nil for table views
				Filter:     filter,
				Renderer:   renderer,
			}

			// Store subscription context by connection ID
			if connectionID, ok := r.Context().Value("connection_id").(string); ok {
				logger.Info().Str("connection_id", connectionID).Msg("IssuesTableSubscription: Storing subscription context")
				StoreSubscriptionContext(connectionID, subCtx)
			} else {
				logger.Warn().Msg("IssuesTableSubscription: connection_id not found in context")
			}

			ctx := WithSubscriptionContext(r.Context(), subCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// IssueDetailSubscription middleware for individual issue detail views
func IssueDetailSubscription(renderer ComponentRenderer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract issue ID from URL path
			issueID := chi.URLParam(r, "issueId")
			if issueID == "" {
				http.Error(w, "Issue ID required", http.StatusBadRequest)
				return
			}

			subCtx := &SubscriptionContext{
				EntityType: "issues",
				EntityID:   &issueID,
				Filter:     make(map[string]interface{}), // No filtering for detail views
				Renderer:   renderer,
			}

			ctx := WithSubscriptionContext(r.Context(), subCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ProjectsTableSubscription middleware for projects dashboard/table views
func ProjectsTableSubscription(renderer ComponentRenderer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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

			subCtx := &SubscriptionContext{
				EntityType: "projects",
				EntityID:   nil,
				Filter:     filter,
				Renderer:   renderer,
			}

			ctx := WithSubscriptionContext(r.Context(), subCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// ProjectDetailSubscription middleware for individual project detail views
func ProjectDetailSubscription(renderer ComponentRenderer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			projectID := chi.URLParam(r, "id")
			if projectID == "" {
				http.Error(w, "Project ID required", http.StatusBadRequest)
				return
			}

			subCtx := &SubscriptionContext{
				EntityType: "projects",
				EntityID:   &projectID,
				Filter:     make(map[string]interface{}),
				Renderer:   renderer,
			}

			ctx := WithSubscriptionContext(r.Context(), subCtx)
			next.ServeHTTP(w, r.WithContext(ctx))
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
