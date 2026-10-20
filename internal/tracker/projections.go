package tracker

import (
	"database/sql"
	"encoding/json"
	"slices"
	"strings"

	"github.com/zweiundeins/lith/events"
	"github.com/zweiundeins/lith/writer"

	"github.com/zweiundeins/tickit/internal/people"
)

// The projectors' versions: bump one when its handlers change, and the
// runner rebuilds its tables from the events.
const (
	projectsVersion = 1
	issuesVersion   = 1
	commentsVersion = 1
)

// projectsProjector keeps projects, statuses, components and sprints.
var projectsProjector = events.NewProjector(ProjectStream, "tracker_projects", projectsVersion,
	events.Handle(ProjectCreatedType, func(tx *writer.Tx, r events.Record[string, ProjectCreated]) error {
		e := r.Data
		if _, err := tx.Exec(`INSERT INTO projects (id, key, name, description, lead, open, archived, created_by, created_at)
			VALUES (?, ?, ?, ?, ?, ?, 0, ?, ?)`, r.ID, e.Key, e.Name, e.Description, e.Lead, e.Open, r.Actor, millis(r.At)); err != nil {
			return err
		}
		for i, s := range e.Workflow.Statuses {
			if _, err := tx.Exec(`INSERT INTO statuses (project_id, id, name, category, position) VALUES (?, ?, ?, ?, ?)`,
				r.ID, s.ID, s.Name, string(s.Category), i+1); err != nil {
				return err
			}
		}
		touchProject(tx, r.ID)
		return nil
	}),
	events.Handle(ProjectEditedType, func(tx *writer.Tx, r events.Record[string, ProjectEdited]) error {
		e := r.Data
		_, err := tx.Exec(`UPDATE projects SET name = coalesce(?, name), description = coalesce(?, description), lead = coalesce(?, lead) WHERE id = ?`,
			e.Name, e.Description, e.Lead, r.ID)
		touchProject(tx, r.ID)
		tx.Touch(IssuesTopic)
		return err
	}),
	events.Handle(ProjectArchivedType, func(tx *writer.Tx, r events.Record[string, ProjectArchived]) error {
		_, err := tx.Exec(`UPDATE projects SET archived = 1 WHERE id = ?`, r.ID)
		touchProject(tx, r.ID)
		tx.Touch(IssuesTopic)
		return err
	}),
	events.Handle(ProjectUnarchivedType, func(tx *writer.Tx, r events.Record[string, ProjectUnarchived]) error {
		_, err := tx.Exec(`UPDATE projects SET archived = 0 WHERE id = ?`, r.ID)
		touchProject(tx, r.ID)
		tx.Touch(IssuesTopic)
		return err
	}),
	events.Handle(ComponentDefinedType, func(tx *writer.Tx, r events.Record[string, ComponentDefined]) error {
		_, err := tx.Exec(`INSERT INTO components (project_id, name, description) VALUES (?, ?, ?)
			ON CONFLICT (project_id, name) DO UPDATE SET description = excluded.description`, r.ID, r.Data.Name, r.Data.Description)
		touchProject(tx, r.ID)
		return err
	}),
	events.Handle(ComponentRemovedType, func(tx *writer.Tx, r events.Record[string, ComponentRemoved]) error {
		_, err := tx.Exec(`DELETE FROM components WHERE project_id = ? AND name = ?`, r.ID, r.Data.Name)
		touchProject(tx, r.ID)
		return err
	}),
	events.Ignore(NumberAssignedType),
	events.Handle(WIPLimitSetType, func(tx *writer.Tx, r events.Record[string, WIPLimitSet]) error {
		_, err := tx.Exec(`UPDATE statuses SET wip_limit = ? WHERE project_id = ? AND id = ?`, r.Data.Limit, r.ID, r.Data.Status)
		touchProject(tx, r.ID)
		return err
	}),
	events.Handle(SprintCreatedType, func(tx *writer.Tx, r events.Record[string, SprintCreated]) error {
		e := r.Data
		_, err := tx.Exec(`INSERT INTO sprints (id, project_id, name, goal, starts, ends, state, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			e.Sprint, r.ID, e.Name, e.Goal, e.Start, e.End, SprintFuture, millis(r.At))
		touchProject(tx, r.ID)
		return err
	}),
	events.Handle(SprintStartedType, func(tx *writer.Tx, r events.Record[string, SprintStarted]) error {
		e := r.Data
		_, err := tx.Exec(`UPDATE sprints SET state = ?, starts = coalesce(nullif(?, ''), starts), ends = coalesce(nullif(?, ''), ends) WHERE id = ?`,
			SprintActive, e.Start, e.End, e.Sprint)
		touchProject(tx, r.ID)
		tx.Touch(IssuesTopic)
		return err
	}),
	events.Handle(SprintCompletedType, func(tx *writer.Tx, r events.Record[string, SprintCompleted]) error {
		_, err := tx.Exec(`UPDATE sprints SET state = ? WHERE id = ?`, SprintClosed, r.Data.Sprint)
		touchProject(tx, r.ID)
		tx.Touch(IssuesTopic)
		return err
	}),
).Tables("projects", "statuses", "components", "sprints")

func touchProject(tx *writer.Tx, id string) {
	tx.Touch(ProjectTopic(id), ProjectsTopic, BoardTopic(id))
}

// issuesProjector keeps issues, issue_labels, labels, issue_links and
// issue_watchers.
var issuesProjector = events.NewProjector(IssueStream, "tracker_issues", issuesVersion,
	events.Handle(IssueCreatedType, func(tx *writer.Tx, r events.Record[string, IssueCreated]) error {
		e := r.Data
		info, _ := readDoc(e.Description)
		labels, _ := json.Marshal(nonNil(slices.Sorted(slices.Values(e.Labels))))
		comps, _ := json.Marshal(nonNil(e.Components))
		var resolved any
		if e.Category == Done {
			resolved = millis(r.At)
		}
		projectKey, _, _ := strings.Cut(e.Key, "-")
		if _, err := tx.Exec(`INSERT INTO issues (id, project_id, project_key, key, number, type, summary, description, description_text,
			status, status_category, priority, priority_order, assignee, reporter, parent_id, labels, components, due, points,
			estimate, remaining, rank, sprint, created_at, updated_at, resolved_at, version)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			r.ID, e.Project, projectKey, e.Key, e.Number, string(e.Type), e.Summary, nullDoc(e.Description), info.Text,
			e.Status, string(e.Category), string(e.Priority), e.Priority.Order(), e.Assignee, e.Reporter, e.Parent,
			string(labels), string(comps), e.Due, e.Points, e.Estimate, e.Remaining, e.Rank, e.Sprint,
			millis(r.At), millis(r.At), resolved, r.Version); err != nil {
			return err
		}
		if err := addLabels(tx, r.ID, e.Project, e.Labels, r.Actor, millis(r.At)); err != nil {
			return err
		}
		if err := addWatchers(tx, r.ID, e.Watchers, millis(r.At)); err != nil {
			return err
		}
		if err := refreshEpics(tx, r.ID); err != nil {
			return err
		}
		if e.Assignee != "" {
			tx.Touch(people.PersonTopic(e.Assignee))
		}
		return finish(tx, r.ID, r.Version, millis(r.At), true)
	}),
	events.Handle(SummaryChangedType, func(tx *writer.Tx, r events.Record[string, SummaryChanged]) error {
		return set(tx, r.ID, r.Version, millis(r.At), `summary = ?`, r.Data.To)
	}),
	events.Handle(DescriptionChangedType, func(tx *writer.Tx, r events.Record[string, DescriptionChanged]) error {
		info, _ := readDoc(r.Data.To)
		return set(tx, r.ID, r.Version, millis(r.At), `description = ?, description_text = ?`, nullDoc(r.Data.To), info.Text)
	}),
	events.Handle(TypeChangedType, func(tx *writer.Tx, r events.Record[string, TypeChanged]) error {
		if err := set(tx, r.ID, r.Version, millis(r.At), `type = ?`, string(r.Data.To)); err != nil {
			return err
		}
		return refreshEpics(tx, r.ID)
	}),
	events.Handle(StatusChangedType, func(tx *writer.Tx, r events.Record[string, StatusChanged]) error {
		return setStatus(tx, r.ID, r.Version, millis(r.At), r.Data.To, r.Data.Category, "")
	}),
	events.Handle(MovedType, func(tx *writer.Tx, r events.Record[string, Moved]) error {
		return setStatus(tx, r.ID, r.Version, millis(r.At), r.Data.Status, r.Data.Category, r.Data.Rank)
	}),
	events.Handle(PriorityChangedType, func(tx *writer.Tx, r events.Record[string, PriorityChanged]) error {
		return set(tx, r.ID, r.Version, millis(r.At), `priority = ?, priority_order = ?`, string(r.Data.To), r.Data.To.Order())
	}),
	events.Handle(AssigneeChangedType, func(tx *writer.Tx, r events.Record[string, AssigneeChanged]) error {
		for _, p := range []string{r.Data.From, r.Data.To} {
			if p != "" {
				tx.Touch(people.PersonTopic(p))
			}
		}
		return set(tx, r.ID, r.Version, millis(r.At), `assignee = ?`, r.Data.To)
	}),
	events.Handle(ParentChangedType, func(tx *writer.Tx, r events.Record[string, ParentChanged]) error {
		if r.Data.From != "" {
			tx.Touch(IssueTopic(r.Data.From))
		}
		if _, err := tx.Exec(`UPDATE issues SET parent_id = ? WHERE id = ?`, r.Data.To, r.ID); err != nil {
			return err
		}
		if err := refreshEpics(tx, r.ID); err != nil {
			return err
		}
		return finish(tx, r.ID, r.Version, millis(r.At), true)
	}),
	events.Handle(DueChangedType, func(tx *writer.Tx, r events.Record[string, DueChanged]) error {
		return set(tx, r.ID, r.Version, millis(r.At), `due = ?`, r.Data.To)
	}),
	events.Handle(PointsChangedType, func(tx *writer.Tx, r events.Record[string, PointsChanged]) error {
		return set(tx, r.ID, r.Version, millis(r.At), `points = ?`, r.Data.To)
	}),
	events.Handle(EstimateChangedType, func(tx *writer.Tx, r events.Record[string, EstimateChanged]) error {
		return set(tx, r.ID, r.Version, millis(r.At), `estimate = ?`, r.Data.To)
	}),
	events.Handle(RemainingChangedType, func(tx *writer.Tx, r events.Record[string, RemainingChanged]) error {
		return set(tx, r.ID, r.Version, millis(r.At), `remaining = ?`, r.Data.To)
	}),
	events.Handle(SprintChangedType, func(tx *writer.Tx, r events.Record[string, SprintChanged]) error {
		return set(tx, r.ID, r.Version, millis(r.At), `sprint = ?`, r.Data.To)
	}),
	events.Handle(LabelsChangedType, func(tx *writer.Tx, r events.Record[string, LabelsChanged]) error {
		var project string
		if err := tx.QueryRow(`SELECT project_id FROM issues WHERE id = ?`, r.ID).Scan(&project); err != nil {
			return err
		}
		for _, l := range r.Data.Removed {
			if _, err := tx.Exec(`DELETE FROM issue_labels WHERE issue_id = ? AND label = ?`, r.ID, l); err != nil {
				return err
			}
		}
		if err := addLabels(tx, r.ID, project, r.Data.Added, r.Actor, millis(r.At)); err != nil {
			return err
		}
		return set(tx, r.ID, r.Version, millis(r.At),
			`labels = (SELECT coalesce(json_group_array(label), '[]') FROM (SELECT label FROM issue_labels WHERE issue_id = issues.id ORDER BY label))`)
	}),
	events.Handle(ComponentsChangedType, func(tx *writer.Tx, r events.Record[string, ComponentsChanged]) error {
		var raw string
		if err := tx.QueryRow(`SELECT components FROM issues WHERE id = ?`, r.ID).Scan(&raw); err != nil {
			return err
		}
		var comps []string
		if err := json.Unmarshal([]byte(raw), &comps); err != nil {
			return err
		}
		b, _ := json.Marshal(nonNil(applySet(comps, r.Data.Added, r.Data.Removed)))
		return set(tx, r.ID, r.Version, millis(r.At), `components = ?`, string(b))
	}),
	events.Handle(WatchersAddedType, func(tx *writer.Tx, r events.Record[string, WatchersAdded]) error {
		if err := addWatchers(tx, r.ID, r.Data.People, millis(r.At)); err != nil {
			return err
		}
		return finish(tx, r.ID, r.Version, millis(r.At), false)
	}),
	events.Handle(WatcherRemovedType, func(tx *writer.Tx, r events.Record[string, WatcherRemoved]) error {
		if _, err := tx.Exec(`DELETE FROM issue_watchers WHERE issue_id = ? AND person_id = ?`, r.ID, r.Data.Person); err != nil {
			return err
		}
		tx.Touch(people.PersonTopic(r.Data.Person))
		return finish(tx, r.ID, r.Version, millis(r.At), false)
	}),
	events.Handle(LinkAddedType, func(tx *writer.Tx, r events.Record[string, LinkAdded]) error {
		if _, err := tx.Exec(`INSERT INTO issue_links (issue_id, other_id, type, outward, created_at, created_by) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT DO NOTHING`, r.ID, r.Data.Issue, string(r.Data.Link), r.Data.Outward, millis(r.At), r.Actor); err != nil {
			return err
		}
		return finish(tx, r.ID, r.Version, millis(r.At), true)
	}),
	events.Handle(LinkRemovedType, func(tx *writer.Tx, r events.Record[string, LinkRemoved]) error {
		if _, err := tx.Exec(`DELETE FROM issue_links WHERE issue_id = ? AND other_id = ? AND type = ?`, r.ID, r.Data.Issue, string(r.Data.Link)); err != nil {
			return err
		}
		tx.Touch(IssueTopic(r.Data.Issue))
		return finish(tx, r.ID, r.Version, millis(r.At), true)
	}),
	events.Handle(IssueDeletedType, func(tx *writer.Tx, r events.Record[string, IssueDeleted]) error {
		return set(tx, r.ID, r.Version, millis(r.At), `deleted_at = ?, deleted_by = ?`, millis(r.At), r.Actor)
	}),
	events.Handle(IssueHiddenType, projectHidden),
	events.Handle(IssueUnhiddenType, projectUnhidden),
	events.Handle(ProjectChangedType, func(tx *writer.Tx, r events.Record[string, ProjectChanged]) error {
		e := r.Data
		tx.Touch(BoardTopic(e.FromProject))
		projectKey, _, _ := strings.Cut(e.Key, "-")
		if _, err := tx.Exec(`UPDATE issues SET project_id = ?, project_key = ?, key = ?, number = ?,
			old_keys = trim(old_keys || ' ' || ?), rank = ? WHERE id = ?`, e.Project, projectKey, e.Key, e.Number, e.FromKey, e.Rank, r.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE issue_labels SET project_id = ? WHERE issue_id = ?`, e.Project, r.ID); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO labels (project_id, name, created_at, created_by)
			SELECT ?, label, ?, ? FROM issue_labels WHERE issue_id = ? ON CONFLICT DO NOTHING`, e.Project, millis(r.At), r.Actor, r.ID); err != nil {
			return err
		}
		return setStatus(tx, r.ID, r.Version, millis(r.At), e.Status, e.Category, e.Rank)
	}),
).Tables("issues", "issue_labels", "labels", "issue_links", "issue_watchers")

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

