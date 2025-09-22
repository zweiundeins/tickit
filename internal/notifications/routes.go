package notifications

import (
	"github.com/mbolli/tickit/internal/shared"
	"net/http"
)

// POST /api/notifications - Create new notification
func HandleCreateNotification(notificationService *NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		title := r.FormValue("title")
		body := r.FormValue("body")
		sessionID := r.Context().Value("session_id").(string)
		if err := notificationService.CreateNotification(r.Context(), sessionID, title, body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

// DELETE /api/notifications - Delete notification(s)
type deleteSignals struct {
	NotificationUID string `json:"notificationuid"`
}

func HandleDeleteNotification(notificationService *NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		signals, err := shared.SignalsAsStruct[deleteSignals](r)
		if err != nil {
			http.Error(w, "invalid signals: "+err.Error(), http.StatusBadRequest)
			return
		}

		sessionID := r.Context().Value("session_id").(string)
		if err := notificationService.DeleteNotification(r.Context(), sessionID, signals.NotificationUID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// PATCH /api/notifications/read - Mark notification(s) as read
type readSignals struct {
	NotificationUID string `json:"notificationuid"`
}

func HandleMarkNotificationRead(notificationService *NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		signals, err := shared.SignalsAsStruct[readSignals](r)
		if err != nil {
			http.Error(w, "invalid signals: "+err.Error(), http.StatusBadRequest)
			return
		}
		sessionID := r.Context().Value("session_id").(string)

		if err := notificationService.MarkNotificationRead(r.Context(), sessionID, signals.NotificationUID); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
