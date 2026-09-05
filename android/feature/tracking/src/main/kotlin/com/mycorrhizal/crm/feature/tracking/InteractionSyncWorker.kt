package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.model.network.Activity
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.network.ApiClient
import com.mycorrhizal.crm.network.ApiError
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.time.Instant
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter
import java.util.UUID

/**
 * Periodic sync of PendingInteractions to the server as Activity rows (§6.1
 * "InteractionSyncWorker"). Each pending call becomes `type=call`; each
 * pending message becomes `type=message` with NO body (the §6.2 privacy
 * boundary — the server only ever sees the contact link + title). Rows that
 * matched no contact are skipped with the contact_ids empty, so they land as
 * unassociated activities rather than being lost.
 *
 * ANDROID-02 (issue #479) made this worker retry-safe and self-healing:
 *
 *  - Every attempt sends the row's [PendingInteraction.idempotencyKey] as the
 *    CON-04/ADR-0010 `Idempotency-Key` header. The outbox is a retry queue by
 *    another name: an attempt whose response is lost after the server commits
 *    (the ambiguous failure) is retried on the next run, and without a key
 *    that retry would create a duplicate server Activity. With the key the
 *    server replays its stored outcome — exactly one Activity per row no
 *    matter how many times it is retried.
 *
 *  - A row whose matched contact was deleted server-side while offline fails
 *    with 404 "One or more contacts not found" (soft-deleted contacts are
 *    excluded from the create's lookup). The interaction itself is the value;
 *    the contact link is best-effort (unmatched numbers already sync
 *    unassociated), so per ADR-0009 the worker drops the stale link and
 *    retries the row unassociated within the same run instead of leaving it
 *    stuck in the queue forever.
 */
@HiltWorker
class InteractionSyncWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted workerParams: WorkerParameters,
    private val pendingInteractionRepository: PendingInteractionRepository,
    private val trackingSettings: TrackingSettingsRepository,
    private val apiClient: ApiClient,
) : CoroutineWorker(appContext, workerParams) {

    override suspend fun doWork(): Result {
        if (!trackingSettings.callTrackingEnabled() && !trackingSettings.smsTrackingEnabled()) {
            return Result.success()
        }

        val pending = pendingInteractionRepository.unsynced()
        if (pending.isEmpty()) return Result.success()

        for (interaction in pending) {
            // Only the two tracked kinds ever sync; anything else (a legacy or
            // corrupt row) is ignored, matching the pre-ANDROID-02 `else -> continue`.
            if (!isSyncableKind(interaction.kind)) continue
            syncInteraction(interaction)
        }

        // deleteSynced() only ever removes rows already marked synced=1 (a
        // per-row flag set individually above), so it is safe to run every
        // time regardless of whether this batch fully succeeded. Gating it on
        // the whole batch succeeding left already-synced rows undeleted (and
        // therefore unbounded growth in the local table) after any partial
        // failure, since a later run's condition would only pass again once a
        // whole batch cleanly succeeded.
        pendingInteractionRepository.deleteSynced()
        return Result.success()
    }

    /**
     * Syncs a single pending row, resolving an ANDROID-02 stale-contact
     * conflict (see the class doc) before giving up. Returns true when the
     * row was durably synced (marked synced=1).
     */
    private suspend fun syncInteraction(interaction: PendingInteraction): Boolean {
        var current = interaction
        var result = createFor(current)

        if (result.isFailure && shouldDropStaleContactLink(current, result.exceptionOrNull())) {
            pendingInteractionRepository.clearMatchedContact(current.id)
            current = current.copy(matchedContactId = null)
            result = createFor(current)
        }

        return if (result.isSuccess) {
            pendingInteractionRepository.markSynced(
                interaction.id,
                Instant.now().toString(),
            )
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

    /** A 404 on a create with a contact link means the referenced contact is
     *  gone (or no longer the user's) server-side — never a valid link anymore. */
    private fun shouldDropStaleContactLink(
        interaction: PendingInteraction,
        error: Throwable?,
    ): Boolean {
        if (interaction.matchedContactId == null) return false
        return error is ApiError.Client && error.code == 404
    }

    internal fun formatDateForTest(epochMillis: Long): String = formatDate(epochMillis)

    private fun isSyncableKind(kind: String): Boolean =
        kind == InteractionCapture.KIND_CALL || kind == InteractionCapture.KIND_MESSAGE

    private fun activityInput(interaction: PendingInteraction): ActivityInput {
        val type = when (interaction.kind) {
            InteractionCapture.KIND_CALL -> "call"
            InteractionCapture.KIND_MESSAGE -> "message"
            else -> error("unreachable: activityInput is only called for syncable kinds")
        }
        val title = when (interaction.kind) {
            InteractionCapture.KIND_MESSAGE -> "Message"
            InteractionCapture.KIND_CALL ->
                when (interaction.direction) {
                    InteractionCapture.DIR_MISSED -> "Missed call"
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

    private fun formatDate(epochMillis: Long): String =
        DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(
            Instant.ofEpochMilli(epochMillis).atOffset(ZoneOffset.UTC),
        )
}
