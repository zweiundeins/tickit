package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/projects"
	"github.com/mbolli/tickit/internal/sharedhttp"
	"github.com/mbolli/tickit/internal/ui/components"
	"github.com/mbolli/tickit/internal/users"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/starfederation/datastar-go/datastar"
)

// Example usage for issues updates:
type IssuesUpdatesSignals struct {
	Filter   issues.ListIssuesFilter `json:"filter"`
	OrderBy  string                  `json:"order_by"`
	OrderDir string                  `json:"order_dir"`
}

// GET /updates/issues - SSE endpoint for real-time issue updates (table)
func HandleIssuesUpdatesSSE(store sessions.Store, nc *nats.Conn, issueService *issues.IssueService, projectService *projects.ProjectService, userService *users.UserService) http.HandlerFunc {
	return HandleSSEUpdates(
		store,
		nc,
		[]string{"IssueCreated", "IssueUpdated", "IssueDeleted", "IssuePatched"},
		&IssuesUpdatesSignals{},
		func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string, signals any) {
			sig := signals.(*IssuesUpdatesSignals)
			query := sig.Filter.ToQuery()
			issuesList, err := issueService.ListIssues(ctx, query)
			if err == nil {
				// Collect unique project IDs
				projectIDs := make(map[string]struct{})
				userIDs := make(map[string]struct{})
				for _, issue := range issuesList {
					projectIDs[issue.ProjectID] = struct{}{}
					if issue.Reporter != "" {
						userIDs[issue.Reporter] = struct{}{}
					}
					if issue.Assignee != "" {
						userIDs[issue.Assignee] = struct{}{}
					}
				}
				// Fetch only needed projects
				projectMap := make(map[string]*projects.Project)
				for id := range projectIDs {
					if p, err := projectService.GetProjectByID(ctx, id); err == nil && p != nil {
						projectMap[id] = p
					}
				}
				// Fetch only needed users
				userMap := make(map[string]*components.UserInfo)
				for id := range userIDs {
					if u, err := userService.GetUserByID(ctx, id); err == nil && u != nil {
						userMap[id] = &components.UserInfo{
							ID:       u.ID,
							Username: u.Username,
						}
					}
				}
				_ = sse.PatchElementTempl(components.IssuesTable(issuesList, projectMap, userMap))
			}
		},
	)
}

// GET /updates/notifications - SSE endpoint for real-time notification updates only
func HandleNotificationsUpdatesSSE(store sessions.Store, nc *nats.Conn, notificationService *notifications.NotificationService) http.HandlerFunc {
	return HandleSSEUpdates(
		store,
		nc,
		[]string{"NotificationCreated", "NotificationDeleted", "NotificationRead"},
		nil,
		func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string, signals any) {
			ntfs, err := notificationService.GetNotifications(ctx, sessionID)
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
func HandleIssueUpdatesSSE(store sessions.Store, nc *nats.Conn, issueService *issues.IssueService, projectService *projects.ProjectService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		issueID := chi.URLParam(r, "id")
		handler := HandleSSEUpdates(
			store,
			nc,
			[]string{"IssueUpdated", "IssueDeleted", "IssuePatched"},
			nil,
			func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string, signals any) {
				issue, err := issueService.GetIssueByID(ctx, issueID)
				projectsList, _ := projectService.ListProjects(ctx)
				if err == nil && issue != nil {
					sse.PatchElementTempl(components.Issue(issue, projectsList))
				}
			},
		)
		handler(w, r)
	}
}

// Generic SSE handler for updates
func HandleSSEUpdates(
	store sessions.Store,
	nc *nats.Conn,
	eventTypes []string,
	signals any,
	updatePatch func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string, signals any),
	initialPatch ...func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string, signals any),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, signals, func(sessionID string, signals any) {
			sse := datastar.NewSSE(w, r, datastar.WithCompression(datastar.WithServerPriority()))
			ctx := r.Context()

			jsCtx, err := jetstream.New(nc)
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
				initialPatch[0](ctx, sse, sessionID, signals)
			} else {
				updatePatch(ctx, sse, sessionID, signals)
			}

			eventTypeSet := make(map[string]struct{})
			for _, t := range eventTypes {
				eventTypeSet[t] = struct{}{}
			}

			sub, err := nc.Subscribe("events", func(msg *nats.Msg) {
				var raw map[string]interface{}

				if err := json.Unmarshal(msg.Data, &raw); err != nil {
					_ = msg.Ack()
					return
				}

				eventType, _ := raw["event_type"].(string)
				if _, ok := eventTypeSet[eventType]; ok {
					updatePatch(ctx, sse, sessionID, signals)
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
		})
	}
}
