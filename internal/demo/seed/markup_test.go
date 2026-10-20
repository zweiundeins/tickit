package seed

import (
	"strings"
	"testing"
)

type fakeResolver struct{}

func (fakeResolver) Issue(key string) bool { return strings.HasPrefix(key, "WEB-") }
func (fakeResolver) Person(id string) (string, bool) {
	return map[string]string{"s-priya": "Priya Raman"}[id], id == "s-priya"
}
func (fakeResolver) Page(ref string) (string, bool) { return "id:" + ref, ref != "Missing" }
func (fakeResolver) Image(id string) bool           { return id == "map" }

func TestParse(t *testing.T) {
	cases := []struct{ in, want string }{
		{"", `{"type":"doc","content":[{"type":"paragraph"}]}`},
		{"Hello **bold *both* too** and `x`", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","text":"Hello "},{"type":"text","marks":[{"type":"strong"}],"text":"bold "},{"type":"text","marks":[{"type":"strong"},{"type":"em"}],"text":"both"},{"type":"text","marks":[{"type":"strong"}],"text":" too"},{"type":"text","text":" and "},{"type":"text","marks":[{"type":"code"}],"text":"x"}]}]}`},
		{"## Title\n\nSee [[WEB-12]] and [[page:Glossary]], @s-priya.", `{"type":"doc","content":[{"type":"heading","attrs":{"level":2},"content":[{"type":"text","text":"Title"}]},{"type":"paragraph","content":[{"type":"text","text":"See "},{"type":"issue_chip","attrs":{"key":"WEB-12"}},{"type":"text","text":" and "},{"type":"page_link","attrs":{"pageId":"id:Glossary"}},{"type":"text","text":", "},{"type":"mention","attrs":{"personId":"s-priya","label":"Priya Raman"}},{"type":"text","text":"."}]}]}`},
		{"- [ ] one\n- [x] two", `{"type":"doc","content":[{"type":"task_list","content":[{"type":"task_item","attrs":{"checked":false},"content":[{"type":"paragraph","content":[{"type":"text","text":"one"}]}]},{"type":"task_item","attrs":{"checked":true},"content":[{"type":"paragraph","content":[{"type":"text","text":"two"}]}]}]}]}`},
		{"3. a\n   - b\n4. c", `{"type":"doc","content":[{"type":"ordered_list","attrs":{"order":3},"content":[{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"a"}]},{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"b"}]}]}]}]},{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"c"}]}]}]}]}`},
		{"| A | B |\n| --- | --- |\n| 1 \\| 2 | |", `{"type":"doc","content":[{"type":"table","content":[{"type":"table_row","content":[{"type":"table_header","attrs":{"colspan":1,"rowspan":1,"colwidth":null},"content":[{"type":"paragraph","content":[{"type":"text","text":"A"}]}]},{"type":"table_header","attrs":{"colspan":1,"rowspan":1,"colwidth":null},"content":[{"type":"paragraph","content":[{"type":"text","text":"B"}]}]}]},{"type":"table_row","content":[{"type":"table_cell","attrs":{"colspan":1,"rowspan":1,"colwidth":null},"content":[{"type":"paragraph","content":[{"type":"text","text":"1 | 2"}]}]},{"type":"table_cell","attrs":{"colspan":1,"rowspan":1,"colwidth":null},"content":[{"type":"paragraph"}]}]}]}]}`},
		{"```go\nfunc main() {\n\n}\n```\n\n---\n\n![A map](lib:map)", `{"type":"doc","content":[{"type":"code_block","attrs":{"language":"go"},"content":[{"type":"text","text":"func main() {\n\n}"}]},{"type":"horizontal_rule"},{"type":"image","attrs":{"imageId":"map","src":"/img/lib/map","alt":"A map"}}]}`},
		{"> quoted\n> on two lines\\\n> broken", `{"type":"doc","content":[{"type":"blockquote","content":[{"type":"paragraph","content":[{"type":"text","text":"quoted on two lines"},{"type":"hard_break"},{"type":"text","text":"broken"}]}]}]}`},
		{"[the **docs**](https://example.com/a) ~~old~~ snake_case mail@example.com \\*", `{"type":"doc","content":[{"type":"paragraph","content":[{"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com/a"}}],"text":"the "},{"type":"text","marks":[{"type":"link","attrs":{"href":"https://example.com/a"}},{"type":"strong"}],"text":"docs"},{"type":"text","text":" "},{"type":"text","marks":[{"type":"strike"}],"text":"old"},{"type":"text","text":" snake_case mail@example.com *"}]}]}`},
		{"- item\n\n  ```bash\n  echo hi\n\n  echo there\n  ```\n- next", `{"type":"doc","content":[{"type":"bullet_list","content":[{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"item"}]},{"type":"code_block","attrs":{"language":"bash"},"content":[{"type":"text","text":"echo hi\n\necho there"}]}]},{"type":"list_item","content":[{"type":"paragraph","content":[{"type":"text","text":"next"}]}]}]}]}`},
	}
	v := loadSchema(t)
	for _, c := range cases {
		got, err := Parse(c.in, fakeResolver{})
		if err != nil {
			t.Errorf("Parse(%q): %v", c.in, err)
			continue
		}
		if string(got) != c.want {
			t.Errorf("Parse(%q)\n got %s\nwant %s", c.in, got, c.want)
		}
		if err := v.validate(got); err != nil {
			t.Errorf("Parse(%q) is not valid: %v", c.in, err)
		}
	}
}

func TestParseRefuses(t *testing.T) {
	for _, in := range []string{
		"#### four", "**open", "a *b", "`code", "[[OPS-1]]", "[[page:Missing]]", "@s-nobody",
		"[x](javascript:alert(1))", "![alt](lib:nope)", "![](lib:map)", "text ![a](lib:map)",
		"```cobol\nx\n```", "```go\nunclosed", "| a |\n| b |", "  indented", "[bare bracket",
		"| a | b |\n| --- | --- |\n| 1 |",
	} {
		if _, err := Parse(in, fakeResolver{}); err == nil {
			t.Errorf("Parse(%q) accepted it", in)
		}
	}
}
