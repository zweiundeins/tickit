package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/realtime"
	"github.com/mbolli/tickit/internal/sharedhttp"
	"github.com/nats-io/nats.go"
	"github.com/starfederation/datastar-go/datastar"
)

// HandleSignalsUpdate updates subscription context based on incoming signals
func HandleSignalsUpdate(deps *bootstrap.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Get connection ID from signals
		signals := ctx.Value("datastar_signals")
		if signals == nil {
			http.Error(w, "No signals found", http.StatusBadRequest)
			return
		}

		signalsMap, ok := signals.(map[string]interface{})
		if !ok {
			http.Error(w, "Invalid signals format", http.StatusBadRequest)
			return
		}

		connectionID, ok := signalsMap["connectionId"].(string)
		if !ok {
			http.Error(w, "No connectionId in signals", http.StatusBadRequest)
			return
		}

		// Get existing subscription context
		subCtx, exists := realtime.GetSubscriptionContextByConnectionID(connectionID)
		if !exists {
			logger.Debug().Str("connection_id", connectionID).Msg("No subscription context found, creating default context")

			// Create a default subscription context for pages that don't use SetEventPatterns()
			subCtx = &realtime.SubscriptionContext{
				Filter:        make(map[string]interface{}),
				EventPatterns: []string{}, // Default: no patterns
			}
			realtime.StoreSubscriptionContext(connectionID, subCtx)
		} else {
			logger.Debug().
				Str("connection_id", connectionID).
				Strs("event_patterns", subCtx.EventPatterns).
				Interface("filter", subCtx.Filter).
				Msg("Found existing subscription context")
		}

		// Marshall signals into subscription context filter
		originalFilter := make(map[string]interface{})
		for k, v := range subCtx.Filter {
			originalFilter[k] = v
		}

		// Safely merge filter data instead of replacing entirely
		if filter, ok := signalsMap["filter"]; ok && filter != nil {
			if filterMap, ok := filter.(map[string]interface{}); ok {
				if subCtx.Filter == nil {
					subCtx.Filter = make(map[string]interface{})
				}
				// Merge filter data, preserving existing pagination etc.
				for k, v := range filterMap {
					subCtx.Filter[k] = v
				}
			}
		}

		// Note: Notification patterns are now handled automatically via registry

		logger.Debug().
			Str("connection_id", connectionID).
			Strs("event_patterns", subCtx.EventPatterns).
			Interface("filter", subCtx.Filter).
			Msg("Updated subscription context")

		// Store the updated subscription context
		realtime.StoreSubscriptionContext(connectionID, subCtx)

		// Check if filter change requires moving to a different group
		if len(subCtx.EventPatterns) > 0 {
			// Generate new group key with updated filter
			newGroupKey := realtime.GenerateGroupKey(subCtx.Filter, subCtx.EventPatterns)
			currentGroup := deps.SessionGroupManager.GetGroupByConnectionID(connectionID)

			// Derive entity type from first event pattern
			entityType := "default"
			if len(subCtx.EventPatterns) > 0 {
				parts := strings.Split(subCtx.EventPatterns[0], ".")
				if len(parts) > 0 {
					entityType = parts[0]
				}
			}

			// If group key changed, move session to new group
			if currentGroup != nil && currentGroup.ID != newGroupKey {
				logger.Info().Msgf("HandleSignalsUpdate: Filter change detected for connection %s, moving from group %s to %s",
					connectionID, currentGroup.ID, newGroupKey)

				// Get session details before removing
				var session *realtime.Session
				session = currentGroup.GetSession(connectionID)

				if session != nil {
					logger.Info().Msgf("HandleSignalsUpdate: Found existing session for connection %s, moving to new group", connectionID)
					// Remove from current group (but don't close channel or cancel context)
					deps.SessionGroupManager.RemoveSessionFromGroup(currentGroup, connectionID, false)
					logger.Info().Msgf("HandleSignalsUpdate: Removed connection %s from group %s (cleanup=false)", connectionID, currentGroup.ID)

					// Add to new group with updated filter (reuse existing session context and channel)
					logger.Info().Msgf("HandleSignalsUpdate: Adding connection %s to new group with key %s", connectionID, newGroupKey)
					newGroup, _ := deps.SessionGroupManager.AddSessionToGroup(
						connectionID,
						session.SessionID,
						session.UserID,
						subCtx.Filter,
						subCtx.EventPatterns,
						session,
					)

					if newGroup != nil {
						logger.Info().Msgf("HandleSignalsUpdate: Successfully moved connection %s to new group %s", connectionID, newGroup.ID)

						// Send update with new filter to trigger re-rendering
						uiEvent := &realtime.UIUpdateEvent{
							EventType:  "filter.updated", // currently not used at all
							EntityType: entityType,
							EntityID:   "",
							Timestamp:  time.Now(),
							Data:       signalsMap,
						}

						err := newGroup.SendUpdateToAllSessions(ctx, uiEvent)
						if err != nil {
							logger.Info().Msgf("HandleSignalsUpdate: Failed to send update to new group: %v", err)
						}
					}
				}
			} else if currentGroup != nil {
				// Same group, just trigger update with new filter data
				uiEvent := &realtime.UIUpdateEvent{
					EventType:  "filter.updated",
					EntityType: entityType,
					EntityID:   "",
					Timestamp:  time.Now(),
					Data:       signalsMap,
				}

				err := currentGroup.SendUpdateToAllSessions(ctx, uiEvent)
				if err != nil {
					logger.Info().Msgf("HandleSignalsUpdate: Failed to send update to group: %v", err)
					http.Error(w, "Failed to trigger update", http.StatusInternalServerError)
					return
				}

				logger.Info().Msgf("HandleSignalsUpdate: Triggered update for connection %s in same group", connectionID)
			} else {
				logger.Info().Msgf("HandleSignalsUpdate: No session group found for connection %s", connectionID)
			}
		} else {
			logger.Info().Msgf("HandleSignalsUpdate: Skipping session group update for default context (connection %s)", connectionID)
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}
}

