package com.mycorrhizal.crm.data.repository

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import com.mycorrhizal.crm.domain.repository.AppSettingsRepository
import dagger.hilt.android.qualifiers.ApplicationContext
import javax.inject.Inject
import javax.inject.Singleton
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.appSettingsDataStore: DataStore<Preferences> by preferencesDataStore(name = "app_settings")

/**
 * M25: device-side app settings (theme + UI-language override) stored in a
 * single DataStore file. Theme is a local preference with no server endpoint
 * (the web's localStorage equivalent); the language override lets
 * `attachBaseContext` resolve `values-XX` resources for the chosen UI
 * language before/without any server round-trip.
 *
 * The language's synchronous cache lives in [AppLocale], which the Activity
 * reads in `attachBaseContext` — DataStore's first read is async, and no
 * composable can run before that method returns. The cache is hydrated at app
 * startup (see [hydrate], called from the Application's `onCreate`), so it is
 * already correct for the very first frame, and kept fresh on every write.
 */
@Singleton
class AppSettingsRepositoryImpl @Inject constructor(
    @ApplicationContext private val context: Context,
) : AppSettingsRepository {

    private val themeKey = stringPreferencesKey("theme_preference")
    private val languageKey = stringPreferencesKey("language_override")

    @Volatile
    private var cachedTheme: String = AppSettingsRepository.THEME_SYSTEM

    /** Blocking read of the persisted prefs into the sync caches. One tiny prefs file, called once at startup. */
    suspend fun hydrate() {
        val prefs = context.appSettingsDataStore.data.first()
        AppLocale.languageTag = prefs[languageKey]
        cachedTheme = prefs[themeKey] ?: AppSettingsRepository.THEME_SYSTEM
    }

    override fun themePreference(): Flow<String> =
        context.appSettingsDataStore.data.map { it[themeKey] ?: AppSettingsRepository.THEME_SYSTEM }

    override suspend fun setThemePreference(preference: String) {
        cachedTheme = preference
        context.appSettingsDataStore.edit { it[themeKey] = preference }
    }

    override fun languageOverride(): Flow<String?> =
        context.appSettingsDataStore.data.map { it[languageKey] }

    override suspend fun setLanguageOverride(languageTag: String?) {
        AppLocale.languageTag = languageTag
        context.appSettingsDataStore.edit {
            if (languageTag == null) it.remove(languageKey) else it[languageKey] = languageTag
        }
    }

    override fun currentThemePreference(): String = cachedTheme
}
