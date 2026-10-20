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
