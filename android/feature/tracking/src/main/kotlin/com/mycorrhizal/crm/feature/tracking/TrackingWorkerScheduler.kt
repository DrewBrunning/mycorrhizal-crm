package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import com.mycorrhizal.crm.ui.R
import java.util.concurrent.TimeUnit

/**
 * Enqueues the periodic Phase-4 workers (§6.4). Idempotent — safe to call
 * from BootReceiver, the app entry point, and the Settings toggles.
 *
 * Issue #721 adds the capture catch-ups to the periodic set: call-log sync
 * (the call itself is normally staged by PhoneStateReceiver's one-shot, but a
 * periodic catch-up is the recovery path for broadcasts that never fired) and
 * SMS backfill (the only way outgoing texts are ever observed). ADR 0019 /
 * issue #1127 adds a third: SMS Inbox reconciliation, the recovery path for a
 * missed *incoming*-SMS broadcast (SmsReceiver already handles the live
 * case). All three are cheap no-ops when the corresponding opt-in or OS grant
 * is missing. The one-shot catch-ups ([enqueueCallLogCatchUp],
 * [enqueueSmsBackfill]) are the immediate runs the Settings toggle issues on
 * a fresh permission grant, under names distinct from the periodic chains so
 * the two never cancel each other.
 *
 * Issue #1200: in a distribution build without the capture feature (the play
 * flavor) none of the three capture chains are enqueued at all; see
 * [schedulePeriodic]'s `callSmsCaptureAvailable`.
 */
object TrackingWorkerScheduler {

    const val UNIQUE_INTERACTION_SYNC = "interaction-sync"
    const val UNIQUE_REMINDER_CHECK = "reminder-check"
    const val UNIQUE_CADENCE_CHECK = "cadence-check"
    const val UNIQUE_BIRTHDAY_CHECK = "birthday-check"

    /** Periodic call-log catch-up (recovery for missed phone-state broadcasts). */
    const val UNIQUE_CALL_LOG_CATCH_UP = "call-log-catch-up"

    /** Periodic outgoing-SMS backfill. */
    const val UNIQUE_SMS_BACKFILL = "sms-backfill"

    /** Periodic incoming-SMS Inbox reconciliation (ADR 0019, issue #1127). */
    const val UNIQUE_SMS_INBOX_RECONCILIATION = "sms-inbox-reconciliation"

    /** Periodic contact phone-index backfill (issue #1122). */
    const val UNIQUE_CONTACT_PHONE_INDEX_BACKFILL = "contact-phone-index-backfill"

    /** One-shot immediate run after a call-tracking grant / phone-state event
     *  (shared with PhoneStateReceiver). */
    const val UNIQUE_CALL_LOG_SYNC = "call-log-sync"

    /** One-shot immediate run after an SMS-tracking grant. */
    const val UNIQUE_SMS_BACKFILL_ONCE = "sms-backfill-once"

    /** How often the capture catch-ups poll. Call-log 30 min (the phone-state
     *  broadcast already stages calls live; this is a recovery net), SMS 15 min
     *  (the minimum — a sent text has no live path, so its latency is bounded
     *  by this cadence). */
    private const val CALL_LOG_CATCH_UP_MINUTES = 30L
    private const val SMS_BACKFILL_MINUTES = 15L

    /** ADR 0019 / issue #1127: like the call-log catch-up, this is a recovery
     *  net behind an already-live path (SmsReceiver's broadcast), not the sole
     *  observation path the way SMS backfill is — so it shares the call-log
     *  catch-up's 30-min cadence rather than the 15-min floor. */
    private const val SMS_INBOX_RECONCILIATION_MINUTES = 30L

    /** Issue #1122: low-priority, bounded-batch — 30 min is plenty. */
    private const val CONTACT_PHONE_INDEX_BACKFILL_MINUTES = 30L

