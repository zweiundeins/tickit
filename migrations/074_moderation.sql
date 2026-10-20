-- Moderation of page versions, images and reports (DM-07.1, DM-08).
-- Page versions and images a moderator hid (DM-07.1). The wiki's versions
-- projector writes the columns from page.version_hidden and
-- page.version_unhidden; HideImage writes hidden_images, which a reset
-- empties with the rest of the demo.
ALTER TABLE page_versions ADD COLUMN hidden_at INTEGER;
ALTER TABLE page_versions ADD COLUMN hidden_by TEXT NOT NULL DEFAULT '';
ALTER TABLE page_versions ADD COLUMN hidden_reason TEXT NOT NULL DEFAULT '';

CREATE TABLE hidden_images (
    image_id  TEXT PRIMARY KEY,
    hidden_at INTEGER NOT NULL,
    hidden_by TEXT NOT NULL,
    reason    TEXT NOT NULL DEFAULT ''
) STRICT, WITHOUT ROWID;

-- Reports of what visitors wrote (DM-08): one open report per reporter and
-- thing; a moderator's hide, delete or dismiss closes them. A reset empties
-- them with the content they are about.
CREATE TABLE reports (
    id          INTEGER PRIMARY KEY,
    kind        TEXT NOT NULL,
    target_id   TEXT NOT NULL,
    label       TEXT NOT NULL,
    reason      TEXT NOT NULL,
    by          TEXT NOT NULL,
    at          INTEGER NOT NULL,
    closed_at   INTEGER,
    closed_by   TEXT NOT NULL DEFAULT '',
    outcome     TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE UNIQUE INDEX reports_open ON reports (kind, target_id, by) WHERE closed_at IS NULL;
CREATE INDEX reports_target ON reports (kind, target_id) WHERE closed_at IS NULL;
