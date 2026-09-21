---
title: CardDAV Sync
nav_order: 4
has_children: false
---

# Contact Sync (CardDAV)

The built-in CardDAV server allows you to synchronize your contacts with your mobile device or computer (e.g. Apple Contacts in iPhone macOS Contacts or on Android with a third-party CardDAV client like DAVx⁵). This page is the *served* direction — your instance's contacts going *out* to your devices. The *subscription* direction — pulling contacts **in** from someone else's CardDAV server — is the "CardDAV contact sync" integration; who owns it when it breaks and how to diagnose it is in [Integrations: ownership, diagnostics, and what breaks](integration-ownership.md#carddav).

Enable CardDAV by setting the `CARDDAV_ENABLED` environment variable to `true`.  Once enabled, the CardDAV server runs alongside the web interface. A standard discovery endpoint is available at `/.well-known/carddav` for automatic configuration.

## Connecting Your Phone

### iOS

1. Open **Settings** > **Contacts** > **Accounts** > **Add Account** > **Other**.
2. Select **Add CardDAV Account**.
3. Enter the following:
   - **Server**: Your Mycorrhizal CRM URL (e.g., `mycorrhizal.example.com`)
   - **User Name**: Your Mycorrhizal CRM username or email
   - **Password**: Your Mycorrhizal CRM password
4. Tap **Next**. iOS will automatically discover the CardDAV endpoint.
5. Your contacts will begin syncing.

### Android

Android does not include a native CardDAV client. You will need a third-party app such as [DAVx5](https://www.davx5.com/) (open source):

1. Install **DAVx5** from F-Droid or Google Play.
2. Open DAVx5 and add a new account.
3. Select **Login with URL and user name**.
4. Enter:
   - **Base URL**: Your Mycorrhizal CRM URL followed by `/carddav/` (e.g., `https://mycorrhizal.example.com/carddav/`)
   - **User name**: Your Mycorrhizal CRM username or email
   - **Password**: Your Mycorrhizal CRM password
5. DAVx5 will detect the address book. Select it and sync.
6. Your Mycorrhizal CRM contacts will appear in your phone's Contacts app.

## Sync Behavior

- **Two-way sync**: Changes made in Mycorrhizal CRM appear on your phone, and changes made on your phone are synced back to Mycorrhizal CRM. This also applies to profile pictures.
- **Conflict detection**: Mycorrhizal CRM uses ETags to detect conflicts. If a contact has been modified on both the server and the client since the last sync, the client will be notified and can resolve the conflict.
- **Supported fields**: Mycorrhizal CRM syncs all fields though now all fields might be visible in your client. In case you add additional fields on your client (like a secondary address) the fields will be preserved in the Mycorrhizal database but will not show in the Mycorrhizal CRM frontend.

## Troubleshooting

- **Contacts not syncing**: Verify that `CARDDAV_ENABLED=true` is set in your server environment and restart the application.
- **Discovery not working**: Some clients require the full CardDAV URL instead of relying on auto-discovery. Try entering `https://your-server.com/carddav/` directly as the server URL.
- **Locked out**: After multiple failed login attempts, your account may be temporarily locked. Wait a few minutes and try again, or reset your password via the web interface.

## Interoperability limitations

This is the operator-facing statement of what the CardDAV/CalDAV interoperability
claim does and does not cover. The engineering evidence — the per-client manual
matrix, the divergence registers for each reference server, and the reference
implementations the automated legs run against — lives in the development docs
([reference-client-matrix.md](development/reference-client-matrix.md) and
[testing.md](development/testing.md)).

- **One address book by design.** The server exposes a single address book per
  user. A client that tries to create a second one (`MKCOL`) is refused with
  `403` — the interop-correct "single address book" answer, not a server error.
- **No CardDAV `sync-token`.** go-webdav does not implement it, so clients fall
  back to a `PROPFIND` of depth 1 and compare locally. Incremental sync
  therefore costs a full listing of the collection.
- **CalDAV is read-only.** A client subscribes to a calendar of activities and
  life events; it never writes back.
- **`address-data` version negotiation is not supported on the client side.**
  Our outbound CardDAV client (used by remote address-book subscriptions)
  cannot request a vCard version, so against SabreDAV-based servers (Baikal,
  Nextcloud) a full refetch receives vCard 3.0 re-serializations. The
  per-server divergence registers in
  [testing.md](development/testing.md) pin exactly what each server does.
- **Automated client coverage is provision + pull.** The automated DAVx5 leg
  covers account setup and pulling the canonical pathological fixture; push,
  incremental re-sync, and an explicit version-negotiation assertion are not
  yet automated. Apple Contacts (macOS/iOS) and Thunderbird are out of scope
  for the automated claim — no CI-drivable harness exists for either. See
  [reference-client-matrix.md](development/reference-client-matrix.md).
- **Field-level fidelity is governed by the DATA-01 matrix.** Which canonical
  fields survive a round trip through each format — and which are unsupported
  or lossy — is stated per field in the
  [field compatibility matrix](data-01-field-compatibility-matrix.md) and
  surfaced to the user as export-loss reports (DATA-02).
