package com.mycorrhizal.crm.data.repository

/**
 * Synchronous bridge between [AppSettingsRepositoryImpl]'s persisted language
 * override and the Activity's `attachBaseContext` (which runs before Hilt
 * injection is available). The repository writes [languageTag] on hydrate and
 * on every set; the Activity reads it to wrap its base context in the chosen
 * locale so `values-XX` resources resolve correctly for the very first frame.
 */
object AppLocale {
    @Volatile
    var languageTag: String? = null
}
