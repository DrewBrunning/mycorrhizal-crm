-- N9 (docs/fork-plan/tickets/30-N9-notification-channels.md) — delivery of
-- reminders beyond email.
--
-- Five additions:
--
--   1. notification_configs — per-user connection settings for the push-style
--      channels (ntfy URL+topic, Gotify URL+token), one row per user. The
--      Gotify token is stored ENCRYPTED via services/credential_crypto.go
--      (AES-256-GCM, key derived from JWT_SECRET_KEY), never plaintext —
--      exactly the ImmichConfig pattern. Soft-deletes and carries a PARTIAL
--      unique index (WHERE deleted_at IS NULL) for the same T26 reason as
--      immich_configs: a soft-deleted row would otherwise block re-creating
--      the config. Email needs no row here — it is configured server-side
--      (Resend/SMTP env vars) and gated per-reminder by reminders.by_mail.
--
--   2. push_subscriptions — per-user Web Push device registrations, written by
--      the browser's Push API. One row per device. Hard-delete (no deleted_at):
--      a device subscription is transient state (an endpoint the push service
--      hands us), re-registered whenever the browser re-subscribes, and
--      auto-removed when the push service reports 404/410. Not user-authored
--      content.
--
--   3. notification_deliveries — per-reminder-per-channel delivery state,
--      mirroring webhook_deliveries: one row per reminder per channel with
--      independent sent/failed tracking. This is the email-shaped
--      reminders.email_sent replacement. Hard-delete (a delivery is an
--      accessory to its reminder, not independently meaningful — T26). The
--      reminder FK carries ON DELETE CASCADE so a hard-purged reminder takes
--      its deliveries with it; soft-deleted reminders are cleaned by the
--      controllers' manual cascade (see deleteReminderDeliveries).
--
--   4. server_settings — a tiny key/value store for server-global generated
--      secrets, currently one entry pair: the Web Push VAPID keypair, generated
--      once on first use and shared by all users (an app-level identity, like
--      the email From address — per-user keys would orphan subscriptions).
--
--   5. users.notify_ntfy / notify_gotify / notify_push — per-user channel
--      toggles, generalising reminders.by_mail. Email stays gated on by_mail
--      per-reminder for backwards compatibility; the new channels are gated by
--      these booleans (a channel dispatches when its toggle is on AND the user
--      has a usable config for it).
--
-- Backfill: every existing reminders row with email_sent=1 (i.e. an email was
-- already delivered for the current occurrence) becomes one 'sent' email
-- delivery so the new eligibility query ("no sent delivery for channel X")
-- does not re-email yesterday's reminders the first run after upgrade.

CREATE TABLE notification_configs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    deleted_at DATETIME,
    user_id INTEGER NOT NULL,
    ntfy_url TEXT NOT NULL DEFAULT '',
    ntfy_topic TEXT NOT NULL DEFAULT '',
    gotify_url TEXT NOT NULL DEFAULT '',
    gotify_token_encrypted TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_notification_configs_user_id ON notification_configs(user_id)
    WHERE deleted_at IS NULL;

CREATE TABLE push_subscriptions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    endpoint TEXT NOT NULL,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    device_label TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE INDEX idx_push_subscriptions_user_id ON push_subscriptions(user_id);

CREATE TABLE notification_deliveries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    reminder_id INTEGER NOT NULL,
    channel TEXT NOT NULL CHECK (channel IN ('email', 'ntfy', 'gotify', 'push')),
    sent_at DATETIME,
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'sent', 'failed')),
    error TEXT,
    FOREIGN KEY (reminder_id) REFERENCES reminders(id) ON DELETE CASCADE
);
CREATE INDEX idx_notification_deliveries_reminder_id ON notification_deliveries(reminder_id);
CREATE INDEX idx_notification_deliveries_channel ON notification_deliveries(channel);

CREATE TABLE server_settings (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL
);

ALTER TABLE users ADD COLUMN notify_ntfy INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN notify_gotify INTEGER NOT NULL DEFAULT 0;
ALTER TABLE users ADD COLUMN notify_push INTEGER NOT NULL DEFAULT 0;

INSERT INTO notification_deliveries (created_at, updated_at, reminder_id, channel, sent_at, status, error)
SELECT created_at, updated_at, id, 'email', COALESCE(last_sent, updated_at), 'sent', NULL
FROM reminders
WHERE email_sent = 1;
