package wiki

import (
	"cmp"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/zweiundeins/lith/writer"

	"github.com/mbolli/tickit/internal/demo/seed"
	"github.com/mbolli/tickit/internal/people"
	"github.com/mbolli/tickit/internal/tabs"
)

// SeedCommands are the commands that write the seed's spaces, pages,
// versions, labels and comments (DM-02.2) as its people, at the times the
// seed gives relative to run, in an order that makes every parent before
// its children. Run them in order, after people.SeedPeople; issue chips
// show their issues once the tracker's seed ran too.
func SeedCommands(c *seed.Content, run time.Time) ([]writer.Command, error) {
	ids := map[string]string{} // "KEY/Title" to a page id
	for _, p := range c.Pages {
		ids[p.Space+"/"+p.Title] = NewID()
	}
	pageID := func(space, title string) string { return ids[space+"/"+title] }
	n := 0
	by := func(person string, at seed.When) tabs.By {
		n++
		p := people.Person{ID: person, Kind: people.Seed, Name: person}
		for _, s := range people.Seeded {
			if s.ID == person {
				p = s
			}
		}
		return tabs.By{Actor: p, Op: "seed-" + strconv.Itoa(n), At: at.Time(run)}
	}
	parse := func(space, body string) (json.RawMessage, error) {
		return seed.Parse(body, c.Resolver(space, pageID))
	}
	var cmds []writer.Command
	for _, sp := range c.Spaces {
		pages := slices.DeleteFunc(slices.Clone(c.Pages), func(p seed.Page) bool { return p.Space != sp.Key })
		home := slices.IndexFunc(pages, func(p seed.Page) bool { return p.Home })
		if home < 0 {
			return nil, fmt.Errorf("wiki: space %s has no home page", sp.Key)
		}
		h := pages[home]
		spaceID := NewID()
		first, err := parse(sp.Key, h.Versions[0].Body)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", h.File, err)
		}
		cmds = append(cmds,
			CreateSpace{By: by(h.Author, h.Created), ID: spaceID, Home: ids[sp.Key+"/"+h.Title], Key: sp.Key, Name: sp.Name, Description: sp.Description, Open: true, HomeDoc: first},
			RenamePage{By: by(h.Author, h.Created), Page: ids[sp.Key+"/"+h.Title], Title: h.Title})
		// Parents first, siblings by position.
		depth := func(p seed.Page) int {
			d := 0
			for cur := p; cur.Parent != "" && d < 10; d++ {
				i := slices.IndexFunc(pages, func(x seed.Page) bool { return x.Title == cur.Parent })
				if i < 0 {
					break
				}
				cur = pages[i]
			}
			return d
		}
		order := slices.Clone(pages)
		slices.SortStableFunc(order, func(a, b seed.Page) int {
			return cmp.Or(cmp.Compare(depth(a), depth(b)), cmp.Compare(a.Parent, b.Parent), cmp.Compare(a.Position, b.Position))
		})
		for _, p := range order {
			id := ids[sp.Key+"/"+p.Title]
			if !p.Home {
				parent := ""
				if p.Parent != "" {
					parent = ids[sp.Key+"/"+p.Parent]
				} else {
					parent = ids[sp.Key+"/"+h.Title]
				}
				d, err := parse(sp.Key, p.Versions[0].Body)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", p.File, err)
				}
				cmds = append(cmds, CreatePage{By: by(p.Versions[0].By, p.Versions[0].At), ID: id, Space: spaceID, Parent: parent, Title: p.Title, Doc: d, Message: p.Versions[0].Message})
			}
			for _, v := range p.Versions[1:] {
				d, err := parse(sp.Key, v.Body)
				if err != nil {
					return nil, fmt.Errorf("%s version %d: %w", p.File, v.Number, err)
				}
				cmds = append(cmds, SetDocument{By: by(v.By, v.At), Page: id, Doc: d, Message: v.Message})
			}
			if len(p.Labels) > 0 {
				last := p.Versions[len(p.Versions)-1]
				cmds = append(cmds, SetPageLabels{By: by(last.By, last.At), Page: id, Add: p.Labels})
			}
		}
		for _, cm := range c.PageComments {
			if cm.Space != sp.Key {
				continue
			}
			page := ids[sp.Key+"/"+cm.Page]
			body, err := parse(sp.Key, string(cm.Body))
			if err != nil {
				return nil, err
			}
			cid := NewID()
			cmds = append(cmds, AddComment{By: by(cm.By, cm.At), ID: cid, Page: page, Body: body})
			for _, r := range cm.Replies {
				rb, err := parse(sp.Key, string(r.Body))
				if err != nil {
					return nil, err
				}
				cmds = append(cmds, AddComment{By: by(r.By, r.At), Page: page, Parent: cid, Body: rb})
			}
		}
	}
	return cmds, nil
}
