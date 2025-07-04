package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/delaneyj/toolbelt"
	"github.com/google/uuid"
	"net/http"
	"time"

	"github.com/mbolli/tickit/internal/models"

	"github.com/delaneyj/toolbelt/embeddednats"
	"github.com/starfederation/datastar/sdk/go/datastar"

	"github.com/go-chi/chi/v5"
	"github.com/gorilla/sessions"
	"github.com/mbolli/tickit/internal/ui/components"
	"github.com/mbolli/tickit/internal/ui/pages"
	"github.com/nats-io/nats.go/jetstream"
)

func setupIndexRoute(router chi.Router, store sessions.Store, ns *embeddednats.Server) error {
	nc, err := ns.Client()
	if err != nil {
		return fmt.Errorf("error creating nats client: %w", err)
	}

	js, err := jetstream.New(nc)
	if err != nil {
		return fmt.Errorf("error creating jetstream client: %w", err)
	}

	kv, err := js.CreateOrUpdateKeyValue(context.Background(), jetstream.KeyValueConfig{
		Bucket:      "notifications",
		Description: "Notifications",
		Compression: true,
		TTL:         time.Hour,
		MaxBytes:    16 * 1024 * 1024,
	})

	if err != nil {
		return fmt.Errorf("error creating key value: %w", err)
	}

	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		if err := pages.Index().Render(r.Context(), w); err != nil {
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	})

	router.Route("/api", func(apiRouter chi.Router) {
		apiRouter.Route("/notifications", func(notifRouter chi.Router) {
			notifRouter.Get("/", func(w http.ResponseWriter, r *http.Request) {
				sessionID, err := upsertSessionID(store, r, w)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				fmt.Println("sessionID:", sessionID)

				sse := datastar.NewSSE(w, r)

				// Watch for updates
				ctx := r.Context()
				watcher, err := kv.Watch(ctx, sessionID)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				defer watcher.Stop()

				for {
					select {
					case <-ctx.Done():
						return
					case entry := <-watcher.Updates():
						if entry == nil {
							continue
						}

						ntfs := &models.Notifications{}

						// Key was updated, so update notifications
						json.Unmarshal(entry.Value(), ntfs)

						fmt.Println("sessionID:", sessionID)
						fmt.Printf("GET notifications: %+v\n", ntfs.Notifications)

						// Update the SSE connection with the new notifications
						if err := sse.PatchElementTempl(components.Notifications(ntfs)); err != nil {
							if err := sse.ConsoleError(err); err != nil {
								http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
							}
							return
						}
					}
				}
			})

			notifRouter.Post("/", func(w http.ResponseWriter, r *http.Request) {
				sessionID, _ := upsertSessionID(store, r, w)
				ntfs, err := models.GetNotifications(sessionID, r.Context(), kv)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}

				ntfs.Notifications = append(ntfs.Notifications, &models.Notification{
					Uid:   uuid.New().String(),
					Date:  time.Now().Format(time.RFC3339),
					Title: r.FormValue("title"),
					Body:  r.FormValue("body"),
				})

				if err := models.SaveNotifications(r.Context(), kv, sessionID, ntfs); err != nil {
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}

			})

			// Reset notifications
			notifRouter.Delete("/", func(w http.ResponseWriter, r *http.Request) {
				sessionID, _ := upsertSessionID(store, r, w)

				if err := models.DeleteNotifications(r, kv, sessionID); err != nil {
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			})

			// Mark notifications as read
			notifRouter.Patch("/read", func(w http.ResponseWriter, r *http.Request) {
				sessionID, _ := upsertSessionID(store, r, w)

				if err := models.ReadNotifications(r, kv, sessionID); err != nil {
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			})
		})
	})

	return nil
}

func MustJSONMarshal(v any) string {
	b, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		panic(err)
	}
	return string(b)
}

func upsertSessionID(store sessions.Store, r *http.Request, w http.ResponseWriter) (string, error) {
	sess, err := store.Get(r, "connections")
	if err != nil {
		return "", fmt.Errorf("failed to get session: %w", err)
	}

	id, ok := sess.Values["id"].(string)

	if !ok {
		id = toolbelt.NextEncodedID()
		sess.Values["id"] = id
		if err := sess.Save(r, w); err != nil {
			return "", fmt.Errorf("failed to save session: %w", err)
		}
	}

	return id, nil
}
