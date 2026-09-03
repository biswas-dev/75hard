-- Workouts are counted as sessions, not as an indoor and an outdoor bucket.
--
-- The rule is two workouts of forty-five minutes, starting at least two hours
-- apart, at least one of them outside. The old model held one required task
-- for an indoor session and one for an outdoor session, which quietly demanded
-- one of each: a morning walk followed by an outdoor swim satisfied nothing
-- but the outdoor task, and the day stayed short however much was done.
--
-- It also summed every workout of a kind, so three twenty-minute walks added
-- up to a forty-five minute workout. They are three walks.

-- The second task is no longer "the indoor one". Its key changes with it;
-- task_entries reference the task by id, so every day already logged keeps its
-- history through the rename.
UPDATE program_tasks
   SET task_key = 'workout_second',
       title    = 'Second 45-minute workout',
       detail   = 'Any location, starting at least 2 hours after the first.'
 WHERE task_key = 'workout_indoor';

UPDATE program_tasks
   SET detail = 'At least 45 minutes outside, in one session.'
 WHERE task_key = 'workout_outdoor';

-- Give the workouts a start time so they can be grouped into sessions.
--
-- Imported activities carry Strava's own start. Hand-logged rows have none and
-- keep none: a record with no time joins the day's most recent session, which
-- is where a figure added by hand almost always belongs, and inventing a time
-- from created_at would place a treadmill leg logged the following evening
-- hours away from the workout it was topping up.
UPDATE workouts
   SET started_at = (
        SELECT sa.start_at FROM strava_activities sa WHERE sa.workout_id = workouts.id
   )
 WHERE started_at IS NULL
   AND EXISTS (SELECT 1 FROM strava_activities sa WHERE sa.workout_id = workouts.id);
