package com.mycorrhizal.crm.data.session

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * How long a pending request stays valid. Mirrors the backend's 600 s handshake
 * window: a callback that arrives later than this is refused even if its state
 * happens to match a long-forgotten request.
 */
const val OIDC_PENDING_REQUEST_TTL_MILLIS = 10 * 60 * 1000L

/**
 * Issue #965: the app's half of an in-flight Android OIDC login. [state] is the
 * CSRF nonce echoed by the backend and checked before redemption; [codeVerifier]
 * is the PKCE secret that never leaves the device until the code is exchanged.
 *
 * Persisted (not in-memory) because the browser can push the app out of memory
 * while the user authenticates at the IdP; a cold-start deep link must still
 * find the values to verify the callback.
 */
data class OidcPendingRequest(
    val state: String,
    val codeVerifier: String,
    val createdAtMillis: Long,
) {
    /** True once the request is older than [OIDC_PENDING_REQUEST_TTL_MILLIS]. */
    fun isExpired(nowMillis: Long = System.currentTimeMillis()): Boolean =
        nowMillis - createdAtMillis > OIDC_PENDING_REQUEST_TTL_MILLIS
}

/**
 * Storage for the pending OIDC request, keyed by server-profile ID (ADR 0028
 * Decision 1: keyed the same way as `jwt:<id>`). The verifier is a short-lived
 * secret, so the production implementation encrypts it at rest.
 */
interface OidcPendingRequestStore {
    suspend fun save(profileId: String, state: String, codeVerifier: String)
    suspend fun load(profileId: String): OidcPendingRequest?
    suspend fun clear(profileId: String)

    /** The pre-profiles pending request, or null. */
    suspend fun loadLegacy(): OidcPendingRequest?

    /** Removes the pre-profiles pending slots once migrated. */
    suspend fun clearLegacy()
}

/**
 * EncryptedSharedPreferences-backed [OidcPendingRequestStore]. The code verifier
 * is a credential in the same sense as the device grant (possession + a stolen
 * code would redeem a session), so it gets the same Keystore-wrapped
 * EncryptedSharedPreferences envelope.
 */
class EncryptedOidcPendingRequestStore(context: Context) : OidcPendingRequestStore {

    private val prefs: SharedPreferences = run {
        val masterKey = MasterKey.Builder(context) // # pragma: no cover — needs a real Android Keystore
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM) // # pragma: no cover
            .build() // # pragma: no cover
        EncryptedSharedPreferences.create( // # pragma: no cover
            context, // # pragma: no cover
            FILE_NAME, // # pragma: no cover
            masterKey, // # pragma: no cover
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV, // # pragma: no cover
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM, // # pragma: no cover
        )
    }

    override suspend fun save(profileId: String, state: String, codeVerifier: String) { // # pragma: no cover — needs a real Android Keystore
        prefs.edit() // # pragma: no cover
            .putString(stateKey(profileId), state) // # pragma: no cover
            .putString(verifierKey(profileId), codeVerifier) // # pragma: no cover
            .putLong(createdAtKey(profileId), System.currentTimeMillis()) // # pragma: no cover
            .apply() // # pragma: no cover
    }

    override suspend fun load(profileId: String): OidcPendingRequest? { // # pragma: no cover — needs a real Android Keystore
        val state = prefs.getString(stateKey(profileId), null) ?: return null // # pragma: no cover
        val verifier = prefs.getString(verifierKey(profileId), null) ?: return null // # pragma: no cover
        return OidcPendingRequest( // # pragma: no cover
            state = state, // # pragma: no cover
            codeVerifier = verifier, // # pragma: no cover
            createdAtMillis = prefs.getLong(createdAtKey(profileId), 0L), // # pragma: no cover
        )
    }

    override suspend fun clear(profileId: String) { // # pragma: no cover — needs a real Android Keystore
        prefs.edit() // # pragma: no cover
            .remove(stateKey(profileId)) // # pragma: no cover
            .remove(verifierKey(profileId)) // # pragma: no cover
            .remove(createdAtKey(profileId)) // # pragma: no cover
            .apply() // # pragma: no cover
    }

    override suspend fun loadLegacy(): OidcPendingRequest? { // # pragma: no cover — needs a real Android Keystore
        val state = prefs.getString(KEY_LEGACY_STATE, null) ?: return null // # pragma: no cover
        val verifier = prefs.getString(KEY_LEGACY_VERIFIER, null) ?: return null // # pragma: no cover
        return OidcPendingRequest( // # pragma: no cover
            state = state, // # pragma: no cover
            codeVerifier = verifier, // # pragma: no cover
            createdAtMillis = prefs.getLong(KEY_LEGACY_CREATED_AT, 0L), // # pragma: no cover
        )
    }

    override suspend fun clearLegacy() { // # pragma: no cover — needs a real Android Keystore
        prefs.edit() // # pragma: no cover
            .remove(KEY_LEGACY_STATE) // # pragma: no cover
            .remove(KEY_LEGACY_VERIFIER) // # pragma: no cover
            .remove(KEY_LEGACY_CREATED_AT) // # pragma: no cover
            .apply() // # pragma: no cover
    }

    private fun stateKey(profileId: String): String = "$KEY_STATE_PREFIX$profileId"
    private fun verifierKey(profileId: String): String = "$KEY_VERIFIER_PREFIX$profileId"
    private fun createdAtKey(profileId: String): String = "$KEY_CREATED_AT_PREFIX$profileId"

    companion object {
        private const val FILE_NAME = "secure_oidc_pending"
        private const val KEY_LEGACY_STATE = "state"
        private const val KEY_LEGACY_VERIFIER = "code_verifier"
        private const val KEY_LEGACY_CREATED_AT = "created_at"
        private const val KEY_STATE_PREFIX = "state:"
        private const val KEY_VERIFIER_PREFIX = "code_verifier:"
        private const val KEY_CREATED_AT_PREFIX = "created_at:"
    }
}
