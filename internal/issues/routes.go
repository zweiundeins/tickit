package issues

import (
	"encoding/json"
	"net/http"

	"github.com/mbolli/tickit/internal/sharedhttp"

	"github.com/gorilla/sessions"
)

// POST /api/issues - Create new issue
func HandleCreateIssue(store sessions.Store, issueService *IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			title := r.FormValue("title")
			description := r.FormValue("description")
			assignee := r.FormValue("assignee")
			reporter := sessionID // For demo, use session as reporter
			if _, err := issueService.CreateIssue(r.Context(), title, description, assignee, reporter); err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
			// No response body needed
		})
	}
}

// PATCH /api/issues/{id} - Update issue
func HandleUpdateIssue(store sessions.Store, issueService *IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			// For demo, get fields from form values
			id := r.URL.Query().Get("id")
			title := r.FormValue("title")
			description := r.FormValue("description")
			status := r.FormValue("status")
			assignee := r.FormValue("assignee")
			// TODO: Call issueService.UpdateIssue with only provided fields
			_ = id
			_ = title
			_ = description
			_ = status
			_ = assignee
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// DELETE /api/issues/{id} - Delete issue
func HandleDeleteIssue(store sessions.Store, issueService *IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			id := r.URL.Query().Get("id")
			// TODO: Call issueService.DeleteIssue
			_ = id
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// GET /api/issues/{id} - Get issue by ID
func HandleGetIssueByID(store sessions.Store, issueService *IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			id := r.URL.Query().Get("id")
			// TODO: Call issueService.GetIssueByID
			_ = id
			writeJSON(w, &Issue{})
		})
	}
}

// GET /api/issues - List issues
func HandleListIssues(store sessions.Store, issueService *IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			// TODO: Parse filters from query params
			// TODO: Call issueService.ListIssues
			writeJSON(w, []*Issue{})
		})
	}
}

// Helper to write JSON response
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
