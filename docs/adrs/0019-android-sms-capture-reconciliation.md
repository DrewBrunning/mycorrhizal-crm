# ADR 0019: Android SMS capture reconciliation — accept the broadcast-only gap, target an `_id` cursor

- **Status:** accepted
- **Date:** 2026-09-17
- **Implements:** issue #1124 ("make the decision explicit" ticket, following the #945 pattern).
- **Feeds:** #1127 (the reconciliation worker itself, split off per this ADR's decision). Related:
  #721 (the original tracking-capture design), #1029 (capture-policy filtering), #1123 (the
  Sent-folder backfill watermark bug this ADR's target design deliberately avoids repeating).

## Context

Android's incoming-SMS capture has exactly one path: the live `SMS_RECEIVED` broadcast
(`SmsReceiver.kt`), registered in `AndroidManifest.xml:123-130`. `SmsHistoryReader.kt`'s doc
comment (issue #721) already states the tradeoff this ADR formalizes: the reader deliberately
never touches the Inbox, because

- the broadcast's PDU-header timestamp and the provider row's `DATE` column are two different
  clocks, so a timestamp watermark can't reliably tell "already captured by the broadcast" apart
  from "new since last Inbox read," and
- the local outbox deletes a `PendingInteraction` row once it syncs to the server, so by the time
  a periodic Inbox read would run, there is nothing left locally to dedupe a re-read against.

In practice this means any missed broadcast is a **permanent** loss: Doze-deferred delivery, the
app force-stopped or not yet running at boot, or SMS permission granted after a message already
arrived. There is no recovery path today.

Separately, MMS and group texts are entirely uncaptured — `AndroidManifest.xml:128` registers only
`Telephony.Sms.Intents.SMS_RECEIVED_ACTION`, with no `WAP_PUSH_RECEIVED_ACTION` receiver or MMS
provider reader anywhere in `feature/tracking`.

## Decision

1. **The target design, when built, reconciles by the provider's stable per-row `_id`
   (`Telephony.Sms._ID`), not by timestamp.** A monotonic integer cursor sidesteps both problems
   above at once: it is unambiguous across the broadcast/provider clock mismatch (the reconciler
   simply never re-reads a row at or below the highest `_id` `SmsReceiver` has already seen,
   regardless of what timestamp that row carries), and it needs no dedupe target to survive in the
   outbox, because the cursor itself *is* the record of what has already been handled. This
   directly answers the "no watermark or exact-match dedupe can reliably stop a provider Inbox
   read from double-logging" concern `SmsHistoryReader.kt` documents.
2. **The reconciliation worker is a separate, later piece of work (#1127), not part of this
   decision's own commit.** Building it correctly needs its own review: a new persisted cursor
   field, a new periodic `WorkManager` job, and — per #1123's lesson from the same subsystem — a
   paged (not single-batch) Inbox read so a large backlog can't silently truncate to "the newest
   N" the way the Sent-folder backfill did before that fix. That is real design surface, not a
   drop-in change, so it is scoped and tracked on its own rather than rushed into this ticket.
3. **Until #1127 ships, the gap is accepted, not silently present.** This ADR is the record that
   the maintainer has seen the tradeoff and chosen to ship v0.8.6 without closing it, rather than
   the gap being an unexamined leftover. A missed broadcast still means a permanently-lost
   interaction until the reconciliation worker lands.
4. **MMS/group-text capture is out of scope of both this decision and #1127.** It is a materially
   larger addition (`WAP_PUSH_RECEIVED_ACTION`, the MMS provider, group-thread membership) that
   should only be scoped once the SMS reconciliation approach here has shipped and proven out,
   per #1124's own instruction — filed as its own issue at that point, not before.

### Alternatives considered

- **Timestamp-based Inbox reconciliation** (what `SmsHistoryReader` already documents rejecting
  for outgoing texts, extended to incoming): rejected for the reasons in Context — two clocks, no
  surviving dedupe target once a row syncs and its outbox entry is deleted.
- **Elevate the broadcast receiver's intent-filter priority** to reduce the odds of a missed
  broadcast: does not address the *un-recoverable* cases (permission granted after the fact, app
  not yet installed/running at boot) and was not pursued as a substitute for reconciliation.
- **Ship the reconciliation worker inside this same ticket:** rejected — #1124 itself asks for a
  "deliberate reconciliation design, not a drop-in change," and CLAUDE.md's workflow cadence
  (research → plan → approve → implement → verify) applies to real design decisions like this one;
  bundling it into a four-issue cleanup PR alongside unrelated fixes would skip that review.

## Consequences

- No code changes ship with this ADR beyond the decision record and the regression test below —
  `SmsReceiver`/`SmsHistoryReader`'s current broadcast-only behavior is unchanged.
- The gap is now a documented, tracked position (this ADR + #1127) rather than an implicit one —
  matching the #945 precedent for "make the decision explicit" tickets.
- #1127 carries the actual implementation, and explicitly inherits #1123's paging lesson (ASC +
  bounded pages, not a single DESC/newest-N read) so the reconciliation worker doesn't reintroduce
  the same class of bug in a new provider read.
- MMS support stays unimplemented and untracked as its own issue until #1127 ships.
