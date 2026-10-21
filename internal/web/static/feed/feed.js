// A long feed in sb-virtual-scroll (views.FeedWindow, internal/web/routes_feed.go):
// the server sends a window of entries, the page's Datastar wiring asks for
// another one as the reader scrolls. This module does the rest for each
// feed on the page:
//
// - Rows are --tk-feed-row high: it measures them for item-size, and keeps
//   the buffer small enough that a window of data-feed-window entries from a
//   multiple of ten still covers what the component asks for.
// - Windows count from the newest entry, so an entry that arrives moves every
//   other one down a row. A reader who scrolled, or whose focus or pointer is
//   in the feed, keeps their place: the entry at the top of the box stays
//   there (found by its id), and "Newer activity" leads back up. At the top
//   and idle, new entries come in.
// - Page Down and Page Up move the focus to the next and the previous entry
//   (the ARIA feed pattern), also to one the window has yet to bring.

const HOSTS = 'sb-virtual-scroll.tk-vfeed__list'
const STEP = 10

function setup(host) {
	if (host.tkFeed) return
	const scroller = host.shadowRoot?.querySelector('.scroller')
	if (!scroller) return void requestAnimationFrame(() => setup(host))
	host.tkFeed = true

	const wrap = () => host.parentElement
	const size = () => +host.getAttribute('item-size') || 1
	const offset = () => +host.getAttribute('offset') || 0
	const items = () => [...host.children].filter((el) => el.localName === 'article')
	const newer = () => wrap()?.querySelector('.tk-vfeed__newer')

	const measure = () => {
		const px = Math.round(wrap()?.querySelector('.tk-vfeed__probe')?.getBoundingClientRect().height || 0)
		if (px > 0 && px !== size()) host.setAttribute('item-size', px)
		const win = +host.dataset.feedWindow || 50
		const view = Math.ceil(scroller.clientHeight / size()) + 1
		const buffer = Math.max(2, Math.floor((win - STEP - view) / 2)) * size()
		if (String(buffer) !== host.getAttribute('buffer')) host.setAttribute('buffer', buffer)
	}

	// anchor: the entry at the top of the box, its index in the feed, and how
	// far the box is scrolled into it.
	let anchor = null
	let pending = -1
	const remember = () => {
		const s = size(), top = scroller.scrollTop, i = Math.floor(top / s)
		const el = items()[i - offset()]
		anchor = el ? { id: el.id, index: i, delta: top - i * s } : null
		if (top < s / 2) hide()
	}
	const hide = () => {
		const b = newer()
		if (b && !b.hidden) b.hidden = true
	}
	const engaged = () => scroller.scrollTop > 0 || host.contains(document.activeElement) || host.matches(':hover')
	// The component tracks the focused entry by its index: after a move, tell
	// it where the focus is now, before its own refocus runs in the next frame.
	const resync = () => {
		const a = document.activeElement
		if (a && a !== host && host.contains(a)) a.dispatchEvent(new FocusEvent('focusin', { bubbles: true, composed: true }))
	}
	const keep = () => {
		if (anchor && engaged()) {
			const el = document.getElementById(anchor.id)
			const i = el && el.parentElement === host ? items().indexOf(el) : -1
			const d = i < 0 ? 0 : offset() + i - anchor.index
			if (d) {
				scroller.scrollTop = (anchor.index + d) * size() + anchor.delta
				resync()
				if (d > 0 && newer()) newer().hidden = false
			}
		}
		if (pending >= 0) {
			const el = items()[pending - offset()]
			if (el) (pending = -1), el.focus()
		}
		remember()
	}

	scroller.addEventListener('scroll', remember, { passive: true })
	// After the component's own observer: the morph is done, its geometry set.
	new MutationObserver(() => queueMicrotask(keep)).observe(host, { childList: true, attributes: true, attributeFilter: ['offset', 'total'] })
	new ResizeObserver(measure).observe(scroller)
	measure()
	remember()

	const go = (i) => {
		const el = items()[i - offset()]
		if (el) return el.focus()
		pending = i
		host.scrollToIndex(i, { block: 'nearest' })
	}
	host.addEventListener('keydown', (evt) => {
		if ((evt.key !== 'PageDown' && evt.key !== 'PageUp') || evt.altKey || evt.ctrlKey || evt.metaKey || evt.shiftKey) return
		const art = evt.target.closest?.('article')
		const at = art ? +art.getAttribute('aria-posinset') - 1 : Math.floor(scroller.scrollTop / size()) - (evt.key === 'PageDown' ? 1 : 0)
		const next = at + (evt.key === 'PageDown' ? 1 : -1)
		if (next < 0 || next >= (+host.getAttribute('total') || 0)) return
		evt.preventDefault()
		go(next)
	})
	wrap()?.addEventListener('click', (evt) => {
		if (!evt.target.closest?.('.tk-vfeed__newer')) return
		const focus = host.contains(document.activeElement) || evt.target.closest('.tk-vfeed__newer') === document.activeElement
		scroller.scrollTop = 0
		hide()
		if (focus) go(0)
	})
}

customElements.whenDefined('sb-virtual-scroll').then(() => document.querySelectorAll(HOSTS).forEach(setup))
