-- Web registration and instance administration (P5).
-- Invitations bind an existing user UUID and store only the SHA-256 digest of
-- the secret; the raw invite value never reaches this table.

-- name: CreateRegistrationInvite :one
INSERT INTO registration_invite (user_id, token_hash, created_by, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING *;

-- name: GetRegistrationInviteByTokenHash :one
SELECT * FROM registration_invite WHERE token_hash = $1;

-- name: GetActiveRegistrationInviteForUser :one
SELECT * FROM registration_invite
WHERE user_id = $1
  AND consumed_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > now()
ORDER BY created_at DESC
LIMIT 1;

-- name: RevokeRegistrationInvitesForUser :execrows
UPDATE registration_invite
SET revoked_at = now()
WHERE user_id = $1 AND consumed_at IS NULL AND revoked_at IS NULL;

-- name: ConsumeRegistrationInvite :execrows
UPDATE registration_invite
SET consumed_at = now()
WHERE id = $1
  AND consumed_at IS NULL
  AND revoked_at IS NULL
  AND expires_at > now();

-- name: DeleteRegistrationInvitesForUser :exec
DELETE FROM registration_invite WHERE user_id = $1 OR created_by = $1;

-- name: ListActiveRegistrationInvites :many
SELECT id, user_id, created_by, expires_at, created_at
FROM registration_invite
WHERE consumed_at IS NULL AND revoked_at IS NULL AND expires_at > now()
ORDER BY created_at DESC;

-- name: GetInstanceSetting :one
SELECT value FROM instance_setting WHERE key = $1;

-- name: UpsertInstanceSetting :one
INSERT INTO instance_setting (key, value)
VALUES ($1, $2)
ON CONFLICT (key) DO UPDATE SET value = EXCLUDED.value, updated_at = now()
RETURNING value;
