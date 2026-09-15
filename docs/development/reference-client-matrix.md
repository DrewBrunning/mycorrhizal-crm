# Reference-client interoperability matrix (TEST-09, issue #681)

Real third-party clients consuming our CardDAV/CalDAV server. The automated
legs are **vdirsyncer** and, since issue #917, **DAVx5** (see `testing.md` →
"Real-server + real-client interoperability" and the `davx5` job in
`reference-clients-e2e.yml`); this document is the **manual matrix** for the
clients that cannot be scripted in CI, per the ticket's own guidance ("A
documented manual matrix with a checklist is a real deliverable; an
imaginary one is not") — and, per issue #917's disposition, the record of
which clients this project's compatibility claim actually covers.

Each row records the **last run date** and the checklist that was executed.
A run that finds a problem must name the client, the property, and the
divergence (that is the ticket's "actionable failure" bar); a clean run records
the date and the tester.

## Scope of the compatibility claim (issue #917)

Issue #917 found this matrix had **never been run** — every manual row read
`(not yet run)` — and gave two acceptable outcomes: run one documented pass,
or explicitly scope the compatibility claim down to what is actually
exercised. The 2026-09-15 pass below does both:

- **Apple Contacts (macOS/iOS) and Thunderbird are explicitly out of scope
  for the automated/CI compatibility claim.** Neither has a CI-drivable
  surface (no headless macOS/iOS runner for Contacts.app; Thunderbird's
  CardDAV code has no scripted harness yet — see its row below), so this
  project cannot claim to exercise them the way `reference-clients-e2e.yml`
  exercises vdirsyncer and DAVx5. This is a scope decision, not a deferral:
  revisit it only if a real scripted harness becomes available for either
  client (Thunderbird's row already names the planned approach).
- **DAVx5 and the Android native contacts provider are now automated**
  (the `davx5` job, added by #917) — see their rows below.
- The **interop test harness** in the next section makes it cheap for a
  maintainer with real Apple/Thunderbird devices to run those two rows'
  checklist by hand whenever they want a data point beyond CI's automated
  coverage; the harness existing doesn't change the scope decision above.

## The server under test

The fastest way to get a real, seeded server to point a client at is the
interop harness:

```bash
docker compose -f docker-compose.interop.yml up -d --build --wait
docker compose -f docker-compose.interop.yml exec mycorrhizal cat /app/data/credentials.json
```

This builds the shipped all-in-one image, seeds it with the canonical
pathological dataset (issue #430 — the exact fixtures the checklist below
asks for) via the same `pentestseed` tool `docker-compose.pentest.yml` uses,
and serves it on `http://localhost:7300` with **both** `CARDDAV_ENABLED` and
`CALDAV_ENABLED` on. Read the credentials (username `pentest`, printed
password) out of the command above. Testing from a real phone or laptop on
your LAN: set `INTEROP_BASE_URL=http://<your-LAN-IP>:7300` before bringing it
up (see the compose file's header for why). `docker compose -f
docker-compose.interop.yml down -v` wipes it.

Equivalently, by hand against any self-hosted instance:

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

**Out of scope for the automated compatibility claim** (see "Scope of the
compatibility claim" above) — no CI-drivable surface exists. Manual checklist
above; the macOS Contacts app is a strict CardDAV client (it re-parses
everything and is sensitive to vCard escapes/line-folding). A maintainer
with a real Mac can run this against the interop harness above at any time;
the row below records the result whenever that happens.

| Last run date | Result | Tester |
|---|---|---|
| (not yet run — out of scope, see above) | | |

### Apple Contacts (iOS)

Same checklist and same scope decision as macOS above. iOS adds no separate
client surface (it uses the same Contacts.app engine as macOS).

| Last run date | Result | Tester |
|---|---|---|
| (not yet run — out of scope, see above) | | |

### Thunderbird (CardDAV address book + CalDAV calendar)

**Out of scope for the automated compatibility claim** (see "Scope of the
compatibility claim" above). Thunderbird's CardDAV implementation lives in
`mailnews`; a scripted harness (a headless profile + an extension driving
the address book) is the planned automation path and has not landed yet —
until then this is manual, same as Apple Contacts above.

| Last run date | Result | Tester |
|---|---|---|
| (not yet run — out of scope, see above) | | |

### Android native contacts (via the Android provider)

**Automated** since issue #917, as part of the DAVx5 leg below: the native
contacts provider consumes the CardDAV/CalDAV server through DAVx5's sync
adapter, and `run.sh` asserts against `content://com.android.contacts/contacts`
directly (the actual provider), not DAVx5's own UI — this row and the DAVx5
row below share one pass.

| Last run date | Result | Tester |
|---|---|---|
| 2026-09-15 | Clean (after the fix below) | Claude (automated, `davx5` job) |

### DAVx5 (Android)

**Automated** since issue #917: `.github/scripts/reference-client-davx5/run.sh`
drives a real, pinned DAVx5-OSE APK (sideloaded, not our own instrumented
code) through account setup on an emulator via `adb`/uiautomator — locating
every UI element by exact text/content-desc rather than hardcoded
coordinates, since DAVx5 is a Jetpack Compose app with no resource-ids in
its accessibility tree — then asserts the seeded canonical-fixture contacts
(issue #430), including non-ASCII names, actually landed in Android's real
`ContactsContract`. Runs nightly + on `reference-clients` path changes as the
`davx5` job in `reference-clients-e2e.yml`, the same gating as the
vdirsyncer job. DAVx5 *requests* vCard 4.0 explicitly — it is the best test
of our version negotiation outside Apple.

**Coverage today: checklist items 1–2 (Provision, Pull) only.** Items 3–4
(Push, Incremental) aren't automated yet — `run.sh` doesn't create/edit/delete
a contact from the DAVx5 side or force a second sync — that's a natural
follow-up to extend the script, not a gap in what's claimed here. Item 5
(version negotiation) is exercised implicitly (DAVx5 requests 4.0 and gets
4.0) but not asserted directly.

| Last run date | Result | Tester |
|---|---|---|
| 2026-09-15 | Found + fixed a real divergence (see below), clean after the fix | Claude (automated, `davx5` job) |

## Divergences found

A manual run that finds a divergence goes in the table below (the ticket's
"actionable failure" format), and if it is a server bug it becomes a pinned
test first.

| Date | Client | Property | Divergence |
|---|---|---|---|
| 2026-09-15 | DAVx5 | `.well-known/carddav` and `.well-known/caldav` discovery method | DAVx5 issues `PROPFIND` directly against the well-known URIs during autodiscovery (not `GET`, which is all RFC 6764 requires and all our routes accepted). Our `router.GET`-only registration 404'd the PROPFIND, and account setup failed outright with "Couldn't find CalDAV or CardDAV service." Fixed by also registering `PROPFIND` on both well-known routes (`backend/routes/routes.go`), pinned by `TestWellKnownDAVDiscovery_AcceptsPROPFIND` (`backend/routes/routes_test.go`) — hand-verified to fail on the old code (404) and pass on the fix (301), per this repo's hand-verify convention. |

## Related

- Automated vdirsyncer + DAVx5 legs, server matrix: `docs/development/testing.md`
- Interop test harness: `docker-compose.interop.yml`
- DAVx5 automation script: `.github/scripts/reference-client-davx5/run.sh`
- CI job: `.github/workflows/reference-clients-e2e.yml` (`davx5` job)
- Server matrix (Radicale/Baikal/Nextcloud): `carddav-e2e.yml`, `TestCardDAVReferenceServer_RoundTrip`
- The pathological fixture: TEST-02, issue #430
- Differential serialization suite: TEST-08, issue #680
