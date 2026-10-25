package docnodes

import (
	"testing"
)

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
