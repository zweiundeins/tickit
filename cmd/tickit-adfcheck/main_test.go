package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheck(t *testing.T) {
	dir := t.TempDir()
	para := func(s string) string {
		return `{"type":"doc","version":1,"content":[{"type":"paragraph","content":[{"type":"text","text":"` + s + `"}]}]}`
	}
	page, _ := json.Marshal(para("page text"))
	files := map[string]string{
		"jira.json": `{"issues":[{"key":"AB-1","fields":{"description":` + para("issue text") + `,"comment":{"comments":[{"body":` + para("comment text") + `}]}}},` +
			`{"key":"AB-2","fields":{"description":null,"comment":{"comments":[]}}}]}`,
		"conf.json": `{"results":[{"id":"1","body":{"atlas_doc_format":{"value":` + string(page) + `,"representation":"atlas_doc_format"}}}]}`,
		"doc.json":  `{"type":"doc","version":1,"content":[{"type":"table","content":[` + strings.Repeat(`{"type":"tableRow","content":[{"type":"tableCell","content":[{"type":"paragraph"}]}]},`, 50) + `{"type":"tableRow","content":[]}]}]}`,
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	c := newCheck()
	if err := c.dir(dir); err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	c.write(&b)
	out := b.String()
	for k, n := range map[string]int{"jira description": 1, "jira description (empty)": 1, "jira comment": 1, "confluence page": 1, "adf": 1} {
		if c.sources[k] != n {
			t.Errorf("%s: %d documents, want %d", k, c.sources[k], n)
		}
	}
	if c.refused["adf: A table holds at most 50 rows."] != 1 {
		t.Errorf("refusals %v", c.refused)
	}
	for _, text := range []string{"issue text", "comment text", "page text"} {
		if strings.Contains(out, text) {
			t.Errorf("the output holds a document's text:\n%s", out)
		}
	}
}
