package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import android.content.pm.PackageManager
import android.util.Log
import androidx.core.content.ContextCompat
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * Reads the *sent* SMS folder since the last watermark and stages outgoing
 * texts as PendingInteractions (§6.2, issue #721). Incoming texts are captured
 * instantly by SmsReceiver; the backfill exists because nothing observes
 * outgoing texts except a periodic read of the Sent folder. It runs on a
 * periodic cadence (see [TrackingWorkerScheduler]) and once immediately after
 * the SMS-tracking permission is granted, so the first run also stages recent
 * pre-grant outgoing history rather than waiting for the next poll.
 *
 * Issue #721 hardening: gated on both the opt-in flag AND the READ_SMS OS
 * grant (revoked mid-enqueue → logged no-op, never a crash), and every row is
 * written through [PendingInteractionRepository.recordIfNew] so an overlapping
 * periodic + one-shot run cannot stage the same text twice.
 */
@HiltWorker
class SmsBackfillWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted workerParams: WorkerParameters,
    private val pendingInteractionRepository: PendingInteractionRepository,
    private val contactRepository: ContactRepository,
    private val trackingSettings: TrackingSettingsRepository,
) : CoroutineWorker(appContext, workerParams) {

    override suspend fun doWork(): Result {
        if (!trackingSettings.smsTrackingEnabled()) return Result.success()

        if (!hasSmsPermission()) {
            Log.w(TAG, "READ_SMS not granted; SMS backfill is a no-op")
            return Result.success()
        }

        val reader = SmsHistoryReader(applicationContext.contentResolver)
        var since = trackingSettings.lastSmsTimestamp()

        // Issue #1123: page forward through the backlog rather than taking a
        // single newest-50 batch — readSentSince is now ASC, so each page's
        // last row is its newest, and the watermark only ever advances to a
        // row we actually processed. Bounded to MAX_PAGES_PER_RUN so a very
        // large backlog (or a corrupted watermark) can't turn one run into an
        // unbounded loop; a run that hits the cap picks up where it left off
        // on the next periodic invocation.
        var pages = 0
        while (pages < MAX_PAGES_PER_RUN) {
            val entries = reader.readSentSince(since, limit = PAGE_SIZE)
            if (entries.isEmpty()) break
            pages++

            var maxTs = since
            entries.forEach { entry ->
                if (entry.timestampMillis > maxTs) maxTs = entry.timestampMillis
                val number = entry.address ?: return@forEach
                // Issue #1029: shared capture policy — an outgoing text to a number
                // that maps to no cached contact is dropped (and counted), not
                // staged. recordIfNew dedupes overlapping periodic + one-shot runs.
                InteractionCapture.capture(
                    contactRepository = contactRepository,
                    pendingInteractionRepository = pendingInteractionRepository,
                    trackingSettings = trackingSettings,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_OUTGOING,
                    number = number,
                    timestampMillis = entry.timestampMillis,
                    dedupe = true,
                )
            }

            since = maxTs
            trackingSettings.setLastSmsTimestamp(since)

            if (entries.size < PAGE_SIZE) break // caught up
        }

        return Result.success()
    }

    private fun hasSmsPermission(): Boolean =
        ContextCompat.checkSelfPermission(
            applicationContext,
            TrackingPermissions.READ_SMS,
        ) == PackageManager.PERMISSION_GRANTED

    private companion object {
        const val TAG = "SmsBackfillWorker"
        const val PAGE_SIZE = 50
        const val MAX_PAGES_PER_RUN = 20
    }
}
