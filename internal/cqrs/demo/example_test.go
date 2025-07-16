package demo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/mbolli/tickit/internal/issues"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/assert"
	"zombiezen.com/go/sqlite"
)

// Example test showing how to use CQRS components
// func TestNotificationService_CQRS_SQLite(t *testing.T) {
// 	ctx := context.Background()
// 	dbFile := ":temp:notification_test_%d.sqlite"
// 	dbFile = fmt.Sprintf(dbFile, time.Now().UnixNano())
// 	migrations := []string{
// 		// Event store migrations
// 		`CREATE TABLE IF NOT EXISTS events (
// 			id TEXT PRIMARY KEY,
// 			event_type TEXT NOT NULL,
// 			aggregate_id TEXT NOT NULL,
// 			event_data TEXT NOT NULL,
// 			timestamp DATETIME NOT NULL,
// 			version INTEGER NOT NULL,
// 			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
// 		)`,
// 		`CREATE INDEX IF NOT EXISTS idx_events_aggregate_id ON events(aggregate_id)`,
// 		`CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type)`,
// 		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp)`,
// 		`CREATE INDEX IF NOT EXISTS idx_events_version ON events(version)`,
// 		// Notification projection migrations
// 		`CREATE TABLE IF NOT EXISTS notifications_projection (
// 			session_id TEXT NOT NULL,
// 			data TEXT NOT NULL,
// 			updated_at DATETIME NOT NULL,
// 			PRIMARY KEY (session_id)
// 		)`,
// 	}
// 	db, err := toolbelt.NewDatabase(ctx, dbFile, migrations)
// 	assert.NoError(t, err)
// 	defer func() {
// 		db.Close()
// 		os.Remove(dbFile)
// 	}()
// 	projectionStore := notifications.NewNotificationProjectionStoreWithDB(db)
// 	eventStore := shared.NewEventStoreWithDB(db)
// 	eventBus := shared.NewEventBusWrapper(nil)
// 	commandHandler := notifications.NewCommandHandler(eventStore, projectionStore, eventBus)
// 	queryHandler := notifications.NewQueryHandler(projectionStore)
// 	commandBus := shared.NewCommandBus()
// 	queryBus := shared.NewQueryBus()
// 	commandBus.RegisterHandler("CreateNotification", notifications.NewCreateNotificationHandler(commandHandler))
// 	queryBus.RegisterHandler("GetNotifications", notifications.NewGetNotificationsHandler(queryHandler))
// 	service := notifications.NewNotificationService(commandBus, queryBus)
// 	sessionID := "user123"
// 	title := "Test Notification"
// 	body := "This is a test notification"
// 	err = service.CreateNotification(ctx, sessionID, title, body)
// 	assert.NoError(t, err)
// 	result, err := service.GetNotifications(ctx, sessionID)
// 	assert.NoError(t, err)
// 	assert.Equal(t, 1, len(result.Notifications))
// 	assert.Equal(t, title, result.Notifications[0].Title)
// }

