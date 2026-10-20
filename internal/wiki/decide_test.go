package wiki

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/zweiundeins/lith/events/eventstest"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/tabs"
)

func refuse(field, msg string) error { return tabs.Refuse(field, msg) }

type errorString string

func (e errorString) Error() string { return string(e) }

var engCreated = SpaceCreated{Key: "ENG", Name: "Engineering", Home: "h", Open: true}

func TestSpaceDeciders(t *testing.T) {
	eventstest.For(createSpace).When(engCreated).Then(t, engCreated)
	eventstest.For(createSpace).Given(engCreated).When(engCreated).ThenError(t, errorString("wiki: the space exists"))

	eng := eventstest.For(editSpace).Given(engCreated)
	name, same, long, desc := "Eng", "Engineering", strings.Repeat("x", 101), "How we build"
	eng.When(EditSpace{Name: &name}).Then(t, SpaceEdited{Name: &name})
	eng.When(EditSpace{Name: &same}).Then(t)
	eng.When(EditSpace{Name: &long}).ThenError(t, refuse("name", "A name has 1 to 100 characters."))
	eng.When(EditSpace{Description: &desc}).Then(t, SpaceEdited{Description: &desc})
	long = strings.Repeat("y", 2001)
	eng.When(EditSpace{Description: &long}).ThenError(t, refuse("description", "A description has at most 2,000 characters."))
	eventstest.For(editSpace).When(EditSpace{Name: &name}).ThenError(t, errNoSpace)

	arch := eventstest.For(archiveSpace).Given(engCreated)
	arch.When(ArchiveSpace{Archived: true}).Then(t, SpaceArchived{})
	arch.When(ArchiveSpace{Archived: false}).Then(t)
	arch.Given(SpaceArchived{}).When(ArchiveSpace{Archived: false}).Then(t, SpaceUnarchived{})
	eventstest.For(archiveSpace).When(ArchiveSpace{Archived: true}).ThenError(t, errNoSpace)
}

var notesCreated = PageCreated{Space: "s", Parent: "h", Rank: "a0", Title: "Notes", Creator: "g-x"}

func TestPageDeciders(t *testing.T) {
	eventstest.For(createPageDecider).When(notesCreated).Then(t, notesCreated)
	eventstest.For(createPageDecider).Given(notesCreated).When(notesCreated).ThenError(t, errorString("wiki: the page exists"))

	ren := eventstest.For(renamePage).Given(notesCreated)
	ren.When(PageRenamed{To: "Minutes"}).Then(t, PageRenamed{From: "Notes", To: "Minutes"})
	ren.When(PageRenamed{To: "Notes"}).Then(t)

	mv := eventstest.For(movePage).Given(notesCreated)
	mv.When(PageMoved{Space: "s", Parent: "h", Rank: "a1"}).Then(t, PageMoved{Space: "s", FromParent: "h", Parent: "h", Rank: "a1"})
	mv.When(PageMoved{Space: "s", Parent: "h", Rank: "a0"}).Then(t)
	mv.When(PageMoved{Space: "o", Parent: "", Rank: "a0"}).Then(t, PageMoved{FromSpace: "s", Space: "o", FromParent: "h", Rank: "a0"})
	eventstest.For(movePage).When(PageMoved{Space: "s"}).ThenError(t, errorString("wiki: no such page"))

	lab := eventstest.For(setLabels).Given(notesCreated)
	lab.When(SetPageLabels{Add: []string{" Release ", "release", "ops"}}).Then(t, PageLabelsChanged{Added: []string{"release", "ops"}})
	lab.When(SetPageLabels{Remove: []string{"ops"}}).Then(t)
	lab.Given(PageLabelsChanged{Added: []string{"ops"}}).When(SetPageLabels{Remove: []string{"OPS"}}).Then(t, PageLabelsChanged{Removed: []string{"ops"}})
	lab.When(SetPageLabels{Add: []string{"two words"}}).ThenError(t, refuse("labels", "A label has 1 to 50 characters and no spaces."))
	many := make([]string, 21)
	for i := range many {
		many[i] = "l" + strings.Repeat("x", i)
	}
	lab.When(SetPageLabels{Add: many}).ThenError(t, refuse("labels", "A page has at most 20 labels."))

	w := eventstest.For(watchPage).Given(notesCreated)
	w.When(PageWatched{Person: "g-x", Watch: true}).Then(t) // its creator watches from the start (FR-95.1)
	w.When(PageWatched{Person: "g-y", Watch: true}).Then(t, PageWatched{Person: "g-y", Watch: true})
	w.When(PageWatched{Person: "g-x", Watch: false}).Then(t, PageWatched{Person: "g-x", Watch: false})

	tr := eventstest.For(trashPage).Given(notesCreated)
	tr.When(PageTrashed{}).Then(t, PageTrashed{Parent: "h", Rank: "a0"})
	tr.Given(PageTrashed{Parent: "h", Rank: "a0"}).When(PageTrashed{}).Then(t)

	rs := eventstest.For(restorePage).Given(notesCreated)
	rs.When(PageRestored{Parent: "h", Rank: "a0"}).Then(t)
	rs.Given(PageTrashed{Parent: "h", Rank: "a0"}).When(PageRestored{Parent: "h", Rank: "a0"}).Then(t, PageRestored{Parent: "h", Rank: "a0"})

	del := eventstest.For(deletePage).Given(notesCreated)
	del.When(DeletePage{}).ThenError(t, &writer.Refusal{Msg: "Only a page in the trash can be deleted."})
	del.Given(PageTrashed{}).When(DeletePage{}).Then(t, PageDeleted{})

	cut := eventstest.For(cutDecider).Given(notesCreated, VersionCut{Number: 1, Upto: 1, Title: "Notes", Reason: "created"})
	cut.When(VersionCut{Upto: 9, Title: "Notes", Reason: "idle"}).Then(t, VersionCut{Number: 2, Upto: 9, Title: "Notes", Reason: "idle"})
	if s := cut.State(); s.Versions != 1 || s.CutUpto != 1 || s.CutAt != 2 {
		t.Errorf("state after a cut %+v", s)
	}
	eventstest.For(cutDecider).When(VersionCut{}).ThenError(t, errorString("wiki: no such page"))
}

func TestDocDeciders(t *testing.T) {
	created := DocCreated{Epoch: "e1", Doc: json.RawMessage(blank)}
	eventstest.For(createDocDecider).When(created).Then(t, created)
	eventstest.For(createDocDecider).Given(created).When(created).ThenError(t, errorString("wiki: the document exists"))
	c := DocCommitted{Ref: "r1", Client: "c", Base: 1, Steps: json.RawMessage(`[]`)}
	eventstest.For(commitDecider).Given(created).When(c).Then(t, c)
	eventstest.For(commitDecider).When(c).ThenError(t, errStale)
	// The state carries no document, only what a commit decides on.
	s := eventstest.For(commitDecider).Given(created, c).State()
	if s.Epoch != "e1" || s.Version != 2 || s.Ref != "r1" {
		t.Errorf("state %+v", s)
	}
	b, _ := json.Marshal(s)
	if strings.Contains(string(b), "paragraph") {
		t.Errorf("the state row holds the document: %s", b)
	}
	// A fold from the events carries it along, for the snapshot.
	step := `[{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"text","text":"Hi"}]}}]`
	s = evolveDoc(evolveDoc(DocState{}, created), DocCommitted{Ref: "r", Steps: json.RawMessage(step)})
	if got := string(marshalDoc(s.doc)); !strings.Contains(got, `"text":"Hi"`) {
		t.Errorf("folded document %s", got)
	}
}

var _ = errors.New
