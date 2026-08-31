# Reference-client interoperability matrix (TEST-09, issue #681)

Real third-party clients consuming our CardDAV/CalDAV server. The automated leg
is **vdirsyncer** (see `testing.md` → "Real-server + real-client interoperability");
this document is the **manual matrix** for the clients that cannot be scripted
in CI, per the ticket's own guidance ("A documented manual matrix with a
checklist is a real deliverable; an imaginary one is not").

Each row records the **last run date** and the checklist that was executed.
A run that finds a problem must name the client, the property, and the
divergence (that is the ticket's "actionable failure" bar); a clean run records
the date and the tester.

## The server under test

Point any DAV client at a self-hosted instance with `CARDDAV_ENABLED=true`
(and `CALDAV_ENABLED=true` for the calendar rows):

- **Base URL:** `https://<host>/`
- **Well-known:** `/.well-known/carddav` → `/carddav/`, `/.well-known/caldav` → `/caldav/`
- **Principal:** `/carddav/principals/<username>/`
- **Address book:** `/carddav/addressbooks/<username>/contacts/`
- **Auth:** HTTP Basic, username = your account username (or email), password = your password
- **CalDAV:** read-only calendar at `/caldav/` (activities + life events; client subscribes, never writes)

## Checklist (each client)

For every client row, run:

1. **Provision** — add the account using the server URL above; the client must
   autodiscover the principal + address book (this exercises `/.well-known`,
   principal PROPFIND, addressbook-home-set).
2. **Pull** — the address book must display the existing contacts (names,
   emails, phones, addresses, non-ASCII names, photos if any). Feed it the
   TEST-02 pathological fixture contacts (they are in the seeded fixture set)
   and confirm they render: multi-name contacts, an empty-note-with-params,
   a country-code-only address, a historical birthday.
3. **Push** — create a contact, edit it, delete it; confirm the change appears
   after a refresh/re-sync (exercises PUT with If-Match + ETag handling).
4. **Incremental** — make a change from a *different* client, then force a sync
   in this client and confirm only the delta is fetched (exercises CTag/ETag).
5. **Version negotiation** — confirm the client is served the vCard version it
   requests (our server negotiates via the `Accept` header; default 4.0).

## Matrix

### vdirsyncer (automated — the reference for everything below)

| Aspect | Value |
|---|---|
| Type | CLI sync client, own protocol + vobject parser |
| Coverage | provision/discover, pull with semantic equality of the client's parsed view, ETag quiescence, PUT create/update, DELETE |
| Where | `TestCardDAVVdirsyncer_ClientRoundTrip`, `reference-clients-e2e.yml` (nightly + path-gated) |
| Last run | automated, nightly — see the workflow's latest run |

### Apple Contacts (macOS)

Not scriptable from CI. Manual checklist above; the macOS Contacts app is a
strict CardDAV client (it re-parses everything and is sensitive to vCard
escapes/line-folding).

| Last run date | Result | Tester |
|---|---|---|
| (not yet run) | | |

### Apple Contacts (iOS)

Same checklist. iOS adds no separate client surface (it uses the same
Contacts.app engine as macOS).

| Last run date | Result | Tester |
|---|---|---|
| (not yet run) | | |

### Thunderbird (CardDAV address book + CalDAV calendar)

Thunderbird's CardDAV implementation lives in `mailnews`; a scripted harness
(a headless profile + an extension driving the address book) is the planned
automation path and has not landed yet — until then this is manual.

| Last run date | Result | Tester |
|---|---|---|
| (not yet run) | | |

### Android native contacts (via the Android provider)

The native contacts provider consumes the CardDAV server when an account is
added. Scripting this requires an emulator + DAVx5 (below); the provider row
is exercised as part of that leg.

| Last run date | Result | Tester |
|---|---|---|
| (not yet run) | | |

### DAVx5 (Android)

DAVx5 is the most feature-complete Android CardDAV/CalDAV client and the
closest to scriptable (an emulator + its account setup flow via `adb`/uiautomator
is the planned automation path). Until then, manual: add the account in the
DAVx5 app, enable both contacts and calendar sync, and run the checklist. Note
DAVx5 *requests* vCard 4.0 explicitly — it is the best test of our version
negotiation outside Apple.

| Last run date | Result | Tester |
|---|---|---|
| (not yet run) | | |

## Divergences found

A manual run that finds a divergence goes in the table below (the ticket's
"actionable failure" format), and if it is a server bug it becomes a pinned
test first.

| Date | Client | Property | Divergence |
|---|---|---|---|
| — | — | — | — |

## Related

- Automated vdirsyncer leg + server matrix: `docs/development/testing.md`
- Server matrix (Radicale/Baikal/Nextcloud): `carddav-e2e.yml`, `TestCardDAVReferenceServer_RoundTrip`
- The pathological fixture: TEST-02, issue #430
- Differential serialization suite: TEST-08, issue #680
