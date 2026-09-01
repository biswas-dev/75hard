-- OAuth support for go-login. Mirrors taskai migration 040: an auth_provider
-- column distinguishing password from OAuth users, and a link table.
--
-- UNIQUE(user_id, provider) is what makes go-login's LinkOAuthProvider
-- upsert work.

ALTER TABLE users ADD COLUMN auth_provider TEXT NOT NULL DEFAULT 'password';

CREATE TABLE oauth_providers (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id          INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    provider         TEXT NOT NULL,
    provider_user_id TEXT NOT NULL,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(provider, provider_user_id),
    UNIQUE(user_id, provider)
);

CREATE INDEX idx_oauth_providers_user_id ON oauth_providers(user_id);
CREATE INDEX idx_oauth_providers_lookup  ON oauth_providers(provider, provider_user_id);
