package wiki

import (
	"encoding/json"
	"errors"
	"strings"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/access"
	"github.com/mbolli/tickit/internal/demo/words"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
)

// SpaceEvent is an event of a space's stream.
type SpaceEvent interface{ spaceEvent() }

// SpaceCreated starts a space's stream (FR-76): its key, name,
// description, its home page (created in the same command) and whether
// everyone has a Member's rights in it (Q14).
type SpaceCreated struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Home        string `json:"home"`
	Open        bool   `json:"open"`
}

// SpaceEdited changes what it names (FR-77); the key stays.
type SpaceEdited struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// SpaceArchived and SpaceUnarchived (FR-77): an archived space is
// read-only.
type SpaceArchived struct{}
type SpaceUnarchived struct{}

func (SpaceCreated) spaceEvent()    {}
func (SpaceEdited) spaceEvent()     {}
func (SpaceArchived) spaceEvent()   {}
func (SpaceUnarchived) spaceEvent() {}

// SpaceStream is the kind of the spaces' streams, keyed by space id.
var SpaceStream = events.NewStream[string, SpaceEvent]("space")

func defineSpace[T SpaceEvent](name string) events.Type[string, SpaceEvent, T] {
	return events.Define(SpaceStream, name, func(t T) SpaceEvent { return t })
}

// The space event types; the names are what the log stores.
var (
	SpaceCreatedType    = defineSpace[SpaceCreated]("space.created")
	SpaceEditedType     = defineSpace[SpaceEdited]("space.edited")
	SpaceArchivedType   = defineSpace[SpaceArchived]("space.archived")
	SpaceUnarchivedType = defineSpace[SpaceUnarchived]("space.unarchived")
)

// SpaceState is what a space's commands decide on.
type SpaceState struct {
	Exists      bool   `json:"exists"`
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Home        string `json:"home"`
	Open        bool   `json:"open"`
	Archived    bool   `json:"archived,omitempty"`
	Version     int64  `json:"version"`
}

const spaceFold = 1

func evolveSpace(s SpaceState, e SpaceEvent) SpaceState {
	s.Version++
	switch e := e.(type) {
	case SpaceCreated:
		s.Exists, s.Key, s.Name, s.Description, s.Home, s.Open = true, e.Key, e.Name, e.Description, e.Home, e.Open
	case SpaceEdited:
		if e.Name != nil {
			s.Name = *e.Name
		}
		if e.Description != nil {
			s.Description = *e.Description
		}
	case SpaceArchived:
		s.Archived = true
	case SpaceUnarchived:
		s.Archived = false
	}
	return s
}

func spaceDecider[C any](decide func(C, SpaceState) ([]SpaceEvent, error)) events.Decider[C, SpaceEvent, SpaceState] {
	return events.Decider[C, SpaceEvent, SpaceState]{Decide: decide, Evolve: evolveSpace, Version: spaceFold}
}

var peekSpaceDecider = peekDecider(SpaceState{}, evolveSpace, spaceFold)

// spaceState reads a space's state inside a command.
func spaceState(tx *writer.Tx, id string) (SpaceState, error) {
	var s SpaceState
	if !ValidID(id) {
		return s, notFound("space", id)
	}
	if err := events.Execute(Store(), tx, peekSpaceDecider, SpaceStream.ID(id), events.Any, peek[SpaceState]{&s}, events.Meta{}); err != nil {
		return s, err
	}
	if !s.Exists {
		return s, notFound("space", id)
	}
	return s, nil
}

// Space keys have 2 to 10 letters A to Z (FR-76.1).
const (
	MinSpaceKey = 2
	MaxSpaceKey = 10
)

// NormalizeSpaceKey turns a key to upper case and checks it.
func NormalizeSpaceKey(key string) (string, bool) {
	k := strings.ToUpper(strings.TrimSpace(key))
	if len(k) < MinSpaceKey || len(k) > MaxSpaceKey {
		return k, false
	}
	for _, r := range k {
		if r < 'A' || r > 'Z' {
			return k, false
		}
	}
	return k, true
}

var errNoSpace = errors.New("wiki: no such space")

// CreateSpace creates a space with its home page, and makes its creator
// its Admin (FR-76). Its result is {"id", "key", "home"} as JSON.
type CreateSpace struct {
	By          tabs.By
	ID, Home    string // optional: new ids when empty
	Key         string
	Name        string
	Description string
	Open        bool
	// HomeDoc is the home page's document; empty: a blank page.
	HomeDoc json.RawMessage
}

func (c CreateSpace) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if c.ID != "" && !ValidID(c.ID) || c.Home != "" && !ValidID(c.Home) {
		return errors.New("wiki: a malformed space")
	}
	return nil
}

func (c CreateSpace) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

