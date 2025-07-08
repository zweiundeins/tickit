package demo

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/shared"
	"github.com/stretchr/testify/assert"
)

func TestEventStore_StoreAndRetrieveEvent(t *testing.T) {
	ctx := context.Background()

	// Create event store with unique temporary file
	eventStore, err := shared.NewEventStore(ctx, fmt.Sprintf(":temp:%d.sqlite", time.Now().UnixNano()))
	assert.NoError(t, err)
	defer eventStore.Close()

	// Create test event
	event := notifications.NotificationCreatedEvent{
		SessionID: "test-session",
		Notification: &notifications.Notification{
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
	timestamp, ok := retrievedEvent.GetTimestamp().(time.Time)
	if !ok {
		t.Fatal("retrieved event timestamp is not time.Time")
	}
	assert.Equal(t, event.Timestamp.Unix(), timestamp.Unix())

	// Verify notification data
	if createdEvent, ok := retrievedEvent.(notifications.NotificationCreatedEvent); ok {
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
	eventStore, err := shared.NewEventStore(ctx, fmt.Sprintf(":temp:%d.sqlite", time.Now().UnixNano()))
	assert.NoError(t, err)
	defer eventStore.Close()

	// Store different types of events
	events := []shared.Event{
		notifications.NotificationCreatedEvent{
			SessionID: "session1",
			Notification: &notifications.Notification{
				Uid:   "uid1",
				Title: "Created 1",
			},
			Timestamp: time.Now(),
		},
		notifications.NotificationCreatedEvent{
			SessionID: "session2",
			Notification: &notifications.Notification{
				Uid:   "uid2",
				Title: "Created 2",
			},
			Timestamp: time.Now(),
		},
		notifications.NotificationDeletedEvent{
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
	eventStore, err := shared.NewEventStore(ctx, fmt.Sprintf(":temp:%d.sqlite", time.Now().UnixNano()))
	assert.NoError(t, err)
	defer eventStore.Close()

	// Store events at different times
	now := time.Now()
	oldTime := now.Add(-2 * time.Hour)
	recentTime := now.Add(-30 * time.Minute)

	events := []shared.Event{
		notifications.NotificationCreatedEvent{
			SessionID: "session1",
			Notification: &notifications.Notification{
				Uid:   "uid1",
				Title: "Old Event",
			},
			Timestamp: oldTime,
		},
		notifications.NotificationCreatedEvent{
			SessionID: "session2",
			Notification: &notifications.Notification{
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
	eventStore, err := shared.NewEventStore(ctx, fmt.Sprintf(":temp:%d.sqlite", time.Now().UnixNano()))
	assert.NoError(t, err)
	defer eventStore.Close()

	// Initially no events
	count, err := eventStore.GetEventCount(ctx)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), count)

	// Store some events
	events := []shared.Event{
		notifications.NotificationCreatedEvent{
			SessionID: "session1",
			Notification: &notifications.Notification{
				Uid:   "uid1",
				Title: "Event 1",
			},
			Timestamp: time.Now(),
		},
		notifications.NotificationCreatedEvent{
			SessionID: "session2",
			Notification: &notifications.Notification{
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
