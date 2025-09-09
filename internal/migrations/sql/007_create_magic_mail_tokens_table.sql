-- +migrate Up
-- Create magic mail tokens table (development only)
CREATE TABLE IF NOT EXISTS magic_mail_tokens (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    token TEXT UNIQUE NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME NOT NULL,
    used_at DATETIME,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create indexes for magic mail tokens table
CREATE INDEX IF NOT EXISTS idx_magic_mail_tokens_token ON magic_mail_tokens(token);
CREATE INDEX IF NOT EXISTS idx_magic_mail_tokens_user_id ON magic_mail_tokens(user_id);
CREATE INDEX IF NOT EXISTS idx_magic_mail_tokens_expires_at ON magic_mail_tokens(expires_at);

-- +migrate Down
DROP INDEX IF EXISTS idx_magic_mail_tokens_expires_at;
DROP INDEX IF EXISTS idx_magic_mail_tokens_user_id;
DROP INDEX IF EXISTS idx_magic_mail_tokens_token;
DROP TABLE IF EXISTS magic_mail_tokens;