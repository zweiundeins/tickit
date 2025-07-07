package events

import (
	"time"

	"github.com/mbolli/tickit/internal/models"
)

// Events

type IssueCreatedEvent struct {
	Issue     *models.Issue
	Timestamp time.Time
}

type IssueUpdatedEvent struct {
	Issue     *models.Issue
	Timestamp time.Time
}

type IssueDeletedEvent struct {
	ID        string
	Timestamp time.Time
}

type IssueAssigneeChangedEvent struct {
	ID          string
	OldAssignee string
	NewAssignee string
	Timestamp   time.Time
}

type IssueStatusChangedEvent struct {
	ID        string
	OldStatus string
	NewStatus string
	Timestamp time.Time
}

type IssueTitleChangedEvent struct {
	ID        string
	OldTitle  string
	NewTitle  string
	Timestamp time.Time
}

type IssueDescriptionChangedEvent struct {
	ID             string
	OldDescription string
	NewDescription string
	Timestamp      time.Time
}

func (e IssueCreatedEvent) GetEventType() string    { return "IssueCreated" }
func (e IssueCreatedEvent) GetTimestamp() time.Time { return e.Timestamp }

func (e IssueUpdatedEvent) GetEventType() string    { return "IssueUpdated" }
func (e IssueUpdatedEvent) GetTimestamp() time.Time { return e.Timestamp }

func (e IssueDeletedEvent) GetEventType() string    { return "IssueDeleted" }
func (e IssueDeletedEvent) GetTimestamp() time.Time { return e.Timestamp }

func (e IssueAssigneeChangedEvent) GetEventType() string    { return "IssueAssigneeChanged" }
func (e IssueAssigneeChangedEvent) GetTimestamp() time.Time { return e.Timestamp }

func (e IssueStatusChangedEvent) GetEventType() string    { return "IssueStatusChanged" }
func (e IssueStatusChangedEvent) GetTimestamp() time.Time { return e.Timestamp }

func (e IssueTitleChangedEvent) GetEventType() string    { return "IssueTitleChanged" }
func (e IssueTitleChangedEvent) GetTimestamp() time.Time { return e.Timestamp }

func (e IssueDescriptionChangedEvent) GetEventType() string    { return "IssueDescriptionChanged" }
func (e IssueDescriptionChangedEvent) GetTimestamp() time.Time { return e.Timestamp }
