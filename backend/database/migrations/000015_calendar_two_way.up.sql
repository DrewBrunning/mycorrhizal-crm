-- T13 (docs/fork-plan/tickets/36-T13-two-way-calendar.md) — two-way calendar
-- sync: push local CRM edits back out to a subscribed remote calendar.
--
--   remote_etag — the remote calendar object's ETag, captured from the
--     CalDAV calendar-query response. Used as the If-Match precondition on
--     PUT so a both-changed conflict is detected rather than silently
--     overwriting an unseen remote edit.
--   remote_path — the remote calendar object's resource path, used to
--     address the same object on PUT. NULL for subscriptions fetched via the
--     plain-ICS GET fallback (no CalDAV collection -> no write path), which
--     therefore can never participate in two-way sync.
--
-- Both are additive and nullable; existing rows simply carry NULL until the
-- next sync populates them.

ALTER TABLE calendar_event_links ADD COLUMN remote_etag TEXT;
ALTER TABLE calendar_event_links ADD COLUMN remote_path TEXT;
