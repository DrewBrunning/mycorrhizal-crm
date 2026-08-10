package com.mycorrhizal.crm.data.session

import android.content.Context
import android.content.SharedPreferences
import androidx.security.crypto.EncryptedSharedPreferences
import androidx.security.crypto.MasterKey

/**
 * EncryptedSharedPreferences-backed [TokenStorage] (ticket §9.1: AES-256-GCM,
 * key in Android Keystore). The JWT is a credential — this file never logs it.
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

    override suspend fun save(token: String) {
        prefs.edit().putString(KEY_JWT, token).apply()
    }

    override suspend fun load(): String? = prefs.getString(KEY_JWT, null)

    override suspend fun clear() {
        prefs.edit().remove(KEY_JWT).apply()
    }

    companion object {
        private const val FILE_NAME = "secure_session"
        private const val KEY_JWT = "jwt"
    }
}
