package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.WorkerParameters
import com.mycorrhizal.crm.data.repository.PendingInteractionFlusher
import com.mycorrhizal.crm.domain.repository.PendingInteractionRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.network.ApiClient
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * Periodic sync of PendingInteractions to the server as Activity rows (§6.1
 * "InteractionSyncWorker"). Each pending call becomes `type=call`; each
 * pending message becomes `type=message` with NO body (the §6.2 privacy
 * boundary — the server only ever sees the contact link + title).
 *
 * Issue #1029: the capture path (SmsReceiver / SmsBackfillWorker /
 * CallLogSyncWorker, all through `InteractionCapture.capture`) stages only
 * interactions whose number resolves to a cached contact by default, so an
 * unknown-number row normally never reaches this worker. A row *can* still have
 * no contact link — the user opted into unknown numbers via
 * `includeUnknownNumbers`, or a legacy queued row predates the filter — and it
 * is synced with `contact_ids` empty rather than dropped: the interaction is
 * the value, the contact link is best-effort.
 *
 * ANDROID-02 (issue #479) made this worker retry-safe and self-healing. The
 * mapping itself now lives in [PendingInteractionFlusher] so the profile-switch
 * drain (ADR 0028 Decision 1) reuses it; this worker is just the periodic gate
 * on the tracking opt-ins.
 */
@HiltWorker
class InteractionSyncWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted workerParams: WorkerParameters,
    private val pendingInteractionRepository: PendingInteractionRepository,
    private val trackingSettings: TrackingSettingsRepository,
    apiClient: ApiClient,
) : CoroutineWorker(appContext, workerParams) {

    private val flusher = PendingInteractionFlusher(pendingInteractionRepository, apiClient)

    override suspend fun doWork(): Result {
        if (!trackingSettings.callTrackingEnabled() && !trackingSettings.smsTrackingEnabled()) {
            return Result.success()
        }
        flusher.flush()
        return Result.success()
    }

    internal fun formatDateForTest(epochMillis: Long): String = PendingInteractionFlusher.formatDate(epochMillis)
}