// HandleUpdatesSSE Generic /updates endpoint that works with any page
// The middleware will determine the subscription context
func HandleUpdatesSSE(deps *bootstrap.Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		connectionID, ok := ctx.Value("connection_id").(string)
		if !ok {
			logger.Info().Msgf("HandleUpdatesSSE: connection_id not found in context")
			http.Error(w, "Connection ID not found", http.StatusInternalServerError)
			return
		}
		sessionID, ok := ctx.Value("session_id").(string)
		if !ok {
			logger.Info().Msgf("HandleUpdatesSSE: session_id not found in context")
			http.Error(w, "Session ID not found", http.StatusInternalServerError)
			return
		}

		// Get subscription context by connection ID (stored by page middleware)
		subCtx, ok := realtime.GetSubscriptionContextByConnectionID(connectionID)
		if !ok {
			logger.Info().Msgf("HandleUpdatesSSE: Subscription context not found for connection ID: %s", connectionID)
			http.Error(w, "Subscription context not found", http.StatusNoContent)
			return
		}

		// Generate unique connection ID for this SSE connection
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithServerPriority()))

		currentUser, _ := sharedhttp.GetUserIDFromContext(ctx)
		logger.Info().Msgf("HandleUpdatesSSE: Current user ID: %s", currentUser)

		// Add session to appropriate group (now returns update channel)
		logger.Info().Msgf("HandleUpdatesSSE: Adding session to group with EventPatterns=%v, Filter=%v",
			subCtx.EventPatterns, subCtx.Filter)

		group, updateCh := deps.SessionGroupManager.AddSession(
			connectionID, // Unique connection ID
			sessionID,    // Browser session ID
			currentUser,  // User ID
			subCtx.Filter,
			subCtx.EventPatterns,
			nil, // Registry used instead
		)

		if group == nil {
			logger.Info().Msgf("AddSession returned nil group for connection %s", connectionID)
			http.Error(w, "Failed to create or add session to group", http.StatusInternalServerError)
			return
		}

		logger.Info().Msgf("%s: Connection %s (session %s) added to group %s (patterns: %v, renderer: %v)",
			r.URL, connectionID, sessionID, group.ID, group.EventPatterns, len(group.GetRenderers()) > 0)

		// Send initial update directly to this session
		sendGroupUpdate(ctx, sse, group)

		// Subscribe to pattern-based events (now the default with registry system)
		var natsSubscription *nats.Subscription
		logger.Info().Msgf("HandleUpdatesSSE: Subscribing to event patterns: %v", subCtx.EventPatterns)

		sub, err := deps.NatsConn.Subscribe("events", func(msg *nats.Msg) {
			var raw map[string]interface{}
			if err := json.Unmarshal(msg.Data, &raw); err != nil {
				logger.Error().Err(err).Msg("Failed to unmarshal NATS event")
				_ = msg.Ack()
				return
			}

			eventType, _ := raw["event_type"].(string)
			if realtime.MatchesEventPattern(eventType, subCtx.EventPatterns) {
				// For notification events, check if they're for this session/user
				if strings.HasPrefix(eventType, "notifications.") {
					// Check if this notification event is for this session or user
					eventSessionID, _ := raw["data"].(map[string]interface{})["session_id"].(string)
					eventUserID, _ := raw["data"].(map[string]interface{})["user_id"].(string)

					// Only process if the notification is for this session or user
					if eventSessionID != sessionID && eventUserID != currentUser {
						_ = msg.Ack()
						return
					}
				}

				logger.Debug().
					Str("event_type", eventType).
					Str("connection_id", connectionID).
					Msg("Pattern-based event matched, triggering update")
			}
			// Use registry to get appropriate renderer for the event type
			renderer := realtime.GetRenderer(eventType)

			if renderer != nil {
				// Prepare render data based on event type
				var renderData interface{}
				if strings.HasPrefix(eventType, "notifications.") {
					// Notification renderer expects the raw event data
					renderData = raw
				} else {
					// Other renderers expect GroupUpdateData format
					// Derive entity type and ID from event patterns
					var entityType string
					var entityID *string
					if len(subCtx.EventPatterns) > 0 {
						parts := strings.Split(subCtx.EventPatterns[0], ".")
						if len(parts) > 0 {
							entityType = parts[0]
						}
						if len(parts) > 1 && parts[1] != "*" {
							entityID = &parts[1]
						}
					}

					renderData = &realtime.GroupUpdateData{
						EntityType: entityType,
						EntityID:   entityID,
						Filter:     subCtx.Filter,
						EventData:  raw,
					}
				}

				if renderer != nil {
					// Create a context with session information for rendering
					renderCtx := context.WithValue(ctx, "session_id", sessionID)
					renderCtx = context.WithValue(renderCtx, "user_id", currentUser)

					// Safely call the renderer with error recovery
					func() {
						defer func() {
							if r := recover(); r != nil {
								logger.Error().
									Interface("panic", r).
									Str("event_type", eventType).
									Str("connection_id", connectionID).
									Msg("Pattern-based renderer panic recovered")
							}
						}()

						component, attrs, renderErr := renderer.RenderUpdate(renderCtx, renderData)
						if renderErr != nil {
							logger.Error().Err(renderErr).
								Str("event_type", eventType).
								Str("renderer_type", renderer.GetEntityType()).
								Msg("Failed to render pattern-based update")
						} else if component != nil {
							// Send any attributes/signals first
							if attrs != nil && len(attrs) > 0 {
								jsonData, jsonErr := json.Marshal(attrs)
								if jsonErr == nil {
									_ = sse.PatchSignals(jsonData)
								}
							}
							// Send the component update
							err := sse.PatchElementTempl(component)
							if err != nil {
								logger.Error().Err(err).Msg("Failed to send pattern-based component update")
							}
						}
					}()
				} // End of renderer != nil
			} // End of renderer logic
			_ = msg.Ack()
		})

		if err != nil {
			logger.Error().Err(err).Msg("Failed to subscribe to pattern-based events")
		} else {
			natsSubscription = sub
		}

		// Listen for updates on the channel and send them via SSE
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case uiEvent, ok := <-updateCh:
					if !ok {
						return // Channel closed
					}

					logger.Info().Msgf("HandleUpdatesSSE/FOR: Received update event for connection %s: %+v", connectionID, uiEvent)

					// Get current group dynamically (in case session was moved)
					currentGroup := deps.SessionGroupManager.GetGroupByConnectionID(connectionID)
					if currentGroup != nil {
						sendGroupUpdate(ctx, sse, currentGroup)
					} else {
						logger.Info().Msgf("HandleUpdatesSSE/FOR: No current group found for connection %s", connectionID)
					}
				}
			}
		}()

		// Wait for the connection to close
		<-ctx.Done()

		// Clean up
		if natsSubscription != nil {
			natsSubscription.Unsubscribe()
		}
		deps.SessionGroupManager.RemoveSession(connectionID)
		realtime.RemoveSubscriptionContext(connectionID)
	}
}

