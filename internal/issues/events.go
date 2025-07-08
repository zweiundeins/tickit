package issues

import (
	"time"

	"github.com/mbolli/tickit/internal/shared"
)

// Events

type IssueCreatedEvent struct {
	Issue     *Issue
	Timestamp time.Time
}

func (e IssueCreatedEvent) GetEventType() string      { return "IssueCreated" }
func (e IssueCreatedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*IssueCreatedEvent)(nil)

type IssueUpdatedEvent struct {
	Issue     *Issue
	Timestamp time.Time
}

func (e IssueUpdatedEvent) GetEventType() string      { return "IssueUpdated" }
func (e IssueUpdatedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*IssueUpdatedEvent)(nil)

type IssueDeletedEvent struct {
	ID        string
	Timestamp time.Time
}

func (e IssueDeletedEvent) GetEventType() string      { return "IssueDeleted" }
func (e IssueDeletedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*IssueDeletedEvent)(nil)

type IssueAssigneeChangedEvent struct {
	ID          string
	OldAssignee string
	NewAssignee string
	Timestamp   time.Time
}

func (e IssueAssigneeChangedEvent) GetEventType() string      { return "IssueAssigneeChanged" }
func (e IssueAssigneeChangedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*IssueAssigneeChangedEvent)(nil)

type IssueStatusChangedEvent struct {
	ID        string
	OldStatus string
	NewStatus string
	Timestamp time.Time
}

func (e IssueStatusChangedEvent) GetEventType() string      { return "IssueStatusChanged" }
func (e IssueStatusChangedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*IssueStatusChangedEvent)(nil)

type IssueTitleChangedEvent struct {
	ID        string
	OldTitle  string
	NewTitle  string
	Timestamp time.Time
}

func (e IssueTitleChangedEvent) GetEventType() string      { return "IssueTitleChanged" }
func (e IssueTitleChangedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*IssueTitleChangedEvent)(nil)

type IssueDescriptionChangedEvent struct {
	ID             string
	OldDescription string
	NewDescription string
	Timestamp      time.Time
}

func (e IssueDescriptionChangedEvent) GetEventType() string      { return "IssueDescriptionChanged" }
func (e IssueDescriptionChangedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*IssueDescriptionChangedEvent)(nil)
