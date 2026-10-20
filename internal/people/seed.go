package people

import (
	"time"

	"github.com/zweiundeins/lith/writer"
)

// Seeded are the six people of the demo's history (DM-02.3): the staff of
// Alpenrose Outfitters, a fictional maker of outdoor gear. They act in the
// seed's history and cannot log in: they have no accounts.
var Seeded = []Person{
	{ID: "s-ada", Kind: Seed, Name: "Ada Brunner", Hue: 22, Role: NoRole},
	{ID: "s-bruno", Kind: Seed, Name: "Bruno Keller", Hue: 200, Role: NoRole},
	{ID: "s-chiara", Kind: Seed, Name: "Chiara Rossi", Hue: 320, Role: NoRole},
	{ID: "s-david", Kind: Seed, Name: "David Meier", Hue: 140, Role: NoRole},
	{ID: "s-elif", Kind: Seed, Name: "Elif Yilmaz", Hue: 265, Role: NoRole},
	{ID: "s-fabian", Kind: Seed, Name: "Fabian Weber", Hue: 75, Role: NoRole},
}

func init() {
	for i := range Seeded {
		Seeded[i].Face = avatarOf(Seeded[i].ID).Face
	}
}

// SeedPeople inserts the seeded people, or sets them back to their names
// and hues: the seed runs it before anything that names them.
type SeedPeople struct{ At time.Time }

func (c SeedPeople) Apply(tx *writer.Tx) error {
	for _, p := range Seeded {
		if _, err := tx.Exec(`INSERT INTO people (id, kind, display_name, hue, created_at, last_seen_at) VALUES (?, 'seed', ?, ?, ?, ?)
			ON CONFLICT (id) DO UPDATE SET kind = 'seed', display_name = excluded.display_name, hue = excluded.hue`,
			p.ID, p.Name, p.Hue, c.At.Unix(), c.At.Unix()); err != nil {
			return err
		}
	}
	tx.Touch(Topic)
	return nil
}
