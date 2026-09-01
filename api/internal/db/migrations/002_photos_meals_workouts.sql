-- Photos, food logging and workouts.

CREATE TABLE photos (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    program_id  INTEGER REFERENCES programs(id) ON DELETE SET NULL,
    day_id      INTEGER REFERENCES days(id) ON DELETE SET NULL,
    -- progress | food | ingredients
    kind        TEXT    NOT NULL DEFAULT 'progress',
    -- Paths relative to PHOTOS_DIR, so the volume can move without a rewrite.
    rel_path    TEXT    NOT NULL,
    thumb_path  TEXT    NOT NULL DEFAULT '',
    mime        TEXT    NOT NULL,
    width       INTEGER NOT NULL DEFAULT 0,
    height      INTEGER NOT NULL DEFAULT 0,
    bytes       INTEGER NOT NULL DEFAULT 0,
    -- Of the stored bytes; lets a re-upload of the same shot reuse the row.
    sha256      TEXT    NOT NULL DEFAULT '',
    caption     TEXT    NOT NULL DEFAULT '',
    taken_at    DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_photos_user_kind ON photos(user_id, kind, taken_at DESC);
CREATE INDEX idx_photos_day ON photos(day_id);
CREATE INDEX idx_photos_sha ON photos(user_id, sha256);

CREATE TABLE meals (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day_id     INTEGER NOT NULL REFERENCES days(id) ON DELETE CASCADE,
    photo_id   INTEGER REFERENCES photos(id) ON DELETE SET NULL,
    name       TEXT    NOT NULL DEFAULT '',
    -- breakfast | lunch | dinner | snack
    slot       TEXT    NOT NULL DEFAULT 'snack',
    kcal       REAL    NOT NULL DEFAULT 0,
    protein_g  REAL    NOT NULL DEFAULT 0,
    carbs_g    REAL    NOT NULL DEFAULT 0,
    fat_g      REAL    NOT NULL DEFAULT 0,
    -- manual | ai — so an AI estimate is never mistaken for a weighed entry.
    source     TEXT    NOT NULL DEFAULT 'manual',
    notes      TEXT    NOT NULL DEFAULT '',
    eaten_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_meals_day ON meals(day_id);

CREATE TABLE meal_items (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    meal_id    INTEGER NOT NULL REFERENCES meals(id) ON DELETE CASCADE,
    name       TEXT    NOT NULL,
    qty        REAL    NOT NULL DEFAULT 1,
    unit       TEXT    NOT NULL DEFAULT 'serving',
    kcal       REAL    NOT NULL DEFAULT 0,
    protein_g  REAL    NOT NULL DEFAULT 0,
    carbs_g    REAL    NOT NULL DEFAULT 0,
    fat_g      REAL    NOT NULL DEFAULT 0,
    confidence REAL,
    sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE INDEX idx_meal_items_meal ON meal_items(meal_id);

CREATE TABLE workouts (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    day_id     INTEGER NOT NULL REFERENCES days(id) ON DELETE CASCADE,
    -- indoor | outdoor — the canonical rules require one of each per day.
    kind       TEXT    NOT NULL DEFAULT 'indoor',
    activity   TEXT    NOT NULL DEFAULT '',
    minutes    INTEGER NOT NULL DEFAULT 0,
    kcal       REAL,
    notes      TEXT    NOT NULL DEFAULT '',
    started_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_workouts_day ON workouts(day_id);
