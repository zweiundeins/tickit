package tracker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Saved filters (FR-56) are relational rows, written by these commands:
// in the demo everyone sees every one, and their owners rename and
// delete them.

// SavedFilter is a saved filter as the Filters menu lists it.
type SavedFilter struct {
	ID      string
	Owner   string
	Name    string
	Filter  Filter
	Sort    Sort
	Stars   int
	Starred bool // by the person who asked
}

// SaveFilter saves a filter and a sort under a name. Its result is the
// saved filter's id as JSON: {"id": "..."}.
type SaveFilter struct {
	By     tabs.By
	ID     string // optional
	Name   string
	Filter Filter
	Sort   Sort
}

func (c SaveFilter) Validate() error {
	if c.ID != "" && !ValidID(c.ID) {
		return errors.New("tracker: a malformed filter id")
	}
	return checkBy(c.By)
}

func (c SaveFilter) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func filterName(name string) (string, error) {
	n, ok := text(name, 1, MaxFilterName)
	if !ok {
		return "", tabs.Refuse("name", "A filter's name has 1 to 60 characters.")
	}
	return n, nil
}

func (c SaveFilter) ApplyResult(tx *writer.Tx) ([]byte, error) {
	name, err := filterName(c.Name)
	if err != nil {
		return nil, err
	}
	if c.ID == "" {
		c.ID = NewID()
	}
	v := c.Filter.Normalize("").Values()
	for k, x := range c.Sort.Values() {
		v[k] = x
	}
	when := at(c.By).UnixMilli()
	if _, err := tx.Exec(`INSERT INTO saved_filters (id, owner, name, query, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (id) DO NOTHING`, c.ID, c.By.Actor.ID, name, v.Encode(), when, when); err != nil {
		return nil, err
	}
	tx.Touch(FiltersTopic)
	return json.Marshal(map[string]string{"id": c.ID})
}

// ownFilter refuses a change to a saved filter by anyone but its owner.
func ownFilter(tx *writer.Tx, by tabs.By, id string) error {
	var owner string
	err := tx.QueryRow(`SELECT owner FROM saved_filters WHERE id = ?`, id).Scan(&owner)
	if errors.Is(err, sql.ErrNoRows) {
		return notFound("filter", id)
	}
	if err != nil {
		return err
	}
	if owner != by.Actor.ID && by.Actor.Role != people.Admin {
		return &denial{writer.Refusal{Msg: "Only its owner can change this filter."}}
	}
	return nil
}

// denial is a refusal for want of a right: the action answers 403.
type denial struct{ writer.Refusal }

func (d *denial) Error() string { return d.Msg }
func (d *denial) Unwrap() error { return &d.Refusal }
func (d *denial) Denied() bool  { return true }

// RenameFilter renames a saved filter: its owner does (FR-56.3).
type RenameFilter struct {
	By     tabs.By
	Filter string
	Name   string
}

func (c RenameFilter) Validate() error { return checkBy(c.By) }

func (c RenameFilter) Apply(tx *writer.Tx) error {
	if err := ownFilter(tx, c.By, c.Filter); err != nil {
		return err
	}
	name, err := filterName(c.Name)
	if err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE saved_filters SET name = ?, updated_at = ? WHERE id = ? AND name != ?`, name, at(c.By).UnixMilli(), c.Filter, name)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		tx.Touch(FiltersTopic)
	}
	return nil
}

// DeleteFilter deletes a saved filter: its owner does.
type DeleteFilter struct {
	By     tabs.By
	Filter string
}

func (c DeleteFilter) Validate() error { return checkBy(c.By) }

func (c DeleteFilter) Apply(tx *writer.Tx) error {
	if err := ownFilter(tx, c.By, c.Filter); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM saved_filter_stars WHERE filter_id = ?`, c.Filter); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM saved_filters WHERE id = ?`, c.Filter); err != nil {
		return err
	}
	tx.Touch(FiltersTopic)
	return nil
}

// StarFilter stars a saved filter for the actor, or takes the star off.
type StarFilter struct {
	By     tabs.By
	Filter string
	Star   bool
}

func (c StarFilter) Validate() error { return checkBy(c.By) }

func (c StarFilter) Apply(tx *writer.Tx) error {
	var one int
	if err := tx.QueryRow(`SELECT 1 FROM saved_filters WHERE id = ?`, c.Filter).Scan(&one); errors.Is(err, sql.ErrNoRows) {
		return notFound("filter", c.Filter)
	} else if err != nil {
		return err
	}
	q := `DELETE FROM saved_filter_stars WHERE filter_id = ? AND person_id = ?`
	if c.Star {
		q = `INSERT INTO saved_filter_stars (filter_id, person_id) VALUES (?, ?) ON CONFLICT DO NOTHING`
	}
	res, err := tx.Exec(q, c.Filter, c.By.Actor.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		tx.Touch(FiltersTopic)
	}
	return nil
}

// SavedFilters lists every saved filter, the ones person starred first,
// then by name.
func SavedFilters(ctx context.Context, q sqlite.Querier, person string) ([]SavedFilter, error) {
	rows, err := q.QueryContext(ctx, `SELECT f.id, f.owner, f.name, f.query,
		(SELECT count(*) FROM saved_filter_stars s WHERE s.filter_id = f.id),
		EXISTS (SELECT 1 FROM saved_filter_stars s WHERE s.filter_id = f.id AND s.person_id = ?) AS starred
		FROM saved_filters f ORDER BY starred DESC, f.name COLLATE NOCASE, f.id`, person)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SavedFilter
	for rows.Next() {
		var f SavedFilter
		var query string
		if err := rows.Scan(&f.ID, &f.Owner, &f.Name, &query, &f.Stars, &f.Starred); err != nil {
			return nil, err
		}
		v, _ := url.ParseQuery(query)
		f.Filter, f.Sort = ParseFilter(v), ParseSort(v)
		out = append(out, f)
	}
	return out, rows.Err()
}
