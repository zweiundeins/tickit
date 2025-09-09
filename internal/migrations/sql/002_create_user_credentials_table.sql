-- +migrate Up
-- Create user credentials table for storing authentication credentials
CREATE TABLE IF NOT EXISTS user_credentials (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL,
    type TEXT NOT NULL, -- 'password', 'oauth', 'webauthn'
    provider TEXT NOT NULL, -- 'local', 'github', 'google', etc.
    provider_id TEXT, -- External provider user ID
    credential_data TEXT NOT NULL, -- Encrypted/hashed data
    created_at DATETIME NOT NULL,
    updated_at DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

-- Create indexes for user credentials table
CREATE INDEX IF NOT EXISTS idx_user_credentials_user_id ON user_credentials(user_id);
CREATE INDEX IF NOT EXISTS idx_user_credentials_provider ON user_credentials(provider, provider_id);
CREATE INDEX IF NOT EXISTS idx_user_credentials_type ON user_credentials(type);

-- +migrate Down
DROP INDEX IF EXISTS idx_user_credentials_type;
DROP INDEX IF EXISTS idx_user_credentials_provider;
DROP INDEX IF EXISTS idx_user_credentials_user_id;
DROP TABLE IF EXISTS user_credentials;