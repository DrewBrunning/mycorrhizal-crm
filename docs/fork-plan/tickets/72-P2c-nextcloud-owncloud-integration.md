# P2c — Nextcloud / ownCloud integration (WebDAV, link contact ↔ files)

| | |
|---|---|
| **Rating** | 2 |
| **Depends on** | [T14](32-T14-external-link-substrate.md) (done — the `ExternalIdentity`/`ExternalActivity` substrate) |
| **Alpha** | after |
| **Source** | `92.7` P2 bucket; split into its own ticket, 2026-08-06 |

**Not implementation-ready.** One `93-integration-spec-template.md` (`93.3`) instance on top of the
T14 substrate. Pulled in only when a concrete need arises.

## Why this exists

The same file-linking use case as [P2a](70-P2a-paperless-ngx-integration.md)/
[P2b](71-P2b-seafile-integration.md), for the far more common case of a self-hosted Nextcloud or
ownCloud instance — reached via **WebDAV**, not a bespoke REST API, which is a genuinely different
integration shape from the other two (standard protocol, not an app-specific API surface).

## §93.3 draft (starting point for the eventual design pass, not a commitment)

```
Integration:            Nextcloud / ownCloud
External system owns:    the file/folder and its version history
Current level / target:  (none) → L1  (L2 — file size/last-modified via WebDAV PROPFIND — plausible next)
Upstream dependency:     none — both speak standard WebDAV; no app-specific API needed for L1-L2

Authentication:
  - App password (Nextcloud/ownCloud's own scoped-credential mechanism), per-user, user-supplied —
    NOT the user's real account password; app passwords are the correct, revocable credential shape
    for this and should be the only form this integration accepts.
  - Stored encrypted (reuse services/credential_crypto.go).

Data imported (external → CRM):
  - File/folder link          → ExternalIdentity{system:"nextcloud", external_id:<WebDAV path>, url:<share link or direct path>}
  - L2: size, last-modified via WebDAV PROPFIND → ExternalActivity metadata cache

Data exported (CRM → external):
  - none

Sync direction:          one-way in
Conflict handling:       n/a (read-only linking/enrichment)
User permissions:        user links their own contacts to files/folders in their own instance
Substrate config:        server base URL + WebDAV path + app-password ref in ExternalIdentity.metadata
```

## Traps

- SSRF: outbound requests to a user-supplied base URL need `httputil.SafeDialContext` — same as
  every other user-supplied-host integration, but worth calling out explicitly here since WebDAV
  PROPFIND requests are a less-common request shape than the JSON-REST calls the other integrations
  in this bucket use, and any SSRF-guard code written generically for "GET a JSON URL" may not
  automatically cover it.
- Nextcloud and ownCloud have diverged since their fork; confirm during the design pass whether one
  shared WebDAV client suffices for both or whether real behavioral differences require branching.
- Only accept app passwords, never the account password — this is a hard requirement, not a
  preference (see the credential-entry safety rules this project already follows elsewhere, e.g.
  Immich/Gotify/calendar credentials are all user-supplied tokens, never the CRM entering a
  password on the user's behalf).

## Done when

N/A — not scheduled. A real design pass happens when this is pulled in.
