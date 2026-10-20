-- Multiplayer (MP-02.4, MP-07.4). presence_input is when each person last
-- gave input in any tab, in unix seconds: a presence row shows them away
-- after 5 minutes without. A page reports input at most once a minute;
-- the sweep deletes rows older than a day.
CREATE TABLE presence_input (
    person_id TEXT PRIMARY KEY,
    at        INTEGER NOT NULL
) STRICT, WITHOUT ROWID;

-- The tab a typing indicator comes from, so that it goes when that tab's
-- stream ends.
ALTER TABLE issue_typing ADD COLUMN tab TEXT NOT NULL DEFAULT '';
CREATE INDEX issue_typing_tab ON issue_typing (person_id, tab);
