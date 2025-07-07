package events

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/models"
	"github.com/stretchr/testify/assert"
)

func TestEventStore_StoreAndRetrieveEvent(t *testing.T) {
	ctx := context.Background()

	// Create event store with unique temporary file
	eventStore, err := NewEventStore(ctx, fmt.Sprintf(":temp:%d.sqlite", time.Now().UnixNano()))
	assert.NoError(t, err)
	defer eventStore.Close()

	// Create test event
	event := NotificationCreatedEvent{
		SessionID: "test-session",
		Notification: &models.Notification{
			Uid:   "test-uid",
			Title: "Test Notification",
			Body:  "Test Body",
		},
		Timestamp: time.Now(),
	}

	// Store event
	err = eventStore.StoreEvent(ctx, event)
	assert.NoError(t, err)

	// Retrieve events for the session
	events, err := eventStore.GetEvents(ctx, "test-session")
	assert.NoError(t, err)
	assert.Len(t, events, 1)

	// Verify event data
	retrievedEvent := events[0]
	assert.Equal(t, "NotificationCreated", retrievedEvent.GetEventType())
	assert.Equal(t, event.Timestamp.Unix(), retrievedEvent.GetTimestamp().Unix())

	// Verify notification data
	if createdEvent, ok := retrievedEvent.(NotificationCreatedEvent); ok {
		assert.Equal(t, "test-session", createdEvent.SessionID)
		assert.Equal(t, "test-uid", createdEvent.Notification.Uid)
		assert.Equal(t, "Test Notification", createdEvent.Notification.Title)
		assert.Equal(t, "Test Body", createdEvent.Notification.Body)
	} else {
		t.Fatal("retrieved event is not NotificationCreatedEvent")
	}
}

func TestEventStore_GetEventsByType(t *testing.T) {
	ctx := context.Background()

	// Create event store
	eventStore, err := NewEventStore(ctx, fmt.Sprintf(":temp:%d.sqlite", time.Now().UnixNano()))
	assert.NoError(t, err)
	defer eventStore.Close()

	// Store different types of events
	events := []Event{
		NotificationCreatedEvent{
			SessionID: "session1",
			Notification: &models.Notification{
				Uid:   "uid1",
				Title: "Created 1",
			},
			Timestamp: time.Now(),
		},
		NotificationCreatedEvent{
			SessionID: "session2",
			Notification: &models.Notification{
				Uid:   "uid2",
				Title: "Created 2",
			},
			Timestamp: time.Now(),
		},
		NotificationDeletedEvent{
			SessionID:              "session1",
			DeletedNotificationUID: "uid1",
			DeletedAll:             false,
			DeletedNotificationIDs: []string{"uid1"},
			Timestamp:              time.Now(),
		},
	}

	for _, event := range events {
		err = eventStore.StoreEvent(ctx, event)
		assert.NoError(t, err)
	}

	// Get events by type
	createdEvents, err := eventStore.GetEventsByType(ctx, "NotificationCreated")
	assert.NoError(t, err)
	assert.Len(t, createdEvents, 2)

	deletedEvents, err := eventStore.GetEventsByType(ctx, "NotificationDeleted")
	assert.NoError(t, err)
	assert.Len(t, deletedEvents, 1)
}

func TestEventStore_GetEventsSince(t *testing.T) {
	ctx := context.Background()

	// Create event store
	eventStore, err := NewEventStore(ctx, fmt.Sprintf(":temp:%d.sqlite", time.Now().UnixNano()))
	assert.NoError(t, err)
	defer eventStore.Close()

	// Store events at different times
	now := time.Now()
	oldTime := now.Add(-2 * time.Hour)
	recentTime := now.Add(-30 * time.Minute)

	events := []Event{
		NotificationCreatedEvent{
			SessionID: "session1",
			Notification: &models.Notification{
				Uid:   "uid1",
				Title: "Old Event",
			},
			Timestamp: oldTime,
		},
		NotificationCreatedEvent{
			SessionID: "session2",
			Notification: &models.Notification{
				Uid:   "uid2",
				Title: "Recent Event",
			},
			Timestamp: recentTime,
		},
	}

	for _, event := range events {
		err = eventStore.StoreEvent(ctx, event)
		assert.NoError(t, err)
	}

	// Get events since 1 hour ago
	since := now.Add(-1 * time.Hour)
	recentEvents, err := eventStore.GetEventsSince(ctx, since)
	assert.NoError(t, err)
	assert.Len(t, recentEvents, 1) // Only the recent event

	// Get events since 3 hours ago
	since = now.Add(-3 * time.Hour)
	allEvents, err := eventStore.GetEventsSince(ctx, since)
	assert.NoError(t, err)
	assert.Len(t, allEvents, 2) // Both events
}

