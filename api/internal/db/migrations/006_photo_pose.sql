-- Progress photos gain an angle, so front/side/back can be compared like with
-- like. Comparing a front shot against a side one shows nothing.
--
-- Deliberately nullable-by-default (empty string): the daily task is satisfied
-- by any one photo, and requiring a pose would make a streak depend on
-- remembering to tag it.

ALTER TABLE photos ADD COLUMN pose TEXT NOT NULL DEFAULT '';

-- The roll groups by day and filters by pose, so index the pair.
CREATE INDEX idx_photos_pose ON photos(user_id, kind, pose, taken_at DESC);
