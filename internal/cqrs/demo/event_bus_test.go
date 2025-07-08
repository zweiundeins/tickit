package demo

import (
	"context"
	"testing"
	"time"

	"github.com/mbolli/tickit/internal/notifications"
	"github.com/mbolli/tickit/internal/shared"

	"github.com/stretchr/testify/assert"
)

func TestEventBusWrapper_PublishEvent(t *testing.T) {
	// Create event bus
	eventBus := shared.NewEventBusWrapper(nil)

	// Track if event was received
	eventReceived := false
	var receivedEvent shared.Event

	// Subscribe to events
	cancel := eventBus.SubscribeToEvents(context.Background(), func(event shared.Event) error {
		eventReceived = true
		receivedEvent = event
		return nil
	})
	defer cancel()

	// Create and publish event
	event := notifications.NotificationCreatedEvent{
		SessionID: "test-session",
		Notification: &notifications.Notification{
			Uid:   "test-uid",
			Title: "Test Notification",
			Body:  "Test Body",
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := eventBus.PublishEvent(ctx, event)

	// Assertions
	assert.NoError(t, err)
	assert.True(t, eventReceived)
	assert.Equal(t, "NotificationCreated", receivedEvent.GetEventType())
}

func TestEventBusWrapper_SubscribeToEventType(t *testing.T) {
	// Create event bus
	eventBus := shared.NewEventBusWrapper(nil)

	// Track events
	createdEvents := 0
	deletedEvents := 0

	// Subscribe to specific event types
	cancelCreated := eventBus.SubscribeToEventType(context.Background(), "NotificationCreated", func(event shared.Event) error {
		createdEvents++
		return nil
	})
	defer cancelCreated()

	cancelDeleted := eventBus.SubscribeToEventType(context.Background(), "NotificationDeleted", func(event shared.Event) error {
		deletedEvents++
		return nil
	})
	defer cancelDeleted()

	// Publish different event types
	ctx := context.Background()

	createdEvent := notifications.NotificationCreatedEvent{
		SessionID: "test-session",
		Notification: &notifications.Notification{
			Uid:   "test-uid",
			Title: "Test",
		},
		Timestamp: time.Now(),
	}

	deletedEvent := notifications.NotificationDeletedEvent{
		SessionID:              "test-session",
		DeletedNotificationUID: "test-uid",
		DeletedAll:             false,
		DeletedNotificationIDs: []string{"test-uid"},
		Timestamp:              time.Now(),
	}

	// Publish events
	err1 := eventBus.PublishEvent(ctx, createdEvent)
	err2 := eventBus.PublishEvent(ctx, deletedEvent)

	// Assertions
	assert.NoError(t, err1)
	assert.NoError(t, err2)
	assert.Equal(t, 1, createdEvents)
	assert.Equal(t, 1, deletedEvents)
}

func TestEventBusWithHandlers(t *testing.T) {
	// Create handlers
	handlers := shared.EventHandlers{
		"NotificationCreated": func(ctx context.Context, event shared.Event) error {
			return nil
		},
	}

	// Create event bus with handlers
	eventBus := shared.NewEventBusWithHandlers(handlers, nil)

	// Publish event
	event := notifications.NotificationCreatedEvent{
		SessionID: "test-session",
		Notification: &notifications.Notification{
			Uid:   "test-uid",
			Title: "Test",
		},
		Timestamp: time.Now(),
	}

	ctx := context.Background()
	err := eventBus.PublishEvent(ctx, event)

	// Assertions
	assert.NoError(t, err)
}

func TestEventBusWrapper_GetSubscriberCount(t *testing.T) {
	// Create event bus
	eventBus := shared.NewEventBusWrapper(nil)

	// Initially no subscribers
	assert.Equal(t, 0, eventBus.GetSubscriberCount())

	// Add subscribers
	cancel1 := eventBus.SubscribeToEvents(context.Background(), func(event shared.Event) error {
		return nil
	})
	defer cancel1()

	assert.Equal(t, 1, eventBus.GetSubscriberCount())

	cancel2 := eventBus.SubscribeToEvents(context.Background(), func(event shared.Event) error {
		return nil
	})
	defer cancel2()

	assert.Equal(t, 2, eventBus.GetSubscriberCount())

	// Cancel one subscription
	cancel1()
	assert.Equal(t, 1, eventBus.GetSubscriberCount())
}
