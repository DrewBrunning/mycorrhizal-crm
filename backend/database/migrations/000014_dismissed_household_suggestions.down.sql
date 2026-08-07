-- Reverses 000014. Destructive by nature: every address-suggestion dismissal
-- is discarded. Down is the only lossy direction — dismissed groups will be
-- re-offered on the next detection pass.

DROP TABLE dismissed_household_suggestions;
