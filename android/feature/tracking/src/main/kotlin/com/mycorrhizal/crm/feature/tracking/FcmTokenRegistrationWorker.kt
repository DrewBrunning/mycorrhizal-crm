package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import androidx.hilt.work.HiltWorker
import androidx.work.CoroutineWorker
import androidx.work.Data
import androidx.work.WorkerParameters
import dagger.assisted.Assisted
import dagger.assisted.AssistedInject

/**
 * Issue #2066: registers a refreshed FCM token with the server. `onNewToken`
 * used to launch this on an unstructured scope that died with the service (and
 * the process), silently dropping the registration; as a persisted,
 * network-constrained, retried WorkManager job it survives process death. The
 * token travels in the input data because a rotated token is not otherwise
 * stored. [DeviceRegistrationManager.register] is idempotent, so a retry or a
 * superseding job (see [TrackingWorkerScheduler.enqueueFcmTokenRegistration])
 * is safe.
 */
@HiltWorker
class FcmTokenRegistrationWorker @AssistedInject constructor(
    @Assisted appContext: Context,
    @Assisted workerParams: WorkerParameters,
    private val deviceRegistration: DeviceRegistrationManager
) : CoroutineWorker(appContext, workerParams) {

    override suspend fun doWork(): Result {
        val token = inputData.getString(KEY_TOKEN) ?: return Result.failure()
        return if (deviceRegistration.register(tokenOverride = token).isSuccess) {
            Result.success()
        } else {
            Result.retry()
        }
    }

    companion object {
        const val KEY_TOKEN = "token"

        fun inputFor(token: String): Data = Data.Builder().putString(KEY_TOKEN, token).build()
    }
}
