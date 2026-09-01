-- Core schema: users, programs, the editable task template, days and entries.

CREATE TABLE users (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    email          TEXT    NOT NULL,
    password_hash  TEXT    NOT NULL,
    name           TEXT    NOT NULL DEFAULT '',
    avatar_url     TEXT    NOT NULL DEFAULT '',
    -- IANA name. "Today" is always computed in the user's zone, never the
    -- server's, or a 11pm check-off lands on the wrong day.
    timezone       TEXT    NOT NULL DEFAULT 'UTC',
    is_admin       INTEGER NOT NULL DEFAULT 0,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at     DATETIME
);

CREATE UNIQUE INDEX idx_users_email ON users(lower(email));

-- A single attempt at the challenge. A user may have many attempts over time
-- but only one active at once.
CREATE TABLE programs (
    id                  INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name                TEXT    NOT NULL DEFAULT '75 Hard',
    -- Local calendar date (YYYY-MM-DD) of day 1, in the user's timezone.
    start_date          TEXT    NOT NULL,
    length_days         INTEGER NOT NULL DEFAULT 75,
    -- active | completed | failed | abandoned
    status              TEXT    NOT NULL DEFAULT 'active',
    -- 1 = a missed day ends the attempt and restarts at day 1 (canonical rules)
    -- 0 = a miss is recorded but the attempt continues
    strict_restart      INTEGER NOT NULL DEFAULT 1,
    -- Which attempt this one restarted from, so the history stays linked.
    previous_attempt_id INTEGER REFERENCES programs(id) ON DELETE SET NULL,
    attempt_number      INTEGER NOT NULL DEFAULT 1,
    daily_kcal_target   INTEGER,
    notes               TEXT    NOT NULL DEFAULT '',
    created_at          DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at            DATETIME
);

CREATE INDEX idx_programs_user ON programs(user_id, status);

-- The editable task template for a program. Copied from the canonical six at
-- creation time, then the user may add, edit, reorder or remove tasks.
CREATE TABLE program_tasks (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    program_id  INTEGER NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    task_key    TEXT    NOT NULL,
    title       TEXT    NOT NULL,
    detail      TEXT    NOT NULL DEFAULT '',
    icon        TEXT    NOT NULL DEFAULT 'check',
    -- check | number | duration | photo | text
    kind        TEXT    NOT NULL DEFAULT 'check',
    -- For number/duration: the amount that counts as done (e.g. 128 oz, 45 min).
    target_num  REAL,
    unit        TEXT    NOT NULL DEFAULT '',
    sort_order  INTEGER NOT NULL DEFAULT 0,
    required    INTEGER NOT NULL DEFAULT 1,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_program_tasks_key ON program_tasks(program_id, task_key);
CREATE INDEX idx_program_tasks_order ON program_tasks(program_id, sort_order);

-- One row per calendar day of the attempt, materialised lazily on first access.
CREATE TABLE days (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    program_id   INTEGER NOT NULL REFERENCES programs(id) ON DELETE CASCADE,
    day_number   INTEGER NOT NULL,
    on_date      TEXT    NOT NULL,
    -- pending | complete | missed
    status       TEXT    NOT NULL DEFAULT 'pending',
    note         TEXT    NOT NULL DEFAULT '',
    weight_kg    REAL,
    completed_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_days_program_number ON days(program_id, day_number);
CREATE UNIQUE INDEX idx_days_program_date ON days(program_id, on_date);

-- A task ticked (or partially progressed) on a given day.
CREATE TABLE task_entries (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    day_id          INTEGER NOT NULL REFERENCES days(id) ON DELETE CASCADE,
    program_task_id INTEGER NOT NULL REFERENCES program_tasks(id) ON DELETE CASCADE,
    -- NULL while a number-kind task is still short of its target.
    completed_at    DATETIME,
    value_num       REAL,
    note            TEXT    NOT NULL DEFAULT '',
    created_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_task_entries_unique ON task_entries(day_id, program_task_id);
