package com.mycorrhizal.crm.feature.tracking

import android.content.Context
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject

/**
 * Issue #1200: whether this distribution build offers the opt-in call/SMS
 * activity-logging feature at all.
 *
 * The feature needs `READ_CALL_LOG`/`READ_PHONE_STATE` (call log) and
 * `READ_SMS`/`RECEIVE_SMS` (messages), which Google Play restricts to an
 * app that is the user's default dialer/SMS handler. The `play` flavor
 * therefore drops the feature: its manifest overlay removes the permissions
 * and the [PhoneStateReceiver]/[SmsReceiver]/[CallDetectionService]
 * components, and `call_sms_tracking_available` resolves to false there. The
 * `obtainium` (gold-standard) and `foss` (F-Droid) flavors keep it.
 *
 * Resolved from a resource rather than a `BuildConfig` field so the
 * DI-free call sites ([TrackingWorkerScheduler] from [BootReceiver]) can read
 * the same value as the injected one — one source of truth.
 */
fun interface CallSmsTrackingCapability {
    fun isAvailable(): Boolean
}

/**
 * Reads the merged `bool` resource: the library default is true
 * (`core/ui/src/main/res/values/bools.xml`); `app/src/play/res/values/bools.xml`
 * overrides it to false for the Play build.
 */
class ResourceCallSmsTrackingCapability @Inject constructor(
    @ApplicationContext private val context: Context,
) : CallSmsTrackingCapability {
    override fun isAvailable(): Boolean =
        context.resources.getBoolean(R.bool.call_sms_tracking_available)
}
