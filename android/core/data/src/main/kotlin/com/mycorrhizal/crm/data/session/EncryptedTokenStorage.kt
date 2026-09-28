package com.mycorrhizal.crm.data.session

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * EncryptedSharedPreferences-backed [TokenStorage] (ticket §9.1: AES-256-GCM,
 * key in Android Keystore). The JWT is a credential — this file never logs it.
 *
 * ADR 0028 Decision 1: each server profile's JWT lives under its own
 * `jwt:<profileId>` key, so switching profiles keeps the sessions isolated.
 * [loadLegacy]/[clearLegacy] expose the pre-profiles single `jwt` slot for the
 * one-time migration only.
 */
class EncryptedTokenStorage(context: Context) : TokenStorage {

    private val prefs: SharedPreferences = run {
        val masterKey = MasterKey.Builder(context)
            .setKeyScheme(MasterKey.KeyScheme.AES256_GCM)
            .build()
        EncryptedSharedPreferences.create(
            context,
            FILE_NAME,
            masterKey,
            EncryptedSharedPreferences.PrefKeyEncryptionScheme.AES256_SIV,
            EncryptedSharedPreferences.PrefValueEncryptionScheme.AES256_GCM,
        )
    }

    override suspend fun save(profileId: String, token: String) {
        prefs.edit().putString(keyFor(profileId), token).apply()
    }

    override suspend fun load(profileId: String): String? =
        prefs.getString(keyFor(profileId), null)

    override suspend fun clear(profileId: String) {
        prefs.edit().remove(keyFor(profileId)).apply()
    }

    override suspend fun loadLegacy(): String? = prefs.getString(KEY_LEGACY_JWT, null)

    override suspend fun clearLegacy() {
        prefs.edit().remove(KEY_LEGACY_JWT).apply()
    }

    private fun keyFor(profileId: String): String = "$KEY_JWT_PREFIX$profileId"

    companion object {
        private const val FILE_NAME = "secure_session"

        /** The pre-profiles single JWT slot. */
        private const val KEY_LEGACY_JWT = "jwt"

        /** Per-profile JWT slot prefix (ADR 0028 Decision 1). */
        private const val KEY_JWT_PREFIX = "jwt:"
    }
}
