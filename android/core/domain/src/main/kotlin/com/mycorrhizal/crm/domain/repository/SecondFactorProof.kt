package com.mycorrhizal.crm.domain.repository

/**
 * Issue #1337: the live second-factor proof the server requires before an
 * ADDITIONAL factor is enrolled (a passkey via `webauthn/register/begin`, or
 * TOTP via `users/2fa/setup`) once the account already holds one. The first
 * factor needs none. Same bar as removing a passkey.
 */
sealed interface SecondFactorProof {
    /** A current TOTP code or an unused recovery code. */
    data class Code(val code: String) : SecondFactorProof

    /**
     * A serialized PublicKeyCredential assertion, obtained against the options
     * of `webauthn/assert/begin` (PasskeyRepository.beginProof) with an
     * existing passkey.
     */
    data class Assertion(val json: String) : SecondFactorProof
}
