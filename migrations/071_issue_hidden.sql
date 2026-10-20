-- Issues a moderator hid (DM-07.1): the tracker's issues projector writes
-- these with issue.hidden and issue.unhidden. A hidden issue leaves every
-- list as a deleted one does, and comes back when restored.
ALTER TABLE issues ADD COLUMN hidden_at INTEGER;
ALTER TABLE issues ADD COLUMN hidden_by TEXT NOT NULL DEFAULT '';
ALTER TABLE issues ADD COLUMN hidden_reason TEXT NOT NULL DEFAULT '';
