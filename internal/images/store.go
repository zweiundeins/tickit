package images

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// The caps of uploads between two resets, beyond the rate limit of an
// upload (DM-04.1) and the demo's daily 200 images of guests (DM-04.3):
// what one person uploads, and what everyone does together.
const (
	PerPerson      = 20
	PerPersonBytes = 50 << 20
	SiteBytes      = 500 << 20
)

// Topic is touched by every upload, hide and delete of an image: the
// moderation page listens.
const Topic = "images"

// Kinds of subjects an image is uploaded into.
const (
	OnPage  = "page"
	OnIssue = "issue"
)

// Upload stores an image Process made, uploaded into a page's document or
// an issue's description. Gate checks inside the writer that the actor may
// edit the subject now, as a commit to it would. Its result is the stored
// image's {id, width, height}.
type Upload struct {
	By      tabs.By
	ID      string // NewID
	Subject string // OnPage or OnIssue
	On      string // the subject's id
	Image   Image
	Gate    func(tx *writer.Tx) error
}

func (c Upload) Validate() error {
	switch {
	case c.By.Actor.IsZero():
		return errors.New("images: an upload needs an actor")
	case !IsUpload(c.ID), c.Subject != OnPage && c.Subject != OnIssue, c.On == "", len(c.On) > 64, c.Gate == nil:
		return errors.New("images: a malformed upload")
	case c.Image.Type != PNG && c.Image.Type != JPEG, len(c.Image.Data) == 0, len(c.Image.Data) > MaxBytes:
		return errors.New("images: an upload holds an image Process made")
	}
	return nil
}

func (c Upload) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

// Result is what an upload answers.
type Result struct {
	ID     string `json:"id"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

func (c Upload) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if err := c.Gate(tx); err != nil {
		return nil, err
	}
	var n, mine, all int64
	if err := tx.QueryRow(`SELECT count(*), coalesce(sum(bytes), 0) FROM uploads WHERE created_by = ?`, c.By.Actor.ID).Scan(&n, &mine); err != nil {
		return nil, err
	}
	if err := tx.QueryRow(`SELECT coalesce(sum(bytes), 0) FROM uploads`).Scan(&all); err != nil {
		return nil, err
	}
	size := int64(len(c.Image.Data))
	switch {
	case n >= PerPerson:
		return nil, &writer.Refusal{Msg: fmt.Sprintf("You uploaded %d images since the last reset, the most one person may. Use one of them again or an image of the library.", PerPerson)}
	case mine+size > PerPersonBytes:
		return nil, &writer.Refusal{Msg: fmt.Sprintf("Your uploads since the last reset come to %d MB, the most one person may. Use one of them again or an image of the library.", PerPersonBytes>>20)}
	case all+size > SiteBytes:
		return nil, &writer.Refusal{Msg: "The demo is full until the next reset"}
	}
	at := c.By.At
	if at.IsZero() {
		at = time.Now()
	}
	if _, err := tx.Exec(`INSERT INTO uploads (id, type, width, height, bytes, data, subject_kind, subject_id, created_by, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, c.ID, c.Image.Type, c.Image.Width, c.Image.Height, size, c.Image.Data,
		c.Subject, c.On, c.By.Actor.ID, at.UnixMilli()); err != nil {
		return nil, err
	}
	tx.Touch(Topic)
	return json.Marshal(Result{ID: c.ID, Width: c.Image.Width, Height: c.Image.Height})
}

// Delete deletes an uploaded image's bytes: a moderator's or a site
// admin's right. Hide it first (wiki.HideImage), so that the pages that
// show it say so.
type Delete struct {
	By tabs.By
	ID string
}

func (c Delete) Validate() error {
	if c.By.Actor.IsZero() {
		return errors.New("images: a delete needs an actor")
	}
	return nil
}

func (c Delete) Apply(tx *writer.Tx) error {
	if err := access.Require(tx, c.By.Actor, access.Moderate, access.Scope{}); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM uploads WHERE id = ?`, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("image %s: %w", c.ID, access.ErrNotFound)
	}
	tx.Touch(Topic)
	return nil
}

// Exists reports whether id names an uploaded image.
func Exists(ctx context.Context, q sqlite.Querier, id string) (bool, error) {
	if !IsUpload(id) {
		return false, nil
	}
	var one int
	err := q.QueryRowContext(ctx, `SELECT 1 FROM uploads WHERE id = ?`, id).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// Get reads an uploaded image's media type and bytes; access.ErrNotFound
// for none.
func Get(ctx context.Context, q sqlite.Querier, id string) (typ string, data []byte, err error) {
	if !IsUpload(id) {
		return "", nil, access.ErrNotFound
	}
	err = q.QueryRowContext(ctx, `SELECT type, data FROM uploads WHERE id = ?`, id).Scan(&typ, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("image %s: %w", id, access.ErrNotFound)
	}
	return typ, data, err
}

// Uploaded is an uploaded image as the moderation page lists it.
type Uploaded struct {
	ID            string
	Width, Height int
	Bytes         int64
	Subject, On   string
	By            string
	At            time.Time
}

// Label names an uploaded image for the moderation log.
func (u Uploaded) Label() string {
	return fmt.Sprintf("An uploaded image, %d by %d pixels", u.Width, u.Height)
}

// Newest lists the uploaded images, the newest first.
func Newest(ctx context.Context, q sqlite.Querier, limit int) ([]Uploaded, error) {
	rows, err := q.QueryContext(ctx, `SELECT id, width, height, bytes, subject_kind, subject_id, created_by, created_at
		FROM uploads ORDER BY created_at DESC, id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Uploaded
	for rows.Next() {
		var u Uploaded
		var at int64
		if err := rows.Scan(&u.ID, &u.Width, &u.Height, &u.Bytes, &u.Subject, &u.On, &u.By, &at); err != nil {
			return nil, err
		}
		u.At = time.UnixMilli(at)
		out = append(out, u)
	}
	return out, rows.Err()
}

// One reads an uploaded image's row without its bytes; access.ErrNotFound
// for none.
func One(ctx context.Context, q sqlite.Querier, id string) (Uploaded, error) {
	u := Uploaded{ID: id}
	var at int64
	err := q.QueryRowContext(ctx, `SELECT width, height, bytes, subject_kind, subject_id, created_by, created_at FROM uploads WHERE id = ?`, id).
		Scan(&u.Width, &u.Height, &u.Bytes, &u.Subject, &u.On, &u.By, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return u, fmt.Errorf("image %s: %w", id, access.ErrNotFound)
	}
	u.At = time.UnixMilli(at)
	return u, err
}
