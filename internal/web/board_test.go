package web_test

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/lith/sqlite"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
	"github.com/zweiundeins/tickit/internal/tracker/trackertest"
	"github.com/zweiundeins/tickit/internal/views"
	"github.com/zweiundeins/tickit/internal/web"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// boardEnv is a server with the project WEB of n issues.
func boardEnv(t *testing.T, n int) (*webtest.Env, trackertest.Project) {
	t.Helper()
	e := webtest.Start(t)
	return e, trackertest.Board(t, e.W, "WEB", n)
}

// onBoard is a guest with the board open in tab: its page and its stream.
func onBoard(t *testing.T, e *webtest.Env, tab, query string) (*http.Client, string, <-chan string) {
	t.Helper()
	c := webtest.Client(t)
	code, page := webtest.Get(t, c, e.URL+"/projects/WEB/board"+query)
	if code != http.StatusOK {
		t.Fatalf("the board: %d", code)
	}
	v := regexp.MustCompile(`data-arg="([^"]+)"`).FindStringSubmatch(page)
	events := webtest.Listen(t, c, e.URL+"/stream?tab="+tab+"&v=board:"+v[1]+",boardhead:WEB", nil)
	webtest.Next(t, events, `id="board-lanes"`)
	webtest.Next(t, events, `"_outcomes"`)
	return c, page, events
}

// versionOf is a card's version in a page.
func versionOf(page, key string) int64 {
	m := regexp.MustCompile(`id="[cb]-` + key + `"[^>]*? data-v="(\d+)"`).FindStringSubmatch(page)
	if m == nil {
		return -1
	}
	var v int64
	fmt.Sscan(m[1], &v)
	return v
}

// laneOf is the column a card is in, in a page or a frame of the lanes.
func laneOf(html, key string) string {
	i := strings.Index(html, `id="c-`+key+`"`)
	if i < 0 {
		return ""
	}
	m := regexp.MustCompile(`data-lane="([a-z]+)"`).FindAllStringSubmatch(html[:i], -1)
	if m == nil {
		return ""
	}
	return m[len(m)-1][1]
}

func move(t *testing.T, e *webtest.Env, c *http.Client, tab, op, key, status, before, after string, seen int64) int {
	t.Helper()
	code, _ := webtest.Post(t, c, e.URL+"/cmd/board/move", fmt.Sprintf(`{"tab":%q,"op":%q,"issue":%q,"status":%q,"before":%q,"after":%q,"seen":%d}`, tab, op, key, status, before, after, seen))
	return code
}

// FR-41.1, MP-04.1, MP-04.4: a move answers 204; every board watching
// gets the frame with the card in its new column, in a view transition
// scoped to the lanes; the mover's tab gets the outcome.
func TestBoardMoveAccepted(t *testing.T) {
	e, pr := boardEnv(t, 24)
	a, page, ea := onBoard(t, e, "tabA", "")
	_, _, eb := onBoard(t, e, "tabB", "")
	key := pr.Keys[1]
	if laneOf(page, key) != "todo" {
		t.Fatalf("%s is in %q", key, laneOf(page, key))
	}
	if code := move(t, e, a, "tabA", "op1", key, "inreview", "", "", versionOf(page, key)); code != http.StatusNoContent {
		t.Fatalf("move: %d", code)
	}
	for name, ev := range map[string]<-chan string{"the mover": ea, "the other": eb} {
		f := webtest.Next(t, ev, `id="board-lanes"`)
		if laneOf(f, key) != "inreview" || !strings.Contains(f, "useViewTransition true") || !strings.Contains(f, "viewTransitionSelector #board-lanes") {
			t.Errorf("%s's frame shows %s in %q:\n%.300s", name, key, laneOf(f, key), f)
		}
	}
	webtest.Next(t, ea, `"op1":{"ok":true`)
}

