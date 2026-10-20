package tracker

import (
	"testing"

	"github.com/zweiundeins/tickit/internal/demo/words"
)

// DM-06.4: labels and project names with a word of the list are refused.
func TestWordList(t *testing.T) {
	e := newEnv(t)
	pid := e.project("WRD", true)
	id := e.issue(sunny, CreateIssue{Project: pid})
	if msg := refusal(t, e.do(SetLabels{By: e.by(sunny), Issue: id, Add: []string{"ok", "casino"}})); msg != words.Refusal {
		t.Errorf("label: %q", msg)
	}
	if msg := refusal(t, e.do(CreateIssue{By: e.by(sunny), Project: pid, Type: Task, Summary: "x", Labels: []string{"p0rn"}})); msg != words.Refusal {
		t.Errorf("label at creation: %q", msg)
	}
	name := "Viagra shop"
	if msg := refusal(t, e.do(EditProject{By: e.by(admin), Project: pid, Name: &name})); msg != words.Refusal {
		t.Errorf("project name: %q", msg)
	}
	if msg := refusal(t, e.do(CreateProject{By: e.by(sunny), Key: "XXX", Name: "Fine"})); msg != words.Refusal {
		t.Errorf("project key: %q", msg)
	}
	e.must(SetLabels{By: e.by(sunny), Issue: id, Add: []string{"specialist"}})
}
