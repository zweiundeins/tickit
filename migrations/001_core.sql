-- The core: people, accounts and their login sessions, tab state,
-- preferences, scopes and memberships, the security log. Times are unix
-- seconds. Every table here is written by commands only; no projector
-- writes them, so their constraints guard rules (NFR-18).

-- Everyone who acts: guests (g-, from a hash of the session id), accounts
-- (a-, a UUIDv7) and the seeded people (s-, a slug). A guest's row appears
-- with their first command (people.Ensure); until then the name and face
-- derive from the id alone.
CREATE TABLE people (
    id           TEXT PRIMARY KEY,
    kind         TEXT NOT NULL CHECK (kind IN ('guest', 'account', 'seed')),
    display_name TEXT NOT NULL,
    hue          INTEGER NOT NULL,
    created_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL
) STRICT, WITHOUT ROWID;

CREATE INDEX people_seen ON people (kind, last_seen_at);

-- The account of a person of kind 'account': staff, who log in (Q14).
CREATE TABLE accounts (
    person_id            TEXT PRIMARY KEY REFERENCES people (id) ON DELETE CASCADE,
    email                TEXT NOT NULL UNIQUE COLLATE NOCASE,
    username             TEXT NOT NULL UNIQUE COLLATE NOCASE,
    full_name            TEXT NOT NULL,
    password_hash        TEXT NOT NULL,  -- argon2id, PHC string
    site_role            TEXT NOT NULL DEFAULT 'none' CHECK (site_role IN ('none', 'moderator', 'admin')),
    status               TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('pending', 'active', 'inactive')),
    email_verified       INTEGER NOT NULL DEFAULT 0,
    must_change_password INTEGER NOT NULL DEFAULT 0,
    last_login_at        INTEGER          -- null: never
) STRICT, WITHOUT ROWID;

-- Who is logged in where: keyed by the SHA-256 of lith's session id, never
-- the id itself. No client address (NFR-25). A row past expires_at counts
-- for nothing and the sweep deletes it.
CREATE TABLE login_sessions (
    sid_hash     TEXT PRIMARY KEY,
    person_id    TEXT NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    created_at   INTEGER NOT NULL,
    expires_at   INTEGER NOT NULL,
    last_seen_at INTEGER NOT NULL,
    user_agent   TEXT NOT NULL
) STRICT, WITHOUT ROWID;

CREATE INDEX login_sessions_person ON login_sessions (person_id);
CREATE INDEX login_sessions_expiry ON login_sessions (expires_at);

-- One loaded page: its outcomes, its areas' state, the views it added to
-- its stream (tabs.State as JSON). Swept 3 days after it was last seen.
CREATE TABLE tab_state (
    sid    TEXT NOT NULL,
    tab_id TEXT NOT NULL,
    kind   TEXT NOT NULL DEFAULT '',  -- the page's kind, as an area sets it
    data   TEXT NOT NULL,
    seen   INTEGER NOT NULL,
    PRIMARY KEY (sid, tab_id)
) STRICT, WITHOUT ROWID;

CREATE INDEX tab_state_seen ON tab_state (seen);

-- Preferences that outlive a page, per session, as JSON.
CREATE TABLE prefs (
    sid  TEXT PRIMARY KEY,
    data TEXT NOT NULL,
    seen INTEGER NOT NULL
) STRICT, WITHOUT ROWID;

-- Projects and spaces as guard rows: their keys are unique per kind, and
-- access reads open and archived from here (access.CreateScope).
CREATE TABLE scopes (
    kind       TEXT NOT NULL CHECK (kind IN ('project', 'space')),
    id         TEXT NOT NULL,
    key        TEXT NOT NULL,
    open       INTEGER NOT NULL,           -- 1: everyone has Member rights (Q14)
    archived   INTEGER NOT NULL DEFAULT 0,
    created_by TEXT NOT NULL,
    created_at INTEGER NOT NULL,
    PRIMARY KEY (kind, id),
    UNIQUE (kind, key)
) STRICT, WITHOUT ROWID;

CREATE TABLE memberships (
    person_id  TEXT NOT NULL REFERENCES people (id) ON DELETE CASCADE,
    scope_kind TEXT NOT NULL,
    scope_id   TEXT NOT NULL,
    role       TEXT NOT NULL CHECK (role IN ('viewer', 'member', 'admin')),
    added_at   INTEGER NOT NULL,
    added_by   TEXT NOT NULL,
    PRIMARY KEY (person_id, scope_kind, scope_id),
    FOREIGN KEY (scope_kind, scope_id) REFERENCES scopes (kind, id) ON DELETE CASCADE
) STRICT, WITHOUT ROWID;

CREATE INDEX memberships_scope ON memberships (scope_kind, scope_id, role);

-- Logins, failed logins, logouts, role changes, deactivations and
-- moderation (FR-156): never a password, a token or a session id. Swept
-- after 90 days.
CREATE TABLE security_log (
    id        INTEGER PRIMARY KEY,
    at        INTEGER NOT NULL,
    kind      TEXT NOT NULL,
    person_id TEXT,                 -- the account it concerns; null for an unknown email
    actor_id  TEXT,                 -- who did it, when that is someone else
    outcome   TEXT NOT NULL,
    detail    TEXT NOT NULL DEFAULT ''
) STRICT;

CREATE INDEX security_log_person ON security_log (person_id, at);
CREATE INDEX security_log_at ON security_log (at);
