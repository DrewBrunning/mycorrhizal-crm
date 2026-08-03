-- Mycorrhizal CRM — squashed baseline schema (T22).
-- Incorporates all schema decisions through pre-alpha: T26 partial index,
-- T5 life_events.deleted_at, T20a preferences table. Legacy columns
-- (food_preference, legacy_relationship_id) and one-shot backfill tools
-- are removed from this baseline — there is no production data.

PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

-- ---------------------------------------------------------------------------
-- users
-- ---------------------------------------------------------------------------
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    username TEXT UNIQUE NOT NULL,
    password TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    password_reset_token_hash TEXT,
    password_reset_expires_at DATETIME,
    password_reset_requested_at DATETIME,
    language TEXT DEFAULT 'en',
    is_admin INTEGER DEFAULT 0,
    date_format TEXT DEFAULT 'eu',
    oidc_subject TEXT,
    oidc_provider TEXT,
    enabled_contact_fields TEXT DEFAULT NULL,
    token_version INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_users_deleted_at ON users(deleted_at);
CREATE INDEX idx_users_email ON users(email);
CREATE UNIQUE INDEX idx_users_oidc_subject ON users(oidc_subject, oidc_provider)
    WHERE oidc_subject IS NOT NULL;
CREATE INDEX idx_users_password_reset_token_hash ON users(password_reset_token_hash);
CREATE INDEX idx_users_username ON users(username);

-- ---------------------------------------------------------------------------
-- contacts
-- ---------------------------------------------------------------------------
CREATE TABLE contacts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    firstname TEXT NOT NULL COLLATE NOCASE,
    lastname TEXT COLLATE NOCASE,
    nickname TEXT COLLATE NOCASE,
    gender TEXT,
    email TEXT COLLATE NOCASE,
    phone TEXT,
    birthday TEXT,
    photo TEXT,
    photo_thumbnail TEXT,
    address TEXT,
    how_we_met TEXT,
    work_information TEXT,
    contact_information TEXT,
    circles TEXT,
    user_id INTEGER,
    vcard_uid TEXT,
    vcard_extra TEXT,
    etag TEXT,
    archived INTEGER DEFAULT 0 NOT NULL,
    emails TEXT DEFAULT '[]',
    phones TEXT DEFAULT '[]',
    addresses TEXT DEFAULT '[]',
    urls TEXT DEFAULT '[]',
    impps TEXT DEFAULT '[]',
    prefix TEXT DEFAULT '',
    middle_name TEXT DEFAULT '',
    suffix TEXT DEFAULT '',
    organization TEXT DEFAULT '',
    department TEXT DEFAULT '',
    job_title TEXT DEFAULT '',
    role TEXT DEFAULT '',
    anniversary TEXT DEFAULT '',
    card TEXT DEFAULT '{}',
    crm TEXT DEFAULT '{}',
    passthrough TEXT DEFAULT '{}',
    fn TEXT DEFAULT '',
    org TEXT DEFAULT ''
);
CREATE INDEX idx_contacts_deleted_at ON contacts(deleted_at);
CREATE INDEX idx_contacts_email ON contacts(email COLLATE NOCASE);
CREATE INDEX idx_contacts_feed ON contacts(user_id, updated_at, id);
CREATE INDEX idx_contacts_firstname ON contacts(firstname COLLATE NOCASE);
CREATE INDEX idx_contacts_lastname ON contacts(lastname COLLATE NOCASE);
CREATE INDEX idx_contacts_user_id ON contacts(user_id);
CREATE UNIQUE INDEX idx_contacts_vcard_uid_user
    ON contacts(user_id, vcard_uid)
    WHERE vcard_uid IS NOT NULL AND deleted_at IS NULL;

-- ---------------------------------------------------------------------------
-- activities
-- ---------------------------------------------------------------------------
CREATE TABLE activities (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    title TEXT NOT NULL,
    description TEXT,
    location TEXT,
    date DATETIME NOT NULL,
    user_id INTEGER,
    uuid TEXT,
    type TEXT,
    external_ref TEXT,
    etag TEXT
);
CREATE INDEX idx_activities_date ON activities(date);
CREATE INDEX idx_activities_deleted_at ON activities(deleted_at);
CREATE INDEX idx_activities_feed ON activities(user_id, updated_at, id);
CREATE INDEX idx_activities_user_id ON activities(user_id);
CREATE INDEX idx_activities_uuid ON activities(uuid);

