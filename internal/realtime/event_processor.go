package realtime

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/mbolli/tickit/internal/logger"
	"github.com/nats-io/nats.go"
)

// EventProcessor handles all incoming events and routes them to appropriate session groups
type EventProcessor struct {
	natsConn     *nats.Conn
	groupManager *GroupManager
	subscription *nats.Subscription
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.RWMutex
	running      bool
}

// NewEventProcessor creates a new global event processor
func NewEventProcessor(natsConn *nats.Conn, groupManager *GroupManager) *EventProcessor {
	ctx, cancel := context.WithCancel(context.Background())

	return &EventProcessor{
		natsConn:     natsConn,
		groupManager: groupManager,
		ctx:          ctx,
		cancel:       cancel,
		running:      false,
	}
}

// Start begins processing events
func (ep *EventProcessor) Start() error {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	if ep.running {
		return nil // Already running
	}

	// Subscribe to all events once
	sub, err := ep.natsConn.Subscribe("events", ep.handleEvent)
	if err != nil {
		return err
	}

	ep.subscription = sub
	ep.running = true

	logger.Debug().Msg("EventProcessor started, subscribed to NATS events")
	return nil
}

// Stop stops processing events
func (ep *EventProcessor) Stop() error {
	ep.mu.Lock()
	defer ep.mu.Unlock()

	if !ep.running {
		return nil
	}

	ep.cancel()

	if ep.subscription != nil {
		if err := ep.subscription.Unsubscribe(); err != nil {
			logger.Error().
				Err(err).
				Msg("Error unsubscribing from NATS")
		}
		ep.subscription = nil
	}

	ep.running = false
	logger.Debug().Msg("EventProcessor stopped")
	return nil
}

// handleEvent processes a single event and routes it to appropriate groups
func (ep *EventProcessor) handleEvent(msg *nats.Msg) {
	logger.Debug().Msgf("🎯 EventProcessor received message on topic: %s", msg.Subject)
	logger.Debug().Msgf("🎯 Message data: %s", string(msg.Data))

	var uiEvent UIUpdateEvent
	if err := json.Unmarshal(msg.Data, &uiEvent); err != nil {
		logger.Error().Msgf("❌ Failed to unmarshal UI event: %v", err)
		_ = msg.Ack()
		return
	}

	logger.Debug().Str("eventType", uiEvent.EventType).Str("entityType", uiEvent.EntityType).Str("entityID", uiEvent.EntityID).
		Msg("Processing UI event")

	// Get all groups that should receive this update
	groups := ep.groupManager.GetGroupsForEntity(uiEvent.EntityType, uiEvent.EntityID)

	// Send update to each relevant group
	for _, group := range groups {
		if group == nil {
			continue
		}

		// Check if this group should actually receive this update
		if !group.ShouldReceiveUpdate(uiEvent.EntityID) {
			continue
		}

		// Send update to all sessions in this group
		if err := group.SendUpdateToAllSessions(ep.ctx, &uiEvent); err != nil {
			logger.Error().Err(err).Str("groupID", group.ID).Msg("Failed to send update to group")
		}
	}

	_ = msg.Ack()
}

// IsRunning returns whether the processor is currently running
func (ep *EventProcessor) IsRunning() bool {
	ep.mu.RLock()
	defer ep.mu.RUnlock()
	return ep.running
}
