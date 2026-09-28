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
and talks to it over a private Unix socket. The whole product is the same code
the server runs, so a local profile has the same dashboard, cadence, duplicates
and exports a remote one does.

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

The entry point ("Use on this device only", on the sign-in screen and in
**Settings → Servers**) is present in every distribution variant but stays
behind a build flag until the account-bundle backup UI ships — a local profile
holds the only copy of its data, so the app does not offer it before a
user-initiated backup exists.

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
