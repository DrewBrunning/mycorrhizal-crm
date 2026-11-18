package com.mycorrhizal.crm.model.util

import java.util.Locale

/**
 * Issue #2654: the [Locale] the Material date picker builds its calendar from, which decides the
 * first day of the week. The device language is kept (weekday and month names stay in the app's
 * language); only the week start is set:
 *  - a first-day-of-week setting on the device (a `-u-fw-` locale extension, e.g. Android 14's
 *    regional preferences) always wins;
 *  - otherwise it follows the user's `date_format` convention: month-first (`us*`) formats start
 *    on Sunday, every other format on Monday. A US-region device with a day-first format therefore
 *    no longer shows a Sunday-first calendar next to `10.11.2026`.
 */
object DatePickerLocale {
    private val SUNDAY_FIRST = setOf(DateFormat.US, DateFormat.US_MMM, DateFormat.US_MMMM)

    fun resolve(deviceLocale: Locale, dateFormat: String): Locale {
        if (deviceLocale.getUnicodeLocaleType("fw") != null) return deviceLocale
        val firstDay = if (dateFormat in SUNDAY_FIRST) "sun" else "mon"
        return Locale.Builder()
            .setLocale(deviceLocale)
            .setUnicodeLocaleKeyword("fw", firstDay)
            .build()
    }
}
