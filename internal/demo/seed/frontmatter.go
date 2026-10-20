package seed

import (
	"fmt"
	"strings"
)

// frontMatter is the head of a page or template file: a small subset of
// YAML between two "---" lines.
//
//	key: value
//	labels: [one, two]
//	versions:
//	  - at: -20d 10:00
//	    by: s-priya
//	    body: |
//	      markup, indented by six spaces
//
// A value is the rest of its line; "|" starts a block of the lines indented
// deeper than its key. Only top-level keys hold lists of items.
type frontMatter map[string]any

func parseFront(src string) (frontMatter, string, error) {
	src = strings.ReplaceAll(src, "\r\n", "\n")
	rest, ok := strings.CutPrefix(src, "---\n")
	if !ok {
		return nil, "", fmt.Errorf("no front matter (the file starts with ---)")
	}
	head, body, ok := strings.Cut(rest, "\n---\n")
	if !ok {
		return nil, "", fmt.Errorf("front matter without its closing ---")
	}
	lines := strings.Split(head, "\n")
	fm, _, err := parseMap(lines, 0, 0)
	if err != nil {
		return nil, "", err
	}
	return fm, strings.Trim(body, "\n"), nil
}

// parseMap reads "key: value" lines indented by indent, from line i.
func parseMap(lines []string, i, indent int) (frontMatter, int, error) {
	m := frontMatter{}
	for i < len(lines) {
		l := lines[i]
		if strings.TrimSpace(l) == "" {
			i++
			continue
		}
		in := indentOf(l)
		if in < indent {
			break
		}
		if in > indent {
			return nil, 0, fmt.Errorf("front matter line %d: unexpected indentation", i+1)
		}
		key, value, ok := strings.Cut(strings.TrimSpace(l), ":")
		if !ok {
			return nil, 0, fmt.Errorf("front matter line %d: not key: value", i+1)
		}
		value = strings.TrimSpace(value)
		if _, dup := m[key]; dup {
			return nil, 0, fmt.Errorf("front matter: %s twice", key)
		}
		i++
		switch value {
		case "|":
			var block []string
			for i < len(lines) && (strings.TrimSpace(lines[i]) == "" || indentOf(lines[i]) > in) {
				block = append(block, lines[i])
				i++
			}
			m[key] = dedent(block)
		case "":
			var items []frontMatter
			for i < len(lines) && strings.HasPrefix(strings.TrimLeft(lines[i], " "), "- ") && indentOf(lines[i]) > in {
				at := indentOf(lines[i])
				// The item's first key sits after "- "; read the item as a
				// map indented to that column.
				item := append([]string{strings.Repeat(" ", at+2) + strings.TrimLeft(lines[i], " ")[2:]}, lines[i+1:]...)
				fm, n, err := parseMap(item, 0, at+2)
				if err != nil {
					return nil, 0, err
				}
				items = append(items, fm)
				i += n
			}
			m[key] = items
		default:
			m[key] = value
		}
	}
	return m, i, nil
}

func dedent(block []string) string {
	min := -1
	for _, l := range block {
		if strings.TrimSpace(l) != "" && (min < 0 || indentOf(l) < min) {
			min = indentOf(l)
		}
	}
	out := make([]string, len(block))
	for k, l := range block {
		if len(l) >= min && min > 0 {
			out[k] = l[min:]
		} else {
			out[k] = strings.TrimLeft(l, " ")
		}
	}
	return strings.Trim(strings.Join(out, "\n"), "\n")
}

func (fm frontMatter) str(key string) string {
	s, _ := fm[key].(string)
	return s
}

// list reads "[a, b]" or "a, b".
func (fm frontMatter) list(key string) []string {
	s := strings.Trim(fm.str(key), "[]")
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (fm frontMatter) items(key string) []frontMatter {
	items, _ := fm[key].([]frontMatter)
	return items
}

func (fm frontMatter) only(keys ...string) error {
	for k := range fm {
		found := false
		for _, want := range keys {
			found = found || k == want
		}
		if !found {
			return fmt.Errorf("unknown front matter key %q", k)
		}
	}
	return nil
}