var createSpace = spaceDecider(func(c SpaceCreated, s SpaceState) ([]SpaceEvent, error) {
	if s.Exists {
		return nil, errors.New("wiki: the space exists")
	}
	return []SpaceEvent{c}, nil
})

func (c CreateSpace) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if c.By.Actor.Kind != people.Seed {
		if err := access.Require(tx, c.By.Actor, access.CreateScope, access.Scope{}); err != nil {
			return nil, err
		}
	}
	key, ok := NormalizeSpaceKey(c.Key)
	if !ok {
		return nil, tabs.Refuse("key", "A key has 2 to 10 letters A to Z.")
	}
	if words.Blocked(key) {
		return nil, tabs.Refuse("key", words.Refusal)
	}
	name, ok := text(c.Name, 1, MaxSpaceName)
	if !ok {
		return nil, tabs.Refuse("name", "A name has 1 to 100 characters.")
	}
	if words.Blocked(name) { // DM-06.4
		return nil, tabs.Refuse("name", words.Refusal)
	}
	desc, ok := longText(c.Description, MaxDescription)
	if !ok {
		return nil, tabs.Refuse("description", "A description has at most 2,000 characters.")
	}
	id, home := c.ID, c.Home
	if id == "" {
		id = NewID()
	}
	if home == "" {
		home = NewID()
	}
	if err := access.AddScope(tx, access.Space, id, key, c.Open, c.By.Actor.ID, at(c.By)); err != nil {
		return nil, err
	}
	e := SpaceCreated{Key: key, Name: name, Description: desc, Home: home, Open: c.Open}
	if err := events.Execute(Store(), tx, createSpace, SpaceStream.ID(id), events.NoStream, e, meta(c.By)); err != nil {
		return nil, err
	}
	if err := createPage(tx, c.By, newPage{id: home, space: id, title: name, home: true, doc: c.HomeDoc}); err != nil {
		return nil, err
	}
	return json.Marshal(map[string]string{"id": id, "key": key, "home": home})
}

// EditSpace changes a space's name or description, those that are set
// (FR-77): the space's Admin does.
type EditSpace struct {
	By          tabs.By
	Space       string
	Name        *string
	Description *string
}

func (c EditSpace) Validate() error { return checkBy(c.By) }

var editSpace = spaceDecider(func(c EditSpace, s SpaceState) ([]SpaceEvent, error) {
	if !s.Exists {
		return nil, errNoSpace
	}
	var e SpaceEdited
	if c.Name != nil {
		name, ok := text(*c.Name, 1, MaxSpaceName)
		if !ok {
			return nil, tabs.Refuse("name", "A name has 1 to 100 characters.")
		}
		if words.Blocked(name) {
			return nil, tabs.Refuse("name", words.Refusal)
		}
		if name != s.Name {
			e.Name = &name
		}
	}
	if c.Description != nil {
		desc, ok := longText(*c.Description, MaxDescription)
		if !ok {
			return nil, tabs.Refuse("description", "A description has at most 2,000 characters.")
		}
		if desc != s.Description {
			e.Description = &desc
		}
	}
	if e == (SpaceEdited{}) {
		return nil, nil
	}
	return []SpaceEvent{e}, nil
})

func (c EditSpace) Apply(tx *writer.Tx) error {
	if _, err := spaceState(tx, c.Space); err != nil {
		return err
	}
	if err := require(tx, c.By, access.Configure, c.Space); err != nil {
		return err
	}
	return events.Execute(Store(), tx, editSpace, SpaceStream.ID(c.Space), events.Any, c, meta(c.By))
}

// ArchiveSpace archives a space or takes it out of the archive (FR-77):
// its Admin does.
type ArchiveSpace struct {
	By       tabs.By
	Space    string
	Archived bool
}

func (c ArchiveSpace) Validate() error { return checkBy(c.By) }

var archiveSpace = spaceDecider(func(c ArchiveSpace, s SpaceState) ([]SpaceEvent, error) {
	switch {
	case !s.Exists:
		return nil, errNoSpace
	case s.Archived == c.Archived:
		return nil, nil
	case c.Archived:
		return []SpaceEvent{SpaceArchived{}}, nil
	}
	return []SpaceEvent{SpaceUnarchived{}}, nil
})

func (c ArchiveSpace) Apply(tx *writer.Tx) error {
	if _, err := spaceState(tx, c.Space); err != nil {
		return err
	}
	if err := require(tx, c.By, access.ArchiveScope, c.Space); err != nil {
		return err
	}
	if err := events.Execute(Store(), tx, archiveSpace, SpaceStream.ID(c.Space), events.Any, c, meta(c.By)); err != nil {
		return err
	}
	return access.SetArchived(tx, access.SpaceScope(c.Space), c.Archived)
}
