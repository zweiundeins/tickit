package images

import (
	"sync"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/demo/seed"
)

// The image library (FR-84), from the seed's images.json.
var library = sync.OnceValue(func() map[string]seed.Image {
	m := map[string]seed.Image{}
	c, err := seed.Load()
	if err != nil {
		return m
	}
	for _, im := range c.Images {
		m[im.ID] = im
	}
	return m
})

// Library is the library's images in the seed's order.
func Library() []seed.Image {
	c, err := seed.Load()
	if err != nil {
		return nil
	}
	return c.Images
}

// InLibrary reports whether id is in the image library.
func InLibrary(id string) bool { _, ok := library()[id]; return ok }

// Allowed reports which image ids a document may hold in a command in tx:
// the library's and the uploaded ones (FR-85); with tx nil, the library's.
func Allowed(tx *writer.Tx) func(id string) bool {
	return func(id string) bool {
		if InLibrary(id) {
			return true
		}
		if tx == nil {
			return false
		}
		ok, _ := Exists(tx.Context(), tx, id)
		return ok
	}
}
