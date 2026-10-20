-- Notifications (internal/notify): one row per recipient. Written only by
-- the commands that notify's catch-up projectors send, with writer.Once
-- keyed by the event's position, and by the recipient's own actions (mark
-- read, dismiss); never by an inline projector, so a rebuild sends no
-- notification again. Times are unix milliseconds of the event.
CREATE TABLE notifications (
    id            INTEGER PRIMARY KEY,
    recipient     TEXT NOT NULL,
    kind          TEXT NOT NULL CHECK (kind IN ('assigned', 'mentioned', 'commented', 'status_changed', 'page_commented')),
    subject_kind  TEXT NOT NULL CHECK (subject_kind IN ('issue', 'page')),
    subject_id    TEXT NOT NULL,
    subject_label TEXT NOT NULL,             -- the issue key or the page title, as it was
    href          TEXT NOT NULL,
    actor         TEXT NOT NULL,
    text          TEXT NOT NULL DEFAULT '',  -- the sentence around a mention, a comment's start
    detail        TEXT NOT NULL DEFAULT '{}', -- what several changes merged into it (FR-113.2)
    source        INTEGER NOT NULL,          -- the position of the event that made it
    last_source   INTEGER NOT NULL,          -- the position of the last event merged into it
    created_at    INTEGER NOT NULL,
    updated_at    INTEGER NOT NULL,
    read_at       INTEGER,
    dismissed_at  INTEGER,
    UNIQUE (recipient, source, kind)
) STRICT;

CREATE INDEX notifications_recipient ON notifications (recipient, updated_at);
CREATE INDEX notifications_subject ON notifications (recipient, subject_id);
CREATE INDEX notifications_read ON notifications (read_at) WHERE read_at IS NOT NULL;
CREATE INDEX notifications_dismissed ON notifications (dismissed_at) WHERE dismissed_at IS NOT NULL;
