-- Rollback of 000060. The resolution memory is derived from user actions on
-- candidates that can always be re-offered; dropping it loses only the "do not
-- ask again" state, not user-authored content. The table itself is new, so
-- there is no existing-data-preservation concern.
DROP TABLE IF EXISTS life_event_suggestion_resolutions;
