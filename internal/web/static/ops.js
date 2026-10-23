// tkOps follows the ops a page sent until the tab's view brings their
// outcome in $_outcomes (internal/views/expr.go). A request that ended
// without one leaves its op to the stream: the server's notice ends it
// (tabs.Notice), and so does the tab view's first frame after the stream
// connected again ($_synced turns true), when the row has no outcome for
// it and none of its requests is in flight. A request still in flight
// then decides once it ends, by opening the stream once more. Never a
// timer.
window.tkOps = (() => {
	const waiting = new Set() // ops sent whose outcome has not come
	const inflight = new Map() // op id to its requests in flight
	const recheck = new Set() // ops a connect found in flight
	let outcomes = {} // $_outcomes, as the last connect handed it
	// end ends op as a notice would, so its control settles as it does on one.
	const end = (op) => document.dispatchEvent(new CustomEvent('datastar-fetch', {
		detail: {
			type: 'datastar-patch-signals',
			el: document.documentElement,
			argsRaw: { signals: JSON.stringify({ _outcomes: { [op]: { ok: false, msg: 'This did not go through. Try again.', field: '', failed: true } } }) },
		},
	}))
	// reopen opens the stream again, as sse.Reconnect has the page do.
	const reopen = () => {
		const s = document.getElementById('stream')
		if (s) s.replaceWith(s.cloneNode(true))
	}
	return {
		// send follows op, whose request is the promise of an @post.
		send(op, request) {
			waiting.add(op)
			inflight.set(op, (inflight.get(op) || 0) + 1)
			return Promise.resolve(request).catch(() => {}).finally(() => {
				const n = inflight.get(op) - 1
				if (n > 0) return void inflight.set(op, n)
				inflight.delete(op)
				if (outcomes[op]) waiting.delete(op)
				if (recheck.delete(op) && waiting.has(op)) reopen()
			})
		},
		// synced runs when the stream connected and its tab view's first
		// frame came, with $_outcomes.
		synced(o) {
			outcomes = o
			for (const op of waiting) {
				if (o[op]) {
					waiting.delete(op)
					recheck.delete(op)
				} else if (inflight.has(op)) {
					recheck.add(op)
				} else {
					waiting.delete(op)
					queueMicrotask(() => end(op))
				}
			}
		},
	}
})()
