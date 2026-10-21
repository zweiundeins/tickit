package e2e

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/wiki"
)

// The rows sb-tree draws: label, link, level, open state, current, focus.
const treeRows = `[...document.querySelector('#page-tree sb-tree').shadowRoot.querySelectorAll('[role=treeitem]')].map((r) => ({
	id: r.dataset.id, label: r.querySelector('.label').textContent, href: r.getAttribute('href') || '', level: +r.getAttribute('aria-level'),
	open: r.getAttribute('aria-expanded') || '', current: r.getAttribute('aria-current') || '', tab: r.tabIndex,
	focused: r === r.getRootNode().activeElement && document.activeElement === r.getRootNode().host}))`

type treeRow struct {
	ID, Label, Href, Open, Current string
	Level, Tab                     int
	Focused                        bool
}

func rows(t *testing.T, p *browsertest.Page) []treeRow {
	t.Helper()
	var out []treeRow
	p.Eval(t, treeRows, &out)
	return out
}

// treeKey presses a key in the tree: sb-tree moves the focus a frame later.
func treeKey(t *testing.T, p *browsertest.Page, name string) {
	t.Helper()
	key(t, p, name)
	time.Sleep(60 * time.Millisecond)
}

func focusedRow(t *testing.T, p *browsertest.Page) treeRow {
	t.Helper()
	for _, r := range rows(t, p) {
		if r.Focused {
			return r
		}
	}
	return treeRow{}
}