// func TestIssueService_EventSourcing(t *testing.T) {
// 	ctx := context.Background()
// 	// Use a unique temp file for SQLite event store
// 	dbFile := ":temp:issue_test_%d.sqlite"
// 	dbFile = fmt.Sprintf(dbFile, time.Now().UnixNano())
// 	migrations := []string{
// 		// Event store migrations
// 		`CREATE TABLE IF NOT EXISTS events (
// 			id TEXT PRIMARY KEY,
// 			event_type TEXT NOT NULL,
// 			aggregate_id TEXT NOT NULL,
// 			event_data TEXT NOT NULL,
// 			timestamp DATETIME NOT NULL,
// 			version INTEGER NOT NULL,
// 			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
// 		)`,
// 		`CREATE INDEX IF NOT EXISTS idx_events_aggregate_id ON events(aggregate_id)`,
// 		`CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type)`,
// 		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp)`,
// 		`CREATE INDEX IF NOT EXISTS idx_events_version ON events(version)`,
// 		// Issue projection migrations
// 		`CREATE TABLE IF NOT EXISTS issues_projection (
// 			id TEXT PRIMARY KEY,
// 			data TEXT NOT NULL,
// 			updated_at DATETIME NOT NULL
// 		)`,
// 	}
// 	db, err := toolbelt.NewDatabase(ctx, dbFile, migrations)
// 	assert.NoError(t, err)
// 	defer func() {
// 		db.Close()
// 		os.Remove(dbFile)
// 	}()
// 	projectionStore := issues.NewIssueProjectionStoreWithDB(db)
// 	eventStore := shared.NewEventStoreWithDB(db)
// 	eventBus := shared.NewEventBusWrapper(nil)
// 	issueCommandHandler := issues.NewIssueCommandHandler(eventBus, eventStore, projectionStore)
// 	issueQueryHandler := issues.NewIssueQueryHandler(projectionStore)
// 	commandBus := shared.NewCommandBus()
// 	queryBus := shared.NewQueryBus()
// 	commandBus.RegisterHandler("CreateIssue", issues.NewCreateIssueHandler(issueCommandHandler))
// 	commandBus.RegisterHandler("UpdateIssue", issues.NewUpdateIssueHandler(issueCommandHandler))
// 	commandBus.RegisterHandler("DeleteIssue", issues.NewDeleteIssueHandler(issueCommandHandler))
// 	queryBus.RegisterHandler("GetIssueByID", issues.NewIssueGetByIDHandler(issueQueryHandler))
// 	queryBus.RegisterHandler("ListIssues", issues.NewIssueListHandler(issueQueryHandler))
// 	service := issues.NewIssueService(commandBus, queryBus)
// 	// Create an issue
// 	title := "Test Issue"
// 	description := "This is a test issue."
// 	assignee := "user1"
// 	reporter := "user2"
// 	_, err = service.CreateIssue(ctx, title, description, assignee, reporter, "", "Medium")
// 	assert.NoError(t, err)
// 	// List issues
// 	issues, err := service.ListIssues(ctx, nil, nil, nil)
// 	assert.NoError(t, err)
// 	assert.Len(t, issues, 1)
// 	issue := issues[0]
// 	assert.Equal(t, title, issue.Title)
// 	assert.Equal(t, description, issue.Description)
// 	assert.Equal(t, assignee, issue.Assignee)
// 	assert.Equal(t, reporter, issue.Reporter)
// 	// Update the issue
// 	newTitle := "Updated Title"
// 	newDescription := "Updated description."
// 	err = service.UpdateIssue(ctx, issue.ID, &newTitle, &newDescription, nil, nil, nil, nil)
// 	assert.NoError(t, err)
// 	// Get by ID
// 	updated, err := service.GetIssueByID(ctx, issue.ID)
// 	assert.NoError(t, err)
// 	assert.Equal(t, newTitle, updated.Title)
// 	assert.Equal(t, newDescription, updated.Description)
// 	// Delete the issue
// 	err = service.DeleteIssue(ctx, issue.ID)
// 	assert.NoError(t, err)
// 	// Should not be found after deletion
// 	deleted, err := service.GetIssueByID(ctx, issue.ID)
// 	assert.NoError(t, err)
// 	assert.Nil(t, deleted)
// }

func TestIssuePatchedEvent_StoredEventData(t *testing.T) {
	ctx := context.Background()
	// Use a unique temp file for SQLite event store
	dbFile := ":temp:issue_patch_test_%d.sqlite"
	dbFile = fmt.Sprintf(dbFile, time.Now().UnixNano())
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS events (
			id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			aggregate_id TEXT NOT NULL,
			event_data TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			version INTEGER NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
	}
	db, err := toolbelt.NewDatabase(ctx, dbFile, migrations)
	assert.NoError(t, err)
	defer func() {
		db.Close()
		os.Remove(dbFile)
	}()
	eventStore := shared.NewEventStoreWithDB(db)

	id := "patch-123"
	changes := map[string]interface{}{"Title": "Patched Title", "Priority": "High"}
	timestamp := time.Now().UTC().Truncate(time.Second)
	event := issues.IssuePatchedEvent{
		ID:        id,
		Changes:   changes,
		Timestamp: timestamp,
	}
	// Store the event
	err = eventStore.StoreEvent(ctx, event)
	assert.NoError(t, err)

	// Read back the raw event_data
	var eventData string
	err = db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`SELECT event_data FROM events WHERE event_type = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, "IssuePatched")
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return fmt.Errorf("no event found")
		}
		eventData = stmt.ColumnText(0)
		return nil
	})
	assert.NoError(t, err)

	// Unmarshal and check fields
	var unmarshaled issues.IssuePatchedEvent
	err = json.Unmarshal([]byte(eventData), &unmarshaled)
	assert.NoError(t, err)
	assert.Equal(t, id, unmarshaled.ID)
	assert.Equal(t, changes["Title"], unmarshaled.Changes["Title"])
	assert.Equal(t, changes["Priority"], unmarshaled.Changes["Priority"])
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