// convertMapToIssuesFilter converts a generic filter map to a strongly typed IssuesFilter
func convertMapToIssuesFilter(filterMap map[string]interface{}) issues.ListIssuesFilter {
	filter := issues.ListIssuesFilter{}

	if projectID, ok := filterMap["project_id"].(string); ok && projectID != "" {
		filter.ProjectID = projectID
	}
	if status, ok := filterMap["status"].(string); ok && status != "" {
		filter.Status = status
	}
	if priority, ok := filterMap["priority"].(string); ok && priority != "" {
		filter.Priority = priority
	}
	if assignee, ok := filterMap["assignee_id"].(string); ok && assignee != "" {
		filter.Assignee = assignee
	}
	if containsText, ok := filterMap["contains_text"].(string); ok && containsText != "" {
		filter.ContainsText = containsText
	}
	if reporter, ok := filterMap["reporter"].(string); ok && reporter != "" {
		filter.Reporter = reporter
	}
	if createdAfter, ok := filterMap["created_after"].(string); ok && createdAfter != "" {
		filter.CreatedAfter = createdAfter
	}
	if createdBefore, ok := filterMap["created_before"].(string); ok && createdBefore != "" {
		filter.CreatedBefore = createdBefore
	}

	return filter
}

// sendGroupUpdate sends an update to all sessions in a group
func sendGroupUpdate(ctx context.Context, sse *datastar.ServerSentEventGenerator, group *realtime.SessionGroup) {
	if group == nil {
		logger.Info().Msgf("sendGroupUpdate: Cannot send group update: group is nil")
		return
	}

	// Get renderers from registry based on group patterns
	renderers := group.GetRenderers()
	if len(renderers) == 0 {
		logger.Info().Msgf("sendGroupUpdate: No renderers found for group %s patterns: %v", group.ID, group.EventPatterns)
		return
	}

	logger.Debug().
		Str("group_id", group.ID).
		Strs("event_patterns", group.EventPatterns).
		Int("renderer_count", len(renderers)).
		Msg("sendGroupUpdate: Preparing render data")

	// Extract entity type from event patterns for backward compatibility with renderers
	entityType := "unknown"
	var entityID *string
	if len(group.EventPatterns) > 0 {
		firstPattern := group.EventPatterns[0]
		if strings.HasPrefix(firstPattern, "issues.") && firstPattern != "issues.*" {
			entityType = "issues"
			// Extract entity ID from pattern like "issues.123.*"
			parts := strings.Split(firstPattern, ".")
			if len(parts) >= 2 && parts[1] != "*" {
				entityID = &parts[1]
			}
		} else if strings.HasPrefix(firstPattern, "issues.") {
			entityType = "issues"
		} else if strings.HasPrefix(firstPattern, "projects.") && firstPattern != "projects.*" {
			entityType = "projects"
			parts := strings.Split(firstPattern, ".")
			if len(parts) >= 2 && parts[1] != "*" {
				entityID = &parts[1]
			}
		} else if strings.HasPrefix(firstPattern, "projects.") {
			entityType = "projects"
		} else if strings.HasPrefix(firstPattern, "notifications.") {
			entityType = "notifications"
		}
	}

	// Send updates using all matching renderers from registry
	for pattern, renderer := range renderers {
		logger.Debug().
			Str("pattern", pattern).
			Str("renderer_type", renderer.GetEntityType()).
			Str("group_id", group.ID).
			Strs("group_patterns", group.EventPatterns).
			Msg("sendGroupUpdate: Processing renderer for pattern")

		// Skip renderer if it doesn't match the entity type to prevent type mismatches
		rendererEntityType := renderer.GetEntityType()
		if entityType != "unknown" && rendererEntityType != entityType {
			logger.Debug().
				Str("pattern", pattern).
				Str("renderer_type", rendererEntityType).
				Str("entity_type", entityType).
				Msg("sendGroupUpdate: Skipping renderer - entity type mismatch")
			continue
		}

		// Determine data type based on specific renderer type using type assertion
		var renderData interface{}
		switch renderer := renderer.(type) {
		case *realtime.IssuesTableRenderer:
			// IssuesTableRenderer expects UnifiedIssuesSignals
			renderData = &realtime.UnifiedIssuesSignals{
				EntityType: entityType,
				EntityID:   entityID,
				Filter:     convertMapToIssuesFilter(group.Filter),
			}
			logger.Debug().Msg("sendGroupUpdate: Using UnifiedIssuesSignals for IssuesTableRenderer")
		case *realtime.IssueDetailRenderer:
			// IssueDetailRenderer expects GroupUpdateData
			renderData = &realtime.GroupUpdateData{
				EntityType: entityType,
				EntityID:   entityID,
				Filter:     group.Filter,
			}
			logger.Debug().Msg("sendGroupUpdate: Using GroupUpdateData for IssueDetailRenderer")
		case *realtime.NotificationRenderer:
			// NotificationRenderer expects GroupUpdateData
			renderData = &realtime.GroupUpdateData{
				EntityType: entityType,
				EntityID:   entityID,
				Filter:     group.Filter,
			}
			logger.Debug().Msg("sendGroupUpdate: Using GroupUpdateData for NotificationRenderer")
		default:
			// Use generic data for other renderer types
			renderData = &realtime.GroupUpdateData{
				EntityType: entityType,
				EntityID:   entityID,
				Filter:     group.Filter,
			}
			logger.Debug().
				Str("renderer_type", fmt.Sprintf("%T", renderer)).
				Msg("sendGroupUpdate: Using generic GroupUpdateData for unknown renderer")
		}

		// Render the update using this renderer
		logger.Debug().
			Str("group_id", group.ID).
			Str("renderer_type", renderer.GetEntityType()).
			Msg("sendGroupUpdate: About to call renderer")

		var component datastar.TemplComponent
		var signals map[string]any
		var err error

		// Safely call the renderer with panic recovery
		func() {
			defer func() {
				if r := recover(); r != nil {
					logger.Error().
						Interface("panic", r).
						Str("group_id", group.ID).
						Str("renderer_type", renderer.GetEntityType()).
						Msg("Group renderer panic recovered")
					err = fmt.Errorf("renderer panic: %v", r)
				}
			}()

			component, signals, err = renderer.RenderUpdate(ctx, renderData)
		}()

		if err != nil {
			logger.Error().
				Err(err).
				Str("group_id", group.ID).
				Str("entity_type", entityType).
				Str("renderer_type", renderer.GetEntityType()).
				Interface("render_data_type", renderData).
				Msgf("sendGroupUpdate: Failed to render update for group %s", group.ID)
			continue // Try next renderer
		}

		// If we have a specific SSE connection, send to it
		if sse != nil {
			// Send the signals first if any
			if signals != nil && len(signals) > 0 {
				jsonData, err := json.Marshal(signals)
				if err == nil {
					_ = sse.PatchSignals(jsonData)
				}
			}

			err := sse.PatchElementTempl(component)
			if err != nil {
				logger.Info().Msgf("sendGroupUpdate: Error sending SSE patch for group %s pattern %s: %v", group.ID, pattern, err)
				// This error might cause the SSE connection to close!
				return
			}
			return
		}

		// Otherwise, send to all active sessions in the group todo is this needed?
		sessions := group.GetActiveSessions()
		logger.Info().Msgf("sendGroupUpdate: Would send update to %d sessions in group %s pattern %s", len(sessions), group.ID, pattern)
		for _, session := range sessions {
			// In a real implementation, you'd need to maintain SSE connections per session
			// For now, this is a placeholder showing the structure
			logger.Info().Msgf("sendGroupUpdate: Would send update to session %s in group %s pattern %s", session.ID, group.ID, pattern)
		}
	} // End of renderer loop
}

// Example usage for issues updates:
type IssuesUpdatesSignals struct {
	Filter   issues.ListIssuesFilter `json:"filter"`
	OrderBy  string                  `json:"order_by"`
	OrderDir string                  `json:"order_dir"`
}

// GET /updates/status - SSE endpoint for API online status (ping)
func HandleStatusUpdatesSSE() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithServerPriority()))
		ctx := r.Context()
		ticker := time.NewTicker(10 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				payload, _ := json.Marshal(map[string]any{"_online": true})
				_ = sse.PatchSignals(payload)
			}
		}
	}
}
