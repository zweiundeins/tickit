package demo_test

import (
	"context"
	"testing"

	"github.com/zweiundeins/tickit/internal/dashboards"
)

// FR-60: the seed holds one dashboard per project, by its lead, with a
// gadget of every kind, so that a visitor sees one at once; its ids are
// the same at every run.
func TestSeedDashboards(t *testing.T) {
	c := content(t)
	e, _ := seeded(t)
	list, err := dashboards.List(context.Background(), e.DB.R)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != len(c.Projects) {
		t.Fatalf("%d dashboards for %d projects", len(list), len(c.Projects))
	}
	leads := map[string]bool{}
	for _, p := range c.Projects {
		leads[p.Lead] = true
	}
	for _, d := range list {
		if d.Count != len(dashboards.Kinds) || !leads[d.Owner] {
			t.Errorf("%s: %d gadgets, owner %s", d.Name, d.Count, d.Owner)
		}
	}
	e2, _ := seeded(t)
	again, _ := dashboards.List(context.Background(), e2.DB.R)
	for i := range list {
		if again[i].ID != list[i].ID {
			t.Errorf("the dashboards' ids differ between runs")
		}
	}
}