// nullDoc stores the empty document as NULL.
func nullDoc(doc json.RawMessage) any {
	if len(doc) == 0 {
		return nil
	}
	return string(doc)
}

// set updates columns of an issue's row, then finishes it.
func set(tx *writer.Tx, id string, version, at int64, columns string, args ...any) error {
	if _, err := tx.Exec(`UPDATE issues SET `+columns+` WHERE id = ?`, append(args, id)...); err != nil {
		return err
	}
	return finish(tx, id, version, at, true)
}

// setStatus changes an issue's status and, unless empty, its rank; it
// sets the resolved time when the issue enters the Done category and
// clears it when it leaves (FR-38).
func setStatus(tx *writer.Tx, id string, version, at int64, status string, c Category, rank string) error {
	return set(tx, id, version, at, `status = ?, status_category = ?,
		resolved_at = CASE WHEN ? != 'done' THEN NULL WHEN status_category = 'done' THEN resolved_at ELSE ? END,
		rank = coalesce(nullif(?, ''), rank)`, status, string(c), string(c), at, rank)
}

// finish records an issue's version and, for a change of its content,
// its updated time, refreshes its search text, and touches the topics of
// every view that shows it: its page, its parent's and epic's, the
// issues linked to it, its project's board, the lists.
func finish(tx *writer.Tx, id string, version, at int64, updated bool) error {
	q := `UPDATE issues SET version = ? WHERE id = ?`
	args := []any{version, id}
	if updated {
		q = `UPDATE issues SET version = ?, updated_at = ? WHERE id = ?`
		args = []any{version, at, id}
	}
	if _, err := tx.Exec(q, args...); err != nil {
		return err
	}
	var key, oldKeys, summary, text, project, parent, epic string
	err := tx.QueryRow(`SELECT key, old_keys, summary, description_text, project_id, parent_id, epic_id FROM issues WHERE id = ?`, id).
		Scan(&key, &oldKeys, &summary, &text, &project, &parent, &epic)
	if err != nil {
		return err
	}
	search := strings.ToLower(strings.Join([]string{key, oldKeys, summary, text}, " "))
	if _, err := tx.Exec(`UPDATE issues SET search = ? WHERE id = ? AND search != ?`, search, id, search); err != nil {
		return err
	}
	tx.Touch(IssueTopic(id), IssuesTopic, BoardTopic(project))
	for _, other := range []string{parent, epic} {
		if other != "" {
			tx.Touch(IssueTopic(other))
		}
	}
	rows, err := tx.Query(`SELECT other_id FROM issue_links WHERE issue_id = ?`, id)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var other string
		if err := rows.Scan(&other); err != nil {
			return err
		}
		tx.Touch(IssueTopic(other))
	}
	return rows.Err()
}

