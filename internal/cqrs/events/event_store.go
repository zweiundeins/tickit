package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/google/uuid"
	"zombiezen.com/go/sqlite"
)

// EventStore provides persistence for domain events
type EventStore struct {
	db *toolbelt.Database
}

// StoredEvent represents an event as stored in the database
type StoredEvent struct {
	ID          string    `json:"id"`
	EventType   string    `json:"event_type"`
	AggregateID string    `json:"aggregate_id"` // SessionID for notifications
	EventData   string    `json:"event_data"`   // JSON serialized event
	Timestamp   time.Time `json:"timestamp"`
	Version     int64     `json:"version"`
}

// NewEventStore creates a new event store
func NewEventStore(ctx context.Context, dbFilename string) (*EventStore, error) {
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
		`CREATE INDEX IF NOT EXISTS idx_events_aggregate_id ON events(aggregate_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_event_type ON events(event_type)`,
		`CREATE INDEX IF NOT EXISTS idx_events_timestamp ON events(timestamp)`,
		`CREATE INDEX IF NOT EXISTS idx_events_version ON events(version)`,
	}

	db, err := toolbelt.NewDatabase(ctx, dbFilename, migrations)
	if err != nil {
		return nil, fmt.Errorf("failed to create database: %w", err)
	}

	return NewEventStoreWithDB(db), nil
}

// NewEventStoreWithDB creates an event store from an existing *toolbelt.Database (no migrations)
func NewEventStoreWithDB(db *toolbelt.Database) *EventStore {
	return &EventStore{db: db}
}

// StoreEvent persists a domain event to the database
func (es *EventStore) StoreEvent(ctx context.Context, event Event) error {
	// Serialize event data
	eventData, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	// Get aggregate ID (SessionID for notifications)
	aggregateID := es.getAggregateID(event)

	// Get next version for this aggregate
	version, err := es.getNextVersion(ctx, aggregateID)
	if err != nil {
		return fmt.Errorf("failed to get next version: %w", err)
	}

	storedEvent := StoredEvent{
		ID:          uuid.New().String(),
		EventType:   event.GetEventType(),
		AggregateID: aggregateID,
		EventData:   string(eventData),
		Timestamp:   event.GetTimestamp(),
		Version:     version,
	}

	return es.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			INSERT INTO events (id, event_type, aggregate_id, event_data, timestamp, version)
			VALUES (?, ?, ?, ?, ?, ?)
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, storedEvent.ID)
		stmt.BindText(2, storedEvent.EventType)
		stmt.BindText(3, storedEvent.AggregateID)
		stmt.BindText(4, storedEvent.EventData)
		stmt.BindInt64(5, storedEvent.Timestamp.Unix())
		stmt.BindInt64(6, storedEvent.Version)

		_, err = stmt.Step()
		return err
	})
}

// GetEvents retrieves all events for an aggregate
func (es *EventStore) GetEvents(ctx context.Context, aggregateID string) ([]Event, error) {
	var events []Event

	err := es.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT event_type, event_data, timestamp, version
			FROM events 
			WHERE aggregate_id = ?
			ORDER BY version ASC
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, aggregateID)

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			eventType := stmt.ColumnText(0)
			eventData := stmt.ColumnText(1)
			timestampUnix := stmt.ColumnInt64(2)
			_ = stmt.ColumnInt64(3) // version - not used in this context

			event, err := es.deserializeEvent(eventType, eventData, time.Unix(timestampUnix, 0))
			if err != nil {
				return fmt.Errorf("failed to deserialize event: %w", err)
			}

			events = append(events, event)
		}

		return nil
	})

	return events, err
}

// GetEventsByType retrieves all events of a specific type
func (es *EventStore) GetEventsByType(ctx context.Context, eventType string) ([]Event, error) {
	var events []Event

	err := es.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT event_type, event_data, timestamp, version
			FROM events 
			WHERE event_type = ?
			ORDER BY timestamp ASC
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, eventType)

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			eventType := stmt.ColumnText(0)
			eventData := stmt.ColumnText(1)
			timestampUnix := stmt.ColumnInt64(2)
			_ = stmt.ColumnInt64(3) // version - not used in this context

			event, err := es.deserializeEvent(eventType, eventData, time.Unix(timestampUnix, 0))
			if err != nil {
				return fmt.Errorf("failed to deserialize event: %w", err)
			}

			events = append(events, event)
		}

		return nil
	})

	return events, err
}

