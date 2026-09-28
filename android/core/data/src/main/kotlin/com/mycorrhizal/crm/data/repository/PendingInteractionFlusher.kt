package com.mycorrhizal.crm.data.repository

import com.mycorrhizal.crm.data.session.OutboxDrainer
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import java.time.Instant
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.UUID
import javax.inject.Inject

/**
 * The outbox→server mapping, extracted from `InteractionSyncWorker` so the
 * profile-switch drain (ADR 0028 Decision 1: "drain pending_interactions, or
 * show a confirmation naming the count to discard") can reuse the exact same
 * behavior the periodic worker uses, rather than a second copy.
 *
 * ANDROID-02 (issue #479) semantics are preserved verbatim: every attempt
 * sends the row's idempotency key, a 404 on a matched contact drops the stale
 * link and retries unassociated (ADR-0009), and a plain failure leaves the row
 * for the next run.
 */
class PendingInteractionFlusher @Inject constructor(
    private val pendingInteractionRepository: PendingInteractionRepository,
    private val apiClient: ApiClient,
) : OutboxDrainer {

    /** [OutboxDrainer]: true when the outbox is empty after the attempt. */
    override suspend fun drain(): Boolean = flush() == 0

    /**
     * Syncs every currently-unsynced row and returns the number still unsynced
     * afterwards (0 = fully drained).
     */
    suspend fun flush(): Int {
        val pending = pendingInteractionRepository.unsynced()
        if (pending.isEmpty()) return 0

        for (interaction in pending) {
            // Only the two tracked kinds ever sync; anything else (a legacy or
            // corrupt row) is ignored, matching the worker's pre-ANDROID-02
            // `else -> continue`.
            if (!isSyncableKind(interaction.kind)) continue
            syncInteraction(interaction)
        }

        // deleteSynced() only removes rows already marked synced=1, so it is
        // safe to run every time regardless of whether the batch fully
        // succeeded — gating it on the whole batch left already-synced rows
        // undeleted (unbounded local growth) after any partial failure.
        pendingInteractionRepository.deleteSynced()
        return pendingInteractionRepository.unsynced().size
    }

    /** Syncs a single pending row, resolving a stale-contact link first. */
    private suspend fun syncInteraction(interaction: PendingInteraction): Boolean {
        var current = interaction
        var result = createFor(current)

        if (result.isFailure && shouldDropStaleContactLink(current, result.exceptionOrNull())) {
            pendingInteractionRepository.clearMatchedContact(current.id)
            current = current.copy(matchedContactId = null)
            result = createFor(current)
        }

        return if (result.isSuccess) {
            pendingInteractionRepository.markSynced(interaction.id, Instant.now().toString())
            true
        } else {
            false
        }
    }

    private suspend fun createFor(interaction: PendingInteraction): kotlin.Result<Activity> {
        val key = interaction.idempotencyKey ?: generateAndPersistKey(interaction.id)
        return apiClient.createActivity(activityInput(interaction), key)
    }

    /** Backfills a key on the rare row that predates the v17 column/backfill. */
    private suspend fun generateAndPersistKey(id: Long): String {
        val key = UUID.randomUUID().toString()
        pendingInteractionRepository.setIdempotencyKey(id, key)
        return key
    }

    /** A 404 on a create with a contact link means the referenced contact is gone. */
    private fun shouldDropStaleContactLink(interaction: PendingInteraction, error: Throwable?): Boolean {
        if (interaction.matchedContactId == null) return false
        return error is ApiError.Client && error.code == 404
    }

    private fun isSyncableKind(kind: String): Boolean =
        kind == KIND_CALL || kind == KIND_MESSAGE

    private fun activityInput(interaction: PendingInteraction): ActivityInput {
        val type = when (interaction.kind) {
            KIND_CALL -> "call"
            KIND_MESSAGE -> "message"
            else -> error("unreachable: activityInput is only called for syncable kinds")
        }
        val title = when (interaction.kind) {
            KIND_MESSAGE -> "Message"
            KIND_CALL ->
                when (interaction.direction) {
                    DIR_MISSED -> "Missed call"
                    else -> "Call"
                }
            else -> "Interaction"
        }
        return ActivityInput(
            title = title,
            date = formatDate(interaction.timestampMillis),
            contactIds = interaction.matchedContactId?.let { listOf(it) },
            type = type,
            externalRef = "device:${interaction.id}",
        )
    }

    companion object {
        const val KIND_CALL = "call"
        const val KIND_MESSAGE = "message"
        const val DIR_MISSED = "missed"

        fun formatDate(epochMillis: Long): String =
            DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(
                Instant.ofEpochMilli(epochMillis).atOffset(ZoneOffset.UTC),
            )
    }
}
