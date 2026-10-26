package com.mycorrhizal.crm.feature.tracking

import android.annotation.SuppressLint
import android.app.Service
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.telephony.TelephonyCallback
import android.telephony.TelephonyManager
import androidx.core.content.ContextCompat
import com.mycorrhizal.crm.domain.repository.ActivityRepository
import com.mycorrhizal.crm.domain.repository.ContactRepository
import com.mycorrhizal.crm.domain.repository.TrackingSettingsRepository
import com.mycorrhizal.crm.ui.R
import dagger.hilt.android.AndroidEntryPoint
import javax.inject.Inject
import kotlinx.coroutines.CoroutineDispatcher
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * Foreground service that shows the quick-capture overlay when a call ends
 * (§6.5/§6.7). Runs only while call tracking is enabled; the overlay appears
 * for a short window after a call, pre-filling the activity form with the
 * called contact so the interaction can be logged without leaving the call
 * screen. Self-stops after 5 minutes of no call activity. The call-log
 * staging itself is handled by PhoneStateReceiver + CallLogSyncWorker; this
 * service is purely the opt-in overlay UX.
 */
@AndroidEntryPoint
class CallDetectionService : Service() {

    private val handler = Handler(Looper.getMainLooper())

    @Inject
    lateinit var contactRepository: ContactRepository

    @Inject
    lateinit var activityRepository: ActivityRepository

    @Inject
    lateinit var trackingSettings: TrackingSettingsRepository

    /**
     * Dispatcher for the (suspend) contact lookup behind the overlay decision.
     * Extracted as an override point so CallDetectionServiceTest can drive the
     * lookup deterministically through a test scheduler (issue #1029).
     */
    internal var serviceDispatcher: CoroutineDispatcher = Dispatchers.Main.immediate

    private var serviceScope: CoroutineScope? = null

    // Owned by this service instance (not a static/singleton holder) so the
    // View it retains while shown is scoped to the service's lifetime;
    // dismiss()'d and dropped in onDestroy. internal (not private) so
    // CallDetectionServiceTest can assert the granted/denied permission
    // branches actually differ, not just that neither throws.
    internal var quickCaptureOverlay: QuickCaptureOverlay? = null

    // API 31+ uses TelephonyCallback; API 26-30 still needs the deprecated
    // PhoneStateListener (TelephonyCallback does not exist there). The
    // suppression is scoped to the legacy factory and its register/unregister
    // helpers, never the whole module. [sdkInt] is overridable so the unit
    // tests -- which run under one Robolectric SDK -- can exercise both paths.
    internal var sdkInt: Int = Build.VERSION.SDK_INT

    @Suppress("DEPRECATION")
    private var legacyPhoneStateListener: android.telephony.PhoneStateListener? = null
    private var telephonyCallback: TelephonyCallback? = null
    private var listening = false

    /**
     * API 31+'s [TelephonyCallback.CallStateListener] reports only the new
     * state, not the number (the platform removed it from the callback for
     * privacy). By the time IDLE arrives the call log has the just-ended call,
     * so read its newest entry to keep both the overlay prefill and the
     * known-contacts-only policy working. Without READ_CALL_LOG the read is a
     * logged no-op and returns null, treated the same as a withheld number.
     */
    internal var recentCallNumber: () -> String? = {
        runCatching {
            CallLogReader(applicationContext.contentResolver)
                .readSince(System.currentTimeMillis() - RECENT_CALL_WINDOW_MS, limit = 1)
                .firstOrNull()
                ?.number
        }.getOrNull()
    }

    internal fun handleTelephonyCallStateChanged(state: Int) {
        handleCallStateChanged(state, recentCallNumber())
    }

    // Extracted from phoneStateListener (issue #320 Phase C) so the call-idle
    // -> show-overlay decision is directly testable -- Robolectric has no
    // shadow for simulating a real TelephonyManager/PhoneStateListener
    // callback, so this was otherwise only reachable via reflection.
    //
    // Issue #1029: the overlay is suppressed for a caller whose number maps to
    // no cached contact (unless the user opted into unknown numbers) — the same
    // known-contacts-only policy the automatic capture path applies. The
    // lookup is suspend (Room is offline-only), so the decision runs on
    // [serviceDispatcher]; the permission check and self-stop stay synchronous
    // so the service's lifecycle behavior is unchanged.
    internal fun handleCallStateChanged(state: Int, phoneNumber: String?) {
        if (state == TelephonyManager.CALL_STATE_IDLE) {
            val hasOverlayPermission = ContextCompat.checkSelfPermission(
                this@CallDetectionService,
                android.Manifest.permission.SYSTEM_ALERT_WINDOW
            ) == PackageManager.PERMISSION_GRANTED
            if (hasOverlayPermission) {
                scope().launch {
                    if (!shouldShowOverlay(phoneNumber)) return@launch
                    (
                        quickCaptureOverlay ?: QuickCaptureOverlay(
                            contactRepository = contactRepository,
                            activityRepository = activityRepository
                        ).also { quickCaptureOverlay = it }
                        )
                        .show(this@CallDetectionService, phoneNumber)
                }
            }
            resetSelfStop()
        }
    }

