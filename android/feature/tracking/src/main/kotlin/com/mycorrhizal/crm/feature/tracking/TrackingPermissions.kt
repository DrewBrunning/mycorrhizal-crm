package com.mycorrhizal.crm.feature.tracking

import android.Manifest

/**
 * Issue #721: the dangerous OS permissions each opt-in tracking toggle needs.
 *
 * These are the single source of truth for the permission sets — the Settings
 * screen requests exactly one of these arrays when a toggle is turned on, and
 * the capture workers gate on the same grants before touching a provider.
 *
 * Issue #1200: the `play` flavor does not offer the capture feature at all, so
 * it removes these permissions from its merged manifest and hides the toggles
 * (`CallSmsTrackingCapability`). That is a build-level exclusion, deliberately
 * NOT a change to these arrays — `obtainium` and `foss` keep requesting
 * exactly these sets.
 */
object TrackingPermissions {

    const val READ_CALL_LOG = Manifest.permission.READ_CALL_LOG
    const val READ_PHONE_STATE = Manifest.permission.READ_PHONE_STATE
    const val READ_SMS = Manifest.permission.READ_SMS
    const val RECEIVE_SMS = Manifest.permission.RECEIVE_SMS

    /**
     * "Show notifications" (issue #1919). Runtime permission on API 33+; below
     * that the app can always post, so [PermissionChecker] reports it granted
     * instead of asking for a permission the platform does not know.
     */
    const val POST_NOTIFICATIONS = Manifest.permission.POST_NOTIFICATIONS

    /**
     * "Log calls as activities": READ_CALL_LOG reads the call-log provider,
     * READ_PHONE_STATE is what the system requires before PHONE_STATE broadcasts
     * are delivered and CallDetectionService may register a PhoneStateListener.
     */
    val CALL_TRACKING: Array<String> = arrayOf(READ_CALL_LOG, READ_PHONE_STATE)

    /**
     * "Log messages as activities": READ_SMS reads the SMS provider for the
     * outgoing-text backfill (the broadcast path cannot observe texts the user
     * sends), RECEIVE_SMS is what the system requires before SMS_RECEIVED
     * broadcasts are delivered to SmsReceiver.
     */
    val SMS_TRACKING: Array<String> = arrayOf(READ_SMS, RECEIVE_SMS)

    /** "Show notifications" (API 33+ only; see [POST_NOTIFICATIONS]). */
    val NOTIFICATIONS: Array<String> = arrayOf(POST_NOTIFICATIONS)
}

/**
 * Whether the OS knows [permission] as a runtime grant on [sdkInt]. Only
 * POST_NOTIFICATIONS is version-scoped today (it arrived in API 33); below
 * that the platform grants notification posting unconditionally, so a query
 * for it would report DENIED and the toggle would ask for nothing.
 */
internal fun isRuntimePermissionApplicable(permission: String, sdkInt: Int): Boolean =
    permission != TrackingPermissions.POST_NOTIFICATIONS || sdkInt >= 33
