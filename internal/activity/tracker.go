package activity

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/sqlite"

	"github.com/zweiundeins/tickit/internal/access"
	"github.com/zweiundeins/tickit/internal/tracker"
)

// The tracker's part of the feeds: issues created, moved and commented on.

func init() {
	addSource(tracker.IssueStream.Kind(), []string{tracker.IssueCreatedType.Name(), tracker.StatusChangedType.Name(), tracker.MovedType.Name()}, issueEntries)
	addSource(tracker.CommentStream.Kind(), []string{tracker.CommentAddedType.Name()}, commentEntries)
	commentSubjects[tracker.SubjectIssue] = issueSubjects
	subjectOfs[tracker.IssueStream.Kind()] = func(ev events.Raw) (string, string) {
		id, _ := ev.ID.(string)
		return id, tracker.SubjectIssue
	}
	subjectOfs[tracker.CommentStream.Kind()] = func(ev events.Raw) (string, string) {
		if ev.Type != tracker.CommentAddedType.Name() {
			return "", ""
		}
		var c struct {
			Subject   string `json:"subject"`
			SubjectID string `json:"subject_id"`
		}
		json.Unmarshal(ev.Data, &c)
		return c.SubjectID, c.Subject
	}
	mineSubjectsBy[tracker.SubjectIssue] = issueSubjects
}

func issueEntries(ctx context.Context, q sqlite.Querier, evs []events.Raw) ([]Entry, error) {
	var out []Entry
	var ids []string
	for _, ev := range evs {
		e, err := events.Decode(tracker.Store(), tracker.IssueStream, ev.Type, ev.Data)
		if err != nil {
			return nil, err
		}
		id, _ := ev.ID.(string)
		entry := Entry{Position: ev.Position, At: ev.Meta.At, Actor: ev.Meta.Actor, Subject: Subject{Kind: tracker.SubjectIssue, ID: id}}
		switch e := e.(type) {
		case tracker.IssueCreated:
			entry.Action = IssueCreated
		case tracker.StatusChanged:
			entry.Action, entry.Status = IssueMoved, e.To
		case tracker.Moved:
			if e.Status == e.FromStatus {
				continue
			}
			entry.Action, entry.Status = IssueMoved, e.Status
		default:
			continue
		}
		out = append(out, entry)
		ids = append(ids, id)
	}
	subjects, err := issueSubjects(ctx, q, ids)
	if err != nil {
		return nil, err
	}
	names, err := statusNames(ctx, q, subjects)
	if err != nil {
		return nil, err
	}
	for i := range out {
		e := &out[i]
		e.Subject = subjects[e.Subject.ID] // the zero Subject for a deleted issue
		if id := e.Status; id != "" {
			st := names[e.Subject.Scope.ID+"\x00"+id]
			e.Status, e.Category = st.name, st.category
			if e.Status == "" {
				e.Status = tracker.DefaultWorkflow().Name(id)
			}
		}
	}
	return out, nil
}

func commentEntries(ctx context.Context, q sqlite.Querier, evs []events.Raw) ([]Entry, error) {
	var out []Entry
	ids := map[string][]string{}
	for _, ev := range evs {
		e, err := events.Decode(tracker.Store(), tracker.CommentStream, ev.Type, ev.Data)
		if err != nil {
			return nil, err
		}
		c, ok := e.(tracker.CommentAdded)
		if !ok {
			continue
		}
		if _, known := commentSubjects[c.Subject]; !known {
			continue
		}
		action := IssueCommented
		if c.Subject == tracker.SubjectPage {
			action = PageCommented
		}
		out = append(out, Entry{Position: ev.Position, At: ev.Meta.At, Actor: ev.Meta.Actor, Action: action,
			Subject: Subject{Kind: c.Subject, ID: c.SubjectID}, Text: excerpt(c.Body)})
		ids[c.Subject] = append(ids[c.Subject], c.SubjectID)
	}
	named := map[string]map[string]Subject{}
	for kind, list := range ids {
		m, err := commentSubjects[kind](ctx, q, list)
		if err != nil {
			return nil, err
		}
		named[kind] = m
	}
	for i := range out {
		out[i].Subject = named[out[i].Subject.Kind][out[i].Subject.ID]
	}
	return out, nil
}

// issueSubjects names issues that are not deleted, as they are now.
func issueSubjects(ctx context.Context, q sqlite.Querier, ids []string) (map[string]Subject, error) {
	out := map[string]Subject{}
	if len(ids) == 0 {
		return out, nil
	}
	list, _ := json.Marshal(ids)
	rows, err := q.QueryContext(ctx, `SELECT id, project_id, key, summary, type FROM issues
		WHERE id IN (SELECT value FROM json_each(?)) AND deleted_at IS NULL`, string(list))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s Subject
		var project string
		if err := rows.Scan(&s.ID, &project, &s.Label, &s.Summary, &s.Type); err != nil {
			return nil, err
		}
		s.Kind, s.Href, s.Scope = tracker.SubjectIssue, "/"+s.Label, access.ProjectScope(project)
		out[s.ID] = s
	}
	return out, rows.Err()
}

type status struct{ name, category string }

// statusNames are the statuses of the subjects' projects, by project and
// status id.
func statusNames(ctx context.Context, q sqlite.Querier, subjects map[string]Subject) (map[string]status, error) {
	var projects []string
	for _, s := range subjects {
		projects = append(projects, s.Scope.ID)
	}
	out := map[string]status{}
	if len(projects) == 0 {
		return out, nil
	}
	list, _ := json.Marshal(projects)
	rows, err := q.QueryContext(ctx, `SELECT project_id, id, name, category FROM statuses WHERE project_id IN (SELECT value FROM json_each(?))`, string(list))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p, id string
		var st status
		if err := rows.Scan(&p, &id, &st.name, &st.category); err != nil {
			return nil, err
		}
		out[p+"\x00"+id] = st
	}
	return out, rows.Err()
}

// maxText is how many characters of a comment an entry shows.
const maxText = 120

// excerpt is the start of a document's text.
func excerpt(doc json.RawMessage) string {
	var b strings.Builder
	writeText(&b, doc)
	s := strings.Join(strings.Fields(b.String()), " ")
	if utf8.RuneCountInString(s) > maxText {
		r := []rune(s)[:maxText-1]
		s = strings.TrimSpace(string(r)) + "…"
	}
	return s
}

// writeText writes a node's text as it stands, spaces beside chips and
// mentions included; excerpt folds the spaces once, for the whole text.
func writeText(b *strings.Builder, doc json.RawMessage) {
	var n struct {
		Type    string            `json:"type"`
		Text    string            `json:"text"`
		Content []json.RawMessage `json:"content"`
		Attrs   struct {
			Label string `json:"label"`
			Key   string `json:"key"`
		} `json:"attrs"`
	}
	if json.Unmarshal(doc, &n) != nil {
		return
	}
	switch n.Type {
	case "text":
		b.WriteString(n.Text)
	case "mention":
		if n.Attrs.Label != "" {
			b.WriteString("@" + n.Attrs.Label)
		}
	case "issue_chip":
		b.WriteString(n.Attrs.Key)
	}
	for _, c := range n.Content {
		if b.Len() > 4*maxText {
			break
		}
		writeText(b, c)
		if n.Type != "paragraph" && n.Type != "heading" && n.Type != "code_block" {
			b.WriteByte(' ') // between blocks
		}
	}
}
