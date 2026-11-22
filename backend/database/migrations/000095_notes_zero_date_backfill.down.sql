-- 000095 only replaced the zero time (0001-01-01, which carried no
-- information) with the note's created_at. There is nothing to restore, and
-- putting the bad date back would re-break the timeline, so the down is a
-- deliberate no-op.
SELECT 1;
