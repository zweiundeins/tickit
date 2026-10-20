// Package dashboards keeps people's dashboards (FR-60): a named grid of
// gadgets, each one of Kinds. They are relational rows that these
// commands write, like saved filters: in the demo everyone sees every
// dashboard, and only its owner (or a site admin) changes it. What a
// gadget shows comes from the tracker and the feeds when the dashboard's
// view renders (internal/web).
package dashboards

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo/words"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Kind is what a gadget shows.
type Kind string

const (
	// Filter is the issues of a saved filter (FR-56.2).
	Filter Kind = "filter"
	// Status is a project's open and done issues counted by status, as a
	// chart with its numbers in a table.
	Status Kind = "status"
	// Assigned is the viewer's own open issues: the one gadget that is
	// the viewer's, rendered by a view per person.
	Assigned Kind = "assigned"
	// Activity is the feed of what the viewer may read (FR-69).
	Activity Kind = "activity"
	// Created is a project's issues created against those resolved, per
	// day (FR-38).
	Created Kind = "created"
)

// Kinds are the kinds of gadgets, in the order the page offers them.
var Kinds = []Kind{Filter, Status, Assigned, Activity, Created}

// Valid reports whether k is a kind of gadget.
func (k Kind) Valid() bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Name is the kind as a person reads it.
func (k Kind) Name() string {
	switch k {
	case Filter:
		return "Saved filter"
	case Status:
		return "Issues by status"
	case Assigned:
		return "Assigned to me"
	case Activity:
		return "Activity"
	case Created:
		return "Created and resolved"
	}
	return string(k)
}

// NeedsProject and NeedsFilter say what a gadget of the kind is about.
func (k Kind) NeedsProject() bool { return k == Status || k == Created }
func (k Kind) NeedsFilter() bool  { return k == Filter }

// Limits of dashboards.
const (
	MaxName       = 60
	MaxGadgets    = 12 // per dashboard
	MaxDashboards = 20 // per owner
)

// Topics: the list of dashboards, and one dashboard's name and gadgets.
const ListTopic = "dashboards"

// Topic is a dashboard's own topic: a root of its own, so a change of one
// dashboard renders that dashboard's views alone.
func Topic(id string) string { return "dashboard:" + id }

// Dashboard is a dashboard with its gadgets in order.
type Dashboard struct {
	ID      string
	Owner   string
	Name    string
	Created time.Time
	Updated time.Time
	Gadgets []Gadget
	Count   int // gadgets, in List
}

// Gadget is one gadget of a dashboard: Project for Status and Created,
// SavedFilter for Filter.
type Gadget struct {
	ID          string
	Kind        Kind
	Position    int
	Project     string
	SavedFilter string
}

// ValidID reports whether id is a dashboard's or a gadget's id.
func ValidID(id string) bool {
	u, err := uuid.Parse(id)
	return err == nil && u.String() == id
}

func newID() string { return uuid.Must(uuid.NewV7()).String() }

func notFound(what, id string) error { return fmt.Errorf("%s %s: %w", what, id, access.ErrNotFound) }

func checkBy(by tabs.By) error {
	if by.Actor.IsZero() || by.At.IsZero() {
		return errors.New("dashboards: an action needs an actor and a time")
	}
	return nil
}

// name checks a dashboard's name.
func name(s string) (string, error) {
	s = strings.TrimSpace(s)
	n := utf8.RuneCountInString(s)
	if n < 1 || n > MaxName || strings.IndexFunc(s, unicode.IsControl) >= 0 {
		return "", tabs.Refuse("name", "A dashboard's name has 1 to 60 characters.")
	}
	if words.Blocked(s) {
		return "", tabs.Refuse("name", words.Refusal)
	}
	return s, nil
}

// denial is a refusal for want of a right: the action answers 403.
type denial struct{ writer.Refusal }

func (d *denial) Error() string { return d.Msg }
func (d *denial) Unwrap() error { return &d.Refusal }
func (d *denial) Denied() bool  { return true }

// own refuses a change to a dashboard by anyone but its owner or a site
// admin.
func own(tx *writer.Tx, by tabs.By, id string) error {
	var owner string
	err := tx.QueryRow(`SELECT owner FROM dashboards WHERE id = ?`, id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("dashboard", id)
	}
	if err != nil {
		return err
	}
	if owner != by.Actor.ID && by.Actor.Role != people.Admin {
		return &denial{writer.Refusal{Msg: "Only its owner can change this dashboard."}}
	}
	return nil
}

func touched(tx *writer.Tx, id string, at time.Time) error {
	if _, err := tx.Exec(`UPDATE dashboards SET updated_at = ? WHERE id = ?`, at.UnixMilli(), id); err != nil {
		return err
	}
	tx.Touch(Topic(id), ListTopic)
	return nil
}

