package com.mycorrhizal.crm.data.passkey

import com.mycorrhizal.crm.model.network.includesPasskey
import com.mycorrhizal.crm.model.network.isPasskeyOnly

/**
 * What the 2FA step of login should present, derived from the account's
 * enrolled methods and the passkey gate
 * ([PasskeyAvailability], issue #1293 /
 * ADR 0034 Decision 4). The code field is on screen in EVERY variant, so the
 * TOTP / recovery-code path is always reachable.
 */
enum class SecondFactorPrompt {
    /** Legacy / TOTP-only / unknown `methods`: the code field with today's copy. */
    STANDARD,

    /** TOTP plus a passkey we cannot run: the normal code field plus a note that passkeys are unavailable. */
    CODE_WITH_PASSKEY_NOTE,

    /** Passkey-only and unusable: steer to a recovery code (the same code field) with the unavailable note. */
    RECOVERY_CODE_ONLY,

    /** Gate open, TOTP and a passkey enrolled: a "Use a passkey" action next to the code field. */
    CODE_OR_PASSKEY,

    /** Gate open, passkey-only: "Use a passkey" is primary, the code field takes a recovery code. */
    PASSKEY_OR_RECOVERY_CODE,
}

/** True when the prompt offers the passkey action. */
val SecondFactorPrompt.offersPasskey: Boolean
    get() = this == SecondFactorPrompt.CODE_OR_PASSKEY || this == SecondFactorPrompt.PASSKEY_OR_RECOVERY_CODE

/**
 * Pure routing for the 2FA step. Absent (older server) or passkey-free
 * [methods] is always [SecondFactorPrompt.STANDARD].
 */
fun secondFactorPrompt(methods: List<String>?, passkeyAvailable: Boolean): SecondFactorPrompt = when {
    !methods.includesPasskey() -> SecondFactorPrompt.STANDARD
    passkeyAvailable ->
        if (methods.isPasskeyOnly()) SecondFactorPrompt.PASSKEY_OR_RECOVERY_CODE else SecondFactorPrompt.CODE_OR_PASSKEY
    methods.isPasskeyOnly() -> SecondFactorPrompt.RECOVERY_CODE_ONLY
    else -> SecondFactorPrompt.CODE_WITH_PASSKEY_NOTE
}

/**
 * Why a passkey attempt dropped the step to the degraded (code-only) state at
 * runtime. Surfaced as distinct copy; both keep the code field.
 */
enum class PasskeyIssue {
    /** The app is not associated with the server's RP ID: "this server isn't set up for Android passkeys". */
    NOT_ASSOCIATED,

    /** No credential provider on the device. */
    NO_PROVIDER,
}
