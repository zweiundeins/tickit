package web

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/zweiundeins/tickit/internal/dbtest"
)

// NFR-39: the page's "Create issue" finds the project a person created an
// issue in last by its index, not by walking every issue (FR-105).
func TestLastReportedPlan(t *testing.T) {
	e := dbtest.Open(t)
	var plan []string
	e.View(t, func(tx *sql.Tx) error {
		rows, err := tx.Query(`EXPLAIN QUERY PLAN `+lastReported, "g-nobody")
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, parent, unused int
			var detail string
			if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
				return err
			}
			plan = append(plan, detail)
		}
		return rows.Err()
	})
	if p := strings.Join(plan, "; "); !strings.Contains(p, "USING INDEX issues_reporter (reporter=?)") {
		t.Errorf("plan: %s", p)
	}
}
