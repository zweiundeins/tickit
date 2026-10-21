-- A page's "Create issue" preselects the project a person last created an
-- issue in (FR-105) on every load of the page: by this index, rather than
-- walking every issue by its creation for someone who reported none.
CREATE INDEX issues_reporter ON issues (reporter, created_at);
