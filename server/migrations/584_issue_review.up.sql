ALTER TABLE issue ADD COLUMN IF NOT EXISTS status_changed_at TIMESTAMPTZ;

-- Only the latest transition can describe the current round of review.
UPDATE issue i SET status_changed_at = COALESCE((
    SELECT CASE WHEN a.details->>'to' = i.status
                     AND a.details->>'from' <> a.details->>'to'
                THEN a.created_at END
    FROM activity_log a
    WHERE a.workspace_id = i.workspace_id AND a.issue_id = i.id
      AND a.action = 'status_changed'
    ORDER BY a.created_at DESC, a.id DESC LIMIT 1
), i.updated_at, i.created_at)
WHERE i.status_changed_at IS NULL;

ALTER TABLE issue ALTER COLUMN status_changed_at SET DEFAULT now();
ALTER TABLE issue ALTER COLUMN status_changed_at SET NOT NULL;

CREATE OR REPLACE FUNCTION maintain_issue_review() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        NEW.status_changed_at := NEW.created_at;
    ELSIF OLD.status IS DISTINCT FROM NEW.status THEN
        NEW.status_changed_at := now();
        IF OLD.status = 'in_review' THEN
            NEW.metadata := NEW.metadata - ARRAY['review_by','review_action','review_user','review_agent'];
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER issue_review_before_insert BEFORE INSERT ON issue
FOR EACH ROW EXECUTE FUNCTION maintain_issue_review();
CREATE TRIGGER issue_review_before_status BEFORE UPDATE OF status ON issue
FOR EACH ROW EXECUTE FUNCTION maintain_issue_review();
