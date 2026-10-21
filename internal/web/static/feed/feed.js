// A long feed in sb-virtual-scroll (views.FeedWindow, internal/web/routes_feed.go):
// the server sends a window of entries, the page's Datastar wiring asks for
// another one as the reader scrolls. This module does the rest for each
// feed on the page:
//
// - Rows are --tk-feed-row high: it measures them for item-size, and keeps
//   the buffer small enough that a window of data-feed-window entries from a
//   multiple of ten still covers what the component asks for.
// - Windows count from the newest entry, so an entry that arrives moves every
//   other one down a row. The component keeps a reader's place (the entries
//   carry data-key); once it has, "Newer activity" shows and leads back up.
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

	// first: the key and place of the window's first entry, to see entries
	// arrive above it.
	let first = null
	let pending = -1
	const remember = () => {
		const el = items()[0]
		first = el ? { key: el.dataset.key, pos: +el.getAttribute('aria-posinset') } : null
		if (scroller.scrollTop < size() / 2) hide()
	}
	const hide = () => {
		const b = newer()
		if (b && !b.hidden) b.hidden = true
	}
	const morphed = () => {
		const el = first && items().find((a) => a.dataset.key === first.key)
		if (el && +el.getAttribute('aria-posinset') > first.pos && scroller.scrollTop > 0 && newer()) newer().hidden = false
		if (pending >= 0) {
			const el = items()[pending - offset()]
			if (el) (pending = -1), el.focus()
		}
		remember()
	}

	scroller.addEventListener('scroll', remember, { passive: true })
	// After the component's own observer: the morph is done, the place kept.
	new MutationObserver(() => queueMicrotask(morphed)).observe(host, { childList: true, subtree: true, attributes: true, attributeFilter: ['offset', 'total', 'data-key'] })
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
