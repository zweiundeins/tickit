package notifications

import (
	"net/http"

	"github.com/mbolli/tickit/internal/sharedhttp"

	"github.com/gorilla/sessions"
)

// POST /api/notifications - Create new notification
func HandleCreateNotification(store sessions.Store, notificationService *NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, nil, func(sessionID string, _ any) {
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

func HandleDeleteNotification(store sessions.Store, notificationService *NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, &deleteSignals{}, func(sessionID string, s any) {
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

func HandleMarkNotificationRead(store sessions.Store, notificationService *NotificationService) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sharedhttp.WithSessionAndSignals(store, w, r, &readSignals{}, func(sessionID string, s any) {
			signals := s.(*readSignals)
			if err := notificationService.MarkNotificationRead(r.Context(), sessionID, signals.NotificationUID); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})
	}
}
