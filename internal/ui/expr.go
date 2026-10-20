package ui

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
})([...el.querySelectorAll('.tk-menu__item:not([disabled]):not([aria-disabled="true"])')])`

// treeKeys is the keyboard of the ARIA tree pattern (NFR-47.2): Up and
// Down move through the visible items, Right opens an item or goes to its
// first child, Left closes it or goes to its parent, Home and End go to the
// ends. One item at a time is in the tab order (roving tabindex). Enter
// follows the item's link. On the tree.
const treeKeys = `['ArrowDown', 'ArrowUp', 'ArrowRight', 'ArrowLeft', 'Home', 'End'].includes(evt.key) && evt.target.matches('[role=treeitem]') && ((t) => {
	evt.preventDefault()
	const items = [...el.querySelectorAll('[role=treeitem]')].filter(i => i.offsetParent !== null)
	const i = items.indexOf(t), open = t.getAttribute('aria-expanded')
	let next
	if (evt.key === 'ArrowDown') next = items[i + 1]
	else if (evt.key === 'ArrowUp') next = items[i - 1]
	else if (evt.key === 'Home') next = items[0]
	else if (evt.key === 'End') next = items[items.length - 1]
	else if (evt.key === 'ArrowRight') open === 'false' ? t.setAttribute('aria-expanded', 'true') : open === 'true' && (next = items[i + 1])
	else if (open === 'true') t.setAttribute('aria-expanded', 'false')
	else next = t.parentElement.closest('[role=group]')?.parentElement.querySelector(':scope > [role=treeitem]')
	if (next) { items.forEach(x => x.tabIndex = -1); next.tabIndex = 0; next.focus() }
})(evt.target)`

// treeToggle opens or closes an item when its twisty is clicked, without
// following the link. On the tree.
const treeToggle = `evt.target.closest('.tk-tree__twisty') && ((t) => {
	evt.preventDefault()
	t.setAttribute('aria-expanded', t.getAttribute('aria-expanded') === 'true' ? 'false' : 'true')
})(evt.target.closest('[role=treeitem]'))`

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
const searchShortcut = `(evt.ctrlKey || evt.metaKey) && evt.key.toLowerCase() === 'k' && (evt.preventDefault(), el.focus(), el.select())`

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
