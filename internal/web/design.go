package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/zweiundeins/lith/edge"

	"github.com/mbolli/tickit/internal/ui"
	"github.com/mbolli/tickit/internal/ui/styleguide"
)

// Stylesheets are the design system's CSS files in the order of the
// bundle, css/tickit.css: tokens first (they declare the layer order), then
// Starbase's mapping, base, layout, the components, one file per area, the
// utilities. A new file goes here. Pages link the bundle alone.
var Stylesheets = []string{
	"css/tokens.css",
	"css/starbase.css",
	"css/base.css",
	"css/layout.css",
	"css/components.css",
	"css/shell.css",
	"css/prose.css",
	"css/editor.css",
	"css/board.css",
	"css/utilities.css",
}

// Bundle is the name the joined stylesheets are served under.
const Bundle = "css/tickit.css"

// Fonts are the font files a page preloads.
var Fonts = []string{"fonts/jetbrains-mono.woff2", "fonts/pixelify-sans.woff2"}

var fontURL = regexp.MustCompile(`url\("\.\./fonts/([a-z0-9-]+\.woff2)"\)`)

// BundleCSS joins Stylesheets into one stylesheet, with the fonts' URLs
// rewritten to their hashed names (url), so the stylesheet and its
// preloads ask for the same immutable files. edge.Static minifies it.
func BundleCSS(files fs.FS, url func(string) string) ([]byte, error) {
	var b bytes.Buffer
	for _, name := range Stylesheets {
		css, err := fs.ReadFile(files, name)
		if err != nil {
			return nil, err
		}
		b.WriteString(fontURL.ReplaceAllStringFunc(string(css), func(m string) string {
			return `url("` + url("fonts/"+fontURL.FindStringSubmatch(m)[1]) + `")`
		}))
		b.WriteByte('\n')
	}
	return b.Bytes(), nil
}

// starbaseLock is the part of vendor/starbase/lock.json that loading a
// component needs (cmd/vendor-starbase writes it).
type starbaseLock struct {
	Components []struct {
		Slug    string   `json:"slug"`
		Entry   string   `json:"entry"`
		Renders []string `json:"renders"`
	} `json:"components"`
}

// StarbaseScripts returns the static names of the modules to load for the
// Starbase components (slugs such as "dropdown"): each one's, after those
// of the components it renders, each once. A page loads them as
// <script type="module"> after its import map.
func StarbaseScripts(files fs.FS, slugs ...string) ([]string, error) {
	b, err := fs.ReadFile(files, "vendor/starbase/lock.json")
	if err != nil {
		return nil, err
	}
	var lock starbaseLock
	if err := json.Unmarshal(b, &lock); err != nil {
		return nil, fmt.Errorf("vendor/starbase/lock.json: %w", err)
	}
	entry := map[string]int{}
	for i, c := range lock.Components {
		entry[c.Slug] = i
	}
	var out []string
	seen := map[string]bool{}
	var add func(string) error
	add = func(slug string) error {
		if seen[slug] {
			return nil
		}
		seen[slug] = true
		i, ok := entry[slug]
		if !ok {
			return fmt.Errorf("starbase: %s is not vendored: add it to vendor/starbase/pins.json and run go tool task vendor-starbase", slug)
		}
		for _, d := range lock.Components[i].Renders {
			if err := add(d); err != nil {
				return err
			}
		}
		out = append(out, "vendor/starbase/"+slug+"/"+lock.Components[i].Entry)
		return nil
	}
	for _, s := range slugs {
		if err := add(s); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// designRoutes serves the styleguide at /design. It also adds what every
// page of the design system loads to the static files: the stylesheet
// bundle and the mark as the favicon (art/mark.svg). (Until the app's own
// pages are built, they live here; New is the place for them then.)
func (s *Server) designRoutes(mux *http.ServeMux) {
	files, err := fs.Sub(staticFiles, "static")
	if err != nil {
		panic(err)
	}
	css, err := BundleCSS(files, s.static.URL)
	if err != nil {
		panic("web: the stylesheet bundle: " + err.Error()) // embedded files: a build error
	}
	s.static.Add(Bundle, css)
	s.static.Add("art/mark.svg", []byte(ui.MarkSVG()))
	scripts, err := StarbaseScripts(files, styleguide.Components...)
	if err != nil {
		panic(err)
	}
	mux.HandleFunc("GET /design", func(w http.ResponseWriter, r *http.Request) { s.design(w, r, scripts) })
}

// design renders the styleguide in the theme, style and density of its
// query (?theme=light&style=smooth&density=compact): no server state.
func (s *Server) design(w http.ResponseWriter, r *http.Request, scripts []string) {
	q := r.URL.Query()
	prefs := ui.Prefs{Theme: q.Get("theme"), Style: q.Get("style"), Density: q.Get("density")}.Normal()
	datastar := s.static.URL("vendor/datastar-rocket.js")
	im, _ := json.Marshal(map[string]any{"imports": map[string]string{"datastar": datastar}}) // escapes <, > and &
	p := styleguide.Props{
		Prefs:     prefs,
		Nonce:     edge.Nonce(r.Context()),
		CSS:       s.static.URL(Bundle),
		DesignCSS: s.static.URL("css/design.css"),
		Mark:      s.static.URL("art/mark.svg"),
		Datastar:  datastar,
		ImportMap: string(im),
		Now:       time.Now(),
		Href: func(key, value string) string {
			v := url.Values{"theme": {prefs.Theme}, "style": {prefs.Style}, "density": {prefs.Density}}
			v.Set(key, value)
			return "/design?" + v.Encode()
		},
	}
	for _, f := range Fonts {
		p.Fonts = append(p.Fonts, s.static.URL(f))
	}
	for _, name := range scripts {
		p.Scripts = append(p.Scripts, s.static.URL(name))
	}
	var buf bytes.Buffer
	if err := styleguide.Page(p).Render(r.Context(), &buf); err != nil {
		s.fail(w, r, err)
		return
	}
	h := w.Header()
	h.Set("Content-Type", "text/html; charset=utf-8")
	h.Set("Cache-Control", "no-store")
	w.Write(buf.Bytes())
}
