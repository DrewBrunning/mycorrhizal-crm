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
 * Reconciles the Inbox against SmsReceiver's live broadcast capture (ADR
 * 0019, issue #1127) — the recovery path for a missed SMS_RECEIVED broadcast
 * (Doze-deferred delivery, app not running at boot, READ_SMS granted after a
 * message already arrived), which today is a permanent loss with no other
 * recovery.
 *
 * Reads `Telephony.Sms.Inbox` forward from the highest `_id`
 * [TrackingSettingsRepository.lastSmsInboxId] recorded — by either
 * SmsReceiver (after each broadcast capture) or a previous run of this
 * worker — never re-reading a row at or below that cursor. A row the
 * broadcast already captured is naturally excluded because the cursor has
 * already advanced past its `_id`; nothing is ever re-fetched, so no row can
 * be captured twice. [InteractionCapture.capture]'s own `dedupe = true` is an
 * extra safety net if this worker's periodic chain overlaps a one-shot run,
 * same rationale as [SmsBackfillWorker].
 *
 * Issue #1123 hardening applies here too: [SmsInboxReader.readSince] pages
 * ASC + bounded, never a single DESC/newest-N read that would silently
 * truncate a large backlog.
 *
 * Gated on the SMS-tracking opt-in and the READ_SMS OS grant, same as
 * [SmsBackfillWorker].
 */
@HiltWorker
class SmsInboxReconciliationWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted workerParams: WorkerParameters,
    private val pendingInteractionRepository: PendingInteractionRepository,
    private val contactRepository: ContactRepository,
    private val trackingSettings: TrackingSettingsRepository,
) : CoroutineWorker(appContext, workerParams) {

    override suspend fun doWork(): Result {
        if (!trackingSettings.smsTrackingEnabled()) return Result.success()

        if (!hasSmsPermission()) {
            Log.w(TAG, "READ_SMS not granted; SMS inbox reconciliation is a no-op")
            return Result.success()
        }

        val reader = SmsInboxReader(applicationContext.contentResolver)
        var sinceId = trackingSettings.lastSmsInboxId() ?: 0L

        var pages = 0
        while (pages < MAX_PAGES_PER_RUN) {
            val entries = reader.readSince(sinceId, limit = PAGE_SIZE)
            if (entries.isEmpty()) break
            pages++

            var maxId = sinceId
            entries.forEach { entry ->
                if (entry.id > maxId) maxId = entry.id
                val number = entry.address ?: return@forEach
                // Issue #1029: shared capture policy — an incoming text from a
                // number that maps to no cached contact is dropped (and
                // counted), not staged. recordIfNew dedupes overlapping runs.
                InteractionCapture.capture(
                    contactRepository = contactRepository,
                    pendingInteractionRepository = pendingInteractionRepository,
                    trackingSettings = trackingSettings,
                    kind = InteractionCapture.KIND_MESSAGE,
                    direction = InteractionCapture.DIR_INCOMING,
                    number = number,
                    timestampMillis = entry.timestampMillis,
                    dedupe = true,
                )
            }

            sinceId = maxId
            trackingSettings.setLastSmsInboxId(sinceId)

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
        const val TAG = "SmsInboxReconciliationWorker"
        const val PAGE_SIZE = 50
        const val MAX_PAGES_PER_RUN = 20
    }
}
