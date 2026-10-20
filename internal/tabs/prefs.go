package tabs

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"github.com/zweiundeins/lith/live"
	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/sse"
	"github.com/zweiundeins/lith/writer"
)

// Prefs are a session's display preferences, in its row of prefs: they
// outlive a page, and the document draws them from its first byte. tickit
// sets no cookie for them (NFR-31). A session without a row has the
// defaults.
type Prefs struct {
	Theme   string `json:"theme"`   // "system" (the default), "dark", "light"
	Style   string `json:"style"`   // "pixel" (the default), "smooth"
	Density string `json:"density"` // "comfortable" (the default), "compact"
	// HideDemo: the demo notice was dismissed (DM-10.1).
	HideDemo bool `json:"hideDemo"`
	// HideHint: the first-visit hint was dismissed (DM-11).
	HideHint bool `json:"hideHint"`
}

// PrefChoices are the preferences SetPref sets, with their values, the
// default first.
var PrefChoices = map[string][]string{
	"theme":   {"system", "dark", "light"},
	"style":   {"pixel", "smooth"},
	"density": {"comfortable", "compact"},
	"demo":    {"shown", "hidden"},
	"hint":    {"shown", "hidden"},
}

// Normal returns p with unknown values replaced by the defaults.
func (p Prefs) Normal() Prefs {
	for key, v := range map[string]*string{"theme": &p.Theme, "style": &p.Style, "density": &p.Density} {
		if !slices.Contains(PrefChoices[key], *v) {
			*v = PrefChoices[key][0]
		}
	}
	return p
}

// set sets the preference key to value, as PrefChoices name them.
func (p *Prefs) set(key, value string) {
	switch key {
	case "theme":
		p.Theme = value
	case "style":
		p.Style = value
	case "density":
		p.Density = value
	case "demo":
		p.HideDemo = value == "hidden"
	case "hint":
		p.HideHint = value == "hidden"
	}
}

// PrefsTopic is touched by every change to a session's preferences; the
// prefs view of each of its tabs lists it. It names the session by a hash.
func PrefsTopic(sid string) string {
	sum := sha256.Sum256([]byte("prefs:" + sid))
	return "prefs:" + hex.EncodeToString(sum[:12])
}

// GetPrefs reads a session's preferences: the defaults without a row.
func GetPrefs(ctx context.Context, q sqlite.Querier, sid string) (Prefs, error) {
	var data string
	err := q.QueryRowContext(ctx, `SELECT data FROM prefs WHERE sid = ?`, sid).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return Prefs{}.Normal(), nil
	}
	if err != nil {
		return Prefs{}, err
	}
	var p Prefs
	if err := json.Unmarshal([]byte(data), &p); err != nil {
		return Prefs{}, err
	}
	return p.Normal(), nil
}

// SetPref sets one preference of a session (POST /cmd/prefs, in Act) and
// touches the session's PrefsTopic, so every tab of it follows.
type SetPref struct {
	Session    string
	Key, Value string
	At         time.Time
}

func (c SetPref) Validate() error {
	if c.Session == "" || c.At.IsZero() {
		return errors.New("a preference needs a session and a time")
	}
	if !slices.Contains(PrefChoices[c.Key], c.Value) {
		return errors.New("no such preference")
	}
	return nil
}

func (c SetPref) Apply(tx *writer.Tx) error {
	p, err := GetPrefs(tx.Context(), tx, c.Session)
	if err != nil {
		return err
	}
	p.set(c.Key, c.Value)
	data, err := json.Marshal(p)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO prefs (sid, data, seen) VALUES (?, ?, ?)
		ON CONFLICT (sid) DO UPDATE SET data = excluded.data, seen = excluded.seen`, c.Session, string(data), c.At.Unix()); err != nil {
		return err
	}
	tx.Touch(PrefsTopic(c.Session))
	return nil
}

// PrefsView patches the signal _prefs of a tab from its session's row:
// the document applies it to <html> and the display menu. Its key holds
// the tab id and it closes over the session, as View does.
func PrefsView(sid, tab string) live.View {
	return live.View{
		Key:    "prefs:" + tab,
		Topics: []string{PrefsTopic(sid)},
		Render: func(ctx context.Context, tx *sql.Tx) (any, error) {
			var q sqlite.Querier = tx
			if wq, ok := sqlite.QuerierFrom(ctx); ok {
				q = wq
			}
			p, err := GetPrefs(ctx, q, sid)
			if err != nil {
				return nil, err
			}
			b, err := json.Marshal(map[string]Prefs{"_prefs": p})
			if err != nil {
				return nil, err
			}
			return sse.PatchSignals(b, false), nil
		},
	}
}
