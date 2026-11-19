package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/browsertest"
	"github.com/zweiundeins/tickit/internal/web/webtest"
)

// pendingLook sets up a busy button, a read-only field and a pending card,
// and reads their pending look (window.look()). It reads it once at the
// start: a transition needs a style to start from.
const pendingLook = `(() => {
	if (!window.look) {
		const box = document.createElement('div');
		box.innerHTML = '<button id="pb" class="tk-btn tk-btn--secondary" type="button"><span class="tk-btn__label">Save</span></button>'
			+ '<input id="pi" class="tk-input" value="5">'
			+ '<div id="pc" class="tk-card"><span class="tk-card__title">A card</span></div>';
		document.body.append(box);
		window.look = () => ({
			label: getComputedStyle(document.querySelector('#pb > span')).opacity,
			pixel: getComputedStyle(document.getElementById('pb'), '::after').visibility,
			edge: getComputedStyle(document.getElementById('pi')).borderTopStyle,
			card: getComputedStyle(document.querySelector('#pc > span')).opacity,
			cardPixel: getComputedStyle(document.getElementById('pc'), '::after').visibility,
		});
		window.pend = (on) => {
			const b = document.getElementById('pb'), i = document.getElementById('pi'), c = document.getElementById('pc');
			on ? b.setAttribute('aria-busy', 'true') : b.removeAttribute('aria-busy');
			i.readOnly = on;
			on ? c.dataset.state = 'pending' : delete c.dataset.state;
			window.since = performance.now();
			return window.look();
		};
	}
	return window.look();
})()`

type look struct {
	Label, Pixel, Edge, Card, CardPixel string
}

func (l look) hidden() bool {
	return l.Label == "1" && l.Pixel == "hidden" && l.Edge == "solid" && l.Card == "1" && l.CardPixel == "hidden"
}

const pendingShown = `((l) => l.label === '0.6' && l.pixel === 'visible' && l.edge === 'dashed' && l.card === '0.55' && l.cardPixel === 'visible')(window.look())`

// MP-09.1 with lith v0.5.10's rule: a pending state is set at once
// (aria-busy, readonly, data-state) but shows about 200 ms later, so an op
// acked within a tick never flashes; ending it is immediate. Reduced
// motion keeps the wait, and forced colours keep the dashed edge.
func TestPendingShowsLate(t *testing.T) {
	e := webtest.Start(t)
	b := browsertest.Start(t)
	for _, media := range []string{"", "prefers-reduced-motion", "forced-colors"} {
		p := b.NewPage(t, 1024, 768)
		if media != "" {
			value := map[string]string{"prefers-reduced-motion": "reduce", "forced-colors": "active"}[media]
			p.Call(t, "Emulation.setEmulatedMedia", map[string]any{"features": []map[string]string{{"name": media, "value": value}}})
		}
		p.Navigate(t, e.URL+"/design")
		p.Wait(t, `document.readyState === 'complete' && getComputedStyle(document.documentElement).getPropertyValue('--tk-pending-delay').trim() === '200ms'`, 10*time.Second)
		p.Eval(t, pendingLook, nil)

		var at look
		p.Eval(t, `window.pend(true)`, &at)
		if !at.hidden() {
			t.Errorf("%s: the pending look showed at once: %+v", media, at)
		}
		p.Wait(t, pendingShown, 3*time.Second)
		var after float64
		p.Eval(t, `performance.now() - window.since`, &after)
		if after < 190 {
			t.Errorf("%s: the pending look showed after %.0f ms, want 200", media, after)
		}
		// Reduced motion makes every change a transition of 0.01 ms, so
		// the look is read a frame later.
		var off look
		p.Eval(t, `new Promise((done) => { window.pend(false); requestAnimationFrame(() => requestAnimationFrame(() => done(window.look()))) })`, &off)
		if off.Label != "1" || off.Edge != "solid" || off.Card != "1" {
			t.Errorf("%s: the pending look stayed after the outcome: %+v", media, off)
		}

		// An outcome within the delay: nothing shows at all.
		p.Eval(t, `new Promise((done) => { window.pend(true); const seen = []; const tick = () => { const t = performance.now() - window.since; t < 190 && seen.push(window.look()); t < 100 ? requestAnimationFrame(tick) : (window.pend(false), done(seen.filter((l) => l.label !== '1' || l.edge !== 'solid' || l.card !== '1').length)) }; requestAnimationFrame(tick) })`, &after)
		if after != 0 {
			t.Errorf("%s: an op acked within 100 ms showed pending in %v frames", media, after)
		}

		// Starbase's pickers and choice menus: :state(pending) at once, the
		// dashed edge after the delay.
		p.Wait(t, `document.querySelector('sb-dropdown.tk-pick-menu')?.shadowRoot?.querySelector('[part~=trigger]') && document.querySelector('sb-select.tk-picker')?.shadowRoot?.querySelector('[part~=control]')`, 5*time.Second)
		var parts struct{ Base, At, Later, Off []any }
		p.Eval(t, `new Promise((done) => {
			const m = document.querySelector('sb-dropdown.tk-pick-menu'), s = document.querySelector('sb-select.tk-picker');
			const mt = m.shadowRoot.querySelector('[part~=trigger]'), sc = s.shadowRoot.querySelector('[part~=control]');
			const read = () => [getComputedStyle(mt).outlineStyle, getComputedStyle(sc).borderTopStyle, m.matches(':state(pending)'), s.matches(':state(pending)')];
			const base = read();
			m.setAttribute('pending', ''); s.setAttribute('pending', '');
			const at = read();
			setTimeout(() => { const later = read(); m.removeAttribute('pending'); s.removeAttribute('pending'); requestAnimationFrame(() => requestAnimationFrame(() => done({base, at, later, off: read()}))) }, 300)
		})`, &parts)
		edge := func(l []any) string { return fmt.Sprint(l[:2]) }
		if edge(parts.At) != edge(parts.Base) || fmt.Sprint(parts.At[2:]) != "[true true]" || edge(parts.Later) != "[dashed dashed]" || fmt.Sprint(parts.Off) != fmt.Sprint(parts.Base) {
			t.Errorf("%s: a choice menu and a picker pending (outline, edge, states): before %v, at once %v, after 300 ms %v, after the outcome %v", media, parts.Base, parts.At, parts.Later, parts.Off)
		}
		p.Close(t)
	}
}
