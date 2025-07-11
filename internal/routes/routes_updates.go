package routes

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/sharedhttp"
	"github.com/mbolli/tickit/internal/ui/components"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/starfederation/datastar/sdk/go/datastar"
)

// GET /updates/issues - SSE endpoint for real-time issue updates (table)
func HandleIssuesUpdatesSSE(store sessions.Store, nc *nats.Conn, issueService *issues.IssueService) http.HandlerFunc {
	return HandleSSEUpdates(
		store,
		nc,
		[]string{"IssueCreated", "IssueUpdated", "IssueDeleted"},
		func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string) {
			issuesList, err := issueService.ListIssues(ctx, nil, nil, nil)
			if err == nil {
				_ = sse.PatchElementTempl(components.IssuesTable(issuesList))
			}
		},
		func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string) {
			issuesList, err := issueService.ListIssues(ctx, nil, nil, nil)
			if err == nil {
				_ = sse.PatchElementTempl(components.IssuesTable(issuesList))
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
		func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string) {
			ntfs, err := notificationService.GetNotifications(ctx, sessionID)
			if err == nil {
				_ = sse.PatchElementTempl(components.Notifications(ntfs))
			}
		},
		func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string) {
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
func HandleIssueUpdatesSSE(store sessions.Store, nc *nats.Conn, issueService *issues.IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		issueID := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
		handler := HandleSSEUpdates(
			store,
			nc,
			[]string{"IssueUpdated", "IssueDeleted"},
			func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string) {
				issue, err := issueService.GetIssueByID(ctx, issueID)
				if err == nil && issue != nil {
					sse.PatchElementTempl(components.Issue(issue))
				}
			},
			func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string) {
				issue, err := issueService.GetIssueByID(ctx, issueID)
				if err == nil && issue != nil {
					sse.PatchElementTempl(components.Issue(issue))
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
	initialPatch func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string),
	updatePatch func(ctx context.Context, sse *datastar.ServerSentEventGenerator, sessionID string),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
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

			// send initial patch
			initialPatch(ctx, sse, sessionID)

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
					updatePatch(ctx, sse, sessionID)
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
