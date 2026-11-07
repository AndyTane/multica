-- CLAWSTA-151 R4 (B8): review marks live on two statuses now.
--
-- Leaving EITHER review status clears all five reserved review keys, so a
-- blocked -> in_review (or in_review -> blocked) transition never carries the
-- previous round's mark into the next one; the owner must publish a new mark.
-- The 584 triggers keep pointing at this function, so this migration only
-- replaces its body and does not touch the already-released 584 file.
CREATE OR REPLACE FUNCTION maintain_issue_review() RETURNS trigger AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        NEW.status_changed_at := NEW.created_at;
    ELSIF OLD.status IS DISTINCT FROM NEW.status THEN
        NEW.status_changed_at := now();
        IF OLD.status IN ('in_review', 'blocked') THEN
            NEW.metadata := NEW.metadata - ARRAY['review_by','review_action','review_user','review_agent','review_kind'];
        END IF;
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;
