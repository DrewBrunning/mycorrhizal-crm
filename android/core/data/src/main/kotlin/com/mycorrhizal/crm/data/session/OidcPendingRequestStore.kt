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
 * Storage for the pending OIDC request. The verifier is a short-lived secret,
 * so the production implementation encrypts it at rest.
 */
interface OidcPendingRequestStore {
    suspend fun save(state: String, codeVerifier: String)
    suspend fun load(): OidcPendingRequest?
    suspend fun clear()
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

    override suspend fun save(state: String, codeVerifier: String) { // # pragma: no cover — needs a real Android Keystore
        prefs.edit() // # pragma: no cover
            .putString(KEY_STATE, state) // # pragma: no cover
            .putString(KEY_VERIFIER, codeVerifier) // # pragma: no cover
            .putLong(KEY_CREATED_AT, System.currentTimeMillis()) // # pragma: no cover
            .apply() // # pragma: no cover
    }

    override suspend fun load(): OidcPendingRequest? { // # pragma: no cover — needs a real Android Keystore
        val state = prefs.getString(KEY_STATE, null) ?: return null // # pragma: no cover
        val verifier = prefs.getString(KEY_VERIFIER, null) ?: return null // # pragma: no cover
        return OidcPendingRequest( // # pragma: no cover
            state = state, // # pragma: no cover
            codeVerifier = verifier, // # pragma: no cover
            createdAtMillis = prefs.getLong(KEY_CREATED_AT, 0L), // # pragma: no cover
        )
    }

    override suspend fun clear() { // # pragma: no cover — needs a real Android Keystore
        prefs.edit().clear().apply() // # pragma: no cover
    }

    companion object {
        private const val FILE_NAME = "secure_oidc_pending"
        private const val KEY_STATE = "state"
        private const val KEY_VERIFIER = "code_verifier"
        private const val KEY_CREATED_AT = "created_at"
    }
}
