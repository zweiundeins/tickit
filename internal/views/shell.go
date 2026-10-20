// Package views renders tickit's pages and the regions their streams
// patch. Layout is the document every page shares; areas add their pages
// and regions in files of their own. A shared region holds nothing about
// its viewer: the same bytes go to every tab. What concerns the viewer
// comes from the document (Shell) or from the tab's and the person's views.
package views

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"

	"github.com/a-h/templ"

	"github.com/mbolli/tickit/internal/people"
)

// Shell is what the document around every page shows.
type Shell struct {
	Title       string // the page's own title; the document's adds "tickit"
	Tab         string // new for every document: names the tab's stream and row
	Nonce       string // the request's CSP nonce, for the import map
	CSS, JS     string // URLs of the stylesheet and of Datastar
	NewestFrame string // URL of lith's newest-frame.js (live.NewestFrame)
	// Views are the views the page's stream watches besides the tab's, the
	// person's and the demo's, as "kind:arg" (docs/architecture.md).
	Views []string
	Me    people.Person
	// Moderates and Administers say which links the person menu shows
	// (FR-137.2).
	Moderates, Administers bool
	Notice                 string // the demo notice (DM-10.1)
	Path                   string // the page's path and query, for the way back after a login
}

// StreamID is the id of the element whose data-init opens the stream: on a
// deploy the stream's last event (live.Spec.Reconnect) re-creates it, and
// the copy opens the stream again on the next process (MP-10, NFR-42).
const StreamID = "stream"

// documentTitle is the title of a document with a page's title.
func documentTitle(page string) string {
	if page == "" {
		return "tickit"
	}
	return page + " · tickit"
}

// jsString is s as a JavaScript string literal: JSON, which escapes quotes,
// <, > and &, so that it is safe in an attribute and a script.
func jsString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// signals are the document's, all local (_): actions send only their
// payload. _views holds the stream's views for openStream, _outcomes the
// outcomes the tab's view brings (op id to {ok, msg, field}).
func signals(s Shell) string {
	return "{_tab: " + jsString(s.Tab) + ", _views: " + jsString(strings.Join(s.Views, ",")) + ", _slow: " + slowLink +
		", _outcomes: {}, _ping: 0, _beat: 0, _stale: false, _goto: '', _problem: '', _rename: {op: '', name: '', msg: ''}}"
}

// importMap maps 'datastar' to Datastar's URL, for the modules that import
// it (newest-frame.js), with the request's CSP nonce.
func importMap(s Shell) templ.Component {
	b, _ := json.Marshal(map[string]any{"imports": map[string]string{"datastar": s.JS}}) // escapes <, > and &: safe in a <script>
	return templ.Raw(`<script type="importmap" nonce="` + templ.EscapeString(s.Nonce) + `">` + string(b) + `</script>`)
}

// meStyle marks the viewer's own face wherever a shared view puts it: the
// shared views know nothing of who looks at them (MP-02.1). The presence
// row shows the own face first, and at most 8 faces with it.
func meStyle(me string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if !people.ValidID(me) {
			return nil
		}
		who := `[data-who="` + me + `"]`
		_, err := io.WriteString(w, `<style>`+who+`{--me:1}.presence>li`+who+`:nth-child(n){order:-1;display:revert}`+
			`.presence:has(>li`+who+`:nth-child(n+9))>li:nth-child(8){display:none}</style>`)
		return err
	})
}

// hueStyle sets a person's hue for the face's colour.
func hueStyle(hue int) templ.SafeCSS {
	return templ.SafeCSS("--h: " + strconv.Itoa(hue))
}

// Shown is how many faces a presence row shows (MP-02.1).
const Shown = 8

// Present is a person in a presence row, with the number of their tabs on
// it (MP-01.1).
type Present struct {
	people.Person
	Tabs int
}

// Tooltip names the person, with their tabs when there are several.
func (p Present) Tooltip() string {
	if p.Tabs > 1 {
		return p.Name + ", " + strconv.Itoa(p.Tabs) + " tabs"
	}
	return p.Name
}

// PresenceID is the id of the presence row of the view with key.
func PresenceID(key string) string {
	return "presence-" + strings.NewReplacer(":", "-", ".", "-").Replace(key)
}

// loginURL is the login page, with the way back to path.
func loginURL(path string) string {
	if path == "" || path == "/" || strings.HasPrefix(path, "/login") {
		return "/login"
	}
	return "/login?next=" + url.QueryEscape(path)
}

// more is "+n" for the people beyond the first Shown.
func more(n int) string { return "+" + strconv.Itoa(n-Shown) }
