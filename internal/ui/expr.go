package ui

import "strings"

// The Datastar expressions the components carry: view behaviour that
// belongs to the design system (keyboard paths through menus, trees and
// toolbars, the state of a popover's button), never a feature's. They are
// constants and read what they need from the element (ids, data
// attributes), so no person's value reaches an expression (NFR-13), and
// each compiles once however many elements carry it.

// syncExpanded keeps aria-expanded on a popover's buttons true while it is
// open (NFR-47.3: the bell is a button with aria-expanded). On the popover.
const syncExpanded = `document.querySelectorAll('[popovertarget="' + el.id + '"]').forEach(b => b.setAttribute('aria-expanded', evt.newState === 'open'))`

// menuKeys moves the focus through a menu's items with the arrow keys,
// Home and End. On the menu.
const menuKeys = `['ArrowDown', 'ArrowUp', 'Home', 'End'].includes(evt.key) && ((items) => {
	evt.preventDefault()
	const i = items.indexOf(document.activeElement), n = items.length
	items[evt.key === 'Home' ? 0 : evt.key === 'End' ? n - 1 : (i + (evt.key === 'ArrowDown' ? 1 : n - 1)) % n]?.focus()
})([...el.querySelectorAll('.tk-menu__item:not([disabled]):not([aria-disabled="true"]):not([hidden])')])`

// toolbarKeys moves the focus along a toolbar's buttons with Left, Right,
// Home and End (the ARIA toolbar pattern). On the toolbar.
const toolbarKeys = `['ArrowRight', 'ArrowLeft', 'Home', 'End'].includes(evt.key) && ((items) => {
	evt.preventDefault()
	const i = items.indexOf(document.activeElement), n = items.length
	const next = items[evt.key === 'Home' ? 0 : evt.key === 'End' ? n - 1 : (i + (evt.key === 'ArrowRight' ? 1 : n - 1)) % n]
	if (next) { items.forEach(x => x.tabIndex = -1); next.tabIndex = 0; next.focus() }
})([...el.querySelectorAll('button:not([disabled])')])`

// searchShortcut focuses the header's search on Ctrl K or Cmd K. On the
// search field.
const searchShortcut = `(evt.ctrlKey || evt.metaKey) && evt.key.toLowerCase() === 'k' && (evt.preventDefault(), el.form.dataset.open = '', el.focus(), el.select())`

// On narrow screens the header's search is a button (searchOpen) that
// opens the field over the header; it closes when the focus leaves it
// (searchClose) or on Escape (searchEscape). Wide headers show the field
// and never the button, so data-open changes nothing there.
const (
	searchOpen = `((f) => {
	f.dataset.open = ''
	el.setAttribute('aria-expanded', 'true')
	f.querySelector('input').focus()
})(document.getElementById('tk-search-form'))`
	searchClose  = `el.contains(evt.relatedTarget) || (delete el.dataset.open, document.querySelector('.tk-search__open')?.setAttribute('aria-expanded', 'false'))`
	searchEscape = `evt.key === 'Escape' && el.dataset.open !== undefined && !evt.defaultPrevented && document.querySelector('.tk-search__open')?.checkVisibility() && document.querySelector('.tk-search__open').focus()`
)

// hushTips hides every tooltip on Escape, until the pointer or the focus
// moves on (WCAG 1.4.13: content on hover is dismissible). On the shell.
const (
	hushTips   = `evt.key === 'Escape' && document.documentElement.classList.add('tk-hush')`
	unhushTips = `document.documentElement.classList.remove('tk-hush')`
)

// toggleSidebar collapses or opens the shell's sidebar, in this page only:
// a feature that keeps the choice posts it besides. On the toggle button.
const toggleSidebar = `((a) => {
	a.dataset.sidebar = a.dataset.sidebar === 'collapsed' ? 'open' : 'collapsed'
	el.setAttribute('aria-expanded', a.dataset.sidebar === 'open')
})(el.closest('.tk-app'))`

// dismiss hides the closest dismissible element, in this page only. On a
// dismiss button.
const dismiss = `el.closest('[data-dismissible]').hidden = true`

// shortcutKey presses the button on its key alone, unless the person
// types in a field or a dialog is open. key is a constant of the page.
func shortcutKey(key string) string {
	return `!evt.ctrlKey && !evt.metaKey && !evt.altKey && evt.key.toLowerCase() === '` + strings.ToLower(key) + `' && !evt.target.closest?.('input, textarea, select, [contenteditable], sb-rich-editor') && !document.querySelector('dialog[open], :popover-open') && (evt.preventDefault(), el.click())`
}
