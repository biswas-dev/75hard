-- Journaling, meditation reflections, hideable optional tasks, and per-account
-- AI credentials.

-- Optional tasks can be hidden rather than deleted.
--
-- Deleting one would take its history with it, and somebody who turns
-- journaling off in March should not lose February. A hidden task is excluded
-- from the day screen, the grid and every count, but its rows survive and
-- reappear intact if it is switched back on.
ALTER TABLE program_tasks ADD COLUMN hidden INTEGER NOT NULL DEFAULT 0;

-- A short note on how the sitting went.
--
-- Deliberately separate from the journal: this is a sentence about the
-- meditation itself, written while closing it off, and pushing it into the
-- day's journal entry would mix two different kinds of writing.
ALTER TABLE meditation_sessions ADD COLUMN reflection TEXT NOT NULL DEFAULT '';

-- Journal entries.
--
-- One entry can be typed directly or uploaded as a PDF. A handwritten page is
-- stored as the original file and, separately, as whatever text could be read
-- out of it — the two are kept apart because the transcription is a machine's
-- best guess and must never overwrite the thing somebody actually wrote.
CREATE TABLE journal_entries (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day_id      INTEGER REFERENCES days(id) ON DELETE SET NULL,
    title       TEXT    NOT NULL DEFAULT '',
    -- typed | pdf
    kind        TEXT    NOT NULL DEFAULT 'typed',
    -- What the person wrote, when they typed it.
    body        TEXT    NOT NULL DEFAULT '',
    -- Path relative to PHOTOS_DIR, for an uploaded file.
    rel_path    TEXT    NOT NULL DEFAULT '',
    file_name   TEXT    NOT NULL DEFAULT '',
    file_bytes  INTEGER NOT NULL DEFAULT 0,
    page_count  INTEGER NOT NULL DEFAULT 0,
    -- Text read out of an upload. Kept apart from body so a transcription can
    -- never be mistaken for, or overwrite, the original writing.
    parsed_text TEXT    NOT NULL DEFAULT '',
    -- '' when nothing is pending | pending | done | failed
    parse_status TEXT   NOT NULL DEFAULT '',
    parse_error  TEXT   NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_journal_user_day ON journal_entries(user_id, day_id);
CREATE INDEX idx_journal_created ON journal_entries(user_id, created_at DESC);
CREATE INDEX idx_journal_parse ON journal_entries(parse_status)
    WHERE parse_status IN ('pending', 'failed');

-- Full-text search over journals.
--
-- An external-content table would stay in step automatically but ties the
-- index to rowids that a future migration could disturb; this is a plain FTS
-- table kept current by triggers, which is more rows and far fewer surprises.
CREATE VIRTUAL TABLE journal_fts USING fts5(
    title,
    content,
    entry_id UNINDEXED,
    user_id  UNINDEXED,
    tokenize = 'porter unicode61'
);

-- The searchable text is the typed body and the transcription together: a
-- person looking for something they wrote does not care which it was.
CREATE TRIGGER journal_fts_insert AFTER INSERT ON journal_entries BEGIN
    INSERT INTO journal_fts (title, content, entry_id, user_id)
    VALUES (new.title, new.body || ' ' || new.parsed_text, new.id, new.user_id);
END;

CREATE TRIGGER journal_fts_update AFTER UPDATE ON journal_entries BEGIN
    DELETE FROM journal_fts WHERE entry_id = old.id;
    INSERT INTO journal_fts (title, content, entry_id, user_id)
    VALUES (new.title, new.body || ' ' || new.parsed_text, new.id, new.user_id);
END;

CREATE TRIGGER journal_fts_delete AFTER DELETE ON journal_entries BEGIN
    DELETE FROM journal_fts WHERE entry_id = old.id;
END;

-- Per-account AI credentials.
--
-- The server's own keys are a fallback for whoever runs the instance; anyone
-- else bringing their own key should be spending their own quota, not the
-- host's. Keys are encrypted at rest with AES-GCM and are never returned to
-- the client — the UI shows only the last four characters, which is enough to
-- recognise a key and useless for using one.
CREATE TABLE user_ai_providers (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    -- 1 is the primary, 2 the first backup, and so on: the go-ai slot order.
    slot        INTEGER NOT NULL,
    provider    TEXT    NOT NULL,
    model       TEXT    NOT NULL DEFAULT '',
    base_url    TEXT    NOT NULL DEFAULT '',
    -- AES-GCM ciphertext, base64. Never sent to a client.
    api_key_enc TEXT    NOT NULL DEFAULT '',
    -- Last four characters of the plaintext, for recognition only.
    key_hint    TEXT    NOT NULL DEFAULT '',
    enabled     INTEGER NOT NULL DEFAULT 1,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, slot)
);

CREATE INDEX idx_user_ai_providers_user ON user_ai_providers(user_id, slot);
