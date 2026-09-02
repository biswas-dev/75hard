-- Strava connectivity, plus the daily vitals that Strava cannot supply.

-- One Strava athlete per user.
--
-- Tokens are stored as issued. They are bearer credentials for a third-party
-- account, so the row is deliberately narrow and the table is the only place
-- they live; revoking is a matter of deleting the row and disconnecting in
-- Strava. Access tokens expire every six hours and are refreshed on demand.
CREATE TABLE strava_accounts (
    user_id       INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    athlete_id    INTEGER NOT NULL,
    username      TEXT    NOT NULL DEFAULT '',
    access_token  TEXT    NOT NULL,
    refresh_token TEXT    NOT NULL,
    -- Unix seconds, as Strava returns it.
    expires_at    INTEGER NOT NULL DEFAULT 0,
    scope         TEXT    NOT NULL DEFAULT '',
    last_sync_at  DATETIME,
    -- Set when a sync fails, so a disconnected account is visible rather than
    -- silently importing nothing.
    last_error    TEXT    NOT NULL DEFAULT '',
    created_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

-- Imported activities.
--
-- strava_id is unique per user so a re-sync updates rather than duplicates —
-- activities are routinely edited after upload (renamed, re-typed, trimmed).
CREATE TABLE strava_activities (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    strava_id      INTEGER NOT NULL,
    day_id         INTEGER REFERENCES days(id) ON DELETE SET NULL,
    -- The workout row this created, so un-linking can clean up after itself.
    workout_id     INTEGER REFERENCES workouts(id) ON DELETE SET NULL,
    name           TEXT    NOT NULL DEFAULT '',
    sport_type     TEXT    NOT NULL DEFAULT '',
    -- indoor | outdoor, as decided by classifyActivity.
    kind           TEXT    NOT NULL DEFAULT 'outdoor',
    -- Strava's own flag: a treadmill or turbo session is never outdoors.
    trainer        INTEGER NOT NULL DEFAULT 0,
    commute        INTEGER NOT NULL DEFAULT 0,
    moving_seconds INTEGER NOT NULL DEFAULT 0,
    elapsed_seconds INTEGER NOT NULL DEFAULT 0,
    distance_m     REAL    NOT NULL DEFAULT 0,
    elevation_m    REAL    NOT NULL DEFAULT 0,
    kcal           REAL,
    average_hr     REAL,
    max_hr         REAL,
    average_speed  REAL,
    start_at       DATETIME NOT NULL,
    -- The activity's own local date, so an evening walk lands on the day it
    -- was walked rather than the UTC day it was uploaded.
    local_date     TEXT    NOT NULL DEFAULT '',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, strava_id)
);

CREATE INDEX idx_strava_activities_day ON strava_activities(day_id);
CREATE INDEX idx_strava_activities_user_date ON strava_activities(user_id, local_date);

-- Resting heart rate, entered by hand.
--
-- Strava's API does not expose it: average_heartrate and max_heartrate are
-- per-activity, and true resting HR lives in the device app (Garmin, Fitbit,
-- Apple Health) rather than in Strava. One optional number each morning, next
-- to the weight, is the only reliable way to see it trend over 75 days.
ALTER TABLE days ADD COLUMN resting_hr INTEGER;
