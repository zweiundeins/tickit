package seed

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/zweiundeins/tickit/internal/markup"
)

// Resolver answers the references a markup text makes while Parse converts
// it. Content.Resolver builds one from the loaded seed.
type Resolver = markup.Resolver

// Parse converts the seed's markup (see the package documentation) into a
// ProseMirror document of tickit's schema, as JSON, strictly: what the
// markup does not define is an error (internal/markup).
func Parse(text string, resolve Resolver) (json.RawMessage, error) {
	return markup.Parse(text, resolve)
}

// headingRe matches a heading line of the markup.
var headingRe = regexp.MustCompile(`^(#{1,6}) (.*)$`)

func indentOf(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }
