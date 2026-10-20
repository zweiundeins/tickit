package doc

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

var diffPairs = [][2]string{
	{"f20k-doc", "f20k-final"}, {"f20k-doc", "f20k-k5-final"}, {"f20k-k0-final", "f20k-k20-final"},
	{"f200k-doc", "f200k-final"}, {"f20k-final", "f20k-doc"},
}

// FindDiffStart and FindDiffEnd agree with prosemirror-model, and the
// restore step turns the current version into the old one (FR-89).
func TestRestoreStep(t *testing.T) {
	o := theOracle(t)
	for _, p := range diffPairs {
		ja, jb := readFixture(t, p[0]+".json"), readFixture(t, p[1]+".json")
		a, b := mustDoc(t, string(ja)), mustDoc(t, string(jb))
		start, ok := FindDiffStart(a.content, b.content, 0)
		if want := o.must(t, "diffStart", string(ja), string(jb)); (ok && strconv.Itoa(start) != want) || (!ok && want != "null") {
			t.Errorf("%v: diffStart %d, oracle %s", p, start, want)
		}
		ea, eb, _ := FindDiffEnd(a.content, b.content, a.content.size, b.content.size)
		var end struct{ A, B int }
		json.Unmarshal([]byte(o.must(t, "diffEnd", string(ja), string(jb))), &end)
		if ea != end.A || eb != end.B {
			t.Errorf("%v: diffEnd %d %d, oracle %+v", p, ea, eb, end)
		}
		st := RestoreStep(a, b)
		got, err := st.Apply(a)
		if err != nil || !got.Eq(b) {
			t.Fatalf("%v: restore step %v", p, err)
		}
		if rs := st.(*ReplaceStep); rs.From == 0 && rs.To == a.content.size {
			t.Logf("%v: the restore replaces the whole document", p)
		}
	}
	d := mustDoc(t, docOf(para))
	if RestoreStep(d, d) != nil {
		t.Fatal("restore of an equal document")
	}
	// Emoji: the step never cuts a pair.
	a := mustDoc(t, inPara(text("x😀y")))
	b := mustDoc(t, inPara(text("x😁y")))
	if s, _ := FindDiffStart(a.content, b.content, 0); s != 2 {
		t.Fatalf("diff start in an emoji: %d", s)
	}
	if got, err := RestoreStep(a, b).Apply(a); err != nil || !got.Eq(b) {
		t.Fatal(err)
	}
}

// FR-88: added text is marked as added, removed text as removed.
func TestDiff(t *testing.T) {
	a := mustDoc(t, docOf(
		`{"type":"heading","attrs":{"level":1},"content":[`+text("Release plan")+`]}`,
		`{"type":"paragraph","content":[`+text("We ship the editor on Monday.")+`]}`,
		`{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"paragraph","content":[`+text("one")+`]}]},{"type":"list_item","content":[{"type":"paragraph","content":[`+text("two")+`]}]}]}`,
		`{"type":"horizontal_rule"}`,
	))
	b := mustDoc(t, docOf(
		`{"type":"heading","attrs":{"level":1},"content":[`+text("Release plan")+`]}`,
		`{"type":"paragraph","content":[`+text("We ship the ")+`,`+text("editor", strong)+`,`+text(" on Tuesday.")+`]}`,
		`{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"paragraph","content":[`+text("one")+`]}]},{"type":"list_item","content":[{"type":"paragraph","content":[`+text("2")+`]}]},{"type":"list_item","content":[{"type":"paragraph","content":[`+text("three")+`]}]}]}`,
		`{"type":"paragraph","content":[`+text("<new>")+`]}`,
	))
	d := Diff(a, b)
	want := `<h2>Release plan</h2>` +
		`<p>We ship the <del class="diff-del">editor</del><ins class="diff-ins"><strong>editor</strong></ins> on <del class="diff-del">Monday</del><ins class="diff-ins">Tuesday</ins>.</p>` +
		`<ul><li><p>one</p></li><li><p><del class="diff-del">two</del><ins class="diff-ins">2</ins></p></li><li data-diff="ins"><p><ins class="diff-ins">three</ins></p></li></ul>` +
		`<del class="diff-del"><hr data-diff="del"></del><p data-diff="ins"><ins class="diff-ins">&lt;new&gt;</ins></p>`
	if got := d.HTML(nil); got != want {
		t.Errorf("diff:\n got %s\nwant %s", got, want)
	}
	if d.Equal() || d.Inserted == 0 || d.Deleted == 0 {
		t.Errorf("counts %d %d", d.Inserted, d.Deleted)
	}
	if !Diff(a, a).Equal() {
		t.Error("a document differs from itself")
	}
	// Large documents compare in reasonable time and render.
	f := mustDoc(t, string(readFixture(t, "f200k-doc.json")))
	g := mustDoc(t, string(readFixture(t, "f200k-final.json")))
	if h := Diff(f, g).HTML(nil); !strings.Contains(h, "diff-ins") {
		t.Error("no insertions in the fixture diff")
	}
}

// FR-152: a page exports as Markdown.
func TestMarkdown(t *testing.T) {
	d := mustDoc(t, docOf(
		`{"type":"heading","attrs":{"level":1},"content":[`+text("Plan")+`]}`,
		`{"type":"paragraph","content":[`+text("a ")+`,`+text("bold", strong)+`,`+text(" *star* ")+`,`+text("x", link("https://x.y"))+`,`+text(" ")+`,`+text("bad", link("javascript:1"))+`,{"type":"issue_chip","attrs":{"key":"WEB-12"}}]}`,
		`{"type":"task_list","content":[{"type":"task_item","attrs":{"checked":true},"content":[{"type":"paragraph","content":[`+text("done")+`]}]},{"type":"task_item","content":[{"type":"paragraph","content":[`+text("open")+`]}]}]}`,
		`{"type":"ordered_list","attrs":{"order":3},"content":[{"type":"list_item","content":[{"type":"paragraph","content":[`+text("three")+`]}]}]}`,
		`{"type":"blockquote","content":[`+para+`]}`,
		`{"type":"code_block","attrs":{"language":"go"},"content":[`+text("x := 1\ny")+`]}`,
		`{"type":"table","content":[{"type":"table_row","content":[{"type":"table_header","content":[{"type":"paragraph","content":[`+text("h|1")+`]}]}]},{"type":"table_row","content":[{"type":"table_cell","content":[`+para+`]}]}]}`,
		`{"type":"horizontal_rule"}`,
		`{"type":"image","attrs":{"imageId":"img-1","alt":"a [diagram]"}}`,
	))
	want := "## Plan\n\na **bold** \\*star\\* [x](<https://x.y>) badWEB-12\n\n- [x] done\n- [ ] open\n\n3. three\n\n> hi\n\n```go\nx := 1\ny\n```\n\n| h\\|1 |\n| --- |\n| hi |\n\n---\n\n![a \\[diagram\\]](/img/lib/img-1)\n"
	if got := Markdown(d, nil); got != want {
		t.Errorf("markdown:\n%s\nwant\n%s", got, want)
	}
}
