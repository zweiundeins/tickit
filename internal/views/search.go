package views

import (
	"net/url"
	"strconv"
	"time"

	"github.com/a-h/templ"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/search"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// Search (FR-57): the results page, whose results are a shared view keyed
// by the query and the scopes its person reads, and the typeahead under
// the header's search box.

// SearchResultsID is the id of the results' region, which its view patches.
const SearchResultsID = "search-results"

// SearchSuggestID is the id of the typeahead's panel under the header's
// search box: GET /search/suggest?q= patches it.
const SearchSuggestID = "search-suggest"

// SearchParams are a search as its URL has it: q, kind, project (keys),
// space (keys), label, person and page.
type SearchParams struct {
	Q        string
	Kind     string // "issue", "page" or "" for both
	Projects []string
	Spaces   []string
	Labels   []string
	People   []string
	Page     int
}

// Values are the params as URL parameters.
func (p SearchParams) Values() url.Values {
	v := url.Values{}
	if p.Q != "" {
		v.Set("q", p.Q)
	}
	if p.Kind != "" {
		v.Set("kind", p.Kind)
	}
	for k, list := range map[string][]string{"project": p.Projects, "space": p.Spaces, "label": p.Labels, "person": p.People} {
		for _, x := range list {
			v.Add(k, x)
		}
	}
	if p.Page > 1 {
		v.Set("page", strconv.Itoa(p.Page))
	}
	return v
}

// Href is the results page of the params, with one of them changed.
func (p SearchParams) Href(change func(*SearchParams)) string {
	if change != nil {
		change(&p)
	}
	return "/search?" + p.Values().Encode()
}

// SearchResults is a page of results as their shared view renders them.
type SearchResults struct {
	Params  SearchParams
	Results search.Results
	Issues  map[string]tracker.Issue // the issues among the hits, by id
	Spaces  map[string]string        // space key to name, for the pages among them
	People  map[string]people.Person
	Now     time.Time
}

// PageHref is the address of page n of the results.
func (r SearchResults) PageHref(n int) string {
	return r.Params.Href(func(p *SearchParams) { p.Page = n })
}

func (r SearchResults) pages() int {
	return max(1, (r.Results.Total+search.PageSize-1)/search.PageSize)
}

func hitCount(n int) string {
	if n == 1 {
		return "1 result"
	}
	return strconv.Itoa(n) + " results"
}

// SearchPage is the results page: the form, the filters and the results.
type SearchPage struct {
	Results SearchResults
	// The filters' values from the model.
	Projects []tracker.Project
	Spaces   []string // keys
	Labels   []string
	People   []string // ids
	Names    map[string]people.Person
	// Me and Moderates are for the issue panel (views.IssuePanel).
	Me        string
	Moderates bool
	Deletes   Deletes
}

// Suggestions are the typeahead's first issues and pages for a text.
type Suggestions struct {
	Q      string
	Issues []search.Hit
	Pages  []search.Hit
	Spaces map[string]string // space key to name
}

// spaceName is the name of the space a page's hit names by its key.
func spaceName(names map[string]string, key string) string {
	if n := names[key]; n != "" {
		return n
	}
	return key
}

func (s Suggestions) count() string {
	n := len(s.Issues) + len(s.Pages)
	switch n {
	case 0:
		return "No issues or pages match"
	case 1:
		return "1 suggestion"
	}
	return strconv.Itoa(n) + " suggestions"
}

func (s Suggestions) allHref() string { return "/search?" + url.Values{"q": {s.Q}}.Encode() }

// The typeahead's expressions, on the header's search box (SearchBoxAttrs).
const (
	// suggest asks for the first hits while the person types, at most
	// every 300 ms; under two characters it empties the panel.
	suggest = `el.value.trim().length < 2 ? document.getElementById('` + SearchSuggestID + `').hidePopover() : @get('/search/suggest?q=' + encodeURIComponent(el.value.trim()), {requestCancellation: 'auto'})`
	// suggestKeys: Down moves into the suggestions, Escape empties them.
	suggestKeys = `evt.key === 'ArrowDown' && document.getElementById('` + SearchSuggestID + `').matches(':popover-open') && (evt.preventDefault(), document.querySelector('#` + SearchSuggestID + ` a')?.focus())`
	// suggestShown opens the panel, in the top layer, once an answer
	// brought suggestions.
	suggestShown = `evt.detail.type === 'finished' && ((s) => s.querySelector('a') && el.value.trim().length >= 2 ? s.matches(':popover-open') || s.showPopover() : s.hidePopover())(document.getElementById('` + SearchSuggestID + `'))`
	// suggestNav moves through the suggestions with Up and Down, Escape
	// goes back to the box. On the panel.
	suggestNav = `['ArrowDown', 'ArrowUp', 'Escape'].includes(evt.key) && ((items) => {
	evt.preventDefault()
	const box = document.getElementById('tk-search')
	if (evt.key === 'Escape') { el.hidePopover(); box.focus(); return }
	const i = items.indexOf(document.activeElement)
	const next = evt.key === 'ArrowDown' ? items[i + 1] : i > 0 ? items[i - 1] : box
	next?.focus()
})([...el.querySelectorAll('a')])`
)

// SearchBoxAttrs are what the header's search box carries for the
// typeahead (FR-57.1): the panel SearchSuggestSlot is its popup.
func SearchBoxAttrs() templ.Attributes {
	return templ.Attributes{
		"data-on:input__throttle.300ms.trailing": suggest,
		"data-on:keydown":                        suggestKeys,
		"data-on:datastar-fetch":                 suggestShown,
		"aria-controls":                          SearchSuggestID,
		"aria-describedby":                       SearchSuggestID + "-count",
	}
}
