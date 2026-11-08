-- CLAWSTA-151 R4 (B9): the person who dispatched an issue, resolved from
-- server-side relations only — never from metadata or client input.
--
--   * a member created the issue           -> that member
--   * an agent created it via quick-create -> the origin task's human requester,
--     but only when the origin task still proves the link: same workspace, the
--     task belongs to the creating agent, its persisted context is a
--     quick_create request for this workspace whose requester is the task's
--     own originator_user_id. Anything missing or mismatched resolves to NULL
--     rather than guessing an agent owner, the current viewer, or a parent
--     issue's dispatcher.
--   * agent_create, autopilot, and untrusted/missing origins -> NULL.
--
-- dispatcher_active reports whether that member is still in the workspace.
-- This is a view (not a column) so it cannot become a second mutable source:
-- both the read projection and the list filters resolve it the same way.
CREATE OR REPLACE VIEW issue_dispatcher AS
WITH resolved AS (
    SELECT
        i.id,
        i.workspace_id,
        CASE
            WHEN i.creator_type = 'member' AND i.creator_id IS NOT NULL THEN i.creator_id
            WHEN i.creator_type = 'agent'
                 AND i.origin_type = 'quick_create'
                 AND i.origin_id IS NOT NULL
            THEN (
                SELECT t.originator_user_id
                FROM agent_task_queue t
                JOIN agent a ON a.id = t.agent_id
                WHERE t.id = i.origin_id
                  AND a.workspace_id = i.workspace_id
                  AND t.agent_id = i.creator_id
                  AND t.context->>'type' = 'quick_create'
                  AND t.context->>'workspace_id' = i.workspace_id::text
                  AND t.originator_user_id IS NOT NULL
                  AND t.context->>'requester_id' = t.originator_user_id::text
            )
        END::uuid AS dispatcher_user_id
    FROM issue i
)
SELECT
    r.id,
    r.workspace_id,
    r.dispatcher_user_id,
    (r.dispatcher_user_id IS NOT NULL AND EXISTS (
        SELECT 1 FROM member m
        WHERE m.workspace_id = r.workspace_id AND m.user_id = r.dispatcher_user_id
    )) AS dispatcher_active
FROM resolved r;

DROP FUNCTION IF EXISTS resolve_issue_dispatcher(uuid, uuid);
