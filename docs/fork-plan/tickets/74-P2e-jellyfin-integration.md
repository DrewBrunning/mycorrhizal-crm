# P2e — Jellyfin integration (shared media / watch-together context)

| | |
|---|---|
| **Rating** | 1 |
| **Depends on** | [T14](32-T14-external-link-substrate.md) (done) |
| **Alpha** | after |
| **Source** | `92.7` P2 bucket; split into its own ticket, 2026-08-06 |

**Not implementation-ready — a feature idea, not a scoped plan.** Pulled in only when a concrete
need arises.

## Why this exists (the idea, not a commitment)

The least-defined idea in this bucket: something connecting a self-hosted Jellyfin instance to
contacts — plausibly "shared a Jellyfin user account/library with this contact" as a simple L1 link
(`ExternalIdentity`), or a note that you two watched something together as an `Activity`. Neither is
scoped; this ticket exists so the idea isn't lost, not because it's ready to design.

## What a real design pass would need to settle

- What the actual use case is — this needs a concrete "someone wants to do X" before it's worth
  scoping past a name in a list.
- If it turns out to be about tracking shared-viewing activity, whether that's really an `Activity`
  type extension rather than a new `ExternalIdentity` integration at all — worth checking existing
  patterns (`Activity.Type` already includes `message`; a `watched-together` type might be a smaller
  change than a full integration).

## Done when

N/A — not scheduled, and not scoped enough to schedule. Re-evaluate if a concrete need arises.
