-- Reverses 000016. Destructive by nature: every audit event is discarded and
-- the immutability trigger is dropped. Audit history is unrecoverable after
-- this — the ticket's own cost-of-deferring note applies in reverse.

DROP TRIGGER audit_events_no_update;
DROP TABLE audit_events;
