---
name: security-sensitive-change
description: Use when touching authentication, authorization, sessions, uploads, exports, import parsing, secrets, database access, dependencies, or a deployment/trust boundary.
license: MIT
---

# Security-sensitive change

Outcome: the change has a negative test, its ASVS/MASVS row is current, and any
moved trust boundary is written down. Read `docs/security/asvs-l2.md`,
`docs/security/masvs-l1.md` and the [trust-boundaries reference](references/trust-boundaries.md)
before editing.

Prerequisites: the security checklist is a living contract — a security-sensitive
PR updates every row it touches in the same commit. If the change cannot point at
a row, it does not yet know what it is changing.

## Steps

1. Identify the attacker-controlled input and the boundary it crosses (HTTP,
   upload, import, outbound sync, template, log, secret store).
2. Write the negative test first: missing/foreign `user_id`, bad token, expired
   session, path traversal, SSRF target, formula injection, oversized input.
3. Enforce ownership at the data-access boundary (query-scoped by `user_id` or
   `Contact.VCardUID`), not only in the handler.
4. Keep sensitivity filtering in the query for every leaving copy; the three
   full-fidelity exceptions are policy, not leaks.
5. Update the ASVS/MASVS row and citation. If the trust boundary moved, update
   the threat model; if a new persistent copy of user data exists, update the
   retention lifecycle and PII inventory.
6. Run `citecheck` (citations) and `docscheck` (links/structure) when docs moved.
7. Never add `os/exec` to a server or request path; tooling is the scoped
   exception in `docs/security/asvs-l2.md` P12.

## DoD

```bash
cd backend && go test ./middleware/... ./routes/...
cd backend && go run ./cmd/citecheck
cd backend && go run ./cmd/docscheck
```

Report any check you could not run; never imply it passed.

## Stop and ask

- The change weakens an ownership check, a sensitivity filter or a CSRF/auth
  control without a corresponding reviewed row.
- A secret would be logged, exported, committed or passed to an untrusted host.
- A new dependency or executable path crosses the supply-chain boundary.
- The threat model would need a new trust boundary and no ADR covers it.

Canonical detail: `docs/security/asvs-l2.md`, `docs/security/masvs-l1.md`,
`docs/security/threat-model.md`, `docs/security/data-retention-lifecycle.md`.
