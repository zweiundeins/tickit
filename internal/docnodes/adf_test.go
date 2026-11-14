package docnodes

import (
	"strings"
	"testing"

	"github.com/zweiundeins/prosemirror-go/adf"
)

// fakeAtlassian knows the issue AB-1, the page 100, the account acc-1 and
// the media m-1.
type fakeAtlassian struct{}

func (fakeAtlassian) Issue(k string) (string, bool) { return k, k == "AB-1" }
func (fakeAtlassian) Page(id string) (string, bool) {
	return "0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b", id == "100"
}
func (fakeAtlassian) Person(id string) (string, bool) {
	return "a-0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c", id == "acc-1"
}
func (fakeAtlassian) Image(id string) (string, bool) { return "img-1", id == "m-1" }

// sampleADF holds every node type and mark seen in Jira and Confluence,
// with made-up content.
const sampleADF = `{"type":"doc","version":1,"content":[
	{"type":"heading","attrs":{"level":5},"content":[{"type":"text","text":"Deep heading"}]},
	{"type":"paragraph","marks":[{"type":"alignment","attrs":{"align":"center"}},{"type":"indentation","attrs":{"level":1}}],"content":[
		{"type":"text","text":"strong","marks":[{"type":"strong"}]},{"type":"text","text":" em","marks":[{"type":"em"}]},
		{"type":"text","text":" strike","marks":[{"type":"strike"}]},{"type":"text","text":" under","marks":[{"type":"underline"}]},
		{"type":"text","text":" code","marks":[{"type":"code"}]},{"type":"text","text":" color","marks":[{"type":"textColor","attrs":{"color":"#ff0000"}}]},
		{"type":"text","text":"2","marks":[{"type":"subsup","attrs":{"type":"sup"}}]},
		{"type":"text","text":" noted","marks":[{"type":"annotation","attrs":{"id":"x","annotationType":"inlineComment"}}]},
		{"type":"text","text":" link","marks":[{"type":"link","attrs":{"href":"https://example.com/a"}}]},
		{"type":"text","text":" bad","marks":[{"type":"link","attrs":{"href":"javascript:alert(1)"}}]},
		{"type":"hardBreak"},
		{"type":"inlineCard","attrs":{"url":"https://example.atlassian.net/browse/AB-1"}},
		{"type":"inlineCard","attrs":{"url":"https://example.atlassian.net/browse/ZZ-9"}},
		{"type":"inlineCard","attrs":{"url":"https://example.atlassian.net/wiki/spaces/DOC/pages/100/Title"}},
		{"type":"mention","attrs":{"id":"acc-1","text":"@Ada Example"}},
		{"type":"mention","attrs":{"id":"acc-2","text":"@Someone Else"}},
		{"type":"status","attrs":{"text":"IN PROGRESS","color":"blue"}},
		{"type":"status","attrs":{"text":"LLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLLL","color":"#8777D9"}},
		{"type":"text","text":" shaded","marks":[{"type":"backgroundColor","attrs":{"color":"#fff0b3"}}]},
		{"type":"date","attrs":{"timestamp":"1763078400000"}},
		{"type":"emoji","attrs":{"shortName":":thumbsup:","text":"👍"}},
		{"type":"inlineExtension","attrs":{"extensionKey":"anchor","extensionType":"com.atlassian.confluence.macro.core"}},
		{"type":"mediaInline","attrs":{"id":"m-1","type":"file"}},
		{"type":"placeholder","attrs":{"text":"Type something"}}]},
	{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"quoted"}]}]},
	{"type":"codeBlock","attrs":{"language":"JavaScript"},"marks":[{"type":"breakout","attrs":{"mode":"wide"}}],"content":[{"type":"text","text":"let a = 1"}]},
	{"type":"codeBlock","attrs":{"language":"cobol"},"content":[{"type":"text","text":"MOVE A TO B\r\n\tSTOP RUN"}]},
	{"type":"paragraph","content":[{"type":"text","text":"tab\tand\nnewline"}]},
	{"type":"rule"},
	{"type":"bulletList","content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]},
		{"type":"orderedList","attrs":{"order":0},"content":[{"type":"listItem","content":[{"type":"paragraph","content":[{"type":"text","text":"nested"}]}]}]}]}]},
	{"type":"taskList","attrs":{"localId":"t"},"content":[{"type":"taskItem","attrs":{"localId":"t1","state":"DONE"},"content":[{"type":"text","text":"done"}]},
		{"type":"taskItem","attrs":{"localId":"t2","state":"TODO"},"content":[{"type":"text","text":"todo"}]}]},
	{"type":"decisionList","content":[{"type":"decisionItem","attrs":{"state":"DECIDED"},"content":[{"type":"text","text":"decided"}]}]},
	{"type":"table","attrs":{"layout":"default"},"content":[
		{"type":"tableRow","content":[{"type":"tableHeader","attrs":{"colwidth":[120]},"content":[{"type":"paragraph","content":[{"type":"text","text":"h"}]}]},
			{"type":"tableHeader","attrs":{"colspan":2,"colwidth":[1]},"content":[{"type":"paragraph","content":[{"type":"text","text":"h2"}]}]}]},
		{"type":"tableRow","content":[{"type":"tableCell","attrs":{"background":"#eee"},"content":[{"type":"paragraph"}]},
			{"type":"tableCell","attrs":{"colspan":2},"content":[{"type":"paragraph","content":[{"type":"text","text":"c"}]}]}]}]},
	{"type":"mediaSingle","attrs":{"layout":"center"},"content":[{"type":"media","attrs":{"id":"m-1","type":"file","alt":"a picture"},"marks":[{"type":"border","attrs":{"size":1,"color":"#000"}}]},
		{"type":"caption","content":[{"type":"text","text":"caption"}]}]},
	{"type":"mediaGroup","content":[{"type":"media","attrs":{"id":"m-2","type":"file"}}]},
	{"type":"panel","attrs":{"panelType":"warning"},"content":[{"type":"paragraph","content":[{"type":"text","text":"careful"}]}]},
	{"type":"extension","attrs":{"extensionKey":"toc","extensionType":"com.atlassian.confluence.macro.core"}},
	{"type":"bodiedExtension","attrs":{"extensionKey":"details","extensionType":"com.atlassian.confluence.macro.core"},"content":[{"type":"paragraph","content":[{"type":"text","text":"body"}]}]},
	{"type":"layoutSection","content":[{"type":"layoutColumn","attrs":{"width":50},"content":[{"type":"paragraph","content":[{"type":"text","text":"left"}]}]},
		{"type":"layoutColumn","attrs":{"width":50},"content":[{"type":"paragraph","content":[{"type":"text","text":"right"}]}]}]},
	{"type":"expand","attrs":{"title":"Open me"},"content":[{"type":"nestedExpand","attrs":{"title":"Inner"},"content":[{"type":"paragraph","content":[{"type":"text","text":"inside"}]}]}]},
	{"type":"blockCard","attrs":{"url":"https://example.atlassian.net/wiki/spaces/DOC/pages/101/Other"}},
	{"type":"embedCard","attrs":{"url":"https://example.com/video","layout":"center"}}
]}`