// Create makes a dashboard of the actor's; its result is {"id": "..."}.
type Create struct {
	By   tabs.By
	ID   string // optional
	Name string
}

func (c Create) Validate() error {
	if c.ID != "" && !ValidID(c.ID) {
		return errors.New("dashboards: a malformed id")
	}
	return checkBy(c.By)
}

func (c Create) Apply(tx *writer.Tx) error { _, err := c.ApplyResult(tx); return err }

func (c Create) ApplyResult(tx *writer.Tx) ([]byte, error) {
	n, err := name(c.Name)
	if err != nil {
		return nil, err
	}
	var count int
	if err := tx.QueryRow(`SELECT count(*) FROM dashboards WHERE owner = ?`, c.By.Actor.ID).Scan(&count); err != nil {
		return nil, err
	}
	if count >= MaxDashboards {
		return nil, tabs.Refuse("name", fmt.Sprintf("You have %d dashboards. Delete one first.", count))
	}
	if c.ID == "" {
		c.ID = newID()
	}
	at := c.By.At.UnixMilli()
	if _, err := tx.Exec(`INSERT INTO dashboards (id, owner, name, created_at, updated_at) VALUES (?, ?, ?, ?, ?) ON CONFLICT (id) DO NOTHING`,
		c.ID, c.By.Actor.ID, n, at, at); err != nil {
		return nil, err
	}
	tx.Touch(ListTopic, Topic(c.ID))
	return json.Marshal(map[string]string{"id": c.ID})
}

// Rename renames a dashboard: its owner does.
type Rename struct {
	By        tabs.By
	Dashboard string
	Name      string
}

func (c Rename) Validate() error { return checkBy(c.By) }

func (c Rename) Apply(tx *writer.Tx) error {
	if err := own(tx, c.By, c.Dashboard); err != nil {
		return err
	}
	n, err := name(c.Name)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE dashboards SET name = ? WHERE id = ?`, n, c.Dashboard); err != nil {
		return err
	}
	return touched(tx, c.Dashboard, c.By.At)
}

// Delete deletes a dashboard with its gadgets: its owner does.
type Delete struct {
	By        tabs.By
	Dashboard string
}

func (c Delete) Validate() error { return checkBy(c.By) }

func (c Delete) Apply(tx *writer.Tx) error {
	if err := own(tx, c.By, c.Dashboard); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM dashboard_gadgets WHERE dashboard_id = ?`, c.Dashboard); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM dashboards WHERE id = ?`, c.Dashboard); err != nil {
		return err
	}
	tx.Touch(Topic(c.Dashboard), ListTopic)
	return nil
}

// AddGadget adds a gadget at the end of a dashboard: its owner does. A
// gadget of Status or Created names a project the owner may read, one of
// Filter a saved filter.
type AddGadget struct {
	By          tabs.By
	Dashboard   string
	ID          string // optional
	Kind        Kind
	Project     string
	SavedFilter string
}

func (c AddGadget) Validate() error {
	if c.ID != "" && !ValidID(c.ID) {
		return errors.New("dashboards: a malformed id")
	}
	return checkBy(c.By)
}

func (c AddGadget) Apply(tx *writer.Tx) error {
	if err := own(tx, c.By, c.Dashboard); err != nil {
		return err
	}
	if !c.Kind.Valid() {
		return tabs.Refuse("kind", "Pick a kind of gadget.")
	}
	var count, last int
	if err := tx.QueryRow(`SELECT count(*), coalesce(max(position), 0) FROM dashboard_gadgets WHERE dashboard_id = ?`, c.Dashboard).Scan(&count, &last); err != nil {
		return err
	}
	if count >= MaxGadgets {
		return tabs.Refuse("kind", fmt.Sprintf("A dashboard holds %d gadgets. Remove one first.", MaxGadgets))
	}
	project, filter := "", ""
	switch {
	case c.Kind.NeedsProject():
		if c.Project == "" {
			return tabs.Refuse("project", "Pick a project.")
		}
		var one int
		err := tx.QueryRow(`SELECT 1 FROM projects WHERE id = ?`, c.Project).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return tabs.Refuse("project", "Pick a project.")
		}
		if err != nil {
			return err
		}
		if c.By.Actor.Kind != people.Seed { // the seed's people act in the seed alone (internal/demo)
			if err := access.Require(tx, c.By.Actor, access.Read, access.ProjectScope(c.Project)); errors.Is(err, access.ErrNotFound) {
				return tabs.Refuse("project", "Pick a project.")
			} else if err != nil {
				return err
			}
		}
		project = c.Project
	case c.Kind.NeedsFilter():
		var one int
		err := tx.QueryRow(`SELECT 1 FROM saved_filters WHERE id = ?`, c.SavedFilter).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			return tabs.Refuse("filter", "Pick a saved filter.")
		}
		if err != nil {
			return err
		}
		filter = c.SavedFilter
	}
	if c.ID == "" {
		c.ID = newID()
	}
	if _, err := tx.Exec(`INSERT INTO dashboard_gadgets (id, dashboard_id, kind, position, project_id, filter_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`, c.ID, c.Dashboard, string(c.Kind), last+1, project, filter, c.By.At.UnixMilli()); err != nil {
		return err
	}
	return touched(tx, c.Dashboard, c.By.At)
}

// RemoveGadget takes a gadget off a dashboard: its owner does.
type RemoveGadget struct {
	By        tabs.By
	Dashboard string
	Gadget    string
}

func (c RemoveGadget) Validate() error { return checkBy(c.By) }

func (c RemoveGadget) Apply(tx *writer.Tx) error {
	if err := own(tx, c.By, c.Dashboard); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM dashboard_gadgets WHERE id = ? AND dashboard_id = ?`, c.Gadget, c.Dashboard)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return notFound("gadget", c.Gadget)
	}
	return touched(tx, c.Dashboard, c.By.At)
}

