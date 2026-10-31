CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS registration_invite_token_hash_idx ON registration_invite (token_hash);
