package wiki

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/zweiundeins/lith-coedit/collab"
	"github.com/zweiundeins/prosemirror-go"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/docnodes"
	"github.com/zweiundeins/tickit/internal/tracker"
)

func (e *env) inline(page string) []InlineComment {
	e.t.Helper()
	var list []InlineComment
	e.View(e.t, func(tx *sql.Tx) (err error) { list, err = InlineComments(context.Background(), tx, page); return err })
	return list
}

func (e *env) highlights(page string) []prosemirror.Highlight {
	e.t.Helper()
	var list []prosemirror.Highlight
	e.View(e.t, func(tx *sql.Tx) (err error) { list, err = Highlights(context.Background(), tx, page); return err })
	return list
}

// FR-91.1: a person comments on a word; the comment lists with its text,
// apart from the page's comments, and the word is a highlight. A range
// made on an older version maps over the commits since.
func TestInlineComment(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", false)
	p := e.page(member, CreatePage{Space: sp, Title: "Notes", Doc: paras("Say hello to the team")})
	ds := e.docState(p)
	// "hello" is 5 to 10, "team" 18 to 22.
	add := func(c AddInlineComment) error {
		if c.By.Actor.IsZero() {
			c.By = e.by(viewer)
		}
		c.Page, c.Epoch = p, ds.Epoch
		if c.Body == nil {
			c.Body = paras("Which hello?")
		}
		return e.do(c)
	}
	e.Touched()
	c := e.result(AddInlineComment{By: e.by(viewer), Page: p, Epoch: ds.Epoch, Base: ds.Version, From: 5, To: 10, Text: "hello", Body: paras("Which hello?")})["id"]
	if got := e.Touched(); !slices.Contains(got, InlineTopic(p)) || !slices.Contains(got, PageTopic(p)) {
		t.Errorf("touched %v", got)
	}
	list := e.inline(p)
	if len(list) != 1 || list[0].ID != c || list[0].Quote != "hello" || list[0].From != 5 || list[0].To != 10 || list[0].Detached || list[0].Author != viewer.ID {
		t.Fatalf("inline %+v", list)
	}
	if hs := e.highlights(p); !slices.Equal(hs, []prosemirror.Highlight{{ID: c, From: 5, To: 10}}) {
		t.Errorf("highlights %v", hs)
	}
	var page []Comment
	e.View(t, func(tx *sql.Tx) (err error) { page, err = Comments(context.Background(), tx, p); return err })
	if len(page) != 0 {
		t.Errorf("the page's comments hold the inline one: %+v", page)
	}

	// Someone types before the text; a comment made before that maps.
	e.commit(member, p, ds.Version, "m1", steps(insert(1, "Now ")))
	team := e.result(AddInlineComment{By: e.by(viewer), Page: p, Epoch: ds.Epoch, Base: ds.Version, From: 18, To: 22, Text: "team", Body: paras("Who?")})["id"]
	list = e.inline(p)
	if len(list) != 2 || list[0].From != 9 || list[0].To != 14 || list[1].ID != team || list[1].From != 22 || list[1].To != 26 || list[1].Quote != "team" {
		t.Fatalf("after typing %+v", list)
	}

	for _, c := range []struct {
		name string
		cmd  AddInlineComment
		msg  string
	}{
		{"another text", AddInlineComment{Base: ds.Version, From: 5, To: 10, Text: "hullo"}, "The text changed while you commented: select it again."},
		{"no text", AddInlineComment{Base: ds.Version + 1, From: 0, To: 1, Text: ""}, "Select some text of the page to comment on."},
		{"beyond the page", AddInlineComment{Base: ds.Version + 1, From: 20, To: 90, Text: "x"}, "Select some text of the page to comment on."},
		{"no body", AddInlineComment{Base: ds.Version, From: 5, To: 10, Text: "hello", Body: paras("")}, "Write something first."},
	} {
		if msg := refusal(t, add(c.cmd)); msg != c.msg {
			t.Errorf("%s: %q", c.name, msg)
		}
	}
	if msg := refusal(t, e.do(AddInlineComment{By: e.by(viewer), Page: p, Epoch: "e-old", Base: ds.Version, From: 5, To: 10, Text: "hello", Body: paras("x")})); msg != "The page was reset since you selected the text: select it again." {
		t.Errorf("another epoch: %q", msg)
	}
	if err := e.do(AddInlineComment{By: e.by(viewer), Page: p, Epoch: ds.Epoch, From: 5, To: 5, Text: "", Body: paras("x")}); err == nil {
		t.Error("an empty range went through")
	}
	if err := add(AddInlineComment{By: e.by(sunny), Base: ds.Version, From: 5, To: 10, Text: "hello"}); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("a guest of a closed space: %v", err)
	}
	if len(e.inline(p)) != 2 {
		t.Error("a refused comment was kept")
	}
}

