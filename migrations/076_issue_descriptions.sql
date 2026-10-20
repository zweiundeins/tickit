-- Co-edited issue descriptions (FR-15, docs/editor.md): the head of each
-- description's document stream ("issuedoc"), as page_doc is a page's,
-- written by the projector tracker_descs; the tabs editing one, as
-- page_editors; and on issues the description stream's version that the
-- issue's last issue.description_changed holds, so that the jobs find the
-- descriptions with changes since their last version. An issue's stream
-- and its description's start together, both at version 1.
CREATE TABLE issue_doc (
    issue_id   TEXT PRIMARY KEY,
    epoch      TEXT NOT NULL,
    version    INTEGER NOT NULL,
    ref        TEXT NOT NULL DEFAULT '',
    checkpoint INTEGER NOT NULL,             -- the version doc holds
    doc        TEXT NOT NULL,
    bytes      INTEGER NOT NULL DEFAULT 0,   -- of the head document, as stored
    blocks     INTEGER NOT NULL DEFAULT 0,
    last_at    INTEGER NOT NULL              -- the last commit's time
) STRICT;

CREATE TABLE issue_editors (
    sid       TEXT NOT NULL,
    tab_id    TEXT NOT NULL,
    issue_id  TEXT NOT NULL,
    person_id TEXT NOT NULL,
    client    TEXT NOT NULL,     -- 12 hex digits the editor's page chose
    epoch     TEXT NOT NULL,
    ack       INTEGER NOT NULL,
    v         INTEGER NOT NULL DEFAULT 0,
    anchor    INTEGER NOT NULL DEFAULT 0,
    head      INTEGER NOT NULL DEFAULT 0,
    seq       INTEGER NOT NULL DEFAULT 0,
    refusal   TEXT NOT NULL DEFAULT '',  -- {"ref", "message"} as JSON
    moved_at  INTEGER NOT NULL,          -- the selection's last change
    seen_at   INTEGER NOT NULL,
    PRIMARY KEY (sid, tab_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX issue_editors_issue ON issue_editors (issue_id);
CREATE INDEX issue_editors_tab ON issue_editors (tab_id);

ALTER TABLE issues ADD COLUMN desc_upto INTEGER NOT NULL DEFAULT 1;
