package cqrs

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/mbolli/tickit/internal/cqrs/application"
	"github.com/mbolli/tickit/internal/cqrs/bus"
	"github.com/mbolli/tickit/internal/cqrs/commands"
	"github.com/mbolli/tickit/internal/cqrs/events"
	"github.com/mbolli/tickit/internal/cqrs/queries"
	"github.com/stretchr/testify/assert"
)

// Example test showing how to use CQRS components
func TestNotificationService_CQRS_SQLite(t *testing.T) {
	ctx := context.Background()
	dbFile := ":temp:notification_test_%d.sqlite"
	dbFile = fmt.Sprintf(dbFile, time.Now().UnixNano())
	migrations := []string{
		// Event store migrations
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			aggregate_id TEXT NOT NULL,
			event_data TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			version INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_aggregate_id ON events(aggregate_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type)`,
		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_events_version ON events(version)`,
		// Notification projection migrations
		`CREATE TABLE IF NOT EXISTS notifications_projection (
			session_id TEXT NOT NULL,
			data TEXT NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (session_id)
		)`,
	}
	db, err := toolbelt.NewDatabase(ctx, dbFile, migrations)
	assert.NoError(t, err)
	defer func() {
		db.Close()
		os.Remove(dbFile)
	}()
	projectionStore := events.NewNotificationProjectionStoreWithDB(db)
	eventStore := events.NewEventStoreWithDB(db)
	eventBus := events.NewEventBusWrapper(nil)
	commandHandler := commands.NewCommandHandler(eventStore, projectionStore, eventBus)
	queryHandler := queries.NewQueryHandler(projectionStore)
	commandBus := bus.NewCommandBus()
	queryBus := bus.NewQueryBus()
	commandBus.RegisterHandler("CreateNotification", bus.NewCreateNotificationHandler(commandHandler))
	queryBus.RegisterHandler("GetNotifications", bus.NewGetNotificationsHandler(queryHandler))
	service := application.NewNotificationService(commandBus, queryBus)
	sessionID := "user123"
	title := "Test Notification"
	body := "This is a test notification"
	err = service.CreateNotification(ctx, sessionID, title, body)
	assert.NoError(t, err)
	result, err := service.GetNotifications(ctx, sessionID)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(result.Notifications))
	assert.Equal(t, title, result.Notifications[0].Title)
}

func TestIssueService_EventSourcing(t *testing.T) {
	ctx := context.Background()
	// Use a unique temp file for SQLite event store
	dbFile := ":temp:issue_test_%d.sqlite"
	dbFile = fmt.Sprintf(dbFile, time.Now().UnixNano())
	migrations := []string{
		// Event store migrations
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			aggregate_id TEXT NOT NULL,
			event_data TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			version INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_events_aggregate_id ON events(aggregate_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type)`,
		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_events_version ON events(version)`,
		// Issue projection migrations
		`CREATE TABLE IF NOT EXISTS issues_projection (
			id TEXT PRIMARY KEY,
			data TEXT NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
	}
	db, err := toolbelt.NewDatabase(ctx, dbFile, migrations)
	assert.NoError(t, err)
	defer func() {
		db.Close()
		os.Remove(dbFile)
	}()
	projectionStore := events.NewIssueProjectionStoreWithDB(db)
	eventStore := events.NewEventStoreWithDB(db)
	eventBus := events.NewEventBusWrapper(nil)
	issueCommandHandler := commands.NewIssueCommandHandler(eventBus, eventStore, projectionStore)
	issueQueryHandler := queries.NewIssueQueryHandler(projectionStore)
	commandBus := bus.NewCommandBus()
	queryBus := bus.NewQueryBus()
	commandBus.RegisterHandler("CreateIssue", bus.NewIssueCreateHandler(issueCommandHandler))
	commandBus.RegisterHandler("UpdateIssue", bus.NewIssueUpdateHandler(issueCommandHandler))
	commandBus.RegisterHandler("DeleteIssue", bus.NewIssueDeleteHandler(issueCommandHandler))
	queryBus.RegisterHandler("GetIssueByID", bus.NewIssueGetByIDHandler(issueQueryHandler))
	queryBus.RegisterHandler("ListIssues", bus.NewIssueListHandler(issueQueryHandler))
	service := application.NewIssueService(commandBus, queryBus)
	// Create an issue
	title := "Test Issue"
	description := "This is a test issue."
	assignee := "user1"
	reporter := "user2"
	_, err = service.CreateIssue(ctx, title, description, assignee, reporter)
	assert.NoError(t, err)
	// List issues
	issues, err := service.ListIssues(ctx, nil, nil, nil)
	assert.NoError(t, err)
	assert.Len(t, issues, 1)
	issue := issues[0]
	assert.Equal(t, title, issue.Title)
	assert.Equal(t, description, issue.Description)
	assert.Equal(t, assignee, issue.Assignee)
	assert.Equal(t, reporter, issue.Reporter)
	// Update the issue
	newTitle := "Updated Title"
	newDescription := "Updated description."
	err = service.UpdateIssue(ctx, issue.ID, &newTitle, &newDescription, nil, nil)
	assert.NoError(t, err)
	// Get by ID
	updated, err := service.GetIssueByID(ctx, issue.ID)
	assert.NoError(t, err)
	assert.Equal(t, newTitle, updated.Title)
	assert.Equal(t, newDescription, updated.Description)
	// Delete the issue
	err = service.DeleteIssue(ctx, issue.ID)
	assert.NoError(t, err)
	// Should not be found after deletion
	deleted, err := service.GetIssueByID(ctx, issue.ID)
	assert.NoError(t, err)
	assert.Nil(t, deleted)
}

func cleanupTempSQLiteFiles() {
	patterns := []string{":temp:*.sqlite", ":temp:*.sqlite-shm", ":temp:*.sqlite-wal"}
	for _, pattern := range patterns {
		files, _ := filepath.Glob(pattern)
		for _, f := range files {
			os.Remove(f)
		}
	}
}

func TestMain(m *testing.M) {
	code := m.Run()
	cleanupTempSQLiteFiles()
	os.Exit(code)
}
