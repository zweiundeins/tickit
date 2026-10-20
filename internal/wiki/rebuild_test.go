package wiki

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/tracker"
)

func (e *env) dump(table string) string {
	e.t.Helper()
	cols := "*"
	if table == "page_doc" { // a fold moves the checkpoint: the head documents are compared instead
		cols = "page_id, epoch, version, ref, bytes, blocks, last_at"
	}
	rows, err := e.DB.R.Query(`SELECT ` + cols + ` FROM ` + table + ` ORDER BY 1, 2`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	names, _ := rows.Columns()
	var b strings.Builder
	for rows.Next() {
		vals := make([]any, len(names))
		ptrs := make([]any, len(names))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			e.t.Fatal(err)
		}
		for i, v := range vals {
			if s, ok := v.([]byte); ok {
				v = string(s)
			}
			fmt.Fprintf(&b, "%s=%v ", names[i], v)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

var wikiTables = []string{"spaces", "pages", "page_labels", "page_watchers", "page_versions", "page_doc", "page_chips", "page_links", "page_comments", "search_pages"}

// FR-155.2: rebuilding every read model of the wiki from the stored
// events yields the same tables, after a fold too. FR-155.1: every event
// has its actor, time and op id.
func TestRebuild(t *testing.T) {
	e := newEnv(t)
	sp, home := e.space("ENG", true)
	ops, opsHome := e.space("OPS", true)
	proj := e.result(tracker.CreateProject{By: e.by(admin), Key: "WEB", Name: "Web", Open: true})["id"]
	e.result(tracker.CreateIssue{By: e.by(sunny), Project: proj, Type: tracker.Bug, Summary: "Fix the login"})
	a := e.page(sunny, CreatePage{Space: sp, Parent: home, Title: "A", Doc: paras("Alpha")})
	b := e.page(rainy, CreatePage{Space: sp, Parent: a, Title: "B"})
	c := e.page(rainy, CreatePage{Space: sp, Parent: home, Title: "C"})
	for i := range 120 { // past two checkpoints
		e.commit(sunny, a, int64(i+1), fmt.Sprintf("a%d", i), steps(insert(1, "x")))
	}
	chip := `{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"issue_chip","attrs":{"key":"WEB-1"}}]}}`
	link := `{"stepType":"replace","from":1,"to":1,"slice":{"content":[{"type":"page_link","attrs":{"pageId":"` + a + `"}}]}}`
	e.commit(rainy, b, 1, "b1", steps(chip, link))
	e.must(CutVersion{By: e.by(sunny), Page: a, Message: "Long"})
	e.must(RenamePage{By: e.by(rainy), Page: b, Title: "Bee"})
	e.must(SetPageLabels{By: e.by(rainy), Page: b, Add: []string{"draft", "ops"}})
	e.must(SetPageLabels{By: e.by(rainy), Page: b, Remove: []string{"draft"}})
	e.must(WatchPage{By: e.by(sunny), Page: b, Watch: true})
	e.must(MovePage{By: e.by(rainy), Page: c, Space: ops, Parent: opsHome})
	e.must(TrashPage{By: e.by(sunny), Page: a})
	e.must(RestorePage{By: e.by(admin), Page: a})
	d := e.page(rainy, CreatePage{Space: sp, Title: "D"})
	e.must(TrashPage{By: e.by(rainy), Page: d})
	e.must(DeletePage{By: e.by(admin), Page: d})
	e.must(RestoreVersion{By: e.by(rainy), Page: a, Number: 1})
	e.must(SetDocument{By: e.by(ada), Page: c, Doc: paras("Seeded", "Twice"), Message: "From the seed"})
	cm := e.result(AddComment{By: e.by(sunny), Page: b, Body: paras("First")})["id"]
	e.result(AddComment{By: e.by(rainy), Page: b, Parent: cm, Body: paras("Reply")})
	e.must(EditComment{By: e.by(sunny), Comment: cm, Body: paras("First, edited")})
	e.must(HideComment{By: e.by(moderator), Comment: cm, Hidden: true})
	e.must(HideComment{By: e.by(moderator), Comment: cm, Hidden: false})
	e.must(DeleteComment{By: e.by(sunny), Comment: cm})
	name := "Engineering"
	e.must(EditSpace{By: e.by(admin), Space: sp, Name: &name})
	e.must(ArchiveSpace{By: e.by(admin), Space: ops, Archived: true})
	e.must(ArchiveSpace{By: e.by(admin), Space: ops, Archived: false})
	e.must(&Fold{At: e.now.Add(time.Hour)})
	e.commit(sunny, a, e.docState(a).Version, "after-fold", steps(insert(1, "y")))

	before := map[string]string{}
	for _, tb := range wikiTables {
		before[tb] = e.dump(tb)
		if before[tb] == "" {
			t.Errorf("%s is empty: the test covers too little", tb)
		}
	}
	heads := map[string]string{}
	for _, p := range []string{a, b, c, home} {
		heads[p] = e.head(p)
	}
	docs.reset()
	commits.reset()
	e.must(writer.Func(func(tx *writer.Tx) error {
		if _, err := tx.Exec(`UPDATE lith_projections SET version = version + 100 WHERE name LIKE 'wiki_%' OR name = 'search_pages'`); err != nil {
			return err
		}
		for _, tb := range wikiTables {
			if _, err := tx.Exec(`DELETE FROM ` + tb); err != nil {
				return err
			}
		}
		return nil
	}))
	r := e.Run(t, Store())
	for deadline := time.Now().Add(10 * time.Second); ; {
		ps, live := r.Stats().Projectors, true
		for _, p := range ps {
			live = live && p.Status == "live"
		}
		if live && len(ps) == len(Parts())+len(tracker.Parts()) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not rebuilt: %+v", ps)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, tb := range wikiTables {
		if got := e.dump(tb); got != before[tb] {
			t.Errorf("%s rebuilt differs:\n%s\nwas\n%s", tb, got, before[tb])
		}
	}
	docs.reset()
	commits.reset()
	for p, want := range heads {
		if got := e.head(p); got != want {
			t.Errorf("head of %s rebuilt %s, was %s", p, got, want)
		}
	}
	var missing int
	e.View(t, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM lith_events WHERE stream IN ('space', 'page', 'pagedoc', 'page_comment')
			AND (actor IS NULL OR actor = '' OR causation IS NULL OR at = 0)`).Scan(&missing)
	})
	if missing != 0 {
		t.Errorf("FR-155.1: %d events lack an actor, an op or a time", missing)
	}
}

// MP-13: a reset starts a page's document anew, with another epoch, and a
// commit made before it is refused.
func TestReset(t *testing.T) {
	e := newEnv(t)
	sp, _ := e.space("ENG", true)
	p := e.page(sunny, CreatePage{Space: sp, Title: "Notes"})
	old := e.docState(p).Epoch
	e.commit(sunny, p, 1, "s1", steps(insert(1, "Mine")))
	e.must(writer.Func(func(tx *writer.Tx) error {
		if _, err := events.Forget(Store(), tx, PageDocStream.ID(p)); err != nil {
			return err
		}
		return events.Execute(Store(), tx, createDocDecider, PageDocStream.ID(p), events.NoStream, DocCreated{Epoch: newEpoch(), Doc: []byte(blank)}, meta(e.by(ada)))
	}))
	ds := e.docState(p)
	if ds.Epoch == old || ds.Version != 1 {
		t.Fatalf("after the reset %+v", ds)
	}
	c := &Commit{By: e.by(sunny), Session: "s", Tab: "t", Page: p, Epoch: old, Base: 2, Ref: "s2", Steps: []byte(steps(insert(1, "x")))}
	op := writer.OnceResult("commit:"+p+":s2", c)
	e.must(op)
	if CommitAnswer(op.Result()[0]) != Stale {
		t.Errorf("MP-13.2: a commit from before the reset: %c", op.Result()[0])
	}
	if got := e.head(p); got != blank {
		t.Errorf("head %s", got)
	}
}
