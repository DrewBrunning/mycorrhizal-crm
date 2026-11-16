package com.mycorrhizal.crm.data.auth

import android.app.Activity

/** Outcome of the OS local-auth prompt used by login-screen biometric re-auth. */
enum class LocalAuthOutcome {
    /** The user passed the biometric / device-credential check. */
    SUCCESS,

    /** The user dismissed the prompt. */
    CANCELLED,

    /** The device has no usable biometric or secure lock screen right now. */
    NOT_AVAILABLE,

    /** The OS reported a transient error. */
    ERROR
}

/**
 * Seam over the platform's local-auth prompt. The app module supplies the
 * CryptoObject-bound `BiometricPrompt` implementation (the same secure surface
 * the app lock uses); feature modules only see this interface, so the login
 * flow is testable without an Activity or an OS dialog.
 */
fun interface LocalAuthPrompt {
    suspend fun requestUnlock(
        activity: Activity,
        title: String,
        subtitle: String,
        negativeButtonText: String?
    ): LocalAuthOutcome
}
