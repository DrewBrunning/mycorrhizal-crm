# P3 — AI / Ollama layer

| | |
|---|---|
| **Rating** | 1 |
| **Alpha** | after |
| **Source** | `90` D1; split out of the former `37-deferred.md` bundle, 2026-08-06 |

**Not implementation-ready and not meant to be.**

## What this is

Summarization, entity/relationship/life-event extraction, timeline synthesis, memory-curator
suggestions.

Gated on two things: everything structured existing first, and the **propose-then-approve**
workflow — which is already the pattern used by household inference ([T1](09-T1-households.md))
and its suggested-edge review surface. Any AI output must land as a *suggestion* a human confirms,
never as fact.

**`90` D1 is explicit: this is not an AI-first project.** Would revisit D1's storage decision *only*
if vector search proves necessary, and then via an external sidecar — never a primary-store
migration.

## Related deferred items

[P2d](73-P2d-dawarich-geopulse-integration.md)'s location-correlation idea and
[P4](68-P4-local-model-pilot.md)'s local-model code-gen pilot are both adjacent to this — P2d is a
narrower, concrete instance of "cross-system inference" this layer would generalize; P4 is
developer-tooling-flavored and gated on mobile client timing rather than on this layer's own gate.
Keep them as separate tickets; don't fold them in here.

## Done when

N/A — not scheduled. Pulled in only when a concrete need arises, and even then starts with the
propose-then-approve mechanism, not a feature that writes data directly.
