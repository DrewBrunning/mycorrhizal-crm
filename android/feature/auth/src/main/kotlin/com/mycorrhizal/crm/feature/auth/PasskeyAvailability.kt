package com.mycorrhizal.crm.feature.auth

import com.mycorrhizal.crm.model.network.includesPasskey
import com.mycorrhizal.crm.model.network.isPasskeyOnly
import dagger.Binds
import dagger.Module
import dagger.hilt.InstallIn
import dagger.hilt.components.SingletonComponent
import javax.inject.Inject

/**
 * Issue #1293 / ADR 0034 Decision 4 — the single gate for "can this build,
 * against this server, run a passkey ceremony right now".
 *
 * The ceremony slice (S3) replaces [NoPasskeyAvailability] with an
 * implementation that is true only when the active Remote profile's `/health`
 * reports the `webauthn_android` capability token AND Credential Manager is
 * usable on the device. Until then it is always false, so every account whose
 * `methods` names a passkey gets the degraded recovery-code state.
 */
fun interface PasskeyAvailability {
    fun isAvailable(): Boolean
}

/** S1 default: the ceremony does not exist yet, so a passkey is never usable. */
class NoPasskeyAvailability @Inject constructor() : PasskeyAvailability {
    override fun isAvailable(): Boolean = false
}

@Module
@InstallIn(SingletonComponent::class)
abstract class PasskeyAvailabilityModule {
    @Binds
    abstract fun bindPasskeyAvailability(impl: NoPasskeyAvailability): PasskeyAvailability
}

/** What the 2FA step of login should present, derived from the account's enrolled methods. */
enum class TwoFactorPrompt {
    /** Legacy / TOTP-only / unknown `methods`: the code field with today's copy. */
    STANDARD,

    /** TOTP plus a passkey we cannot run: the normal code field plus a note that passkeys are unavailable. */
    CODE_WITH_PASSKEY_NOTE,

    /** Passkey-only and unusable: steer to a recovery code (the same code field) with the unavailable note. */
    RECOVERY_CODE_ONLY,
}

/**
 * Pure routing for the 2FA step. Absent (older server) or passkey-free
 * [methods] is always [TwoFactorPrompt.STANDARD]. When the gate is open
 * ([passkeyAvailable]) the passkey path is S3's to add; the code field stays
 * reachable either way, so this returns STANDARD there too.
 */
fun twoFactorPrompt(methods: List<String>?, passkeyAvailable: Boolean): TwoFactorPrompt = when {
    passkeyAvailable || !methods.includesPasskey() -> TwoFactorPrompt.STANDARD
    methods.isPasskeyOnly() -> TwoFactorPrompt.RECOVERY_CODE_ONLY
    else -> TwoFactorPrompt.CODE_WITH_PASSKEY_NOTE
}
