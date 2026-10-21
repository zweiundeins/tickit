-- Inline comments on pages (FR-91, internal/wiki/inline.go). An inline
-- comment is a page comment (the comment stream, its thread in
-- page_comment_threads) with a stream of its own, "inline", keyed by the
-- comment's id: the range of text it was made on and its resolution.

-- wiki_inline: a row per inline comment, with the range at the document's
-- version v it was made at, the text it was on, and its resolution.
CREATE TABLE page_inline (
    comment_id  TEXT PRIMARY KEY,
    page_id     TEXT NOT NULL,
    quote       TEXT NOT NULL,
    v           INTEGER NOT NULL,
    from_pos    INTEGER NOT NULL,
    to_pos      INTEGER NOT NULL,
    created_at  INTEGER NOT NULL,
    resolved_at INTEGER,
    resolved_by TEXT NOT NULL DEFAULT ''
) STRICT, WITHOUT ROWID;

CREATE INDEX page_inline_page ON page_inline (page_id);

-- Written by the commands, never by a projector: where each inline
-- comment's text is in the page's head document. Every commit maps the
-- page's rows through its steps in its own transaction, so they are at
-- page_doc's version; a row is detached once its text is gone. The fold
-- drops the commits a rebuild would map through, so this is no
-- projection.
CREATE TABLE page_anchors (
    comment_id TEXT PRIMARY KEY,
    page_id    TEXT NOT NULL,
    from_pos   INTEGER NOT NULL,
    to_pos     INTEGER NOT NULL,
    detached   INTEGER NOT NULL DEFAULT 0
) STRICT, WITHOUT ROWID;

CREATE INDEX page_anchors_page ON page_anchors (page_id, detached);