// FR-78, NFR-47.2: the space's page tree is sb-tree over the server's
// lists: its rows are links, the page shown is aria-current with its
// branch open and in the tab order, the arrow keys move and open and close
// branches, Enter follows a link, a frame keeps what the reader closed,
// and nothing scrolls sideways at 320, 768 and 1280 pixels.
func TestPageTree(t *testing.T) {
	e, _ := seededDemo(t)
	var page string
	e.DB.R.QueryRow(`SELECT id FROM pages WHERE title = 'Deploy to production'`).Scan(&page)
	title := "Deploy to production"
	b := browsertest.Start(t)
	p := b.NewPage(t, 1280, 900)
	p.Navigate(t, e.URL+wiki.PageURL("ENG", page, title))
	p.Wait(t, `document.getElementById('reconnecting')?.hidden && document.querySelector('#page-tree sb-tree')?.shadowRoot?.querySelector('[aria-current=page]')`, 10*time.Second)

	rs := rows(t, p)
	t.Logf("tree: %+v", rs)
	var cur treeRow
	for _, r := range rs {
		if r.Current == "page" {
			cur = r
		}
	}
	if cur.Label != title || cur.Tab != 0 || cur.Href == "" {
		t.Fatalf("the current row is %+v", cur)
	}
	var path string
	p.Eval(t, `location.pathname`, &path)
	if cur.Href != path {
		t.Errorf("the current row links to %s, the page is %s", cur.Href, path)
	}
	for _, r := range rs {
		if r.Tab == 0 && r.ID != cur.ID {
			t.Errorf("%s is in the tab order too", r.Label)
		}
	}

	// The keyboard: the focus lands on the current row, Up and Down move.
	p.Eval(t, `(document.querySelector('#page-tree sb-tree').shadowRoot.querySelector('[tabindex="0"]').focus(), true)`, nil)
	if f := focusedRow(t, p); f.ID != cur.ID {
		t.Fatalf("the focus is on %+v", f)
	}
	treeKey(t, p, "Home")
	first := focusedRow(t, p)
	if first.ID != rs[0].ID {
		t.Errorf("Home went to %+v", first)
	}
	treeKey(t, p, "ArrowDown")
	if f := focusedRow(t, p); f.ID != rs[1].ID {
		t.Errorf("Down went to %+v, want %s", f, rs[1].Label)
	}
	treeKey(t, p, "ArrowUp")

	// Left closes the first branch, Right opens it again; the rows below
	// it go and come back.
	var branch treeRow
	for _, r := range rs {
		if r.Open == "true" {
			branch = r
			break
		}
	}
	if branch.ID == "" {
		t.Fatal("no open branch")
	}
	for focusedRow(t, p).ID != branch.ID {
		treeKey(t, p, "ArrowDown")
	}
	treeKey(t, p, "ArrowLeft")
	p.Wait(t, treeRows+`.find((r) => r.id === `+jsQuote(branch.ID)+`).open === 'false'`, time.Second)
	closed := len(rows(t, p))
	if closed >= len(rs) {
		t.Errorf("closing %s left %d rows of %d", branch.Label, closed, len(rs))
	}
	treeKey(t, p, "ArrowRight")
	p.Wait(t, treeRows+`.length === `+fmt.Sprint(len(rs)), time.Second)

	// A frame (a page renamed elsewhere) keeps a branch the reader closed.
	treeKey(t, p, "ArrowLeft")
	p.Wait(t, treeRows+`.length === `+fmt.Sprint(closed), time.Second)
	ada := people.Seeded[0]
	e.Do(t, wiki.RenamePage{By: tabs.By{Actor: ada, Op: "rn1", At: time.Now()}, Page: rs[0].ID, Title: "Renamed elsewhere"})
	p.Wait(t, treeRows+`.some((r) => r.label === 'Renamed elsewhere')`, 5*time.Second)
	time.Sleep(200 * time.Millisecond)
	if n := len(rows(t, p)); n != closed {
		t.Errorf("the frame changed the tree from %d rows to %d", closed, n)
	}
	if f := focusedRow(t, p); f.ID != branch.ID {
		t.Errorf("after the frame the focus is on %+v", f)
	}

	// Enter follows the focused link.
	treeKey(t, p, "ArrowRight")
	treeKey(t, p, "ArrowDown")
	next := focusedRow(t, p)
	enter(t, p)
	p.Wait(t, `location.pathname === `+jsQuote(next.Href), 5*time.Second)
	p.Wait(t, `document.querySelector('#page-tree sb-tree')?.shadowRoot?.querySelector('[aria-current=page] .label')?.textContent === `+jsQuote(next.Label), 5*time.Second)
	noErrors(t, p)

	for _, w := range []int{320, 768, 1280} {
		q := b.NewPage(t, w, 800)
		q.Navigate(t, e.URL+wiki.PageURL("ENG", page, title))
		q.Wait(t, `document.querySelector('#page-tree sb-tree')?.shadowRoot?.querySelector('[aria-current=page]')`, 10*time.Second)
		time.Sleep(300 * time.Millisecond)
		// Narrower, the sidebar is in the menu's panel.
		q.Eval(t, `([...document.querySelectorAll('#page-tree sb-tree')].some((x) => x.checkVisibility()) || document.getElementById('tk-nav').showModal(), true)`, nil)
		time.Sleep(300 * time.Millisecond)
		var bad []string
		q.Eval(t, `(() => {
			const out = [], h = [...document.querySelectorAll('#page-tree sb-tree')].find((x) => x.checkVisibility())
			if (!h) return ['no tree in view']
			if (document.documentElement.scrollWidth > innerWidth) out.push('the page is ' + document.documentElement.scrollWidth + ' px wide')
			const box = h.getBoundingClientRect()
			for (const r of h.shadowRoot.querySelectorAll('[role=treeitem]')) {
				const rr = r.getBoundingClientRect()
				if (rr.width && (rr.right > box.right + 1 || rr.left < box.left - 1)) out.push(r.textContent.trim() + ' overflows the tree')
			}
			return out
		})()`, &bad)
		for _, s := range bad {
			t.Errorf("%d px: %s", w, s)
		}
		shoot(t, q, fmt.Sprintf("page-tree-%d", w))
		noErrors(t, q)
		q.Close(t)
	}
}

