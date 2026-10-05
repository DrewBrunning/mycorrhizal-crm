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

Create and revoke tokens under **Settings → API Tokens**. A full-scope token is required; a
CardDAV-scoped token is rejected, and so is a Basic credential. The endpoint runs behind the same
`AuthMiddleware`, rate limit and CORS policy as `/api/v1`, and every tool runs as the token's owner.

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
The token already belongs to you, so this is the same opt-in the REST API offers — not a new trust
boundary.

## Availability

The MCP endpoint is a network surface, so it is **omitted in embedded mode** (the on-device, storage-only
deployment; ADR 0028). It is present whenever the server runs normally.

## Related

- [API reference](api-reference.md)
- [Architecture decision 0032 — MCP server](adrs/0032-mcp-server.md)