    fun schedulePeriodic(
        context: Context,
        // Issue #1200: the play flavor omits the call/SMS capture feature
        // (Google Play restricts its permissions), so its two catch-up chains
        // are never enqueued there. The default reads the same merged bool
        // resource the injected CallSmsTrackingCapability reads.
        callSmsCaptureAvailable: Boolean = context.resources.getBoolean(R.bool.call_sms_tracking_available),
    ) {
        val workManager = WorkManager.getInstance(context)

        // Sync pending interactions every 15 min (WorkManager minimum interval).
        val interactionSync = PeriodicWorkRequestBuilder<InteractionSyncWorker>(
            15,
            TimeUnit.MINUTES,
        ).build()
        workManager.enqueueUniquePeriodicWork(
            UNIQUE_INTERACTION_SYNC,
            ExistingPeriodicWorkPolicy.UPDATE,
            interactionSync,
        )

        // Notification checks (§6.4). Reminders hourly, cadences 4-hourly,
        // birthdays daily.
        val reminderCheck = PeriodicWorkRequestBuilder<ReminderNotificationWorker>(
            1,
            TimeUnit.HOURS,
        ).build()
        workManager.enqueueUniquePeriodicWork(
            UNIQUE_REMINDER_CHECK,
            ExistingPeriodicWorkPolicy.UPDATE,
            reminderCheck,
        )

        val cadenceCheck = PeriodicWorkRequestBuilder<CadenceCheckWorker>(
            4,
            TimeUnit.HOURS,
        ).build()
        workManager.enqueueUniquePeriodicWork(
            UNIQUE_CADENCE_CHECK,
            ExistingPeriodicWorkPolicy.UPDATE,
            cadenceCheck,
        )

        val birthdayCheck = PeriodicWorkRequestBuilder<BirthdayCheckWorker>(
            24,
            TimeUnit.HOURS,
        ).build()
        workManager.enqueueUniquePeriodicWork(
            UNIQUE_BIRTHDAY_CHECK,
            ExistingPeriodicWorkPolicy.UPDATE,
            birthdayCheck,
        )

        // Issue #721: capture catch-ups (see the class doc). Issue #1200: only
        // where the distribution build offers the capture feature at all.
        if (callSmsCaptureAvailable) {
            val callLogCatchUp = PeriodicWorkRequestBuilder<CallLogSyncWorker>(
                CALL_LOG_CATCH_UP_MINUTES,
                TimeUnit.MINUTES,
            ).build()
            workManager.enqueueUniquePeriodicWork(
                UNIQUE_CALL_LOG_CATCH_UP,
                ExistingPeriodicWorkPolicy.UPDATE,
                callLogCatchUp,
            )

            val smsBackfill = PeriodicWorkRequestBuilder<SmsBackfillWorker>(
                SMS_BACKFILL_MINUTES,
                TimeUnit.MINUTES,
            ).build()
            workManager.enqueueUniquePeriodicWork(
                UNIQUE_SMS_BACKFILL,
                ExistingPeriodicWorkPolicy.UPDATE,
                smsBackfill,
            )

            val smsInboxReconciliation = PeriodicWorkRequestBuilder<SmsInboxReconciliationWorker>(
                SMS_INBOX_RECONCILIATION_MINUTES,
                TimeUnit.MINUTES,
            ).build()
            workManager.enqueueUniquePeriodicWork(
                UNIQUE_SMS_INBOX_RECONCILIATION,
                ExistingPeriodicWorkPolicy.UPDATE,
                smsInboxReconciliation,
            )
        }

        // Issue #1122: hydrate cached contacts' full multi-phone index in the
        // background so a non-primary number can match without the user
        // having opened that contact's detail screen.
        val contactPhoneIndexBackfill = PeriodicWorkRequestBuilder<ContactPhoneIndexBackfillWorker>(
            CONTACT_PHONE_INDEX_BACKFILL_MINUTES,
            TimeUnit.MINUTES,
        ).build()
        workManager.enqueueUniquePeriodicWork(
            UNIQUE_CONTACT_PHONE_INDEX_BACKFILL,
            ExistingPeriodicWorkPolicy.UPDATE,
            contactPhoneIndexBackfill,
        )
    }

    /**
     * Kicks a one-time CallLogSyncWorker now (fresh grant catch-up, or reuse by
     * PhoneStateReceiver). REPLACE keeps at most one such run pending; both
     * issuers are idempotent watermark-based reads.
     */
    fun enqueueCallLogCatchUp(context: Context) {
        val request = OneTimeWorkRequestBuilder<CallLogSyncWorker>().build()
        WorkManager.getInstance(context).enqueueUniqueWork(
            UNIQUE_CALL_LOG_SYNC,
            ExistingWorkPolicy.REPLACE,
            request,
        )
    }

    /**
     * Kicks a one-time SmsBackfillWorker now (fresh grant catch-up). KEEP so a
     * rapid grant/revoke/grant cycle doesn't stack redundant runs.
     */
    fun enqueueSmsBackfill(context: Context) {
        val request = OneTimeWorkRequestBuilder<SmsBackfillWorker>().build()
        WorkManager.getInstance(context).enqueueUniqueWork(
            UNIQUE_SMS_BACKFILL_ONCE,
            ExistingWorkPolicy.KEEP,
            request,
        )
    }
}
