DROP TRIGGER IF EXISTS issue_review_before_status ON issue;
DROP TRIGGER IF EXISTS issue_review_before_insert ON issue;
DROP FUNCTION IF EXISTS maintain_issue_review();
ALTER TABLE issue DROP COLUMN IF EXISTS status_changed_at;
