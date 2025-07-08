package routes

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/sharedhttp"
	"github.com/mbolli/tickit/internal/ui/components"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/starfederation/datastar/sdk/go/datastar"
)

// GET /api/updates - SSE endpoint for real-time notifications and issues updates
func handleUpdatesSSE(store sessions.Store, nc *nats.Conn, notificationService *notifications.NotificationService, issueService *issues.IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			sse := datastar.NewSSE(w, r)
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

			// send initial elements
			ntfs, err := notificationService.GetNotifications(ctx, sessionID)
			if err == nil {
				_ = sse.PatchElementTempl(components.Notifications(ntfs))
			}
			issues, err := issueService.ListIssues(ctx, nil, nil, nil)
			if err == nil {
				_ = sse.PatchElementTempl(components.Issues(issues))
			}

			sub, err := nc.Subscribe("events", func(msg *nats.Msg) {
				var raw map[string]interface{}

				if err := json.Unmarshal(msg.Data, &raw); err != nil {
					_ = msg.Ack()
					return
				}

				log.Printf("raw event: %+v", raw)
				eventType, _ := raw["event_type"].(string)
				if payload, ok := raw["payload"].(map[string]interface{}); ok {
					if sid, _ := payload["session_id"].(string); sid == sessionID && (eventType == "NotificationCreated" || eventType == "NotificationDeleted" || eventType == "NotificationRead") {
						ntfs, err := notificationService.GetNotifications(ctx, sessionID)
						if err == nil {
							_ = sse.PatchElementTempl(components.Notifications(ntfs))
						}
					}
				}
				if eventType == "IssueCreated" || eventType == "IssueUpdated" || eventType == "IssueDeleted" {
					issues, err := issueService.ListIssues(ctx, nil, nil, nil)
					if err == nil {
						_ = sse.PatchElementTempl(components.Issues(issues))
					}
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
