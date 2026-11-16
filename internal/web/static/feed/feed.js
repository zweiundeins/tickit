// A long feed in sb-virtual-scroll (views.FeedWindow, internal/web/routes_feed.go).
// The component sizes the rows, aligns the windows and moves between entries
// with the feed keys; this module adds "Newer activity". Windows count from
// the newest entry, so an entry that arrives moves every other one down a
// row. The component keeps a reader's place (the entries carry data-key);
// once it has, the button shows and leads back up.

const HOSTS = 'sb-virtual-scroll.tk-vfeed__list'

function setup(host) {
	if (host.tkFeed) return
	const scroller = host.shadowRoot?.querySelector('.scroller')
	if (!scroller) return void requestAnimationFrame(() => setup(host))
	host.tkFeed = true

	const wrap = () => host.parentElement
	const items = () => [...host.children].filter((el) => el.localName === 'article')
	const newest = () => host.querySelector(':scope > article[aria-posinset="1"]')
	const newer = () => wrap()?.querySelector('.tk-vfeed__newer')
	const atTop = () => scroller.scrollTop < (items()[0]?.offsetHeight || 1) / 2

	// first: the key and place of the window's first entry, to see entries
	// arrive above it. focusNewest: the button took the focus out of the feed
	// before the top window came.
	let first = null
	let focusNewest = false
	const remember = () => {
		const el = items()[0]
		first = el ? { key: el.dataset.key, pos: +el.getAttribute('aria-posinset') } : null
		if (atTop()) hide()
	}
	const hide = () => {
		const b = newer()
		if (b && !b.hidden) b.hidden = true
	}
	const morphed = () => {
		const el = first && items().find((a) => a.dataset.key === first.key)
		if (el && +el.getAttribute('aria-posinset') > first.pos && scroller.scrollTop > 0 && newer()) newer().hidden = false
		if (focusNewest && newest()) (focusNewest = false), newest().focus({ preventScroll: true })
		remember()
	}

	scroller.addEventListener('scroll', remember, { passive: true })
	// After the component's own observer: the morph is done, the place kept.
	new MutationObserver(() => queueMicrotask(morphed)).observe(host, { childList: true, subtree: true, attributes: true, attributeFilter: ['offset', 'data-key'] })
	remember()

	wrap()?.addEventListener('click', (evt) => {
		const button = evt.target.closest?.('.tk-vfeed__newer')
		if (!button) return
		const focus = host.contains(document.activeElement) || button === document.activeElement
		host.scrollToIndex(0)
		hide()
		if (!focus) return
		if (newest()) newest().focus({ preventScroll: true })
		else focusNewest = true
	})
}

customElements.whenDefined('sb-virtual-scroll').then(() => document.querySelectorAll(HOSTS).forEach(setup))
