# T65 — Wire up circle/tag rename & delete on web

| | |
|---|---|
| **Platform** | Web |
| **Rating** | 3 — cheap; the whole stack already exists except the button |
| **Size** | S — backend, API client and hooks all exist; this is the calling page |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 — reverse-direction "wire it up" finding (structural finding #4) |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

The M8 audit found that Android has genuine standalone circle/tag management (`CirclesScreen`/
`CircleDetailScreen`/`TagsScreen`/`TagDetailScreen`) including rename and delete — and, digging into
why web doesn't, found that web isn't actually missing the capability. It's missing the *button*:

- `PUT`/`DELETE /circles/:id` and `/tags/:id` backend endpoints exist.
- `frontend/src/api/circles.ts:67-84` (`updateCircle`, `deleteCircle`) and
  `frontend/src/api/tags.ts:67-84` (`updateTag`, `deleteTag`) — typed API client functions exist.
- `frontend/src/hooks/useCircles.ts:68-91` (`handleUpdate`, `handleDelete`) and
  `frontend/src/hooks/useTags.ts:71-83` (same) — hook callbacks exist and are correct.
- None of the four pages that use these hooks (`ContactDetailPage.tsx:286-291`,
  `ContactsPage.tsx:39`, `NetworkPage.tsx:37`, `DashboardPage.tsx:50`) destructure or call
  `handleUpdate`/`handleDelete`. There is no rename or delete UI anywhere on web today.

This is the exact same shape as Android's notes/activities gap — implementation done, entry point
missing — just on the other platform. Filed as its own ticket (not folded into M8's Android tickets)
because it's frontend-only work with no Android dependency.

## Scope

Web currently manages circles/tags only as a side effect of editing a contact
(`ContactHeader.tsx:582-663`) — there's no page whose subject is "this circle" or "this tag" the way
Android's `CircleDetailScreen`/`TagDetailScreen` are. Two ways to close this, pick one at
implementation time:

1. **Minimal**: add rename/delete affordances wherever circles/tags are already surfaced (e.g. the
   circle/tag filter dropdown on `ContactsPage.tsx`, or the chip editor on `ContactHeader.tsx`).
2. **Matches Android's shape**: add standalone `/circles` and `/tags` list+detail pages, giving web
   the same "browse all circles/tags, see all members, rename, delete" capability Android already
   has — this would also close the reverse-direction gap M8 recorded (Android leads here) rather
   than just adding rename/delete to the existing side-effect-only surfaces.

Recommend deciding between these two during implementation rather than here — it's a small enough
scope either way that a quick call at the time is fine.

## Done when

- Renaming a circle/tag from web works and is visible on Android afterward (and vice versa).
- Deleting a circle/tag from web works, with confirmation, and removes it from all members.
- Hand-verified: rename/delete both directions across platforms.
