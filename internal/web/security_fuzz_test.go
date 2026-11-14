package web_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// NFR-16, requirements 12.3: no action answers 500 or panics, whatever
// its body: a table-driven sweep of every action with malformed, oversized,
// wrong-typed, deeply nested and unicode-edge payloads, and Go fuzz
// targets per family of actions (go tool task security, docs/security.md).

// sweepBy is who sends an action in the sweep and the fuzzers: who may,
// so that the payload reaches the command, but for the actions that would
// end the world the rest of the sweep needs.
func sweepBy(c secCase) who {
	switch c.name {
	case "admin/reset":
		return wOwner // a refusal: a reset would wipe the fixture
	case "logout", "account/password", "account/end-session":
		return wOther
	}
	return c.by
}

// oddValues are the values each field of a body takes in turn.
var oddValues = []any{
	nil, true, false, 0, -1, 1e308, -1e308, 9007199254740993, "9" + strings.Repeat("9", 400),
	"", " ", strings.Repeat("x", 100<<10), "\x00", "\ufeff", "\u202eevil", "\u200b\u200d", "\U0001F469\u200d\U0001F469\u200d\U0001F467\u200d\U0001F466",
	"e\u0301" + strings.Repeat("\u0301", 500), "\U0010FFFF", "../../etc/passwd", "<script>alert(1)</script>",
	`'"` + "`${x}", "%00%ff", "javascript:alert(1)", "me", "-0", "1e999", "SEC-1", "SEC-99999999999999999999",
	[]any{}, []any{1, "a", nil}, map[string]any{}, map[string]any{"a": map[string]any{"b": []any{1}}}, nested(200),
}

// oddBodies replace a whole body.
var oddBodies = [][]byte{
	nil, []byte("null"), []byte("[]"), []byte("1"), []byte(`"x"`), []byte("{"), []byte("{}}"), []byte(`{"tab":`),
	[]byte("\xff\xfe\x00{}"), []byte("\xef\xbb\xbf{}"), []byte(`{"tab":"\ud800","op":"\udfff"}`), []byte(`{"tab":"sec","op":"o","tab":"x"}`),
	[]byte(strings.Repeat("[", 100000) + strings.Repeat("]", 100000)), []byte(`{"a":` + strings.Repeat(`{"a":`, 20000) + "1" + strings.Repeat("}", 20001)),
	[]byte(`{"tab":"sec","op":"big","x":"` + strings.Repeat("y", 2<<20) + `"}`), []byte(strings.Repeat(" ", 1<<20) + "{}"),
}

func nested(n int) any {
	var v any = "deep"
	for range n {
		v = []any{v}
	}
	return v
}

// send posts body to path as c without failing the test: the status.
func send(t testing.TB, c *http.Client, url string, body []byte) int {
	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return 0
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	webtest.IDs(req, body)
	res, err := c.Do(req)
	if err != nil {
		t.Errorf("POST %s: %v", url, err)
		return 0
	}
	io.Copy(io.Discard, res.Body)
	res.Body.Close()
	return res.StatusCode
}

// TestMalformedEveryAction is the sweep: every action, every whole odd
// body, and every field of its body as every odd value and as missing.
func TestMalformedEveryAction(t *testing.T) {
	f := newFixture(t)
	n := 0
	for _, c := range secCases {
		w := sweepBy(c)
		cl := f.client(w)
		path, valid := c.req(f, c.open)
		valid["tab"], valid["op"] = "sweep", "v"
		check := func(what string, body []byte) {
			n++
			if code := send(t, cl, f.e.URL+path, body); code >= 500 || code == 0 {
				t.Errorf("NFR-16: %s with %s: %d", c.name, what, code)
			}
		}
		for i, b := range oddBodies {
			check("odd body "+string(rune('a'+i)), b)
		}
		for field := range valid {
			for i, v := range oddValues {
				body := map[string]any{}
				for k, x := range valid {
					body[k] = x
				}
				body[field] = v
				if field != "tab" && field != "op" {
					body["op"] = "o" + strings.Repeat("x", i%7) + field[:min(len(field), 20)]
				}
				b, _ := json.Marshal(body)
				label := mustJSON(v)
				check(field+" as "+label[:min(40, len(label))], b)
			}
			missing := map[string]any{}
			for k, x := range valid {
				if k != field {
					missing[k] = x
				}
			}
			b, _ := json.Marshal(missing)
			check("no "+field, b)
		}
		// Odd path values for actions with ids in the path.
		if strings.Contains(c.pattern, "{") {
			for _, odd := range []string{"nope", "%00", "..", strings.Repeat("a", 4000), "0192d000-0000-7000-8000-000000000001", "%F0%9F%92%A9"} {
				p := path
				for _, id := range []string{f.id("issue", c.open), f.id("page", c.open)} {
					p = strings.Replace(p, id, odd, 1)
				}
				n++
				b, _ := json.Marshal(valid)
				if code := send(t, cl, f.e.URL+p, b); code >= 500 {
					t.Errorf("NFR-16: %s at %s: %d", c.name, p[:min(len(p), 80)], code)
				}
			}
		}
	}
	if logs := f.e.Logs(); strings.Contains(logs, "panic") {
		t.Errorf("NFR-16: a panic in the log:\n%s", firstLines(logs, "panic", 20))
	}
	t.Logf("%d malformed requests", n)
}

