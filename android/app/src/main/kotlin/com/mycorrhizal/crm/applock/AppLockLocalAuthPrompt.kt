package com.mycorrhizal.crm.applock

import android.app.Activity
import androidx.fragment.app.FragmentActivity
import com.mycorrhizal.crm.data.auth.LocalAuthOutcome
import com.mycorrhizal.crm.data.auth.LocalAuthPrompt
import javax.inject.Inject
import javax.inject.Singleton

/** Creates the real OS-backed prompter for an Activity; a fake in JVM tests. */
fun interface AppUnlockPrompterFactory {
    fun create(activity: FragmentActivity): AppUnlockPrompter
}

/**
 * Production [LocalAuthPrompt]: reuses the app lock's CryptoObject-bound
 * biometric/device-credential prompt, so login-screen biometric re-auth has the
 * same OS-key binding as the app-lock gate (CodeQL
 * `android/insecure-local-authentication`).
 */
@Singleton
class AppLockLocalAuthPrompt @Inject constructor(
    private val prompterFactory: AppUnlockPrompterFactory
) : LocalAuthPrompt {
    override suspend fun requestUnlock(
        activity: Activity,
        title: String,
        subtitle: String,
        negativeButtonText: String?
    ): LocalAuthOutcome {
        val fragmentActivity = activity as? FragmentActivity
            ?: return LocalAuthOutcome.NOT_AVAILABLE
        return prompterFactory.create(fragmentActivity)
            .requestUnlock(title, subtitle, negativeButtonText)
            .toLocalAuthOutcome()
    }
}

/** The one mapping between the app-lock outcome and the login-screen seam. */
internal fun AppLockAuthOutcome.toLocalAuthOutcome(): LocalAuthOutcome = when (this) {
    AppLockAuthOutcome.Success -> LocalAuthOutcome.SUCCESS
    AppLockAuthOutcome.Cancelled -> LocalAuthOutcome.CANCELLED
    AppLockAuthOutcome.NotAvailable -> LocalAuthOutcome.NOT_AVAILABLE
    AppLockAuthOutcome.Error -> LocalAuthOutcome.ERROR
}
