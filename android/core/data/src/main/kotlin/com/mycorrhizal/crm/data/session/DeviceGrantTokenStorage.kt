package com.mycorrhizal.crm.data.session

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/** The pre-profiles device grant (issue #722), read once for the migration. */
data class LegacyDeviceGrant(val token: String, val id: Long?)

/**
 * Stores the plaintext device grant (issue #722's fully-biometric-login
 * layer), keyed by server-profile ID (ADR 0028 Decision 1: the device-grant
 * token and the OIDC pending request are keyed the same way as `jwt:<id>`).
 * The grant is a long-lived credential — "possession of this token"
 * is what the server accepts as proof the enrolled device is present — so it
 * gets the same EncryptedSharedPreferences + Keystore `MasterKey` envelope the
 * session JWT already uses (`EncryptedTokenStorage`), never plaintext prefs.
 */
interface DeviceGrantTokenStorage {
    suspend fun save(profileId: String, token: String, id: Long)
    suspend fun loadToken(profileId: String): String?
    suspend fun loadGrantId(profileId: String): Long?
    suspend fun clear(profileId: String)

    /** The pre-profiles `device_grant` / `device_grant_id` pair, or null. */
    suspend fun loadLegacy(): LegacyDeviceGrant?

    /** Removes the pre-profiles grant slots once migrated. */
    suspend fun clearLegacy()
}

/**
 * EncryptedSharedPreferences-backed [DeviceGrantTokenStorage]. Every method is
 * exercised only via instrumented tests on a real Keystore (the same reason
 * `EncryptedTokenStorage` and `RoomPassphraseStore` cannot run under
 * Robolectric); the backend is pinned by [EncryptedDeviceGrantStorageGuardTest].
 */
class EncryptedDeviceGrantTokenStorage(context: Context) : DeviceGrantTokenStorage {

    private val prefs: SharedPreferences = run {
        val masterKey = MasterKey.Builder(context) // # pragma: no cover — needs a real Android Keystore (see EncryptedDeviceGrantStorageGuardTest)
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

    override suspend fun save(profileId: String, token: String, id: Long) { // # pragma: no cover — needs a real Android Keystore (see EncryptedDeviceGrantStorageGuardTest)
        prefs.edit() // # pragma: no cover
            .putString(tokenKey(profileId), token) // # pragma: no cover
            .putLong(idKey(profileId), id) // # pragma: no cover
            .apply() // # pragma: no cover
    }

    override suspend fun loadToken(profileId: String): String? =
        prefs.getString(tokenKey(profileId), null) // # pragma: no cover — needs a real Android Keystore

    override suspend fun loadGrantId(profileId: String): Long? =
        prefs.getLong(idKey(profileId), 0L).takeIf { it != 0L } // # pragma: no cover — needs a real Android Keystore

    override suspend fun clear(profileId: String) { // # pragma: no cover — needs a real Android Keystore (see EncryptedDeviceGrantStorageGuardTest)
        prefs.edit() // # pragma: no cover
            .remove(tokenKey(profileId)) // # pragma: no cover
            .remove(idKey(profileId)) // # pragma: no cover
            .apply() // # pragma: no cover
    }

    override suspend fun loadLegacy(): LegacyDeviceGrant? { // # pragma: no cover — needs a real Android Keystore
        val token = prefs.getString(KEY_LEGACY_GRANT, null) ?: return null // # pragma: no cover
        val id = prefs.getLong(KEY_LEGACY_GRANT_ID, 0L).takeIf { it != 0L } // # pragma: no cover
        return LegacyDeviceGrant(token = token, id = id) // # pragma: no cover
    }

    override suspend fun clearLegacy() { // # pragma: no cover — needs a real Android Keystore
        prefs.edit() // # pragma: no cover
            .remove(KEY_LEGACY_GRANT) // # pragma: no cover
            .remove(KEY_LEGACY_GRANT_ID) // # pragma: no cover
            .apply() // pragma: no cover
    }

    private fun tokenKey(profileId: String): String = "$KEY_GRANT_PREFIX$profileId"
    private fun idKey(profileId: String): String = "$KEY_GRANT_ID_PREFIX$profileId"

    companion object {
        private const val FILE_NAME = "secure_device_grant"
        private const val KEY_LEGACY_GRANT = "device_grant"
        private const val KEY_LEGACY_GRANT_ID = "device_grant_id"
        private const val KEY_GRANT_PREFIX = "device_grant:"
        private const val KEY_GRANT_ID_PREFIX = "device_grant_id:"
    }
}