-- ---------------------------------------------------------------------------
-- activity_contacts
-- ---------------------------------------------------------------------------
CREATE TABLE activity_contacts (
    activity_id INTEGER NOT NULL,
    contact_id INTEGER NOT NULL,
    PRIMARY KEY (activity_id, contact_id),
    FOREIGN KEY (activity_id) REFERENCES activities(id) ON DELETE CASCADE,
    FOREIGN KEY (contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);
CREATE INDEX idx_activity_contacts_activity_id ON activity_contacts(activity_id);
CREATE INDEX idx_activity_contacts_contact_id ON activity_contacts(contact_id);

-- ---------------------------------------------------------------------------
-- api_tokens
-- ---------------------------------------------------------------------------
CREATE TABLE api_tokens (
    id           INTEGER  PRIMARY KEY AUTOINCREMENT,
    created_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at   DATETIME,
    user_id      INTEGER  NOT NULL,
    name         TEXT     NOT NULL,
    token_hash   TEXT     NOT NULL UNIQUE,
    last_used_at DATETIME,
    revoked_at   DATETIME,
    expires_at   DATETIME,
    scope        TEXT     NOT NULL DEFAULT 'full',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_api_tokens_expires_at ON api_tokens(expires_at);
CREATE INDEX idx_api_tokens_token_hash ON api_tokens(token_hash);
CREATE INDEX idx_api_tokens_user_id ON api_tokens(user_id);

-- ---------------------------------------------------------------------------
-- notes
-- ---------------------------------------------------------------------------
CREATE TABLE notes (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    content TEXT NOT NULL,
    date DATETIME NOT NULL,
    contact_id INTEGER,
    user_id INTEGER,
    FOREIGN KEY (contact_id) REFERENCES contacts(id) ON UPDATE CASCADE ON DELETE CASCADE
);
CREATE INDEX idx_notes_contact_id ON notes(contact_id);
CREATE INDEX idx_notes_date ON notes(date);
CREATE INDEX idx_notes_deleted_at ON notes(deleted_at);
CREATE INDEX idx_notes_feed ON notes(user_id, updated_at, id);
CREATE INDEX idx_notes_user_id ON notes(user_id);

-- ---------------------------------------------------------------------------
-- reminders
-- ---------------------------------------------------------------------------
CREATE TABLE reminders (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    message TEXT NOT NULL,
    by_mail INTEGER DEFAULT 0,
    remind_at DATETIME NOT NULL,
    recurrence TEXT NOT NULL,
    reoccur_from_completion INTEGER DEFAULT 1,
    last_sent DATETIME,
    contact_id INTEGER NOT NULL,
    completed BOOLEAN DEFAULT false NOT NULL,
    user_id INTEGER,
    email_sent BOOLEAN DEFAULT FALSE NOT NULL,
    life_event_id TEXT,
    FOREIGN KEY (contact_id) REFERENCES contacts(id) ON UPDATE CASCADE ON DELETE CASCADE
);
CREATE INDEX idx_reminders_contact_id ON reminders(contact_id);
CREATE INDEX idx_reminders_deleted_at ON reminders(deleted_at);
CREATE INDEX idx_reminders_life_event_id ON reminders(life_event_id);
CREATE INDEX idx_reminders_remind_at ON reminders(remind_at);
CREATE INDEX idx_reminders_user_id ON reminders(user_id);

-- ---------------------------------------------------------------------------
-- reminder_completions
-- ---------------------------------------------------------------------------
CREATE TABLE reminder_completions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    reminder_id INTEGER,
    contact_id INTEGER NOT NULL,
    message TEXT NOT NULL,
    completed_at DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (contact_id) REFERENCES contacts(id) ON DELETE CASCADE
);
CREATE INDEX idx_reminder_completions_contact_id ON reminder_completions(contact_id);
CREATE INDEX idx_reminder_completions_user_id ON reminder_completions(user_id);

-- ---------------------------------------------------------------------------
-- job_executions
-- ---------------------------------------------------------------------------
CREATE TABLE job_executions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    job_name TEXT UNIQUE NOT NULL,
    last_run_at DATETIME NOT NULL,
    locked_at DATETIME,
    locked_by TEXT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME
);
CREATE INDEX idx_job_executions_deleted_at ON job_executions(deleted_at);
CREATE UNIQUE INDEX idx_job_executions_job_name ON job_executions(job_name);

