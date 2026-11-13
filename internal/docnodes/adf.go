package docnodes

import (
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/zweiundeins/prosemirror-go"
	"github.com/zweiundeins/prosemirror-go/adf"
)

// Atlassian is what an import from Jira and Confluence knows of what it
// brought in: tickit's issue key for a Jira issue key, page id for a
// Confluence content id, person for an Atlassian account id, and image
// library id for an ADF media id. A miss keeps the reference as a link
// or its text.
type Atlassian interface {
	Issue(jiraKey string) (key string, ok bool)
	Page(contentID string) (pageID string, ok bool)
	Person(accountID string) (personID string, ok bool)
	Image(mediaID string) (imageID string, ok bool)
}

// codeAliases are names Jira and Confluence give code blocks for
// tickit's codeLanguages.
var codeAliases = map[string]string{
	"javascript": "js", "typescript": "ts", "shell": "bash", "sh": "bash", "zsh": "bash",
	"golang": "go", "yml": "yaml", "py": "python", "md": "markdown", "plaintext": "text",
	"none": "text", "xml": "html", "patch": "diff", "mysql": "sql", "postgresql": "sql",
}

// ADF returns tickit's rules for converting Atlassian Document Format
// (adf.Convert with Schema()) with what a reports for Jira's and
// Confluence's references. The result passes Rules{} but for the size
// and table limits: unknown code languages become none, links and cards
// that are not http, https or mailto plain text, control characters
// spaces (CR a newline in code), and a status's text is cut to
// statusText characters. Panels, statuses, text colours, the toc macro
// and the six heading levels map to tickit's own (adf.Defaults);
// backgroundColor is dropped, as tickit has no highlight.
func ADF(a Atlassian) *adf.Mapping {
	lim := SchemaLimits()
	names := adf.DefaultNames
	names.Highlight = ""
	names.HeadingLevels = slices.Max(lim.HeadingLevels)
	m := adf.Defaults(names)
	m.Text = adfText
	m.Nodes["status"] = func(n *adf.Node) adf.Result {
		color := n.Attr("color")
		text := []rune(strings.Join(strings.Fields(adfText(n.Attr("text"), "")), " "))
		if len(text) == 0 {
			return adf.Drop().Kind(color)
		}
		text = text[:min(len(text), lim.StatusText)]
		return adf.To("status", map[string]any{"text": strings.TrimSpace(string(text)), "color": adf.StatusColor(color)}).Kind(color)
	}
	m.Nodes["codeBlock"] = func(n *adf.Node) adf.Result {
		lang := strings.ToLower(n.Attr("language"))
		if l, ok := codeAliases[lang]; ok {
			lang = l
		}
		if !slices.Contains(lim.CodeLanguages, lang) {
			return adf.To("code_block", nil).Kind("language-other")
		}
		return adf.To("code_block", map[string]any{"language": lang})
	}
	m.Nodes["orderedList"] = func(n *adf.Node) adf.Result {
		o, _ := n.Number("order")
		return adf.To("ordered_list", map[string]any{"order": min(max(int(o), 1), 9999)})
	}
	cell := func(typ string) adf.Rule {
		return func(n *adf.Node) adf.Result {
			cs, _ := n.Number("colspan")
			rs, _ := n.Number("rowspan")
			span := min(max(int(cs), 1), lim.TableColumns)
			attrs := map[string]any{"colspan": span, "rowspan": min(max(int(rs), 1), lim.TableRows)}
			if w, ok := n.Attrs["colwidth"].([]any); ok && len(w) == span {
				valid := true
				for _, x := range w {
					f, ok := x.(float64)
					valid = valid && ok && f >= 0 && f <= 10000
				}
				if valid {
					attrs["colwidth"] = w
				}
			}
			return adf.To(typ, attrs)
		}
	}
	m.Nodes["tableCell"], m.Nodes["tableHeader"] = cell("table_cell"), cell("table_header")
	card := func(n *adf.Node) adf.Result {
		u := n.Attr("url")
		if u == "" {
			return adf.Drop().Kind("datasource")
		}
		c := adf.ParseCard(u)
		switch c.Kind {
		case "jira-issue":
			if key, ok := a.Issue(c.Issue); ok {
				return adf.To("issue_chip", map[string]any{"key": key}).Kind(c.Kind)
			}
		case "confluence-page", "confluence-blog":
			if id, ok := a.Page(c.Page); ok {
				return adf.To("page_link", map[string]any{"pageId": id}).Kind(c.Kind)
			}
		}
		if _, ok := prosemirror.SafeHref(u); !ok {
			return adf.Text(u).Kind("unsafe-url")
		}
		return adf.Link(u, u).Kind(c.Kind + "-link")
	}
	m.Nodes["inlineCard"], m.Nodes["blockCard"], m.Nodes["embedCard"] = card, card, card
	m.Nodes["mention"] = func(n *adf.Node) adf.Result {
		label := strings.TrimPrefix(n.Attr("text"), "@")
		if id, ok := a.Person(n.Attr("id")); ok {
			if l := utf8.RuneCountInString(label); l < 2 || l > 30 {
				label = ""
			}
			return adf.To("mention", map[string]any{"personId": id, "label": label})
		}
		return adf.Text(n.Attr("text")).Kind("unknown-person")
	}
	media := func(n *adf.Node) adf.Result {
		if n.Attr("type") == "file" {
			if id, ok := a.Image(n.Attr("id")); ok {
				alt := n.Attr("alt")
				if utf8.RuneCountInString(alt) > 500 {
					alt = string([]rune(alt)[:500])
				}
				return adf.To("image", map[string]any{"imageId": id, "alt": alt}).Kind("file")
			}
			return adf.Drop().Kind("file-unresolved")
		}
		if u := n.Attr("url"); n.Attr("type") == "external" && u != "" {
			if _, ok := prosemirror.SafeHref(u); ok {
				return adf.Link(u, n.Attr("alt")).Kind("external")
			}
		}
		return adf.Drop().Kind(n.Attr("type"))
	}
	m.Nodes["media"], m.Nodes["mediaInline"] = media, media
	m.Marks["link"] = func(mk adf.Mark) adf.MarkResult {
		if _, ok := prosemirror.SafeHref(mk.Attr("href")); !ok {
			return adf.DropMark().Kind("unsafe-url")
		}
		return adf.ToMark("link", map[string]any{"href": mk.Attr("href")})
	}
	return m
}

func adfText(text, parent string) string {
	code := parent == "codeBlock"
	if code {
		text = strings.ReplaceAll(text, "\r\n", "\n")
	}
	return strings.Map(func(c rune) rune {
		switch {
		case code && (c == '\t' || c == '\n'):
			return c
		case code && c == '\r':
			return '\n'
		case c < 0x20 || (c >= 0x7f && c <= 0x9f):
			return ' '
		}
		return c
	}, text)
}
