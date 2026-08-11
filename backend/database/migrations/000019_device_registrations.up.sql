-- M2 (docs/fork-plan/tickets/81-M2-fcm-mobile-push.md) — mobile push device
-- registrations, generalized for any push client (fcm today, apns later).
--
-- The web app's push path (N9) registers browser Web Push subscriptions in
-- push_subscriptions: an endpoint + p256dh/auth keys handed out by the browser
-- Push API. A native mobile app cannot do that — it registers a device token
-- with its platform push service (Firebase Cloud Messaging on Android, APNS on
-- iOS) instead. This table is that token surface, deliberately keyed on a
-- generic (client, token) pair so an iOS client can register against the same
-- endpoint without a contract change; the backend dispatches delivery by
-- client.
--
-- Hard-delete (no deleted_at), exactly like push_subscriptions: a device
-- registration is transient device state, re-registered on every app launch
-- and auto-removed when the push service reports the token dead. Not
-- user-authored content.
--
-- The natural key is (client, token), NOT (user_id, client, token): an FCM
-- token identifies a physical app install, not a person. If the key included
-- user_id, a device that logs out of user A and into user B would accumulate
-- a live row for both users (they differ only by user_id), and A's reminders
-- would keep pushing to a device now signed in as B. Re-registering the same
-- (client, token) reassigns the existing row's user_id instead of duplicating
-- it, mirroring how the platform push services themselves treat the token —
-- one install, one current owner.

CREATE TABLE device_registrations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    created_at DATETIME,
    updated_at DATETIME,
    user_id INTEGER NOT NULL,
    token TEXT NOT NULL,
    client TEXT NOT NULL CHECK (client IN ('fcm', 'apns')),
    device_label TEXT NOT NULL DEFAULT '',
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX idx_device_registrations_client_token
    ON device_registrations(client, token);
CREATE INDEX idx_device_registrations_user_id ON device_registrations(user_id);
