-- Account-level serialization (P5-R1).
--
-- Writes that must not interleave take the same transaction-scoped advisory
-- lock for a logical key: 'instance_admins' for anything that reads the
-- administrator count, and 'user:<uuid>' for anything that mutates one
-- account's invitations or credentials. Keys are hashed by Postgres, so the
-- lock manager sees one int8 per key and no key can collide with another
-- subsystem's advisory locks by construction.

-- name: LockAccountKey :exec
SELECT pg_advisory_xact_lock(hashtextextended($1, 0));

-- name: GetUserForUpdate :one
-- Row lock for the account whose identity is being re-checked inside a write
-- transaction. A delete and a concurrent promote cannot both observe the
-- pre-delete state.
SELECT * FROM "user" WHERE id = $1 FOR UPDATE;
