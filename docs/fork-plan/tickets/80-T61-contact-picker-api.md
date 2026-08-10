# T61 — W3C Contact Picker API for PWA contact import

| | |
|---|---|
| **Rating** | 1 — nice UX improvement for a narrow audience, but strictly supplemental to the native Android path |
| **Depends on** | — |
| **Alpha** | after |
| **Source** | M1 mobile design pass, 2026-08-09 |

**Not implementation-ready — a feature idea, not a scoped plan.** Pulled in only when a concrete
need arises.

## Why this exists (the idea, not a commitment)

The existing import flows (file upload: CSV, VCF, JSContact) work everywhere but require the user to
export contacts to a file first — a friction point on mobile where the source of truth is the device's
own Contacts app. The W3C Contact Picker API (`navigator.contacts.select()`) lets a PWA read
selected contacts directly from the device without a file round-trip, and without requesting the
broad `READ_CONTACTS` permission (the browser mediates access).

The native Android client solves this more thoroughly via `ContactsContract` (see
[M1](67-M1-mobile-android-app.md)), but not every user will install a native app. This ticket exists
for the PWA-only user who wants a quicker import path than exporting a `.vcf` first.

## What it would look like

1. On the import page, detect `'contacts' in navigator && 'ContactsManager' in window`
2. Call `navigator.contacts.select(['name', 'email', 'tel', 'address', 'icon'], { multiple: true })`
3. Map the returned `ContactInfo[]` (name, email, tel, address, icon) to `Card` fields — same mapping
   logic as the existing VCF import path
4. Send to `POST /api/v1/contacts` (or a batch endpoint if T57 lands) with the same deduplication
   preview as the existing import wizard

## Constraints

- **Browser support**: Chrome on Android only (as of 2026). Safari, Firefox, and desktop Chrome do
  not implement it. No polyfill exists — the API requires OS-level contact access that only Android's
  WebView/Chrome integration exposes.
- **Field surface is limited**: the API returns `name` (string array), `email` (string array), `tel`
  (string array), `address` (array of `PaymentAddress`), and `icon` (blob). No structured name parts,
  no organization, no birthday, no IM handles. This is a subset of what VCF import or
  `ContactsContract.Data` can provide — acceptable for a quick-add path, not a full import.
- **No LOOKUP_KEY**: the API doesn't expose persistent device contact IDs, so cross-reference with
  the native Contacts app (T57's "Open in Contacts" action) isn't possible through this path alone.
- **Permission model is different**: the browser prompt is per-invocation and transient — the user
  picks contacts each time, there's no persistent grant. Good for privacy, bad for "sync my contacts
  periodically."

## Why it's rated 1 and not higher

The audience is users who (a) are on Android, (b) use the PWA rather than the native app, and (c)
want a faster import path than file upload. That's a narrow intersection — Android + PWA + impatience.
The native Android client covers this use case with a strictly richer implementation, and the
file-upload path works everywhere. This is a PWA polish item, not a capability gap.

## Done when

N/A — not scheduled. Re-evaluate if PWA usage patterns (or user feedback) show demand for a
file-free import path that the native app isn't covering.
