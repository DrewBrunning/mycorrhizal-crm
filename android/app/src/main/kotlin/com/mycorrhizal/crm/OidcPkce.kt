package com.mycorrhizal.crm

import java.security.MessageDigest
import java.security.SecureRandom
import java.util.Base64

/**
 * Issue #965: the Android half of the OIDC native-return PKCE binding.
 *
 * The custom scheme the callback returns on (`mycorrhizal://oidc/callback`) is
 * first-come-first-served and can be intercepted by any other app. To make the
 * redirect worthless to an interceptor, the app generates a PKCE code verifier
 * here, sends only its SHA-256 challenge on the login start, keeps the verifier
 * on-device, and redeems the callback's single-use code with it over a direct
 * HTTPS request. A stolen redirect carries the code but never the verifier.
 *
 * Pure and deterministic (the [SecureRandom] is injectable) so the RFC 7636
 * transform is unit-testable without an Android host.
 */
internal object OidcPkce {

    // 32 random bytes -> 43 base64url characters, the RFC 7636 §4.1 minimum
    // length for an S256 verifier.
    private const val RANDOM_BYTES = 32

    // RFC 7636 §4.2: BASE64URL-ENCODE, no padding.
    private val urlEncoder: Base64.Encoder = Base64.getUrlEncoder().withoutPadding()

    /** A fresh code verifier: 32 random bytes as 43 base64url characters. */
    fun generateVerifier(random: SecureRandom = SecureRandom()): String =
        randomToken(random)

    /**
     * The app's CSRF state nonce. The backend echoes it in the deep link and
     * [verifyState] checks it before the code is redeemed, so an unsolicited
     * callback (one this app never started) is refused.
     */
    fun generateState(random: SecureRandom = SecureRandom()): String =
        randomToken(random)

    /** The S256 code challenge for [verifier]: BASE64URL(SHA-256(verifier)). */
    fun challenge(verifier: String): String {
        val digest = MessageDigest.getInstance("SHA-256")
            .digest(verifier.toByteArray(Charsets.US_ASCII))
        return urlEncoder.encodeToString(digest)
    }

    /**
     * Constant-time comparison of the state this app stored against the one
     * the callback returned. A null/blank either side is not a match, and a
     * mismatch (or an empty pending state) fails closed.
     */
    fun verifyState(pendingState: String?, returnedState: String?): Boolean {
        if (pendingState.isNullOrEmpty() || returnedState.isNullOrEmpty()) return false
        return MessageDigest.isEqual(
            pendingState.toByteArray(Charsets.US_ASCII),
            returnedState.toByteArray(Charsets.US_ASCII),
        )
    }

    private fun randomToken(random: SecureRandom): String {
        val bytes = ByteArray(RANDOM_BYTES)
        random.nextBytes(bytes)
        return urlEncoder.encodeToString(bytes)
    }
}
