# P2f — Audiobookshelf integration (shared/recommended listening context)

| | |
|---|---|
| **Rating** | 1 |
| **Depends on** | [T14](32-T14-external-link-substrate.md) (done) |
| **Alpha** | after |
| **Source** | `92.7` P2 bucket; split into its own ticket, 2026-08-06 |

**Not implementation-ready — a feature idea, not a scoped plan.** Pulled in only when a concrete
need arises.

## Why this exists (the idea, not a commitment)

Same shape of idea as [P2e](74-P2e-jellyfin-integration.md) (Jellyfin), for a self-hosted
Audiobookshelf instance instead — a book/audiobook a contact recommended, or a shared listening
context. Equally unscoped; recorded so the idea isn't lost, not because it's ready to design.

## What a real design pass would need to settle

- The actual use case — same open question as P2e: what does linking a contact to Audiobookshelf
  data concretely mean before this is worth designing.
- Whether "a contact recommended this book" is better modeled as a `Gift`-adjacent idea/note than a
  new external-system integration — check for overlap with [T20b](28-T20b-gift-tracking.md)'s gift
  tracking before assuming this needs its own substrate wiring.

## Done when

N/A — not scheduled, and not scoped enough to schedule. Re-evaluate if a concrete need arises.
