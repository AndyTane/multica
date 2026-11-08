-- Resume trusted issue wakeups from their immutable task-source snapshot.
-- The current wakeup configuration may have changed or been deleted.
-- Resolve each requested issue through indexed, tenant-scoped lineage walks.
-- A scalar SQL function keeps recursion scoped to one issue instead of
-- materializing a recursive CTE over the installation. The view remains the
-- single public projection for validation, batch reads, and list predicates.
CREATE OR REPLACE FUNCTION resolve_issue_dispatcher(issue_id uuid, workspace_id uuid)
RETURNS uuid LANGUAGE sql STABLE AS $dispatcher$
    WITH RECURSIVE parents AS (
        SELECT i.id, i.workspace_id, i.creator_type, i.creator_id,
               i.origin_type, i.origin_id, i.parent_issue_id,
               ARRAY[i.id] AS path, 0 AS depth
        FROM issue i WHERE i.id = $1 AND i.workspace_id = $2
        UNION ALL
        SELECT p.id, p.workspace_id, p.creator_type, p.creator_id,
               p.origin_type, p.origin_id, p.parent_issue_id,
               child.path || p.id, child.depth + 1
        FROM parents child
        JOIN issue p ON p.id = child.parent_issue_id
                    AND p.workspace_id = child.workspace_id
        -- A member-created card always owns its dispatcher, even as a child.
        WHERE child.creator_type = 'agent' AND NOT p.id = ANY(child.path)
    ), sources AS (
        SELECT p.depth,
            CASE
                WHEN p.creator_type = 'member' THEN p.creator_id
                WHEN p.creator_type = 'agent' AND p.origin_type = 'quick_create' THEN (
                    -- Preserve the original, strict quick-create contract,
                    -- including legacy requests without attribution labels.
                    SELECT t.originator_user_id
                    FROM agent_task_queue t
                    JOIN agent a ON a.id = t.agent_id AND a.workspace_id = p.workspace_id
                    WHERE t.id = p.origin_id AND t.agent_id = p.creator_id
                      AND t.context->>'type' = 'quick_create'
                      AND t.context->>'workspace_id' = p.workspace_id::text
                      AND t.originator_user_id IS NOT NULL
                      AND t.context->>'requester_id' = t.originator_user_id::text
                )
                WHEN p.creator_type = 'agent' AND p.origin_type = 'agent_create' THEN (
                    WITH RECURSIVE tasks AS (
                        SELECT t.id, t.originator_user_id, t.originator_source,
                               t.delegated_from_task_id, t.trigger_evidence_kind, t.trigger_evidence_ref_id, ARRAY[t.id] AS path
                        FROM agent_task_queue t
                        JOIN agent a ON a.id = t.agent_id AND a.workspace_id = p.workspace_id
                        WHERE t.id = p.origin_id AND t.agent_id = p.creator_id
                          AND t.originator_user_id IS NOT NULL
                        UNION ALL
                        SELECT parent.id, parent.originator_user_id, parent.originator_source,
                               parent.delegated_from_task_id, parent.trigger_evidence_kind, parent.trigger_evidence_ref_id, child.path || parent.id
                        FROM tasks child
                        JOIN agent_task_queue parent ON parent.id = child.delegated_from_task_id
                        JOIN agent a ON a.id = parent.agent_id AND a.workspace_id = p.workspace_id
                        WHERE (child.originator_source IN ('delegation', 'comment_source')
                               -- A server-stamped wakeup resumes its source run;
                               -- its owner alone is never a human request.
                               OR (child.originator_source = 'trigger_owner'
                                   AND child.trigger_evidence_kind = 'issue_wakeup'
                                   AND child.trigger_evidence_ref_id IS NOT NULL))
                          AND parent.originator_user_id = child.originator_user_id
                          AND NOT parent.id = ANY(child.path)
                    )
                    -- An armed automation can carry a human for authorization.
                    -- Only a direct human root proves that a member asked for work;
                    -- owner/accountable values and truncated chains prove nothing.
                    SELECT t.originator_user_id FROM tasks t
                    WHERE t.originator_source = 'direct_human'
                    LIMIT 1
                )
            END::uuid AS dispatcher_user_id
        FROM parents p
    )
    SELECT s.dispatcher_user_id
    FROM sources s
    WHERE s.dispatcher_user_id IS NOT NULL
      AND NOT EXISTS (
          SELECT 1 FROM parents p
          WHERE p.creator_type = 'agent' AND p.parent_issue_id = ANY(p.path)
      )
    -- Resolve the parent before falling back to a card's own run. Applying
    -- this at every level keeps the child's result identical to its parent's;
    -- membership is checked afterwards, never used to skip a departed human.
    ORDER BY s.depth DESC
    LIMIT 1
$dispatcher$;
