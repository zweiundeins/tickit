package views

import (
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/a-h/templ"

	"github.com/zweiundeins/tickit/internal/demo"
	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/ui"
)

// The admin's pages (FR-140 to FR-144, FR-156) and the account's own
// (FR-125, FR-129). Live regions have ids of their own, which their views
// patch: the accounts table, the statistics, the demo's resets.
const (
	AdminAccountsID = "admin-accounts"
	AdminStatsID    = "admin-stats"
	AdminDemoID     = "admin-demo"
)

// AdminTab names a tab of the admin pages.
type AdminTab string

const (
	TabAccounts AdminTab = "accounts"
	TabGuests   AdminTab = "guests"
	TabStats    AdminTab = "stats"
	TabSecurity AdminTab = "security"
	TabDemo     AdminTab = "demo"
)

func adminTabs(cur AdminTab) []ui.NavItem {
	item := func(t AdminTab, label, href, icon string) ui.NavItem {
		return ui.NavItem{Label: label, Href: href, Icon: icon, Current: t == cur}
	}
	return []ui.NavItem{
		item(TabAccounts, "Accounts", "/admin", "users"),
		item(TabGuests, "Guests", "/admin/guests", "user"),
		item(TabStats, "Statistics", "/admin/stats", "chart"),
		item(TabSecurity, "Security log", "/admin/security", "lock"),
		item(TabDemo, "Demo", "/admin/demo", "clock"),
	}
}

// AdminAccounts is the accounts list (FR-140.1).
type AdminAccounts struct {
	Rows   []people.AccountRow
	Total  int
	Filter people.AccountFilter
	Me     string // the admin's person id
	Now    time.Time
}

// Pages is how many pages the list has.
func (a AdminAccounts) Pages() int {
	return (a.Total + people.AccountsPage - 1) / people.AccountsPage
}

// href is the list's address at page n.
func (a AdminAccounts) href(n int) string {
	v := url.Values{}
	if a.Filter.Search != "" {
		v.Set("q", a.Filter.Search)
	}
	if a.Filter.Role != "" {
		v.Set("role", string(a.Filter.Role))
	}
	if a.Filter.Status != "" {
		v.Set("status", a.Filter.Status)
	}
	if n > 1 {
		v.Set("page", strconv.Itoa(n))
	}
	if len(v) == 0 {
		return "/admin"
	}
	return "/admin?" + v.Encode()
}

// AdminGuests are the guests active in the last 24 hours (FR-140.2).
type AdminGuests struct {
	Rows []people.GuestRow
	Now  time.Time
}

// AdminStats are the numbers of FR-144.
type AdminStats struct {
	Counts []ui.Stat // accounts, guests today, issues, pages, streams
	Writer []ui.Stat // the writer's numbers
	Hub    []ui.Stat // the render loop's numbers
	// Views are the view keys rendered most, most first (MP-11.4).
	Views []ViewRenders
}

// ViewRenders is a view key with its renders since it was first watched.
type ViewRenders struct {
	Key     string
	Renders int64
}

// AdminSecurity is a page of the security log (FR-156.2).
type AdminSecurity struct {
	Entries []people.SecurityEntry
	People  map[string]people.Person
	Filter  people.SecurityFilter
	// Accounts are the accounts to filter by.
	Accounts []people.AccountRow
	Now      time.Time
}

// next is the address of the page after this one, "" for none.
func (s AdminSecurity) next() string {
	if len(s.Entries) < people.SecurityPage {
		return ""
	}
	v := url.Values{"before": {strconv.FormatInt(s.Entries[len(s.Entries)-1].ID, 10)}}
	if s.Filter.Person != "" {
		v.Set("account", s.Filter.Person)
	}
	if s.Filter.Kind != "" {
		v.Set("kind", s.Filter.Kind)
	}
	return "/admin/security?" + v.Encode()
}

// AdminDemo is the demo's state: the next reset, the caps, the resets.
type AdminDemo struct {
	Status  demo.Status
	Next    time.Time
	Created map[string]int
	Resets  []demo.ResetEntry
	People  map[string]people.Person
	Now     time.Time
}

// AccountPage is an account's own page: its password and its sessions
// (FR-125, FR-129).
type AccountPage struct {
	Me       people.Person
	Sessions []people.SessionInfo
	Now      time.Time
}

