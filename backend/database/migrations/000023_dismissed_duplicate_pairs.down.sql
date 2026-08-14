-- Reverses 000023. Destructive by nature: every duplicate-pair dismissal is
-- discarded. Down is the only lossy direction — dismissed pairs will be
-- re-offered on the next scan.

DROP TABLE dismissed_duplicate_pairs;
