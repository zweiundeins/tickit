-- The demo (internal/demo): what outlives the nightly reset (its epoch,
-- the log of resets, moderation and blocks) and what the reset empties
-- (the caps of DM-04.3). Times are unix seconds. Written by commands only.

-- One row: the epoch, which every reset bumps (MP-13.2). A page's editor
-- names the epoch it opened in, and the wiki refuses an edit of an older
-- one: the page it was made for is gone, whatever its id.
CREATE TABLE demo_state (
    id    INTEGER PRIMARY KEY CHECK (id = 1),
    epoch INTEGER NOT NULL
) STRICT;

INSERT INTO demo_state (id, epoch) VALUES (1, 1);

-- Every reset, nightly or "Reset now" (DM-03), with how it went.
CREATE TABLE demo_resets (
    id          INTEGER PRIMARY KEY,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    outcome     TEXT NOT NULL DEFAULT 'running' CHECK (outcome IN ('running', 'ok', 'failed')),
    by          TEXT NOT NULL DEFAULT '',   -- the site admin's person id, '' for the schedule
    epoch       INTEGER NOT NULL,
    detail      TEXT NOT NULL DEFAULT ''    -- what the seed wrote, or why it failed
) STRICT;

-- What guests created since the reset, against the demo-wide caps of
-- DM-04.3: 'issue', 'page', 'image'. The reset empties it.
CREATE TABLE demo_caps (
    kind  TEXT PRIMARY KEY,
    count INTEGER NOT NULL
) STRICT, WITHOUT ROWID;

-- Every moderation action (DM-07.4, FR-156): who, what, on which target,
-- why, when. It survives the reset; the label is the target as it was
-- shown (an issue key, the start of a comment), since the target may be
-- gone.
CREATE TABLE moderation_log (
    id           INTEGER PRIMARY KEY,
    at           INTEGER NOT NULL,
    moderator    TEXT NOT NULL,
    action       TEXT NOT NULL CHECK (action IN ('hide', 'restore', 'delete', 'block', 'unblock')),
    target_kind  TEXT NOT NULL,
    target_id    TEXT NOT NULL,
    target_label TEXT NOT NULL DEFAULT '',
    reason       TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE INDEX moderation_log_at ON moderation_log (at);

-- Guests whose changes are paused (DM-07.2) until a time. Their client
-- address is blocked in memory only (NFR-25.2).
CREATE TABLE blocks (
    person_id TEXT PRIMARY KEY,
    until     INTEGER NOT NULL,
    by        TEXT NOT NULL,
    reason    TEXT NOT NULL DEFAULT '',
    at        INTEGER NOT NULL
) STRICT, WITHOUT ROWID;
