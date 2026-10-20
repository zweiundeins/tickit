package tracker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/zweiundeins/tickit/internal/access"
)

// FR-16.1: the status menu reads the project's workflow, from the state
// row the commands keep.
func TestProjectWorkflow(t *testing.T) {
	e := newEnv(t)
	p := e.project("WEB", true)
	wf, err := ProjectWorkflow(context.Background(), e.DB.R, p)
	if err != nil {
		t.Fatal(err)
	}
	if len(wf.Statuses) != 7 || wf.Start != StatusToDo || !wf.Allows(StatusToDo, StatusDone) {
		t.Errorf("workflow %+v", wf)
	}
	if _, err := ProjectWorkflow(context.Background(), e.DB.R, NewID()); err == nil {
		t.Error("an unknown project has a workflow")
	}
}

// MP-07: typing shows for 5 s after the last keystroke, goes at once
// when the field closes, and needs the right to read the issue.
func TestTyping(t *testing.T) {
	e := newEnv(t)
	open, closed := e.project("WEB", true), e.project("SEC", false)
	issue := e.issue(sunny, CreateIssue{Project: open})
	secret := e.issue(admin, CreateIssue{Project: closed})
	by := e.by(sunny)
	e.Touched()
	e.must(SetTyping{By: by, Issue: issue, Field: TypingComment})
	if topics := e.Touched(); len(topics) != 1 || topics[0] != TypingTopic(issue) {
		t.Errorf("touched %v", topics)
	}
	list, err := Typing(context.Background(), e.DB.R, issue, by.At.Add(4*time.Second))
	if err != nil || len(list) != 1 || list[0].Person != sunny.ID || list[0].Field != TypingComment {
		t.Errorf("typing %v %v", list, err)
	}
	if list, _ := Typing(context.Background(), e.DB.R, issue, by.At.Add(6*time.Second)); len(list) != 0 {
		t.Errorf("still typing after 6 s: %v", list)
	}
	e.must(SetTyping{By: e.by(sunny), Issue: issue, Field: TypingComment, Stop: true})
	if list, _ := Typing(context.Background(), e.DB.R, issue, e.now); len(list) != 0 {
		t.Errorf("still typing after stop: %v", list)
	}
	if err := e.do(SetTyping{By: e.by(sunny), Issue: secret, Field: TypingComment}); err == nil || !errorsIs(err, access.ErrNotFound) {
		t.Errorf("a guest typed on a closed project's issue: %v", err)
	}
	if err := (SetTyping{By: e.by(sunny), Issue: issue, Field: "nose"}).Validate(); err == nil {
		t.Error("an unknown field")
	}
}

func errorsIs(err, target error) bool { return errors.Is(err, target) }
