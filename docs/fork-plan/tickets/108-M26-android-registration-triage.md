# M26 — Registration + circle/tag triage on Android

| | |
|---|---|
| **Rating** | 2 — both real but low-frequency: one-time account creation, one-time legacy cleanup |
| **Source** | [M8](89-M8-web-android-parity-audit.md) audit, 2026-08-11 |
| **Depends on** | Nothing |
| **Status** | TO BE DONE |

Neither surface was marked deliberately-not-on-mobile at M8 sign-off (unlike admin user management,
which was), so both default to build-it per the parity target — filed together because both are
small, self-contained, and low-priority relative to the rest of this board.

## Scope

**Registration** (`RegisterPage.tsx:25-111`) — Android is currently sign-in only:
- Register a new account: username/email/password.
- Link from the login screen to registration (`LoginPage.tsx:104-106`).
- Forgot-password flow: request + confirm reset (`ForgotPasswordDialog.tsx`) — zero Android
  footprint; grouped here since it lives on the same screen as registration on web.

**Circle/Tag triage** (`CircleTagTriagePage.tsx`) — one-time cleanup tool for legacy free-text
circle strings inherited from the meerkat-crm fork:
- Collect distinct legacy-circle strings.
- Reclassify an item (circle / tag / skip), with inline rename before creating.
- Preview classification summary.
- Apply: create circle/tag entities and add members to them.

## Done when

- New-account creation and forgot-password both work from Android's login flow.
- Circle/tag triage is reachable and functionally equivalent to web's two-step (classify → preview
  → apply) flow.
- Hand-verified on-device: register a new account end-to-end; run triage against an instance with
  at least one un-triaged legacy circle string.
- New strings translated in all five locales.
