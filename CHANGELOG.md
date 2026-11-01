# Changelog

All notable changes to Mycorrhizal CRM are documented here. This changelog is written for people who
use or self-host the app, not for developers; the
[GitHub Releases](https://github.com/DrewBrunning/mycorrhizal-crm/releases) and the commit history
keep the full technical record.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [1.5.1] - 2026-10-27

> **Upgrade**: if you use two-factor authentication with CardDAV or CalDAV, create an app password
> and update your sync client. App passwords and API tokens now have narrower permissions, and an
> API token can no longer create or revoke other tokens. The default Docker Compose file now pulls
> the published image; pin `IMAGE_TAG` if you need reproducible upgrades.

### Added

- **Scoped app passwords and API tokens**
  ([#2010](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2010)): connect contact and
  calendar sync clients and automation with credentials limited to what they need.
- **Third-party license notices** in the container image and Android app
  ([#1880](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1880)).
- **All app languages in Android builds**, so the in-app language picker also works in the Google
  Play build.

### Changed

- **Docker Compose pulls the published image by default**: source builds use the build override
  file, and `IMAGE_TAG` pins a release for reproducible upgrades.
- **Contact and calendar sync imports everything**, instead of stopping at 2,000 contacts or 500
  events.
- **CardDAV and CalDAV clients with unusual ETag headers** are now accepted.

### Fixed

- **CardDAV updates no longer clear circles, how you met, work details, or date periods.**
- **Android: a notification or link that arrives while the app is reopening** is now opened.
- **Android: refreshing a contact no longer undoes a favorite change.**
- **Import errors** no longer repeat "not found".

### Security

- **Contacts no longer show another account's data** when sync identifiers collide.
- **API tokens stop working when their user is deleted**, and the two-factor lockout and
  password-reset races are closed.
- **Account passwords cannot be used for CardDAV or CalDAV when two-factor authentication is on**;
  use an app password instead.
- **App passwords and tokens are scoped to their surface**, so a contact-sync credential cannot be
  used against calendars or to manage credentials.
- **Contact data is scoped to its owner** in every outward projection.
- **Dependency and container updates** clear known security advisories.

## [1.5.0] - 2026-10-24

> **Upgrade**: on first start, dated gifts are set once to their calendar day in your server's
> `REMINDER_TIMEZONE`. A gift entered from another time zone may land a day off; edit it to correct
> it. Database migrations run automatically. Nothing else to do.

### Added

- **Ambient contacts** ([#1733](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1733)): mark
  someone such as a housemate so they never show as overdue. Given gifts now count as interactions.
- **Target closeness per contact**
  ([#1796](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1796)): set how close you want to
  be to someone; the dashboard suggests a reach-out when you fall behind.
- **Grouped custom fields** ([#1730](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1730)):
  give custom fields a group label, shown on web and Android contact pages.
- **Choose how times are shown** ([#1830](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1830)):
  a new time-format setting on web and Android.
- **Share text into a note** ([#1294](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1294)):
  text shared to the web app becomes a note draft.
- **Import contacts from your device's contact picker**
  ([#164](https://github.com/DrewBrunning/mycorrhizal-crm/issues/164)).
- **Format examples on contact fields**
  ([#1725](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1725)): phone, email, and date
  fields show the expected format.
- **Android: date pickers for every date field**
  ([#1726](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1726)), and the app languages
  appear in the system language picker on Android 13 and later.

### Changed

- **Android allows screenshots by default**
  ([#1741](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1741)); an opt-in setting restores
  blocking, and the recent-apps preview is blank on Android 13 and later.
- **Due dates use the calendar day you entered**
  ([#1823](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1823)), fixing off-by-one dates
  west of UTC.
- **The dashboard shows at most 25 rows per block**
  ([#1791](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1791)); Android shows
  "Showing N of M".
- **Skills, work, and anniversaries have their own sections**
  ([#1729](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1729)) on the contact page and
  Android.
- **Android navigation is reorganised** with a grouped drawer, sub-screens, and a navigation rail on
  wide screens.

### Deprecated

- **Hobby preferences** ([#1738](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1738)):
  hobbies now live in the contact's personal info, and existing ones move there on first start. The
  old API category keeps working until v2.0.0, not before 2027-01-21.

### Fixed

- **The contact map renders again**
  ([#1777](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1777)).
- **Failed loads show an error with Retry** instead of an empty page, and an offline reload says so
  ([#1779](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1779)).
- **Phone layouts** no longer overflow, and Add Contact works on phones
  ([#1783](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1783)).
- **Unsaved input is no longer lost**: dialogs and Android forms ask before discarding changes
  ([#1780](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1780)).
- **Screen readers announce page changes** and bulk-selection counts; TalkBack no longer opens the
  Android drawer by accident ([#1917](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1917)).
- **Android: importing contacts no longer crashes** without the contacts permission
  ([#1750](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1750)).
- **Android: the notifications toggle asks for permission** on Android 13 and later.
- **A busy database answers 503** with a retryable error instead of 500.
- **Relationship health explanations** are shown in your language.

### Security

- **Private and secret hobbies stay private**
  ([#1738](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1738)): above-normal entries are
  kept out of sync, shares, and standards exports.
- **Updated web server image** (nginx 1.30.6) to clear known advisories.

## [1.4.3] - 2026-10-15

### Security

- **Updated the bundled web server and compression library** to address security advisories.

## [1.4.2] - 2026-10-14

### Added

- **Conflict detection on web and Android**
  ([#1656](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1656)): if a contact changed
  elsewhere, you are warned before overwriting it.

### Changed

- **Password-reset emails are sent in the background** and rate-limited per account.

### Fixed

- **Android: a notification or deep link that arrives while the app is reopening** is now opened
  ([#1654](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1654)).
- **Android: refreshing a contact no longer undoes a favorite change.**
- **Saving notes, activities, reminders, and life events no longer fails** after the change was
  saved.
- **Import errors no longer leak database details.**

### Security

- **Updated dependencies** to clear known security advisories.

## [1.4.1] - 2026-10-06

### Added

- **Android: GeoPulse location history can suggest activities.**
- **Address sensitivity on web and Android**: mark an individual address as private or secret, with
  a lock indicator.
- **CSV export includes every address** with its coordinates and sensitivity.

### Changed

- **The contact map can look up coordinates before an address is saved**, and shows a basemap when
  you use a custom tile style.
- **Changing an integration or subscription address now requires re-entering its secret**, and
  redirects are not followed with saved credentials.
- **Password-reset requests always answer normally**, even when the reset email cannot be sent.

### Fixed

- **Contact map rendering, clustering, and the accessible contact list.**
- **Editing an activity no longer loses its link to the external service.**
- **CardDAV keeps coordinates and labels with the correct address**, including coordinates that do
  not match a saved address.

### Security

- **Private and secret addresses stay out of MCP search, exports, sync, and shares** unless
  explicitly included.
- **The geocoder cache is per user**, and lookups are bounded.
- **Saved credentials cannot be sent to a redirect target or reused after an address change.**

## [1.4.0] - 2026-10-05

> **Upgrade**: a database migration adds address identifiers automatically. The geocoder is off by
> default; the new map settings, including `MAP_TILE_STYLE_URL` and `GEOCODER_PROVIDER`, are
> optional. The MCP server is now reachable at `/mcp` through the bundled proxy.

### Added

- **Contact map on web and Android**
  ([#1286](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1286)): plot addresses with
  coordinates, choose a basemap, and optionally look up coordinates with a geocoder.
- **GeoPulse location history can suggest activities**
  ([#160](https://github.com/DrewBrunning/mycorrhizal-crm/issues/160)).
- **A read-only MCP server** ([#176](https://github.com/DrewBrunning/mycorrhizal-crm/issues/176))
  lets an AI assistant read your CRM data with your API token.
- **Android: an About screen** showing app and server version.

### Changed

- **Addresses can be marked private or secret**
  ([#1433](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1433)); sensitive addresses stay
  out of sync, exports, shares, and MCP by default.
- **vCard and CardDAV keep coordinates and labels with the right address**
  ([#1434](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1434)).

### Fixed

- **Calendar sync clients receive proper error codes**
  ([#1439](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1439)).
- **Contact map coordinates round-trip correctly** for small values
  ([#1447](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1447)).

### Security

- **The geocoder is off by default**; when enabled, only the chosen address is sent, through
  guarded requests.
- **Location data is never written to logs.**

## [1.3.0] - 2026-10-01

### Added

- **Android local mode** ([#1108](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1108)):
  use the app entirely on your phone.
- **Server profiles** ([#1261](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1261)):
  switch between multiple Mycorrhizal servers.
- **Account bundles** ([#1259](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1259)):
  export a full account and import it elsewhere, with local backup and restore on Android.
- **Private Atom feeds for contacts**
  ([#1274](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1274)), with a management screen.
- **Passkeys for web and Android login**
  ([#593](https://github.com/DrewBrunning/mycorrhizal-crm/issues/593),
  [#1293](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1293)), including recovery codes.
- **Admins can reset a locked-out user's two-factor authentication**
  ([#596](https://github.com/DrewBrunning/mycorrhizal-crm/issues/596)).
- **Deep links and share-to-CRM on Android**
  ([#1269](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1269),
  [#1271](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1271)): open a contact from a link
  or share text into a note draft.
- **Web push notifications open the linked contact**
  ([#1270](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1270)).
- **Android: a migration wizard** moves a local account onto a server.

### Changed

- **Android navigation is reorganised**, with a paged timeline explorer and save buttons kept above
  the fold.

### Fixed

- **Merging contacts no longer loses occasions, event attendance, or feeds**
  ([#1309](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1309)).
- **Account bundles preserve photos, relationship details, and mailing flags.**
- **Deleting a user or contact removes all related data**
  ([#1310](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1310)).
- **Second-factor and passkey flows are safer**, including adding a second factor and removing the
  last passkey.

### Security

- **Two-factor and passkey fixes close account-takeover paths.**
- **Local-mode sessions are isolated** and no longer time out or clear unexpectedly.

## [1.2.0] - 2026-09-26

### Added

- **Relationship health**: the network graph colors contacts by how well you are staying in touch.
- **Occasions** ([#1222](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1222)): recurring
  card, gift, and invite obligations.
- **Events with invitee lists and RSVP tracking** on web and Android
  ([#1228](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1228)).
- **Temporal periods** ([#354](https://github.com/DrewBrunning/mycorrhizal-crm/issues/354)): record
  when an employer, title, or address applied, with suggestions for life events.
- **Deceased contacts** ([#1193](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1193)),
  driven by a death anniversary; they drop out of birthday, cadence, and reach-out prompts.
- **Hobby proficiency levels** ([#246](https://github.com/DrewBrunning/mycorrhizal-crm/issues/246)).
- **Data decay reminders** ([#352](https://github.com/DrewBrunning/mycorrhizal-crm/issues/352)) to
  confirm contact details are still current.
- **Custom fields can be reordered** on web and Android.

### Changed

- **The network graph remembers your circle filter**
  ([#350](https://github.com/DrewBrunning/mycorrhizal-crm/issues/350)).

### Fixed

- **New occasions no longer start active unexpectedly.**

### Security

- **Occasion exports are protected against spreadsheet formula injection.**

## [1.1.0] - 2026-09-23

> **Upgrade**: servers older than v1.0.0 are no longer supported and the Android app refuses them.
> Android now has separate F-Droid, Obtainium, and Google Play builds; the GitHub release APK is
> named `app-obtainium-release.apk`. Moving between Android channels requires uninstall and
> reinstall because each store signs with a different key.

### Added

- **Android: custom field management** and per-contact value editing.
- **Android: "Mark as Me"** with a You badge.
- **Android: choose which contact fields are shown and editable.**
- **Android: selective export, CSV import with history, and Immich sync**
  ([#835](https://github.com/DrewBrunning/mycorrhizal-crm/issues/835),
  [#834](https://github.com/DrewBrunning/mycorrhizal-crm/issues/834),
  [#836](https://github.com/DrewBrunning/mycorrhizal-crm/issues/836)).
- **Android: connection screens** for Paperless, Seafile, and Nextcloud.
- **Android: missed call and SMS activity is now captured**
  ([#1127](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1127)).

### Changed

- **Android distribution channels**
  ([#1133](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1133)): F-Droid, Obtainium, and
  Google Play builds, with the Play build omitting call and SMS logging.
- **Servers below v1.0.0 are no longer supported**
  ([#1170](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1170)).

### Security

- **Android backups are disabled** on the F-Droid and Play builds.

## [1.0.0] - 2026-09-21

The first stable release.

### Added

- **Contacts** with a standards-based card model, custom fields, circles, and tags.
- **Relationships, households, and pets**, with a network graph.
- **Staying in touch**: cadence, favorites, Prep View, a conversation agenda, and reminders by
  email, ntfy, Gotify, or browser push.
- **Life events, deceased contacts, and gift tracking**, plus links to Immich, Paperless, Seafile,
  and Nextcloud.
- **CardDAV and CalDAV sync**, vCard 3.0/4.0 and JSContact export, full backup and restore, and
  one-time contact sharing.
- **Two-factor authentication, OIDC single sign-on, and API tokens.**
- **A native Android app** with offline cache and favorites.
