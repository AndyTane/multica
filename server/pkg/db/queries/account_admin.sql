-- Instance user administration helpers (P5).
--
-- Delete preview and delete share InspectInstanceUserDependencies: one query
-- enumerates every schema reference to an account and classifies it. The
-- preview endpoint returns exactly that result, and the delete transaction
-- re-runs it under the account locks before and after cleanup, so the preview
-- and the mutation cannot drift apart.
--
-- Classification:
--   blocker  the row represents ownership or an audit attribution that cannot
--            be reassigned, so the account cannot be deleted until an operator
--            resolves it. Shared business history is never silently deleted.
--   remove   an account-scoped row that is removed with the account.
--   (absent) nullable shared-history attribution is detached (SET NULL) by the
--            cleanup statements below.

-- name: ListUserWorkspaceOwnerships :many
SELECT w.id, w.slug, w.name
FROM member m
JOIN workspace w ON w.id = m.workspace_id
WHERE m.user_id = $1 AND m.role = 'owner'
ORDER BY w.created_at;

-- name: CountUserOwnedAgents :one
SELECT count(*) FROM agent WHERE owner_id = $1;

-- name: CountUserOwnedRuntimes :one
SELECT count(*) FROM agent_runtime WHERE owner_id = $1;

-- name: InspectInstanceUserDependencies :many
SELECT code, kind, count FROM (
    SELECT 'workspace_owner'::text AS code, 'blocker'::text AS kind, count(*) AS count
    FROM member m WHERE m.user_id = $1 AND m.role = 'owner'
    UNION ALL
    SELECT 'runtime_owner', 'blocker', count(*) FROM agent_runtime ar WHERE ar.owner_id = $1
    UNION ALL
    SELECT 'agent_owner', 'blocker', count(*) FROM agent a WHERE a.owner_id = $1
    UNION ALL
    SELECT 'channel_installation', 'blocker', count(*) FROM channel_installation ci WHERE ci.installer_user_id = $1
    UNION ALL
    SELECT 'lark_installation', 'blocker', count(*) FROM lark_installation li WHERE li.installer_user_id = $1
    UNION ALL
    SELECT 'share_link_creator', 'blocker', count(*) FROM workspace_share_link wsl WHERE wsl.created_by = $1
    UNION ALL
    SELECT 'source_context_capture', 'blocker', count(*) FROM issue_source_context isc WHERE isc.captured_by_user_id = $1
    UNION ALL
    SELECT 'registration_invite', 'remove', count(*) FROM registration_invite ri WHERE ri.user_id = $1 OR ri.created_by = $1
    UNION ALL
    SELECT 'password', 'remove', count(*) FROM local_password_credential lpc WHERE lpc.user_id = $1
    UNION ALL
    SELECT 'personal_access_token', 'remove', count(*) FROM personal_access_token pat WHERE pat.user_id = $1
    UNION ALL
    SELECT 'task_token', 'remove', count(*) FROM task_token tt WHERE tt.user_id = $1
    UNION ALL
    SELECT 'membership', 'remove', count(*) FROM member m WHERE m.user_id = $1
    UNION ALL
    SELECT 'workspace_invitation', 'remove', count(*) FROM workspace_invitation wi WHERE wi.invitee_user_id = $1 OR wi.inviter_id = $1
    UNION ALL
    SELECT 'identity_binding', 'remove',
           (SELECT count(*) FROM channel_user_binding cub WHERE cub.multica_user_id = $1)
         + (SELECT count(*) FROM lark_user_binding lub WHERE lub.multica_user_id = $1)
    UNION ALL
    SELECT 'notification_preference', 'remove', count(*) FROM notification_preference np WHERE np.user_id = $1
    UNION ALL
    SELECT 'pinned_item', 'remove', count(*) FROM pinned_item pi WHERE pi.user_id = $1
    UNION ALL
    SELECT 'feedback', 'remove', count(*) FROM feedback f WHERE f.user_id = $1
    UNION ALL
    SELECT 'client_usage', 'remove', count(*) FROM client_usage_daily cud WHERE cud.user_id = $1
    UNION ALL
    SELECT 'issue_subscription', 'remove', count(*) FROM issue_subscriber iss WHERE iss.user_id = $1 AND iss.user_type = 'member'
    UNION ALL
    SELECT 'composio_connection', 'remove', count(*) FROM user_composio_connection ucc WHERE ucc.user_id = $1
    UNION ALL
    SELECT 'seat_capacity', 'remove', count(*) FROM seat_capacity_outbox sco WHERE sco.user_id = $1
    UNION ALL
    SELECT 'chat_pin', 'remove', count(*) FROM chat_pinned_agent cpa WHERE cpa.user_id = $1
    UNION ALL
    SELECT 'autopilot_collaborator', 'remove', count(*) FROM autopilot_collaborator ac WHERE ac.user_id = $1 AND ac.user_type = 'member'
    UNION ALL
    SELECT 'autopilot_subscriber', 'remove', count(*) FROM autopilot_subscriber asub WHERE asub.user_id = $1 AND asub.user_type = 'member'
    UNION ALL
    SELECT 'issue_view_preference', 'remove', count(*) FROM issue_view_preference ivp WHERE ivp.user_id = $1
    UNION ALL
    SELECT 'issue_view', 'remove', count(*) FROM issue_view iv WHERE iv.owner_id = $1
    UNION ALL
    SELECT 'account', 'remove', count(*) FROM "user" u WHERE u.id = $1
) probes
WHERE count > 0
ORDER BY (kind = 'blocker') DESC, code;

