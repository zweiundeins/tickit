package wiki

import (
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/doc"
	"github.com/zweiundeins/tickit/internal/tabs"
)

// ChipSelection puts an issue's chip in place of a text a reader selected
// on a page in view mode (FR-105): the nth occurrence of find in the
// document's top-level block at index block (doc.FindInBlock), in one
// commit on the head that open editors apply like any other. The actor
// must be a Member of the page's space. It reports whether it found the
// text; a page that changed since the reader saw it may no longer hold it.
func ChipSelection(tx *writer.Tx, by tabs.By, page string, block, nth int, find, key string) (bool, error) {
	if _, err := current(tx, by, page, access.EditIssues); err != nil {
		return false, err
	}
	if !doc.IssueKey(key) {
		return false, nil
	}
	ds, err := pageDocs.State(tx, page)
	if err != nil {
		return false, err
	}
	head, err := pageDocs.StateDoc(tx.Context(), tx, page, ds)
	if err != nil {
		return false, err
	}
	from, to, ok := doc.FindInBlock(head, block, find, nth)
	if !ok {
		return false, nil
	}
	step, err := doc.ChipStep(from, to, key)
	if err != nil {
		return false, err
	}
	next, err := step.Apply(head)
	if err != nil {
		return false, nil
	}
	if by.At.IsZero() {
		by.At = at(by)
	}
	return pageDocs.CommitWhole(tx, by, page, next, "chip-"+by.Op, "body", nil)
}
