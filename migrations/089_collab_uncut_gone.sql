-- The rows of collab_uncut whose subject is gone: pages in the trash or
-- deleted, issues deleted or hidden. No version is cut for them, so they
-- held the fold back for every document of their kind. TrashPage,
-- DeletePage, DeleteIssue and HideIssue delete them now; a restore puts
-- one back (internal/collab, Kind.Release and Kind.Hold).
DELETE FROM collab_uncut WHERE kind = 'pagedoc'
AND id NOT IN (SELECT id FROM pages WHERE state = 'current');

DELETE FROM collab_uncut WHERE kind = 'issuedoc'
AND id NOT IN (SELECT id FROM issues WHERE deleted_at IS NULL);
