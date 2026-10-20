package tracker

import (
	"bytes"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"
)

// Descriptions and comments are rich-text documents in the editor's
// schema (internal/doc/schema.json), stored as ProseMirror's JSON. Until
// internal/doc validates them in full, this package checks their shape
// and size, reads their text for search and their mentions for
// notifications, and turns plain text into a document (PlainDoc).

// node is a node of a ProseMirror document as JSON holds it.
type node struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	Attrs   json.RawMessage `json:"attrs,omitempty"`
	Content []node          `json:"content,omitempty"`
	Marks   json.RawMessage `json:"marks,omitempty"`
}

// PlainDoc turns plain text into a document: a paragraph per line, the
// empty lines left out. Empty text gives an empty document, nil.
func PlainDoc(text string) json.RawMessage {
	var paras []node
	for line := range strings.Lines(text) {
		line = strings.TrimRight(line, "\r\n")
		if strings.TrimSpace(line) == "" {
			continue
		}
		paras = append(paras, node{Type: "paragraph", Content: []node{{Type: "text", Text: line}}})
	}
	if len(paras) == 0 {
		return nil
	}
	b, _ := json.Marshal(node{Type: "doc", Content: paras})
	return b
}

// docInfo is what a document holds that the tracker reads: its text, how
// many characters of it, and the people it mentions, each once.
type docInfo struct {
	Text     string
	Length   int
	Mentions []string
	blocks   map[string]bool // the block types it uses
}

var errNotDoc = errors.New("not a document")

// readDoc parses a document. Empty input, or JSON null, is the empty
// document.
func readDoc(raw json.RawMessage) (docInfo, error) {
	var info docInfo
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return info, nil
	}
	if len(raw) > MaxDocBytes {
		return info, errors.New("too large")
	}
	var root node
	if err := json.Unmarshal(raw, &root); err != nil || root.Type != "doc" {
		return info, errNotDoc
	}
	info.blocks = map[string]bool{}
	var b strings.Builder
	var walk func(n node, depth int) error
	walk = func(n node, depth int) error {
		if depth > 64 {
			return errNotDoc
		}
		switch n.Type {
		case "":
			return errNotDoc
		case "text":
			b.WriteString(n.Text)
			info.Length += utf8.RuneCountInString(n.Text)
			return nil
		case "hard_break":
			b.WriteByte('\n')
			return nil
		case "mention":
			var a struct {
				PersonID string `json:"personId"`
				Label    string `json:"label"`
			}
			if json.Unmarshal(n.Attrs, &a) != nil || a.PersonID == "" {
				return errNotDoc
			}
			if !slices.Contains(info.Mentions, a.PersonID) {
				info.Mentions = append(info.Mentions, a.PersonID)
			}
			if a.Label != "" {
				b.WriteString("@" + a.Label)
			}
			return nil
		case "issue_chip":
			var a struct {
				Key string `json:"key"`
			}
			if json.Unmarshal(n.Attrs, &a) == nil {
				b.WriteString(a.Key)
			}
			return nil
		}
		info.blocks[n.Type] = true
		for _, c := range n.Content {
			if err := walk(c, depth+1); err != nil {
				return err
			}
		}
		if len(n.Content) > 0 && n.Type != "doc" {
			b.WriteByte('\n')
		}
		return nil
	}
	if err := walk(root, 0); err != nil {
		return docInfo{}, err
	}
	info.Text = strings.TrimSpace(b.String())
	return info, nil
}

// canonical returns a document's JSON compacted, nil for the empty one,
// so that equal documents compare equal.
func canonical(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var b bytes.Buffer
	if json.Compact(&b, raw) != nil {
		return raw
	}
	// As the event log stores it: json.Marshal escapes <, > and & in a
	// RawMessage, so a projection built live equals one rebuilt.
	var out bytes.Buffer
	json.HTMLEscape(&out, b.Bytes())
	return out.Bytes()
}

// added returns the ids in now that were not in before, in order.
func added(before, now []string) []string {
	var out []string
	for _, id := range now {
		if !slices.Contains(before, id) {
			out = append(out, id)
		}
	}
	return out
}
