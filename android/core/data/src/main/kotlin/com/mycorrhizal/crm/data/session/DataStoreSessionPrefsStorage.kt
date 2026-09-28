package com.mycorrhizal.crm.data.session

import android.content.Context
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.first

private val Context.dataStore by preferencesDataStore(name = "session_prefs")

/**
 * DataStore-backed [SessionPrefsStorage] for non-secret preferences. ADR 0028
 * Decision 1: the server profile list and the active profile ID live here (the
 * old single `server_url` key is retained only for the one-time in-place
 * migration). None of these are credentials.
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

    override suspend fun saveProfiles(snapshot: ProfilesSnapshot) {
        dataStore.edit { prefs ->
            if (snapshot.profiles.isEmpty()) {
                prefs.remove(KEY_PROFILES)
            } else {
                prefs[KEY_PROFILES] = ServerProfileCodec.encode(snapshot)
            }
            if (snapshot.activeProfileId == null) {
                prefs.remove(KEY_ACTIVE_PROFILE_ID)
            } else {
                prefs[KEY_ACTIVE_PROFILE_ID] = snapshot.activeProfileId
            }
        }
    }

    override suspend fun loadProfiles(): ProfilesSnapshot {
        val prefs = dataStore.data.first()
        return ServerProfileCodec.decode(
            raw = prefs[KEY_PROFILES],
            activeId = prefs[KEY_ACTIVE_PROFILE_ID],
        )
    }

    override suspend fun clear() {
        dataStore.edit { prefs ->
            prefs.remove(KEY_SERVER_URL)
            prefs.remove(KEY_PROFILES)
            prefs.remove(KEY_ACTIVE_PROFILE_ID)
        }
    }

    companion object {
        private val KEY_SERVER_URL = stringPreferencesKey("server_url")
        private val KEY_PROFILES = stringPreferencesKey("server_profiles")
        private val KEY_ACTIVE_PROFILE_ID = stringPreferencesKey("active_profile_id")
    }
}
