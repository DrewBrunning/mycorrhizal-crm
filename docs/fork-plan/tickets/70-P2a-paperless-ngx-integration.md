# P2a — Paperless-ngx integration (link contact ↔ documents)

| | |
|---|---|
| **Rating** | 2 |
| **Depends on** | [T14](32-T14-external-link-substrate.md) (done — the `ExternalIdentity`/`ExternalActivity` substrate) |
| **Alpha** | after |
| **Source** | `92.7` P2 bucket; split into its own ticket, 2026-08-06 |

**Not implementation-ready.** One `93-integration-spec-template.md` (`93.3`) instance on top of the
T14 substrate. Pulled in only when a concrete need arises — see `tickets/README.md`'s Deferred
section for why integrations in this bucket sit here rather than being scheduled.

## Why this exists

A document a contact gave you — a contract, a form, a scan — that already lives in a self-hosted
Paperless-ngx instance shouldn't need re-uploading into this app (see [N7](29-N7-attachments.md),
which is the separate "store the file locally" feature — this ticket is *linking to* a
Paperless-ngx-owned document, not storing a copy).

## §93.3 draft (starting point for the eventual design pass, not a commitment)

```
Integration:            Paperless-ngx
External system owns:    the document itself — scanned/uploaded file, OCR text, tags, correspondent
Current level / target:  (none) → L1  (L2 — pulling title/tag/date metadata in — plausible next)
Upstream dependency:     none expected; Paperless-ngx has a documented REST API

Authentication:
  - API token, per-user, user-supplied (Paperless-ngx issues these natively).
  - Stored encrypted (reuse services/credential_crypto.go).

Data imported (external → CRM):
  - Document ID/link         → ExternalIdentity{system:"paperless", external_id, url:<document page>}
  - L2: title, tags, correspondent, added-date → ExternalActivity metadata cache

Data exported (CRM → external):
  - none

Sync direction:          one-way in
Conflict handling:       n/a (read-only linking/enrichment)
User permissions:        user links their own contacts to documents in their own Paperless-ngx instance
Substrate config:        Paperless-ngx base URL + API token ref in ExternalIdentity.metadata
```

## Traps

- SSRF: outbound requests to a user-supplied base URL need `httputil.SafeDialContext`, same as every
  other integration with a user-supplied host.
- Don't duplicate N7's job — this is a link, not a copy. If a real need emerges for uploading *into*
  Paperless-ngx from this app, that's a materially different (write) integration, not an extension
  of this one.

## Done when

N/A — not scheduled. A real design pass (verifying the actual current Paperless-ngx API surface,
not assuming the sketch above is still accurate) happens when this is pulled in.
