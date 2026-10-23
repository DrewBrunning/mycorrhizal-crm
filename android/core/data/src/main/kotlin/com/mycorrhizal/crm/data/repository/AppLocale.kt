package com.mycorrhizal.crm.data.repository

import android.app.LocaleManager
import android.content.Context
import android.os.Build
import android.os.LocaleList

/**
 * Synchronous bridge between [AppSettingsRepositoryImpl]'s persisted language
 * override and the Activity's `attachBaseContext` (which runs before Hilt
 * injection is available). The repository writes [languageTag] on hydrate and
 * on every set; the Activity reads it to wrap its base context in the chosen
 * locale so `values-XX` resources resolve correctly for the very first frame.
 *
 * Issue #1921: from API 33 the OS owns a per-app locale (the
 * `android:localeConfig` entry in the manifest exposes the five supported
 * languages to the system picker). On those devices the in-app choice is
 * mirrored to [LocaleManager] and the system's explicit locale wins in
 * `attachBaseContext`, so a change made from system Settings is not
 * overwritten by the local cache.
 */
object AppLocale {
    @Volatile
    var languageTag: String? = null

    /**
     * Whether [languageTag] should be applied by `attachBaseContext` on
     * [sdkInt]. Below API 33 there is no OS per-app locale, so the local
     * override is the only source. From API 33, an explicitly selected system
     * locale is authoritative; the local override remains the fallback until
     * the app has mirrored its choice to the OS (and for a fresh upgrade whose
     * install still only has the in-app setting).
     */
    fun shouldApplyInAppOverride(sdkInt: Int, systemHasLocale: Boolean): Boolean =
        sdkInt < Build.VERSION_CODES.TIRAMISU || !systemHasLocale

    /** The OS per-app language tag on API 33+, or null when unset/unsupported. */
    fun systemLanguageTag(context: Context): String? {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return null
        val locales = context.getSystemService(LocaleManager::class.java)?.applicationLocales
        return locales?.takeUnless { it.isEmpty }?.get(0)?.language
    }

    /**
     * Mirror the in-app choice to the OS per-app locale on API 33+. A null or
     * blank tag asks for the system default. No-op below API 33.
     */
    fun applyToSystem(context: Context, languageTag: String?) {
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) return
        val manager = context.getSystemService(LocaleManager::class.java) ?: return
        val locales = if (languageTag.isNullOrBlank()) {
            LocaleList.getEmptyLocaleList()
        } else {
            LocaleList.forLanguageTags(languageTag)
        }
        if (manager.applicationLocales != locales) manager.applicationLocales = locales
    }
}
