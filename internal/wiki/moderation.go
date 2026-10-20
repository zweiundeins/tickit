package wiki

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// Moderation of page versions and images (DM-07.1). A moderator or a site
// admin hides a page's version: it leaves the page's history and compare
// for everyone but staff, and nobody else opens or restores it. The newest
// version is the page as it is and is not hidden: restore an older one
// first. A hidden image of the library shows as "Image hidden by a
// moderator" in every page that shows it, from the next frame of their
// readers on, and /img/lib/ no longer serves it.

// VersionHidden hides version Number of a page; VersionUnhidden shows it
// again.
type VersionHidden struct {
	Number int    `json:"number"`
	Reason string `json:"reason,omitempty"`
}
type VersionUnhidden struct {
	Number int `json:"number"`
}

func (VersionHidden) pageEvent()   {}
func (VersionUnhidden) pageEvent() {}

var (
	VersionHiddenType   = definePage[VersionHidden]("page.version_hidden")
	VersionUnhiddenType = definePage[VersionUnhidden]("page.version_unhidden")
)

// HideVersion hides a page's version, or shows it again when Hidden is
// false: a moderator's or a site admin's right (access.Moderate).
type HideVersion struct {
	By     tabs.By
	Page   string
	Number int
	Hidden bool
	Reason string
}

func (c HideVersion) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if c.Number < 1 {
		return errors.New("wiki: no such version")
	}
	return nil
}

var hideVersionDecider = pageDecider(func(c HideVersion, s PageState) ([]PageEvent, error) {
	hidden := slices.Contains(s.HiddenVersions, c.Number)
	switch {
	case c.Number > s.Versions:
		return nil, notFound("version", fmt.Sprint(c.Number))
	case hidden == c.Hidden:
		return nil, nil
	case c.Hidden && c.Number == s.Versions:
		return nil, &writer.Refusal{Msg: "The newest version is the page as it is. Restore an older version first, then hide this one."}
	case c.Hidden:
		reason, ok := text(c.Reason, 0, 200)
		if !ok {
			return nil, tabs.Refuse("reason", "A reason has at most 200 characters.")
		}
		return []PageEvent{VersionHidden{Number: c.Number, Reason: reason}}, nil
	}
	return []PageEvent{VersionUnhidden{Number: c.Number}}, nil
})

func (c HideVersion) Apply(tx *writer.Tx) error {
	if _, err := pageState(tx, c.Page); err != nil {
		return err
	}
	if err := access.Require(tx, c.By.Actor, access.Moderate, access.Scope{}); err != nil {
		return err
	}
	return events.Execute(Store(), tx, hideVersionDecider, PageStream.ID(c.Page), events.Any, c, meta(c.By))
}

func projectVersionHidden(tx *writer.Tx, r events.Record[string, VersionHidden]) error {
	_, err := tx.Exec(`UPDATE page_versions SET hidden_at = ?, hidden_by = ?, hidden_reason = ? WHERE page_id = ? AND number = ?`,
		millis(r.At), r.Actor, r.Data.Reason, r.ID, r.Data.Number)
	tx.Touch(PageTopic(r.ID))
	return err
}

func projectVersionUnhidden(tx *writer.Tx, r events.Record[string, VersionUnhidden]) error {
	_, err := tx.Exec(`UPDATE page_versions SET hidden_at = NULL, hidden_by = '', hidden_reason = '' WHERE page_id = ? AND number = ?`, r.ID, r.Data.Number)
	tx.Touch(PageTopic(r.ID))
	return err
}

// errVersionHidden refuses a hidden version to everyone but staff.
var errVersionHidden = &writer.Refusal{Msg: "A moderator hid this version."}

// HideImage hides an image of the library in every page, or shows it
// again when Hidden is false: a moderator's or a site admin's right. The
// readers of every page render again (ImagesTopic).
type HideImage struct {
	By     tabs.By
	Image  string
	Hidden bool
	Reason string
}

func (c HideImage) Validate() error { return checkBy(c.By) }

func (c HideImage) Apply(tx *writer.Tx) error {
	if err := access.Require(tx, c.By.Actor, access.Moderate, access.Scope{}); err != nil {
		return err
	}
	if !ImageExists(c.Image) {
		return notFound("image", c.Image)
	}
	reason, ok := text(c.Reason, 0, 200)
	if !ok {
		return tabs.Refuse("reason", "A reason has at most 200 characters.")
	}
	var err error
	if c.Hidden {
		_, err = tx.Exec(`INSERT INTO hidden_images (image_id, hidden_at, hidden_by, reason) VALUES (?, ?, ?, ?)
			ON CONFLICT (image_id) DO NOTHING`, c.Image, millis(at(c.By)), c.By.Actor.ID, reason)
	} else {
		_, err = tx.Exec(`DELETE FROM hidden_images WHERE image_id = ?`, c.Image)
	}
	tx.Touch(ImagesTopic, chipsBelow)
	return err
}

// ImagesTopic is touched when an image is hidden or shown again.
const ImagesTopic = "images"

// chipsBelow touches every page's chips topic, which its reader and
// editor views list: a hidden image is in any of them.
const chipsBelow = "chips:*"

// HiddenImages reports which of the ids name a hidden image.
func HiddenImages(ctx context.Context, q sqlite.Querier, ids ...string) (map[string]bool, error) {
	out := map[string]bool{}
	for _, id := range ids {
		var n int
		if err := q.QueryRowContext(ctx, `SELECT count(*) FROM hidden_images WHERE image_id = ?`, id).Scan(&n); err != nil {
			return nil, err
		}
		if n > 0 {
			out[id] = true
		}
	}
	return out, nil
}

// ImageHidden reports whether an image of the library is hidden.
func ImageHidden(ctx context.Context, q sqlite.Querier, id string) (bool, error) {
	m, err := HiddenImages(ctx, q, id)
	return m[id], err
}

// HiddenImage is an image a moderator hid, for the moderation page.
type HiddenImage struct {
	ID, By, Reason string
	At             time.Time
}

// HiddenImageList lists the hidden images, the newest first.
func HiddenImageList(ctx context.Context, q sqlite.Querier, limit int) ([]HiddenImage, error) {
	rows, err := q.QueryContext(ctx, `SELECT image_id, hidden_by, reason, hidden_at FROM hidden_images ORDER BY hidden_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HiddenImage
	for rows.Next() {
		var h HiddenImage
		var at int64
		if err := rows.Scan(&h.ID, &h.By, &h.Reason, &at); err != nil {
			return nil, err
		}
		h.At = time.UnixMilli(at)
		out = append(out, h)
	}
	return out, rows.Err()
}

// VersionLabel names a page's version for the moderation log.
func VersionLabel(tx *writer.Tx, page string, number int) (string, error) {
	var title string
	err := tx.QueryRow(`SELECT title FROM page_versions WHERE page_id = ? AND number = ?`, page, number).Scan(&title)
	if errors.Is(err, sql.ErrNoRows) {
		return "", notFound("version", fmt.Sprint(number))
	}
	return fmt.Sprintf("%s, version %d", title, number), err
}
