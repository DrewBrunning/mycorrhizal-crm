# T50 — vCard 2.1 import produces blank phone/email/photo, and routes to the wrong adapter

| | |
|---|---|
| **Rating** | 5 — real, common case (many phone/Google/Apple exports are still vCard 2.1), not a corner case |
| **Size** | M |
| **Depends on** | [T49](58-T49-vcf-import-merge-corrupts-existing-contact.md) — not a hard blocker (this ticket's fix stands alone), but land T49 first: until it does, a vCard 2.1 import that happens to match an existing contact by name is the easiest real-world way to trigger T49's data loss |
| **Alpha** | n/a — real data exists; this is import-parsing logic, no schema change |
| **Source** | v0.3.0 post-release testing, 2026-08-06 — a real vCard exported from Google Contacts (`VERSION:2.1`) imported with photo absent, phone and email blank, only the name recovered |

## Why this exists — confirmed against the actual library, not assumed

Decoded the real reported file directly through `github.com/emersion/go-vcard` (the library both
`vcard3.Adapter` and `vcard4.Adapter` build on) outside this app's own code, to isolate exactly
where the data goes missing:

```
=== TEL (1 fields) ===
  [0] Value="" Params=map[PREF:[608-514-2711]]
=== EMAIL (1 fields) ===
  [0] Value="" Params=map[HOME:[elizabeth.brunning@gmail.com]]
=== PHOTO (1 fields) ===
  [0] Value="" Params=map[ENCODING:[BASE64] JPEG:[<the entire base64 photo>]]
```

The source file uses vCard **2.1**'s legacy parameter grammar — bare tokens with no `TYPE=`/
`ENCODING=` prefix (`TEL;CELL;PREF:608-514-2711`, `EMAIL;PREF;HOME:...`,
`PHOTO;ENCODING=BASE64;JPEG:...`). `go-vcard`'s decoder — built for vCard 3.0/4.0's
`;TYPE=token,token` grammar — doesn't recognize this shape: it parses each bare token as a
parameter *name* with no explicit value, and the actual property value (the phone number, the
email address, the entire photo) ends up misfiled as if it were the last bare token's parameter
*value* instead. The property's real `Value` comes back empty every time. This is a decoder-level
issue, upstream of either adapter's own logic, which is why `importPhones`/`importEmails`
(`vcard3/adapter.go:616-637`, mirrored in `vcard4`) — both of which correctly skip on
`f.Value == ""` — end up skipping real data they were never actually given a chance to see.

**Compounding, separate issue found in the same investigation:** `sniffVCardVersion`
(`services/import_service.go:96`) only special-cases the literal string `"3.0"` — any other
`VERSION` value, including `"2.1"`, falls through to `vcard4.Adapter{}`. So this file wasn't just
parsed by a decoder that mishandles 2.1's grammar — it was routed to the vCard **4.0** adapter for
a **2.1** document, the largest version gap the app has any adapter for at all. Given the root
parsing failure happens inside the shared `go-vcard` decoder before either adapter's own code runs,
routing to `vcard3` instead wouldn't have fixed this specific file — but the version-sniff itself is
still wrong on its own terms: it silently treats "not exactly 3.0" as "assume 4.0" rather than
recognizing 2.1 as a distinct, older version that neither adapter is actually built for.

## What to build

**Decision made, 2026-08-06: build real vCard 2.1 support, not a reject-and-message path.**
Discussed as part of scoping this ticket — recorded here rather than left as an open menu, since
the "which option" question is settled:

- `go-vcard` has **zero** vCard-2.1 or QUOTED-PRINTABLE awareness anywhere in its source (checked
  directly, not assumed) — there is no library flag to flip; any 2.1 support is hand-built either
  way, so "reject explicitly" isn't actually the cheaper option here.
- Once [T49](58-T49-vcf-import-merge-corrupts-existing-contact.md) lands, a bad/incomplete parse
  can no longer destroy existing data — worst case is a new contact with some fields blank, visible
  and fixable by hand. That removes this ticket from "safety" territory; it's a completeness/UX
  question now, and "silently import less than we could have" is worse UX than actually parsing
  the file correctly.
- vCard 2.1 is not a rare legacy corner case in practice — many phones' native "export/share
  contact" flows still emit it today, unrelated to how old the device is. Telling a user to
  "re-export from something that supports 3.0" may not be something they're able to do at all.
- The actual gap is bounded and well-documented, not open-ended: (1) bare-token `TYPE=`/`ENCODING=`
  parameter grammar, and (2) QUOTED-PRINTABLE text encoding (Go's `mime/quotedprintable` already
  handles the decode — this is wiring, not new logic). Both are finite, testable-against-the-real-
  file pieces of work.

1. **Detect vCard 2.1 explicitly** in `sniffVCardVersion` rather than letting it silently fall
   through to 4.0.
2. **Normalize the bare-token legacy parameter grammar into `TYPE=`/`ENCODING=` form on the raw
   block bytes, before handing it to `go-vcard`'s decoder** — a pre-processing pass, since the
   corruption happens inside the shared decoder, not in either adapter's own code. Add
   QUOTED-PRINTABLE decoding for any property that declares it.
3. **Whatever still can't be recovered after normalization goes through
   `contactmodel.Diagnostic` warnings** (the existing degradation pattern this codebase already
   uses for unmappable data) — normalizing most of the format doesn't mean claiming to handle all
   of it silently.
4. The photo case needs its own explicit check — `extractPhotoFromRecord` (`import_service.go:133`)
   depends on the photo actually landing in the neutral `Record`, which requires the underlying
   property value to be recoverable at all.

## Traps

- Don't fix this by hard-coding a special case for exactly this file's parameter ordering
  (`;CELL;PREF` vs `;PREF;CELL` vs any other combination) — vCard 2.1's grammar allows any number of
  bare tokens in any order; a fix scoped to this one example will look fixed and still fail the next
  real-world 2.1 export.
- Normalize on the raw block bytes for the *whole* vCard, not per-property — `PHOTO`'s bare `JPEG`
  token needs the same treatment as `TEL`'s bare `CELL`/`PREF`, and a property-specific patch will
  miss whichever one wasn't tested against.
- Don't hand-roll QUOTED-PRINTABLE decoding — `mime/quotedprintable` in the stdlib already does
  this correctly; a custom decoder here is unnecessary surface area for a solved problem.
- See [T49](58-T49-vcf-import-merge-corrupts-existing-contact.md) — even after this ticket lands,
  a genuinely malformed or partial import merging into an existing contact must not delete what the
  existing contact already had. That's T49's fix, not this one's, but don't consider this ticket
  "done" as a substitute for it.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green.
- A real-file test using the reported vCard (or an equivalent minimal fixture reproducing the same
  `TEL;CELL;PREF:`/`EMAIL;PREF;HOME:`/`PHOTO;ENCODING=BASE64;JPEG:` shapes) proves phone, email and
  photo all survive import.
- A QUOTED-PRINTABLE-encoded fixture proves that decoding path works too, not just BASE64.
- A property genuinely unrecoverable even after normalization surfaces as a
  `contactmodel.Diagnostic` warning, not silent data loss.
- Hand-verified against the real reported file.