// refreshEpics sets the epic of an issue, its children and theirs: an
// issue's epic is its parent when that is an epic, else its parent's
// epic (FR-20).
func refreshEpics(tx *writer.Tx, id string) error {
	const epicOf = `epic_id = CASE WHEN parent_id = '' THEN '' ELSE coalesce((SELECT CASE WHEN p.type = 'epic' THEN p.id ELSE p.epic_id END
		FROM issues p WHERE p.id = issues.parent_id), '') END`
	for _, q := range []string{
		`UPDATE issues SET ` + epicOf + ` WHERE id = ?`,
		`UPDATE issues SET ` + epicOf + ` WHERE parent_id = ?`,
		`UPDATE issues SET ` + epicOf + ` WHERE parent_id IN (SELECT id FROM issues WHERE parent_id = ?)`,
	} {
		if _, err := tx.Exec(q, id); err != nil {
			return err
		}
	}
	return nil
}

func addLabels(tx *writer.Tx, id, project string, labels []string, by string, at int64) error {
	for _, l := range labels {
		if _, err := tx.Exec(`INSERT INTO issue_labels (issue_id, project_id, label) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, id, project, l); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO labels (project_id, name, created_at, created_by) VALUES (?, ?, ?, ?) ON CONFLICT DO NOTHING`, project, l, at, by); err != nil {
			return err
		}
	}
	return nil
}

