-- Reverses 000015. The two-way sync columns are derived sync state — losing
-- them on rollback only means one push loses its If-Match precondition until
-- the next sync re-captures them; no user data is destroyed.

ALTER TABLE calendar_event_links DROP COLUMN remote_etag;
ALTER TABLE calendar_event_links DROP COLUMN remote_path;
