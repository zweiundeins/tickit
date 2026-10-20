package tracker

import (
	"errors"
	"testing"

	"github.com/zweiundeins/tickit/internal/access"
)

// DM-07.1: a moderator hides an issue, which leaves the lists as a deleted
// one does, and restores it; nobody else may.
func TestHideIssue(t *testing.T) {
	e := newEnv(t)
	pid := e.project("MOD", true)
	id := e.issue(sunny, CreateIssue{Project: pid, Summary: "Buy cheap watches"})
	var denial *access.Denial
	for _, p := range []struct {
		name string
		err  error
	}{{"a guest", e.do(HideIssue{By: e.by(sunny), Issue: id, Hidden: true})}, {"a project admin", e.do(HideIssue{By: e.by(admin), Issue: id, Hidden: true})}} {
		if !errors.As(p.err, &denial) {
			t.Errorf("%s hides: %v", p.name, p.err)
		}
	}
	e.Touched()
	e.must(HideIssue{By: e.by(moderator), Issue: id, Hidden: true, Reason: "spam"})
	if got := e.Touched(); !contains(got, IssueTopic(id)) || !contains(got, BoardTopic(pid)) {
		t.Errorf("hiding touched %v", got)
	}
	if e.row(`SELECT count(*) FROM issues WHERE id = ? AND deleted_at IS NOT NULL AND hidden_by = ?`, id, moderator.ID) != "1" {
		t.Error("the hidden issue is not gone from the lists")
	}
	e.must(HideIssue{By: e.by(siteAdmin), Issue: id, Hidden: false})
	if e.row(`SELECT count(*) FROM issues WHERE id = ? AND deleted_at IS NULL AND hidden_at IS NULL`, id) != "1" {
		t.Error("the restored issue did not come back")
	}
	e.must(DeleteIssue{By: e.by(moderator), Issue: id})
	if msg := refusal(t, e.do(HideIssue{By: e.by(moderator), Issue: id, Hidden: true})); msg != errDeleted.Msg {
		t.Errorf("hiding a deleted issue: %q", msg)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
