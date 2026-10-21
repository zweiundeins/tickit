package web

import (
	"net/http"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/tracker"
)

// routesWorkflow registers a project's workflow and board columns (FR-36,
// FR-37, FR-43), each a project Admin's action (requirements 3.2) whose
// refusal reaches the settings on the tab's stream:
//
//	/cmd/projects/status            {project, name, category}
//	/cmd/projects/status/rename     {project, status, name}
//	/cmd/projects/status/category   {project, status, category}
//	/cmd/projects/status/move       {project, status, earlier}
//	/cmd/projects/status/remove     {project, status, move_to}
//	/cmd/projects/status/rule       {project, status, any, from}
//	/cmd/projects/status/column     {project, status, column}
//	/cmd/projects/column            {project, name}
//	/cmd/projects/column/edit       {project, column, name, limit}
//	/cmd/projects/column/move       {project, column, earlier}
//	/cmd/projects/column/remove     {project, column}
func (s *Server) routesWorkflow() {
	s.post("POST /cmd/projects/status", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.AddStatus{By: a.By, Project: project, Name: q.Name, Category: tracker.Category(q.Category)}
	}))
	s.post("POST /cmd/projects/status/rename", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.RenameStatus{By: a.By, Project: project, Status: q.Status, Name: q.Name}
	}))
	s.post("POST /cmd/projects/status/category", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.SetStatusCategory{By: a.By, Project: project, Status: q.Status, Category: tracker.Category(q.Category)}
	}))
	s.post("POST /cmd/projects/status/move", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.MoveStatus{By: a.By, Project: project, Status: q.Status, Earlier: q.Earlier}
	}))
	s.post("POST /cmd/projects/status/remove", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.RemoveStatus{By: a.By, Project: project, Status: q.Status, MoveTo: q.MoveTo}
	}))
	s.post("POST /cmd/projects/status/rule", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.SetTransitionRule{By: a.By, Project: project, Status: q.Status, Any: q.Any, From: q.From}
	}))
	s.post("POST /cmd/projects/status/column", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.MapStatus{By: a.By, Project: project, Status: q.Status, Column: q.Column}
	}))
	s.post("POST /cmd/projects/column", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.AddColumn{By: a.By, Project: project, Name: q.Name}
	}))
	s.post("POST /cmd/projects/column/edit", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.EditColumn{By: a.By, Project: project, Column: q.Column, Name: q.Name, Limit: q.Limit}
	}))
	s.post("POST /cmd/projects/column/move", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.MoveColumn{By: a.By, Project: project, Column: q.Column, Earlier: q.Earlier}
	}))
	s.post("POST /cmd/projects/column/remove", s.workflowAction(func(a action, project string, q workflowRequest) writer.Command {
		return tracker.RemoveColumn{By: a.By, Project: project, Column: q.Column}
	}))
}

// workflowRequest is every workflow action's payload; each reads the
// fields it names.
type workflowRequest struct {
	projectRequest
	Status   string   `json:"status"`
	Column   string   `json:"column"`
	Name     string   `json:"name"`
	Category string   `json:"category"`
	Earlier  bool     `json:"earlier"`
	MoveTo   string   `json:"move_to"`
	Any      bool     `json:"any"`
	From     []string `json:"from"`
	Limit    int      `json:"limit"`
}

// valid checks the ids a request names: a status, a column or a target
// is a stable id, never text a person typed.
func (q workflowRequest) valid() bool {
	id := func(s string) bool { return s == "" || statusPattern.MatchString(s) }
	ok := id(q.Status) && id(q.Column) && id(q.MoveTo) && len(q.From) <= tracker.MaxStatuses && len(q.Category) <= 20
	for _, f := range q.From {
		ok = ok && statusPattern.MatchString(f)
	}
	return ok
}

// workflowAction decodes a workflow action, resolves its project and
// runs the command cmd makes of it.
func (s *Server) workflowAction(cmd func(a action, project string, q workflowRequest) writer.Command) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req workflowRequest
		a, ok := s.decode(w, r, classField, &req)
		if !ok {
			return
		}
		if !req.valid() {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if id, ok := s.projectID(w, r, req.Project); ok {
			s.act(w, r, a, cmd(a, id, req))
		}
	}
}