func firstLines(log, want string, n int) string {
	var out []string
	for _, l := range strings.Split(log, "\n") {
		if strings.Contains(l, want) && len(out) < n {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// fuzzFamily fuzzes the actions whose names start with one of prefixes:
// sel picks the action, body is what the fuzzer makes of its payload.
// No answer is a 500, and a refusal, an answer of 4xx or an outcome that
// is not OK, leaves every table but the bookkeeping as it was.
func fuzzFamily(f *testing.F, prefixes ...string) {
	fx := newFixture(f)
	var cases []secCase
	for _, c := range secCases {
		for _, p := range prefixes {
			if strings.HasPrefix(c.name, p) {
				cases = append(cases, c)
			}
		}
	}
	if len(cases) == 0 {
		f.Fatalf("no actions start with %v", prefixes)
	}
	type target struct {
		url  string
		w    who
		c    *http.Client
		logs bool // a refusal is a security event (FR-156.1): a failed login
	}
	targets := make([]target, len(cases))
	for i, c := range cases {
		path, body := c.req(fx, c.open)
		body["tab"], body["op"] = "fuzz", "seed"
		targets[i] = target{fx.e.URL + path, sweepBy(c), fx.client(sweepBy(c)), c.name == "login"}
		b, _ := json.Marshal(body)
		f.Add(uint8(i), b)
		f.Add(uint8(i), b[:len(b)/2])
		f.Add(uint8(i), bytes.ReplaceAll(b, []byte(`"`), []byte(`'`)))
	}
	fx.countChanges(f)
	f.Fuzz(func(t *testing.T, sel uint8, body []byte) {
		tg := targets[int(sel)%len(targets)]
		before := fx.changes(t)
		code := send(t, tg.c, tg.url, body)
		if code >= 500 {
			t.Errorf("NFR-16: %s with %q: %d", tg.url, body, code)
		}
		if fx.refused(t, tg.w, code, body) {
			diff := changed(before, fx.changes(t))
			if tg.logs {
				diff = slices.DeleteFunc(diff, func(table string) bool { return table == "security_log" })
			}
			if len(diff) > 0 {
				t.Errorf("NFR-01.1: %s refused %q (%d) and changed %v", tg.url, body, code, diff)
			}
		}
	})
}

// countChanges counts the committed changes of each table but the
// bookkeeping in temp.fuzz_changes, by temporary triggers on the writer's
// connection: a digest of every table per input is 13 times slower. The
// search index, a virtual table, changes only with the rows it indexes.
func (f *fixture) countChanges(tb testing.TB) {
	f.e.Do(tb, writer.Func(func(tx *writer.Tx) error {
		if _, err := tx.ExecUncached(`CREATE TEMP TABLE fuzz_changes (tbl TEXT PRIMARY KEY, n INTEGER NOT NULL)`); err != nil {
			return err
		}
		rows, err := tx.QueryUncached(`SELECT name, coalesce(sql, '') FROM main.sqlite_schema WHERE type = 'table' AND name NOT LIKE 'sqlite\_%' ESCAPE '\' ORDER BY name`)
		if err != nil {
			return err
		}
		var tables, virtual []string
		for rows.Next() {
			var name, def string
			if err := rows.Scan(&name, &def); err != nil {
				rows.Close()
				return err
			}
			if strings.HasPrefix(strings.ToUpper(def), "CREATE VIRTUAL") {
				virtual = append(virtual, name)
			}
			tables = append(tables, name)
		}
		rows.Close()
		for i, table := range tables {
			if slices.Contains(bookkeeping, table) || slices.ContainsFunc(virtual, func(v string) bool { return table == v || strings.HasPrefix(table, v+"_") }) {
				continue
			}
			var cols []string
			crows, err := tx.QueryUncached(`SELECT name FROM pragma_table_info(?)`, table)
			if err != nil {
				return err
			}
			for crows.Next() {
				var c string
				crows.Scan(&c)
				if !slices.Contains(bookkeeping, table+"."+c) {
					cols = append(cols, `"`+c+`"`)
				}
			}
			crows.Close()
			for _, ev := range []string{"INSERT", "DELETE", "UPDATE OF " + strings.Join(cols, ", ")} {
				q := fmt.Sprintf(`CREATE TEMP TRIGGER fuzz_%d_%s AFTER %s ON main."%s" BEGIN INSERT INTO fuzz_changes VALUES ('%s', 1) ON CONFLICT DO UPDATE SET n = n + 1; END`, i, ev[:1], ev, table, table)
				if _, err := tx.ExecUncached(q); err != nil {
					return fmt.Errorf("%s: %w", q, err)
				}
			}
		}
		return nil
	}))
}

// changes are the counts of countChanges once everything queued is written.
func (f *fixture) changes(t *testing.T) map[string]string {
	out := map[string]string{}
	if err := f.e.W.Do(context.Background(), writer.Func(func(tx *writer.Tx) error {
		rows, err := tx.Query(`SELECT tbl, n FROM temp.fuzz_changes`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var table, n string
			if err := rows.Scan(&table, &n); err != nil {
				return err
			}
			out[table] = n
		}
		return rows.Err()
	})); err != nil {
		t.Fatal(err)
	}
	return out
}

// refused reports whether w's action with body was refused: answered 4xx,
// or with an outcome in the tab's row that is not OK.
func (f *fixture) refused(t *testing.T, w who, code int, body []byte) bool {
	if code >= 400 && code < 500 {
		return true
	}
	var p struct{ Tab, Op, Ref string }
	if json.Unmarshal(body, &p) != nil || p.Tab == "" {
		return false
	}
	if p.Op == "" {
		p.Op = p.Ref
	}
	var st tabs.State
	if err := f.e.DB.View(context.Background(), func(tx *sql.Tx) (err error) {
		st, err = tabs.Get(context.Background(), tx, f.sid(w), p.Tab)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	for _, o := range st.Outcomes {
		if o.Op == p.Op {
			return !o.OK
		}
	}
	return false
}

func FuzzCoreActions(f *testing.F) {
	fuzzFamily(f, "watch", "unwatch", "login", "rename", "prefs", "input", "list/", "filters/", "dashboards/")
}
func FuzzIssueActions(f *testing.F) { fuzzFamily(f, "issues/") }
func FuzzBoardActions(f *testing.F) { fuzzFamily(f, "projects/", "board/", "backlog/", "sprints/") }
func FuzzWikiActions(f *testing.F) {
	fuzzFamily(f, "spaces/", "pages/r", "pages/m", "pages/l", "pages/w", "pages/t", "pages/d", "pages/s", "pages/c", "pages/i")
}
func FuzzEditorActions(f *testing.F) {
	fuzzFamily(f, "pages/open", "pages/commit", "pages/presence", "pages/image", "issues/description/")
}
func FuzzNotifyActions(f *testing.F) { fuzzFamily(f, "notify/") }
func FuzzStaffActions(f *testing.F)  { fuzzFamily(f, "moderation/", "report", "admin/", "account/") }

// FuzzUploads sends the uploads (FR-85) with every file the fuzzer makes,
// as the editor sends them: none answers 500.
func FuzzUploads(f *testing.F) {
	fx := newFixture(f)
	type target struct {
		url string
		c   *http.Client
	}
	var targets []target
	for _, c := range secCases {
		if strings.HasSuffix(c.name, "/image") {
			path, _ := c.req(fx, c.open)
			targets = append(targets, target{fx.e.URL + path, fx.client(sweepBy(c))})
		}
	}
	for i := range targets {
		f.Add(uint8(i), uploadPNG())
		f.Add(uint8(i), uploadPNG()[:20])
		f.Add(uint8(i), []byte("<svg onload=alert(1)>"))
	}
	f.Fuzz(func(t *testing.T, sel uint8, file []byte) {
		tg := targets[int(sel)%len(targets)]
		fields := map[string]string{"tab": "fuzz", "op": fmt.Sprintf("f%d", fx.n.Add(1))}
		if code, answer := webtest.Upload(t, tg.c, tg.url, fields, file); code >= 500 && code != http.StatusServiceUnavailable {
			t.Errorf("NFR-16: %s with %d bytes: %d %s", tg.url, len(file), code, answer)
		}
	})
}
