package web_test

import (
	"context"
	"database/sql"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// FR-36, FR-37, FR-43: an admin changes the workflow in the settings, and
// the other tabs follow within a tick: a guest's board gets the rules
// and the new columns, its card menu the columns, the issue's status
// menu only the allowed moves, and a forbidden move is refused in words.
func TestWorkflowLive(t *testing.T) {
	e, pr := boardEnv(t, 16)
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	admin := webtest.Client(t)
	e.Login(t, admin, "ada@example.com", "correct horse battery")
	code, page := webtest.Get(t, admin, e.URL+"/projects/WEB/settings")
	if code != http.StatusOK || !strings.Contains(page, `id="`+views.WorkflowID+`"`) || !strings.Contains(page, `data-status="inreview"`) {
		t.Fatalf("the settings show no workflow: %d", code)
	}
	settings := webtest.Listen(t, admin, e.URL+"/stream?tab=adm&v=settings:WEB", nil)
	webtest.Next(t, settings, `id="`+views.WorkflowID+`"`)

	guest, _, board := onBoard(t, e, "g1", "")
	key := pr.Keys[1] // in To Do
	issue := pr.IDs[key]
	_, ipage := webtest.Get(t, guest, e.URL+"/"+key)
	if !strings.Contains(ipage, `data-field="status" data-value="done"`) {
		t.Fatal("the status menu does not offer Done before the rule")
	}
	issueView := webtest.Listen(t, guest, e.URL+"/stream?tab=g2&v=issue:"+issue, nil)
	webtest.Next(t, issueView, `id="issue-main"`)

	post := func(c *http.Client, path, body string) {
		t.Helper()
		if code, msg := webtest.Post(t, c, e.URL+path, body); code != http.StatusNoContent {
			t.Fatalf("%s: %d %s", path, code, msg)
		}
	}
	post(admin, "/cmd/projects/status/rule", `{"tab":"adm","op":"r1","project":"WEB","status":"done","from":["inreview"]}`)
	webtest.Next(t, settings, `"r1":{"ok":true`)
	f := webtest.Next(t, board, `data-rules=`)
	if !strings.Contains(f, `&#34;todo&#34;:[&#34;selected&#34;,&#34;todo&#34;,&#34;inprogress&#34;,&#34;inreview&#34;,&#34;blocked&#34;]`) {
		t.Errorf("the board's rules: %.400s", f[strings.Index(f, "data-rules="):])
	}
	f = webtest.Next(t, issueView, `id="issue-main"`)
	if strings.Contains(f, `data-field="status" data-value="done"`) || !strings.Contains(f, `data-field="status" data-value="inreview"`) {
		t.Error("FR-37.1: the issue's menu still offers Done")
	}
	// FR-37.1, FR-41.3: the board refuses the move in words.
	post(guest, "/cmd/board/move", `{"tab":"g1","op":"m1","issue":"`+key+`","column":"done","before":"","after":"","seen":1}`)
	webtest.Next(t, board, `"m1":{"ok":false,"msg":"Done can only follow In Review.","field":"move"}`)
	// FR-58: the menu's change goes through the same decider.
	post(guest, "/cmd/issues/"+issue+"/status", `{"tab":"g2","op":"s1","seen":1,"value":"done"}`)
	webtest.Next(t, issueView, `"s1":{"ok":false,"msg":"Done can only follow In Review."`)

	// FR-36.1, FR-36.3: a new status with its column, live on the board's
	// lanes, its card menu and the settings.
	post(admin, "/cmd/projects/status", `{"tab":"adm","op":"a1","project":"WEB","name":"In QA","category":"inprogress"}`)
	webtest.Next(t, settings, `data-status="in-qa"`)
	webtest.Next(t, board, `id="card-menu-lanes"`) // the head's frame
	// FR-36.2: a removal moves the status's issues, in their history.
	blocked := ""
	for _, k := range pr.Keys {
		var st string
		e.DB.R.QueryRow(`SELECT status FROM issues WHERE key = ?`, k).Scan(&st)
		if st == tracker.StatusBlocked {
			blocked = k
		}
	}
	post(admin, "/cmd/projects/status/remove", `{"tab":"adm","op":"x1","project":"WEB","status":"blocked"}`)
	webtest.Next(t, settings, `"x1":{"ok":false,"msg":"Pick the status its 2 issues move to.","field":"move_to"}`)
	post(admin, "/cmd/projects/status/remove", `{"tab":"adm","op":"x2","project":"WEB","status":"blocked","move_to":"inprogress"}`)
	f = webtest.Next(t, settings, `"x2":{"ok":true`)
	_, hist := webtest.Get(t, guest, e.URL+"/"+blocked)
	if !strings.Contains(hist, "changed the status from Blocked to In Progress") {
		t.Error("the moved issue's history has no entry of the move")
	}
	if _, s := webtest.Get(t, admin, e.URL+"/projects/WEB/settings"); strings.Contains(s, `data-status="blocked"`) || strings.Contains(s, `data-column="blocked"`) {
		t.Error("Blocked and its column are still in the settings")
	}
	// FR-43.1: a column of two statuses, renamed, with a limit.
	post(admin, "/cmd/projects/status/column", `{"tab":"adm","op":"c1","project":"WEB","status":"in-qa","column":"inreview"}`)
	post(admin, "/cmd/projects/column/edit", `{"tab":"adm","op":"c2","project":"WEB","column":"inreview","name":"Review and QA","limit":1}`)
	webtest.Next(t, settings, `"c2":{"ok":true`)
	_, bpage := webtest.Get(t, guest, e.URL+"/projects/WEB/board")
	i := strings.Index(bpage, `data-lane="inreview"`)
	if i < 0 || !strings.Contains(bpage[i:i+400], "Review and QA") || !strings.Contains(bpage[i:i+800], "Over limit of 1") {
		t.Errorf("the column of two statuses: %.800s", bpage[max(i, 0):])
	}
	// The column In QA came with stays, empty, until it is removed.
	_, s := webtest.Get(t, admin, e.URL+"/projects/WEB/settings")
	col := regexp.MustCompile(`data-column="(c[0-9]+)"`).FindStringSubmatch(s)
	if col == nil || !strings.Contains(s, "No status: cards cannot go here") {
		t.Fatal("no empty column in the settings")
	}
	post(admin, "/cmd/projects/column/remove", `{"tab":"adm","op":"c3","project":"WEB","column":"`+col[1]+`"}`)
	webtest.Next(t, settings, `"c3":{"ok":true`)
	if _, bpage = webtest.Get(t, guest, e.URL+"/projects/WEB/board"); strings.Contains(bpage, `data-lane="`+col[1]+`"`) {
		t.Error("the removed column is still on the board")
	}
}

// FR-44, NFR-37.1: the board groups cards in rows by epic or assignee,
// chosen in the page's URL; each grouping is a view of its own, and a
// frame of a hundred cards in rows stays within 60 KB.
func TestBoardSwimlanes(t *testing.T) {
	e, pr := boardEnv(t, 100)
	ctx := context.Background()
	for _, g := range []string{views.LanesEpic, views.LanesAssignee} {
		var frame []byte
		err := e.DB.View(ctx, func(tx *sql.Tx) error {
			l, err := web.ReadLanes(ctx, sqlite.Querier(tx), pr.ID, tracker.Filter{}, g)
			if err != nil {
				return err
			}
			frame, err = web.LanesFrame(ctx, l)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		cards, rows := strings.Count(string(frame), `class="tk-card"`), strings.Count(string(frame), `class="tk-swim"`)
		t.Logf("by %s: %d cards in %d rows, a frame of %d bytes", g, cards, rows, len(frame))
		if cards != 100 || rows < 2 {
			t.Errorf("by %s: %d cards in %d rows", g, cards, rows)
		}
		if len(frame) > 60_000 {
			t.Errorf("by %s: the frame has %d bytes", g, len(frame))
		}
	}
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/projects/WEB/board?lanes=epic")
	arg := regexp.MustCompile(`data-arg="([^"]+)"`).FindStringSubmatch(page)[1]
	if arg != "WEB.ge" || !strings.Contains(page, `aria-label="`+pr.Keys[0]+` Board v1"`) || !strings.Contains(page, `aria-label="No epic"`) {
		t.Errorf("the board by epic: %s", arg)
	}
	events := webtest.Listen(t, c, e.URL+"/stream?tab=t1&v=board:"+arg, nil)
	webtest.Next(t, events, `tk-board--swim`)
	if code, _ := webtest.Get(t, c, e.URL+"/stream?tab=t2&v=board:WEB.gx"); code != http.StatusBadRequest {
		t.Errorf("a board view of unknown swimlanes: %d", code)
	}
	// A move in a row keeps the card in it.
	key := pr.Keys[1]
	if code, msg := webtest.Post(t, c, e.URL+"/cmd/board/move", `{"tab":"t1","op":"m1","issue":"`+key+`","column":"inprogress","before":"","after":"","seen":`+strconv.FormatInt(versionOf(page, key), 10)+`}`); code != http.StatusNoContent {
		t.Fatalf("the move: %d %s", code, msg)
	}
	webtest.Next(t, events, `"m1":{"ok":true`)
}