// MP-04.3: of two moves of a card from one version, the second is
// refused with the first mover's name, and stores nothing.
func TestBoardMoveRefused(t *testing.T) {
	e, pr := boardEnv(t, 24)
	a, page, ea := onBoard(t, e, "tabA", "")
	b, _, eb := onBoard(t, e, "tabB", "")
	key := pr.Keys[1]
	seen := versionOf(page, key)
	if code := move(t, e, b, "tabB", "op1", key, "inreview", "", "", seen); code != http.StatusNoContent {
		t.Fatalf("the first move: %d", code)
	}
	webtest.Next(t, eb, `"op1":{"ok":true`)
	_, me := webtest.Get(t, b, e.URL+"/projects/WEB/board")
	name := regexp.MustCompile(`id="tk-me-label">([^<]*)<`).FindStringSubmatch(me)[1]
	if code := move(t, e, a, "tabA", "op2", key, "blocked", "", "", seen); code != http.StatusNoContent {
		t.Fatalf("the second move: %d", code)
	}
	ev := webtest.Next(t, ea, `"op2":`)
	want := fmt.Sprintf(`"op2":{"ok":false,"msg":"%s moved %s to In Review first.","field":"move"`, name, key)
	if !strings.Contains(ev, want) {
		t.Errorf("the outcome:\n%s\nwant %s", ev, want)
	}
	_, now := webtest.Get(t, a, e.URL+"/projects/WEB/board")
	if laneOf(now, key) != "inreview" {
		t.Errorf("%s is in %s", key, laneOf(now, key))
	}
	// A retry of the refused op hears the same and stores nothing.
	if code := move(t, e, a, "tabA", "op2", key, "blocked", "", "", seen); code != http.StatusNoContent {
		t.Errorf("the retry: %d", code)
	}
}

// NFR-01, FR-41: a Viewer may not move cards: 403, and the reason on the
// stream. WIP limits refuse nothing (FR-43.1).
func TestBoardMoveForbidden(t *testing.T) {
	e := webtest.Start(t)
	seed := people.Seeded[0]
	by := tabs.By{Actor: seed, Op: "s1", At: time.Now()}
	e.Do(t, people.SeedPeople{At: time.Now()})
	pid := tracker.NewID()
	e.Do(t, tracker.CreateProject{By: by, ID: pid, Key: "SEC", Name: "Secret", Lead: seed.ID})
	e.Do(t, tracker.CreateIssue{By: by, Project: pid, Type: tracker.Task, Summary: "Hidden work"})
	e.Do(t, tracker.SetWIPLimit{By: by, Project: pid, Status: tracker.StatusInProgress, Limit: 1})
	viewer := e.Account(t, "vera@example.com", "correct horse battery", people.NoRole)
	e.Do(t, writer.Func(func(tx *writer.Tx) error {
		return access.SetMember(tx, access.ProjectScope(pid), viewer, access.Viewer, seed.ID, time.Now())
	}))
	c := webtest.Client(t)
	e.Login(t, c, "vera@example.com", "correct horse battery")
	code, page := webtest.Get(t, c, e.URL+"/projects/SEC/board")
	if code != http.StatusOK {
		t.Fatalf("the viewer's board: %d", code)
	}
	if strings.Contains(page, `data-edit="1"`) {
		t.Error("the viewer's board offers drags")
	}
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tabV&v=board:SEC", nil)
	webtest.Next(t, events, `"_outcomes"`)
	if code := move(t, e, c, "tabV", "op1", "SEC-1", "inprogress", "", "", versionOf(page, "SEC-1")); code != http.StatusForbidden {
		t.Errorf("a viewer's move: %d", code)
	}
	webtest.Next(t, events, `"op1":{"ok":false,"msg":"Only members of this project can change its issues."`)
	// A guest does not see the closed project at all.
	if code, _ := webtest.Get(t, webtest.Client(t), e.URL+"/projects/SEC/board"); code != http.StatusNotFound {
		t.Errorf("a guest's board of a closed project: %d", code)
	}
	// Over the limit is a colour and words, never a refusal.
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	admin := webtest.Client(t)
	e.Login(t, admin, "ada@example.com", "correct horse battery")
	_, page = webtest.Get(t, admin, e.URL+"/projects/SEC/board")
	webtest.Listen(t, admin, e.URL+"/stream?tab=tabA&v=board:SEC", nil)
	if code := move(t, e, admin, "tabA", "op2", "SEC-1", "inprogress", "", "", versionOf(page, "SEC-1")); code != http.StatusNoContent {
		t.Errorf("a site admin's move: %d", code)
	}
}