-- ---------------------------------------------------------------------------
-- carddav_sync
-- ---------------------------------------------------------------------------
CREATE TABLE carddav_sync (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL UNIQUE,
    sync_token TEXT NOT NULL,
    last_modified DATETIME NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_carddav_sync_user ON carddav_sync(user_id);

-- ---------------------------------------------------------------------------
-- calendar_subscriptions
-- ---------------------------------------------------------------------------
CREATE TABLE calendar_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    username TEXT NOT NULL DEFAULT '',
    password_encrypted TEXT NOT NULL DEFAULT '',
    sync_enabled INTEGER NOT NULL DEFAULT 1,
    past_days INTEGER NOT NULL DEFAULT 5,
    future_days INTEGER NOT NULL DEFAULT 10,
    last_synced_at DATETIME,
    last_sync_status TEXT NOT NULL DEFAULT '',
    last_sync_error TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_calendar_subscriptions_deleted_at ON calendar_subscriptions(deleted_at);
CREATE INDEX idx_calendar_subscriptions_user_id ON calendar_subscriptions(user_id);

-- ---------------------------------------------------------------------------
-- calendar_event_links
-- ---------------------------------------------------------------------------
CREATE TABLE calendar_event_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME, updated_at DATETIME,
    subscription_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    uid TEXT NOT NULL,
    activity_id INTEGER NOT NULL,
    content_hash TEXT NOT NULL,
    FOREIGN KEY (subscription_id) REFERENCES calendar_subscriptions(id) ON DELETE CASCADE
);
CREATE INDEX idx_calendar_event_links_activity_id ON calendar_event_links(activity_id);
CREATE UNIQUE INDEX idx_calendar_event_links_sub_uid ON calendar_event_links(subscription_id, uid);
CREATE INDEX idx_calendar_event_links_user_id ON calendar_event_links(user_id);

-- ---------------------------------------------------------------------------
-- contact_subscriptions
-- ---------------------------------------------------------------------------
CREATE TABLE contact_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    username TEXT NOT NULL DEFAULT '',
    password_encrypted TEXT NOT NULL DEFAULT '',
    sync_enabled INTEGER NOT NULL DEFAULT 1,
    sync_token TEXT NOT NULL DEFAULT '',
    last_synced_at DATETIME,
    last_sync_status TEXT NOT NULL DEFAULT '',
    last_sync_error TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_contact_subscriptions_deleted_at ON contact_subscriptions(deleted_at);
CREATE INDEX idx_contact_subscriptions_user_id ON contact_subscriptions(user_id);

-- ---------------------------------------------------------------------------
-- contact_sync_links
-- ---------------------------------------------------------------------------
CREATE TABLE contact_sync_links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME, updated_at DATETIME,
    subscription_id INTEGER NOT NULL,
    user_id INTEGER NOT NULL,
    href TEXT NOT NULL,
    contact_id INTEGER NOT NULL,
    etag TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL,
    FOREIGN KEY (subscription_id) REFERENCES contact_subscriptions(id) ON DELETE CASCADE
);
CREATE INDEX idx_contact_sync_links_contact_id ON contact_sync_links(contact_id);
CREATE UNIQUE INDEX idx_contact_sync_links_sub_href ON contact_sync_links(subscription_id, href);
CREATE INDEX idx_contact_sync_links_user_id ON contact_sync_links(user_id);

