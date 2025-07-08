package sharedhttp

import (
	"fmt"
	"net/http"

	"github.com/delaneyj/toolbelt"
	"github.com/gorilla/sessions"
	"github.com/starfederation/datastar/sdk/go/datastar"
)

// upsertSessionID ensures a session ID exists and returns it
func UpsertSessionID(store sessions.Store, r *http.Request, w http.ResponseWriter) (string, error) {
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

// WithSessionAndSignals prepares sessionID and reads signals into the provided struct
func WithSessionAndSignals(store sessions.Store, w http.ResponseWriter, r *http.Request, signals any, handler func(sessionID string, signals any)) {
	sessionID, err := UpsertSessionID(store, r, w)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if signals != nil {
		_ = datastar.ReadSignals(r, signals) // Ignore error, signals will be zero if not provided
	}
	handler(sessionID, signals)
}
