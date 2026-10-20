package notify

import (
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/people"
)

// MergeWindow is how close in time several changes to one subject by one
// person become one notification (FR-113.2).
const MergeWindow = time.Minute

// item is one notification a reaction makes.
type item struct {
	Recipient string
	Kind      Kind
	Text      string
	Detail    Detail // for the kinds of watchers: what it adds
}

// reaction is the command of one event: the notifications it makes,
// worked out in the writer from what the log held at the event's
// position, so that a retry or a replay decides the same.
type reaction struct {
	Position int64
	At       time.Time
	Actor    string
	// Build reads what the notifications need and returns their subject
	// and the items, each recipient's first one counting.
	Build func(tx *writer.Tx) (Subject, []item, error)
}

func (r reaction) Apply(tx *writer.Tx) error {
	subject, items, err := r.Build(tx)
	if err != nil || len(items) == 0 {
		return err
	}
	told := map[string]bool{}
	for _, it := range items {
		if it.Recipient == r.Actor || told[it.Recipient] || !notifiable(it.Recipient) {
			continue
		}
		told[it.Recipient] = true
		if err := r.put(tx, subject, it); err != nil {
			return err
		}
		tx.Touch(people.PersonTopic(it.Recipient))
	}
	return nil
}

// put stores a notification, or merges it into the recipient's unread
// one of the same subject by the same actor from within MergeWindow.
func (r reaction) put(tx *writer.Tx, s Subject, it item) error {
	at := r.At.UnixMilli()
	if slices.Contains(watching, it.Kind) {
		var id, last int64
		var detail string
		err := tx.QueryRow(`SELECT id, detail, last_source FROM notifications
			WHERE recipient = ? AND subject_id = ? AND actor = ? AND kind IN ('commented', 'status_changed', 'page_commented')
			AND read_at IS NULL AND dismissed_at IS NULL AND created_at >= ? AND created_at <= ?
			ORDER BY id DESC LIMIT 1`, it.Recipient, s.ID, r.Actor, at-MergeWindow.Milliseconds(), at).Scan(&id, &detail, &last)
		switch {
		case err == nil:
			if last >= r.Position {
				return nil // merged before: a replay
			}
			var d Detail
			if err := json.Unmarshal([]byte(detail), &d); err != nil {
				return err
			}
			if it.Detail.Status != "" {
				d.Status = it.Detail.Status
			}
			d.Comments += it.Detail.Comments
			b, _ := json.Marshal(d)
			text := it.Text
			_, err := tx.Exec(`UPDATE notifications SET detail = ?, text = CASE WHEN ? != '' THEN ? ELSE text END,
				updated_at = max(updated_at, ?), last_source = ? WHERE id = ?`, string(b), text, text, at, r.Position, id)
			return err
		case !errors.Is(err, sql.ErrNoRows):
			return err
		}
	}
	b, _ := json.Marshal(it.Detail)
	_, err := tx.Exec(`INSERT INTO notifications (recipient, kind, subject_kind, subject_id, subject_label, href, actor, text, detail,
		source, last_source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (recipient, source, kind) DO NOTHING`,
		it.Recipient, string(it.Kind), s.Kind, s.ID, s.Label, s.Href, r.Actor, it.Text, string(b), r.Position, r.Position, at, at)
	return err
}

// mentions are the items of the people a text mentions for the first
// time, each with the sentence around their mention.
func mentions(ids []string, doc json.RawMessage) []item {
	var out []item
	for _, id := range ids {
		out = append(out, item{Recipient: id, Kind: Mentioned, Text: MentionSentence(doc, id)})
	}
	return out
}

// watchers are the items of the people who watch the subject.
func watchers(ids []string, kind Kind, text string, d Detail) []item {
	var out []item
	for _, id := range ids {
		out = append(out, item{Recipient: id, Kind: kind, Text: text, Detail: d})
	}
	return out
}
