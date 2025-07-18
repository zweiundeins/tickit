package projects

import (
	"time"

	"github.com/mbolli/tickit/internal/shared"
)

// ProjectCreatedEvent is emitted when a project is created
// Contains the full Project and timestamp

type ProjectCreatedEvent struct {
	Project   *Project
	Timestamp time.Time
}

func (e ProjectCreatedEvent) GetEventType() string      { return "ProjectCreated" }
func (e ProjectCreatedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*ProjectCreatedEvent)(nil)

// ProjectUpdatedEvent is emitted when a project is updated
// Contains the full Project and timestamp

type ProjectUpdatedEvent struct {
	Project   *Project
	Timestamp time.Time
}

func (e ProjectUpdatedEvent) GetEventType() string      { return "ProjectUpdated" }
func (e ProjectUpdatedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*ProjectUpdatedEvent)(nil)

// ProjectDeletedEvent is emitted when a project is deleted
// Contains the project ID and timestamp

type ProjectDeletedEvent struct {
	ID        string
	Timestamp time.Time
}

func (e ProjectDeletedEvent) GetEventType() string      { return "ProjectDeleted" }
func (e ProjectDeletedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*ProjectDeletedEvent)(nil)

// ProjectPatchedEvent is emitted when a project is patched (partial update)
type ProjectPatchedEvent struct {
	ID        string
	Changes   map[string]interface{}
	Timestamp time.Time
}

func (e ProjectPatchedEvent) GetEventType() string      { return "ProjectPatched" }
func (e ProjectPatchedEvent) GetTimestamp() interface{} { return e.Timestamp }

var _ shared.Event = (*ProjectPatchedEvent)(nil)
