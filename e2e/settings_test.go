package e2e

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
)

// NFR-46, FR-05, FR-36, FR-43: a project's settings tables (members,
// statuses, board columns) take the page's width and scroll sideways only
// when the window cannot hold them: none at 1280 pixels and more, the
// statuses' six columns at 768; every select in them shows its longest
// option beside its arrow. SETTINGS_SHOTS names a folder for screenshots.
func TestSettingsTablesFit(t *testing.T) {
	e, _ := seededDemo(t)
	dir := os.Getenv("SETTINGS_SHOTS")
	b := browsertest.Start(t)
	fit := map[int][]string{768: {"Members", "Board columns"}, 1280: nil, 1440: nil, 1920: nil}
	for _, width := range []int{320, 768, 1280, 1440, 1920} {
		p := staffPage(t, e, b, "ada@example.com", width)
		p.Navigate(t, e.URL+"/projects/WEB/settings")
		p.Wait(t, `document.getElementById('reconnecting')?.hidden && document.fonts.status === 'loaded' && document.querySelector('#wf-statuses tr')`, 10*time.Second)
		p.Eval(t, `(document.querySelector('#first-visit:not([hidden]) button')?.click(), true)`, nil)
		time.Sleep(300 * time.Millisecond)
		var bad []string
		names, wide := fit[width]
		p.Eval(t, settingsFit(wide, names), &bad)
		for _, s := range bad {
			t.Errorf("%d px: %s", width, s)
		}
		if dir != "" {
			p.Screenshot(t, filepath.Join(dir, fmt.Sprintf("settings-%d.png", width)), true)
		}
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("%d px: the page reported %q", width, errs)
		}
		p.Close(t)
	}
}

// settingsFit lists a page wider than the window, a settings table that
// scrolls or leaves its section's width unused (when wide is true; only
// the tables named, if any), and a select in a table narrower than its
// longest option and the arrow.
func settingsFit(wide bool, names []string) string {
	only, _ := json.Marshal(names)
	return `(() => {
	const wide = ` + fmt.Sprint(wide) + `, only = ` + string(only) + `;
	const out = [];
	if (document.documentElement.scrollWidth > innerWidth) out.push('the page is ' + document.documentElement.scrollWidth + ' px wide');
	const tables = document.querySelectorAll('.tk-settings .tk-table-wrap');
	if (tables.length < 3) out.push('only ' + tables.length + ' tables');
	for (const w of tables) {
		const name = w.querySelector('caption')?.textContent.trim();
		if (only && !only.includes(name)) continue;
		if (wide && w.scrollWidth > w.clientWidth) out.push(name + ' scrolls: ' + w.scrollWidth + ' in ' + w.clientWidth);
		const room = w.closest('.tk-settings').getBoundingClientRect().width;
		if (wide && w.getBoundingClientRect().width < room - 1) out.push(name + ' is ' + Math.round(w.getBoundingClientRect().width) + ' px of ' + Math.round(room));
	}
	const ctx = document.createElement('canvas').getContext('2d');
	for (const s of document.querySelectorAll('.tk-settings .tk-table select')) {
		const c = getComputedStyle(s);
		ctx.font = c.font;
		const text = Math.max(...[...s.options].map((o) => ctx.measureText(o.text).width));
		const arrow = s.parentElement.querySelector('.tk-icon').getBoundingClientRect();
		const room = arrow.left - s.getBoundingClientRect().left - parseFloat(c.paddingLeft) - parseFloat(c.borderLeftWidth);
		if (room < text) out.push(s.getAttribute('aria-label') + ': ' + Math.round(room) + ' px for ' + Math.round(text) + ' px of text');
		if (wide && s.selectedOptions[0] && s.clientWidth - parseFloat(c.paddingLeft) - parseFloat(c.paddingRight) < ctx.measureText(s.selectedOptions[0].text).width) out.push(s.getAttribute('aria-label') + ' cuts ' + s.selectedOptions[0].text);
	}
	return out;
})()`
}

// FR-03.1, FR-03.2: a seeded project's settings show its seeded lead in
// the picker; the admin chooses another and saves, and a list of projects
// in another tab shows the new lead within a tick.
func TestSettingsLeadLive(t *testing.T) {
	e, _ := seededDemo(t)
	b := browsertest.Start(t)
	settings := staffPage(t, e, b, "ada@example.com", 1280)
	settings.Navigate(t, e.URL+"/projects/WEB/settings")
	settings.Wait(t, `document.getElementById('reconnecting')?.hidden && document.getElementById('pd-lead')`, 10*time.Second)
	var shown string
	settings.Eval(t, `(() => { const s = document.getElementById('pd-lead'); return s.value + ' ' + s.selectedOptions[0]?.text })()`, &shown)
	if shown != "s-priya Priya Raman" {
		t.Errorf("the lead picker shows %q", shown)
	}

	list := staffPage(t, e, b, "ada@example.com", 1280)
	list.Navigate(t, e.URL+"/projects")
	lead := `[...document.querySelectorAll('#projects-list tr')].find((r) => r.querySelector('.tk-key')?.textContent === 'WEB')?.cells[2].textContent.trim()`
	list.Wait(t, `document.getElementById('reconnecting')?.hidden && `+lead+` === 'Priya Raman'`, 10*time.Second)

	settings.Eval(t, `(() => {
		const s = document.getElementById('pd-lead')
		s.value = 's-lena'
		s.dispatchEvent(new Event('change', {bubbles: true}))
		s.closest('form').querySelector('button[type=submit]').click()
		return true
	})()`, nil)
	list.Wait(t, lead+` === 'Lena Brunner'`, 3*time.Second)
	for _, p := range []*browsertest.Page{settings, list} {
		if errs := p.Errors(); len(errs) > 0 {
			t.Errorf("the page reported %q", errs)
		}
	}
}
