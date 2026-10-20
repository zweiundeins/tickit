package web

import (
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"testing"
)

// TestVendored checks the Starbase files against lock.json, without
// Starbase's checkout: every file the lock lists is there with its SHA-384,
// and the fonts are the ones it records (NFR-29: all of it served by the
// app).
func TestVendored(t *testing.T) {
	b, err := fs.ReadFile(staticFiles, "static/vendor/starbase/lock.json")
	if err != nil {
		t.Fatal(err)
	}
	var lock struct {
		Commit     string            `json:"commit"`
		Datastar   string            `json:"datastar"`
		Fonts      map[string]string `json:"fonts"`
		Components []struct {
			Slug  string            `json:"slug"`
			Files map[string]string `json:"files"`
		} `json:"components"`
	}
	if err := json.Unmarshal(b, &lock); err != nil {
		t.Fatal(err)
	}
	sri := func(name string) string {
		b, err := fs.ReadFile(staticFiles, name)
		if err != nil {
			t.Fatal(err)
		}
		sum := sha512.Sum384(b)
		return "sha384-" + base64.StdEncoding.EncodeToString(sum[:])
	}
	if lock.Commit == "" || len(lock.Components) == 0 {
		t.Fatal("lock.json names no commit or no components")
	}
	if got := sri("static/vendor/datastar-rocket.js"); got != lock.Datastar {
		t.Errorf("datastar-rocket.js is %s, the lock says %s", got, lock.Datastar)
	}
	for _, c := range lock.Components {
		for name, want := range c.Files {
			if got := sri("static/vendor/starbase/" + c.Slug + "/" + name); got != want {
				t.Errorf("%s/%s is %s, the lock says %s", c.Slug, name, got, want)
			}
		}
	}
	for name, want := range lock.Fonts {
		if got := sri("static/fonts/" + name); got != want {
			t.Errorf("fonts/%s is %s, the lock says %s", name, got, want)
		}
	}
}
