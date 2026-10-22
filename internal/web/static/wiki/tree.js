// A space's page tree (views.PageTree): sb-tree with lazy branches. The
// shared tree view sends a branch off the current page's path without its
// children; this module fetches them for this tab when it opens one
// (GET /spaces/{key}/tree?parent=), and fetches the branches it opened
// again when the tree's data-rev changes, since frames send them lazy.

const loaded = new WeakMap() // tree -> ids of the branches it fetched

function load(tree, id) {
	const url = '/spaces/' + encodeURIComponent(tree.dataset.space) + '/tree?parent=' + encodeURIComponent(id)
	fetch(url, { headers: { Accept: 'text/html' } })
		.then((r) => (r.ok ? r.text() : null))
		.then((html) => {
			const ids = loaded.get(tree) ?? loaded.set(tree, new Set()).get(tree)
			const li = [...tree.querySelectorAll('li[data-lazy]')].find((l) => l.dataset.id === id)
			if (html === null || !li) return void ids.delete(id)
			ids.add(id)
			li.querySelector(':scope > ul')?.remove()
			li.insertAdjacentHTML('beforeend', html)
		})
}

document.addEventListener('sb-load', (evt) => {
	const tree = evt.target
	if (tree.matches?.('sb-tree[data-space]')) load(tree, evt.detail.id)
})

new MutationObserver((records) => {
	for (const r of records) {
		const tree = r.target
		if (tree.matches('sb-tree[data-space]')) loaded.get(tree)?.forEach((id) => load(tree, id))
	}
}).observe(document.documentElement, { subtree: true, attributeFilter: ['data-rev'] })