// Moderation is the moderation page (DM-07.3): what visitors wrote
// lately, the guests whose changes are paused, the log.
type Moderation struct {
	Items  []demo.Target
	Names  map[string]string // kind to its name: "Issue"
	Delete map[string]bool   // kinds that can be deleted
	People map[string]people.Person
	Blocks []demo.Blocked
	Log    []demo.LogEntry
	// Reports are the open reports, the oldest first (DM-08).
	Reports []demo.OpenReport
	Now     time.Time
}

// ReportID is the id of the report dialog, in every document.
const ReportID = "report"

// ReportAttrs are the attributes of a Report button for a thing of a
// moderated kind (DM-08).
func ReportAttrs(kind, id string) templ.Attributes {
	return templ.Attributes{"data-report-kind": kind, "data-report-id": id, "data-on:click": reportOpen}
}

// ModerationID is the id of the moderation page's live region.
const ModerationID = "moderation"

// siteRoleName is a site role as pages say it.
func siteRoleName(r people.SiteRole) string {
	switch r {
	case people.Admin:
		return "Site admin"
	case people.Moderator:
		return "Moderator"
	}
	return "No role"
}

func statusTone(status string) ui.Tone {
	switch status {
	case "active":
		return ui.ToneOK
	case "inactive":
		return ui.ToneDanger
	}
	return ui.ToneWarn
}

func capitalized(s string) string {
	if s == "" {
		return s
	}
	return string(s[0]-'a'+'A') + s[1:]
}

func lastLogin(t, now time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return ui.Ago(t, now)
}