-- ---------------------------------------------------------------------------
-- relationship_edges
-- ---------------------------------------------------------------------------
CREATE TABLE relationship_edges (
    id TEXT PRIMARY KEY,
    created_at DATETIME, updated_at DATETIME,
    user_id INTEGER NOT NULL,
    source_id TEXT NOT NULL,
    target_id TEXT NOT NULL,
    type TEXT NOT NULL,
    directional INTEGER NOT NULL DEFAULT 0,
    metadata TEXT,
    source TEXT NOT NULL,
    confidence REAL NOT NULL DEFAULT 1,
    status TEXT NOT NULL,
    sensitivity TEXT NOT NULL DEFAULT 'normal',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_relationship_edges_feed ON relationship_edges(user_id, updated_at, id);
CREATE INDEX idx_relationship_edges_sensitivity ON relationship_edges(sensitivity);
CREATE INDEX idx_relationship_edges_source_id ON relationship_edges(source_id);
CREATE INDEX idx_relationship_edges_status ON relationship_edges(status);
CREATE INDEX idx_relationship_edges_target_id ON relationship_edges(target_id);
CREATE INDEX idx_relationship_edges_type ON relationship_edges(type);
CREATE INDEX idx_relationship_edges_user_id ON relationship_edges(user_id);

-- ---------------------------------------------------------------------------
-- households
-- ---------------------------------------------------------------------------
CREATE TABLE households (
    id TEXT PRIMARY KEY,
    created_at DATETIME, updated_at DATETIME,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    type TEXT NOT NULL,
    address TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_households_feed ON households(user_id, updated_at, id);
CREATE INDEX idx_households_user_id ON households(user_id);

-- ---------------------------------------------------------------------------
-- household_members
-- ---------------------------------------------------------------------------
CREATE TABLE household_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME, updated_at DATETIME,
    household_id TEXT NOT NULL,
    user_id INTEGER NOT NULL,
    member_vcard_uid TEXT NOT NULL,
    role TEXT,
    since TEXT,
    until TEXT,
    FOREIGN KEY (household_id) REFERENCES households(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_household_members_household_member
    ON household_members(household_id, member_vcard_uid);
CREATE INDEX idx_household_members_member_vcard_uid ON household_members(member_vcard_uid);
CREATE INDEX idx_household_members_user_id ON household_members(user_id);

-- ---------------------------------------------------------------------------
-- circles
-- ---------------------------------------------------------------------------
CREATE TABLE circles (
    id TEXT PRIMARY KEY,
    created_at DATETIME, updated_at DATETIME,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_circles_feed ON circles(user_id, updated_at, id);
CREATE INDEX idx_circles_user_id ON circles(user_id);

-- ---------------------------------------------------------------------------
-- circle_members
-- ---------------------------------------------------------------------------
CREATE TABLE circle_members (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME, updated_at DATETIME,
    circle_id TEXT NOT NULL,
    user_id INTEGER NOT NULL,
    member_vcard_uid TEXT NOT NULL,
    FOREIGN KEY (circle_id) REFERENCES circles(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_circle_members_circle_member
    ON circle_members(circle_id, member_vcard_uid);
CREATE INDEX idx_circle_members_member_vcard_uid ON circle_members(member_vcard_uid);
CREATE INDEX idx_circle_members_user_id ON circle_members(user_id);

-- ---------------------------------------------------------------------------
-- tags
-- ---------------------------------------------------------------------------
CREATE TABLE tags (
    id TEXT PRIMARY KEY,
    created_at DATETIME, updated_at DATETIME,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_tags_feed ON tags(user_id, updated_at, id);
CREATE INDEX idx_tags_user_id ON tags(user_id);

-- ---------------------------------------------------------------------------
-- contact_tags
-- ---------------------------------------------------------------------------
CREATE TABLE contact_tags (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME, updated_at DATETIME,
    tag_id TEXT NOT NULL,
    user_id INTEGER NOT NULL,
    contact_vcard_uid TEXT NOT NULL,
    FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_contact_tags_tag_contact
    ON contact_tags(tag_id, contact_vcard_uid);
CREATE INDEX idx_contact_tags_contact_vcard_uid ON contact_tags(contact_vcard_uid);
CREATE INDEX idx_contact_tags_user_id ON contact_tags(user_id);

-- ---------------------------------------------------------------------------
-- life_events
-- ---------------------------------------------------------------------------
CREATE TABLE life_events (
    id TEXT PRIMARY KEY,
    created_at DATETIME, updated_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    type TEXT,
    date TEXT,
    description TEXT,
    source TEXT,
    related_entity_ids TEXT,
    deleted_at DATETIME,
    remind INTEGER NOT NULL DEFAULT 0,
    etag TEXT,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_life_events_deleted_at ON life_events(deleted_at);
CREATE INDEX idx_life_events_entity_id ON life_events(entity_id);
CREATE INDEX idx_life_events_feed ON life_events(user_id, updated_at, id);
CREATE INDEX idx_life_events_user_id ON life_events(user_id);

-- ---------------------------------------------------------------------------
-- field_definitions
-- ---------------------------------------------------------------------------
CREATE TABLE field_definitions (
    id TEXT PRIMARY KEY,
    created_at DATETIME, updated_at DATETIME,
    user_id INTEGER NOT NULL,
    label TEXT NOT NULL,
    key TEXT NOT NULL,
    target TEXT NOT NULL,
    type TEXT NOT NULL,
    constraints TEXT,
    projection TEXT NOT NULL DEFAULT 'internal-only',
    sensitivity TEXT NOT NULL DEFAULT 'normal',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_field_definitions_feed ON field_definitions(user_id, updated_at, id);
CREATE INDEX idx_field_definitions_sensitivity ON field_definitions(sensitivity);
CREATE UNIQUE INDEX idx_field_definitions_user_key ON field_definitions(user_id, key);

-- ---------------------------------------------------------------------------
-- field_values
-- ---------------------------------------------------------------------------
CREATE TABLE field_values (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME, updated_at DATETIME,
    field_definition_id TEXT NOT NULL,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    value TEXT,
    FOREIGN KEY (field_definition_id) REFERENCES field_definitions(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_field_values_definition_entity
    ON field_values(field_definition_id, entity_id);
CREATE INDEX idx_field_values_entity_id ON field_values(entity_id);
CREATE INDEX idx_field_values_user_id ON field_values(user_id);

-- ---------------------------------------------------------------------------
-- preferences
-- ---------------------------------------------------------------------------
CREATE TABLE preferences (
    id TEXT PRIMARY KEY,
    created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    entity_id TEXT NOT NULL,
    category TEXT NOT NULL,
    key TEXT,
    value TEXT NOT NULL,
    source TEXT,
    confidence REAL,
    last_confirmed DATETIME,
    sensitivity TEXT NOT NULL DEFAULT 'normal',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_preferences_entity_id ON preferences(entity_id);
CREATE INDEX idx_preferences_feed ON preferences(user_id, updated_at, id);
CREATE INDEX idx_preferences_sensitivity ON preferences(sensitivity);
CREATE INDEX idx_preferences_user_id ON preferences(user_id);

-- ---------------------------------------------------------------------------
-- webhooks
-- ---------------------------------------------------------------------------
CREATE TABLE webhooks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    url TEXT NOT NULL,
    events TEXT NOT NULL DEFAULT '[]',
    secret TEXT NOT NULL,
    is_active INTEGER NOT NULL DEFAULT 1,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_webhooks_deleted_at ON webhooks(deleted_at);
CREATE INDEX idx_webhooks_user_id ON webhooks(user_id);

-- ---------------------------------------------------------------------------
-- webhook_deliveries
-- ---------------------------------------------------------------------------
CREATE TABLE webhook_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME, updated_at DATETIME, deleted_at DATETIME,
    webhook_id INTEGER NOT NULL,
    event_type TEXT NOT NULL,
    payload TEXT NOT NULL,
    status_code INTEGER,
    error TEXT,
    attempts INTEGER NOT NULL DEFAULT 1,
    next_retry_at DATETIME,
    FOREIGN KEY (webhook_id) REFERENCES webhooks(id) ON DELETE CASCADE
);
CREATE INDEX idx_webhook_deliveries_next_retry_at ON webhook_deliveries(next_retry_at);
CREATE INDEX idx_webhook_deliveries_webhook_id ON webhook_deliveries(webhook_id);
