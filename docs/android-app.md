---
title: Android app
nav_order: 3
---

# Android app

The Android client talks to **your own** Mycorrhizal server — it is not a standalone app and ships no
default or hosted backend. Before installing it, have a server running; the client refuses to talk to
a server below the client/server compatibility floor (see [client/server compatibility](client-compatibility-policy.md)),
which moved to **`v1.0.0`** at the 1.0 major (issue #1170). The app requires Android **8.0 (API 26)**
or later.

## Install channels

The same source tree is built into three distribution variants, one per channel (the *why* is
[ADR 0022](adrs/0022-distribution-variants.md)). **Each channel signs its build with a different
key**, and Android refuses to update an installed app with an artifact signed by another key — so
install from the channel you intend to stay on.

| Channel | Build | Reminder push | Where to get it |
|---|---|---|---|
| GitHub Releases, updated by [Obtainium](https://obtainium.imranr.dev/) | `obtainium` | FCM fast path, WorkManager polling fallback | the project's [Releases page](https://github.com/DrewBrunning/mycorrhizal-crm/releases) |
| [F-Droid](https://f-droid.org/) | `foss` — no Firebase/Google Play Services | WorkManager polling only | the official F-Droid client (submission in progress, issue #1133) |
| Google Play | `play` — no call/SMS capture | FCM fast path, WorkManager polling fallback | Play listing in progress (issue #1201) |

The `obtainium` and `play` builds include Firebase Cloud Messaging (FCM) as a *latency fast-path*; it
is optional at runtime and does nothing without a server-side Firebase configuration. Every channel
delivers reminders through the WorkManager polling workers, so the `foss` (F-Droid) build — which
carries no Firebase or Google Play Services class at all — loses only the fast path, not reminders.

The `play` build omits the opt-in call/SMS activity-logging feature entirely, because Google Play
restricts `READ_SMS`/`RECEIVE_SMS`/`READ_CALL_LOG` to an app that is the user's default dialer or SMS
handler (issue #1200). The `obtainium` and `foss` builds keep it.

## Switching channels

All three variants deliberately share `applicationId = com.mycorrhizal.crm`, but each store signs
with its own key (the project's release keystore for Obtainium, F-Droid's key for F-Droid, and Google
Play App Signing for Play). Android therefore treats them as different apps for update purposes:

**Switching channels requires uninstalling the old app and installing the new one.** This is inherent
to publishing one application through multiple stores; it is not a bug.

Your contacts and history live on your server, so a reinstall re-downloads them after you sign in.
Only the app's on-device settings and offline mirror are local; the app is safe to uninstall if you
can sign in again.

## Server profiles

The app keeps a list of **server profiles**, exactly one active at a time
([ADR 0028](adrs/0028-local-only-android-mode-and-server-profiles.md)
Decision 1). A profile is a named server: a **label** you choose and its URL.
Each profile's sign-in credential is stored separately, so switching between
servers — a home instance and a work instance, or re-pointing after a domain
move — does not sign you out of the other.

Manage them under **Settings → Servers**:

- **Add server** — a label and a URL, then sign in for that server. A new
  profile has no saved session, so the app lands on the sign-in screen for it.
- **Switch** — tap a profile. This is a full switch: the offline mirror is a
  per-profile cache and is wiped, so one server's cached data never appears
  under another. If the call/SMS outbox holds interactions that have not synced
  yet, the app names the count and asks before discarding them.
- **Rename** — change a profile's label.
- **Remove** — revoke the profile's saved sign-in and forget it on this device.

An upgrade preserves your existing server and sign-in: the first launch after
updating migrates them into a single Remote profile, with no re-login.

## Local (on-device) profiles

A **local profile** runs the real Mycorrhizal backend *inside the app* — no
server at all ([ADR 0028](adrs/0028-local-only-android-mode-and-server-profiles.md)
Decision 2). The app starts the embedded server lazily on the first request,
stores its data under its own app-private directory (`filesDir/local-server/`),
and talks to it over a private Unix socket. The CRM data product is the same
code the server runs, so a local profile has the same dashboard, cadence,
duplicates and exports a remote one does.

A local profile is **storage only** (ADR 0028 Decision 2, amended 2026-09-29):
it has no network or multi-user surface. Immich, Paperless, Seafile, Nextcloud,
calendar and contact subscriptions, the Monica import and live Meerkat fetch,
notification channels, admin/user management, webhooks, API tokens and DAV are
not available on-device, and their Settings entries are hidden. Reminders are
not pushed to external channels. File-based imports and exports work as usual.

Constraints:

- **arm64-v8a devices only.** The embedded server is built for, and shipped on,
  arm64-v8a alone; on any other ABI the "Use on this device only" entry is
  hidden. (The pure-Go SQLite stack's x86_64 syscalls are blocked by Android's
  seccomp filter, and `GOOS=android` links internally only for arm64.)
- **The local store is the only copy.** It is not backed by any server and is
  *not* included in Android's cloud backup (`allowBackup=false`). Removing the
  app, or **Settings → Servers → (local) → Delete local data**, permanently
  deletes it — hence the typed confirmation.
- **Backup and attach are user-initiated.** "Export account bundle to file"
  writes a full-fidelity JSON backup through the system file picker; a local
  profile can later be *attached* to a remote server by importing that bundle
  (one-time move, never a live two-way sync). See ADR 0028 Decisions 3 and 5.

### Moving a local profile to a server

**Settings → Servers**, on the active local profile, offers **Move this data to
a server** (ADR 0028 Decision 3, one-time — never a live sync). The wizard:

1. asks you to add or pick a *Remote* server and sign in to it (2FA supported) —
   without switching to it, so the local profile stays active and writable;
2. exports the account bundle from the on-device server, **in memory** (nothing
   is written to disk);
3. uploads it to the remote's `mycorrhizal` import source under an
   `Idempotency-Key`;
4. shows the remote's preview with per-contact add / merge / skip — the same
   review screen the VCF import uses — plus a count of anything the bundle can't
   carry over exactly;
5. on confirm, applies it (and drops the remote's finished import session, which
   would otherwise count against its per-user cap), then **switches to the remote
   profile**;
6. marks the local profile a **read-only archive**: still browsable, every write
   is refused before it leaves the app (with a "read-only archive" message), and
   a persistent **Delete archived local data** action stays on its row. Nothing
   is deleted automatically.

**Size limit.** The bundle is one JSON document held in memory, and the import
accepts at most **64 MiB**. The export enforces the same limit: if the account
is too large (typically many contacts with photos), the **Export** step (and
"Export account bundle to file") fails immediately with a message giving the
bundle's size and the limit — before anything is uploaded — and the local
profile is untouched. Remove contacts or photos you no longer need, or use the
CSV/vCard export instead. The server also reports the limit in an
`X-Mycorrhizal-Bundle-Max-Bytes` response header on every successful export.
Streamed/chunked bundles are out of scope.

Backing out, an error, or the app being killed at any step *before* the switch
leaves the local profile active and writable; re-running is safe because the
server's import ledger keys on the bundle's stable IDs, so contacts already
moved are not created twice. Nothing ever flows from the remote back to the
local profile.

The profile's own "Me" contact is merged into the "Me" contact the destination
already has (restoring a backup into a fresh local profile, or attaching to a
server that has its own Me) rather than creating a second "Me"; the bundle
records which contact is Me (`self_contact_uid`), and a bundle without it
imports every contact as new.

The entry point ("Use on this device only", on the sign-in screen and in
**Settings → Servers**, below the profile list so it stays reachable however many
profiles exist) is offered in every distribution variant. A local profile
holds the only copy of its data, so the app pairs it with the user-initiated
account-bundle backup described above.

## Deep links

Mycorrhizal can be opened from another app — a launcher shortcut, an automation tool, a notes app — or
from a notification, through a `mycorrhizal://` link. A link can only **navigate**: it never changes
your data, and it carries an opaque id, never a name, number, note, or token (ADR 0029, issue #384).
The worst a forged link can do is show you a screen of your own data you could have opened yourself.

**A link opens nothing until you unlock the app.** It is held until sign-in, the server-compatibility
check, and the app lock ([ADR 0014](adrs/0014-local-app-lock-and-biometric-resume.md)) have all
completed. It is then dropped — not opened later, in a different account — if you sign out, switch
[server profiles](#server-profiles) (including an active-profile switch), or the link has already
waited more than 10 minutes. Deep-link URIs are never logged.

The complete set of routes (ADR 0029 §2):

| Route | Link | Opens |
|---|---|---|
| Home / dashboard | `mycorrhizal://home` | the dashboard |
| Contact | `mycorrhizal://contacts/{id}` | that contact's page |
| Contact activity timeline | `mycorrhizal://contacts/{id}/activities` | that contact's activity timeline |
| Search | `mycorrhizal://search?q={query}` | the contact list, with `{query}` filled into search |
| Circle | `mycorrhizal://circles/{id}` | that circle |
| Tag | `mycorrhizal://tags/{id}` | that tag |
| Household | `mycorrhizal://households/{id}` | that household |

A link always opens the screen it names with the argument it carries: a search link filters the
contact list even if Contacts is already open, and a contact link opens that contact, never the
one you looked at before.

Anything else — an unknown route, a malformed or out-of-range id, an extra path segment — opens
nothing: the app shows whatever it would have shown without the link. The same routes also work as
paths on your own server (`/contacts/{id}`, `/search?q=…`, …), which is the form a Web Push
notification taps into; where the web has no matching screen it opens the route's nearest parent (a
tag opens `/circles?tab=tags`, a circle opens `/circles`).

## Contact timeline

A contact's page shows the **5 most recent** events of its merged timeline (activities, notes,
reminders, life events, gifts and external activities such as Immich photos). **View all** opens a
full-screen timeline explorer that pages through the contact's timeline with the same type and
recency filters as the web app and a **Load more** button. The explorer needs a connection to the
server (it is not kept in the offline mirror); on a local profile it reads the on-device server.

## Contact map

**Map** in the navigation drawer (or rail) plots every contact address that has coordinates, on the tile style your
server is configured with. Tap a pin to see the contact's name and address and open the contact; the top-bar toggle
switches to a plain list of the same points (the map canvas itself cannot be traversed with TalkBack). If the server
holds more than 5000 plottable addresses the screen shows the first 5000 and a notice saying so.

In a contact's address editor, **Coordinates (latitude, longitude)** takes a pair such as `51.5007, -0.1246`, and
**Find coordinates** looks up a *saved* address through the geocoder your server operator enabled
(`GEOCODER_PROVIDER`). A private or secret address is never sent to the geocoder, and the lookup is not available in a
[local (on-device) profile](#local-on-device-profiles), where you enter coordinates by hand. Private and secret
addresses still appear on your own map. How sensitivity, geocoding and the operator settings work is in
[Contact map](map.md).

## About and build details

**Settings → About** lists the app's **Version**, **Version code**, **Build type** (build type and distribution
flavor, for example `release / obtainium`, plus any application-id suffix), and, when known, the **Commit** and
**Built** date. **Server version** shows the version (and commit) of the server you are signed in to, with the build date
beneath it when the server reports one; if the server lookup fails it shows a dash and the rest of Settings is
unaffected. **Copy build details** puts all of this on the clipboard as plain text for a bug report.

## Tablets and landscape phones

On a tablet-sized window the app shows a permanent navigation rail and a two-pane contacts list and detail. A phone held
in landscape is wide but short, so the rail would not leave room to scroll its own destinations. The permanent rail and
two-pane layout therefore apply only when the window is both wide (the Expanded width class) and at least medium in
height; a landscape phone keeps the modal navigation drawer, which scrolls as one list so every destination stays reachable.

## Passkeys

The app can use a passkey as your second sign-in step and lets you add, list and remove passkeys
(**Settings → Passkeys**), through the platform's Credential Manager. Your passkey's private key
stays in the Android credential provider (for example Google Password Manager), never in the app.
The design and its limits are recorded in [ADR 0034](adrs/0034-android-passkeys.md).

Passkeys appear in the app only when **all** of these hold:

- you are signed in to a **remote server** (the on-device local profile has no domain, so never);
- that server's operator has turned on native Android passkeys — the server then reports the
  `webauthn_android` capability on `/health` (see `WEBAUTHN_ANDROID_ENABLED` in the
  [configuration reference](configuration-reference.md)); this only works on a publicly reachable
  HTTPS instance whose domain Google can fetch `/.well-known/assetlinks.json` from;
- the device can run Credential Manager: Android 14 or newer, or — on Android 8–13 — the
  Play Store and GitHub (Obtainium) builds, which bundle the Google Password Manager provider. The
  F-Droid build has no Google Play services dependency, so below Android 14 it has no passkeys.

When any of these is missing your account is never a dead end. Sign-in falls back to your
authenticator-app code or a recovery code, and a passkey-only account is told to use a recovery
code and why: "this server isn't set up for passkeys on Android", "this device has no passkey
provider", or "passkeys aren't available on this app for this server". Cancelling the system
passkey prompt is silent. Removing a passkey needs a live second-factor proof — an authenticator or
recovery code, or an assertion from a *different* passkey (offered only when you have another).
Accounts that sign in through an identity provider (OIDC) cannot add passkeys, same as the web app.
When your first passkey is your account's first second factor, the app shows your recovery codes once.
Regenerating recovery codes (Settings > Two-factor authentication) works with a code or, when a passkey
can answer on this device, **Verify with a passkey instead** — so a passkey-only account can rotate its set.

## TLS

The app trusts only system certificate authorities — there is no "import a self-signed certificate"
path. A self-hosted server must present a certificate that chains to an OS-trusted CA (for example a
Let's Encrypt certificate), or the app will refuse the connection. See
[client/server compatibility](client-compatibility-policy.md) and the Android network-security
posture in [MASVS-L1](security/masvs-l1.md).

## Verifying a build

An APK downloaded from the GitHub Releases page can be checked against its build provenance and
signature — see [Verifying release artifacts](security/release-verification.md). F-Droid and Google
Play verify the artifacts they distribute themselves; the [release verification page](security/release-verification.md#android-distribution-channels-and-signing-keys)
also records which key signs each channel.
