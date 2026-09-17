package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import android.util.Log
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * Issue #1122: a cached contact's non-primary phone numbers can't match an
 * incoming SMS/call until its detail screen has been opened at least once —
 * `listContacts`/`syncContacts` only ever carry the single `primaryPhone` on
 * the wire (see [com.mycorrhizal.crm.data.local.CachedContact.phonesNormalized]'s
 * doc comment), so the full multi-number index is populated only by a full
 * detail fetch ([ContactRepository.getContact]).
 *
 * This periodically hydrates a small batch of contacts that have never had
 * one, so the full index eventually becomes available without the user
 * having to open every contact manually. A no-op when neither capture path
 * is enabled, since nothing consumes the index otherwise. Bounded to
 * [BATCH_SIZE] per run so a large contact list can't turn one run into a
 * network storm; a fetch failure simply leaves that contact eligible again
 * on the next run (nothing to retry explicitly — the DAO query re-selects
 * any row still missing its full detail).
 */
@HiltWorker
class ContactPhoneIndexBackfillWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted workerParams: WorkerParameters,
    private val contactRepository: ContactRepository,
    private val trackingSettings: TrackingSettingsRepository,
) : CoroutineWorker(appContext, workerParams) {

    override suspend fun doWork(): Result {
        if (!trackingSettings.smsTrackingEnabled() && !trackingSettings.callTrackingEnabled()) {
            return Result.success()
        }

        val ids = contactRepository.getContactIdsMissingPhoneIndex(BATCH_SIZE)
        ids.forEach { id ->
            val outcome = runCatching { contactRepository.getContact(id) }
            if (outcome.isFailure || outcome.getOrNull()?.isFailure == true) {
                Log.w(TAG, "Phone-index backfill: failed to hydrate contact $id; will retry next run")
            }
        }
        return Result.success()
    }

    private companion object {
        const val TAG = "ContactPhoneIndexBackfillWorker"
        const val BATCH_SIZE = 20
    }
}
