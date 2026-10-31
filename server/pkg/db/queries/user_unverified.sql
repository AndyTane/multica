-- name: CreateUnverifiedUser :one
-- Open self-registration: the account exists and can sign in, but the address
-- is only a claim until some flow proves control of the mailbox.
INSERT INTO "user" (name, email, email_verified)
VALUES ($1, $2, FALSE)
RETURNING *;
