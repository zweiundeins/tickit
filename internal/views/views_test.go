package views

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/ui"
)

// A person looks the same everywhere: people draws a guest's or an
// account's face and hue as ui.FaceOf does, and UIPerson keeps a seeded
// person's own hue.
func TestOneFace(t *testing.T) {
	for i := range 500 {
		p := people.GuestOf(fmt.Sprint("session ", i))
		f := ui.FaceOf(p.ID)
		if p.Face != f.Path || p.Hue != f.Hue || UIPerson(p).HueIndex() != f.Hue {
			t.Fatalf("%s: people %q hue %d, ui %q hue %d", p.ID, p.Face, p.Hue, f.Path, f.Hue)
		}
	}
	for _, p := range people.Seeded {
		if p.Face != ui.FaceOf(p.ID).Path || UIPerson(p).HueIndex() != p.Hue {
			t.Errorf("%s: face or hue differ", p.ID)
		}
	}
}

// MP-01, MP-02: the presence row is shared: every face carries its id and
// the tab count in its name, the ninth and later are overflow, and the
// document's own style puts the viewer's face first.
func TestPresenceRow(t *testing.T) {
	var list []Present
	for i := range 10 {
		list = append(list, Present{Person: people.GuestOf(fmt.Sprint("s", i)), Tabs: 1 + i%3})
	}
	var b bytes.Buffer
	if err := Presence("presence-x", list).Render(context.Background(), &b); err != nil {
		t.Fatal(err)
	}
	row := b.String()
	if strings.Count(row, "data-who=") != 10 || strings.Count(row, "data-overflow") != 2 || !strings.Contains(row, `aria-label="`+list[1].Name+`, 2 tabs"`) ||
		!strings.Contains(row, `<ul id="presence-x" class="tk-presence"`) || !strings.Contains(row, ">+2<") || strings.Contains(row, "data-you") {
		t.Errorf("the row:\n%s", row)
	}
	b.Reset()
	meStyle(list[9].ID).Render(context.Background(), &b)
	if !strings.Contains(b.String(), `li[data-who="`+list[9].ID+`"]{order:-1`) {
		t.Errorf("the own face: %s", b.String())
	}
}