// NFR-16: a malformed move answers 400, an unknown card 404, never 500.
func TestBoardMoveMalformed(t *testing.T) {
	e, _ := boardEnv(t, 4)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/projects/WEB/board")
	for body, want := range map[string]int{
		`{`: http.StatusBadRequest,
		`{"op":"o1","issue":"WEB-2","status":"todo"}`:                            http.StatusBadRequest,
		`{"tab":"t","op":"o2","issue":"web-2","status":"todo"}`:                  http.StatusBadRequest,
		`{"tab":"t","op":"o3","issue":"WEB-2","status":"To Do"}`:                 http.StatusBadRequest,
		`{"tab":"t","op":"o4","issue":"WEB-2","status":"todo","seen":-1}`:        http.StatusBadRequest,
		`{"tab":"t","op":"o5","issue":"WEB-2","status":"todo","before":"x"}`:     http.StatusBadRequest,
		`{"tab":"t","op":"o6","issue":"WEB-999","status":"todo"}`:                http.StatusNotFound,
		`{"tab":"t","op":"o7","issue":"WEB-2","status":"todo","seen":"one"}`:     http.StatusBadRequest,
		`{"tab":"t","op":"o8","issue":"WEB-2","status":"nope","seen":0}`:         http.StatusNoContent, // refused: no such status
		`{"tab":"t","op":"o9","issue":"NOPE-2","status":"todo"}`:                 http.StatusNotFound,
		`{"tab":"t","op":"o10","issue":"WEB-2","status":"todo","after":"WEB-3"}`: http.StatusNoContent,
	} {
		if code, msg := webtest.Post(t, c, e.URL+"/cmd/board/move", body); code != want {
			t.Errorf("%s: %d %s, want %d", body, code, msg, want)
		}
	}
}

// FR-02: a project's key is upper-cased; a key taken is refused next to
// the field; the page goes to the new board, which the list shows live
// (FR-01.1).
func TestCreateProject(t *testing.T) {
	e, _ := boardEnv(t, 2)
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/projects")
	if !strings.Contains(page, `>WEB</span>`) || !strings.Contains(page, `commandfor="create-project"`) {
		t.Fatalf("the projects page:\n%s", page)
	}
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=projects:", nil)
	webtest.Next(t, events, `id="projects-list"`)
	code, body := webtest.Post(t, c, e.URL+"/cmd/projects/create", `{"tab":"tab1","op":"op1","name":"Mobile app","key":"app","description":"For phones."}`)
	if code != http.StatusOK || !strings.Contains(body, `"_goto":"/projects/APP/board"`) {
		t.Fatalf("create: %d %s", code, body)
	}
	webtest.Next(t, events, `>APP</span>`)
	code, _ = webtest.Post(t, c, e.URL+"/cmd/projects/create", `{"tab":"tab1","op":"op2","name":"Another","key":"web"}`)
	if code != http.StatusNoContent {
		t.Errorf("a taken key: %d", code)
	}
	webtest.Next(t, events, `"op2":{"ok":false,"msg":"The key WEB is taken.","field":"key"}`)
	if code, _ := webtest.Get(t, c, e.URL+"/projects/APP/board"); code != http.StatusOK {
		t.Errorf("the new board: %d", code)
	}
	// Q24: two a day per session.
	if code, _ := webtest.Post(t, c, e.URL+"/cmd/projects/create", `{"tab":"tab1","op":"op3","name":"Third","key":"THR"}`); code != http.StatusTooManyRequests {
		t.Errorf("a third project in a day: %d", code)
	}
}

