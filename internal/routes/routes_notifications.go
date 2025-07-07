package routes

import (
	"net/http"

	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/cqrs/application"
	"github.com/starfederation/datastar/sdk/go/datastar"
)

// Routing helper: prepares sessionID and reads signals into the provided struct
func withSessionAndSignals(store sessions.Store, w http.ResponseWriter, r *http.Request, signals any, handler func(sessionID string, signals any)) {
	sessionID, err := upsertSessionID(store, r, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if signals != nil {
		_ = datastar.ReadSignals(r, signals) // Ignore error, signals will be zero if not provided
	}
	handler(sessionID, signals)
}

// POST /api/notifications - Create new notification
func handleCreateNotification(store sessions.Store, notificationService *application.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
			title := r.FormValue("title")
			body := r.FormValue("body")
			if err := notificationService.CreateNotification(r.Context(), sessionID, title, body); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusCreated)
		})
	}
}

// DELETE /api/notifications - Delete notification(s)
type deleteSignals struct {
	NotificationUID string `json:"notificationuid"`
}

func handleDeleteNotification(store sessions.Store, notificationService *application.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withSessionAndSignals(store, w, r, &deleteSignals{}, func(sessionID string, s any) {
			signals := s.(*deleteSignals)
			if err := notificationService.DeleteNotification(r.Context(), sessionID, signals.NotificationUID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}

// PATCH /api/notifications/read - Mark notification(s) as read
type readSignals struct {
	NotificationUID string `json:"notificationuid"`
}

func handleMarkNotificationRead(store sessions.Store, notificationService *application.NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		withSessionAndSignals(store, w, r, &readSignals{}, func(sessionID string, s any) {
			signals := s.(*readSignals)
			if err := notificationService.MarkNotificationRead(r.Context(), sessionID, signals.NotificationUID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}
