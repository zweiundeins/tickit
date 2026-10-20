package people

import (
	"time"

	"github.com/zweiundeins/lith/writer"
)

// Seeded are the six people of the demo's history (DM-02.3): the staff of
// Alpenglow, a fictional Swiss shop for outdoor gear (internal/demo/seed's
// company.json, whose hue index times 22.5 gives these degrees). They act in the
// seed's history and cannot log in: they have no accounts.
var Seeded = []Person{
	{ID: "s-priya", Kind: Seed, Name: "Priya Raman", Hue: 248, Role: NoRole},
	{ID: "s-tomasz", Kind: Seed, Name: "Tomasz Nowak", Hue: 68, Role: NoRole},
	{ID: "s-mateo", Kind: Seed, Name: "Mateo Silva", Hue: 135, Role: NoRole},
	{ID: "s-yuki", Kind: Seed, Name: "Yuki Tanaka", Hue: 203, Role: NoRole},
	{ID: "s-amara", Kind: Seed, Name: "Amara Okafor", Hue: 315, Role: NoRole},
	{ID: "s-lena", Kind: Seed, Name: "Lena Brunner", Hue: 23, Role: NoRole},
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
