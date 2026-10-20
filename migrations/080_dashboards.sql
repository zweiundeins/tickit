-- Dashboards (FR-60): a person's named grid of gadgets. Relational rows
-- that internal/dashboards' commands write, like saved filters: in the
-- demo everyone sees every dashboard, and its owner changes it.
CREATE TABLE dashboards (
    id         TEXT PRIMARY KEY,
    owner      TEXT NOT NULL,
    name       TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
) WITHOUT ROWID;

CREATE INDEX dashboards_owner ON dashboards (owner);

CREATE TABLE dashboard_gadgets (
    id           TEXT PRIMARY KEY,
    dashboard_id TEXT NOT NULL,
    kind         TEXT NOT NULL CHECK (kind IN ('filter', 'status', 'assigned', 'activity', 'created')),
    position     INTEGER NOT NULL,
    project_id   TEXT NOT NULL DEFAULT '',
    filter_id    TEXT NOT NULL DEFAULT '',
    created_at   INTEGER NOT NULL
) WITHOUT ROWID;

CREATE INDEX dashboard_gadgets_dashboard ON dashboard_gadgets (dashboard_id, position);
