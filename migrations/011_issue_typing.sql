-- Who is typing or editing on an issue now (MP-07): a row per issue,
-- person and field ("summary", "description", "comment"), with the time of
-- the last keystroke in unix ms. Indicators go 5 s after it; the command
-- that writes a row deletes the issue's rows older than a minute.
CREATE TABLE issue_typing (
    issue_id  TEXT NOT NULL,
    person_id TEXT NOT NULL,
    field     TEXT NOT NULL,
    at        INTEGER NOT NULL,
    PRIMARY KEY (issue_id, person_id, field)
) STRICT, WITHOUT ROWID;
