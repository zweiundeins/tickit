package notifications

import (
	"time"
)

type Notification struct {
	Uid    string    `json:"uid"`
	UserID string    `json:"user_id,omitempty"` // Optional user ID for user-specific notifications
	Date   time.Time `json:"date"`
	Title  string    `json:"title"`
	Body   string    `json:"body"`
	Read   bool      `json:"read"`
}

type Notifications struct {
	Notifications []*Notification `json:"notifications"`
}

func (ntfs *Notifications) UnreadCount() int {
	count := 0
	for _, ntf := range ntfs.Notifications {
		if !ntf.Read {
			count++
		}
	}
	return count
}
