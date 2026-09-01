-- AI features: a run ledger that doubles as the quota counter and the cache.

CREATE TABLE ai_runs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- food_photo | recipes | plan | coach
    feature      TEXT    NOT NULL,
    provider     TEXT    NOT NULL DEFAULT '',
    model        TEXT    NOT NULL DEFAULT '',
    -- SHA-256 of the inputs. Identical inputs reuse the stored result rather
    -- than paying for the same answer twice.
    input_hash   TEXT    NOT NULL,
    result_json  TEXT    NOT NULL DEFAULT '',
    tokens_in    INTEGER NOT NULL DEFAULT 0,
    tokens_out   INTEGER NOT NULL DEFAULT 0,
    -- How many providers were tried, so a fallback is visible after the fact.
    attempts     INTEGER NOT NULL DEFAULT 1,
    error        TEXT    NOT NULL DEFAULT '',
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Cache lookups.
CREATE INDEX idx_ai_runs_cache ON ai_runs(user_id, feature, input_hash);
-- Quota: rows in the trailing 24 hours.
CREATE INDEX idx_ai_runs_quota ON ai_runs(user_id, created_at);
