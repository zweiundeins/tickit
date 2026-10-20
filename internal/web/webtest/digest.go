package webtest

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
	"testing"
)

// Digests is a hash of each table's rows, by table, but the tables and
// columns skip names ("tab_state", "login_sessions.last_seen_at"): what
// a test allows a request to write besides nothing.
func (e *Env) Digests(t testing.TB, skip ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := e.DB.View(context.Background(), func(tx *sql.Tx) error {
		var tables []string
		rows, err := tx.Query(`SELECT name FROM sqlite_schema WHERE type = 'table' ORDER BY name`)
		if err != nil {
			return err
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return err
			}
			tables = append(tables, name)
		}
		rows.Close()
		for _, table := range tables {
			if slices.Contains(skip, table) {
				continue
			}
			cols, err := columns(tx, table)
			if err != nil {
				return err
			}
			cols = slices.DeleteFunc(cols, func(c string) bool { return slices.Contains(skip, table+"."+c) })
			lines, err := dumpColumns(tx, table, cols)
			if err != nil {
				return err
			}
			h := sha256.Sum256([]byte(strings.Join(lines, "\n")))
			out[table] = hex.EncodeToString(h[:])
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func quote(name string) string { return `"` + strings.ReplaceAll(name, `"`, `""`) + `"` }

func columns(tx *sql.Tx, table string) ([]string, error) {
	rows, err := tx.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func dumpColumns(tx *sql.Tx, table string, cols []string) ([]string, error) {
	if len(cols) == 0 {
		return nil, nil
	}
	q := make([]string, len(cols))
	for i, c := range cols {
		q[i] = quote(c)
	}
	rows, err := tx.Query(`SELECT ` + strings.Join(q, ", ") + ` FROM ` + quote(table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lines []string
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		lines = append(lines, fmt.Sprintf("%q", vals))
	}
	slices.Sort(lines)
	return lines, rows.Err()
}
