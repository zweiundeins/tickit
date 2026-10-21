package doc

import (
	"strconv"
	"strings"
	"testing"
)

const savedID = "0192f0c4-7b1e-7cc3-9d2a-5f6e7a8b9c0d"

func issueList(attrs string) string { return `{"type":"issue_list","attrs":` + attrs + `}` }

// FR-104: the issue list block agrees with prosemirror-model: valid and
// invalid documents, its defaults, and attr steps on it, applied and
// written back as V8 does.
func TestIssueListAgreesWithOracle(t *testing.T) {
	o := theOracle(t)
	cases := []string{
		docOf(`{"type":"issue_list"}`),
		docOf(issueList(`{"filter":"status=todo&project=WEB","limit":5}`)),
		docOf(issueList(`{"savedFilter":"`+savedID+`"}`), para),
		docOf(issueList(`{"limit":"5"}`)),
		docOf(issueList(`{"filter":7}`)),
		docOf(`{"type":"paragraph","content":[{"type":"issue_list"}]}`),
		docOf(`{"type":"bullet_list","content":[{"type":"list_item","content":[` + para + `,{"type":"issue_list"}]}]}`),
		docOf(`{"type":"issue_list","content":[` + para + `]}`),
	}
	for i, c := range cases {
		want := o.must(t, "check", c)
		_, err := ParseDoc([]byte(c))
		got := ""
		if err != nil {
			got = err.Error()
		}
		if got != want {
			t.Errorf("case %d %s:\n go: %q\n js: %q", i, c, got, want)
		}
		if err == nil {
			d, _ := ParseDoc([]byte(c))
			if out, _ := MarshalDoc(d); string(out) != o.must(t, "roundtrip", c) {
				t.Errorf("case %d: writes %s", i, out)
			}
		}
	}

	start := docOf(para, issueList(`{"filter":"status=todo"}`), para)
	d := mustDoc(t, start)
	id, _ := strconv.Atoi(o.must(t, "load", start))
	defer o.call("free", id)
	for _, raw := range []string{
		`{"stepType":"attr","pos":4,"attr":"filter","value":"assignee=unassigned&q=login"}`,
		`{"stepType":"attr","pos":4,"attr":"limit","value":25}`,
		`{"stepType":"attr","pos":4,"attr":"savedFilter","value":"` + savedID + `"}`,
		`{"stepType":"replace","from":4,"to":5}`,
		`{"stepType":"replace","from":0,"to":0,"slice":{"content":[{"type":"issue_list","attrs":{"filter":"type=bug","savedFilter":"","limit":3}}]}}`,
	} {
		st, err := tickit.ParseStep([]byte(raw))
		if err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		if res := o.must(t, "apply", id, raw); res != "" {
			t.Fatalf("%s: oracle %s", raw, res)
		}
		if d, err = st.Apply(d); err != nil {
			t.Fatalf("%s: %v", raw, err)
		}
		got, _ := MarshalDoc(d)
		if want := o.must(t, "json", id); string(got) != want {
			t.Fatalf("%s:\n go: %s\n js: %s", raw, got, want)
		}
	}
}

// FR-104, DM-05: the server's rules for an issue list.
func TestIssueListRules(t *testing.T) {
	refused := []struct{ attrs, msg string }{
		{`{"savedFilter":"nope"}`, "An issue list names a saved filter of this site."},
		{`{"filter":"` + strings.Repeat("a", 501) + `"}`, "An issue list's filter holds at most 500 characters."},
		{`{"filter":"q=a\u0007"}`, "An issue list's filter holds a control character."},
		{`{"filter":"q=%zz"}`, "An issue list's filter is written as the issue list's address writes it, such as status=todo&project=WEB."},
		{`{"limit":0}`, "An issue list shows 1 to 25 rows."},
		{`{"limit":26}`, "An issue list shows 1 to 25 rows."},
		{`{"limit":2.5}`, "An issue list shows 1 to 25 rows."},
	}
	for _, c := range refused {
		if err := library.Check(nil, mustDoc(t, docOf(issueList(c.attrs)))); err == nil || err.Error() != c.msg {
			t.Errorf("%s: %v, want %q", c.attrs, err, c.msg)
		}
	}
	for _, attrs := range []string{`{}`, `{"filter":"status=todo&assignee=me&q=log in","limit":25}`, `{"savedFilter":"` + savedID + `","limit":1}`} {
		if err := library.Check(nil, mustDoc(t, docOf(issueList(attrs)))); err != nil {
			t.Errorf("%s: %v", attrs, err)
		}
	}
}

