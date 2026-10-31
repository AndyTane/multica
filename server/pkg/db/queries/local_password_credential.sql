-- name: GetLocalPasswordCredentialByUsername :one
SELECT * FROM local_password_credential
WHERE username_normalized = $1;

-- name: GetLocalPasswordCredentialByUserID :one
SELECT * FROM local_password_credential
WHERE user_id = $1;

-- name: CreateLocalPasswordCredential :one
INSERT INTO local_password_credential (user_id, username_normalized, password_hash)
VALUES ($1, $2, $3)
RETURNING *;

-- name: UpdateLocalPasswordCredentialHash :one
-- Resets the password in place. The username is immutable here so a reset
-- cannot silently free a name another account already answers to.
UPDATE local_password_credential
SET password_hash = $2, updated_at = now()
WHERE username_normalized = $1
RETURNING *;

-- name: DeleteLocalPasswordCredentialByUsername :execrows
DELETE FROM local_password_credential
WHERE username_normalized = $1;

-- name: ListLocalPasswordCredentials :many
-- Local admin listing only: usernames and their target users, never hashes.
SELECT
    c.username_normalized,
    c.user_id,
    u.email,
    u.name,
    c.created_at,
    c.updated_at
FROM local_password_credential c
JOIN "user" u ON u.id = c.user_id
ORDER BY c.username_normalized;
