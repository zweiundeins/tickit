-- CutDue finds the pages it may cut in collab_uncut, which a rename now
-- marks too. Pages renamed since their last version before that, and any
-- other current page with changes no version holds, get their row; the
-- last version's time is a lower bound, as in migration 087.
INSERT OR IGNORE INTO collab_uncut (kind, id, at)
SELECT 'pagedoc', p.id, v.created_at FROM pages p
JOIN page_doc d ON d.page_id = p.id
JOIN page_versions v ON v.page_id = p.id AND v.number = (SELECT max(number) FROM page_versions WHERE page_id = p.id)
WHERE p.state = 'current' AND (d.version > v.upto OR v.title != p.title);
