package projects

import (
	"encoding/json"
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/sharedhttp"
)

// POST /api/projects - Create new project
func HandleCreateProject(store sessions.Store, projectService *ProjectService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			name := r.FormValue("name")
			slug := r.FormValue("slug")
			description := r.FormValue("description")
			if _, err := projectService.CreateProject(r.Context(), name, slug, description); err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
		})
	}
}

// PATCH /api/projects/{id} - Update project
func HandleUpdateProject(store sessions.Store, projectService *ProjectService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			id := r.URL.Query().Get("id")
			if id == "" {
				id = r.FormValue("id")
			}
			if id == "" {
				http.Error(w, "Missing id", http.StatusBadRequest)
				return
			}
			var namePtr, slugPtr, descPtr *string
			if v := r.FormValue("name"); v != "" {
				namePtr = &v
			}
			if v := r.FormValue("slug"); v != "" {
				slugPtr = &v
			}
			if v := r.FormValue("description"); v != "" {
				descPtr = &v
			}
			if err := projectService.UpdateProject(r.Context(), id, namePtr, slugPtr, descPtr); err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// DELETE /api/projects/{id} - Delete project
func HandleDeleteProject(store sessions.Store, projectService *ProjectService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			id := r.URL.Query().Get("id")
			if id == "" {
				id = r.FormValue("id")
			}
			if id == "" {
				http.Error(w, "Missing id", http.StatusBadRequest)
				return
			}
			if err := projectService.DeleteProject(r.Context(), id); err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// GET /api/projects/{id} - Get project by ID
func HandleGetProjectByID(store sessions.Store, projectService *ProjectService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			id := r.URL.Query().Get("id")
			if id == "" {
				http.Error(w, "Missing id", http.StatusBadRequest)
				return
			}
			project, err := projectService.GetProjectByID(r.Context(), id)
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			if project == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			writeJSON(w, project)
		})
	}
}

// GET /api/projects - List projects
func HandleListProjects(store sessions.Store, projectService *ProjectService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			projects, err := projectService.ListProjects(r.Context())
			if err != nil {
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				return
			}
			writeJSON(w, projects)
		})
	}
}

// Helper to write JSON response
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
