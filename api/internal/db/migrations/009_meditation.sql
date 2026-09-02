-- Optional meditation tracking.
--
-- Deliberately not part of the challenge: 75 Hard has six rules and meditation
-- is not one of them, so this task is created with required = 0 and can never
-- fail a run. It is here because the habit fits the same daily rhythm, and
-- tracking it beside the rest beats tracking it somewhere else.

CREATE TABLE meditation_sessions (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day_id     INTEGER NOT NULL REFERENCES days(id) ON DELETE CASCADE,
    minutes    INTEGER NOT NULL DEFAULT 0,
    -- Free text rather than an enum: the app someone uses is their business,
    -- and a closed list would be wrong within a year. The client offers the
    -- common ones (Calm, Headspace, Waking Up, Muse, Insight Timer) as
    -- shortcuts and accepts anything else typed in.
    source     TEXT    NOT NULL DEFAULT '',
    -- guided | unguided | breathwork | body_scan | walking | other
    style      TEXT    NOT NULL DEFAULT 'guided',
    notes      TEXT    NOT NULL DEFAULT '',
    started_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_meditation_day ON meditation_sessions(day_id);
CREATE INDEX idx_meditation_user_source ON meditation_sessions(user_id, source);

-- Add the optional task to every existing program that does not have it, so
-- people mid-run gain the panel without editing anything. It sorts last and
-- carries required = 0, so nobody's streak changes.
INSERT INTO program_tasks (program_id, task_key, title, detail, icon, kind, unit, sort_order, required, color, tracker)
SELECT p.id,
       'meditation',
       'Meditate',
       'Optional. Log how long and where — this never fails the challenge.',
       'lotus',
       'check',
       '',
       (SELECT COALESCE(MAX(sort_order), -1) + 1 FROM program_tasks WHERE program_id = p.id),
       0,
       '#7dd3fc',
       'meditation'
FROM programs p
WHERE NOT EXISTS (
    SELECT 1 FROM program_tasks t WHERE t.program_id = p.id AND t.task_key = 'meditation'
);