// MP-11.1, NFR-38: the tabs with the same quick filter watch one view,
// rendered once per tick in which a move changes it however many watch;
// another filter is another view.
func TestBoardOneRenderPerTick(t *testing.T) {
	e, pr := boardEnv(t, 24)
	var streams []<-chan string
	var page string
	var c *http.Client
	for i := range 12 {
		var ev <-chan string
		c, page, ev = onBoard(t, e, fmt.Sprintf("tab%d", i), "")
		streams = append(streams, ev)
	}
	_, _, bugs := onBoard(t, e, "tabBugs", "?type=bug")
	var pid string
	e.DB.View(context.Background(), func(tx *sql.Tx) error {
		p, err := tracker.ProjectByKey(context.Background(), tx, "WEB")
		pid = p.ID
		return err
	})
	key := views.BoardKey(pid, tracker.Filter{}, views.LanesNone)
	before := e.Hub.Stats().RendersByKey[key]
	if code := move(t, e, c, "tab11", "op1", pr.Keys[1], "inreview", "", "", versionOf(page, pr.Keys[1])); code != http.StatusNoContent {
		t.Fatalf("move: %d", code)
	}
	for _, ev := range streams {
		webtest.Next(t, ev, `id="board-lanes"`)
	}
	if got := e.Hub.Stats().RendersByKey[key] - before; got != 1 {
		t.Errorf("%d renders of %s for one move and 12 tabs", got, key)
	}
	webtest.Next(t, bugs, `id="board-lanes"`) // its counts of the columns changed
	keys := 0
	for k := range e.Hub.Stats().RendersByKey {
		if strings.HasPrefix(k, "board:"+pid+":") {
			keys++
		}
	}
	if keys != 2 {
		t.Errorf("%d board views for two filters", keys)
	}
}

