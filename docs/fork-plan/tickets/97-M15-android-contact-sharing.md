# M15 — Contact sharing (P1) on Android

| | |
|---|---|
| **Rating** | 3 |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing — P1's backend already exists and serves web today |
| **Status** | TO BE DONE |

`/shares` (`ContactSharesPage.tsx`) has zero Android footprint, confirmed by a repo-wide search —
not just the standalone page but also the entry point on a contact's own header
(`ContactHeader.tsx:374-379` → `ShareContactDialog.tsx`).

## Scope (mirrors `ContactSharesPage.tsx` + `ShareContactDialog.tsx`)

- View incoming shares list.
- View outgoing shares list.
- Accept an incoming share (preview then confirm).
- Decline an incoming share.
- Initiate an outgoing share from a contact's own detail screen ("Share this contact").

## Done when

- All five actions above work on Android and round-trip with web (share from web, accept on
  Android and vice versa).
- Entry point exists both as a standalone shares screen (drawer-reachable) and from
  `ContactDetailScreen`'s header/menu.
- Hand-verified on-device with a second test account, sharing in both directions.
- New strings translated in all five locales.
