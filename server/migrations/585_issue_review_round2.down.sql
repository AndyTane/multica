-- Restore the 584 behavior: only leaving in_review cleared the four keys.
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