-- name: DeleteUserPersonalAccessTokens :exec
DELETE FROM personal_access_token WHERE user_id = $1;

-- name: DeleteUserMemberships :exec
DELETE FROM member WHERE user_id = $1;

-- name: DeleteUserNotificationPreferences :exec
DELETE FROM notification_preference WHERE user_id = $1;

-- name: DeleteUserPinnedItems :exec
DELETE FROM pinned_item WHERE user_id = $1;

-- name: DeleteUserPasswordCredentialByUserID :exec
DELETE FROM local_password_credential WHERE user_id = $1;

-- name: DeleteUserTaskTokens :exec
DELETE FROM task_token WHERE user_id = $1;

-- name: DeleteUserWorkspaceInvitations :exec
DELETE FROM workspace_invitation WHERE invitee_user_id = $1 OR inviter_id = $1;

-- name: DeleteUserFeedback :exec
DELETE FROM feedback WHERE user_id = $1;

-- name: DeleteUserClientUsage :exec
DELETE FROM client_usage_daily WHERE user_id = $1;

-- name: DeleteUserIssueSubscriptions :exec
DELETE FROM issue_subscriber WHERE user_id = $1 AND user_type = 'member';

-- name: DeleteUserComposioConnections :exec
DELETE FROM user_composio_connection WHERE user_id = $1;

-- name: DeleteUserSeatCapacityOutbox :exec
DELETE FROM seat_capacity_outbox WHERE user_id = $1;

-- name: DeleteUserChatPins :exec
DELETE FROM chat_pinned_agent WHERE user_id = $1;

-- name: DeleteUserAutopilotCollaborators :exec
DELETE FROM autopilot_collaborator WHERE user_id = $1 AND user_type = 'member';

-- name: DeleteUserAutopilotSubscribers :exec
DELETE FROM autopilot_subscriber WHERE user_id = $1 AND user_type = 'member';

-- name: DeleteUserIssueViewPreferences :exec
DELETE FROM issue_view_preference WHERE user_id = $1;

-- name: DeleteUserIssueViews :exec
DELETE FROM issue_view WHERE owner_id = $1;

-- name: DeleteUserChannelBindings :exec
DELETE FROM channel_user_binding WHERE multica_user_id = $1;

-- name: DeleteUserLarkBindings :exec
DELETE FROM lark_user_binding WHERE multica_user_id = $1;

-- name: ListUserIDsWithLocalPassword :many
SELECT user_id FROM local_password_credential;

-- Detach nullable attribution on shared history. The rows stay: only the
-- pointer to the removed account is cleared, so a task, skill, wakeup or
-- package authored by that person keeps existing with an unattributed author.
-- name: DetachUserAgentInvocationTargets :exec
UPDATE agent_invocation_target SET created_by = NULL WHERE created_by = $1;

-- name: DetachUserArchivedAgents :exec
-- agent.archived_by references "user" with no delete action, so the pointer is
-- cleared before the row is removed. The agent itself is shared history.
UPDATE agent SET archived_by = NULL WHERE archived_by = $1;

-- name: DetachUserAgentTaskQueue :exec
UPDATE agent_task_queue
SET accountable_user_id = CASE WHEN accountable_user_id = $1 THEN NULL ELSE accountable_user_id END,
    initiator_user_id = CASE WHEN initiator_user_id = $1 THEN NULL ELSE initiator_user_id END,
    originator_user_id = CASE WHEN originator_user_id = $1 THEN NULL ELSE originator_user_id END
WHERE accountable_user_id = $1 OR initiator_user_id = $1 OR originator_user_id = $1;

-- name: DetachUserChannelChatContextGenerations :exec
UPDATE channel_chat_context_generation SET initiator_user_id = NULL WHERE initiator_user_id = $1;

-- name: DetachUserIssueWakeups :exec
UPDATE issue_wakeup SET created_by = NULL WHERE created_by = $1;

-- name: DetachUserPluginPackages :exec
UPDATE plugin_package SET created_by = NULL WHERE created_by = $1;

-- name: DetachUserProjectResources :exec
UPDATE project_resource SET created_by = NULL WHERE created_by = $1;

-- name: DetachUserRuntimeProfiles :exec
UPDATE runtime_profile SET created_by = NULL WHERE created_by = $1;

-- name: DetachUserSkills :exec
UPDATE skill SET created_by = NULL WHERE created_by = $1;

-- name: DetachUserWorkspaceMCPServers :exec
UPDATE workspace_mcp_server SET created_by = NULL WHERE created_by = $1;
