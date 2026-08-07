# P2b — Seafile integration (link contact ↔ files/folders)

| | |
|---|---|
| **Rating** | 2 |
| **Depends on** | [T14](32-T14-external-link-substrate.md) (done — the `ExternalIdentity`/`ExternalActivity` substrate) |
| **Alpha** | after |
| **Source** | `92.7` P2 bucket; split into its own ticket, 2026-08-06 |

**Not implementation-ready.** One `93-integration-spec-template.md` (`93.3`) instance on top of the
T14 substrate. Pulled in only when a concrete need arises.

## Why this exists

A shared folder or a specific file a contact should have access to, or that's about them, already
managed in a self-hosted Seafile library — link to it from the contact rather than duplicating
storage (see [N7](29-N7-attachments.md) for the separate local-storage attachments feature).

## §93.3 draft (starting point for the eventual design pass, not a commitment)

```
Integration:            Seafile
External system owns:    the file/folder and its version history
Current level / target:  (none) → L1  (L2 — file size/last-modified enrichment — plausible next)
Upstream dependency:     none expected; Seafile has a documented Web API

Authentication:
  - API token, per-user, user-supplied (Seafile issues these natively).
  - Stored encrypted (reuse services/credential_crypto.go).

Data imported (external → CRM):
  - File/folder link          → ExternalIdentity{system:"seafile", external_id, url:<share link>}
  - L2: filename, size, last-modified → ExternalActivity metadata cache

Data exported (CRM → external):
  - none

Sync direction:          one-way in
Conflict handling:       n/a (read-only linking/enrichment)
User permissions:        user links their own contacts to files/folders in their own Seafile account
Substrate config:        Seafile server URL + API token ref in ExternalIdentity.metadata
```

## Traps

- SSRF: outbound requests to a user-supplied base URL need `httputil.SafeDialContext`.
- Seafile share links can themselves carry a password/expiry — decide during the design pass whether
  the CRM stores a raw share link (simplest, but the link itself may be sensitive) or the
  file/folder's internal repo-relative path plus a token to resolve it at view time.

## Done when

N/A — not scheduled. A real design pass (verifying the actual current Seafile Web API surface, not
assuming the sketch above is still accurate) happens when this is pulled in.
