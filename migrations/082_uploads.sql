-- Images people upload into pages and issue descriptions (FR-85), as
-- images.Upload writes them: decoded and encoded again without metadata,
-- under a generated id that documents name as they name the library's
-- images. A moderator's hide is a row of hidden_images, as for the library;
-- a delete removes the row. A reset empties it with the rest of the demo.
CREATE TABLE uploads (
    id           TEXT PRIMARY KEY,
    type         TEXT NOT NULL,
    width        INTEGER NOT NULL,
    height       INTEGER NOT NULL,
    bytes        INTEGER NOT NULL,
    data         BLOB NOT NULL,
    subject_kind TEXT NOT NULL,
    subject_id   TEXT NOT NULL,
    created_by   TEXT NOT NULL,
    created_at   INTEGER NOT NULL
) STRICT;

CREATE INDEX uploads_by ON uploads (created_by);
CREATE INDEX uploads_at ON uploads (created_at);
