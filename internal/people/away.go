package people

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"
)

// Away (MP-02.4): a person who has given no input in any tab for
// AwayAfter shows as away in presence rows. Pages report input with a
// cheap action at most every InputEvery while there is some (GET never
// writes), into presence_input (migration 081); a presence row reads it
// for the faces it shows. The topic AwayTopic(person) is touched when the
// person comes back, by Input, and when they turn away, by a sweep that
// the server runs every few seconds (web.Server.PresenceJobs), so that
// only the rows that show them render again.

// AwayAfter is how long without input makes a person away.
const AwayAfter = 5 * time.Minute

// InputEvery is how often a page reports input at most; Input writes
// nothing for a report within half of it.
const InputEvery = time.Minute

// AwayTopic is touched when the person turns away or comes back.
func AwayTopic(person string) string { return "away:" + person }

// Input records that the person gave input at At.
type Input struct {
	Person string
	At     time.Time
}

func (c Input) Validate() error {
	if !ValidID(c.Person) || c.At.IsZero() {
		return errors.New("people: input needs a person and a time")
	}
	return nil
}

func (c Input) Apply(tx *writer.Tx) error {
	at := c.At.Unix()
	var last int64
	err := tx.QueryRow(`SELECT at FROM presence_input WHERE person_id = ?`, c.Person).Scan(&last)
	switch {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return err
	case at-last < int64(InputEvery/time.Second)/2:
		return nil
	}
	if _, err := tx.Exec(`INSERT INTO presence_input (person_id, at) VALUES (?, ?)
		ON CONFLICT (person_id) DO UPDATE SET at = max(at, excluded.at)`, c.Person, at); err != nil {
		return err
	}
	if last > 0 && at-last >= int64(AwayAfter/time.Second) {
		tx.Touch(AwayTopic(c.Person)) // back
	}
	return nil
}

// Away reports which of the persons are away at now. A person without a
// report is not: their page reports one as it opens.
func Away(ctx context.Context, q sqlite.Querier, now time.Time, ids ...string) (map[string]bool, error) {
	out := map[string]bool{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := json.Marshal(ids)
	if err != nil {
		return nil, err
	}
	rows, err := q.QueryContext(ctx, `SELECT person_id FROM presence_input WHERE person_id IN (SELECT value FROM json_each(?)) AND at <= ?`,
		string(list), now.Add(-AwayAfter).Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// TurnedAway are the persons who turned away after from and by to: their
// last input came AwayAfter before a time in (from, to].
func TurnedAway(ctx context.Context, q sqlite.Querier, from, to time.Time) ([]string, error) {
	rows, err := q.QueryContext(ctx, `SELECT person_id FROM presence_input WHERE at > ? AND at <= ?`,
		from.Add(-AwayAfter).Unix(), to.Add(-AwayAfter).Unix())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SweepInput deletes the reports older than a day: nobody shows a face
// that long without a page that reports again.
type SweepInput struct{ At time.Time }

func (c SweepInput) Apply(tx *writer.Tx) error {
	_, err := tx.Exec(`DELETE FROM presence_input WHERE at < ?`, c.At.Add(-24*time.Hour).Unix())
	return err
}