func TestADF(t *testing.T) {
	doc, r, err := adf.Convert(Schema(), ADF(fakeAtlassian{}), []byte(sampleADF))
	if err != nil {
		t.Fatal(err)
	}
	if err := (Rules{}).Check(nil, doc); err != nil {
		t.Fatalf("the converted document fails tickit's rules: %v", err)
	}
	got := string(Marshal(doc))
	for _, want := range []string{
		`{"type":"heading","attrs":{"level":5}`,
		`{"type":"panel","attrs":{"kind":"warning"},"content":[{"type":"paragraph","content":[{"type":"text","text":"careful"}]}]}`,
		`{"type":"toc"}`,
		`{"type":"status","attrs":{"text":"IN PROGRESS","color":"blue"}}`,
		`{"type":"status","attrs":{"text":"` + strings.Repeat("L", 40) + `","color":"purple"}}`,
		`{"type":"text","marks":[{"type":"text_color","attrs":{"color":"red"}}],"text":" color"}`,
		`{"type":"text","text":" shaded"},{"type":"date","attrs":{"date":"2025-11-14"}},{"type":"text","text":"👍"}`,
		`{"type":"details","attrs":{"summary":"Open me","open":false},"content":[{"type":"details","attrs":{"summary":"Inner","open":false}`,
		`{"type":"details","attrs":{"summary":"Details","open":false},"content":[{"type":"paragraph","content":[{"type":"text","text":"body"}]}]}`,
		`{"type":"issue_chip","attrs":{"key":"AB-1"}}`,
		`{"type":"page_link","attrs":{"pageId":"0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5b"}}`,
		`{"type":"mention","attrs":{"personId":"a-0190a1b2-c3d4-7e5f-8a9b-0c1d2e3f4a5c","label":"Ada Example"}}`,
		`{"type":"image","attrs":{"imageId":"img-1","src":"","alt":"a picture"}}`,
		`{"type":"code_block","attrs":{"language":"js"}`,
		`{"type":"code_block","attrs":{"language":""}`,
		`{"type":"ordered_list","attrs":{"order":1}`,
		`{"type":"task_item","attrs":{"checked":true}`,
		`"colwidth":[120]`,
		`"MOVE A TO B\n\tSTOP RUN"`,
		`"tab and newline"`,
		`"href":"https://example.atlassian.net/browse/ZZ-9"`,
		`@Someone Else`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %s in\n%s", want, got)
		}
	}
	if strings.Contains(got, "javascript:") || strings.Contains(got, `"colwidth":[1]`) {
		t.Errorf("an unsafe link or a wrong colwidth is kept:\n%s", got)
	}
	nodes := []string{"text", "paragraph", "listItem", "tableCell", "bulletList", "inlineCard", "hardBreak", "taskItem",
		"mention", "tableRow", "blockquote", "heading", "taskList", "tableHeader", "orderedList", "codeBlock", "media",
		"table", "status", "date", "emoji", "mediaSingle", "mediaGroup", "rule", "panel", "extension", "bodiedExtension",
		"inlineExtension", "layoutSection", "layoutColumn", "expand", "nestedExpand", "decisionList", "decisionItem",
		"placeholder", "caption", "embedCard", "blockCard", "mediaInline"}
	for _, n := range nodes {
		if r.Nodes[n] == nil {
			t.Errorf("node %s is not in the report", n)
		}
	}
	for _, m := range []string{"code", "link", "strong", "strike", "em", "underline", "breakout", "alignment",
		"annotation", "textColor", "backgroundColor", "subsup", "indentation", "border"} {
		if r.Marks[m] == nil {
			t.Errorf("mark %s is not in the report", m)
		}
	}
	for key, want := range map[string]adf.Counts{
		"inlineCard/jira-issue":          {Converted: 1},
		"inlineCard/jira-issue-link":     {Linked: 1},
		"inlineCard/confluence-page":     {Converted: 1},
		"blockCard/confluence-page-link": {Linked: 1},
		"mention/unknown-person":         {Replaced: 1},
		"media/file":                     {Converted: 1},
		"media/file-unresolved":          {Dropped: 1},
		"mediaInline/file":               {Converted: 1},
		"panel/warning":                  {Converted: 1},
		"status/blue":                    {Converted: 1},
		"status/(other)":                 {Converted: 1},
		"textColor/red":                  {Converted: 1},
		"backgroundColor/yellow":         {Dropped: 1},
		"codeBlock/language-other":       {Converted: 1},
		"link/unsafe-url":                {Dropped: 1},
		"extension/toc":                  {Converted: 1},
	} {
		if c := r.Kinds[key]; c == nil || *c != want {
			t.Errorf("%s: %+v, want %+v", key, c, want)
		}
	}
	if r.Lost() != 0 {
		var b strings.Builder
		r.Write(&b)
		t.Errorf("lost %d:\n%s", r.Lost(), b.String())
	}
}

// An empty ADF document becomes a blank one.
func TestADFEmpty(t *testing.T) {
	doc, _, err := adf.Convert(Schema(), ADF(fakeAtlassian{}), []byte(`{"type":"doc","version":1,"content":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(Marshal(doc)); got != Blank {
		t.Errorf("got %s, want %s", got, Blank)
	}
}
