# Database Migrations and Seed Data

This package provides a comprehensive database migration and seeding system for the user management module.

## Features

### Migration System
- **Versioned migrations**: Each migration has a unique version number
- **Up/Down migrations**: Support for both applying and rolling back migrations
- **Migration tracking**: Tracks applied migrations in `schema_migrations` table
- **Validation**: Validates migration files for conflicts and missing versions
- **Rollback support**: Can rollback to any previous version

### Seed Data System
- **Default users**: Creates admin, project manager, developer, and viewer users
- **Project memberships**: Assigns users to projects with appropriate roles
- **Token cleanup**: Removes expired tokens from the database
- **Idempotent**: Can be run multiple times safely

## Migration Files

Migration files are stored in `internal/migrations/sql/` with the naming convention:
```
{version}_{name}.sql
```

Each migration file contains:
```sql
-- +migrate Up
-- SQL for applying the migration

-- +migrate Down  
-- SQL for rolling back the migration
```

### Current Migrations

1. **001_create_users_tables.sql** - Creates the main users table with indexes
2. **002_create_user_credentials_table.sql** - Creates table for authentication credentials
3. **003_create_user_sessions_table.sql** - Creates table for user sessions
4. **004_create_project_memberships_table.sql** - Creates table for project memberships
5. **005_create_email_verification_tokens_table.sql** - Creates table for email verification
6. **006_create_password_reset_tokens_table.sql** - Creates table for password reset tokens
7. **007_create_magic_mail_tokens_table.sql** - Creates table for magic mail tokens (dev only)
8. **008_create_user_invitations_table.sql** - Creates table for project invitations
9. **009_add_database_constraints.sql** - Adds database constraints and validation triggers

## Usage

### Command Line Tool

Use the `cmd/migrate` tool to manage migrations:

```bash
# Check migration status
go run ./cmd/migrate -action=status

# Apply all pending migrations
go run ./cmd/migrate -action=up

# Rollback to version 5
go run ./cmd/migrate -action=down -version=5

# Create a new migration file
go run ./cmd/migrate -action=create -name=add_user_preferences
```

### Programmatic Usage

```go
import "github.com/mbolli/tickit/internal/migrations"

// Create migration runner
userStore, _ := users.NewUserStore(ctx, "data/users.db")
runner, _ := migrations.NewRunner(ctx, "data/users.db", userStore)

// Run migrations
err := runner.RunMigrations(ctx)

// Seed database
err = runner.SeedDatabase(ctx, projectIDs)

// Check status
err = runner.GetMigrationStatus(ctx)
```

### Integration with Seed Command

The seed command (`cmd/dev/seed.go`) has been updated to:
1. Run database migrations first
2. Create projects and issues
3. Seed user data with project memberships

## Default Seed Data

The system creates the following default users:

| Email | Username | Password | Role |
|-------|----------|----------|------|
| admin@tickit.local | admin | admin123 | Admin |
| pm@tickit.local | projectmanager | pm123 | ProjectManager |
| dev@tickit.local | developer | dev123 | Developer |
| viewer@tickit.local | viewer | viewer123 | Viewer |

All users are:
- Email verified
- Active status
- Assigned to the first project (if available)

## Database Constraints

The migration system includes comprehensive database constraints:

### Validation Triggers
- Email format validation
- User role validation (Admin, ProjectManager, Developer, Viewer)
- User status validation (Active, Inactive, Pending)
- Credential type validation (password, oauth, webauthn)
- Project membership role validation
- Token expiration validation

### Automatic Timestamps
- Updates `updated_at` timestamp on user and credential changes
- Tracks creation and modification times

### Foreign Key Constraints
- All related tables have proper foreign key relationships
- Cascade deletes for user-related data

## Security Features

- Password hashing with bcrypt
- Token-based authentication for various flows
- Session management with expiration
- Audit trail through timestamps
- Input validation at database level

## Testing

The migration system can be tested with:

```go
// Create test database
userStore, _ := users.NewUserStore(ctx, "test.db")
runner, _ := migrations.NewRunner(ctx, "test.db", userStore)

// Test migrations
err := runner.RunMigrations(ctx)
err = runner.SeedDatabase(ctx, []string{"test-project"})
```

## Requirements Satisfied

This implementation satisfies the following requirements from the task:

✅ **Write migration scripts for all database schema changes**
- 9 comprehensive migration files covering all user management tables
- Proper up/down migration support
- Database constraints and validation

✅ **Create seed data for default admin user and roles**
- Default users for all roles (Admin, ProjectManager, Developer, Viewer)
- Integration with existing seed command
- Project membership assignments

✅ **Add migration rollback scripts for safe deployment**
- All migrations include rollback SQL
- Version-based rollback system
- Safe rollback to any previous version

✅ **Implement database constraint validation**
- Comprehensive validation triggers
- Foreign key constraints
- Data integrity checks
- Automatic timestamp management

✅ **Requirements 12.9 and database integrity**
- Traditional database storage (not event sourcing) for security
- Proper indexing for performance
- Data integrity through constraints
- Audit trail capabilities