    /**
     * Issue #1029: whether an ended call's number warrants the quick-capture
     * overlay. A known contact always does; an unknown/withheld/blank number
     * only does when the user enabled [TrackingSettingsRepository.includeUnknownNumbers].
     * A lookup failure counts as unknown (a failed local read cannot prove the
     * number is known).
     */
    internal suspend fun shouldShowOverlay(phoneNumber: String?): Boolean {
        val includeUnknown = trackingSettings.includeUnknownNumbers()
        if (phoneNumber.isNullOrBlank()) return includeUnknown
        val known = runCatching { contactRepository.findByPhone(phoneNumber) }.getOrNull() != null
        return known || includeUnknown
    }

    private fun scope(): CoroutineScope = serviceScope ?: CoroutineScope(
        SupervisorJob() + serviceDispatcher
    ).also { serviceScope = it }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        startForeground(
            NOTIFICATION_ID,
            NotificationBuilder.cadence(
                applicationContext,
                applicationContext.getString(R.string.app_name),
                applicationContext.getString(R.string.call_service_notification_text)
            )
        )
        startListening()
        resetSelfStop()
        return START_STICKY
    }

    /**
     * Issue #721: if READ_PHONE_STATE was revoked while the service was
     * running and the system then restarts it (START_STICKY), registration
     * throws SecurityException — a missing grant is a logged no-op (the
     * call-log capture keeps working via its own grant check), never a crash
     * loop.
     */
    // sdkInt defaults to Build.VERSION.SDK_INT, so the >= S branch is the
    // same runtime guard lint accepts for Build.VERSION.
    @SuppressLint("NewApi")
    private fun startListening() {
        if (listening) return
        val telephonyManager = getSystemService(TelephonyManager::class.java) ?: return
        try {
            if (sdkInt >= Build.VERSION_CODES.S) {
                val callback = telephonyCallback
                    ?: createTelephonyCallback().also { telephonyCallback = it }
                telephonyManager.registerTelephonyCallback(
                    ContextCompat.getMainExecutor(this),
                    callback
                )
            } else {
                val listener = legacyPhoneStateListener
                    ?: createLegacyPhoneStateListener().also { legacyPhoneStateListener = it }
                registerLegacyListener(telephonyManager, listener)
            }
            listening = true
        } catch (e: SecurityException) {
            android.util.Log.w(TAG, "READ_PHONE_STATE missing; phone-state listening disabled", e)
        }
    }

    @Suppress("DEPRECATION", "OVERRIDE_DEPRECATION")
    private fun createLegacyPhoneStateListener(): android.telephony.PhoneStateListener =
        object : android.telephony.PhoneStateListener() {
            override fun onCallStateChanged(state: Int, phoneNumber: String?) =
                handleCallStateChanged(state, phoneNumber)
        }

    @Suppress("DEPRECATION")
    private fun registerLegacyListener(
        telephonyManager: TelephonyManager,
        listener: android.telephony.PhoneStateListener
    ) {
        telephonyManager.listen(listener, android.telephony.PhoneStateListener.LISTEN_CALL_STATE)
    }

    @SuppressLint("NewApi") // only created inside the >= S branch above.
    private fun createTelephonyCallback(): TelephonyCallback =
        object : TelephonyCallback(), TelephonyCallback.CallStateListener {
            override fun onCallStateChanged(state: Int) = handleTelephonyCallStateChanged(state)
        }

    @SuppressLint("NewApi") // only unregisters what the >= S branch registered.
    private fun stopListening() {
        if (!listening) return
        val telephonyManager = getSystemService(TelephonyManager::class.java) ?: return
        try {
            if (sdkInt >= Build.VERSION_CODES.S) {
                telephonyCallback?.let { telephonyManager.unregisterTelephonyCallback(it) }
            } else {
                legacyPhoneStateListener?.let {
                    unregisterLegacyListener(telephonyManager, it)
                }
            }
        } catch (e: SecurityException) {
            android.util.Log.w(TAG, "READ_PHONE_STATE missing while unregistering listener", e)
        } finally {
            listening = false
        }
    }

    @Suppress("DEPRECATION")
    private fun unregisterLegacyListener(
        telephonyManager: TelephonyManager,
        listener: android.telephony.PhoneStateListener
    ) {
        telephonyManager.listen(listener, android.telephony.PhoneStateListener.LISTEN_NONE)
    }

    private fun resetSelfStop() {
        handler.removeCallbacksAndMessages(null)
        handler.postDelayed({ stopSelf() }, SELF_STOP_MS)
    }

    override fun onDestroy() {
        handler.removeCallbacksAndMessages(null)
        serviceScope?.cancel()
        serviceScope = null
        quickCaptureOverlay?.dismiss()
        quickCaptureOverlay = null
        stopListening()
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    companion object {
        const val NOTIFICATION_ID = 1001
        private const val SELF_STOP_MS = 5 * 60 * 1000L
        private const val RECENT_CALL_WINDOW_MS = 5 * 60 * 1000L
        private const val TAG = "CallDetectionService"
    }
}
