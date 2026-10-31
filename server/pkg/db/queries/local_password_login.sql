-- Password login state read as ONE row (P5-R1).
--
-- The credential hash and the account auth_version must come from the same
-- snapshot. Reading the hash first and the user row afterwards let a password
-- change commit in between: the request then verified the OLD hash but minted
-- a token carrying the NEW version, which is exactly the stale-credential
-- upgrade this query removes. The version used for signing is the version that
-- was current when the hash being compared was current, so a concurrent change
-- either fails the comparison or invalidates the signed token.

-- name: GetLocalPasswordLoginState :one
SELECT c.password_hash,
       u.id AS user_id,
       u.name,
       u.email,
       u.auth_version
FROM local_password_credential c
JOIN "user" u ON u.id = c.user_id
WHERE c.username_normalized = $1;

-- name: UpdateLocalPasswordCredentialHashConditional :execrows
-- Conditional credential rotation: the update only applies while the stored
-- hash is still the one the caller verified, so of two concurrent requests
-- using the same old password exactly one wins and the loser reports a stale
-- current password instead of silently replacing the winner's hash.
UPDATE local_password_credential
SET password_hash = sqlc.arg('new_hash'), updated_at = now()
WHERE user_id = sqlc.arg('user_id') AND password_hash = sqlc.arg('old_hash');
