package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/mbolli/tickit/internal/bootstrap"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/realtime"
	"github.com/mbolli/tickit/web/ui/components"

	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/sharedhttp"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
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

			// Create a default subscription context for pages that don't have specific middleware
			subCtx = &realtime.SubscriptionContext{
				EntityType: "default",
				EntityID:   nil,
				Filter:     make(map[string]interface{}),
				Renderer:   nil,
			}
			realtime.StoreSubscriptionContext(connectionID, subCtx)
		} else {
			entityIDStr := ""
			if subCtx.EntityID != nil {
				entityIDStr = *subCtx.EntityID
			}
			logger.Debug().
				Str("connection_id", connectionID).
				Str("entity_type", subCtx.EntityType).
				Str("entity_id", entityIDStr).
				Interface("filter", subCtx.Filter).
				Msg("Found existing subscription context")
		}

		// Marshall signals into subscription context filter
		originalFilter := make(map[string]interface{})
		for k, v := range subCtx.Filter {
			originalFilter[k] = v
		}

		// Safely update EntityID if provided
		if entityId, ok := signalsMap["entityId"]; ok && entityId != nil {
			if entityIdStr, ok := entityId.(string); ok {
				subCtx.EntityID = &entityIdStr
			}
		}

		// Safely update EntityType if provided
		if entityType, ok := signalsMap["entityType"]; ok && entityType != nil {
			if entityTypeStr, ok := entityType.(string); ok {
				subCtx.EntityType = entityTypeStr
			}
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

		entityIDStr := ""
		if subCtx.EntityID != nil {
			entityIDStr = *subCtx.EntityID
		}
		logger.Debug().
			Str("connection_id", connectionID).
			Str("entity_type", subCtx.EntityType).
			Str("entity_id", entityIDStr).
			Interface("filter", subCtx.Filter).
			Msg("Updated subscription context")

		// Store the updated subscription context
		realtime.StoreSubscriptionContext(connectionID, subCtx)

		// Check if filter change requires moving to a different group
		if subCtx.EntityType != "default" {
			// Generate new group key with updated filter
			newGroupKey := realtime.GenerateGroupKey(subCtx.EntityType, subCtx.EntityID, subCtx.Filter)
			currentGroup := deps.SessionGroupManager.GetGroupByConnectionID(connectionID)

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
						subCtx.EntityType,
						subCtx.EntityID,
						subCtx.Filter,
						subCtx.Renderer,
						session,
					)

					if newGroup != nil {
						logger.Info().Msgf("HandleSignalsUpdate: Successfully moved connection %s to new group %s", connectionID, newGroup.ID)

						// Send update with new filter to trigger re-rendering
						uiEvent := &realtime.UIUpdateEvent{
							EventType:  "filter.updated", // currently not used at all
							EntityType: subCtx.EntityType,
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
					EntityType: subCtx.EntityType,
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
			http.Error(w, "Subscription context not found", http.StatusInternalServerError)
			return
		}

		// Generate unique connection ID for this SSE connection
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithServerPriority()))

		currentUser, _ := sharedhttp.GetUserIDFromContext(ctx)
		logger.Info().Msgf("HandleUpdatesSSE: Current user ID: %s", currentUser)

		// Add session to appropriate group (now returns update channel)
		logger.Info().Msgf("HandleUpdatesSSE: Adding session to group with EntityType=%s, EntityID=%v, Filter=%v",
			subCtx.EntityType, subCtx.EntityID, subCtx.Filter)

		group, updateCh := deps.SessionGroupManager.AddSession(
			connectionID, // Unique connection ID
			sessionID,    // Browser session ID
			currentUser,  // User ID
			subCtx.EntityType,
			subCtx.EntityID,
			subCtx.Filter,
			subCtx.Renderer,
		)

		if group == nil {
			logger.Info().Msgf("AddSession returned nil group for connection %s", connectionID)
			http.Error(w, "Failed to create or add session to group", http.StatusInternalServerError)
			return
		}

		entityIDStr := "nil"
		if group.EntityID != nil {
			entityIDStr = *group.EntityID
		}

		logger.Info().Msgf("%s: Connection %s (session %s) added to group %s (entity: %s/%s, renderer: %v)",
			r.URL, connectionID, sessionID, group.ID, group.EntityType, entityIDStr, group.Renderer != nil)

		// Send initial update directly to this session
		sendGroupUpdate(ctx, sse, group)

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

	if group.Renderer == nil {
		logger.Info().Msgf("sendGroupUpdate: Cannot send group update for group %s: renderer is nil", group.ID)
		return
	}

	// Prepare data for rendering
	var renderData interface{}

	if group.EntityType == "issues" {
		// Use strongly typed signals for issues
		renderData = &realtime.UnifiedIssuesSignals{
			EntityType: group.EntityType,
			EntityID:   group.EntityID,
			Filter:     convertMapToIssuesFilter(group.Filter),
		}
	} else {
		// Use generic data for other entity types
		renderData = &realtime.GroupUpdateData{
			EntityType: group.EntityType,
			EntityID:   group.EntityID,
			Filter:     group.Filter,
		}
	}

	// Render the update using the group's renderer
	component, signals, err := group.Renderer.RenderUpdate(ctx, renderData)
	if err != nil {
		logger.Info().Msgf("sendGroupUpdate: Failed to render update for group %s: %v", group.ID, err)
		// Don't return here - try to continue, the error might be recoverable
		return
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
			logger.Info().Msgf("sendGroupUpdate: Error sending SSE patch for group %s: %v", group.ID, err)
			// This error might cause the SSE connection to close!
			return
		}
		return
	}

	// Otherwise, send to all active sessions in the group todo is this needed?
	sessions := group.GetActiveSessions()
	logger.Info().Msgf("sendGroupUpdate: Would send update to %d sessions in group %s", len(sessions), group.ID)
	for _, session := range sessions {
		// In a real implementation, you'd need to maintain SSE connections per session
		// For now, this is a placeholder showing the structure
		logger.Info().Msgf("sendGroupUpdate: Would send update to session %s in group %s", session.ID, group.ID)
	}
}

