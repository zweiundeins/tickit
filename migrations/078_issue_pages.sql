-- The pages linked to an issue by hand (FR-102), beside the pages whose
-- documents show it: the tracker's issues projector keeps it from the
-- issue stream's issue.page_linked and issue.page_unlinked.
CREATE TABLE issue_pages (
    issue_id  TEXT NOT NULL,
    page_id   TEXT NOT NULL,
    linked_at INTEGER NOT NULL,
    linked_by TEXT NOT NULL,
    PRIMARY KEY (issue_id, page_id)
) WITHOUT ROWID;

CREATE INDEX issue_pages_page ON issue_pages (page_id, issue_id);
