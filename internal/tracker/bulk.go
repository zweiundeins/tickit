package tracker

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/demo/words"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// BulkDelete is the "field" of a bulk edit that deletes its issues.
const BulkDelete = "delete"

// BulkEdit changes one field of many issues, or deletes them (FR-58.1).
// Each issue goes through the command a single edit of it sends, in a
// savepoint of its own, so each changes or is refused on its own
// (FR-58.2) by the same rules. A bulk edit writes the last value: it sends
// each issue's version as it is now, so a field someone else changed
// since the page showed it is overwritten, and the history says by whom
// (MP-08). Its result is a BulkResult as JSON; with every issue refused it
// is refused as a *BulkRefused, a denial when every one was for want of a
// right.
type BulkEdit struct {
	By     tabs.By
	Issues []string
	// Field is FieldStatus, FieldAssignee, FieldPriority, FieldLabels,
	// FieldSprint or BulkDelete.
	Field string
	// Value is the status's id, the assignee ("" for none), the priority,
	// the label, the sprint's id ("" for the backlog).
	Value string
	// Remove takes the label off instead of adding it.
	Remove bool
}

// BulkResult is what became of each issue of a bulk edit, in its order.
type BulkResult struct {
	Field   string        `json:"field"`
	Changed []string      `json:"changed"` // the issues' keys
	Refused []BulkRefusal `json:"refused"`
}

// BulkRefusal is an issue a bulk edit refused and why. Key is empty for
// an issue the actor may not read, which a refusal does not name.
type BulkRefusal struct {
	ID  string `json:"id"`
	Key string `json:"key,omitempty"`
	Msg string `json:"msg"`
}

func (c BulkEdit) Validate() error {
	if err := checkBy(c.By); err != nil {
		return err
	}
	if len(c.Issues) == 0 || len(c.Issues) > MaxBatch {
		return errors.New("tracker: 1 to 100 issues")
	}
	for i, id := range c.Issues {
		if !ValidID(id) || slices.Contains(c.Issues[:i], id) {
			return errors.New("tracker: a malformed or repeated issue")
		}
	}
	switch c.Field {
	case FieldStatus:
		if c.Value == "" {
			return errors.New("tracker: a bulk edit needs a status")
		}
	case FieldPriority:
		if !Priority(c.Value).Valid() {
			return errors.New("tracker: no such priority")
		}
	case FieldAssignee:
		if !validPerson(c.Value) {
			return errors.New("tracker: a malformed assignee")
		}
	case FieldSprint:
		if c.Value != "" && !ValidID(c.Value) {
			return errors.New("tracker: a malformed sprint")
		}
	case FieldLabels, BulkDelete:
	default:
		return fmt.Errorf("tracker: no bulk edit of %q", c.Field)
	}
	return nil
}

// one is the single edit of an issue at version seen.
func (c BulkEdit) one(id string, seen int64) writer.Command {
	switch c.Field {
	case FieldStatus:
		return SetStatus{By: c.By, Issue: id, Seen: seen, Status: c.Value}
	case FieldPriority:
		return SetPriority{By: c.By, Issue: id, Seen: seen, Priority: Priority(c.Value)}
	case FieldAssignee:
		return Assign{By: c.By, Issue: id, Seen: seen, Assignee: c.Value}
	case FieldSprint:
		return SetSprint{By: c.By, Issue: id, Seen: seen, Sprint: c.Value}
	case FieldLabels:
		if c.Remove {
			return SetLabels{By: c.By, Issue: id, Remove: []string{c.Value}}
		}
		return SetLabels{By: c.By, Issue: id, Add: []string{c.Value}}
	}
	return DeleteIssue{By: c.By, Issue: id}
}

func (c BulkEdit) Apply(tx *writer.Tx) error {
	_, err := c.ApplyResult(tx)
	return err
}

func (c BulkEdit) ApplyResult(tx *writer.Tx) ([]byte, error) {
	if c.Field == FieldLabels { // one refusal for the label, not one per issue
		l, ok := NormalizeLabel(c.Value)
		switch {
		case !ok:
			return nil, tabs.Refuse(FieldLabels, "A label has 1 to 50 characters and no spaces.")
		case words.Blocked(l):
			return nil, tabs.Refuse(FieldLabels, words.Refusal)
		}
	}
	res := BulkResult{Field: c.Field, Changed: []string{}, Refused: []BulkRefusal{}}
	denied := 0
	for _, id := range c.Issues {
		var s IssueState
		err := tx.Savepoint(func(tx *writer.Tx) error {
			var err error
			if s, err = issueState(tx, id); err != nil {
				return err
			}
			return c.one(id, s.Version).Apply(tx)
		})
		var refusal *writer.Refusal
		var denial interface{ Denied() bool }
		switch {
		case err == nil:
			res.Changed = append(res.Changed, s.Key)
		case errors.Is(err, access.ErrNotFound):
			denied++
			res.Refused = append(res.Refused, BulkRefusal{ID: id, Msg: "This issue does not exist, or you cannot see it."})
		case errors.As(err, &refusal):
			if errors.As(err, &denial) && denial.Denied() {
				denied++
			}
			res.Refused = append(res.Refused, BulkRefusal{ID: id, Key: s.Key, Msg: refusal.Msg})
		default:
			return nil, err
		}
	}
	b, err := json.Marshal(res)
	if err != nil {
		return nil, err
	}
	if len(res.Changed) == 0 {
		return nil, &BulkRefused{Refusal: writer.Refusal{Msg: res.Message()}, Data: b, denied: denied == len(res.Refused)}
	}
	return b, nil
}

// BulkRefused is a bulk edit that changed no issue: its data is the
// BulkResult, which names each issue and why.
type BulkRefused struct {
	writer.Refusal
	Data   json.RawMessage
	denied bool
}

func (r *BulkRefused) Error() string                { return r.Msg }
func (r *BulkRefused) Unwrap() error                { return &r.Refusal }
func (r *BulkRefused) OutcomeData() json.RawMessage { return r.Data }
func (r *BulkRefused) Denied() bool                 { return r.denied }

// Message says in words what a bulk edit did: how many issues changed,
// then one line per refused issue, "WEB-12: Only admins of this project
// can delete issues."
func (r BulkResult) Message() string {
	did := "changed"
	if r.Field == BulkDelete {
		did = "deleted"
	}
	var b strings.Builder
	switch n := len(r.Changed); {
	case n == 0:
		b.WriteString("No issue was " + did + ".")
	case n == 1:
		b.WriteString("1 issue " + did + ".")
	default:
		fmt.Fprintf(&b, "%d issues %s.", n, did)
	}
	switch n := len(r.Refused); {
	case n == 1 && len(r.Changed) > 0:
		b.WriteString(" 1 was not:")
	case n > 1 && len(r.Changed) > 0:
		fmt.Fprintf(&b, " %d were not:", n)
	}
	for _, x := range r.Refused {
		b.WriteString("\n")
		if x.Key != "" {
			b.WriteString(x.Key + ": ")
		}
		b.WriteString(x.Msg)
	}
	return b.String()
}

// BulkNote is tabs.Act's Note for a bulk edit that went through: the
// outcome says what changed and names the issues refused (FR-58.2).
func BulkNote(result []byte) (string, json.RawMessage) {
	var r BulkResult
	if json.Unmarshal(result, &r) != nil {
		return "", nil
	}
	return r.Message(), result
}
