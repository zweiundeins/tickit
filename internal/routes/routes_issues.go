package routes

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/cqrs/application"
	"github.com/mbolli/tickit/internal/models"
)

// Routing helper for issues (reuse withSessionAndSignals if available)
func withSessionAndSignalsIssue(store sessions.Store, w http.ResponseWriter, r *http.Request, signals any, handler func(sessionID string, signals any)) {
	sessionID, err := upsertSessionID(store, r, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if signals != nil {
		// For now, assume signals are not used for issues, but can be extended
	}
	handler(sessionID, signals)
}

// POST /api/issues - Create new issue
func handleCreateIssue(store sessions.Store, issueService *application.IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withSessionAndSignalsIssue(store, w, r, nil, func(sessionID string, _ any) {
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
func handleUpdateIssue(store sessions.Store, issueService *application.IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withSessionAndSignalsIssue(store, w, r, nil, func(sessionID string, _ any) {
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
func handleDeleteIssue(store sessions.Store, issueService *application.IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withSessionAndSignalsIssue(store, w, r, nil, func(sessionID string, _ any) {
			id := r.URL.Query().Get("id")
			// TODO: Call issueService.DeleteIssue
			_ = id
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// GET /api/issues/{id} - Get issue by ID
func handleGetIssueByID(store sessions.Store, issueService *application.IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withSessionAndSignalsIssue(store, w, r, nil, func(sessionID string, _ any) {
			id := r.URL.Query().Get("id")
			// TODO: Call issueService.GetIssueByID
			_ = id
			writeJSON(w, &models.Issue{})
		})
	}
}

// GET /api/issues - List issues
func handleListIssues(store sessions.Store, issueService *application.IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withSessionAndSignalsIssue(store, w, r, nil, func(sessionID string, _ any) {
			// TODO: Parse filters from query params
			// TODO: Call issueService.ListIssues
			writeJSON(w, []*models.Issue{})
		})
	}
}

// Helper to write JSON response
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
