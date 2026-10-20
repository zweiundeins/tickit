package seed

import (
	"regexp"
	"slices"
)

var crossSpaceRe = regexp.MustCompile(`^([A-Z]{2,10})/(.+)$`)

// Resolver returns a Resolver for markup of the space with the given key ("" for
// issues and comments on them, whose page links must name a space). pageID
// gives the id of a page; nil gives "<space>/<title>".
func (c *Content) Resolver(space string, pageID func(space, title string) string) Resolver {
	if pageID == nil {
		pageID = func(space, title string) string { return space + "/" + title }
	}
	return &resolver{c: c, space: space, pageID: pageID}
}

type resolver struct {
	c      *Content
	space  string
	pageID func(space, title string) string
}

func (r *resolver) Issue(key string) bool { return r.c.Issue(key) != nil }

func (r *resolver) Person(id string) (string, bool) {
	if p := r.c.Person(id); p != nil {
		return p.Name, true
	}
	return "", false
}

func (r *resolver) Page(ref string) (string, bool) {
	space, title := r.space, ref
	if m := crossSpaceRe.FindStringSubmatch(ref); m != nil && slices.ContainsFunc(r.c.Spaces, func(s Space) bool { return s.Key == m[1] }) {
		space, title = m[1], m[2]
	}
	if space == "" || r.c.Page(space, title) == nil {
		return "", false
	}
	return r.pageID(space, title), true
}

func (r *resolver) Image(id string) bool {
	return slices.ContainsFunc(r.c.Images, func(im Image) bool { return im.ID == id })
}
