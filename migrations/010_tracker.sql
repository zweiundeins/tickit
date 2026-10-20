-- The tracker (internal/tracker): projects, issues and comments are event
-- streams. The projection tables below are written by the tracker's
-- inline projectors only, and rebuilt from the events when a projector's
-- version changes: no triggers on them, no foreign keys into them. Times
-- are unix milliseconds; '' stands for none in text columns.

-- tracker_projects: a row per project, its workflow's statuses with the
-- board's work-in-progress limits, its components and its sprints.
CREATE TABLE projects (
    id          TEXT PRIMARY KEY,
    key         TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    lead        TEXT NOT NULL DEFAULT '',
    open        INTEGER NOT NULL,
    archived    INTEGER NOT NULL DEFAULT 0,
    created_by  TEXT NOT NULL,
    created_at  INTEGER NOT NULL
) STRICT;

CREATE INDEX projects_key ON projects (key);

CREATE TABLE statuses (
    project_id TEXT NOT NULL,
    id         TEXT NOT NULL,
    name       TEXT NOT NULL,
    category   TEXT NOT NULL,
    position   INTEGER NOT NULL,
    wip_limit  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (project_id, id)
) STRICT, WITHOUT ROWID;

CREATE TABLE components (
    project_id  TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (project_id, name)
) STRICT, WITHOUT ROWID;

CREATE TABLE sprints (
    id         TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    name       TEXT NOT NULL,
    goal       TEXT NOT NULL DEFAULT '',
    starts     TEXT NOT NULL DEFAULT '',
    ends       TEXT NOT NULL DEFAULT '',
    state      TEXT NOT NULL,
    created_at INTEGER NOT NULL
) STRICT;

CREATE INDEX sprints_project ON sprints (project_id, state);

-- tracker_issues: a row per issue for lists, boards, search and chips,
-- and its labels, links and watchers; labels are created on first use.
CREATE TABLE issues (
    id               TEXT PRIMARY KEY,
    project_id       TEXT NOT NULL,
    project_key      TEXT NOT NULL,
    key              TEXT NOT NULL,
    number           INTEGER NOT NULL,
    old_keys         TEXT NOT NULL DEFAULT '',  -- the keys before moves, space-separated
    type             TEXT NOT NULL,
    summary          TEXT NOT NULL,
    description      TEXT,                      -- the document's JSON
    description_text TEXT NOT NULL DEFAULT '',
    status           TEXT NOT NULL,
    status_category  TEXT NOT NULL,
    priority         TEXT NOT NULL,
    priority_order   INTEGER NOT NULL,
    assignee         TEXT NOT NULL DEFAULT '',
    reporter         TEXT NOT NULL,
    parent_id        TEXT NOT NULL DEFAULT '',
    epic_id          TEXT NOT NULL DEFAULT '',
    labels           TEXT NOT NULL DEFAULT '[]',
    components       TEXT NOT NULL DEFAULT '[]',
    due              TEXT NOT NULL DEFAULT '',
    points           INTEGER,
    estimate         INTEGER,
    remaining        INTEGER,
    rank             TEXT NOT NULL,
    sprint           TEXT NOT NULL DEFAULT '',
    created_at       INTEGER NOT NULL,
    updated_at       INTEGER NOT NULL,
    resolved_at      INTEGER,
    deleted_at       INTEGER,
    deleted_by       TEXT NOT NULL DEFAULT '',
    version          INTEGER NOT NULL,
    search           TEXT NOT NULL DEFAULT ''   -- keys, summary and description in lower case
) STRICT;

CREATE INDEX issues_project_rank ON issues (project_id, rank);
CREATE INDEX issues_project_status ON issues (project_id, status);
CREATE INDEX issues_created ON issues (created_at);
CREATE INDEX issues_assignee ON issues (assignee);
CREATE INDEX issues_parent ON issues (parent_id);
CREATE INDEX issues_epic ON issues (epic_id);
CREATE INDEX issues_sprint ON issues (sprint);