// MoveGadget moves a gadget one place earlier (Earlier) or later in its
// dashboard: its owner does. At either end it stays.
type MoveGadget struct {
	By        tabs.By
	Dashboard string
	Gadget    string
	Earlier   bool
}

func (c MoveGadget) Validate() error { return checkBy(c.By) }

func (c MoveGadget) Apply(tx *writer.Tx) error {
	if err := own(tx, c.By, c.Dashboard); err != nil {
		return err
	}
	var pos int
	err := tx.QueryRow(`SELECT position FROM dashboard_gadgets WHERE id = ? AND dashboard_id = ?`, c.Gadget, c.Dashboard).Scan(&pos)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("gadget", c.Gadget)
	}
	if err != nil {
		return err
	}
	q := `SELECT id, position FROM dashboard_gadgets WHERE dashboard_id = ? AND position > ? ORDER BY position LIMIT 1`
	if c.Earlier {
		q = `SELECT id, position FROM dashboard_gadgets WHERE dashboard_id = ? AND position < ? ORDER BY position DESC LIMIT 1`
	}
	var other string
	var opos int
	err = tx.QueryRow(q, c.Dashboard, pos).Scan(&other, &opos)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE dashboard_gadgets SET position = ? WHERE id = ?`, opos, c.Gadget); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE dashboard_gadgets SET position = ? WHERE id = ?`, pos, other); err != nil {
		return err
	}
	return touched(tx, c.Dashboard, c.By.At)
}

// List is every dashboard, by name, with its count of gadgets.
func List(ctx context.Context, q sqlite.Querier) ([]Dashboard, error) {
	rows, err := q.QueryContext(ctx, `SELECT d.id, d.owner, d.name, d.created_at, d.updated_at,
		(SELECT count(*) FROM dashboard_gadgets g WHERE g.dashboard_id = d.id)
		FROM dashboards d ORDER BY d.name COLLATE NOCASE, d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dashboard
	for rows.Next() {
		var d Dashboard
		var created, updated int64
		if err := rows.Scan(&d.ID, &d.Owner, &d.Name, &created, &updated, &d.Count); err != nil {
			return nil, err
		}
		d.Created, d.Updated = time.UnixMilli(created), time.UnixMilli(updated)
		out = append(out, d)
	}
	return out, rows.Err()
}

// Get is a dashboard with its gadgets, access.ErrNotFound for none.
func Get(ctx context.Context, q sqlite.Querier, id string) (Dashboard, error) {
	var d Dashboard
	if !ValidID(id) {
		return d, notFound("dashboard", id)
	}
	var created, updated int64
	err := q.QueryRowContext(ctx, `SELECT id, owner, name, created_at, updated_at FROM dashboards WHERE id = ?`, id).
		Scan(&d.ID, &d.Owner, &d.Name, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return d, notFound("dashboard", id)
	}
	if err != nil {
		return d, err
	}
	d.Created, d.Updated = time.UnixMilli(created), time.UnixMilli(updated)
	rows, err := q.QueryContext(ctx, `SELECT id, kind, position, project_id, filter_id FROM dashboard_gadgets WHERE dashboard_id = ? ORDER BY position, id`, id)
	if err != nil {
		return d, err
	}
	defer rows.Close()
	for rows.Next() {
		var g Gadget
		if err := rows.Scan(&g.ID, &g.Kind, &g.Position, &g.Project, &g.SavedFilter); err != nil {
			return d, err
		}
		d.Gadgets = append(d.Gadgets, g)
	}
	d.Count = len(d.Gadgets)
	return d, rows.Err()
}
