package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.PendingInteraction
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * Reads the call log since the last watermark and stages new calls as
 * PendingInteractions (§6.1). Triggered by PhoneStateReceiver / a one-time
 * request; also run periodically as a catch-up.
 */
@HiltWorker
class CallLogSyncWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted workerParams: WorkerParameters,
    private val pendingInteractionRepository: PendingInteractionRepository,
    private val contactRepository: ContactRepository,
    private val trackingSettings: TrackingSettingsRepository,
) : CoroutineWorker(appContext, workerParams) {

    override suspend fun doWork(): Result {
        // Respect the opt-in toggle (§8.3): without READ_CALL_LOG granted this
        // is a no-op, not an error.
        if (!trackingSettings.callTrackingEnabled()) return Result.success()

        val since = trackingSettings.lastCallLogTimestamp()
        val entries = CallLogReader(applicationContext.contentResolver).readSince(since)
        if (entries.isEmpty()) return Result.success()

        var maxTs = since
        entries.forEach { entry ->
            if (entry.timestampMillis > maxTs) maxTs = entry.timestampMillis
            val number = entry.number ?: return@forEach
            val contact = runCatching { contactRepository.findByPhone(number) }.getOrNull()
            val direction = when (entry.type) {
                CallLogKinds.INCOMING -> InteractionCapture.DIR_INCOMING
                CallLogKinds.OUTGOING -> InteractionCapture.DIR_OUTGOING
                CallLogKinds.MISSED -> InteractionCapture.DIR_MISSED
                else -> null
            }
            pendingInteractionRepository.record(
                PendingInteraction(
                    timestampMillis = entry.timestampMillis,
                    kind = InteractionCapture.KIND_CALL,
                    direction = direction,
                    phoneNumber = number,
                    matchedContactId = contact?.id,
                ),
            )
        }

        // Advance the watermark past everything we saw so the next run is
        // incremental (new calls only).
        trackingSettings.setLastCallLogTimestamp(maxTs)
        return Result.success()
    }
}
