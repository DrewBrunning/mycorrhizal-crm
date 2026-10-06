---
title: MCP Server
nav_order: 22
has_children: false
---

# MCP Server

Mycorrhizal CRM exposes a read-only [Model Context Protocol](https://modelcontextprotocol.io) server so an
external AI assistant (Claude, Cursor, and similar) can read your CRM data. The endpoint is the
streamable-HTTP URL:

```
https://<your-instance>/mcp
```

It is the same path in both the all-in-one image and the split `-frontend` image; nginx proxies it to the
backend exactly like `/api/`. The backend registers it at `POST /mcp` only — it is not part of `/api/v1`
and is not described in `backend/openapi.yaml`.

## Authentication

MCP uses the **same per-user API token** as the REST API, sent as a bearer credential:

```
Authorization: Bearer mycorrhizal_<...>
```

Create and revoke tokens under **Settings → API Tokens**. The endpoint runs behind the same
`AuthMiddleware`, rate limit and CORS policy as `/api/v1`, and every tool runs as the credential's owner.

| Credential | Result |
|---|---|
| Full-scope API token as `Authorization: Bearer mycorrhizal_...` | accepted |
| A browser session (the `auth_token` cookie, or a session JWT as a bearer token) | accepted, exactly as on `/api/v1` — the cookie takes precedence over an `Authorization` header when both are sent |
| CardDAV-scoped API token | rejected, `403` |
| HTTP Basic credential | rejected, `401` (the header must start with `Bearer `) |
| No credential, or an expired/revoked token | rejected, `401` |

An MCP client should use a dedicated full-scope API token so it can be revoked on its own.

## Tools

Four read-only tools, matching the REST read endpoints:

| Tool | Equivalent REST endpoint |
|---|---|
| `search_contacts` | `GET /api/v1/search` |
| `get_contact` | `GET /api/v1/contacts/:id/detail` |
| `list_timeline` | `GET /api/v1/contacts/:id/timeline` |
| `run_cadence_report` | `GET /api/v1/cadence-policies/overdue` |

There are no write tools in v1. Limits are clamped server-side, never rejected.

## Sensitivity

Every tool accepts an optional `include_sensitive` boolean, **default `false`**. As with the REST API,
`private` and `secret` data is withheld unless the caller explicitly passes `include_sensitive: true`.

- `get_contact` and `list_timeline` withhold private/secret relationships, addresses and timeline items.
- `search_contacts` does not return a contact whose only match is a private/secret address (the search
  index covers every address, so returning it would confirm that such an address exists). A contact
  that carries a private/secret address is still found by its name, email, phone or organisation, but
  not by its normal-sensitivity address text.
- `run_cadence_report` carries no sensitivity-tiered fields (contact display fields and dates only), so
  the flag is accepted for uniformity and changes nothing.
The token already belongs to you, so this is the same opt-in the REST API offers — not a new trust
boundary.

## Availability

The MCP endpoint is a network surface, so it is **omitted in embedded mode** (the on-device, storage-only
deployment; ADR 0028). It is present whenever the server runs normally.

## Related

- [API reference](api-reference.md)
- [Architecture decision 0032 — MCP server](adrs/0032-mcp-server.md)
