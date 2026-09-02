-- Background AI estimation for food photos.
--
-- Photographing a meal should cost one tap. Waiting thirty seconds for a
-- vision model to answer before the app acknowledges the photo does not,
-- so the meal row is written immediately and the estimate fills in behind it.
-- The status column is what lets the UI show that honestly rather than
-- displaying a zero-calorie meal as if it were a real reading.
--
-- '' = nothing pending (a manual entry) | pending | done | failed
ALTER TABLE meals ADD COLUMN estimate_status TEXT NOT NULL DEFAULT '';

-- Why a failed estimate failed, so it can be retried or explained rather than
-- silently sitting empty.
ALTER TABLE meals ADD COLUMN estimate_error TEXT NOT NULL DEFAULT '';

-- Finding work to resume after a restart, and polling from the client.
CREATE INDEX idx_meals_estimate_status ON meals(estimate_status)
    WHERE estimate_status IN ('pending', 'failed');
