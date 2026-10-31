-- Instance user administration helpers (P5).
-- The account deletion flow enumerates the account-level dependencies it owns;
-- workspace-owned business content is not deleted here.

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

-- name: DeleteUserPersonalAccessTokens :exec
DELETE FROM personal_access_token WHERE user_id = $1;

-- name: DeleteUserMemberships :exec
DELETE FROM member WHERE user_id = $1;

-- name: DeleteUserNotificationPreferences :exec
DELETE FROM notification_preference WHERE user_id = $1;

-- name: DeleteUserPinnedItems :exec
DELETE FROM pinned_item WHERE user_id = $1;

-- name: DeleteUserPasswordCredential :exec
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
DELETE FROM issue_subscriber WHERE user_id = $1;

-- name: DeleteUserComposioConnections :exec
DELETE FROM user_composio_connection WHERE user_id = $1;

-- name: DeleteUserSeatCapacityOutbox :exec
DELETE FROM seat_capacity_outbox WHERE user_id = $1;

-- name: DeleteUserChatPins :exec
DELETE FROM chat_pinned_agent WHERE user_id = $1;

-- name: DeleteUserAutopilotCollaborators :exec
DELETE FROM autopilot_collaborator WHERE user_id = $1;

-- name: DeleteUserAutopilotSubscribers :exec
DELETE FROM autopilot_subscriber WHERE user_id = $1;

-- name: DeleteUserIssueViewPreferences :exec
DELETE FROM issue_view_preference WHERE user_id = $1;

-- name: DeleteLocalPasswordCredentialByUserID :exec
DELETE FROM local_password_credential WHERE user_id = $1;

-- name: ListUserIDsWithLocalPassword :many
SELECT user_id FROM local_password_credential;
