package com.mycorrhizal.crm.feature.tracking

import android.app.Service
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import android.telephony.PhoneStateListener
import android.telephony.TelephonyManager
import androidx.core.content.ContextCompat

/**
 * Foreground service that shows the quick-capture overlay when a call ends
 * (§6.5/§6.7). Runs only while call tracking is enabled; the overlay appears
 * for a short window after a call, tapping it deep-links into the activity
 * create form. Self-stops after 5 minutes of no call activity. The call-log
 * staging itself is handled by PhoneStateReceiver + CallLogSyncWorker; this
 * service is purely the opt-in overlay UX.
 */
class CallDetectionService : Service() {

    private val handler = Handler(Looper.getMainLooper())

    private val phoneStateListener = object : PhoneStateListener() {
        override fun onCallStateChanged(state: Int, phoneNumber: String?) {
            if (state == TelephonyManager.CALL_STATE_IDLE) {
                val hasOverlayPermission = ContextCompat.checkSelfPermission(
                    this@CallDetectionService,
                    android.Manifest.permission.SYSTEM_ALERT_WINDOW,
                ) == PackageManager.PERMISSION_GRANTED
                if (hasOverlayPermission) {
                    QuickCaptureOverlay.show(this@CallDetectionService)
                }
                resetSelfStop()
            }
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        startForeground(
            NOTIFICATION_ID,
            NotificationBuilder.cadence(applicationContext, "Mycorrhizal", "Listening for calls"),
        )
        val telephonyManager = getSystemService(TelephonyManager::class.java)
        telephonyManager.listen(phoneStateListener, PhoneStateListener.LISTEN_CALL_STATE)
        resetSelfStop()
        return START_STICKY
    }

    private fun resetSelfStop() {
        handler.removeCallbacksAndMessages(null)
        handler.postDelayed({ stopSelf() }, SELF_STOP_MS)
    }

    override fun onDestroy() {
        handler.removeCallbacksAndMessages(null)
        QuickCaptureOverlay.dismiss()
        val telephonyManager = getSystemService(TelephonyManager::class.java)
        telephonyManager.listen(phoneStateListener, PhoneStateListener.LISTEN_NONE)
        super.onDestroy()
    }

    override fun onBind(intent: Intent?): IBinder? = null

    companion object {
        const val NOTIFICATION_ID = 1001
        private const val SELF_STOP_MS = 5 * 60 * 1000L
    }
}
