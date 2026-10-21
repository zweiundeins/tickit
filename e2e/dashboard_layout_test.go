package e2e

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
)

// NFR-46, FR-60: at every width no part of a seeded dashboard reaches past
// its gadget, and each issue row of a list gadget stays on one line, its
// summary the part that shortens. DASH_SHOTS names a folder for screenshots.
func TestDashboardRowsFit(t *testing.T) {
	s := newRealServer(t)
	s.seedAll()
	var ids []string
	err := s.db.View(context.Background(), func(tx *sql.Tx) error {
		rows, err := tx.Query(`SELECT id FROM dashboards ORDER BY name`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err != nil {
				return err
			}
			ids = append(ids, id)
		}
		return rows.Err()
	})
	if err != nil || len(ids) == 0 {
		t.Fatalf("the seed's dashboards: %v %d", err, len(ids))
	}
	dir := os.Getenv("DASH_SHOTS")
	b := browsertest.Start(t)
	for _, width := range []int{320, 768, 1100, 1280, 1440, 1920} {
		p := b.NewPage(t, width, 900)
		for i, id := range ids {
			p.Navigate(t, s.URL+"/dashboards/"+id)
			p.Wait(t, `document.getElementById('reconnecting')?.hidden && document.fonts.status === 'loaded' && document.querySelector('[id^="gme-"] .tk-worklist, [id^="gme-"] p.muted, .tk-dash:not(:has([id^="gme-"]))')`, 10*time.Second)
			time.Sleep(300 * time.Millisecond)
			var bad []string
			p.Eval(t, fitCheck, &bad)
			for _, b := range bad {
				t.Errorf("%d px, dashboard %d: %s", width, i, b)
			}
			if dir != "" {
				p.Screenshot(t, filepath.Join(dir, fmt.Sprintf("dashboard-%d-%d.png", i, width)), true)
			}
		}
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("%d px: the page reported %q", width, errs)
		}
		p.Close(t)
	}
}

// fitCheck lists what reaches past its gadget, a list row on more than one
// line, and a page wider than the window.
const fitCheck = `(() => {
	const out = [];
	if (document.documentElement.scrollWidth > innerWidth) out.push('the page is ' + document.documentElement.scrollWidth + ' px wide');
	const hidden = el => { const c = getComputedStyle(el); return c.clipPath.startsWith('inset(50%') || c.position === 'absolute' && el.getBoundingClientRect().width <= 1; };
	for (const g of document.querySelectorAll('.tk-gadget')) {
		const box = g.getBoundingClientRect();
		for (const el of g.querySelectorAll('*')) {
			const r = el.getBoundingClientRect();
			if (r.width === 0 || hidden(el)) continue;
			if (r.right > box.right + 0.5 || r.left < box.left - 0.5) out.push((g.querySelector('h2')?.textContent.trim() || g.id) + ': ' + el.tagName.toLowerCase() + '.' + el.className + ' reaches ' + Math.round(r.left) + ' to ' + Math.round(r.right) + ' past ' + Math.round(box.left) + ' to ' + Math.round(box.right));
		}
		for (const li of g.querySelectorAll('.tk-worklist > li')) {
			const kids = [...li.children].filter(k => !hidden(k)).map(k => k.getBoundingClientRect());
			if (Math.max(...kids.map(k => k.top)) >= Math.min(...kids.map(k => k.bottom))) out.push(li.textContent.trim().replace(/\s+/g, ' ') + ': on two lines');
		}
	}
	return out;
})()`
