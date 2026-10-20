-- Search (internal/search): an FTS5 index per kind of document, issues,
-- pages and comments, each a projection that one projector writes and a
-- rebuild replaces (search_issues by the tracker's issue stream,
-- search_comments by its comment stream, search_pages by the wiki's page
-- stream). A kind has two tables: its documents, whose n is the rowid of
-- their text in the FTS5 table beside it. No triggers, no foreign keys:
-- lith's rebuild copies them from these statements and swaps them in.
-- Times are unix milliseconds.

CREATE TABLE search_issues (
    n        INTEGER PRIMARY KEY,
    id       TEXT NOT NULL UNIQUE,
    scope    TEXT NOT NULL,              -- the project's id
    key      TEXT NOT NULL DEFAULT '',   -- the issue's key, upper case
    old_keys TEXT NOT NULL DEFAULT '',   -- the keys before moves, space-separated
    title    TEXT NOT NULL DEFAULT '',   -- the summary, folded for exact matches
    href     TEXT NOT NULL DEFAULT '',
    place    TEXT NOT NULL DEFAULT '',   -- what results show beside the title
    labels   TEXT NOT NULL DEFAULT '[]',
    people   TEXT NOT NULL DEFAULT '[]', -- reporter and assignee
    hidden   INTEGER NOT NULL DEFAULT 0,
    updated  INTEGER NOT NULL
) STRICT;

CREATE VIRTUAL TABLE search_issues_fts USING fts5(key, title, body, tokenize = 'unicode61 remove_diacritics 2', prefix = '2 3');

CREATE TABLE search_pages (
    n        INTEGER PRIMARY KEY,
    id       TEXT NOT NULL UNIQUE,
    scope    TEXT NOT NULL,              -- the space's id
    key      TEXT NOT NULL DEFAULT '',
    old_keys TEXT NOT NULL DEFAULT '',
    title    TEXT NOT NULL DEFAULT '',
    href     TEXT NOT NULL DEFAULT '',
    place    TEXT NOT NULL DEFAULT '',   -- the space's key
    labels   TEXT NOT NULL DEFAULT '[]',
    people   TEXT NOT NULL DEFAULT '[]', -- author and editors
    hidden   INTEGER NOT NULL DEFAULT 0,
    updated  INTEGER NOT NULL
) STRICT;

CREATE VIRTUAL TABLE search_pages_fts USING fts5(key, title, body, tokenize = 'unicode61 remove_diacritics 2', prefix = '2 3');

-- Comments on issues and pages: scope is the subject's id, key its kind
-- ('issue', 'page'). A hidden comment keeps its text for when it shows
-- again.
CREATE TABLE search_comments (
    n        INTEGER PRIMARY KEY,
    id       TEXT NOT NULL UNIQUE,
    scope    TEXT NOT NULL,
    key      TEXT NOT NULL DEFAULT '',
    old_keys TEXT NOT NULL DEFAULT '',
    title    TEXT NOT NULL DEFAULT '',
    href     TEXT NOT NULL DEFAULT '',
    place    TEXT NOT NULL DEFAULT '',
    labels   TEXT NOT NULL DEFAULT '[]',
    people   TEXT NOT NULL DEFAULT '[]', -- the author
    hidden   INTEGER NOT NULL DEFAULT 0,
    updated  INTEGER NOT NULL
) STRICT;

CREATE VIRTUAL TABLE search_comments_fts USING fts5(key, title, body, tokenize = 'unicode61 remove_diacritics 2', prefix = '2 3');

-- Lists whose query is too long for a view's name (list:<query>): a
-- list's view names its hash instead, and this row holds the query. The
-- action that changes a list writes it; rows not used for 3 days go.
CREATE TABLE list_queries (
    hash  TEXT PRIMARY KEY,
    query TEXT NOT NULL,
    seen  INTEGER NOT NULL               -- unix seconds
) STRICT, WITHOUT ROWID;

CREATE INDEX list_queries_seen ON list_queries (seen);
