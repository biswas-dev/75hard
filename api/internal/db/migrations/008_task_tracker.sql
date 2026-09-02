-- A task can optionally carry a tracker: a richer panel for people who want to
-- log detail, sitting behind a task that is still satisfied by a single tap.
--
-- "Follow the diet" is the motivating case. Some days you want to record every
-- meal and watch the calorie total; most days you ate on plan and want to say
-- so and move on. Both have to be first-class, so the tracker is additive and
-- never gates completion.
--
-- '' = no tracker | 'nutrition' = meals and calories | 'workout' = sessions

ALTER TABLE program_tasks ADD COLUMN tracker TEXT NOT NULL DEFAULT '';

-- Backfill the defaults so existing programs gain the panels without the user
-- editing anything.
UPDATE program_tasks SET tracker = 'nutrition' WHERE task_key = 'diet';
UPDATE program_tasks SET tracker = 'workout'
 WHERE task_key IN ('workout_indoor', 'workout_outdoor');
