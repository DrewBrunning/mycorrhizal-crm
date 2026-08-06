-- T36 (docs/fork-plan/tickets/45-T36-life-event-categories.md) — life event
-- categories + expanded default types.
--
-- Additive nullable column: LifeEvent.Type stays the existing unvalidated
-- open string (CLAUDE.md's "conventional, unvalidated open string"
-- convention); Category is new and closed
-- (home_living|health_wellness|work_education|travel_experiences|
-- family_relationships), enforced by LifeEventInput's `oneof` validator tag
-- rather than a SQL CHECK constraint, consistent with how Source (also
-- oneof-validated) is left unconstrained at this layer.
--
-- Backfill: only the 7 pre-existing LifeEventType* constants
-- (models/life_event_type_registry.go's LifeEventTypeCategories) have a
-- reliable category. Free-text Type values that predate this ticket have no
-- way to infer a category and are deliberately left NULL — the frontend
-- picker must offer an "Other / Uncategorized" bucket for these rather than
-- guessing, per the ticket's own trap note.
ALTER TABLE life_events ADD COLUMN category TEXT;

UPDATE life_events SET category = 'home_living' WHERE type = 'moved';
UPDATE life_events SET category = 'work_education' WHERE type IN ('job_change', 'retired', 'graduated');
UPDATE life_events SET category = 'family_relationships' WHERE type IN ('married', 'had_child', 'adopted_pet');
