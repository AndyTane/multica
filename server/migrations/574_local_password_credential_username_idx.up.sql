-- One canonical username per credential. Concurrent build, own single-statement
-- migration file (AGENTS.md database rules).
CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS local_password_credential_username_idx
    ON local_password_credential (username_normalized);