func TestEventStore_GetEventCount(t *testing.T) {
	ctx := context.Background()

	// Create event store
	eventStore, err := NewEventStore(ctx, fmt.Sprintf(":temp:%d.sqlite", time.Now().UnixNano()))
	assert.NoError(t, err)
	defer eventStore.Close()

	// Initially no events
	count, err := eventStore.GetEventCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Store some events
	events := []Event{
		NotificationCreatedEvent{
			SessionID: "session1",
			Notification: &models.Notification{
				Uid:   "uid1",
				Title: "Event 1",
			},
			Timestamp: time.Now(),
		},
		NotificationCreatedEvent{
			SessionID: "session2",
			Notification: &models.Notification{
				Uid:   "uid2",
				Title: "Event 2",
			},
			Timestamp: time.Now(),
		},
	}

	for _, event := range events {
		err = eventStore.StoreEvent(ctx, event)
		assert.NoError(t, err)
	}

	// Check count
	count, err = eventStore.GetEventCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(2), count)
}

func TestReadModelBuilder_BuildNotificationsFromEvents(t *testing.T) {
	ctx := context.Background()

	// Create event store
	eventStore, err := NewEventStore(ctx, fmt.Sprintf(":temp:%d.sqlite", time.Now().UnixNano()))
	assert.NoError(t, err)
	defer eventStore.Close()

	// Create read model builder
	readModelBuilder := NewReadModelBuilder(eventStore)

	// Store a sequence of events
	sessionID := "test-session"
	events := []Event{
		NotificationCreatedEvent{
			SessionID: sessionID,
			Notification: &models.Notification{
				Uid:   "notif-1",
				Title: "First Notification",
				Body:  "Welcome!",
				Date:  time.Now(),
				Read:  false,
			},
			Timestamp: time.Now(),
		},
		NotificationCreatedEvent{
			SessionID: sessionID,
			Notification: &models.Notification{
				Uid:   "notif-2",
				Title: "Second Notification",
				Body:  "Important update",
				Date:  time.Now(),
				Read:  false,
			},
			Timestamp: time.Now(),
		},
		NotificationReadEvent{
			SessionID:           sessionID,
			ReadNotificationUID: "notif-1",
			MarkedAllAsRead:     false,
			ReadNotificationIDs: []string{"notif-1"},
			PreviousUnreadCount: 2,
			NewUnreadCount:      1,
			Timestamp:           time.Now(),
		},
		NotificationDeletedEvent{
			SessionID:              sessionID,
			DeletedNotificationUID: "notif-2",
			DeletedAll:             false,
			DeletedNotificationIDs: []string{"notif-2"},
			Timestamp:              time.Now(),
		},
	}

	for _, event := range events {
		err = eventStore.StoreEvent(ctx, event)
		assert.NoError(t, err)
	}

	// Build notifications from events
	notifications, err := readModelBuilder.BuildNotificationsFromEvents(ctx, sessionID)
	assert.NoError(t, err)

	// Verify results
	assert.Len(t, notifications.Notifications, 1) // Only notif-1 remains
	assert.Equal(t, "notif-1", notifications.Notifications[0].Uid)
	assert.True(t, notifications.Notifications[0].Read) // Should be marked as read
	assert.Equal(t, 0, notifications.UnreadCount())     // No unread notifications
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
