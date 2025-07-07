package bootstrap

import (
	"context"
	"fmt"
	"time"

	"github.com/delaneyj/toolbelt/embeddednats"
	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/cqrs/application"
	"github.com/mbolli/tickit/internal/cqrs/bus"
	"github.com/mbolli/tickit/internal/cqrs/commands"
	"github.com/mbolli/tickit/internal/cqrs/events"
	"github.com/mbolli/tickit/internal/cqrs/queries"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type CQRSDeps struct {
	NotificationService *application.NotificationService
	IssueService        *application.IssueService
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

	eventStore, err := events.NewEventStore(ctx, "data/events.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create event store: %w", err)
	}
	notificationProjectionStore, err := events.NewNotificationProjectionStore(ctx, "data/notifications.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create notification projection store: %w", err)
	}
	issueProjectionStore, err := events.NewIssueProjectionStore(ctx, "data/issues.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create issue projection store: %w", err)
	}
	eventHandlers := events.CreateDefaultEventHandlers()
	eventHandlers["*"] = events.CreateEventStoreHandler(eventStore)
	eventBus := events.NewEventBusWrapper(js)
	commandHandler := commands.NewCommandHandler(eventStore, notificationProjectionStore, eventBus)
	queryHandler := queries.NewQueryHandler(notificationProjectionStore)
	issueCommandHandler := commands.NewIssueCommandHandler(eventBus, eventStore, issueProjectionStore)
	issueQueryHandler := queries.NewIssueQueryHandler(issueProjectionStore)
	commandBus := bus.NewCommandBus()
	queryBus := bus.NewQueryBus()
	// Register command handlers
	commandBus.RegisterHandler("CreateNotification", bus.NewCreateNotificationHandler(commandHandler))
	commandBus.RegisterHandler("DeleteNotification", bus.NewDeleteNotificationHandler(commandHandler))
	commandBus.RegisterHandler("MarkNotificationRead", bus.NewMarkNotificationReadHandler(commandHandler))
	commandBus.RegisterHandler("CreateIssue", bus.NewIssueCreateHandler(issueCommandHandler))
	commandBus.RegisterHandler("UpdateIssue", bus.NewIssueUpdateHandler(issueCommandHandler))
	commandBus.RegisterHandler("DeleteIssue", bus.NewIssueDeleteHandler(issueCommandHandler))
	// Register query handlers
	queryBus.RegisterHandler("GetNotifications", bus.NewGetNotificationsHandler(queryHandler))
	queryBus.RegisterHandler("GetUnreadCount", bus.NewGetUnreadCountHandler(queryHandler))
	queryBus.RegisterHandler("GetNotificationByUID", bus.NewGetNotificationByUIDHandler(queryHandler))
	queryBus.RegisterHandler("GetIssueByID", bus.NewIssueGetByIDHandler(issueQueryHandler))
	queryBus.RegisterHandler("ListIssues", bus.NewIssueListHandler(issueQueryHandler))
	notificationService := application.NewNotificationService(commandBus, queryBus)
	issueService := application.NewIssueService(commandBus, queryBus)
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