// thousands writes n with a comma between thousands: "1,000".
func thousands(n int) string {
	s := strconv.Itoa(n)
	for i := len(s) - 3; i > 0 && s[i-1] != '-'; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func personOr(m map[string]people.Person, id string) people.Person {
	if p, ok := m[id]; ok {
		return p
	}
	return people.Person{ID: id, Name: id}
}

// AdminMenuID is the accounts' row menu.
const AdminMenuID = "admin-row-menu"

// adminHidden lists the row menu's actions that do not apply to an
// account: the other of Reactivate and Deactivate, and for the admin's
// own account Deactivate and Erase.
func adminHidden(r people.AccountRow, me string) string {
	switch {
	case r.Person.ID == me && r.Status == "inactive":
		return "deactivate erase"
	case r.Person.ID == me:
		return "reactivate deactivate erase"
	case r.Status == "inactive":
		return "deactivate"
	default:
		return "reactivate"
	}
}

// The admin's and the moderator's actions: one pending op each page.
var (
	adminSignals = `{_admin: {op: '', ok: false, msg: '', email: '', username: '', name: '', role: 'moderator', password: ''}}`
	adminSettled = Settle("_admin.op", "$_admin.ok = o.ok, $_admin.msg = o.ok ? 'Done.' : o.msg, o.ok && ($_admin.password = '')")
	adminFailed  = Failed("_admin.op")
	adminBusy    = Busy("_admin.op")
	adminKind    = `$_admin.msg ? ($_admin.ok ? 'ok' : 'refused') : false`
	adminCreate  = Act("_admin.op", "/cmd/admin/create", "email: $_admin.email, username: $_admin.username, name: $_admin.name, role: $_admin.role, password: $_admin.password")
	adminRole    = Act("_admin.op", "/cmd/admin/role", "account: el.dataset.account, role: el.dataset.role")
	adminActive  = Act("_admin.op", "/cmd/admin/active", "account: el.dataset.account, active: el.dataset.active === 'true'")
	// adminMenuAction runs the row menu's action for the account it opened
	// for; Erase asks first, through the hidden button beside the menu.
	adminMenuAction = `((a, id) => a.startsWith('role:') ? ` + Act("_admin.op", "/cmd/admin/role", "account: id, role: a.slice(5)") +
		` : a === 'reactivate' || a === 'deactivate' ? ` + Act("_admin.op", "/cmd/admin/active", "account: id, active: a === 'reactivate'") +
		` : a === 'erase' && ((b) => (b.dataset.account = id, b.click()))(document.getElementById('` + AdminMenuID + `-erase')))(evt.detail.action, evt.detail.contextId)`
	adminErase = Ask("Erase this account? Its email, username, name and sessions are deleted for good, and what it did shows as by \"Deleted person\".", "Erase", Act("_admin.op", "/cmd/admin/erase", "account: el.dataset.account"))
	adminReset = Ask("Reset the demo now? Every visitor's changes are deleted, and every open page shows the fresh demo.", "Reset now", Act("_admin.op", "/cmd/admin/reset", ""))
	modSignals = `{_mod: {op: '', ok: false, msg: '', reason: ''}}`
	modSettled = Settle("_mod.op", "$_mod.ok = o.ok, $_mod.msg = o.ok ? 'Done.' : o.msg")
	modFailed  = Failed("_mod.op")
	modBusy    = Busy("_mod.op")
	modKind    = `$_mod.msg ? ($_mod.ok ? 'ok' : 'refused') : false`
	modAct     = Act("_mod.op", "/cmd/moderation/act", "action: el.dataset.action, kind: el.dataset.kind, id: el.dataset.id, reason: $_mod.reason")
	modDelete  = Ask("Delete this for good? It cannot be restored.", "Delete for good", modAct)
	modBlock   = Act("_mod.op", "/cmd/moderation/block", "person: el.dataset.person, reason: $_mod.reason")
	modUnblock = Act("_mod.op", "/cmd/moderation/unblock", "person: el.dataset.person")
	modDismiss = Act("_mod.op", "/cmd/moderation/dismiss", "report: +el.dataset.report")
	// The report dialog (DM-08), which a Report button opens with its
	// thing's kind and id in data-report-kind and data-report-id.
	reportSignals  = `{_report: {op: '', kind: '', id: '', reason: '', msg: '', ok: false}}`
	reportOpen     = `$_report.kind = el.dataset.reportKind, $_report.id = el.dataset.reportId, $_report.reason = '', $_report.msg = '', $_report.ok = false, el.closest('[popover]')?.hidePopover(), document.getElementById('` + ReportID + `').showModal()`
	reportSend     = `$_report.reason.trim() && ` + Act("_report.op", "/cmd/report", "kind: $_report.kind, id: $_report.id, reason: $_report.reason")
	reportSettled  = Settle("_report.op", "$_report.ok = o.ok, $_report.msg = o.ok ? 'Thank you. A moderator will look at it.' : o.msg")
	reportFailed   = Failed("_report.op")
	accountSignals = `{_pw: {op: '', ok: false, msg: '', current: '', password: '', again: ''}}`
	pwChange       = `$_pw.password !== $_pw.again ? ($_pw.ok = false, $_pw.msg = 'The new passwords differ.') : ` + Act("_pw.op", "/cmd/account/password", "current: $_pw.current, password: $_pw.password")
	pwSettled      = Settle("_pw.op", "$_pw.ok = o.ok, $_pw.msg = o.ok ? 'Your password is changed. Your other sessions ended.' : o.msg, o.ok && ($_pw.current = '', $_pw.password = '', $_pw.again = '', $_goto = '/account')")
	pwFailed       = `evt.detail.el === el && ['error', 'retries-failed'].includes(evt.detail.type) && ($_pw.op = '', $_pw.ok = false, $_pw.msg = evt.detail.argsRaw.status === '429' ? 'Too many attempts. Wait a minute.' : 'This did not go through. Try again in a moment.')`
	pwBusy         = Busy("_pw.op")
	pwKind         = `$_pw.msg ? ($_pw.ok ? 'ok' : 'refused') : false`
	sessionEnd     = Act("_pw.op", "/cmd/account/end-session", "session: el.dataset.session")
)

// zurich is the demo's time zone, for times of day (Q21).
var zurich = func() *time.Location {
	l, err := time.LoadLocation("Europe/Zurich")
	if err != nil {
		return time.UTC
	}
	return l
}()

// browserName is a user agent as a person recognises it.
func browserName(ua string) string {
	for _, b := range []struct{ token, name string }{{"Firefox/", "Firefox"}, {"Edg/", "Edge"}, {"Chrome/", "Chrome"}, {"Safari/", "Safari"}, {"curl/", "curl"}, {"Go-http-client", "a script"}} {
		if strings.Contains(ua, b.token) {
			os := ""
			for _, o := range []struct{ token, name string }{{"Windows", "Windows"}, {"Mac OS X", "macOS"}, {"Android", "Android"}, {"iPhone", "iOS"}, {"Linux", "Linux"}} {
				if strings.Contains(ua, o.token) {
					os = " on " + o.name
					break
				}
			}
			return b.name + os
		}
	}
	if ua == "" {
		return "An unknown browser"
	}
	return ua
}
