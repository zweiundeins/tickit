-- +migrate Up
-- Create user invitations table for project invitations
CREATE TABLE IF NOT EXISTS user_invitations (
    id TEXT PRIMARY KEY,
    project_id TEXT NOT NULL,
    email TEXT NOT NULL,
    role TEXT NOT NULL,
    token TEXT UNIQUE NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    created_by TEXT NOT NULL,
    accepted_at DATETIME,
    declined_at DATETIME
);

-- Create indexes for user invitations table
CREATE INDEX IF NOT EXISTS idx_user_invitations_token ON user_invitations(token);
CREATE INDEX IF NOT EXISTS idx_user_invitations_email ON user_invitations(email);
CREATE INDEX IF NOT EXISTS idx_user_invitations_project_id ON user_invitations(project_id);
CREATE INDEX IF NOT EXISTS idx_user_invitations_expires_at ON user_invitations(expires_at);

-- +migrate Down
DROP INDEX IF EXISTS idx_user_invitations_expires_at;
DROP INDEX IF EXISTS idx_user_invitations_project_id;
DROP INDEX IF EXISTS idx_user_invitations_email;
DROP INDEX IF EXISTS idx_user_invitations_token;
DROP TABLE IF EXISTS user_invitations;