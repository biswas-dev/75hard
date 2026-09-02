-- Each task gets its own colour so the per-activity grids are distinguishable
-- at a glance, and the colour is editable per task.

ALTER TABLE program_tasks ADD COLUMN color TEXT NOT NULL DEFAULT '';

-- Backfill by position so existing programs get a varied palette rather than
-- six identical grids. Matches the default palette the client offers.
UPDATE program_tasks SET color = CASE sort_order % 6
    WHEN 0 THEN '#ff6b35'
    WHEN 1 THEN '#37d67a'
    WHEN 2 THEN '#4a9eff'
    WHEN 3 THEN '#ffd166'
    WHEN 4 THEN '#b47aea'
    ELSE '#ff5d8f'
END
WHERE color = '';
