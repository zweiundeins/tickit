package models

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/starfederation/datastar/sdk/go/datastar"
	"net/http"

	"github.com/nats-io/nats.go/jetstream"
)

type Notification struct {
	Uid   string `json:"uid"`
	Date  string `json:"date"`
	Title string `json:"title"`
	Body  string `json:"body"`
	Read  bool   `json:"read"`
}

type Notifications struct {
	Notifications []*Notification `json:"notifications"`
}

func SaveNotifications(ctx context.Context, kv jetstream.KeyValue, sessionID string, ntfs *Notifications) error {
	b, err := json.Marshal(ntfs)
	if err != nil {
		return fmt.Errorf("failed to marshal mvc: %w", err)
	}
	if _, err := kv.Put(ctx, sessionID, b); err != nil {
		return fmt.Errorf("failed to put key value: %w", err)
	}
	return nil
}

func DeleteNotifications(r *http.Request, kv jetstream.KeyValue, sessionID string) error {
	ctx := r.Context()
	type Signals struct {
		NotificationUid string `json:"notificationuid"`
	}
	signals := &Signals{}

	if err := datastar.ReadSignals(r, signals); err != nil {
		return fmt.Errorf("failed to read signals: %w", err)
	}

	// print signals for debugging
	fmt.Printf("Signals: %+v\n", signals)

	if signals.NotificationUid == "" {
		if err := kv.Delete(ctx, sessionID); err != nil {
			return fmt.Errorf("failed to delete key value: %w", err)
		}
		return nil // No specific notification UID provided, delete all notifications
	}

	// If a specific notification UID is provided, delete only that notification
	item, err := kv.Get(ctx, sessionID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil // No notifications found
		}
		return fmt.Errorf("failed to get key value: %w", err)
	}

	var notifications Notifications
	if err := json.Unmarshal(item.Value(), &notifications); err != nil {
		return fmt.Errorf("failed to unmarshal notifications: %w", err)
	}

	for i, ntf := range notifications.Notifications {
		if signals.NotificationUid == ntf.Uid {
			// Remove the notification with the matching UID
			// Apparently there is no better way to remove an item from a slice in Go
			notifications.Notifications = append(notifications.Notifications[:i], notifications.Notifications[i+1:]...)
			break
		}
	}

	if err := SaveNotifications(ctx, kv, sessionID, &notifications); err != nil {
		return fmt.Errorf("failed to save notifications: %w", err)
	}

	return nil
}

func GetNotifications(sessionID string, ctx context.Context, kv jetstream.KeyValue) (*Notifications, error) {
	item, err := kv.Get(ctx, sessionID)
	if err != nil {
		if err == jetstream.ErrKeyNotFound {
			return &Notifications{}, nil // No notifications found, return empty Notifications
		}
		return nil, fmt.Errorf("failed to get key value: %w", err)
	}

	var notifications Notifications
	if err := json.Unmarshal(item.Value(), &notifications); err != nil {
		return nil, fmt.Errorf("failed to unmarshal notifications: %w", err)
	}

	return &notifications, nil
}

func GetUnreadCount(ntfs *Notifications) int {
	count := 0
	for _, ntf := range ntfs.Notifications {
		if !ntf.Read {
			count++
		}
	}
	return count
}

func ReadNotifications(r *http.Request, kv jetstream.KeyValue, sessionID string) error {
	ctx := r.Context()
	type Signals struct {
		NotificationUid string `json:"notificationuid"`
	}
	signals := &Signals{}

	if err := datastar.ReadSignals(r, signals); err != nil {
		return fmt.Errorf("failed to read signals: %w", err)
	}

	// print signals for debugging
	fmt.Printf("Signals: %+v\n", signals)

	item, err := kv.Get(ctx, sessionID)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil // No notifications found
		}
		return fmt.Errorf("failed to get key value: %w", err)
	}

	var notifications Notifications
	if err := json.Unmarshal(item.Value(), &notifications); err != nil {
		return fmt.Errorf("failed to unmarshal notifications: %w", err)
	}

	for _, ntf := range notifications.Notifications {
		if signals.NotificationUid == "" || (signals.NotificationUid != "" && ntf.Uid == signals.NotificationUid) {
			ntf.Read = true
		}
	}

	if err := SaveNotifications(ctx, kv, sessionID, &notifications); err != nil {
		return fmt.Errorf("failed to save notifications: %w", err)
	}

	return nil
}
