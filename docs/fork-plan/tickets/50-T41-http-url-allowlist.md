# T41 — Web-link fields should allow http(s) only, not just deny four bad schemes

| | |
|---|---|
| **Rating** | 3 — defense-in-depth hardening of a guard that already blocks the known attacks |
| **Size** | S |
| **Depends on** | [T34](43-T34-contact-field-linking.md) (done — owns the shared validator), [T35](44-T35-gift-tracking-gaps.md) (done — added the most recent field to the set) |
| **Alpha** | n/a — real data exists, but this is a **write-path** change only: no migration, no backfill, nothing revalidated on read. See the trap about existing values failing on their next edit |
| **Source** | Review finding on the T35 branch |

## Why this exists

`safeurl` (`backend/middleware/validation.go`'s `validateSafeURL`) and its client-side mirror
`isSafeUrlString` (`frontend/src/utils/linkResolution.ts`) are **blocklists**: they normalize the
value (strip everything ≤ `U+0020`, which is what defeats the `java&Tab;script:` bypass, then
lowercase), read the scheme up to the first `:`, and reject exactly four —
`javascript`, `data`, `vbscript`, `file`. Everything else passes.

That is correct as far as it goes, and the normalize step is the part most implementations get
wrong. But "everything else" includes `blob:`, `intent:` (launches an installed Android app with a
payload), `ms-msdt:` (the Follina class), `view-source:`, `about:`, and **every scheme invented
after the list was written**. A blocklist is a list of yesterday's attacks; anything unknown is
allowed by default. For a field that means "a web page" — a gift's product link, an agenda item's
reference article — the honest constraint is `http`/`https`, and then unknown schemes are denied by
default.

The threat model is genuinely narrow, which is why this is R3 and not higher: these fields are
written by the account owner about their own contacts and clicked by that same person, the links
already carry `rel="noopener noreferrer"`, and browsers prompt before handing off to an external
protocol handler. The paths where someone *else's* string reaches one of these fields are CardDAV
sync, vCard/JSContact import, and [P1](31-P1-contact-sharing.md) contact sharing — real, but not
the classic stored-XSS-hits-every-user case.

## What to build

A **second** validator alongside `safeurl`, not a change to `safeurl` itself (see the first trap).

1. **Backend:** register `httpurl` in `middleware/validation.go` next to `safeurl`. Same normalize
   step — factor it out rather than copy-pasting it, so the two can't drift — then accept only
   `http` and `https`, reject everything else including a value with no scheme at all. Add its
   message to `formatValidationError`'s switch ("… must be an http:// or https:// URL").

2. **Frontend:** the matching `isHttpUrlString` in `utils/linkResolution.ts`, sharing the same
   normalize helper as `isSafeUrlString` for the same reason.

3. **Apply it to the fields that are unambiguously web links**, replacing `safeurl` on each:
   - `Gift.URL` (`models/gift.go`) + `GiftInput.URL` (`models/dtos.go`)
   - `ConversationAgenda.ReferenceURL` (`models/conversation_agenda.go`) + its DTO
   - `ExternalIdentity.URL` (`models/external_identity.go`, both the model and the input DTO)
   - `ImmichConfig.BaseURL` (`models/immich_config.go`) — already http-only in practice, and it is
     the one field here also fetched server-side (through the SSRF-guarded dialer), so it benefits
     most.

4. **Client-side pre-checks.** `GiftDialog.tsx` is currently the *only* dialog that pre-checks a
   URL before saving (T35 added it) — the agenda, external-link and Immich surfaces just let the
   API's 400 come back. Switch GiftDialog's check to `isHttpUrlString`; adding matching pre-checks
   to the others is optional polish, since the guarantee is enforced server-side either way.

5. **Fix the unguarded render sites**, found while writing this ticket. `GiftList.tsx` and
   `ContactInformation.tsx` check `looksLikeAbsoluteUri && isSafeUrlString` before building an
   `<a href>`; `ConversationAgendaList.tsx` (the `reference_url` caption) and `ExternalLinkPanel.tsx`
   (both `<Link href={identity.url}>` sites, Immich and generic) do **not** — they build the href
   straight from the stored value and rely entirely on write-time validation. A value that predates
   the validator, or arrives through CardDAV/vCard import rather than the API, is therefore rendered
   unchecked today. Give them the same guard. This is worth doing even if the rest of the ticket is
   deferred.

6. **Leave `safeurl` exactly as it is** for `Card.Links` / IMPP values (`models/contact.go`) and
   `LinkFieldType.Protocol` (`models/dtos.go`), plus `custom_field_service.go`'s `uri,safeurl`
   check for url-typed custom fields.

## Traps

- **Do not tighten `safeurl` itself.** It is shared by field types that legitimately carry
  non-http schemes: `Card.Links`/IMPP values are `xmpp:`, `matrix:`, `sip:`, `mailto:`, `tel:`, and
  T34's `LinkFieldType.Protocol` is a *user-editable* template registry where someone may
  reasonably add `tel:{value}` or an app's own scheme. Narrowing the shared validator breaks all of
  those.

- **Existing stored values are not revalidated on read, but they will fail on their next edit.**
  A `mailto:` sitting in a `ConversationAgenda.ReferenceURL` today keeps rendering fine and keeps
  syncing fine; the moment the user opens that item and saves it, the write is rejected. Decide
  this deliberately: either accept it (the value is rare and the message is clear) or run a
  one-time query to find non-http values in the four affected columns before landing, so the
  decision is made against a real count rather than a guess. Do **not** silently rewrite stored
  data to make it conform.

- **The render-time guard is separate and stays.** A stored value can predate any validator, and a
  T34 `{value}` template only becomes a full URL at render time. Swapping the *write* validator
  does not remove the need for the *render* check — but the render check on the web-link fields
  should use `isHttpUrlString` too, so the two agree. Note the asymmetry this ticket has to
  resolve: `Card.Links`/IMPP keep `isSafeUrlString` (they are legitimately non-http), so after this
  lands the codebase has two render guards and each site must use the right one.

- **Frontend enum/registry mirrors** — per `CLAUDE.md`'s frontend trap 4, the client-side validator
  is a hand-maintained mirror of the backend one. Keep the comment that says so, and make it point
  at the new function.

- T35 already normalizes a scheme-less gift URL to `https://` on save (`GiftDialog.tsx`), so that
  field's ordinary typed path already produces a conforming value. Don't remove that — it is what
  keeps the stricter validator from rejecting the most natural thing a user types.

## Done when

- `go build ./... && go vet ./... && gofmt -l . && go test ./...` green, with table-driven tests
  for `httpurl` covering: `http`/`https` accepted; `javascript`/`data`/`vbscript`/`file`/`blob`/
  `intent`/`ms-msdt` rejected; the `java&Tab;script:` obfuscation rejected; a scheme-less value
  rejected; empty accepted (fields opt into `required` separately).
- A real-DB controller test proves at least one of the four fields returns 400 for a
  previously-accepted non-http scheme.
- `npx tsc --noEmit` clean, `npx vitest run` green, with `isHttpUrlString` unit tests mirroring the
  Go table exactly — the two implementations disagreeing is the actual risk here.
- A component test proves each previously-unguarded render site (agenda `reference_url`, both
  `ExternalLinkPanel` link sites) shows an unsafe stored value as text rather than as an `<a href>`
  — the same shape as `GiftList.test.tsx`'s "an unsafe-scheme URL is shown as text" case.
- All 5 locale files have real translations for any new validation message.
- The decision about pre-existing non-http values is recorded in this ticket's landing note, with
  the count it was made against.
