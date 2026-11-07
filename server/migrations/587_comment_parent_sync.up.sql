-- CLAWSTA-151 R4 (B11): the persisted result of "sync this sub-issue comment to
-- the parent issue's assignee". Only the server writes it — clients send the
-- boolean switch and never a target. It records the parent, the resolved mode,
-- the recipient identity and the delivery outcome so a retry/replay can tell an
-- already delivered sync from a fresh one, and so the UI can explain a
-- downgrade (member / dispatcher / admin) instead of claiming an agent ran.
ALTER TABLE comment ADD COLUMN IF NOT EXISTS parent_sync JSONB;
