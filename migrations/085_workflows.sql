-- Workflows and board columns (FR-36, FR-37, FR-43), kept by
-- tracker_projects. A status keeps its row once removed (removed = 1), so
-- that history and feeds still name it; board_column is the board column
-- that shows it, '' for none; follows is the JSON list of the statuses it
-- may follow, '' for any.
ALTER TABLE statuses ADD COLUMN board_column TEXT NOT NULL DEFAULT '';
ALTER TABLE statuses ADD COLUMN follows TEXT NOT NULL DEFAULT '';
ALTER TABLE statuses ADD COLUMN removed INTEGER NOT NULL DEFAULT 0;

-- A board's columns in order, each with its work-in-progress limit (0 for
-- none); its statuses are those whose board_column names it.
CREATE TABLE board_columns (
    project_id TEXT NOT NULL,
    id         TEXT NOT NULL,
    name       TEXT NOT NULL,
    position   INTEGER NOT NULL,
    wip_limit  INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (project_id, id)
) STRICT, WITHOUT ROWID;
