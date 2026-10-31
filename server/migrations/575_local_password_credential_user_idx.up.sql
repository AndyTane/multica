-- Exactly one credential per user: the admin tool resets a password in place
-- instead of accumulating credentials. Separate concurrent index migration file.
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS local_password_credential_user_idx
    ON local_password_credential (user_id);
