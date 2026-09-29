---
title: User Settings
nav_order: 6
has_children: false
---

# User Settings

## Language

The interface is available in **English**, **Deutsch** (German), **Italiano** (Italian), **Español** (Spanish) and **Français** (French). The language change takes effect immediately.

Your language preference is also used by the backend for notifications — reminder emails, and the messages sent to ntfy, Gotify and browser push. Backend messages (especially errors) always stay in English.


## Date Format

You can choose between the European (DD.MM.YYYY), US (MM/DD/YYYY), and ISO (YYYY-MM-DD) date format. This affects all date displays and also determines the expected input format when entering dates like birthdays.


## Appearance

Choose your preference between light mode and dark mode. This setting is stored locally in your browser and is not synced across devices.


## Custom Field Names

Define custom fields that appear on all your contacts for tracking information that doesn't fit into the standard contact fields. You can change the order of the custom fields, though they will always be displayed after the standard fields.


## Notifications

Choose how reminders reach you — email, [ntfy](https://ntfy.sh), [Gotify](https://gotify.net), or browser push — and register the browsers that should receive push notifications. Each channel has a test button that reports the actual failure reason rather than failing quietly.

See [Notifications](notifications.html) for what each channel needs, how they differ, and what to check when one stops arriving.


## Two-factor authentication

<a id="two-factor-authentication"></a>

**Settings → Two-factor authentication** adds a second step to signing in with your password. The
second step is either a 6-digit code from an authenticator app (TOTP) or a [passkey](#passkeys) —
whichever you have enrolled, and either one is enough. CardDAV sync and API tokens are not affected.

### Turning on an authenticator app

1. Choose **Enable two-factor authentication**.
2. Scan the QR code with your authenticator app (or type the manual setup key), then enter the
   6-digit code it shows and choose **Enable and continue**.
3. Save your **recovery codes** (below).

Enabling your first second factor signs out every other session and device you have. Your current
browser stays signed in.

### Recovery codes

When your account gets its first second factor, the app shows **10 recovery codes** once. Each code
works a single time and takes the place of the second step — use one if you lose your authenticator
app or device. They are not shown again, so copy them (**Copy all codes**) and store them somewhere
safe, such as a password manager.

To replace them, choose **Regenerate recovery codes** and confirm with a current code. The old set
stops working immediately and the new set is shown once. Adding a passkey to an account that already
has recovery codes keeps the existing set; it does not mint new ones.

### Turning it off

**Disable** asks for a current code and signs out every other session. If you still have a passkey
enrolled, the recovery codes and the passkeys stay; if not, the recovery codes are deleted too.

### If you lose every factor

A user with no working authenticator, passkey or recovery code cannot sign in on their own. An
instance admin can clear the account's second factors from **User Management** using the
**Reset 2FA** action: it disables the authenticator app, removes every registered passkey, deletes
the recovery codes and ends the user's sessions. The user can then sign in with just their password
and enroll again. Use it only for a genuinely locked-out user; it is a no-op for an account with no
2FA. Only admins see it.

### Accounts that sign in through an identity provider

Accounts provisioned through OIDC single sign-on cannot enroll an authenticator app or a passkey —
the app refuses with "Two-factor authentication is not available for accounts that sign in through
an identity provider". Second-factor policy for those accounts belongs to the identity provider.


## Passkeys

<a id="passkeys"></a>

**Settings → Passkeys** lets you use your device's fingerprint, face or screen lock (or a hardware
security key) as the second step instead of typing a code. A passkey works alongside an authenticator
app; you can have both, and at sign-in either satisfies the second step (the sign-in page offers
**Use a passkey instead**).

- **Add a passkey:** optionally give it a name (for example "Laptop" or "YubiKey"), choose **Add a
  passkey** and follow your browser's prompt. If you cancel or the prompt times out, nothing is
  saved. If this passkey is your account's first second factor, you are shown recovery codes once —
  see [Recovery codes](#recovery-codes) — and your other sessions are signed out.
- **The list** shows each passkey's name, when it was added and when it was last used.
- **Remove a passkey:** removing is deliberately harder than adding, because it weakens the
  account. **Remove** asks for a proof: a current authenticator code, a recovery code, or — via
  **Verify with another passkey instead** — an assertion from a *different* passkey (that button only
  appears when you have another one; the passkey being removed can never vouch for itself).
- **Removing your last factor:** if the passkey you remove is the only second factor left (no
  authenticator app either), two-factor authentication is effectively off. Your recovery codes are
  deleted and your other sessions are signed out. Add a factor again to turn it back on.

### Passkeys and `FRONTEND_URL`

A passkey is bound to the address you load the app from. The instance uses the hostname of its
`FRONTEND_URL` setting as the passkey's relying-party ID and accepts passkeys only from that origin.
Two consequences:

- **Changing the hostname of `FRONTEND_URL` invalidates every enrolled passkey** for every user.
  Authenticator apps and recovery codes are unaffected, so keep those set up before a move. Operators:
  see [`FRONTEND_URL`](configuration-reference.md) and [Deployment](deployment.md).
- **Passkeys are unavailable** when `FRONTEND_URL` is `*` (the development default), because that
  names no concrete origin. Adding one then fails with "passkeys require FRONTEND_URL to be set to the
  concrete origin users load the app from". A browser without WebAuthn support shows "This browser
  doesn't support passkeys." instead of the add form.

### Passkeys on Android

Passkeys you create in the Android app are the **same credentials** as the ones you create on the
web, because both are bound to the same instance host: the same list, the same removal rules. Native
Android passkeys are an operator opt-in and only work on publicly reachable HTTPS instances. See
[Android app → Passkeys](android-app.md#passkeys) and [ADR 0034](adrs/0034-android-passkeys.md).


## Feeds

<a id="feeds"></a>

**Settings → Feeds** (below API tokens) creates private [Atom](https://www.rfc-editor.org/rfc/rfc4287)
feeds so a feed reader can follow the timelines of your contacts. Design details are in
[ADR 0030](adrs/0030-feeds-atom-emission.md).

- **Create Feed:** give it a name, then choose **Feed of** — **One contact** (that contact's merged
  timeline) or **All contacts** (the aggregate across your non-archived contacts) — and a **Detail**
  level. **Headlines** carries entry titles and dates only; **Full** also copies note text, activity
  details and gift descriptions into the feed. By design, items above normal sensitivity are never
  included in a feed at either detail level.
- **From a contact:** on a contact's page, open the actions menu and choose **Subscribe via feed**
  to open the same dialog already set to that contact.
- **The feed list** shows each feed's name, what it follows, its detail level, when it was created
  and when it was last accessed.

### The feed URL is a password

A feed's URL contains a secret token, and **anyone who has the URL can read the feed** — no sign-in is
involved, which is how feed readers work. Treat it like a password:

- The URL is shown **once**, when you create (or rotate) the feed. If you lose it, rotate the feed.
- Do not paste it into chats or public trackers. A feed reader keeps its own copy of everything it
  fetches, and this app cannot delete that copy later, so prefer a self-hosted or local reader.
- If a URL leaks, **revoke** it.

### Rotate, revoke, and revoke all

- **Rotate** revokes the feed's URL immediately and issues a new one with the same settings; the new
  URL is shown once.
- **Revoke** stops one feed's URL working immediately. It cannot be undone; create a new feed instead.
- **Revoke All** revokes every active feed you have.

Each account can have at most **50 active feeds**. Creating a 51st fails with "feed limit reached
(maximum 50 active feeds)"; revoke one you no longer use. Rotating does not count against the cap.

Feeds are managed in the web app only; the Android app has no feed management screen.


## Account bundle export

<a id="account-bundle-export"></a>

The **account bundle** is a single JSON file holding a complete, re-importable copy of your data. It
is served by `GET /api/v1/export/account` (see the [API reference](api-reference.md)) and is what the
[Android app](android-app.md) writes with **Export account bundle to file** and uses to move a
local-only profile to a server. The web app has no button that downloads one, but it is where the
bundle is *imported*: **Settings → Data → Import account bundle** takes a bundle exported from
another Mycorrhizal instance or from a local-only Android profile.

### What it includes

The bundle is **full fidelity**, like the CSV export: it includes every item regardless of its
sensitivity (normal, private and secret), relationships still awaiting confirmation (`suggested`
rows), and embedded profile photos. There is no opt-in for sensitive data, because the bundle is your
own backup landing on your own device.

| Export | Sensitive (private/secret) items | Suggested rows | Purpose |
|---|---|---|---|
| **Account bundle** | Included | Included | Full backup; move between your own instances. Re-importable. |
| **CSV** (**Download CSV**) | Included, labelled per column | Included | Full backup you can read in a spreadsheet; profile photos not included. |
| **vCard** (**Download VCF**), JSContact | Left out unless you opt in | Left out | Handing contacts to another app or person. |

Because the bundle carries everything, keep the file as private as a password-manager export.

### Size limit

The bundle is built in memory as one document, and the import accepts at most **64 MiB**. If your
account would produce a larger bundle, the export refuses instead of handing you a file that could not
be imported: the response is HTTP 507 with a message giving the bundle's size and the 64 MiB limit.
Remove contacts or profile photos you no longer need and try again, or use the CSV or vCard export.
Successful exports also report the limit in an `X-Mycorrhizal-Bundle-Max-Bytes` response header.

### Moving data between instances

Export the bundle from the source, then import it on the destination with **Settings → Data → Import
account bundle**. You get the same review step as other imports, with per-contact add, merge or skip.
The import is a one-time move, never a live sync, and re-running it does not duplicate contacts that
already arrived. For moving an Android local-only profile to a server, see
[Android app](android-app.md).

