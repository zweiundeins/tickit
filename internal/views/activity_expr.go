package views

import "strconv"

// feedWindow answers sb-window on a feed's sb-virtual-scroll: the window
// it asks for starts at a multiple of FeedStep (its step), and
// the tab's stream moves to that window's view (POST /cmd/feed/window);
// $_views names it for a reconnect.
var feedWindow = `((from) => {
	if (String(from) === el.getAttribute('offset')) return
	const kind = el.dataset.feedKind, scope = el.dataset.feedScope, spec = kind + ':' + (scope ? scope + '.' : '') + from
	@post('/cmd/feed/window', {headers: {'Tab-Id': $_tab}, payload: {tab: $_tab, views: $_views, feed: spec}, requestCancellation: 'disabled'})
	$_views = $_views.split(',').map((v) => v.startsWith(kind + ':' + scope) ? spec : v).join(',')
})(Math.min(+el.dataset.feedMax, Math.max(0, Math.floor(evt.detail.offset / ` + strconv.Itoa(FeedStep) + `) * ` + strconv.Itoa(FeedStep) + `)))`

// FeedScript is the module of pages with a long feed: it shows "Newer
// activity" once entries arrived above the reader's place
// (static/feed/feed.js).
const FeedScript = "feed/feed.js"