// NFR-37.1: a board of 100 cards is one frame of at most 60 KB of HTML
// before compression; its render time is logged.
func TestBoardFrameSize(t *testing.T) {
	e, pr := boardEnv(t, 100)
	ctx := context.Background()
	var frame []byte
	const runs = 50
	start := time.Now()
	for range runs {
		err := e.DB.View(ctx, func(tx *sql.Tx) error {
			l, err := web.ReadLanes(ctx, sqlite.Querier(tx), pr.ID, tracker.Filter{}, views.LanesNone)
			if err != nil {
				return err
			}
			frame, err = web.LanesFrame(ctx, l)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	per := time.Since(start) / runs
	cards := strings.Count(string(frame), `class="tk-card"`)
	t.Logf("a board of %d cards: a frame of %d bytes (%d a card), read and rendered in %v", cards, len(frame), len(frame)/max(cards, 1), per)
	if cards != 100 {
		t.Errorf("%d cards on the board", cards)
	}
	if len(frame) > 60_000 {
		t.Errorf("the frame has %d bytes", len(frame))
	}
}

// FR-45.2, FR-46.1: checked issues go to a sprint at once; a sprint
// starts, the board shows its issues alone with days left and points
// (FR-46.2), and completing it sends its open issues back.
func TestSprints(t *testing.T) {
	e, pr := boardEnv(t, 12)
	c := webtest.Client(t)
	_, page := webtest.Get(t, c, e.URL+"/projects/WEB/backlog")
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=backlog:WEB", nil)
	webtest.Next(t, events, `id="backlog"`)
	post := func(path, body string, want int) {
		t.Helper()
		if code, msg := webtest.Post(t, c, e.URL+path, body); code != want {
			t.Fatalf("%s %s: %d %s", path, body, code, msg)
		}
	}
	post("/cmd/sprints/create", `{"tab":"tab1","op":"op1","project":"WEB","name":"Sprint 1","goal":"Ship the board"}`, http.StatusNoContent)
	f := webtest.Next(t, events, `data-name="Sprint 1"`)
	sprint := regexp.MustCompile(`data-section="([0-9a-f-]{36})"`).FindStringSubmatch(f)[1]
	two := fmt.Sprintf(`[{"issue":%q,"seen":%d},{"issue":%q,"seen":%d}]`, pr.Keys[1], versionOf(page, pr.Keys[1]), pr.Keys[2], versionOf(page, pr.Keys[2]))
	post("/cmd/backlog/plan", `{"tab":"tab1","op":"op2","project":"WEB","issues":`+two+`,"sprint":"`+sprint+`"}`, http.StatusNoContent)
	webtest.Next(t, events, `"op2":{"ok":true`)
	post("/cmd/sprints/start", `{"tab":"tab1","op":"op3","project":"WEB","sprint":"`+sprint+`"}`, http.StatusNoContent)
	webtest.Next(t, events, `"op3":{"ok":true`)
	_, board := webtest.Get(t, c, e.URL+"/projects/WEB/board")
	if n := strings.Count(board, `class="tk-card"`); n != 2 || !strings.Contains(board, "14 days left") || !strings.Contains(board, "Sprint 1") {
		t.Errorf("the sprint's board: %d cards", n)
	}
	post("/cmd/sprints/complete", `{"tab":"tab1","op":"op4","project":"WEB","sprint":"`+sprint+`","move_to":""}`, http.StatusNoContent)
	webtest.Next(t, events, `"op4":{"ok":true`)
	_, board = webtest.Get(t, c, e.URL+"/projects/WEB/board")
	if strings.Contains(board, "Active sprint") || strings.Count(board, `class="tk-card"`) < 5 {
		t.Errorf("the board after the sprint shows %d cards", strings.Count(board, `class="tk-card"`))
	}
	// Top: the checked issue before the first of the backlog.
	_, page = webtest.Get(t, c, e.URL+"/projects/WEB/backlog")
	last := pr.Keys[len(pr.Keys)-1]
	post("/cmd/backlog/plan", fmt.Sprintf(`{"tab":"tab1","op":"op5","project":"WEB","issues":[{"issue":%q,"seen":%d}],"rank":true,"before":%q}`, last, versionOf(page, last), pr.Keys[0]), http.StatusNoContent)
	if f := webtest.Next(t, events, `"op5":`); !strings.Contains(f, `"op5":{"ok":true`) {
		t.Fatalf("to the top: %s", f)
	}
	_, page = webtest.Get(t, c, e.URL+"/projects/WEB/backlog")
	if i, j := strings.Index(page, `id="b-`+last+`"`), strings.Index(page, `id="b-`+pr.Keys[0]+`"`); i < 0 || i > j {
		t.Errorf("%s is not first: %d, %d", last, i, j)
	}
	post("/cmd/backlog/plan", `{"tab":"tab1","op":"op6","project":"WEB","issues":[],"rank":true}`, http.StatusBadRequest)
}

// FR-03, FR-04, FR-05, FR-24, FR-43: an admin edits the project, sets a
// limit, keeps the last admin, adds a component and archives it; others
// hear 403.
func TestProjectSettings(t *testing.T) {
	e, _ := boardEnv(t, 16)
	guest := webtest.Client(t)
	if code, _ := webtest.Get(t, guest, e.URL+"/projects/WEB/settings"); code != http.StatusForbidden {
		t.Errorf("a guest's settings: %d", code)
	}
	if code, _ := webtest.Post(t, guest, e.URL+"/cmd/projects/edit", `{"tab":"t","op":"o1","project":"WEB","name":"Mine now"}`); code != http.StatusForbidden {
		t.Errorf("a guest's edit: %d", code)
	}
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	c := webtest.Client(t)
	e.Login(t, c, "ada@example.com", "correct horse battery")
	if code, page := webtest.Get(t, c, e.URL+"/projects/WEB/settings"); code != http.StatusOK || !strings.Contains(page, `id="project-settings"`) {
		t.Fatalf("the site admin's settings: %d", code)
	}
	events := webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=settings:WEB,boardhead:WEB", nil)
	webtest.Next(t, events, `id="project-settings"`)
	post := func(path, body string) {
		t.Helper()
		if code, msg := webtest.Post(t, c, e.URL+path, body); code != http.StatusNoContent {
			t.Fatalf("%s: %d %s", path, code, msg)
		}
	}
	post("/cmd/projects/edit", `{"tab":"tab1","op":"op1","project":"WEB","name":"Web shop","description":"All of it.","lead":""}`)
	webtest.Next(t, events, `<h1>Web shop</h1>`) // FR-03.2
	post("/cmd/projects/wip", `{"tab":"tab1","op":"op2","project":"WEB","status":"todo","limit":1}`)
	webtest.Next(t, events, `"op2":{"ok":true`)
	if _, board := webtest.Get(t, c, e.URL+"/projects/WEB/board"); !strings.Contains(board, "Over limit of 1") {
		i := strings.Index(board, `data-lane="todo"`)
		t.Errorf("the board shows no limit: %.600s", board[max(i, 0):])
	}
	seed := people.Seeded[0].ID
	post("/cmd/projects/member/remove", `{"tab":"tab1","op":"op3","project":"WEB","person":"`+seed+`"}`)
	webtest.Next(t, events, `"op3":{"ok":false,"msg":"Every project needs an admin: make someone else admin first.","field":"role"}`)
	post("/cmd/projects/component", `{"tab":"tab1","op":"op4","project":"WEB","name":"Checkout"}`)
	webtest.Next(t, events, `<strong>Checkout</strong>`)
	post("/cmd/projects/archive", `{"tab":"tab1","op":"op5","project":"WEB","archived":true}`)
	webtest.Next(t, events, `"op5":{"ok":true`)
	if _, list := webtest.Get(t, c, e.URL+"/projects"); !strings.Contains(list, `data-archived`) {
		t.Error("the list does not dim the archived project")
	}
}

// FR-03.1: the settings of a seeded project show its seeded lead as
// chosen among everyone who may lead it; a new lead shows in another
// tab's list of projects, and one who may not lead is refused in words.
func TestSettingsLead(t *testing.T) {
	e := seededEnv(t)
	e.Account(t, "ada@example.com", "correct horse battery", people.Admin)
	c := webtest.Client(t)
	e.Login(t, c, "ada@example.com", "correct horse battery")
	code, page := webtest.Get(t, c, e.URL+"/projects/WEB/settings")
	if code != http.StatusOK {
		t.Fatalf("the settings: %d", code)
	}
	i := strings.Index(page, `id="pd-lead"`)
	if i < 0 {
		t.Fatal("no lead picker")
	}
	picker := page[i : i+strings.Index(page[i:], "</sb-select>")]
	if !strings.Contains(picker, `value="s-priya"`) || !strings.Contains(picker, `{&#34;value&#34;:&#34;s-priya&#34;,&#34;label&#34;:&#34;Priya Raman&#34;}`) {
		t.Errorf("the lead is not chosen: %s", picker)
	}
	for _, p := range people.Seeded {
		if !strings.Contains(picker, `&#34;value&#34;:&#34;`+p.ID+`&#34;`) {
			t.Errorf("the picker leaves out %s", p.Name)
		}
	}

	var name, desc string
	e.DB.R.QueryRow(`SELECT name, description FROM projects WHERE key = 'WEB'`).Scan(&name, &desc)
	edit := func(op, lead string) {
		t.Helper()
		body := fmt.Sprintf(`{"tab":"tab1","op":%q,"project":"WEB","name":%q,"description":%q,"lead":%q}`, op, name, desc, lead)
		if code, msg := webtest.Post(t, c, e.URL+"/cmd/projects/edit", body); code != http.StatusNoContent {
			t.Fatalf("%s: %d %s", op, code, msg)
		}
	}
	settings := webtest.Listen(t, c, e.URL+"/stream?tab=tab1&v=settings:WEB", nil)
	webtest.Next(t, settings, `id="project-settings"`)
	list := webtest.Listen(t, c, e.URL+"/stream?tab=tab2&v=projects:", nil)
	if f := webtest.Next(t, list, `id="projects-list"`); strings.Contains(f, "Lena Brunner") {
		t.Fatal("Lena leads a project already")
	}
	edit("op1", "s-lena")
	webtest.Next(t, settings, `"op1":{"ok":true`)
	webtest.Next(t, list, "Lena Brunner")
	stranger := people.GuestOf("stranger")
	edit("op2", stranger.ID)
	webtest.Next(t, settings, `"op2":{"ok":false,"msg":"`+stranger.Name+` cannot lead WEB: they are not a member of it.","field":"lead"}`)
	if _, page := webtest.Get(t, c, e.URL+"/projects/WEB/settings"); !strings.Contains(page, `name="lead" options=`) || !regexp.MustCompile(`id="pd-lead"[^>]* value="s-lena"`).MatchString(page) {
		t.Error("the new lead is not chosen")
	}
}

// NFR-06: the projects' pages write nothing, with a project and its
// issues there.
func TestBoardPagesNeverWrite(t *testing.T) {
	e, _ := boardEnv(t, 8)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	e.W.Flush(context.Background())
	before := e.Fingerprint(t)
	for _, u := range []string{"/projects", "/projects/WEB", "/projects/WEB/board?assignee=me&q=a", "/projects/WEB/backlog", "/projects/WEB/activity", "/projects/WEB/settings", "/projects/WEB/board?create=todo"} {
		if code, _ := webtest.Get(t, c, e.URL+u); code >= 500 {
			t.Errorf("%s: %d", u, code)
		}
	}
	e.W.Flush(context.Background())
	if e.Fingerprint(t) != before {
		t.Error("a GET wrote")
	}
}