type listResolver struct {
	StaticResolver
	calls int
}

func (r *listResolver) List(l IssueList) string {
	r.calls++
	if l.Filter == "none" {
		return ""
	}
	return `<table data-n="` + strconv.Itoa(r.calls) + `"></table>`
}

// FR-104: an issue list renders its filter as attributes and what a
// ListResolver gives inside; without one (versions, comparisons) its
// description; a BlockCache renders its block every time.
func TestIssueListHTML(t *testing.T) {
	d := mustDoc(t, docOf(issueList(`{"filter":"status=todo&q=<b>"}`), issueList(`{"savedFilter":"`+savedID+`","limit":3}`), issueList(`{}`)))
	want := `<div class="issue-list" data-filter="status=todo&amp;q=&lt;b&gt;" data-saved-filter="" data-limit="10"><p class="issue-list__desc">Issues matching <code>status=todo&amp;q=&lt;b&gt;</code>, at most 10</p></div>` +
		`<div class="issue-list" data-filter="" data-saved-filter="` + savedID + `" data-limit="3"><p class="issue-list__desc">Issues of a saved filter, at most 3</p></div>` +
		`<div class="issue-list" data-filter="" data-saved-filter="" data-limit="10"><p class="issue-list__desc">An issue list without a filter, at most 10</p></div>`
	if got := RenderHTML(d, nil); got != want {
		t.Errorf("without a ListResolver:\n got %s\nwant %s", got, want)
	}
	if got := ListDescription(IssueList{SavedFilter: savedID, Limit: 5}, "Open <bugs>"); got != `<p class="issue-list__desc">Issues of the saved filter <strong>Open &lt;bugs&gt;</strong>, at most 5</p>` {
		t.Errorf("a saved filter's name: %s", got)
	}
	if l := IssueLists(mustDoc(t, docOf(`{"type":"blockquote","content":[`+issueList(`{"filter":"type=bug","limit":4}`)+`]}`, para))); len(l) != 1 || l[0].Key() != "|type=bug|4" {
		t.Errorf("IssueLists: %+v", l)
	}

	r := &listResolver{}
	var c BlockCache
	d = mustDoc(t, docOf(para, issueList(`{"filter":"type=bug"}`), issueList(`{"filter":"none"}`)))
	first := c.Render(d, r)
	if !strings.Contains(first, `<div class="issue-list" data-filter="type=bug" data-saved-filter="" data-limit="10"><table data-n="1"></table></div>`) ||
		!strings.Contains(first, `<p class="issue-list__desc">Issues matching <code>none</code>`) {
		t.Errorf("with a ListResolver: %s", first)
	}
	if second := c.Render(d, r); !strings.Contains(second, `data-n="3"`) || !strings.HasPrefix(second, "<p>hi</p>") {
		t.Errorf("the second render keeps the list's old rows: %s", second)
	}
	if md := Markdown(mustDoc(t, docOf(issueList(`{"filter":"q=log in"}`))), nil); !strings.Contains(md, "[issues matching q=log in](/issues?q=log+in)") {
		t.Errorf("Markdown: %q", md)
	}
	if txt := Text(d); txt != "hi" {
		t.Errorf("Text: %q", txt)
	}
}
