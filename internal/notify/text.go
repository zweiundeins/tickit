package notify

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxText is how many characters of a sentence or a comment a
// notification keeps.
const MaxText = 160

// node is a node of a document as ProseMirror's JSON holds it (the
// editor's schema, internal/docnodes/schema.json).
type node struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	Attrs   json.RawMessage `json:"attrs,omitempty"`
	Content []node          `json:"content,omitempty"`
}

// blocks returns the text of each textblock of a document, mentions as
// "@" and their label, and for each block where the person's first
// mention in it starts (-1 for none).
func blocks(doc json.RawMessage, person string) (texts []string, at []int) {
	doc = bytes.TrimSpace(doc)
	if len(doc) == 0 {
		return nil, nil
	}
	var root node
	if json.Unmarshal(doc, &root) != nil {
		return nil, nil
	}
	var walk func(n node, depth int)
	walk = func(n node, depth int) {
		if depth > 64 {
			return
		}
		if !textblock(n) {
			for _, c := range n.Content {
				walk(c, depth+1)
			}
			return
		}
		var b strings.Builder
		pos := -1
		for _, c := range n.Content {
			switch c.Type {
			case "text":
				b.WriteString(c.Text)
			case "hard_break":
				b.WriteByte('\n')
			case "mention":
				var a struct {
					PersonID string `json:"personId"`
					Label    string `json:"label"`
				}
				json.Unmarshal(c.Attrs, &a)
				if a.PersonID == person && pos < 0 && person != "" {
					pos = b.Len()
				}
				if a.Label != "" {
					b.WriteString("@" + a.Label)
				}
			case "issue_chip":
				var a struct {
					Key string `json:"key"`
				}
				json.Unmarshal(c.Attrs, &a)
				b.WriteString(a.Key)
			}
		}
		texts, at = append(texts, b.String()), append(at, pos)
	}
	walk(root, 0)
	return texts, at
}

// textblock reports whether a node holds inline content.
func textblock(n node) bool {
	switch n.Type {
	case "paragraph", "heading", "code_block":
		return true
	}
	return false
}

// MentionSentence is the sentence of a document around the person's
// first mention in it (FR-112.1), at most MaxText characters, or "" when
// it does not mention them.
func MentionSentence(doc json.RawMessage, person string) string {
	texts, at := blocks(doc, person)
	for i, pos := range at {
		if pos < 0 {
			continue
		}
		t := texts[i]
		start, end := 0, len(t)
		for j := pos - 1; j > 0; j-- {
			if t[j] == '\n' {
				start = j + 1
				break
			}
			if strings.ContainsRune(".!?", rune(t[j-1])) && unicode.IsSpace(rune(t[j])) {
				start = j
				break
			}
		}
		for j := pos; j < len(t); j++ {
			if t[j] == '\n' {
				end = j
				break
			}
			if strings.ContainsRune(".!?", rune(t[j])) && (j+1 == len(t) || unicode.IsSpace(rune(t[j+1]))) {
				end = j + 1
				break
			}
		}
		return clip(t[start:end], t[start:pos])
	}
	return ""
}

// Excerpt is the start of a document's text, at most MaxText characters:
// what a comment says (FR-113).
func Excerpt(doc json.RawMessage) string {
	texts, _ := blocks(doc, "")
	var parts []string
	n := 0
	for _, t := range texts {
		t = strings.Join(strings.Fields(t), " ")
		if t == "" {
			continue
		}
		parts = append(parts, t)
		if n += len(t); n > 4*MaxText {
			break
		}
	}
	return clip(strings.Join(parts, " "), "")
}

// clip squeezes the spaces of s and cuts it to MaxText characters: around
// the mention when lead (the text before it) is long, at a word.
func clip(s, lead string) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= MaxText {
		return s
	}
	r := []rune(s)
	from := 0
	if l := utf8.RuneCountInString(strings.Join(strings.Fields(lead), " ")); l > MaxText/2 {
		from = min(l-MaxText/3, len(r)-MaxText)
	}
	out := string(r[from : from+MaxText-1])
	if i := strings.LastIndexByte(out, ' '); i > len(out)*2/3 {
		out = out[:i]
	}
	if from > 0 {
		if i := strings.IndexByte(out, ' '); i >= 0 && i < len(out)/3 {
			out = out[i+1:]
		}
		out = "…" + out
	}
	return strings.TrimSpace(out) + "…"
}