func addWatchers(tx *writer.Tx, id string, ids []string, at int64) error {
	for _, p := range ids {
		if _, err := tx.Exec(`INSERT INTO issue_watchers (issue_id, person_id, since) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, id, p, at); err != nil {
			return err
		}
		tx.Touch(people.PersonTopic(p))
	}
	return nil
}

// commentsProjector keeps comments.
var commentsProjector = events.NewProjector(CommentStream, "tracker_comments", commentsVersion,
	events.Handle(CommentAddedType, func(tx *writer.Tx, r events.Record[string, CommentAdded]) error {
		e := r.Data
		info, _ := readDoc(e.Body)
		mentions, _ := json.Marshal(nonNil(e.Mentions))
		_, err := tx.Exec(`INSERT INTO comments (id, subject, subject_id, project_id, author, body, search, mentions, created_at, version)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, r.ID, e.Subject, e.SubjectID, e.Project, r.Actor, string(e.Body),
			strings.ToLower(info.Text), string(mentions), millis(r.At), r.Version)
		touchComment(tx, e.Subject, e.SubjectID)
		return err
	}),
	events.Handle(CommentEditedType, func(tx *writer.Tx, r events.Record[string, CommentEdited]) error {
		info, _ := readDoc(r.Data.Body)
		mentions, _ := json.Marshal(nonNil(info.Mentions))
		return updateComment(tx, r.ID, `body = ?, search = ?, mentions = ?, edited_at = ?, version = ?`,
			string(r.Data.Body), strings.ToLower(info.Text), string(mentions), millis(r.At), r.Version)
	}),
	events.Handle(CommentDeletedType, func(tx *writer.Tx, r events.Record[string, CommentDeleted]) error {
		return updateComment(tx, r.ID, `body = NULL, search = '', mentions = '[]', deleted_at = ?, deleted_by = ?, version = ?`,
			millis(r.At), r.Actor, r.Version)
	}),
	events.Handle(CommentHiddenType, func(tx *writer.Tx, r events.Record[string, CommentHidden]) error {
		return updateComment(tx, r.ID, `hidden_at = ?, hidden_by = ?, hidden_reason = ?, version = ?`, millis(r.At), r.Actor, r.Data.Reason, r.Version)
	}),
	events.Handle(CommentUnhiddenType, func(tx *writer.Tx, r events.Record[string, CommentUnhidden]) error {
		return updateComment(tx, r.ID, `hidden_at = NULL, hidden_by = '', hidden_reason = '', version = ?`, r.Version)
	}),
).Tables("comments")

func updateComment(tx *writer.Tx, id, columns string, args ...any) error {
	if _, err := tx.Exec(`UPDATE comments SET `+columns+` WHERE id = ?`, append(args, id)...); err != nil {
		return err
	}
	var subject, subjectID string
	err := tx.QueryRow(`SELECT subject, subject_id FROM comments WHERE id = ?`, id).Scan(&subject, &subjectID)
	if err == sql.ErrNoRows {
		return nil
	}
	touchComment(tx, subject, subjectID)
	return err
}

func touchComment(tx *writer.Tx, subject, id string) {
	if subject == SubjectIssue {
		tx.Touch(IssueTopic(id), IssuesTopic)
	} else if s, ok := subjects[subject]; ok {
		tx.Touch(s.Topics(id)...)
	}
}