// FR-79.1, NFR-47.2: the Move dialog picks the new place in a tree of the
// spaces and their pages: it opens on where the page is now, the keyboard
// moves to another page and Enter picks it, and Move moves the page there.
func TestMoveDialogTree(t *testing.T) {
	e, _ := seededDemo(t)
	var page string
	e.DB.R.QueryRow(`SELECT id FROM pages WHERE title = 'Deploy to production'`).Scan(&page)
	b := browsertest.Start(t)
	p := b.NewPage(t, 1280, 900)
	p.Navigate(t, e.URL+wiki.PageURL("ENG", page, "Deploy to production"))
	p.Wait(t, `document.getElementById('reconnecting')?.hidden && customElements.get('sb-tree')`, 10*time.Second)
	p.Eval(t, `(document.querySelector('[commandfor=move]').click(), true)`, nil)
	tree := `document.getElementById('move-target')`
	p.Wait(t, `document.getElementById('move').open && `+tree+`.shadowRoot?.querySelector('[aria-selected=true]')`, 5*time.Second)
	var sel string
	p.Eval(t, tree+`.shadowRoot.querySelector('[aria-selected=true] .label').textContent`, &sel)
	if sel != "Runbooks" {
		t.Errorf("the tree opens on %q, want Runbooks", sel)
	}
	var target string
	p.Eval(t, tree+`.shadowRoot.querySelector('[tabindex="0"]').focus(), `+tree+`.shadowRoot.activeElement.querySelector('.label').textContent`, &target)
	if target != "Runbooks" {
		t.Errorf("the focus lands on %q", target)
	}
	// Up to Architecture (closed, above Runbooks), and Enter picks it.
	treeKey(t, p, "ArrowUp")
	p.Eval(t, tree+`.shadowRoot.activeElement.querySelector('.label').textContent`, &target)
	if target != "Architecture" {
		t.Fatalf("Up went to %q", target)
	}
	enter(t, p)
	time.Sleep(60 * time.Millisecond)
	p.Eval(t, tree+`.value`, &target)
	if !strings.HasPrefix(target, "page:") {
		t.Fatalf("the tree's value is %q", target)
	}
	var still bool
	p.Eval(t, `document.getElementById('move').open`, &still)
	if !still {
		t.Fatal("Enter closed the dialog")
	}
	shoot(t, p, "move-tree")
	p.Eval(t, `(document.querySelector('#move [form=move-form][type=submit]').click(), true)`, nil)
	p.Wait(t, `!document.getElementById('move').open`, 5*time.Second)
	p.Wait(t, `[...document.querySelectorAll('.tk-crumbs a')].some((a) => a.textContent.trim() === 'Architecture')`, 5*time.Second)
	var parent string
	e.DB.R.QueryRow(`SELECT parent_id FROM pages WHERE id = ?`, page).Scan(&parent)
	if "page:"+parent != target {
		t.Errorf("the page is under %q, the tree picked %q", parent, target)
	}
	noErrors(t, p)
}

