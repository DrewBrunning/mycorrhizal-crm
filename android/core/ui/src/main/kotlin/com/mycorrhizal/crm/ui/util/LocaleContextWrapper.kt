package com.mycorrhizal.crm.ui.util

import android.content.Context
import android.content.res.Configuration
import java.util.Locale

/**
 * M25: applies the user's chosen UI language to a base context so Android
 * resource resolution (`values-XX`) follows the app setting rather than the
 * device locale. The Activity overrides `attachBaseContext` with this wrapper;
 * the value comes from [com.mycorrhizal.crm.data.repository.AppLocale]'s
 * synchronous cache (hydrated at startup, kept fresh on change).
 */
object LocaleContextWrapper {

    /** Wraps [base] with a configuration localized to [languageTag], or returns [base] unchanged when null/blank. */
    fun wrap(base: Context, languageTag: String?): Context {
        if (languageTag.isNullOrBlank()) return base
        val locale = Locale.forLanguageTag(languageTag)
        val config = Configuration(base.resources.configuration)
        config.setLocale(locale)
        return base.createConfigurationContext(config)
    }
}
