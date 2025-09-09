-- +migrate Up
-- Create project memberships table for user-project relationships
CREATE TABLE IF NOT EXISTS project_memberships (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    project_id TEXT NOT NULL,
    role TEXT NOT NULL, -- 'ProjectAdmin', 'Developer', 'Viewer'
    created_at DATETIME NOT NULL,
    created_by TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(user_id, project_id)
);

-- Create indexes for project memberships table
CREATE INDEX IF NOT EXISTS idx_project_memberships_user_id ON project_memberships(user_id);
CREATE INDEX IF NOT EXISTS idx_project_memberships_project_id ON project_memberships(project_id);
CREATE INDEX IF NOT EXISTS idx_project_memberships_role ON project_memberships(role);

-- +migrate Down
DROP INDEX IF EXISTS idx_project_memberships_role;
DROP INDEX IF EXISTS idx_project_memberships_project_id;
DROP INDEX IF EXISTS idx_project_memberships_user_id;
DROP TABLE IF EXISTS project_memberships;