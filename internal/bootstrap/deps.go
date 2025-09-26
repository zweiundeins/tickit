package bootstrap

import (
	"context"
	"fmt"

	"github.com/delaneyj/toolbelt/embeddednats"
	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/projects"
	"github.com/mbolli/tickit/internal/realtime"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/mbolli/tickit/internal/users"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Deps struct {
	NotificationService *notifications.NotificationService
	IssueService        *issues.IssueService
	ProjectService      *projects.ProjectService
	UserService         *users.UserService
	UserRoutes          *users.UserRoutes
	SessionStore        sessions.Store
	SessionGroupManager *realtime.GroupManager
	EventProcessor      *realtime.EventProcessor
	CommandBus          *shared.CommandBus
	QueryBus            *shared.QueryBus
	JetStream           jetstream.JetStream
	NatsConn            *nats.Conn
}

func WireDeps(ctx context.Context, ns *embeddednats.Server, sessionStore sessions.Store) (*Deps, error) {
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
	projectProjectionStore, err := projects.NewProjectProjectionStore(ctx, "data/projects.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create project projection store: %w", err)
	}
	userStore, err := users.NewUserStore(ctx, "data/users.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create user store: %w", err)
	}
	// Create user projection store first (needed for event handlers)
	// Note: Using the same database as the main user store for simplified architecture
	userProjectionStore, err := users.NewUserProjectionStore(ctx, "data/users.db")
	if err != nil {
		return nil, fmt.Errorf("failed to create user projection store: %w", err)
	}

	// Create base event handlers with event store handler for persistence
	eventHandlers := shared.EventHandlers{
		"*": shared.CreateEventStoreHandler(eventStore),
	}

	// Create event bus with base handlers first
	eventBusWithHandlers := shared.NewEventBusWithHandlers(eventHandlers, js)

	// Register user projection event handlers
	userEventHandler := users.NewUserEventHandler(userProjectionStore, eventBusWithHandlers, userStore)
	eventBusWithHandlers.AddHandler("UserRegistered", userEventHandler.HandleUserRegistered)
	eventBusWithHandlers.AddHandler("UserEmailVerified", userEventHandler.HandleUserEmailVerified)
	eventBusWithHandlers.AddHandler("UserLoggedIn", userEventHandler.HandleUserLoggedIn)
	eventBusWithHandlers.AddHandler("UserLoggedOut", userEventHandler.HandleUserLoggedOut)
	eventBusWithHandlers.AddHandler("UserProfileUpdated", userEventHandler.HandleUserProfileUpdated)
	eventBusWithHandlers.AddHandler("UserAddedToProject", userEventHandler.HandleUserAddedToProject)
	eventBusWithHandlers.AddHandler("UserRemovedFromProject", userEventHandler.HandleUserRemovedFromProject)
	eventBusWithHandlers.AddHandler("UserRoleAssigned", userEventHandler.HandleUserRoleAssigned)
	eventBusWithHandlers.AddHandler("UserMFAEnabled", userEventHandler.HandleUserMFAEnabled)
	eventBusWithHandlers.AddHandler("UserMFADisabled", userEventHandler.HandleUserMFADisabled)
	eventBusWithHandlers.AddHandler("OAuthAccountLinked", userEventHandler.HandleOAuthAccountLinked)
	eventBusWithHandlers.AddHandler("OAuthAccountUnlinked", userEventHandler.HandleOAuthAccountUnlinked)

	// Register cross-module integration event handlers
	projectCreatedHandler := users.NewProjectCreatedEventHandler(userStore)
	issueCreatedHandler := users.NewIssueCreatedEventHandler(userStore)
	issueUpdatedHandler := users.NewIssueUpdatedEventHandler(userStore)

	eventBusWithHandlers.AddHandler("ProjectCreated", func(ctx context.Context, event shared.Event) error {
		return projectCreatedHandler.Handle(ctx, event)
	})
	eventBusWithHandlers.AddHandler("IssueCreated", func(ctx context.Context, event shared.Event) error {
		return issueCreatedHandler.Handle(ctx, event)
	})
	eventBusWithHandlers.AddHandler("IssueUpdated", func(ctx context.Context, event shared.Event) error {
		return issueUpdatedHandler.Handle(ctx, event)
	})

	// Register notification realtime event handlers for UI updates
	notificationRealtimeHandler := notifications.NewRealtimeEventHandler(nc)
	eventBusWithHandlers.AddHandler("NotificationCreated", func(ctx context.Context, event shared.Event) error {
		return notificationRealtimeHandler.Handle(ctx, event)
	})
	eventBusWithHandlers.AddHandler("NotificationDeleted", func(ctx context.Context, event shared.Event) error {
		return notificationRealtimeHandler.Handle(ctx, event)
	})
	eventBusWithHandlers.AddHandler("NotificationRead", func(ctx context.Context, event shared.Event) error {
		return notificationRealtimeHandler.Handle(ctx, event)
	})

	// Use the event bus with handlers as the main event bus
	eventBus := eventBusWithHandlers // Use the full EventBusWithHandlers instead of just the wrapper
	notifCommandHandler := notifications.NewCommandHandler(eventStore, notificationProjectionStore, eventBus)
	notifQueryHandler := notifications.NewQueryHandler(notificationProjectionStore)
	// Create user validator for issue assignments
	userValidatorImpl := users.NewIssueAssignmentValidator(userStore)
	userValidator := issues.NewIssueAssignmentValidatorAdapter(userValidatorImpl)
	issueCommandHandler := issues.NewIssueCommandHandlerWithValidator(eventBus, eventStore, issueProjectionStore, userValidator)
	issueQueryHandler := issues.NewIssueQueryHandler(issueProjectionStore)
	projectCommandHandler := projects.NewProjectCommandHandler(eventBus, eventStore, projectProjectionStore)
	projectQueryHandler := projects.NewProjectQueryHandler(projectProjectionStore)

	// User services
	passwordHasher := users.NewPasswordHasher()
	emailService := users.NewEmailServiceFromEnv()
	webauthnService := users.NewWebAuthnService(userStore)
	userCommandHandler := users.NewUserCommandHandler(eventBus, eventStore, userStore, passwordHasher, emailService, webauthnService, nil)

	// Create command and query buses
	commandBus := shared.NewCommandBus()
	queryBus := shared.NewQueryBus()
	// Register command handlers and factories in one call
	shared.RegisterCommandHandler(commandBus, "CreateNotification", notifCommandHandler.HandleCreateNotification)
	shared.RegisterCommandHandler(commandBus, "DeleteNotification", notifCommandHandler.HandleDeleteNotification)
	shared.RegisterCommandHandler(commandBus, "MarkNotificationRead", notifCommandHandler.HandleMarkNotificationRead)
	shared.RegisterCommandHandler(commandBus, "CreateIssue", issueCommandHandler.HandleCreateIssue)
	shared.RegisterCommandHandler(commandBus, "UpdateIssue", issueCommandHandler.HandleUpdateIssue)
	shared.RegisterCommandHandler(commandBus, "DeleteIssue", issueCommandHandler.HandleDeleteIssue)
	shared.RegisterCommandHandler(commandBus, "PatchIssue", issueCommandHandler.HandlePatchIssue)
	shared.RegisterCommandHandler(commandBus, "CreateProject", projectCommandHandler.HandleCreateProject)
	shared.RegisterCommandHandler(commandBus, "UpdateProject", projectCommandHandler.HandleUpdateProject)
	shared.RegisterCommandHandler(commandBus, "DeleteProject", projectCommandHandler.HandleDeleteProject)

	// User command handlers using new pattern
	shared.RegisterCommandHandler(commandBus, "RegisterUser", userCommandHandler.HandleRegisterUser)
	shared.RegisterCommandHandler(commandBus, "VerifyEmail", userCommandHandler.HandleVerifyEmail)
	shared.RegisterCommandHandler(commandBus, "LoginUser", userCommandHandler.HandleLoginUser)
	shared.RegisterCommandHandler(commandBus, "ForgotPassword", userCommandHandler.HandleForgotPassword)
	shared.RegisterCommandHandler(commandBus, "ResetPassword", userCommandHandler.HandleResetPassword)
	shared.RegisterCommandHandler(commandBus, "ChangePassword", userCommandHandler.HandleChangePassword)

	// Register query handlers using new pattern
	shared.RegisterQueryHandler(queryBus, "GetNotifications", func(ctx context.Context, query notifications.GetNotificationsQuery) (interface{}, error) {
		return notifQueryHandler.HandleGetNotifications(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "GetUnreadCount", func(ctx context.Context, query notifications.GetUnreadCountQuery) (interface{}, error) {
		return notifQueryHandler.HandleGetUnreadCount(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "GetNotificationByUID", func(ctx context.Context, query notifications.GetNotificationByUIDQuery) (interface{}, error) {
		return notifQueryHandler.HandleGetNotificationByUID(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "GetIssueByID", func(ctx context.Context, query issues.GetIssueByIDQuery) (interface{}, error) {
		return issueQueryHandler.HandleGetIssueByID(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "ListIssues", func(ctx context.Context, query issues.ListIssuesQuery) (interface{}, error) {
		return issueQueryHandler.HandleListIssues(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "GetIssueByKey", func(ctx context.Context, query issues.GetIssueByKeyQuery) (interface{}, error) {
		return issueQueryHandler.HandleGetIssueByKey(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "GetProjectByID", func(ctx context.Context, query projects.GetProjectByIDQuery) (interface{}, error) {
		return projectQueryHandler.HandleGetProjectByID(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "ListProjects", func(ctx context.Context, query projects.ListProjectsQuery) (interface{}, error) {
		return projectQueryHandler.HandleListProjects(ctx, query)
	})

	// User query handlers
	userQueryHandler := users.NewUserQueryHandler(userProjectionStore, userStore)
	shared.RegisterQueryHandler(queryBus, "GetUserByID", func(ctx context.Context, query users.GetUserByIDQuery) (interface{}, error) {
		return userQueryHandler.HandleGetUserByID(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "GetUserByEmail", func(ctx context.Context, query users.GetUserByEmailQuery) (interface{}, error) {
		return userQueryHandler.HandleGetUserByEmail(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "GetUserByUsername", func(ctx context.Context, query users.GetUserByUsernameQuery) (interface{}, error) {
		return userQueryHandler.HandleGetUserByUsername(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "GetUserSession", func(ctx context.Context, query users.GetUserSessionQuery) (interface{}, error) {
		return userQueryHandler.HandleGetUserSession(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "GetUserSessions", func(ctx context.Context, query users.GetUserSessionsQuery) (interface{}, error) {
		return userQueryHandler.HandleGetUserSessions(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "ListUsers", func(ctx context.Context, query users.ListUsersQuery) (interface{}, error) {
		return userQueryHandler.HandleListUsers(ctx, query)
	})
	shared.RegisterQueryHandler(queryBus, "SearchUsers", func(ctx context.Context, query users.SearchUsersQuery) (interface{}, error) {
		return userQueryHandler.HandleSearchUsers(ctx, query)
	})

	notificationService := notifications.NewNotificationService(commandBus, queryBus)
	projectService := projects.NewProjectService(commandBus, queryBus)
	issueService := issues.NewIssueService(commandBus, queryBus, projectService, issueProjectionStore)
	userService := users.NewUserService(commandBus, queryBus)

	// Create auth service that implements the interface needed by UserRoutes
	authService := users.NewAuthServiceImpl(userService)

	sessionGroupManager := realtime.NewGroupManager()

	// Initialize the global renderer registry with default renderers
	realtime.InitializeDefaultRenderers(issueService, projectService, userService, notificationService)

	eventProcessor := realtime.NewEventProcessor(nc, sessionGroupManager)
	userRoutes := users.NewUserRoutes(userService, sessionStore, authService, webauthnService)
	return &Deps{
		NotificationService: notificationService,
		IssueService:        issueService,
		ProjectService:      projectService,
		UserService:         userService,
		UserRoutes:          userRoutes,
		SessionStore:        sessionStore,
		SessionGroupManager: sessionGroupManager,
		EventProcessor:      eventProcessor,
		CommandBus:          commandBus,
		QueryBus:            queryBus,
		JetStream:           js,
		NatsConn:            nc,
	}, nil
}
