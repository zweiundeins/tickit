package web

import (
	"net/http"

	"github.com/zweiundeins/tickit/internal/people"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// Bulk edit (FR-58): the issue list's selection is the tab's own (the
// signal $_bulk, views/list_bulk.go), and POST /cmd/issues/bulk
// {tab, op, issues, field, value, ref} changes the selected issues'
// status, assignee ("me" for the actor), priority, a label (ref "add" or
// "remove") or sprint, or deletes them (field "delete"), through
// tracker.BulkEdit in tabs.Act. The outcome names the issues refused and
// why (tracker.BulkNote); a bulk edit refused for want of a right on
// every issue answers 403.

func (s *Server) routesBulk() {
	s.post("POST /cmd/issues/bulk", s.bulkEdit)
}

type bulkRequest struct {
	Payload
	Issues []string `json:"issues"`
	Field  string   `json:"field"`
	Value  string   `json:"value"`
	Ref    string   `json:"ref"`
}

func (s *Server) bulkEdit(w http.ResponseWriter, r *http.Request) {
	var req bulkRequest
	a, ok := s.decode(w, r, classBulk, &req)
	if !ok {
		return
	}
	cmd := tracker.BulkEdit{By: a.By, Issues: req.Issues, Field: req.Field, Value: req.Value}
	switch {
	case req.Field == tracker.FieldAssignee && req.Value == "me":
		cmd.Value = a.By.Actor.ID
	case req.Field == tracker.FieldLabels && req.Ref == "remove":
		cmd.Remove = true
	case req.Field == tracker.FieldLabels && req.Ref != "add":
		http.Error(w, "add or remove", http.StatusBadRequest)
		return
	}
	if cmd.Field == tracker.FieldAssignee && cmd.Value != "" && !people.ValidID(cmd.Value) {
		http.Error(w, "bad assignee", http.StatusBadRequest)
		return
	}
	res, ok := s.runNoted(w, r, a, cmd, tracker.BulkNote)
	switch {
	case !ok:
	case res.Denied:
		http.Error(w, "forbidden", http.StatusForbidden)
	default:
		w.WriteHeader(http.StatusNoContent)
	}
}
