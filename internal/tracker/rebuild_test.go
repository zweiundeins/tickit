package tracker

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/collab"
)

// dump is every row of a table, in order, as text.
func (e *env) dump(table string) string {
	e.t.Helper()
	sel := "*"
	if table == "issue_doc" { // a fold moves the checkpoint, and the ref of a head it folded: the head documents are compared instead
		sel = "issue_id, epoch, version, bytes, blocks, last_at"
	}
	rows, err := e.DB.R.Query(`SELECT ` + sel + ` FROM ` + table + ` ORDER BY 1, 2, 3`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer rows.Close()
	cols, _ := rows.Columns()
	var b strings.Builder
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
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
			fmt.Fprintf(&b, "%s=%v ", cols[i], v)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

var projectionTables = []string{"projects", "statuses", "components", "sprints", "issues", "issue_labels", "labels", "issue_links", "issue_watchers", "comments", "issue_doc"}

// FR-155.2: rebuilding every read model from the stored events yields the
// same tables. FR-155.1: every event has its actor, time and op id.
func TestRebuild(t *testing.T) {
	e := newSearch(t)
	web1, web2, web3, web4 := e.ids["WEB-1"], e.ids["WEB-2"], e.ids["WEB-3"], e.ids["WEB-4"]
	epic := e.issue(sunny, CreateIssue{Project: e.web, Type: Epic, Summary: "Epic"})
	e.must(SetParent{By: e.by(sunny), Issue: web2, Seen: 1 << 40, Parent: epic})
	sub := e.issue(sunny, CreateIssue{Project: e.web, Type: Subtask, Parent: web2, Summary: "Sub"})
	e.must(SetType{By: e.by(sunny), Issue: web3, Seen: 1 << 40, Type: Bug})
	e.must(Move{By: e.by(sunny), Issue: web4, Status: StatusInProgress, After: web1, Seen: 1 << 40})
	e.must(Move{By: e.by(sunny), Issue: web1, Before: web2, Seen: 1 << 40})
	e.must(SetDescription{By: e.by(sunny), Issue: web1, Description: PlainDoc("Now with words, a < b & c")})
	// FR-15: a co-edited description, a version of it, its fold.
	for i := range collab.Checkpoint + 3 {
		e.commitDesc(rainy, web2, e.descState(web2).Version, fmt.Sprintf("r%d", i), steps(insert(1, "<&>")))
	}
	e.must(CutDescriptions{At: e.now})
	e.must(&FoldDescriptions{At: e.now.Add(collab.FoldAfter + time.Hour)})
	e.commitDesc(sunny, web2, e.descState(web2).Version, "after-fold", steps(insert(1, "y")))
	e.must(SetDue{By: e.by(sunny), Issue: web1, Seen: 1 << 40, Due: "2026-12-24"})
	e.must(SetPoints{By: e.by(sunny), Issue: web1, Seen: 1 << 40, Points: ptr(8)})
	e.must(SetEstimate{By: e.by(sunny), Issue: web1, Seen: 1 << 40, Estimate: ptr(60)})
	e.must(SetRemaining{By: e.by(sunny), Issue: web1, Seen: 1 << 40, Remaining: ptr(30)})
	e.must(SetSummary{By: e.by(sunny), Issue: web1, Seen: 1 << 40, Summary: "Fix the login for good"})
	e.must(SetLabels{By: e.by(sunny), Issue: web1, Add: []string{"auth"}, Remove: []string{"ux"}})
	e.must(DefineComponent{By: e.by(admin), Project: e.web, Name: "Cart", Description: "The cart"})
	e.must(SetComponents{By: e.by(sunny), Issue: web2, Add: []string{"Cart", "Search"}})
	e.must(RemoveComponent{By: e.by(admin), Project: e.web, Name: "Search"})
	e.must(SetWIPLimit{By: e.by(admin), Project: e.web, Status: StatusInReview, Limit: 2})
	e.must(LinkIssues{By: e.by(sunny), Issue: web1, Type: Blocks, Other: web4})
	e.must(LinkIssues{By: e.by(sunny), Issue: web1, Type: Duplicates, Other: e.ids["APP-1"]})
	e.must(UnlinkIssues{By: e.by(sunny), Issue: web1, Type: Duplicates, Other: e.ids["APP-1"]})
	e.must(Watch{By: e.by(rainy), Issue: web3, Watch: true})
	e.must(Watch{By: e.by(sunny), Issue: web3, Watch: false})
	c := e.result(AddComment{By: e.by(sunny), Issue: web1, Body: PlainDoc("First")})["id"]
	e.must(EditComment{By: e.by(sunny), Comment: c, Body: PlainDoc("First, edited")})
	e.must(HideComment{By: e.by(moderator), Comment: c, Hidden: true, Reason: "spam"})
	e.must(HideComment{By: e.by(moderator), Comment: c, Hidden: false})
	e.must(DeleteComment{By: e.by(sunny), Comment: c})
	sp2 := e.result(CreateSprint{By: e.by(sunny), Project: e.web, Name: "S2", Goal: "More"})["id"]
	e.must(SetSprint{By: e.by(sunny), Issue: web4, Seen: 1 << 40, Sprint: e.sprint})
	e.must(StartSprint{By: e.by(sunny), Project: e.web, Sprint: e.sprint, End: "2026-10-30"})
	e.must(CompleteSprint{By: e.by(sunny), Project: e.web, Sprint: e.sprint, MoveTo: sp2})
	e.must(DeleteIssue{By: e.by(admin), Issue: sub})
	e.must(HideIssue{By: e.by(moderator), Issue: web3, Hidden: true, Reason: "spam"}) // DM-07.1
	e.must(HideIssue{By: e.by(moderator), Issue: web2, Hidden: true})
	e.must(HideIssue{By: e.by(moderator), Issue: web2, Hidden: false})
	e.must(MoveToProject{By: e.by(sunny), Issue: web4, Project: e.app, Status: StatusSelected})
	name, desc := "Web shop", "The storefront"
	e.must(EditProject{By: e.by(admin), Project: e.web, Name: &name, Description: &desc})
	e.must(ArchiveProject{By: e.by(admin), Project: e.arc, Archived: false})

	before := map[string]string{}
	for _, tb := range projectionTables {
		before[tb] = e.dump(tb)
		if before[tb] == "" {
			t.Errorf("%s is empty: the test covers too little", tb)
		}
	}
	head := e.descHead(web2)
	collab.ResetCaches()
	e.must(writer.Func(func(tx *writer.Tx) error {
		if _, err := tx.Exec(`UPDATE lith_projections SET version = version + 100 WHERE name LIKE 'tracker_%'`); err != nil {
			return err
		}
		for _, tb := range projectionTables {
			if _, err := tx.Exec(`DELETE FROM ` + tb); err != nil {
				return err
			}
		}
		return nil
	}))
	r := e.Run(t, Store())
	// Run may return before the runner has read its projectors.
	for deadline := time.Now().Add(10 * time.Second); ; {
		ps, live := r.Stats().Projectors, true
		for _, p := range ps {
			live = live && p.Status == "live"
		}
		if live && len(ps) == len(Parts()) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("not rebuilt: %+v", ps)
		}
		time.Sleep(10 * time.Millisecond)
	}
	for _, tb := range projectionTables {
		if got := e.dump(tb); got != before[tb] {
			t.Errorf("%s rebuilt differs:\n%s\nwas\n%s", tb, got, before[tb])
		}
	}
	collab.ResetCaches()
	if got := e.descHead(web2); got != head {
		t.Errorf("the description rebuilt %q, was %q", got, head)
	}
	var missing int
	e.View(t, func(tx *sql.Tx) error {
		return tx.QueryRow(`SELECT count(*) FROM lith_events WHERE stream IN ('project', 'issue', 'comment', 'issuedoc')
			AND (actor IS NULL OR causation IS NULL OR at = 0)`).Scan(&missing)
	})
	if missing != 0 {
		t.Errorf("FR-155.1: %d events lack an actor, an op or a time", missing)
	}
}
