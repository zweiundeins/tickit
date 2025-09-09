-- +migrate Up
-- Add basic database constraints

-- Create trigger to validate user email format
CREATE TRIGGER IF NOT EXISTS validate_user_email
    BEFORE INSERT ON users
    FOR EACH ROW
    WHEN NEW.email NOT LIKE '%@%.%'
BEGIN
    SELECT RAISE(ABORT, 'Invalid email format');
END;

-- Note: Additional validation triggers are handled in the application layer
-- Complex triggers can cause transaction conflicts during migrations

-- +migrate Down
DROP TRIGGER IF EXISTS validate_user_email;