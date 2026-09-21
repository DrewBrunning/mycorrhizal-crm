---
title: FAQ & Troubleshooting
nav_order: 11
has_children: false
---

# FAQ & Troubleshooting

## Start here

Most operational problems are one of the following; each link is the runbook or
page that names the symptom and the fix.

- **You are not sure where to look** — run the one-pass diagnostics sweep,
  `GET /admin/diagnostics` (admin-only), described in
  [Observability](operations/observability.md).
- **The server will not start, or refuses to migrate a database** —
  [Migration recovery runbook](operations/migration-recovery.md) and
  [Upgrade compatibility](upgrade-compatibility.md) (the below-floor and
  dirty-schema refusals).
- **You need to restore from a backup, or recover after disk failure or
  corruption** — [Disaster recovery boundaries](operations/disaster-recovery.md).
- **An integration is failing** (CardDAV/CalDAV sync, Immich, Paperless,
  Seafile, WebDAV, email, notifications, OIDC) — the per-integration symptoms
  and diagnostic path are in
  [Integrations: ownership and diagnostics](integration-ownership.md).
- **Search results are wrong or missing after a restore or import** —
  [Rebuilding the full-text search index](operations/search-index.md).
- **A client shows a stale app or a version error** —
  [Client/server compatibility policy](client-compatibility-policy.md) and
  [Service-worker updates](service-worker-updates.md).

## Common configuration problems

### When registering a new user or signing in I get the error `Failed to execute 'json' on 'Response': Unexpected end of JSON input`.

**Answer**: Make sure to set the environment variable `FRONTEND_URL` in your .env to the correct URL.


### When logging in it works but then redirects to login again.

**Answer**: Make sure to change the `COOKIE_DOMAIN` environment variable in your .env to your host IP.

### When using an OIDC login provider the user is not found. 

**Answer**: The OIDC provider has to either set the property email_verified=true or you have to set the OIDC_TRUST_EMAIL=true environment variable.

