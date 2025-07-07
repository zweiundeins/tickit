package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/delaneyj/toolbelt"
	"github.com/mbolli/tickit/internal/models"
	"zombiezen.com/go/sqlite"
)

type NotificationProjectionStore struct {
	db *toolbelt.Database
}

func NewNotificationProjectionStore(ctx context.Context, dbFilename string) (*NotificationProjectionStore, error) {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS notifications_projection (
			session_id TEXT NOT NULL,
			data TEXT NOT NULL,
			updated_at DATETIME NOT NULL,
			PRIMARY KEY (session_id)
		)`,
	}
	db, err := toolbelt.NewDatabase(ctx, dbFilename, migrations)
	if err != nil {
		return nil, fmt.Errorf("failed to create notifications projection db: %w", err)
	}
	return NewNotificationProjectionStoreWithDB(db), nil
}

// NewNotificationProjectionStoreWithDB creates a projection store from an existing *toolbelt.Database (no migrations)
func NewNotificationProjectionStoreWithDB(db *toolbelt.Database) *NotificationProjectionStore {
	return &NotificationProjectionStore{db: db}
}

// ApplyEvent updates the projection based on a notification event
func (ps *NotificationProjectionStore) ApplyEvent(ctx context.Context, event Event) error {
	switch e := event.(type) {
	case NotificationCreatedEvent:
		return ps.applyCreated(ctx, e)
	case NotificationDeletedEvent:
		return ps.applyDeleted(ctx, e)
	case NotificationReadEvent:
		return ps.applyRead(ctx, e)
	default:
		return nil // ignore unrelated events
	}
}

func (ps *NotificationProjectionStore) applyCreated(ctx context.Context, e NotificationCreatedEvent) error {
	notifs, err := ps.GetNotifications(ctx, e.SessionID)
	if err != nil {
		return err
	}
	notifs.Notifications = append(notifs.Notifications, e.Notification)
	return ps.save(ctx, e.SessionID, notifs)
}

func (ps *NotificationProjectionStore) applyDeleted(ctx context.Context, e NotificationDeletedEvent) error {
	notifs, err := ps.GetNotifications(ctx, e.SessionID)
	if err != nil {
		return err
	}
	if e.DeletedAll {
		notifs.Notifications = []*models.Notification{}
	} else {
		var filtered []*models.Notification
		for _, n := range notifs.Notifications {
			keep := true
			for _, delID := range e.DeletedNotificationIDs {
				if n.Uid == delID {
					keep = false
					break
				}
			}
			if keep {
				filtered = append(filtered, n)
			}
		}
		notifs.Notifications = filtered
	}
	return ps.save(ctx, e.SessionID, notifs)
}

func (ps *NotificationProjectionStore) applyRead(ctx context.Context, e NotificationReadEvent) error {
	notifs, err := ps.GetNotifications(ctx, e.SessionID)
	if err != nil {
		return err
	}
	for _, n := range notifs.Notifications {
		for _, readID := range e.ReadNotificationIDs {
			if n.Uid == readID {
				n.Read = true
			}
		}
	}
	return ps.save(ctx, e.SessionID, notifs)
}

func (ps *NotificationProjectionStore) GetNotifications(ctx context.Context, sessionID string) (*models.Notifications, error) {
	var data string
	err := ps.db.ReadTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`SELECT data FROM notifications_projection WHERE session_id = ?`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, sessionID)
		rowReturned, err := stmt.Step()
		if err != nil {
			return err
		}
		if !rowReturned {
			return nil // not found
		}
		data = stmt.ColumnText(0)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if data == "" {
		return &models.Notifications{}, nil
	}
	var notifs models.Notifications
	if err := json.Unmarshal([]byte(data), &notifs); err != nil {
		return nil, err
	}
	return &notifs, nil
}

func (ps *NotificationProjectionStore) save(ctx context.Context, sessionID string, notifs *models.Notifications) error {
	b, err := json.Marshal(notifs)
	if err != nil {
		return err
	}
	return ps.db.WriteTX(ctx, func(tx *sqlite.Conn) error {
		stmt, err := tx.Prepare(`INSERT OR REPLACE INTO notifications_projection (session_id, data, updated_at) VALUES (?, ?, ?)`)
		if err != nil {
			return err
		}
		defer stmt.Finalize()
		stmt.BindText(1, sessionID)
		stmt.BindText(2, string(b))
		stmt.BindInt64(3, time.Now().Unix())
		_, err = stmt.Step()
		return err
	})
}
