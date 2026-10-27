package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tabs"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// uxIDs are ids of the seed a test of the tracker's gaps uses.
type uxIDs struct {
	dashboard, deploy, web1, web1ID string
}

func uxSeeded(t *testing.T) (*realServer, uxIDs) {
	t.Helper()
	s := newRealServer(t)
	s.seedAll()
	var ids uxIDs
	err := s.db.View(context.Background(), func(tx *sql.Tx) error {
		if err := tx.QueryRow(`SELECT g.dashboard_id FROM dashboard_gadgets g JOIN projects p ON p.id = g.project_id WHERE p.key = 'WEB' AND g.kind = 'status'`).Scan(&ids.dashboard); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT id FROM pages WHERE title = 'Deploy to production'`).Scan(&ids.deploy); err != nil {
			return err
		}
		return tx.QueryRow(`SELECT id, key FROM issues WHERE key = 'WEB-1'`).Scan(&ids.web1ID, &ids.web1)
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, ids
}

// The tracker's gaps in a real browser: at 320 pixels the header's search
// is a button that opens the field (QA); a dashboard's "Assigned to me"
// gadget is filled by the viewer's own view and keeps its content when
// the shared grid changes (FR-60); a saved filter feeds the board
// (FR-56.2); "Link page" links a page found by search (FR-102); a
// mention in the comment box's draft shows the name (FR-67.2).
func TestTrackerGaps(t *testing.T) {
	s, ids := uxSeeded(t)
	b := browsertest.Start(t)

	t.Run("search at 320", func(t *testing.T) {
		p := b.NewPage(t, 320, 700)
		p.Navigate(t, s.URL+"/projects")
		p.Wait(t, `getComputedStyle(document.querySelector('.tk-search__open')).display !== 'none'`, 5*time.Second)
		var hidden bool
		p.Eval(t, `getComputedStyle(document.getElementById('tk-search-form')).display === 'none'`, &hidden)
		if !hidden {
			t.Errorf("the field shows beside the button at 320 pixels")
		}
		p.Eval(t, `document.querySelector('.tk-search__open').click(), true`, nil)
		p.Wait(t, `document.activeElement?.id === 'tk-search' && getComputedStyle(document.getElementById('tk-search-form')).display !== 'none'`, 2*time.Second)
		var wide float64
		p.Eval(t, `document.getElementById('tk-search').getBoundingClientRect().width`, &wide)
		if wide < 200 {
			t.Errorf("the open field is %.0f pixels wide", wide)
		}
		p.Press(t, "Escape")
		p.Wait(t, `getComputedStyle(document.getElementById('tk-search-form')).display === 'none' && document.activeElement?.classList.contains('tk-search__open')`, 2*time.Second)
		p.Close(t)
	})

	t.Run("dashboard", func(t *testing.T) {
		p := b.NewPage(t, 1440, 900)
		p.Navigate(t, s.URL+"/dashboards/"+ids.dashboard)
		p.Wait(t, `document.querySelector('[id^="gme-"]')?.textContent.includes('Nothing is assigned to you.')`, 5*time.Second)
		var bars int
		p.Eval(t, `document.querySelectorAll('.tk-bars tbody tr').length`, &bars)
		if bars < 5 {
			t.Errorf("%d rows of counts by status", bars)
		}
		var before string
		p.Eval(t, `document.querySelector('.tk-bars').textContent`, &before)
		if err := s.do(tracker.SetStatus{By: byAt("e2e-d1"), Issue: ids.web1ID, Seen: 1 << 40, Status: tracker.StatusBlocked}); err != nil {
			t.Fatal(err)
		}
		p.Wait(t, `document.querySelector('.tk-bars').textContent !== `+jsonString(before), 5*time.Second)
		var kept bool
		p.Eval(t, `document.querySelector('[id^="gme-"]').textContent.includes('Nothing is assigned to you.')`, &kept)
		if !kept {
			t.Errorf("a frame of the shared grid emptied the viewer's own gadget")
		}
		p.Close(t)
	})

	t.Run("board filter", func(t *testing.T) {
		p := boardPage(t, b, s.URL+"/projects/WEB/board", 1440, 900)
		p.Eval(t, `document.querySelector('#board-saved [data-filter="saved"]').click(), true`, nil)
		p.Wait(t, `location.search.includes('filter=') && [...document.querySelectorAll('#board-lanes .tk-card > i[data-type]')].every(i => i.dataset.type === 'bug')`, 5*time.Second)
		var n int
		p.Eval(t, `document.querySelectorAll('#board-lanes .tk-card').length`, &n)
		if n == 0 {
			t.Errorf("the saved filter shows no card")
		}
		p.Close(t)
	})

	t.Run("link page and mention", func(t *testing.T) {
		p := b.NewPage(t, 1440, 900)
		p.Navigate(t, s.URL+"/"+ids.web1)
		p.Wait(t, `document.getElementById('issue-page-q') && document.querySelector('#issue-main[data-pid]')`, 5*time.Second)
		typeInto(t, p, "issue-page-q", "Deploy to")
		p.Wait(t, `document.querySelector('#issue-page-pick [data-value="`+ids.deploy+`"]')`, 5*time.Second)
		p.Eval(t, `document.querySelector('#issue-page-pick [data-value="`+ids.deploy+`"]').click(), true`, nil)
		p.Wait(t, `[...document.querySelectorAll('#issue-related .tk-issue__refs li')].some(li => li.textContent.includes('linked') && li.querySelector('[data-page="`+ids.deploy+`"]'))`, 5*time.Second)

		typeInto(t, p, "issue-comment", "Ask @")
		p.Wait(t, `[...document.querySelectorAll('#issue-mentions [data-act="mention"]')].some(b => !b.hidden)`, 3*time.Second)
		var draft string
		p.Eval(t, `(() => { const b = [...document.querySelectorAll('#issue-mentions [data-act="mention"]')].find(b => !b.hidden); b.click(); return b.dataset.name })()`, &draft)
		p.Wait(t, `document.getElementById('issue-comment').value.includes('@' + `+jsonString(draft)+`)`, 2*time.Second)
		var value string
		p.Eval(t, `document.getElementById('issue-comment').value`, &value)
		if strings.Contains(value, "@s-") || strings.Contains(value, "@g-") {
			t.Errorf("the draft shows an id: %q", value)
		}
		p.Close(t)
	})
}

// TestTrackerShots writes screenshots of the screens of the tracker's
// gaps to UX_SHOTS: dashboards, a dashboard, a person's page, the home
// page, an issue with a linked page, the board's Filters menu, the issue
// list, and the narrow header's search, dark and light, 1440 and 375
// pixels wide.
func TestTrackerShots(t *testing.T) {
	dir := os.Getenv("UX_SHOTS")
	if dir == "" {
		t.Skip("UX_SHOTS unset")
	}
	os.MkdirAll(dir, 0o755)
	s, ids := uxSeeded(t)
	b := browsertest.Start(t)
	for _, theme := range []string{"dark", "light"} {
		for _, w := range []int{1440, 375} {
			p := b.NewPage(t, w, 900)
			p.Navigate(t, s.URL+"/projects")
			p.Eval(t, fmt.Sprintf(`((tab) => fetch('/cmd/prefs', {method: 'POST', headers: {'Content-Type': 'application/json', 'Datastar-Request': 'true', 'Tab-Id': tab, 'Op-Id': 'th%d'}, body: JSON.stringify({tab, op: 'th%d', key: 'theme', value: %q})}).then(r => r.status))(document.body.dataset.signals.match(/_tab: "([^"]+)"/)[1])`, w, w, theme), nil)
			shot := func(name, url string, full bool, ready, then string) {
				p.Navigate(t, s.URL+url)
				if ready != "" {
					p.Wait(t, ready, 10*time.Second)
				}
				if then != "" {
					p.Eval(t, then, nil)
				}
				time.Sleep(500 * time.Millisecond)
				p.Screenshot(t, filepath.Join(dir, fmt.Sprintf("%s-%s-%d.png", name, theme, w)), full)
			}
			shot("issue", "/"+ids.web1, true, `document.querySelector('#issue-main[data-pid]')`, "")
			shot("dashboards", "/dashboards", false, "", "")
			shot("dashboard", "/dashboards/"+ids.dashboard, true, `document.querySelector('[id^="gme-"] .tk-worklist, [id^="gme-"] p.muted')`, "")
			shot("person", "/people/"+people.Seeded[0].ID, true, "", "")
			shot("home", "/", true, `document.getElementById('home-viewed')`, "")
			shot("board-filters", "/projects/WEB/board", false, `document.getElementById('board-lanes').dataset.rev !== ''`,
				`document.querySelector('[popovertarget="board-saved"]')?.click(), true`)
			shot("list", "/issues?project=WEB&sort=resolved&dir=desc", false, "", "")
			if w == 375 {
				p.Call(t, "Emulation.setDeviceMetricsOverride", map[string]any{"width": 320, "height": 700, "deviceScaleFactor": 1, "mobile": true})
				shot("search", "/projects", false, "", `document.querySelector('.tk-search__open').click(), true`)
			}
			p.Close(t)
		}
	}
}

// byAt is a seeded person's change now, as the seed's people act.
func byAt(op string) tabs.By { return tabs.By{Actor: people.Seeded[0], Op: op, At: time.Now()} }
