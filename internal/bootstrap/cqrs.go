package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/delaneyj/toolbelt/embeddednats"
	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type CQRSDeps struct {
	NotificationService *notifications.NotificationService
	IssueService        *issues.IssueService
	SessionStore        sessions.Store
	JetStream           jetstream.JetStream
	NatsConn            *nats.Conn
}

func WireCQRS(ctx context.Context, ns *embeddednats.Server) (*CQRSDeps, error) {
	nc, err := ns.Client()
	if err != nil {
		return nil, fmt.Errorf("error creating nats client: %w", err)
	}
	js, err := jetstream.New(nc)
	if err != nil {
		return nil, fmt.Errorf("error creating jetstream client: %w", err)
	}

	// Create or update the unified 'events' stream for all real-time updates
	_, err = js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        "events",
		Subjects:    []string{"events"},
		Storage:     jetstream.MemoryStorage,
		Retention:   jetstream.LimitsPolicy,
		MaxBytes:    16 * 1024 * 1024, // 16MB, adjust as needed
		Description: "Unified event stream for notifications, issues, and more",
	})
	if err != nil {
		return nil, fmt.Errorf("error creating unified events stream: %w", err)
	}

	eventStore, err := shared.NewEventStore(ctx, "data/events.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create event store: %w", err)
	}
	notificationProjectionStore, err := notifications.NewNotificationProjectionStore(ctx, "data/notifications.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create notification projection store: %w", err)
	}
	issueProjectionStore, err := issues.NewIssueProjectionStore(ctx, "data/issues.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create issue projection store: %w", err)
	}
	eventHandlers := shared.CreateDefaultEventHandlers()
	eventHandlers["*"] = shared.CreateEventStoreHandler(eventStore)
	eventBus := shared.NewEventBusWrapper(js)
	notifCommandHandler := notifications.NewCommandHandler(eventStore, notificationProjectionStore, eventBus)
	notifQueryHandler := notifications.NewQueryHandler(notificationProjectionStore)
	issueCommandHandler := issues.NewIssueCommandHandler(eventBus, eventStore, issueProjectionStore)
	issueQueryHandler := issues.NewIssueQueryHandler(issueProjectionStore)
	commandBus := shared.NewCommandBus()
	queryBus := shared.NewQueryBus()
	// Register command handlers
	commandBus.RegisterHandler("CreateNotification", notifications.NewCreateNotificationHandler(notifCommandHandler))
	commandBus.RegisterHandler("DeleteNotification", notifications.NewDeleteNotificationHandler(notifCommandHandler))
	commandBus.RegisterHandler("MarkNotificationRead", notifications.NewMarkNotificationReadHandler(notifCommandHandler))
	commandBus.RegisterHandler("CreateIssue", issues.NewCreateIssueHandler(issueCommandHandler))
	commandBus.RegisterHandler("UpdateIssue", issues.NewUpdateIssueHandler(issueCommandHandler))
	commandBus.RegisterHandler("DeleteIssue", issues.NewDeleteIssueHandler(issueCommandHandler))
	// Register query handlers
	queryBus.RegisterHandler("GetNotifications", notifications.NewGetNotificationsHandler(notifQueryHandler))
	queryBus.RegisterHandler("GetUnreadCount", notifications.NewGetUnreadCountHandler(notifQueryHandler))
	queryBus.RegisterHandler("GetNotificationByUID", notifications.NewGetNotificationByUIDHandler(notifQueryHandler))
	queryBus.RegisterHandler("GetIssueByID", issues.NewIssueGetByIDHandler(issueQueryHandler))
	queryBus.RegisterHandler("ListIssues", issues.NewIssueListHandler(issueQueryHandler))
	notificationService := notifications.NewNotificationService(commandBus, queryBus)
	issueService := issues.NewIssueService(commandBus, queryBus)
	sessionStore := sessions.NewCookieStore([]byte("session-secret"))
	sessionStore.MaxAge(int(24 * time.Hour / time.Second))
	return &CQRSDeps{
		NotificationService: notificationService,
		IssueService:        issueService,
		SessionStore:        sessionStore,
		JetStream:           js,
		NatsConn:            nc,
	}, nil
}