// GetEventsSince retrieves events since a specific timestamp
func (es *EventStore) GetEventsSince(ctx context.Context, since time.Time) ([]Event, error) {
	var events []Event

	err := es.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT event_type, event_data, timestamp, version
			FROM events 
			WHERE timestamp >= ?
			ORDER BY timestamp ASC
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindInt64(1, since.Unix())

		for {
			rowReturned, err := stmt.Step()
			if err != nil {
				return err
			}
			if !rowReturned {
				break
			}

			eventType := stmt.ColumnText(0)
			eventData := stmt.ColumnText(1)
			timestampUnix := stmt.ColumnInt64(2)
			_ = stmt.ColumnInt64(3) // version - not used in this context

			event, err := es.deserializeEvent(eventType, eventData, time.Unix(timestampUnix, 0))
			if err != nil {
				return fmt.Errorf("failed to deserialize event: %w", err)
			}

			events = append(events, event)
		}

		return nil
	})

	return events, err
}

// GetEventCount returns the total number of events
func (es *EventStore) GetEventCount(ctx context.Context) (int64, error) {
	var count int64

	err := es.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`SELECT COUNT(*) FROM events`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			count = stmt.ColumnInt64(0)
		}

		return nil
	})

	return count, err
}

// Close closes the database connection
func (es *EventStore) Close() error {
	return es.db.Close()
}

// Helper methods

func (es *EventStore) getAggregateID(event Event) string {
	switch e := event.(type) {
	case NotificationCreatedEvent:
		return e.SessionID
	case NotificationDeletedEvent:
		return e.SessionID
	case NotificationReadEvent:
		return e.SessionID
	// Issue events
	case IssueCreatedEvent:
		if e.Issue != nil {
			return e.Issue.ID
		}
		return "unknown"
	case IssueUpdatedEvent:
		if e.Issue != nil {
			return e.Issue.ID
		}
		return "unknown"
	case IssueDeletedEvent:
		return e.ID
	case IssueAssigneeChangedEvent:
		return e.ID
	case IssueStatusChangedEvent:
		return e.ID
	case IssueTitleChangedEvent:
		return e.ID
	case IssueDescriptionChangedEvent:
		return e.ID
	default:
		return "unknown"
	}
}

func (es *EventStore) getNextVersion(ctx context.Context, aggregateID string) (int64, error) {
	var version int64

	err := es.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`
			SELECT COALESCE(MAX(version), 0) + 1
			FROM events 
			WHERE aggregate_id = ?
		`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()

		stmt.BindText(1, aggregateID)

		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if rowReturned {
			version = stmt.ColumnInt64(0)
		}

		return nil
	})

	return version, err
}

func (es *EventStore) deserializeEvent(eventType, eventData string, timestamp time.Time) (Event, error) {
	switch eventType {
	case "NotificationCreated":
		var event NotificationCreatedEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			return nil, err
		}
		event.Timestamp = timestamp
		return event, nil

	case "NotificationDeleted":
		var event NotificationDeletedEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			return nil, err
		}
		event.Timestamp = timestamp
		return event, nil

	case "NotificationRead":
		var event NotificationReadEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			return nil, err
		}
		event.Timestamp = timestamp
		return event, nil

	// Issue events
	case "IssueCreated":
		var event IssueCreatedEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			return nil, err
		}
		event.Timestamp = timestamp
		return event, nil
	case "IssueUpdated":
		var event IssueUpdatedEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			return nil, err
		}
		event.Timestamp = timestamp
		return event, nil
	case "IssueDeleted":
		var event IssueDeletedEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			return nil, err
		}
		event.Timestamp = timestamp
		return event, nil
	case "IssueAssigneeChanged":
		var event IssueAssigneeChangedEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			return nil, err
		}
		event.Timestamp = timestamp
		return event, nil
	case "IssueStatusChanged":
		var event IssueStatusChangedEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			return nil, err
		}
		event.Timestamp = timestamp
		return event, nil
	case "IssueTitleChanged":
		var event IssueTitleChangedEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			return nil, err
		}
		event.Timestamp = timestamp
		return event, nil
	case "IssueDescriptionChanged":
		var event IssueDescriptionChangedEvent
		if err := json.Unmarshal([]byte(eventData), &event); err != nil {
			return nil, err
		}
		event.Timestamp = timestamp
		return event, nil
	default:
		return nil, fmt.Errorf("unknown event type: %s", eventType)
	}
}