// FR-140, FR-142, NFR-47.2: the accounts' rows share one sb-context-menu.
// A row's button opens it on that account with its role checked and only
// the actions that apply; the arrow keys move, Escape closes it and gives
// the focus back, Enter on a role sets it, Erase asks first, and the menu
// stays inside the window at 320, 768 and 1280 pixels.
func TestAdminRowMenu(t *testing.T) {
	e, ids := seededDemo(t)
	b := browsertest.Start(t)
	p := staffPage(t, e, b, "ada@example.com", 1280)
	p.Navigate(t, e.URL+"/admin")
	p.Wait(t, `document.getElementById('reconnecting')?.hidden && customElements.get('sb-context-menu') && document.querySelector('[data-menu-for=admin-row-menu]')`, 10*time.Second)
	bea := `document.querySelector('[data-menu-for=admin-row-menu][aria-label^="Change bea"]')`
	var label string
	p.Eval(t, `[...document.querySelectorAll('[data-menu-for=admin-row-menu]')].map((b) => b.getAttribute('aria-label')).join(' | ')`, &label)
	t.Logf("buttons: %s", label)
	menu := `document.getElementById('admin-row-menu')`
	items := `[...` + menu + `.querySelectorAll('[role^=menuitem]')].map((i) => ({label: i.textContent.trim(), role: i.getAttribute('role'), checked: i.getAttribute('aria-checked') || '', focused: i === document.activeElement}))`
	type item struct {
		Label, Role, Checked string
		Focused              bool
	}
	read := func() []item {
		var out []item
		p.Eval(t, items, &out)
		return out
	}

	p.Eval(t, `(`+bea+`.focus(), true)`, nil)
	enter(t, p)
	p.Wait(t, menu+`.matches(':popover-open') && document.activeElement?.closest('#admin-row-menu')`, 2*time.Second)
	got := read()
	t.Logf("bea's menu: %+v", got)
	labels := []string{}
	for _, it := range got {
		labels = append(labels, it.Label)
	}
	if strings.Join(labels, ",") != "No role,Moderator,Site admin,Deactivate,Erase…" {
		t.Errorf("bea's menu has %v", labels)
	}
	if got[0].Checked != "true" || got[1].Checked != "false" || got[0].Role != "menuitemradio" || !got[0].Focused {
		t.Errorf("bea's roles: %+v", got[:3])
	}
	key(t, p, "ArrowDown")
	if f := read(); !f[1].Focused {
		t.Errorf("Down left the focus on %+v", f)
	}
	key(t, p, "Escape")
	p.Wait(t, `!`+menu+`.matches(':popover-open') && document.activeElement === `+bea, 2*time.Second)

	// Enter on Moderator sets bea's role; the row says so, and its menu checks it.
	enter(t, p)
	p.Wait(t, menu+`.matches(':popover-open')`, 2*time.Second)
	key(t, p, "ArrowDown")
	enter(t, p)
	p.Wait(t, `!`+menu+`.matches(':popover-open') && `+bea+`.getAttribute('data-menu-checked') === 'role:moderator' && `+bea+`.closest('tr').cells[2].textContent.trim() === 'Moderator'`, 5*time.Second)

	// Erase asks first; Cancel leaves the account.
	p.Eval(t, `(`+bea+`.focus(), true)`, nil)
	enter(t, p)
	p.Wait(t, menu+`.matches(':popover-open')`, 2*time.Second)
	key(t, p, "End")
	enter(t, p)
	p.Wait(t, `document.getElementById('tk-confirm').open && document.querySelector('#tk-confirm .tk-confirm__text').textContent.startsWith('Erase this account?')`, 2*time.Second)
	p.Eval(t, `(document.getElementById('tk-confirm').close(), true)`, nil)

	// The admin's own row: roles only, without the separator after them.
	own := `document.querySelector('[data-menu-for=admin-row-menu][data-context-id="` + ids["ada"] + `"]')`
	p.Eval(t, `(`+own+`.click(), true)`, nil)
	p.Wait(t, menu+`.matches(':popover-open')`, 2*time.Second)
	labels = labels[:0]
	for _, it := range read() {
		labels = append(labels, it.Label)
	}
	var seps int
	p.Eval(t, menu+`.querySelectorAll('[role=separator]').length`, &seps)
	if strings.Join(labels, ",") != "No role,Moderator,Site admin" || seps != 0 {
		t.Errorf("the admin's own menu has %v and %d separators", labels, seps)
	}
	key(t, p, "Escape")
	noErrors(t, p)

	// NFR-45.1: axe finds nothing serious with bea's menu open.
	if src := os.Getenv("TICKIT_AXE"); src != "" {
		axe, err := os.ReadFile(src)
		if err != nil {
			t.Fatal(err)
		}
		p.Eval(t, `(`+bea+`.click(), true)`, nil)
		p.Wait(t, menu+`.matches(':popover-open')`, 2*time.Second)
		p.Eval(t, string(axe)+";true", nil)
		var res []struct{ ID, Impact, Help string }
		p.Eval(t, `axe.run(document, {resultTypes: ['violations']}).then(r => r.violations.map(v => ({id: v.id, impact: v.impact, help: v.help})))`, &res)
		for _, v := range res {
			if v.Impact == "serious" || v.Impact == "critical" {
				t.Errorf("axe: %s (%s): %s", v.ID, v.Impact, v.Help)
			} else {
				t.Logf("axe: %s (%s): %s", v.ID, v.Impact, v.Help)
			}
		}
		key(t, p, "Escape")
	}

	for _, w := range []int{320, 768, 1280} {
		q := staffPage(t, e, b, "ada@example.com", w)
		q.Navigate(t, e.URL+"/admin")
		q.Wait(t, `document.getElementById('reconnecting')?.hidden && customElements.get('sb-context-menu') && document.querySelector('[data-menu-for=admin-row-menu]')`, 10*time.Second)
		q.Eval(t, `((b) => (b.scrollIntoView({block: 'center', inline: 'nearest'}), b.click(), true))(`+bea+`)`, nil)
		q.Wait(t, menu+`.matches(':popover-open')`, 2*time.Second)
		time.Sleep(200 * time.Millisecond)
		var bad []string
		q.Eval(t, `(() => {
			const out = [], r = `+menu+`.getBoundingClientRect()
			if (r.left < 0 || r.right > innerWidth || r.top < 0 || r.bottom > innerHeight) out.push('the menu is at ' + JSON.stringify(r))
			if (document.documentElement.scrollWidth > innerWidth) out.push('the page is ' + document.documentElement.scrollWidth + ' px wide')
			return out
		})()`, &bad)
		for _, s := range bad {
			t.Errorf("%d px: %s", w, s)
		}
		shoot(t, q, fmt.Sprintf("admin-menu-%d", w))
		noErrors(t, q)
		q.Close(t)
	}
}
