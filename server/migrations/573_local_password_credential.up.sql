-- Local password credentials for deployments where the host administrator
-- provisions accounts. One credential per user; the username is stored in its
-- normalized form (see internal/auth.NormalizeLocalUsername) and is matched
-- case-insensitively.
--
-- Deliberately no foreign key on user_id: this repo does not add foreign keys
-- (see AGENTS.md). The login handler treats a credential whose user row is gone
-- as a failed login, and the admin tool refuses to create credentials for
-- missing users.
CREATE TABLE local_password_credential (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    username_normalized TEXT NOT NULL,
    password_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
