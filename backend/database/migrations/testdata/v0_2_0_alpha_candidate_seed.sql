-- Issue #254: full-chain migration data-preservation regression test fixture.
--
-- Representative rows for a database at the v0.2.0-alpha-candidate tag —
-- the point at which real production data starts to exist (see CLAUDE.md's
-- orientation section). Deliberately contains ONLY INSERT/UPDATE statements,
-- no CREATE TABLE: the schema at that tag is migrations 000001-000008
-- themselves (verified identical in shape to the tag, comment-only diffs),
-- and those files are the source of truth for it. Duplicating their DDL
-- here would drift the moment a future change touched wording in them
-- without touching this fixture.
--
-- database/migrate_datapreservation_test.go applies exactly migrations
-- 000001-000008 (matching the tag) before loading this file, so every
-- statement below must target that schema shape and no later one:
-- no addresses_flat/phones_normalized/sort_name columns, no
-- reach_out_suggestions tables, etc.
--
-- Covers: a contact with nested Card data (name/organization/title/address/
-- note components), a plain contact, a soft-deleted contact, a note, an
-- activity (with an activity_contacts join row), a reminder, and a confirmed
-- relationship edge — the exact set CLAUDE.md's soft-vs-hard-delete rule and
-- issue #254 both call out.

INSERT INTO users (id, created_at, updated_at, username, password, email)
VALUES (1, '2026-07-01 12:00:00', '2026-07-01 12:00:00', 'seed-v020', 'not-a-real-hash', 'seed-v020@example.com');

-- Alice: the rich contact. Her `card` column carries nested RFC 9553-shaped
-- data (name components, an organization, a title, an address with
-- components, and a note) that has no flat-column home of its own — exactly
-- the data class CLAUDE.md backend trap #3 warns a wrong plain-save path can
-- silently drop.
INSERT INTO contacts (
    id, created_at, updated_at, firstname, lastname, email, phone,
    organization, job_title, user_id, vcard_uid, org, fn, card
) VALUES (
    1, '2026-07-01 12:05:00', '2026-07-01 12:05:00', 'Alice', 'Nakamura', 'alice@example.com', '+15035551234',
    'Fungal Networks Ltd', 'Mycologist', 1, 'vcard-seed-alice', 'Fungal Networks Ltd', 'Alice Nakamura',
    '{"uid":"vcard-seed-alice","kind":"individual","name":{"components":[{"kind":"given","value":"Alice"},{"kind":"surname","value":"Nakamura"}],"full":"Alice Nakamura"},"organizations":[{"name":"Fungal Networks Ltd"}],"titles":[{"title":"Mycologist"}],"emails":[{"address":"alice@example.com","contexts":["work"]}],"addresses":[{"components":[{"kind":"name","value":"742 Spore Lane"},{"kind":"locality","value":"Portland"},{"kind":"region","value":"OR"},{"kind":"postcode","value":"97201"},{"kind":"country","value":"USA"}],"contexts":["home"]}],"notes":[{"note":"Met at the North American Mycological Association conference."}]}'
);

-- Bob: a plain contact, used as the relationship edge's target.
INSERT INTO contacts (id, created_at, updated_at, firstname, lastname, email, user_id, vcard_uid)
VALUES (2, '2026-07-02 09:00:00', '2026-07-02 09:00:00', 'Bob', 'Reyes', 'bob@example.com', 1, 'vcard-seed-bob');

-- Carol: soft-deleted, GORM-style (inserted live, then UPDATEd with
-- deleted_at set) — a lingering soft-deleted row is someone's undo button,
-- not a test fixture, per CLAUDE.md.
INSERT INTO contacts (id, created_at, updated_at, firstname, lastname, email, user_id, vcard_uid)
VALUES (3, '2026-07-03 10:00:00', '2026-07-03 10:00:00', 'Carol', 'Diaz', 'carol@example.com', 1, 'vcard-seed-carol');
UPDATE contacts SET deleted_at = '2026-07-15 09:30:00', updated_at = '2026-07-15 09:30:00' WHERE id = 3;

INSERT INTO notes (id, created_at, updated_at, content, date, contact_id, user_id)
VALUES (1, '2026-07-04 08:00:00', '2026-07-04 08:00:00', 'Loves truffle hunting and slow correspondence.', '2026-07-04 08:00:00', 1, 1);

INSERT INTO activities (id, created_at, updated_at, title, description, location, date, user_id, uuid, type)
VALUES (1, '2026-07-05 17:00:00', '2026-07-05 17:00:00', 'Coffee catch-up', 'Talked about her new mycology lab', 'Cafe Roma', '2026-07-05 17:00:00', 1, 'activity-seed-1', 'meeting');

INSERT INTO activity_contacts (activity_id, contact_id) VALUES (1, 1);
INSERT INTO activity_contacts (activity_id, contact_id) VALUES (1, 2);

INSERT INTO reminders (id, created_at, updated_at, message, remind_at, recurrence, contact_id, user_id)
VALUES (1, '2026-07-06 07:00:00', '2026-07-06 07:00:00', 'Follow up about the lab opening', '2026-09-01 09:00:00', 'none', 1, 1);

INSERT INTO relationship_edges (
    id, created_at, updated_at, user_id, source_id, target_id, type,
    directional, source, confidence, status, sensitivity
) VALUES (
    'edge-seed-1', '2026-07-07 11:00:00', '2026-07-07 11:00:00', 1, 'vcard-seed-alice', 'vcard-seed-bob', 'friend_of',
    0, 'user-confirmed', 1, 'confirmed', 'normal'
);
