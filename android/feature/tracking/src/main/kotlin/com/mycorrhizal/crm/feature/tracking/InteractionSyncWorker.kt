package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.model.network.ActivityInput
import com.mycorrhizal.crm.network.ApiClient
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject
import java.time.Instant
import java.time.ZoneOffset
import java.time.format.DateTimeFormatter

/**
 * Periodic sync of PendingInteractions to the server as Activity rows (§6.1
 * "InteractionSyncWorker"). Each pending call becomes `type=call`; each
 * pending message becomes `type=message` with NO body (the §6.2 privacy
 * boundary — the server only ever sees the contact link + title). Rows that
 * matched no contact are skipped with the contact_ids empty, so they land as
 * unassociated activities rather than being lost.
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

        var syncedCount = 0
        for (interaction in pending) {
            val type = when (interaction.kind) {
                InteractionCapture.KIND_CALL -> "call"
                InteractionCapture.KIND_MESSAGE -> "message"
                else -> continue
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
            val input = ActivityInput(
                title = title,
                date = formatDate(interaction.timestampMillis),
                contactIds = interaction.matchedContactId?.let { listOf(it) },
                type = type,
                externalRef = "device:${interaction.id}",
            )
            val result = apiClient.createActivity(input)
            if (result.isSuccess) {
                pendingInteractionRepository.markSynced(
                    interaction.id,
                    Instant.now().toString(),
                )
                syncedCount++
            }
        }

        if (syncedCount == pending.size) {
            pendingInteractionRepository.deleteSynced()
        }
        trackingSettings.setLastCallLogTimestamp(trackingSettings.lastCallLogTimestamp())
        return Result.success()
    }

    internal fun formatDateForTest(epochMillis: Long): String = formatDate(epochMillis)

    private fun formatDate(epochMillis: Long): String =
        DateTimeFormatter.ISO_OFFSET_DATE_TIME.format(
            Instant.ofEpochMilli(epochMillis).atOffset(ZoneOffset.UTC),
        )
}
