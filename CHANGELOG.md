# Changelog

All notable changes to Mycorrhizal CRM are documented here. This changelog is written for people who
use or self-host the app, not for developers; the
[GitHub Releases](https://github.com/DrewBrunning/mycorrhizal-crm/releases) and the commit history
keep the full technical record.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added

- **Standing shares on the web** ([#2292](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2292)): share a contact, note, activity or circle as a read-only, always-current view the recipient must accept. Manage them under Settings in "Shared with me" and "Shared by me". Revoking stops future access but cannot take back what was seen.

- **Choose occasion invitees by searching contacts** ([#2178](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2178), [#2177](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2177)): on the web, an occasion event's invitee dialog now has a contact search that adds each person you pick, circle suggestions have an "Add everyone" button, and a hint appears when you have no circles yet. The user guide also gains a table explaining activities, occasions and reminders.
- **Android import review matches the web** ([#2210](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2210)): filter the review by new rows, duplicates or errors, get Keep/Skip on plain rows and Keep both/Merge/Discard new only on duplicates, fix rows with errors in place, and see every skipped or failed row with its reason after the import, with a share-as-CSV button.
- **Gift preferences** ([#2194](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2194)): on the web, media, jewelry and gift ideas each gain an "other" category for tastes that fit nothing else, and the preference form's "Key" field is now called "Type" (Favorite, Dislike, Allergy, or your own wording). Existing preferences are unchanged.
- **Merge several selected contacts on Android** ([#2212](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2212)): the contacts list's bulk bar now has Merge once two or more contacts are selected. Pick the one to keep, then review each other contact's field conflicts in turn as it is folded into it.
- **Circle timeline names the member** ([#2361](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2361)): each entry in a circle's timeline on the web now shows which member it is about, and the name links to that contact. An activity shared by several members lists each of them.
- **Email address check** ([#2377](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2377)): the web and Android contact forms and contact pages now point out an email that does not look like an address, so you can fix it. Nothing is rejected or changed, so imports and sync still keep every contact.

### Changed

- **Tidier web menu** ([#2175](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2175)): Shares, Audit log, Data and, for admins, Users and System pages now sit under Settings in the left menu. Open them with the arrow beside Settings; every page keeps its address.
- **Clearer vCard custom-field import** ([#2183](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2183)): on the import step for custom `X-` properties, each choice now has a short plain-language explanation, "passthrough" is spelled out (kept untouched, exported again, nothing lost), and "Map to existing field" is greyed out with a reason when you have no custom fields yet.

### Fixed

- **Dark-mode scrollbars** ([#2363](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2363)): in dark mode the navigation sidebar and page scrollbars no longer show a bright white track. Light mode is unchanged.

## [1.6.0] - 2026-11-06

> **Upgrade**: database migrations run automatically on first start. Images are stored in a new
> directory, `IMAGES_DIR` (by default an `images` folder beside your photo folder): add it to your
> backups ([Backups](docs/deployment.md#backups)). `make backup-verify` now also checks the files of
> contacts in Trash, which needs the server's `DATA_ENCRYPTION_KEY` (or `JWT_SECRET_KEY`).
> Narrowing the CardDAV sync scope makes sync clients remove the excluded contacts from the device;
> the server keeps them ([Sync Behavior](docs/carddav.md#sync-behavior)). If you sync contacts over
> CardDAV and deleted any from a sync client on v1.5.1 or earlier, those contacts have no trash
> entry and an incomplete cleanup; there is a repair (see
> [the CardDAV delete repair guide](docs/operations/carddav-delete-repair.md)).

### Added

- **Merge several contacts at once** ([#2197](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2197)): on the web Contacts page, select two or more contacts and choose Merge. With more than two, you pick the contact to keep and then review each merge in turn. Selection still applies to the current page.
- **Contacts list pages** ([#2188](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2188)): the web Contacts page now has page controls with the total number of contacts and a page-size choice (25, 50 or 100; 50 by default, up from 10) instead of a Load more button. The page and size are kept in the address, so reloading or going back returns you to the same spot. Selecting contacts applies to the current page and is cleared when you change page.
- **Fix and review failed import rows** ([#2186](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2186), [#2187](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2187)): on the web, you can correct rows with errors during import review, and a contact with no name now takes its email or phone as its name. The final report lists every skipped or failed contact with the reason, with a CSV download.
- **Email address check** ([#2377](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2377)): the web and Android contact forms and contact pages now point out an email that does not look like an address, so you can fix it. Nothing is rejected or changed, so imports and sync still keep every contact.
- **ntfy access token** ([#2339](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2339)): the ntfy settings accept an optional access token, so reminders can be
  sent through a self-hosted ntfy server that requires authentication to publish. Web and Android.
- **Contact trash** ([#2002](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2002)): deleted contacts go to a Trash page on web and Android, where you
  can restore them or delete them permanently. They are purged after `DELETED_RETENTION_DAYS`.
- **Images on activities, notes and contacts** ([#2004](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2004)): attach images to activities and notes,
  keep a photo gallery per contact with a chosen main image, see every image from a contact's notes
  and activities in one place, and mark any image private or secret. Web and Android.
- **CardDAV sync scope** ([#1993](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1993)): choose which circles sync to your phone or other CardDAV
  clients. Everything still syncs by default.
- **Per-contact field visibility** ([#1997](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1997)): hide or show a field on a single contact, and hide
  empty fields. Web and Android.
- **Bulk-edit contact fields** ([#1996](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1996)): set the same fields on a whole selection of contacts.
  Web and Android.
- **Circle descriptions, colours and timelines** ([#1995](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1995)): describe and colour a circle, and see
  its members' notes and activities merged on one timeline.
- **Relationship start and end dates** ([#2001](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2001)): record when a relationship began or ended, such as
  "married 2010-2018".
- **Import into circles and tags** ([#2000](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2000)): give every imported contact the same circles or tags.
  Web and Android.
- **Import from a running Meerkat** ([#2013](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2013)): connect to your Meerkat server instead of uploading
  its database. Photos come across too. Web and Android.
- **Markdown in notes and activities** ([#2005](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2005)): bold, lists, links and other basic formatting
  render safely.
- **Multi-day activities** ([#2006](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2006)): give an activity an end date. Web and Android.
- **Add a contact to existing activities** ([#2007](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2007)) from the contact's page. Web and Android.
- **Type dates directly** ([#2198](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2198)): date fields accept typed dates in your date format, on web and
  Android.
- **Custom sign-in button name** ([#1992](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1992)): set `OIDC_PROVIDER_NAME` to show your provider's name
  instead of a generic label.
- **Translated error messages** ([#2093](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2093)): errors from the server now appear in your language on
  web and Android, and server errors show a reference you can quote when asking for help.

### Changed

- **"This is me" marks the contact as ambient** ([#2193](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2193)), so you are never shown as overdue. The
  option moved to the bottom of the contact menu and asks for confirmation.
- **The Notes page lists all notes** ([#2180](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2180)), with a filter for all or unfiled notes, instead of
  only unfiled ones.
- **The import review badges are filters** ([#2184](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2184)), and per-row actions appear only for duplicates.
- **Clearer explanations** of circles, tags, households, occasions, the prep view and skill labels
  on web and Android.
- **Log from location and Immich linking are hidden** until GeoPulse or Immich is configured.
- **The storage warning needs low free space as well as high use** ([#2182](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2182)), so large disks stop
  showing "degraded" while hundreds of gigabytes remain.

### Deprecated

- **Flat lockout and sign-in error bodies** ([#2107](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2107)): the account-lockout `429` and two OIDC
  errors will use the standard error format in v2.0.0, not before 2027-02-04. Clients that read
  both shapes keep working. See [the deprecation list](docs/deprecations.md).

### Fixed

- **Contacts list no longer jumps on the first selection** ([#2365](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2365)): ticking the first contact used to push the list down as the bulk actions appeared, so a quick second click could land on the wrong row. The space is now reserved.
- **The Activities page lists by the date of the activity** ([#2360](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2360)), newest first, instead of by when you last edited it, so editing an old activity no longer moves it to the top. This applies on the web and in the Android app.
- **The hobby preference `Sunset` header now names 2027-01-21** ([#1738](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1738)),
  the date in the deprecation list, instead of two days earlier.
- **Busy servers no longer turn away an unlucky save**: concurrent saves wait their turn, so slow
  storage no longer starves one into a "database busy" error, and the audit log keeps up under load.
- **"Export as JSContact" exported every contact** ([#2196](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2196)); it now exports the one you chose.
- **CardDAV deletes, creates and updates now behave like the app**: they use the trash, respect
  the contact limit, and fire webhooks (([#2249](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2249)), ([#2261](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2261)), ([#2255](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2255))).
- **Bulk actions fire contact webhooks** ([#2311](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2311)), so updates and deletes made in bulk reach your
  integrations.
- **Duplicate contact-share imports**, a missed import-link cleanup on delete, and stale note
  lists are fixed (([#2062](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2062)), ([#2063](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2063)), ([#2061](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2061))).
- **Android: sessions and profiles are fully separated** ([#2065](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2065)); offline contact lists respect
  filters (([#2067](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2067))); incoming SMS is handled reliably (([#2066](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2066))).
- **Navigation highlights only the current item** ([#2154](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2154)), and the date picker keeps a steady
  height ([#2199](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2199)).
- **Web: only the reason is shown for invalid input** ([#2094](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2094)), instead of raw field names.
- **Phone-friendly import review** and Android dialog titles wrap correctly.

### Security

- **Purging the trash can no longer remove another account's files** ([#2309](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2309)), and removes the
  image files of purged contacts (([#2310](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2310))).
- **Other accounts' import sessions are indistinguishable from missing ones** ([#2203](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2203)).
- **Image uploads enforce per-owner limits reliably** ([#2313](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2313)).
- **The Monica import no longer follows redirects** ([#2386](https://github.com/DrewBrunning/mycorrhizal-crm/pull/2386)), so your API token is only sent to the
  address you entered. If your Monica URL redirects (for example http to https), enter the final address.
- **Updated dependencies** clear known security advisories.

### Internal

- **Errors carry machine-readable reason codes** with a CI check that blocks new ones without a
  code ([#2098](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2098)).
- **Architecture decisions recorded** for standing shares ([#1998](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1998)) and reusable organisations,
  places and pets ([#1999](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1999)); neither is implemented yet.

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

## [0.9.0] - 2026-09-19

### Internal

- **Release validation is composed from reusable checks** ([#1161](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1161)), and the Release is now owned by the publish workflow ([#1162](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1162)). A durable record lets a failed release be resumed ([#1164](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1164)).

### Fixed

- **Android: the dashboard benchmark logs in and loads reliably.**
- **Background jobs retry a briefly busy database** instead of failing on lock acquire or release.

## [0.8.8] - 2026-09-18

### Internal

- **Releases carry a residual-risk record and a per-release security delta** ([#953](https://github.com/DrewBrunning/mycorrhizal-crm/issues/953)).

## [0.8.7] - 2026-09-18

### Added

- **Optional per-account resource quotas** ([#950](https://github.com/DrewBrunning/mycorrhizal-crm/issues/950)): four `PER_USER_*` variables cap what one account can create on a shared instance. They default to unlimited.
- **Android: delete activities from the inbox**, and pull to refresh on lists.

### Fixed

- **Android: a missing contact id shows its error** instead of an empty state.
- **Release workflow can resume** when a final release fails before the tag.

### Security

- **The scheduled integrity check verifies the audit hash chain** ([#1146](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1146)) and raises an alert if it is broken.

## [0.8.6] - 2026-09-17

Maintenance release with no user-visible changes.

## [0.8.5] - 2026-09-17

> **Upgrade**: 15 configuration variables are now range-checked at start. A value outside its
> documented range stops the server from starting ([#1107](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1107)). Check your settings before upgrading.

### Changed

- **Configuration is validated at start** ([#933](https://github.com/DrewBrunning/mycorrhizal-crm/issues/933)): invalid or out-of-range settings fail fast with a message naming the variable. A configuration register lists every setting.

### Removed

- **The unused `RESEND_TO_EMAIL` variable.**

## [0.8.4] - 2026-09-17

> **Upgrade**: if only some of `OIDC_PROVIDER_URL`, `OIDC_CLIENT_ID` and `OIDC_CLIENT_SECRET` are
> set, or the provider URL is malformed, the server now refuses to start ([#1106](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1106)). On an internet-facing
> instance with any `*_BLOCK_PRIVATE_URLS` flag off, start now logs a warning.

### Changed

- **Incomplete single sign-on settings fail at start** instead of failing at login.

### Security

- **Request-smuggling and filename-injection behaviour is pinned**, and the bundled web server is hardened.
- **Gradle dependencies are locked** and expired dependency exceptions escalate.

## [0.8.3] - 2026-09-16

### Fixed

- **Dates**: day-only imports and date-and-time values normalise correctly.
- **Email and phone labels, and other online services, are no longer silently dropped** in format conversion.
- **Exports**: very long loss reports are truncated efficiently.
- **Android**: fixes for session handling, secure erase and concurrency found through coverage analysis.

## [0.8.2] - 2026-09-15

> **Upgrade**: after the server upgrade, an Android app older than this release can no longer sign
> in with single sign-on ([#1043](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1043)). Update the app.

### Fixed

- **Android single sign-on no longer hands the session token to the app directly** ([#965](https://github.com/DrewBrunning/mycorrhizal-crm/issues/965)): the return is bound to PKCE and carries a one-time code.
- **Android**: logout ends the session and push registration, server deletions reach the offline copy, and the offline database passphrase is saved before encryption.
- **Web**: dates show in local time, not UTC; search results cannot be overwritten by a slower, older request; a failed side request no longer shows a contact as not found.

## [0.8.1] - 2026-09-14

> **Upgrade**: several start-up checks are stricter.
> - `DELETED_RETENTION_DAYS=0` now keeps deleted rows forever, as documented; it used to purge them. A negative value stops start-up ([#1008](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1008)).
> - A corrupt database, or one with tables but no migration record, refuses to start or migrate. Restore a backup ([#1019](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1019)).
> - `make backup` and `make backup-verify` need the same encryption key as the server and write a `.manifest.json` beside each snapshot. Copy it with the snapshot ([#1027](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1027)).
> - `TRUSTED_PROXIES` defaults to loopback; `0.0.0.0/0` and `::/0` are refused ([#1034](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1034)).
> - New login-spray protection is on by default; see `AUTH_SPRAY_ENABLED` ([#1036](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1036)).
> - Android logs only calls and messages that match a contact by default ([#1037](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1037)).

### Added

- **Login-spray detection** ([#1036](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1036)): a burst of failed logins from many sources raises an alert and briefly throttles unfamiliar sources.

### Changed

- **Rate limits group IPv6 clients by /64 network.**

### Fixed

- **Concurrent edits can no longer overwrite each other** (the revision compare-and-swap).
- **Duplicate relationships are removed and prevented** ([#928](https://github.com/DrewBrunning/mycorrhizal-crm/issues/928)); a migration cleans existing duplicates.
- **Purge jobs report failures**, and purging also clears integration settings, subscriptions and suggestions of deleted contacts ([#978](https://github.com/DrewBrunning/mycorrhizal-crm/issues/978)).
- **Backups are signed** ([#1027](https://github.com/DrewBrunning/mycorrhizal-crm/issues/1027)): `backup_stale` now measures your `make backup` schedule.

## [0.7.0] - 2026-09-13

### Added

- **Release candidates**, mandatory release gates, reproducible release artifacts and SLSA provenance for releases ([#446](https://github.com/DrewBrunning/mycorrhizal-crm/issues/446), [#447](https://github.com/DrewBrunning/mycorrhizal-crm/issues/447), [#448](https://github.com/DrewBrunning/mycorrhizal-crm/issues/448), [#499](https://github.com/DrewBrunning/mycorrhizal-crm/issues/499)).
- **Android: calendar and contact sync management** with a sync-health view.

### Fixed

- **Web accessibility findings** from the automated sweep ([#196](https://github.com/DrewBrunning/mycorrhizal-crm/issues/196)).
- **Android: the "server outdated" notice** floats above the bottom bar.

### Internal

- **Versioning, changelog and governance policies** were written ([#445](https://github.com/DrewBrunning/mycorrhizal-crm/issues/445), [#449](https://github.com/DrewBrunning/mycorrhizal-crm/issues/449), [#508](https://github.com/DrewBrunning/mycorrhizal-crm/issues/508)).

## [0.6.12] - 2026-09-10

### Added

- **Sessions are recorded on the server** ([#866](https://github.com/DrewBrunning/mycorrhizal-crm/issues/866)): sign out a single device, and sessions expire when idle.

### Fixed

- **Three web-server defects**: a port leaked from the `.well-known` path, export returned 502, and the import limits were unreachable ([#865](https://github.com/DrewBrunning/mycorrhizal-crm/issues/865), [#863](https://github.com/DrewBrunning/mycorrhizal-crm/issues/863), [#876](https://github.com/DrewBrunning/mycorrhizal-crm/issues/876)).
- **CardDAV returns 404 or 401**, not 500, for unknown collections ([#874](https://github.com/DrewBrunning/mycorrhizal-crm/issues/874)).

### Security

- **Login timing no longer reveals which accounts exist** ([#862](https://github.com/DrewBrunning/mycorrhizal-crm/issues/862)); lockout no longer locks out other users ([#867](https://github.com/DrewBrunning/mycorrhizal-crm/issues/867)).
- **Accepted two-factor codes are single-use** ([#873](https://github.com/DrewBrunning/mycorrhizal-crm/issues/873)).
- **An admin cannot demote or delete another admin** ([#871](https://github.com/DrewBrunning/mycorrhizal-crm/issues/871)).
- **Health checks without a sign-in show less detail** ([#864](https://github.com/DrewBrunning/mycorrhizal-crm/issues/864)); API responses are not cached ([#872](https://github.com/DrewBrunning/mycorrhizal-crm/issues/872)); the webhook error oracle is closed ([#869](https://github.com/DrewBrunning/mycorrhizal-crm/issues/869)).

## [0.6.11] - 2026-09-08

### Added

- **Operator documentation set** ([#486](https://github.com/DrewBrunning/mycorrhizal-crm/issues/486)).
- **Unicode normalisation in search** ([#485](https://github.com/DrewBrunning/mycorrhizal-crm/issues/485)): names compare equal however they are encoded.

### Fixed

- **Dates**: one rule for 29 February, correct days-until across daylight-saving changes ([#482](https://github.com/DrewBrunning/mycorrhizal-crm/issues/482), [#483](https://github.com/DrewBrunning/mycorrhizal-crm/issues/483)).

## [0.6.10] - 2026-09-07

### Added

- **Android: two-factor sign-in** ([#814](https://github.com/DrewBrunning/mycorrhizal-crm/issues/814)), **app lock with biometrics** ([#722](https://github.com/DrewBrunning/mycorrhizal-crm/issues/722)), duplicate review, contact attachments and dataset export ([#710](https://github.com/DrewBrunning/mycorrhizal-crm/issues/710)).
- **Android: call and SMS tracking asks for permission** ([#721](https://github.com/DrewBrunning/mycorrhizal-crm/issues/721)).
- **Android: forced update** when the server requires a newer app ([#528](https://github.com/DrewBrunning/mycorrhizal-crm/issues/528)).
- **Web: session expiry recovery** and a warning before leaving a form with unsaved changes ([#557](https://github.com/DrewBrunning/mycorrhizal-crm/issues/557)).

### Fixed

- **Android: the server address stays filled in after logout** ([#723](https://github.com/DrewBrunning/mycorrhizal-crm/issues/723)).

## [0.6.9] - 2026-09-04

> **Upgrade**: upgrading into v0.6.1 to v0.6.8 from v0.6.0 or older, on an instance with audit history,
> crash-loops at start. Upgrade to v0.6.9 or later directly ("Historical defect" in
> [upgrade compatibility](docs/upgrade-compatibility.md)).

### Changed

- **Faster relationship graph traversal** ([#792](https://github.com/DrewBrunning/mycorrhizal-crm/issues/792)).
- **Go 1.27.1** toolchain.

### Fixed

- **Upgrade crash-loop** while backfilling encrypted fields on databases with audit history.

## [0.6.8] - 2026-09-02

### Added

- **Database health check and `doctor` command** ([#460](https://github.com/DrewBrunning/mycorrhizal-crm/issues/460)).
- **Search index rebuild and consistency checks** ([#461](https://github.com/DrewBrunning/mycorrhizal-crm/issues/461), [#462](https://github.com/DrewBrunning/mycorrhizal-crm/issues/462)).

### Fixed

- **Remote photo links survive a plain save.**

## [0.6.7] - 2026-09-02

### Added

- **Safe concurrent edits** ([#456](https://github.com/DrewBrunning/mycorrhizal-crm/issues/456)): updates use `If-Match`, and `Idempotency-Key` makes retries safe ([#459](https://github.com/DrewBrunning/mycorrhizal-crm/issues/459)).
- **Missed scheduled jobs run once** after downtime ([#526](https://github.com/DrewBrunning/mycorrhizal-crm/issues/526)).

## [0.6.6] - 2026-09-01

> **Upgrade**: migrations now take a verified backup first and refuse to run without one ([#530](https://github.com/DrewBrunning/mycorrhizal-crm/issues/530)).
> Make sure the backup directory (or `MYCORRHIZAL_PRE_MIGRATION_BACKUP_DIR`) is writable before upgrading.

### Added

- **Backup before every migration** ([#530](https://github.com/DrewBrunning/mycorrhizal-crm/issues/530)), restore tests across versions, and documented recovery goals and disaster-recovery boundaries ([#506](https://github.com/DrewBrunning/mycorrhizal-crm/issues/506), [#455](https://github.com/DrewBrunning/mycorrhizal-crm/issues/455)).

## [0.6.5] - 2026-09-01

### Added

- **Import assistants for Monica and Meerkat** on the web ([#549](https://github.com/DrewBrunning/mycorrhizal-crm/issues/549), [#550](https://github.com/DrewBrunning/mycorrhizal-crm/issues/550)).
- **vCard and JSContact `X-` properties** import as custom fields ([#514](https://github.com/DrewBrunning/mycorrhizal-crm/issues/514)).
- **Export loss report** ([#442](https://github.com/DrewBrunning/mycorrhizal-crm/issues/442)): an export says what the target format cannot hold.

## [0.6.4] - 2026-08-30

> **Upgrade**: the supported upgrade floor is v0.6.0. Migrations refuse a dirty, newer or older-than-floor
> database instead of guessing ([#439](https://github.com/DrewBrunning/mycorrhizal-crm/issues/439), [#546](https://github.com/DrewBrunning/mycorrhizal-crm/issues/546)).

### Added

- **Monica and Meerkat import mappings** ([#351](https://github.com/DrewBrunning/mycorrhizal-crm/issues/351), [#353](https://github.com/DrewBrunning/mycorrhizal-crm/issues/353)).

### Fixed

- **Contacts created through the API now have a card UID** ([#693](https://github.com/DrewBrunning/mycorrhizal-crm/issues/693)).

## [0.6.3] - 2026-08-29

### Added

- **Revision tokens for contacts** ([#591](https://github.com/DrewBrunning/mycorrhizal-crm/issues/591)), groundwork for safe concurrent edits.

### Internal

- **Testing**: visual regression, failure injection, property-based and round-trip tests.

## [0.6.2] - 2026-08-28

### Added

- **Observability**: separate liveness, readiness and deep health checks ([#421](https://github.com/DrewBrunning/mycorrhizal-crm/issues/421)), request correlation IDs ([#425](https://github.com/DrewBrunning/mycorrhizal-crm/issues/425)), opt-in `/metrics` ([#389](https://github.com/DrewBrunning/mycorrhizal-crm/issues/389)), an admin system-status page ([#649](https://github.com/DrewBrunning/mycorrhizal-crm/issues/649)) and alerts when a subsystem fails or recovers ([#428](https://github.com/DrewBrunning/mycorrhizal-crm/issues/428)).
- **Update check** ([#650](https://github.com/DrewBrunning/mycorrhizal-crm/issues/650)): opt-in notice of a newer version.
- **Storage growth projection** with thresholds ([#652](https://github.com/DrewBrunning/mycorrhizal-crm/issues/652)).
- **Import history** ([#651](https://github.com/DrewBrunning/mycorrhizal-crm/issues/651)).

### Security

- **Webhook records are trimmed** ([#622](https://github.com/DrewBrunning/mycorrhizal-crm/issues/622)); GORM query logs no longer include personal values ([#621](https://github.com/DrewBrunning/mycorrhizal-crm/issues/621)).
- **Single sign-on cookies follow `COOKIE_SECURE`.**

## [0.6.1] - 2026-08-27

### Added

- **Android: admin user management** ([#348](https://github.com/DrewBrunning/mycorrhizal-crm/issues/348)).
- **CardDAV sync conflicts are shown** when a local edit was overwritten.
- **`security.txt`** at `/.well-known/security.txt` ([#363](https://github.com/DrewBrunning/mycorrhizal-crm/issues/363)).

### Security

- **Weak or default `JWT_SECRET_KEY` stops start-up** ([#393](https://github.com/DrewBrunning/mycorrhizal-crm/issues/393)).
- **Sensitive fields are encrypted at rest** ([#380](https://github.com/DrewBrunning/mycorrhizal-crm/issues/380)), including the Android offline copy ([#385](https://github.com/DrewBrunning/mycorrhizal-crm/issues/385)).
- **Tamper-evident audit log** with a hash chain ([#381](https://github.com/DrewBrunning/mycorrhizal-crm/issues/381)).
- **Session cookies use `SameSite=Strict`**; added `Permissions-Policy` and HSTS headers ([#364](https://github.com/DrewBrunning/mycorrhizal-crm/issues/364)).
- **Log fields are sanitised** and sensitive query values redacted ([#394](https://github.com/DrewBrunning/mycorrhizal-crm/issues/394), [#365](https://github.com/DrewBrunning/mycorrhizal-crm/issues/365)); errors no longer leak panics or database details ([#366](https://github.com/DrewBrunning/mycorrhizal-crm/issues/366)).

## [0.6.0] - 2026-08-22

> **Upgrade**: this release is the first supported-upgrade floor. Upgrading to it from an older
> release takes two steps ([upgrade compatibility](docs/upgrade-compatibility.md#what-happens-below-the-floor)).

### Added

- **Event-driven reach-out suggestions** ([#177](https://github.com/DrewBrunning/mycorrhizal-crm/issues/177)).

### Fixed

- **Android**: gifts section, registration gate and launcher icon fixes.
- **Web accessibility**: Add Contact dialog errors, audit-log target size and text spacing.

### Internal

- **Test and supply-chain tooling**: coverage reporting, SBOM and signed provenance, Dependabot.