// FR-91.2: the highlight moves with its text at every commit; a comment
// whose text was deleted is detached and lists after the others.
func TestInlineAnchorsFollow(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	p := e.page(sunny, CreatePage{Space: sp, Title: "Notes", Doc: paras("Say hello to the team")})
	ds := e.docState(p)
	hello := e.result(AddInlineComment{By: e.by(rainy), Page: p, Epoch: ds.Epoch, Base: ds.Version, From: 5, To: 10, Text: "hello", Body: paras("A")})["id"]
	team := e.result(AddInlineComment{By: e.by(rainy), Page: p, Epoch: ds.Epoch, Base: ds.Version, From: 18, To: 22, Text: "team", Body: paras("B")})["id"]
	v := ds.Version
	commit := func(s string) {
		t.Helper()
		if a := e.commit(sunny, p, v, "r"+time.Now().Format("150405.000000000"), steps(s)); a != Applied {
			t.Fatalf("commit %s: %c", s, a)
		}
		v++
	}
	commit(insert(1, "Oh, "))     // before both
	commit(insert(14, " there"))  // at the end of hello: stays outside
	commit(insert(9, "and say ")) // at the start of hello: stays outside
	hs := e.highlights(p)
	if len(hs) != 2 || hs[0].ID != hello || hs[1].ID != team {
		t.Fatalf("highlights %v", hs)
	}
	head := e.headDoc(p)
	for _, h := range hs {
		got, _ := prosemirror.InlineText(head, h.From, h.To)
		if want := map[string]string{hello: "hello", team: "team"}[h.ID]; got != want {
			t.Errorf("%s is on %q, want %q (%s)", h.ID, got, want, docnodes.Text(head))
		}
	}
	// Deleting the word detaches it: InlineTopic tells the views.
	e.Touched()
	commit(`{"stepType":"replace","from":` + strconv.Itoa(hs[0].From) + `,"to":` + strconv.Itoa(hs[0].To) + `}`)
	if got := e.Touched(); !slices.Contains(got, InlineTopic(p)) {
		t.Errorf("detached: touched %v", got)
	}
	list := e.inline(p)
	if len(list) != 2 || list[0].ID != team || list[1].ID != hello || !list[1].Detached || list[0].Detached || list[1].Quote != "hello" {
		t.Fatalf("after the delete %+v", list)
	}
	if hs := e.highlights(p); len(hs) != 1 || hs[0].ID != team {
		t.Errorf("highlights after the delete %v", hs)
	}
	// Typing the word again does not bring it back.
	commit(insert(hs[0].From-5, "hello"))
	if list := e.inline(p); !list[1].Detached {
		t.Error("a detached comment came back")
	}
	// A whole document set at once maps too (CommitWhole).
	e.must(SetDocument{By: e.by(sunny), Page: p, Doc: paras("Another text"), Message: "New"})
	if hs := e.highlights(p); len(hs) != 0 {
		t.Errorf("after the document changed %v", hs)
	}
}

