-- A project's open issues are counted on every render of the home page and
-- the project list: this index counts them without reading the table.
CREATE INDEX issues_project_open ON issues (project_id, deleted_at, status_category);
