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
	"github.com/mbolli/tickit/internal/tabs"
	"github.com/mbolli/tickit/internal/ui"
)

// Shell is what the document around every page shows. web.Page sets the
// page's part (Title to Create); the server fills in the rest.
type Shell struct {
	Title string // the page's own title; the document's adds "tickit"
	// Nav is the main link the page belongs to: "projects", "spaces",
	// "filters", "dashboards", or "" for none (ui.MainNav).
	Nav string
	// Sidebar is the project's or space's navigation (ui.SideNav), nil for
	// none; SidebarLabel names it ("Project", the default, or "Space").
	Sidebar      templ.Component
	SidebarLabel string
	// Width of main: "" (80rem), "prose" (46rem) or "full" (boards).
	Width string
	// Query is the header search box's text, on the search page.
	Query string
	// Views are the views the page's stream watches besides the tab's, the
	// person's and the demo's, as "kind:arg" (docs/architecture.md).
	Views []string
	// Create is the dialog the header's Create opens, with the id
	// CreateID; nil shows a placeholder.
	Create templ.Component

	Tab         string // new for every document: names the tab's stream and row
	Nonce       string // the request's CSP nonce, for the import map
	CSS, JS     string // URLs of the stylesheet bundle and of Datastar
	NewestFrame string // URL of lith's newest-frame.js (live.NewestFrame)
	Mark        string // URL of the favicon
	Fonts       []string
	// Scripts are module URLs that load after Datastar: Starbase's
	// components and the editor, as the page asks for them.
	Scripts []string
	Me      people.Person
	// Moderates and Administers say which links the header shows
	// (FR-137.2).
	Moderates, Administers bool
	NextReset              string // when the demo resets: "5 h" (DM-10.1)
	DemoAlert              string // the reset's warning, or that it is under way (DM-03.2, DM-03.4)
	Prefs                  tabs.Prefs
	Bell                   Bell   // the bell's count and panel (notify, web.Server.bell)
	Path                   string // the page's path and query, for the way back after a login
}

// CreateID is the id of the create dialog.
const CreateID = "create"

// UIPerson is p as internal/ui draws people: the one place where a person
// becomes a face, so a person looks the same everywhere. The hue is the
// index the person has (people.Person.Hue), 0 to 15.
func UIPerson(p people.Person) ui.Person {
	hue := p.Hue
	return ui.Person{Seed: p.ID, Name: p.Name, Hue: &hue, Deactivated: p.Inactive || p.Kind == people.Deleted}
}

// UIPrefs are the preferences as <html> carries them.
func UIPrefs(p tabs.Prefs) ui.Prefs {
	p = p.Normal()
	return ui.Prefs{Theme: p.Theme, Style: p.Style, Density: p.Density}
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
// outcomes the tab's view brings (op id to {ok, msg, field}), _prefs the
// session's preferences (tabs.PrefsView).
func signals(s Shell) string {
	prefs, _ := json.Marshal(s.Prefs.Normal())
	return "{_tab: " + jsString(s.Tab) + ", _views: " + jsString(strings.Join(s.Views, ",")) + ", _slow: " + slowLink +
		", _outcomes: {}, _ping: 0, _beat: 0, _stale: false, _goto: '', _problem: '', _prefs: " + string(prefs) +
		", _display: {op: ''}, _dismiss: {op: ''}, _rename: {op: '', name: '', msg: '', ok: false}}"
}

// importMap maps 'datastar' to Datastar's URL, for the modules that import
// it (newest-frame.js), with the request's CSP nonce.
func importMap(s Shell) templ.Component {
	b, _ := json.Marshal(map[string]any{"imports": map[string]string{"datastar": s.JS}}) // escapes <, > and &: safe in a <script>
	return templ.Raw(`<script type="importmap" nonce="` + templ.EscapeString(s.Nonce) + `">` + string(b) + `</script>`)
}

// meStyle marks the viewer's own face wherever a shared view puts it: the
// shared views know nothing of who looks at them (MP-02.1). In a presence
// row the own face comes first with an accent ring, and shows even when it
// is past the eighth, which then gives its place.
func meStyle(me string) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		if !people.ValidID(me) {
			return nil
		}
		who := `li[data-who="` + me + `"]`
		_, err := io.WriteString(w, `<style>.tk-presence>`+who+`{order:-1;display:revert}`+
			`.tk-presence>`+who+`>.tk-tip{outline:2px solid var(--tk-accent);outline-offset:2px}`+
			`.tk-presence:has(>`+who+`[data-overflow])>li:nth-child(`+strconv.Itoa(Shown)+`){display:none}</style>`)
		return err
	})
}

// Shown is how many faces a presence row shows (MP-02.1).
const Shown = 8

// Present is a person in a presence row, with the number of their tabs on
// it (MP-01.1).
type Present struct {
	people.Person
	Tabs int
}

// entries are the presence row's people as internal/ui draws them.
func entries(list []Present) []ui.PresenceEntry {
	out := make([]ui.PresenceEntry, len(list))
	for i, p := range list {
		out[i] = ui.PresenceEntry{Person: UIPerson(p.Person), Tabs: p.Tabs}
	}
	return out
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
