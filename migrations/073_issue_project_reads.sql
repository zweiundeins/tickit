-- The issue page reads its project's epics and its 25 issues changed last
-- on every change to the issue: these indexes read them without walking
-- the project's issues.
CREATE INDEX issues_project_type ON issues (project_id, type, rank);
CREATE INDEX issues_project_updated ON issues (project_id, updated_at);
