package com.mycorrhizal.crm.data.local

import androidx.room.Entity
import androidx.room.PrimaryKey

/**
 * A device interaction staged for optional server sync (Phase 4 §6.1/6.2).
 * Call/SMS tracking writes a row here instead of calling the server directly,
 * so the user can review (and the sync worker can batch) them. Once synced
 * into an Activity, [synced] is set and the row is cleaned up.
 *
 * The SMS body is deliberately NOT stored: only the sender phone + timestamp
 * are captured (§6.2 privacy boundary — the server never sees message content).
 *
 * ANDROID-02 (issue #479): [idempotencyKey] is the row's per-row CON-04 retry
 * key (ADR-0010), generated at record time and backfilled by the v17
 * migration — it makes an ambiguous-failure retry (server committed, response
 * lost) replay instead of duplicating. See the domain type's doc comment.
 */
@Entity(tableName = "pending_interactions")
data class PendingInteraction(
    @PrimaryKey(autoGenerate = true) val id: Long = 0,
    /** Local device timestamp of the interaction (epoch millis). */
    val timestampMillis: Long,
    /** kind: "call" or "message". */
    val kind: String,
    /** Direction/outcome for calls: incoming|outgoing|missed. */
    val direction: String? = null,
    /** Phone number of the other party, as captured by the device. */
    val phoneNumber: String? = null,
    /** The contact this phone matched to (by id), if any. */
    val matchedContactId: Int? = null,
    /** True once a server Activity has been created from this row. */
    val synced: Boolean = false,
    val syncedAt: String? = null,
    /** CON-04/ADR-0010 retry key (v17); null only pre-backfill. */
    val idempotencyKey: String? = null,
)
