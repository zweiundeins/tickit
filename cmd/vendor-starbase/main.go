// Command vendor-starbase copies Starbase's components and fonts into
// tickit, from a local checkout of github.com/zweiundeins/starbase. It reads
// the wanted components and fonts from
// internal/web/static/vendor/starbase/pins.json, copies each component's
// module (components/<slug>/<slug>.js) and the modules of the components it
// renders, the fonts with their OFL texts into internal/web/static/fonts,
// and Starbase's MIT licence, and writes lock.json and VERSION: the commit,
// and every file with its SHA-384. It refuses a checkout with uncommitted
// changes in what it copies, and one whose Datastar build is not the one
// tickit vendors (internal/web/static/vendor/datastar-rocket.js).
//
//	go run ./cmd/vendor-starbase -src /develop/starbase    (go tool task vendor-starbase)
//
// Run it again after a change to pins.json or to update Starbase: it
// rewrites the components' folders, the fonts, lock.json and VERSION. The
// modules stay as Starbase wrote them; edge.Static minifies them in
// production like tickit's own scripts. Each imports only 'datastar', which
// the page's import map resolves.
package main

import (
	"bytes"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

const (
	dir      = "internal/web/static/vendor/starbase"
	fontDir  = "internal/web/static/fonts"
	datastar = "internal/web/static/vendor/datastar-rocket.js"
)

// Pins is what to vendor: pins.json.
type Pins struct {
	Repo       string   `json:"repo"`
	Components []string `json:"components"`
	Fonts      []string `json:"fonts"`
}

// Lock is what was vendored: lock.json. internal/web reads it to load a
// component with the components it renders.
type Lock struct {
	Repo     string            `json:"repo"`
	Commit   string            `json:"commit"`
	Date     string            `json:"date"`
	License  string            `json:"license"`
	Datastar string            `json:"datastar"`
	Fonts    map[string]string `json:"fonts"`
	// Components by slug, sorted.
	Components []Component `json:"components"`
}

// Component is one vendored component.
type Component struct {
	Slug string `json:"slug"`
	Tag  string `json:"tag"`
	// Pinned: listed in pins.json; else vendored because a pinned one
	// renders it.
	Pinned  bool     `json:"pinned"`
	Entry   string   `json:"entry"`
	Renders []string `json:"renders,omitempty"`
	// Files are the component's files under its folder with their SRI.
	Files map[string]string `json:"files"`
}

var (
	tagRe    = regexp.MustCompile(`<(sb-[a-z0-9]+(?:-[a-z0-9]+)*)`)
	importRe = regexp.MustCompile(`(?:from|import)\s*\(?\s*["'](\./[^"']+)["']`)
	bareRe   = regexp.MustCompile(`(?:from|import)\s*\(?\s*["']([^./][^"']*)["']`)
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("vendor-starbase: ")
	src := flag.String("src", os.Getenv("STARBASE"), "a checkout of "+"github.com/zweiundeins/starbase (or $STARBASE)")
	dirty := flag.Bool("dirty", false, "copy from a checkout with uncommitted changes (the lock then says so)")
	flag.Parse()
	if *src == "" {
		log.Fatal("name Starbase's checkout with -src or $STARBASE")
	}
	if err := run(*src, *dirty); err != nil {
		log.Fatal(err)
	}
}

func run(src string, allowDirty bool) error {
	var pins Pins
	b, err := os.ReadFile(filepath.Join(dir, "pins.json"))
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, &pins); err != nil {
		return fmt.Errorf("pins.json: %w", err)
	}
	commit, err := git(src, "rev-parse", "HEAD")
	if err != nil {
		return err
	}
	date, err := git(src, "show", "-s", "--format=%cI", "HEAD")
	if err != nil {
		return err
	}
	status, err := git(src, "status", "--porcelain", "--", "components", "static/vendor", "static/fonts", "LICENSE")
	if err != nil {
		return err
	}
	if status != "" {
		if !allowDirty {
			return fmt.Errorf("%s has uncommitted changes in what this copies:\n%s\ncommit them, or pass -dirty", src, status)
		}
		commit += "-dirty"
	}

	theirs, err := os.ReadFile(filepath.Join(src, "static/vendor/datastar-rocket.js"))
	if err != nil {
		return err
	}
	ours, err := os.ReadFile(datastar)
	if err != nil {
		return err
	}
	if integrity(theirs) != integrity(ours) {
		return fmt.Errorf("Starbase at %s ships Datastar %s, tickit vendors %s: update %s and its VERSION first", commit, integrity(theirs), integrity(ours), datastar)
	}

	lock := Lock{Repo: pins.Repo, Commit: commit, Date: date, License: "MIT", Datastar: integrity(ours), Fonts: map[string]string{}}
	out := map[string][]byte{}
	queue := slices.Clone(pins.Components)
	done := map[string]*Component{}
	for len(queue) > 0 {
		slug := queue[0]
		queue = queue[1:]
		if done[slug] != nil {
			continue
		}
		c := &Component{Slug: slug, Tag: "sb-" + slug, Pinned: slices.Contains(pins.Components, slug), Entry: slug + ".js", Files: map[string]string{}}
		done[slug] = c
		folder := filepath.Join(src, "components", slug)
		pending := []string{c.Entry}
		for len(pending) > 0 {
			p := pending[0]
			pending = pending[1:]
			if c.Files[p] != "" {
				continue
			}
			b, err := os.ReadFile(filepath.Join(folder, filepath.FromSlash(p)))
			if err != nil {
				return fmt.Errorf("%s: %w", slug, err)
			}
			for _, m := range bareRe.FindAllStringSubmatch(string(b), -1) {
				if m[1] != "datastar" {
					return fmt.Errorf("%s/%s imports %q: only 'datastar' and files in its folder can be vendored", slug, p, m[1])
				}
			}
			c.Files[p] = integrity(b)
			out[slug+"/"+p] = b
			for _, m := range importRe.FindAllStringSubmatch(string(b), -1) {
				q := path.Clean(path.Join(path.Dir(p), m[1]))
				if strings.HasPrefix(q, "../") {
					return fmt.Errorf("%s/%s imports %s, outside its folder", slug, p, m[1])
				}
				pending = append(pending, q)
			}
			for _, m := range tagRe.FindAllStringSubmatch(string(b), -1) {
				dep := strings.TrimPrefix(m[1], "sb-")
				if dep == slug || slices.Contains(c.Renders, dep) {
					continue
				}
				if _, err := os.Stat(filepath.Join(src, "components", dep, dep+".js")); err != nil {
					continue // a tag in its docs or an example, not a component
				}
				c.Renders = append(c.Renders, dep)
				queue = append(queue, dep)
			}
		}
	}
	for _, c := range done {
		slices.Sort(c.Renders)
		lock.Components = append(lock.Components, *c)
	}
	slices.SortFunc(lock.Components, func(a, b Component) int { return strings.Compare(a.Slug, b.Slug) })

	// Rewrite the components' folders.
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
				return err
			}
		}
	}
	for name, b := range out {
		if err := write(filepath.Join(dir, filepath.FromSlash(name)), b); err != nil {
			return err
		}
	}
	license, err := os.ReadFile(filepath.Join(src, "LICENSE"))
	if err != nil {
		return err
	}
	if err := write(filepath.Join(dir, "LICENSE-starbase.txt"), license); err != nil {
		return err
	}
	for _, name := range pins.Fonts {
		b, err := os.ReadFile(filepath.Join(src, "static/fonts", name))
		if err != nil {
			return err
		}
		lock.Fonts[name] = integrity(b)
		if err := write(filepath.Join(fontDir, name), b); err != nil {
			return err
		}
	}

	j, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	if err := write(filepath.Join(dir, "lock.json"), append(j, '\n')); err != nil {
		return err
	}
	if err := write(filepath.Join(dir, "VERSION"), version(lock)); err != nil {
		return err
	}
	var n int
	for _, c := range lock.Components {
		n += len(c.Files)
	}
	log.Printf("%d components (%d files) and %d font files from %s at %s", len(lock.Components), n, len(lock.Fonts), lock.Repo, lock.Commit)
	return nil
}

// version is VERSION: what lock.json says, for people.
func version(l Lock) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "Starbase's Rocket components and fonts, copied by cmd/vendor-starbase from\n%s at commit %s (%s).\n", l.Repo, l.Commit, l.Date)
	fmt.Fprintf(&b, "Licence: %s, LICENSE-starbase.txt. The fonts are SIL OFL 1.1 (static/fonts/OFL-*.txt).\n", l.License)
	fmt.Fprintf(&b, "Update: change pins.json, then go run ./cmd/vendor-starbase -src <checkout>.\n\n")
	for _, c := range l.Components {
		how := "pinned"
		if !c.Pinned {
			how = "rendered by a pinned one"
		}
		fmt.Fprintf(&b, "%s\t%s\t%s\n", c.Tag, c.Files[c.Entry], how)
	}
	return b.Bytes()
}

func write(name string, b []byte) error {
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		return err
	}
	return os.WriteFile(name, b, 0o644)
}

func git(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// integrity is b's SRI: sha384, base64.
func integrity(b []byte) string {
	sum := sha512.Sum384(b)
	return "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
}
