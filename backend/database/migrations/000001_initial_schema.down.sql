-- Mycorrhizal CRM — squashed baseline rollback (T22).
-- Drops every entity in reverse dependency order.

DROP TABLE IF EXISTS webhook_deliveries;
DROP TABLE IF EXISTS webhooks;

DROP TABLE IF EXISTS preferences;

DROP TABLE IF EXISTS field_values;
DROP TABLE IF EXISTS field_definitions;

DROP TABLE IF EXISTS life_events;

DROP TABLE IF EXISTS contact_tags;
DROP TABLE IF EXISTS tags;

DROP TABLE IF EXISTS circle_members;
DROP TABLE IF EXISTS circles;

DROP TABLE IF EXISTS household_members;
DROP TABLE IF EXISTS households;

DROP TABLE IF EXISTS relationship_edges;

DROP TABLE IF EXISTS contact_sync_links;
DROP TABLE IF EXISTS contact_subscriptions;

DROP TABLE IF EXISTS calendar_event_links;
DROP TABLE IF EXISTS calendar_subscriptions;

DROP TABLE IF EXISTS carddav_sync;

DROP TABLE IF EXISTS job_executions;

DROP TABLE IF EXISTS reminder_completions;
DROP TABLE IF EXISTS reminders;

DROP TABLE IF EXISTS notes;

DROP TABLE IF EXISTS api_tokens;

DROP TABLE IF EXISTS activity_contacts;
DROP TABLE IF EXISTS activities;

DROP TABLE IF EXISTS contacts;
DROP TABLE IF EXISTS users;
