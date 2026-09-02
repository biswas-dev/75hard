-- Personal API tokens, and the weight unit preference.

-- Tokens follow the go-api scheme, shared with folioworth: a per-application
-- prefix ("75h_") followed by 64 hex characters of CSPRNG output.
--
-- Only the SHA-256 of the token is stored. The plaintext is shown exactly once,
-- at creation, and is unrecoverable afterwards — a leaked database gives an
-- attacker hashes, not working credentials.
--
-- The prefix is the marker plus eight hex characters, kept in clear so a token
-- can be named and revoked from a list without storing the secret itself.
CREATE TABLE api_tokens (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- What it is for, so an unfamiliar token can be judged before revoking.
    name         TEXT    NOT NULL DEFAULT '',
    token_hash   TEXT    NOT NULL UNIQUE,
    prefix       TEXT    NOT NULL,
    -- Comma-separated: 'read' or 'read,write'. Postgres gives folioworth a
    -- TEXT[]; SQLite has no array type, and go-api's ParseScopes reads both.
    scopes       TEXT    NOT NULL DEFAULT 'read',
    last_used_at DATETIME,
    -- Optional expiry. NULL never expires.
    expires_at   DATETIME,
    revoked_at   DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_api_tokens_user ON api_tokens(user_id, created_at DESC);
-- The lookup on every token-authenticated request.
CREATE INDEX idx_api_tokens_hash ON api_tokens(token_hash) WHERE revoked_at IS NULL;

-- Display unit for weight.
--
-- Weight is always stored in kilograms; this only changes what is shown and
-- what an entered number is read as. Storing per-user units instead would mean
-- every chart, average and comparison had to know which rows were which.
ALTER TABLE users ADD COLUMN weight_unit TEXT NOT NULL DEFAULT 'kg';
