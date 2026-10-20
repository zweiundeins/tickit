-- The wiki (internal/wiki): spaces, pages, their documents, versions and
-- comments are event streams ("space", "page", "pagedoc",
-- "page_comment"). The projection tables are written by the wiki's inline
-- projectors only and rebuilt from the events when a projector's version
-- changes: no triggers, no foreign keys into them. Times are unix
-- milliseconds; '' stands for none in text columns.

-- wiki_spaces: a row per space.
CREATE TABLE spaces (
    id          TEXT PRIMARY KEY,
    key         TEXT NOT NULL,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    home_id     TEXT NOT NULL,
    open        INTEGER NOT NULL,
    archived    INTEGER NOT NULL DEFAULT 0,
    created_by  TEXT NOT NULL,
    created_at  INTEGER NOT NULL
) STRICT;

CREATE INDEX spaces_key ON spaces (key);

-- wiki_pages: a row per page, its place in the tree (parent and a
-- fractional rank among its siblings), its labels and watchers.
CREATE TABLE pages (
    id           TEXT PRIMARY KEY,
    space_id     TEXT NOT NULL,
    parent_id    TEXT NOT NULL DEFAULT '',
    rank         TEXT NOT NULL,
    title        TEXT NOT NULL,
    home         INTEGER NOT NULL DEFAULT 0,
    state        TEXT NOT NULL,              -- 'current', 'trashed', 'deleted'
    labels       TEXT NOT NULL DEFAULT '[]',
    created_by   TEXT NOT NULL,
    created_at   INTEGER NOT NULL,
    edited_by    TEXT NOT NULL,              -- who changed it last: title, place or a version
    edited_at    INTEGER NOT NULL,
    trashed_by   TEXT NOT NULL DEFAULT '',
    trashed_at   INTEGER,
    trash_parent TEXT NOT NULL DEFAULT '',   -- where a restore puts it back
    trash_rank   TEXT NOT NULL DEFAULT '',
    version      INTEGER NOT NULL,           -- of the page's stream
    search       TEXT NOT NULL DEFAULT ''    -- the title in lower case
) STRICT;

CREATE INDEX pages_tree ON pages (space_id, parent_id, rank);

CREATE TABLE page_labels (
    page_id  TEXT NOT NULL,
    space_id TEXT NOT NULL,
    label    TEXT NOT NULL,
    PRIMARY KEY (page_id, label)
) STRICT, WITHOUT ROWID;

CREATE INDEX page_labels_label ON page_labels (label, page_id);

CREATE TABLE page_watchers (
    page_id   TEXT NOT NULL,
    person_id TEXT NOT NULL,
    since     INTEGER NOT NULL,
    PRIMARY KEY (page_id, person_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX page_watchers_person ON page_watchers (person_id);

-- wiki_versions: every version of every page (FR-87), from the page's
-- page.version_cut events; upto is the pagedoc version it holds.
CREATE TABLE page_versions (
    page_id    TEXT NOT NULL,
    number     INTEGER NOT NULL,
    upto       INTEGER NOT NULL,
    title      TEXT NOT NULL,
    doc        TEXT NOT NULL,
    editors    TEXT NOT NULL DEFAULT '[]',
    message    TEXT NOT NULL DEFAULT '',
    reason     TEXT NOT NULL,
    created_by TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (page_id, number)
) STRICT, WITHOUT ROWID;

-- wiki_docs: the head document of each page (docs/editor.md): its epoch,
-- version and last ref, the document as of a checkpoint every few
-- commits, the time of the last commit; and what the head document refers
-- to: issue keys (live chips, FR-100, FR-101) and pages (FR-103.2).
CREATE TABLE page_doc (
    page_id    TEXT PRIMARY KEY,
    epoch      TEXT NOT NULL,
    version    INTEGER NOT NULL,
    ref        TEXT NOT NULL DEFAULT '',
    checkpoint INTEGER NOT NULL,             -- the version doc holds
    doc        TEXT NOT NULL,
    bytes      INTEGER NOT NULL DEFAULT 0,   -- of the head document, as stored
    blocks     INTEGER NOT NULL DEFAULT 0,
    last_at    INTEGER NOT NULL              -- the last commit's time
) STRICT;

CREATE TABLE page_chips (
    page_id TEXT NOT NULL,
    key     TEXT NOT NULL,
    PRIMARY KEY (page_id, key)
) STRICT, WITHOUT ROWID;

CREATE INDEX page_chips_key ON page_chips (key, page_id);

CREATE TABLE page_links (
    page_id   TEXT NOT NULL,
    target_id TEXT NOT NULL,
    PRIMARY KEY (page_id, target_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX page_links_target ON page_links (target_id, page_id);

-- wiki_threads: which page comment replies to which (FR-90): page
-- comments live in the tracker's comment stream and comments table, with
-- subject 'page'; their one level of replies is the wiki's.
CREATE TABLE page_comment_threads (
    comment_id TEXT PRIMARY KEY,
    page_id    TEXT NOT NULL,
    parent_id  TEXT NOT NULL DEFAULT ''
) STRICT, WITHOUT ROWID;

CREATE INDEX page_comment_threads_page ON page_comment_threads (page_id);

-- Written by the commands, never by a projector, so their keys guard
-- rules (NFR-18).

-- Page titles, unique per space without regard to case (FR-81.2,
-- FR-86.2, FR-79.3). A trashed page keeps its title until it is deleted.
CREATE TABLE page_titles (
    space_id  TEXT NOT NULL,
    title_key TEXT NOT NULL,     -- the title in lower case
    page_id   TEXT NOT NULL UNIQUE,
    PRIMARY KEY (space_id, title_key)
) STRICT, WITHOUT ROWID;

-- A page's parent, for the rule that a page never moves under itself or
-- a descendant (FR-79.2), for moving children with their page and up
-- when it goes to the trash.
CREATE TABLE page_parents (
    page_id   TEXT PRIMARY KEY,
    space_id  TEXT NOT NULL,
    parent_id TEXT NOT NULL      -- '' at the top of the space, or in the trash
) STRICT, WITHOUT ROWID;

CREATE INDEX page_parents_parent ON page_parents (parent_id);

-- The editing tabs (docs/editor.md): which tab edits which page, the
-- version it acknowledged, its selection at version v and the seq of its
-- last presence post, the last refusal of one of its commits. A row is
-- written by OpenEditor and Presence, and goes with CloseEditor or the
-- sweep.
CREATE TABLE page_editors (
    sid       TEXT NOT NULL,
    tab_id    TEXT NOT NULL,
    page_id   TEXT NOT NULL,
    person_id TEXT NOT NULL,
    client    TEXT NOT NULL,     -- the first 12 hex digits of SHA-256 of the tab id
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

CREATE INDEX page_editors_page ON page_editors (page_id);
CREATE INDEX page_editors_tab ON page_editors (tab_id);