CREATE TABLE issue_labels (
    issue_id   TEXT NOT NULL,
    project_id TEXT NOT NULL,
    label      TEXT NOT NULL,
    PRIMARY KEY (issue_id, label)
) STRICT, WITHOUT ROWID;

CREATE INDEX issue_labels_label ON issue_labels (project_id, label);

CREATE TABLE labels (
    project_id TEXT NOT NULL,
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    created_by TEXT NOT NULL,
    PRIMARY KEY (project_id, name)
) STRICT, WITHOUT ROWID;

CREATE TABLE issue_links (
    issue_id   TEXT NOT NULL,
    other_id   TEXT NOT NULL,
    type       TEXT NOT NULL,
    outward    INTEGER NOT NULL,
    created_at INTEGER NOT NULL,
    created_by TEXT NOT NULL,
    PRIMARY KEY (issue_id, other_id, type)
) STRICT, WITHOUT ROWID;

CREATE TABLE issue_watchers (
    issue_id  TEXT NOT NULL,
    person_id TEXT NOT NULL,
    since     INTEGER NOT NULL,
    PRIMARY KEY (issue_id, person_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX issue_watchers_person ON issue_watchers (person_id);

-- tracker_comments: a row per comment on an issue (and later a page).
CREATE TABLE comments (
    id            TEXT PRIMARY KEY,
    subject       TEXT NOT NULL,  -- 'issue'
    subject_id    TEXT NOT NULL,
    project_id    TEXT NOT NULL DEFAULT '',
    author        TEXT NOT NULL,
    body          TEXT,           -- null once deleted
    search        TEXT NOT NULL DEFAULT '',
    mentions      TEXT NOT NULL DEFAULT '[]',
    created_at    INTEGER NOT NULL,
    edited_at     INTEGER,
    deleted_at    INTEGER,
    deleted_by    TEXT NOT NULL DEFAULT '',
    hidden_at     INTEGER,
    hidden_by     TEXT NOT NULL DEFAULT '',
    hidden_reason TEXT NOT NULL DEFAULT '',
    version       INTEGER NOT NULL
) STRICT;

CREATE INDEX comments_subject ON comments (subject_id, created_at);

-- Written by the commands, never by a projector, so their keys guard
-- rules (NFR-18).

-- Every key an issue ever had (FR-11, FR-12): numbers unique per project
-- and never given again, old keys after a move still resolve.
CREATE TABLE issue_keys (
    key        TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    number     INTEGER NOT NULL,
    issue_id   TEXT NOT NULL,
    UNIQUE (project_id, number)
) STRICT, WITHOUT ROWID;

CREATE INDEX issue_keys_issue ON issue_keys (issue_id);

-- An issue's parent, for the rules on the types of issues with children
-- (FR-19.2) and on deleting them.
CREATE TABLE issue_parents (
    issue_id  TEXT PRIMARY KEY,
    parent_id TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE INDEX issue_parents_parent ON issue_parents (parent_id);

-- When each person last acted in each project: who may be assigned
-- (FR-17.1).
CREATE TABLE tracker_actors (
    project_id TEXT NOT NULL,
    person_id  TEXT NOT NULL,
    at         INTEGER NOT NULL,
    PRIMARY KEY (project_id, person_id)
) STRICT, WITHOUT ROWID;

-- Saved filters (FR-56): everyone sees every one in the demo.
CREATE TABLE saved_filters (
    id         TEXT PRIMARY KEY,
    owner      TEXT NOT NULL,
    name       TEXT NOT NULL,
    query      TEXT NOT NULL,  -- the filter and sort as URL values
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) STRICT;

CREATE TABLE saved_filter_stars (
    filter_id TEXT NOT NULL,
    person_id TEXT NOT NULL,
    PRIMARY KEY (filter_id, person_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX saved_filter_stars_person ON saved_filter_stars (person_id);
