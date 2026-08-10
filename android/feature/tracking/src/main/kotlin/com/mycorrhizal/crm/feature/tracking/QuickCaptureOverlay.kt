package com.mycorrhizal.crm.feature.tracking

import android.annotation.SuppressLint
import android.content.Context
import android.content.Intent
import android.graphics.PixelFormat
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.View
import android.view.WindowManager
import android.widget.TextView

/**
 * A lightweight `SYSTEM_ALERT_WINDOW` overlay shown briefly after a call ends
 * (§6.5). It renders as a small floating "log" pill; tapping it deep-links
 * into the app so the user can record the interaction. It dismisses itself
 * after [DISPLAY_MS] or on tap. The overlay requires the SYSTEM_ALERT_WINDOW
 * special permission; without it, nothing is shown (graceful degradation).
 *
 * A full in-overlay Compose sheet (pre-filled activity form) is the natural
 * extension; the current version keeps the overlay minimal and testable.
 */
object QuickCaptureOverlay {

    private var view: View? = null
    private val handler = Handler(Looper.getMainLooper())

    fun show(context: Context) {
        dismiss()
        val windowManager = context.getSystemService(Context.WINDOW_SERVICE) as WindowManager
        val pill = TextView(context).apply {
            text = "Mycorrhizal — log interaction"
            setTextColor(0xFFFFFFFF.toInt())
            setPadding(48, 32, 48, 32)
            background = android.graphics.drawable.GradientDrawable().apply {
                setColor(0xFF3E543E.toInt())
                cornerRadius = 24f
            }
            isClickable = true
            setOnClickListener {
                launchApp(context)
                dismiss()
            }
        }
        val params = WindowManager.LayoutParams(
            WindowManager.LayoutParams.WRAP_CONTENT,
            WindowManager.LayoutParams.WRAP_CONTENT,
            WindowManager.LayoutParams.TYPE_APPLICATION_OVERLAY,
            WindowManager.LayoutParams.FLAG_NOT_FOCUSABLE or
                WindowManager.LayoutParams.FLAG_NOT_TOUCH_MODAL,
            PixelFormat.TRANSLUCENT,
        ).apply {
            gravity = Gravity.BOTTOM or Gravity.CENTER_HORIZONTAL
            y = 160
        }
        runCatching { windowManager.addView(pill, params) }
            .onSuccess { view = pill }
        handler.removeCallbacksAndMessages(null)
        handler.postDelayed({ dismiss() }, DISPLAY_MS)
    }

    fun dismiss() {
        handler.removeCallbacksAndMessages(null)
        view?.let { v ->
            runCatching {
                (v.context.getSystemService(Context.WINDOW_SERVICE) as WindowManager).removeView(v)
            }
            view = null
        }
    }

    private fun launchApp(context: Context) {
        val launch = context.packageManager.getLaunchIntentForPackage(context.packageName)
        if (launch != null) {
            launch.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_ACTIVITY_RESET_TASK_IF_NEEDED)
            context.startActivity(launch)
        }
    }

    private const val DISPLAY_MS = 30_000L
}
