package issues

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/mbolli/tickit/internal/logger"
	"github.com/mbolli/tickit/internal/shared"
)

// POST /api/issues - Create new issue
func HandleCreateIssue(issueService *IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sessionID := r.Context().Value("session_id").(string)
		title := r.FormValue("title")
		description := r.FormValue("description")
		assignee := r.FormValue("assignee")
		reporter := sessionID // For demo, use session as reporter
		projectID := r.FormValue("project_id")
		priority := r.FormValue("priority")
		if _, err := issueService.CreateIssue(r.Context(), title, description, assignee, reporter, projectID, priority); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		// No response body needed
	}
}

// PATCH /api/issues/{id} - Update issue
// PatchIssueSignals is used to decode JSON signals for patching an issue
// Only non-nil fields will be updated
// ID is required
type PatchIssueSignals struct {
	Issue struct {
		ID          string  `json:"id"`
		Title       *string `json:"title,omitempty"`
		Description *string `json:"description,omitempty"`
		Status      *string `json:"status,omitempty"`
		Assignee    *string `json:"assignee,omitempty"`
		ProjectID   *string `json:"project_id,omitempty"`
		Priority    *string `json:"priority,omitempty"`
	} `json:"issue"`
	Extra map[string]any `json:"extra,omitempty"` // For any additional data
}

func HandlePatchIssue(issueService *IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logger.Debug().Msg("PATCH /api/issues called")
		signals, err := shared.SignalsAsStruct[PatchIssueSignals](r)
		if err != nil {
			logger.Error().Err(err).Msg("Failed to parse signals")
			http.Error(w, "invalid signals: "+err.Error(), http.StatusBadRequest)
			return
		}

		logger.Debug().Interface("signals", signals).Msg("Parsed signals successfully")

		input := PatchIssueInput{
			ID: func() string {
				if signals.Issue.ID != "" {
					return signals.Issue.ID
				}
				return chi.URLParam(r, "id")
			}(),
			Title:       signals.Issue.Title,
			Description: signals.Issue.Description,
			Status:      signals.Issue.Status,
			Assignee:    signals.Issue.Assignee,
			ProjectID:   signals.Issue.ProjectID,
			Priority:    signals.Issue.Priority,
		}
		logger.Debug().
			Interface("input", input).
			Interface("signals", signals).
			Msg("PatchIssue called")
		if _, err := issueService.PatchIssue(r.Context(), input); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// DELETE /api/issues/{id} - Delete issue
func HandleDeleteIssue(issueService *IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		if err := issueService.DeleteIssue(r.Context(), id); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent) // 204
	}
}

// GET /api/issues/{id} - Get issue by ID
func HandleGetIssueByID(issueService *IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := r.URL.Query().Get("id")
		issue, err := issueService.GetIssueByID(r.Context(), id)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if issue == nil {
			http.Error(w, "Issue not found", http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		writeJSON(w, issue)
	}
}

// GET /api/issues - List issues
type listIssuesSignals struct {
	Filter   ListIssuesFilter `json:"filter"`
	OrderBy  string           `json:"order_by"`
	OrderDir string           `json:"order_dir"`
}

func HandleListIssues(issueService *IssueService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		signals, err := shared.SignalsAsStruct[listIssuesSignals](r)
		if err != nil {
			http.Error(w, "invalid signals: "+err.Error(), http.StatusBadRequest)
			return
		}

		query := signals.Filter.ToQuery()
		issues, err := issueService.ListIssues(r.Context(), query)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		writeJSON(w, issues)
	}
}

// Helper to write JSON response
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