// FR-91.3: resolving hides the highlight and keeps the comment in the
// resolved list; reopening brings both back. Replies, hiding and
// deleting are a page comment's.
func TestInlineResolve(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", false)
	p := e.page(member, CreatePage{Space: sp, Title: "Notes", Doc: paras("Say hello")})
	ds := e.docState(p)
	c := e.result(AddInlineComment{By: e.by(member), Page: p, Epoch: ds.Epoch, Base: ds.Version, From: 5, To: 10, Text: "hello", Body: paras("A")})["id"]
	reply := e.result(AddComment{By: e.by(viewer), Page: p, Parent: c, Body: paras("Agreed")})["id"]
	if list := e.inline(p); len(list) != 1 || len(list[0].Replies) != 1 || list[0].Replies[0].ID != reply {
		t.Fatalf("replies %+v", list)
	}
	e.Touched()
	e.must(ResolveInline{By: e.by(viewer), Comment: c, Resolved: true})
	if got := e.Touched(); !slices.Contains(got, InlineTopic(p)) {
		t.Errorf("resolved: touched %v", got)
	}
	list := e.inline(p)
	if len(list) != 1 || !list[0].Resolved || list[0].ResolvedBy != viewer.ID || list[0].ResolvedAt.IsZero() {
		t.Fatalf("resolved %+v", list)
	}
	if hs := e.highlights(p); len(hs) != 0 {
		t.Errorf("a resolved comment's highlight %v", hs)
	}
	e.must(ResolveInline{By: e.by(viewer), Comment: c, Resolved: true}) // again: nothing
	e.must(ResolveInline{By: e.by(member), Comment: c, Resolved: false})
	if hs := e.highlights(p); len(hs) != 1 {
		t.Errorf("reopened: %v", hs)
	}
	if err := e.do(ResolveInline{By: e.by(sunny), Comment: c, Resolved: true}); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("a guest of a closed space: %v", err)
	}
	if err := e.do(ResolveInline{By: e.by(member), Comment: reply, Resolved: true}); !errors.Is(err, access.ErrNotFound) {
		t.Errorf("a reply is no inline comment: %v", err)
	}
	e.must(tracker.HideComment{By: e.by(admin), Comment: c, Hidden: true})
	if hs := e.highlights(p); len(hs) != 0 {
		t.Errorf("a hidden comment's highlight %v", hs)
	}
	if list := e.inline(p); len(list) != 1 || !list[0].Hidden {
		t.Errorf("hidden %+v", list)
	}
	e.must(tracker.HideComment{By: e.by(admin), Comment: c, Hidden: false})
	e.must(tracker.DeleteComment{By: e.by(viewer), Comment: reply})
	e.must(tracker.DeleteComment{By: e.by(member), Comment: c})
	if list := e.inline(p); len(list) != 0 {
		t.Errorf("a deleted comment without replies listed: %+v", list)
	}
	if hs := e.highlights(p); len(hs) != 0 {
		t.Errorf("a deleted comment's highlight %v", hs)
	}
}

// A range made on a version the fold removed is refused: the person
// selects again.
func TestInlineAfterFold(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	p := e.page(sunny, CreatePage{Space: sp, Title: "Notes", Doc: paras("Say hello")})
	ds := e.docState(p)
	e.commit(sunny, p, ds.Version, "f1", steps(insert(1, "x")))
	e.must(CutVersion{By: e.by(sunny), Page: p})
	e.must(&Fold{At: e.now.Add(time.Hour)})
	collab.ResetCaches() // the commit cache still maps over a fold
	if msg := refusal(t, e.do(AddInlineComment{By: e.by(rainy), Page: p, Epoch: ds.Epoch, Base: ds.Version, From: 5, To: 10, Text: "hello", Body: paras("A")})); msg != "The page changed too much since you selected the text: select it again." {
		t.Errorf("folded: %q", msg)
	}
	ds = e.docState(p)
	e.result(AddInlineComment{By: e.by(rainy), Page: p, Epoch: ds.Epoch, Base: ds.Version, From: 6, To: 11, Text: "hello", Body: paras("A")})
}

func (e *env) headDoc(page string) prosemirror.Doc {
	e.t.Helper()
	d, err := docnodes.ParseDoc([]byte(e.head(page)))
	if err != nil {
		e.t.Fatal(err)
	}
	return d
}
