-- Freeze hand-logged workouts into the session they were logged against.
--
-- A workout with no start time joins the day's most recent session, which was
-- right when it was written and wrong ever afterwards: the day's most recent
-- session changes as more training is logged, so a ten-minute stretch added to
-- top up a morning walk quietly moved to the evening session that arrived
-- hours later, and the walk fell back below its target. The day had been
-- complete and then was not, with nothing to show why.
--
-- The session it was logged against is still recoverable: it is the latest
-- session that existed when the row was created. Writing that down as a real
-- start time makes it stay there.
UPDATE workouts
   SET started_at = (
        SELECT MAX(o.started_at)
          FROM workouts o
         WHERE o.day_id = workouts.day_id
           AND o.started_at IS NOT NULL
           AND o.created_at <= workouts.created_at
   )
 WHERE started_at IS NULL
   AND EXISTS (
        SELECT 1 FROM workouts o
         WHERE o.day_id = workouts.day_id
           AND o.started_at IS NOT NULL
           AND o.created_at <= workouts.created_at
   );

-- Give existing programs the optional journal task.
--
-- It was added to the template after these programs were created, so the
-- journalling built for them had nowhere to appear on the day screen.
INSERT INTO program_tasks (program_id, task_key, title, detail, icon, kind, unit, required, sort_order, tracker)
SELECT p.id, 'journal', 'Journal',
       'Optional. Type an entry or upload a page — this never fails the challenge.',
       'pen', 'check', '', 0,
       (SELECT COALESCE(MAX(sort_order), 0) + 1 FROM program_tasks WHERE program_id = p.id),
       'journal'
  FROM programs p
 WHERE NOT EXISTS (
        SELECT 1 FROM program_tasks t WHERE t.program_id = p.id AND t.task_key = 'journal'
 );
