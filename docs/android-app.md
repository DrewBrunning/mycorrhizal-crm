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

A **local, on-device profile** (no server at all) is designed (ADR 0028
Decision 2) but not yet offered: the "Use on this device only" entry stays
behind a build flag until the embedded server and account-bundle backup ship.

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

Anything else — an unknown route, a malformed or out-of-range id, an extra path segment — opens
nothing: the app shows whatever it would have shown without the link. The same routes also work as
paths on your own server (`/contacts/{id}`, `/search?q=…`, …), which is the form a Web Push
notification taps into; where the web has no matching screen it opens the route's nearest parent (a
tag opens `/circles?tab=tags`, a circle opens `/circles`).

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
