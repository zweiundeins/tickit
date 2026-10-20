package markup_test

import (
	"strings"
	"testing"

	"github.com/mbolli/tickit/internal/demo/seed"
	"github.com/mbolli/tickit/internal/markup"
)

// Every seeded description, comment and page reads back as the same
// document after Write: editing an existing text as markup loses nothing.
func TestWriteRoundTrip(t *testing.T) {
	c, err := seed.Load()
	if err != nil {
		t.Fatal(err)
	}
	var texts []struct {
		name, text, space string
	}
	for _, is := range c.Issues {
		texts = append(texts, struct{ name, text, space string }{is.Key, string(is.Description), ""})
		for i, cm := range is.Comments {
			texts = append(texts, struct{ name, text, space string }{is.Key + " comment " + string(rune('1'+i)), string(cm.Body), ""})
		}
	}
	for _, pg := range c.Pages {
		texts = append(texts, struct{ name, text, space string }{pg.Space + "/" + pg.Title, pg.Body, pg.Space})
	}
	if len(texts) < 50 {
		t.Fatalf("only %d texts", len(texts))
	}
	for _, x := range texts {
		r := c.Resolver(x.space, func(space, title string) string { return space + "/" + title })
		doc, err := markup.Parse(x.text, r)
		if err != nil {
			t.Errorf("%s: %v", x.name, err)
			continue
		}
		back, err := markup.Write(doc, func(id string) (string, bool) { return id, true })
		if err != nil {
			t.Errorf("%s: %v", x.name, err)
			continue
		}
		again, err := markup.Parse(back, r)
		if err != nil {
			t.Errorf("%s: Write wrote what Parse refuses: %v\n%s", x.name, err, back)
			continue
		}
		if string(again) != string(doc) {
			t.Errorf("%s: the round trip changed the document\nmarkup:\n%s\nwrote:\n%s\n got %s\nwant %s", x.name, x.text, back, again, doc)
		}
	}
}

type people map[string]string

func (people) Issue(key string) bool { return strings.HasPrefix(key, "WEB-") }
func (p people) Person(id string) (string, bool) {
	n, ok := p[id]
	return n, ok
}
func (people) Page(string) (string, bool) { return "", false }
func (people) Image(string) bool          { return false }

// What people type stays text where the markup does not read it, and
// comments hold no headings or tables.
func TestParseText(t *testing.T) {
	r := people{"g-aaaaaaaaaaaaaaaaaaaa": "Sunny Otter"}
	cases := []struct {
		mode     markup.Mode
		in, want string
	}{
		{markup.Text, "2 * 3 * 4 and a_b", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"2 * 3 * 4 and a_b"}]}]}`},
		{markup.Text, "**open and `tick", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"**open and ` + "`" + `tick"}]}]}`},
		{markup.Text, "see [[web-3]] and [[OPS-1]] [bare", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"see "},{"type":"issue_chip","attrs":{"key":"WEB-3"}},{"type":"text","text":" and [[OPS-1]] [bare"}]}]}`},
		{markup.Comment, "# not a heading\n\nhi @g-aaaaaaaaaaaaaaaaaaaa, @g-bbbbbbbbbbbbbbbbbbbb", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"# not a heading"}]},{"type":"paragraph","content":[{"type":"text","text":"hi "},{"type":"mention","attrs":{"personId":"g-aaaaaaaaaaaaaaaaaaaa","label":"Sunny Otter"}},{"type":"text","text":", @g-bbbbbbbbbbbbbbbbbbbb"}]}]}`},
		{markup.Text, "  indented\n#### four", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"indented"}]},{"type":"heading","attrs":{"level":3},"content":[{"type":"text","text":"four"}]}]}`},
	}
	for _, c := range cases {
		got, err := markup.ParseMode(c.in, r, c.mode)
		if err != nil {
			t.Errorf("ParseMode(%q): %v", c.in, err)
			continue
		}
		if string(got) != c.want {
			t.Errorf("ParseMode(%q)\n got %s\nwant %s", c.in, got, c.want)
		}
	}
	if _, err := markup.ParseMode(strings.Repeat("x", 2001), r, markup.Comment); err == nil {
		t.Error("a text over the limit went through")
	}
}
