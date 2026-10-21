-- Written by the commands, never by a projector: per co-edited document
-- (kind "pagedoc" or "issuedoc", the subject's id) the time of its oldest
-- commit that no version holds yet, so that the fold finds its bound by
-- index (NFR-43.3). A commit inserts the row if there is none, a version
-- cut deletes it (internal/collab, Kind.Cut).
CREATE TABLE collab_uncut (
    kind TEXT NOT NULL,
    id   TEXT NOT NULL,
    at   INTEGER NOT NULL,
    PRIMARY KEY (kind, id)
) STRICT, WITHOUT ROWID;

CREATE INDEX collab_uncut_at ON collab_uncut (kind, at);

-- The documents with commits after their last version. lith_events may
-- not exist yet, so the times are lower bounds: a page's last version's
-- (its commits came after it), 0 for a description. The next cut of
-- each replaces them; until then the fold holds back as before.
INSERT INTO collab_uncut (kind, id, at)
SELECT 'pagedoc', d.page_id, v.created_at FROM page_doc d
JOIN page_versions v ON v.page_id = d.page_id AND v.number = (SELECT max(number) FROM page_versions WHERE page_id = d.page_id)
WHERE d.version > v.upto;

INSERT INTO collab_uncut (kind, id, at)
SELECT 'issuedoc', d.issue_id, 0 FROM issue_doc d JOIN issues i ON i.id = d.issue_id
WHERE d.version > i.desc_upto;
