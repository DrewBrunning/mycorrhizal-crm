package com.mycorrhizal.crm.data.session

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first

private val Context.dataStore by preferencesDataStore(name = "session_prefs")

/**
 * DataStore-backed [SessionPrefsStorage] for non-secret preferences (server
 * URL). The server URL is not a credential, so plain DataStore is fine.
 */
class DataStoreSessionPrefsStorage(context: Context) : SessionPrefsStorage {

    private val dataStore = context.dataStore

    override suspend fun save(serverUrl: String?) {
        dataStore.edit { prefs ->
            if (serverUrl == null) prefs.remove(KEY_SERVER_URL)
            else prefs[KEY_SERVER_URL] = serverUrl
        }
    }

    override suspend fun loadServerUrl(): String? =
        dataStore.data.first()[KEY_SERVER_URL]

    override suspend fun clear() {
        dataStore.edit { prefs -> prefs.remove(KEY_SERVER_URL) }
    }

    companion object {
        private val KEY_SERVER_URL = stringPreferencesKey("server_url")
    }
}
