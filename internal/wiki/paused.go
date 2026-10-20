package wiki

import (
	"encoding/json"

	"github.com/zweiundeins/lith/writer"
)

// An editing tab whose commits the demo's guard refuses (a blocked
// guest, MP-14.1) is paused: its row's refusal says so, and its view then
// renders the editor read-only with the reason, so the editor stops
// sending, says why at the editor and keeps the person's text, unsaved,
// in their browser only (MP-14.2). Opening the editor again clears it.

type pausedRefusal struct {
	Paused  bool   `json:"paused"`
	Message string `json:"message"`
}

// PauseEditor marks the tab's editor of the page as paused with msg, and
// touches its view the first time.
func PauseEditor(tx *writer.Tx, session, tab, page, msg string) error {
	b, err := json.Marshal(pausedRefusal{Paused: true, Message: msg})
	if err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE page_editors SET refusal = ? WHERE sid = ? AND tab_id = ? AND page_id = ? AND refusal != ?`, string(b), session, tab, page, string(b))
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n > 0 {
		tx.Touch(EditorTopic(tab))
	}
	return nil
}

// Paused is the message of a paused editor's refusal, and whether it is
// one.
func Paused(refusal string) (string, bool) {
	var r pausedRefusal
	if refusal == "" || refusal[0] != '{' || json.Unmarshal([]byte(refusal), &r) != nil || !r.Paused {
		return "", false
	}
	return r.Message, true
}
