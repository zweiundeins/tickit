package views

import (
	"strconv"
	"time"

	"github.com/zweiundeins/tickit/internal/notify"
	"github.com/zweiundeins/tickit/internal/people"
)

// Bell is what the header's bell shows: the person's unread count and
// their panel (FR-110), with the people its notifications name and the
// time it was rendered at, for relative times.
type Bell struct {
	notify.Panel
	People map[string]people.Person
	Now    time.Time
}

// person is a notification's actor as the panel draws them.
func (b Bell) person(id string) people.Person {
	if p, ok := b.People[id]; ok {
		return p
	}
	return people.Person{ID: id, Kind: people.Deleted, Name: people.DeletedName}
}

// The bell's ids and signals, which the header (ui.Header) and the person
// view share (docs/architecture.md, "Notifications").
const (
	// NotifyPanelID is the panel's root inside the bell's popover; the
	// person view patches it.
	NotifyPanelID = "tk-notify-panel"
	// UnreadSignal holds the unread count in every tab of the person: the
	// window title follows it (FR-110.2).
	UnreadSignal = "_unread"
	// notifyOp is the pending op id of the panel's actions.
	notifyOp = "_notify.op"
)

// notifySignals are the panel's signals, set once per document.
func notifySignals(unread int) string {
	return "{_notify: {op: ''}, " + UnreadSignal + ": " + strconv.Itoa(unread) + "}"
}

// The panel's Datastar expressions: constants, and the notification's id
// comes from the element (data-id), never from a person (NFR-13).
var (
	readOne    = Act(notifyOp, "/cmd/notify/read", "id: Number(el.dataset.id)")
	dismissOne = Act(notifyOp, "/cmd/notify/dismiss", "id: Number(el.dataset.id)")
	readAll    = Act(notifyOp, "/cmd/notify/read-all", "")
	dismissAll = Act(notifyOp, "/cmd/notify/dismiss-all", "")
	notifyBusy = Busy(notifyOp)
	notifyFail = Failed(notifyOp)
	// openOne marks the notification read and goes to its subject with the
	// answer's $_goto; a click with a modifier, or while another action is
	// pending, follows the link as it is (FR-115.2).
	openOne = `evt.button === 0 && !(evt.ctrlKey || evt.metaKey || evt.shiftKey || evt.altKey) && $_notify.op === '' && (evt.preventDefault(), $_problem = '', $_notify.op = ` +
		newOp + `, @post('/cmd/notify/open', {headers: {'Tab-Id': $_tab, 'Op-Id': $_notify.op}, payload: {tab: $_tab, op: $_notify.op, id: Number(el.dataset.id)}}))`
	// openFailed follows the link when the action reached no outcome.
	openFailed = `evt.detail.el === el && ['error', 'retries-failed'].includes(evt.detail.type) && ($_notify.op = '', location.assign(el.href))`
	// seenSubject tells the server that the page opened a subject.
	seenSubject = `@post('/cmd/notify/seen', {headers: {'Tab-Id': $_tab, 'Op-Id': ` + newOp + `}, payload: {tab: $_tab, subject: el.dataset.subject, kind: el.dataset.kind || ''}})`
	// notifyEffects keep the window title's count (FR-110.2) and end the
	// panel's pending state when the outcome arrives.
	notifyEffects = `document.title = document.title.replace(/ \(\d+\)$/, '') + ($` + UnreadSignal + ` > 0 ? ' (' + $` + UnreadSignal + ` + ')' : ''); ` +
		Settle(notifyOp, "true")
)

// notificationDOMID is the id of a notification's row.
func notificationDOMID(id int64) string { return "notification-" + strconv.FormatInt(id, 10) }

func itoa64(n int64) string { return strconv.FormatInt(n, 10) }

// olderText says how many notifications the panel leaves out.
func olderText(n int) string {
	if n == 1 {
		return "1 older notification"
	}
	return strconv.Itoa(n) + " older notifications"
}
