-- What each person viewed lately (FR-59.1): the newest 20 issues and
-- pages per person, for "Recently viewed" on the home page. A page posts
-- the view once per open (GET never writes); the hourly sweep drops rows
-- older than 30 days.
CREATE TABLE recent_views (
    person_id  TEXT NOT NULL,
    subject_id TEXT NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('issue', 'page')),
    seen_at    INTEGER NOT NULL,
    PRIMARY KEY (person_id, subject_id)
) WITHOUT ROWID;

CREATE INDEX recent_views_seen ON recent_views (person_id, seen_at);
