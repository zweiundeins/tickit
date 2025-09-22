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

	// Use the event bus with handlers as the main event bus
	eventBus := eventBusWithHandlers.EventBusWrapper
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
	commandBus := shared.NewCommandBus()
	queryBus := shared.NewQueryBus()
	// Register command handlers
	commandBus.RegisterHandler("CreateNotification", notifications.NewCreateNotificationHandler(notifCommandHandler))
	commandBus.RegisterHandler("DeleteNotification", notifications.NewDeleteNotificationHandler(notifCommandHandler))
	commandBus.RegisterHandler("MarkNotificationRead", notifications.NewMarkNotificationReadHandler(notifCommandHandler))
	commandBus.RegisterHandler("CreateIssue", issues.NewCreateIssueHandler(issueCommandHandler))
	commandBus.RegisterHandler("UpdateIssue", issues.NewUpdateIssueHandler(issueCommandHandler))
	commandBus.RegisterHandler("DeleteIssue", issues.NewDeleteIssueHandler(issueCommandHandler))
	commandBus.RegisterHandler("PatchIssue", issues.NewPatchIssueHandler(issueCommandHandler))
	commandBus.RegisterHandler("CreateProject", projects.NewCreateProjectHandler(projectCommandHandler))
	commandBus.RegisterHandler("UpdateProject", projects.NewUpdateProjectHandler(projectCommandHandler))
	commandBus.RegisterHandler("DeleteProject", projects.NewDeleteProjectHandler(projectCommandHandler))

	// User command handlers
	commandBus.RegisterHandler("RegisterUser", users.NewRegisterUserHandler(userCommandHandler))
	commandBus.RegisterHandler("VerifyEmail", users.NewVerifyEmailHandler(userCommandHandler))
	commandBus.RegisterHandler("LoginUser", users.NewLoginUserHandler(userCommandHandler))
	commandBus.RegisterHandler("ForgotPassword", users.NewForgotPasswordHandler(userCommandHandler))
	commandBus.RegisterHandler("ResetPassword", users.NewResetPasswordHandler(userCommandHandler))
	commandBus.RegisterHandler("ChangePassword", users.NewChangePasswordHandler(userCommandHandler))

	// Register query handlers
	queryBus.RegisterHandler("GetNotifications", notifications.NewGetNotificationsHandler(notifQueryHandler))
	queryBus.RegisterHandler("GetUnreadCount", notifications.NewGetUnreadCountHandler(notifQueryHandler))
	queryBus.RegisterHandler("GetNotificationByUID", notifications.NewGetNotificationByUIDHandler(notifQueryHandler))
	queryBus.RegisterHandler("GetIssueByID", issues.NewIssueGetByIDHandler(issueQueryHandler))
	queryBus.RegisterHandler("ListIssues", issues.NewIssueListHandler(issueQueryHandler))
	queryBus.RegisterHandler("GetIssueByKey", issues.NewGetIssueByKeyHandler(issueQueryHandler))
	queryBus.RegisterHandler("GetProjectByID", projects.NewGetProjectByIDHandler(projectQueryHandler))
	queryBus.RegisterHandler("ListProjects", projects.NewListProjectsHandler(projectQueryHandler))

	// User query handlers
	userQueryHandler := users.NewUserQueryHandler(userProjectionStore, userStore)
	queryBus.RegisterHandler("GetUserByID", users.NewGetUserByIDHandler(userQueryHandler))
	queryBus.RegisterHandler("GetUserByEmail", users.NewGetUserByEmailHandler(userQueryHandler))
	queryBus.RegisterHandler("GetUserByUsername", users.NewGetUserByUsernameHandler(userQueryHandler))
	queryBus.RegisterHandler("ListUsers", users.NewListUsersHandler(userQueryHandler))
	queryBus.RegisterHandler("SearchUsers", users.NewSearchUsersHandler(userQueryHandler))

	notificationService := notifications.NewNotificationService(commandBus, queryBus)
	projectService := projects.NewProjectService(commandBus, queryBus)
	issueService := issues.NewIssueService(commandBus, queryBus, projectService, issueProjectionStore)
	userService := users.NewUserService(commandBus, queryBus)

	// Create auth service that implements the interface needed by UserRoutes
	authService := users.NewAuthServiceImpl(userService)

	sessionGroupManager := realtime.NewGroupManager()
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
		JetStream:           js,
		NatsConn:            nc,
	}, nil
}
