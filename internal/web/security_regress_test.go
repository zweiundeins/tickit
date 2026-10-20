package web_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/mbolli/tickit/internal/web/webtest"
)

// Regression tests of the findings of the security review (docs/security.md).

// NFR-13, DM-06.1: text that closes the editor page's script of the
// document stays inside it as JSON.
func TestEditorScriptHoldsNoMarkup(t *testing.T) {
	e := startWiki(t)
	c := webtest.Client(t)
	webtest.Get(t, c, e.URL+"/")
	space, home := wikiSpace(t, e, c, "ENG")
	page := wikiPage(t, e, c, space, home, "Notes")
	h := head(t, e, page)
	if code, _ := act(t, e, c, "/cmd/pages/"+page+"/open", "tab1", map[string]any{"base": h.Version, "client": "0123456789ab"}); code != http.StatusNoContent {
		t.Fatalf("open: %d", code)
	}
	evil := `</script><div data-init='alert(1)'>x</div><!--`
	steps, _ := json.Marshal([]any{map[string]any{"stepType": "replace", "from": 1, "to": 1,
		"slice": map[string]any{"content": []any{map[string]any{"type": "text", "text": evil}}}}})
	code, body := webtest.Post(t, c, e.URL+"/cmd/pages/"+page+"/commit",
		fmt.Sprintf(`{"tab":"tab1","epoch":%q,"base":%d,"ref":"r1","client":"0123456789ab","steps":%s}`, h.Epoch, h.Version, steps))
	if code != http.StatusNoContent {
		t.Fatalf("commit: %d %s", code, body)
	}
	_, edit := webtest.Get(t, webtest.Client(t), e.URL+"/pages/"+page+"/edit")
	if loc := regexp.MustCompile(`/spaces/[^"]+/edit`).FindString(edit); loc != "" {
		_, edit = webtest.Get(t, webtest.Client(t), e.URL+loc)
	}
	m := regexp.MustCompile(`(?s)<script type="application/json"[^>]*>(.*?)</script>`).FindStringSubmatch(edit)
	if m == nil {
		t.Fatalf("no script of the document in the editor's page:\n%.2000s", edit)
	}
	if strings.Contains(edit, `<div data-init='alert(1)'>`) {
		t.Error("the person's markup reached the editor's page")
	}
	var d any
	if err := json.Unmarshal([]byte(m[1]), &d); err != nil || !strings.Contains(fmt.Sprint(d), evil) {
		t.Errorf("the script's document: %v %.300s", err, m[1])
	}
}