// Example usage for issues updates:
type IssuesUpdatesSignals struct {
	Filter   issues.ListIssuesFilter `json:"filter"`
	OrderBy  string                  `json:"order_by"`
	OrderDir string                  `json:"order_dir"`
}

// GET /updates/issues - SSE endpoint for real-time issue updates (table)

// GET /updates/notifications - SSE endpoint for real-time notification updates only
func HandleNotificationsUpdatesSSE(deps *bootstrap.Deps) http.HandlerFunc {
	return HandleSSEUpdates(
		deps,
		[]string{"NotificationCreated", "NotificationDeleted", "NotificationRead"},
		nil,
		func(r *http.Request, sse *datastar.ServerSentEventGenerator, sessionID string, signals any) {
			ntfs, err := deps.NotificationService.GetNotifications(r.Context(), sessionID)
			if err == nil {
				_ = sse.PatchElementTempl(components.Notifications(ntfs))
			}
		},
	)
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

// GET /updates/issue/{id} - SSE endpoint for real-time updates for a single issue

// Generic SSE handler for updates
func HandleSSEUpdates(
	deps *bootstrap.Deps,
	eventTypes []string,
	updatePatch func(r *http.Request, sse *datastar.ServerSentEventGenerator, sessionID string, signals any),
	initialPatch ...func(r *http.Request, sse *datastar.ServerSentEventGenerator, sessionID string, signals any)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		signals := ctx.Value("datastar_signals")
		sessionID := ctx.Value("session_id").(string)
		sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithServerPriority()))

		jsCtx, err := jetstream.New(deps.NatsConn)
		if err != nil {
			http.Error(w, "Failed to get JetStream context", http.StatusInternalServerError)
			return
		}

		_, err = jsCtx.CreateOrUpdateConsumer(ctx, "events", jetstream.ConsumerConfig{
			Durable:       "updates_sse_" + sessionID,
			FilterSubject: "events",
			AckPolicy:     jetstream.AckExplicitPolicy,
		})
		if err != nil {
			http.Error(w, "Failed to create or get events consumer", http.StatusInternalServerError)
			return
		}

		// send initial patch (use initialPatch if provided, else updatePatch)
		if len(initialPatch) > 0 && initialPatch[0] != nil {
			initialPatch[0](r, sse, sessionID, signals)
		} else {
			updatePatch(r, sse, sessionID, signals)
		}

		eventTypeSet := make(map[string]struct{})
		for _, t := range eventTypes {
			eventTypeSet[t] = struct{}{}
		}

		sub, err := deps.NatsConn.Subscribe("events", func(msg *nats.Msg) {
			var raw map[string]interface{}

			if err := json.Unmarshal(msg.Data, &raw); err != nil {
				_ = msg.Ack()
				return
			}

			eventType, _ := raw["event_type"].(string)
			if _, ok := eventTypeSet[eventType]; ok {
				updatePatch(r, sse, sessionID, signals)
			}
			_ = msg.Ack()
		})
		if err != nil {
			http.Error(w, "Failed to subscribe to events deliver subject", http.StatusInternalServerError)
			return
		}
		defer sub.Unsubscribe()

		<-ctx.Done()
		_ = sub.Unsubscribe()
	}
}
